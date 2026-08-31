package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"equiptra/internal/db"
	"equiptra/internal/handlers"
	"equiptra/internal/middleware"
	"equiptra/internal/storage"
)

func main() {
	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	s3Client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("s3 client: %v", err)
	}
	if s3Client == nil {
		log.Printf("S3_BUCKET not set — product photo uploads are disabled")
	}

	api := &handlers.API{DB: pool, S3: s3Client}

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	if frontendOrigin == "" {
		frontendOrigin = "http://localhost:5173"
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{frontendOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Post("/api/auth/login", api.Login)
	r.Post("/api/auth/logout", api.Logout)

	// Public fault-report form: reachable by freelancers with no Equiptra
	// account, so it sits outside RequireAuth. OptionalAuth still injects
	// claims when a staff member happens to have a session, so the handler
	// can auto-fill the reporter rather than asking them to re-type it.
	// Rate-limited since it's an open write (and read) surface on the
	// public internet, not gated by auth at all.
	r.Route("/api/public", func(r chi.Router) {
		r.Use(middleware.RateLimit(20, time.Minute))
		r.Use(middleware.OptionalAuth)
		r.Get("/assets", api.SearchPublicAssets)
		r.Post("/fault-reports", api.CreateFaultReport)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(middleware.RequirePasswordSet)

		r.Get("/me", api.Me)

		r.Route("/products", func(r chi.Router) {
			r.Get("/", api.ListProducts)
			r.Get("/{id}", api.GetProduct)
			r.Get("/{id}/assets", api.ListProductAssets)
			// Create/edit intentionally open to any authenticated user for now
			// (admin vs standard permissions still TBD — see README). Delete and
			// photo upload stay admin-gated since they weren't part of that ask.
			r.Post("/", api.CreateProduct)
			r.Put("/{id}", api.UpdateProduct)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Delete("/{id}", api.DeleteProduct)
				r.Post("/{id}/photo/presign", api.PresignProductPhoto)
			})
		})

		r.Route("/assets", func(r chi.Router) {
			r.Get("/", api.ListAssets)
			r.Get("/{id}", api.GetAsset)
			r.Get("/{id}/history", api.GetAssetHistory)
			// Same as products above: create/edit open to any authenticated user
			// for now; delete stays admin-gated.
			r.Post("/", api.CreateAsset)
			r.Put("/{id}", api.UpdateAsset)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Delete("/{id}", api.DeleteAsset)
			})
		})

		r.Route("/projects", func(r chi.Router) {
			r.Get("/", api.ListProjects)
			r.Post("/", api.CreateProject)
			r.Get("/{id}", api.GetProject)
			r.Put("/{id}", api.UpdateProject)
			r.Delete("/{id}", api.DeleteProject)
			r.Post("/{id}/cancel", api.CancelProject)
			r.Get("/{id}/carnet", api.GetCarnetView)
			r.Get("/{id}/carnet/export.csv", api.ExportCarnetCSV)
			r.Get("/{id}/carnet/export.pdf", api.ExportCarnetPDF)
			r.Get("/{id}/delivery-note", api.GetDeliveryNoteView)
			r.Get("/{id}/delivery-note/export.pdf", api.ExportDeliveryNotePDF)
		})

		r.Route("/booking-requests", func(r chi.Router) {
			r.Get("/", api.ListBookingRequests)
			r.Post("/", api.CreateBookingRequest)
			r.Get("/{id}", api.GetBookingRequest)
			r.Put("/{id}", api.UpdateBookingRequest)
			r.Delete("/{id}", api.DeleteBookingRequest)
			r.Post("/{id}/cancel", api.CancelBookingRequest)
			r.Get("/{id}/allocations", api.ListAllocationsForRequest)
			r.Post("/{id}/allocations", api.CreateAllocation)
		})

		r.Route("/booking-allocations", func(r chi.Router) {
			r.Delete("/{id}", api.DeleteAllocation)
			r.Post("/{id}/checkout", api.CheckoutAllocation)
			r.Post("/{id}/checkin", api.CheckinAllocation)
		})

		r.Route("/service-records", func(r chi.Router) {
			r.Get("/", api.ListServiceRecords)
			r.Get("/{id}", api.GetServiceRecord)
			// Creation happens via check-in damage (CheckinAllocation) or the
			// public fault-report form (/api/public/fault-reports) — not here.
			r.Put("/{id}", api.UpdateServiceRecord)
		})

		// Account/login management is more sensitive than products/assets, so
		// unlike those this resource stays admin-only — except changing your
		// own password, which any authenticated user (including a
		// must_change_password-restricted session) can do.
		r.Route("/users", func(r chi.Router) {
			r.Patch("/me/password", api.ChangeOwnPassword)

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin)
				r.Get("/", api.ListUsers)
				r.Post("/", api.CreateUser)
				r.Patch("/{id}", api.UpdateUser)
				r.Patch("/{id}/password", api.AdminResetPassword)
				r.Delete("/{id}", api.DeleteUser)
			})
		})
	})

	// Render, Fly, and most PaaS platforms inject PORT and require the app
	// to bind to it; LISTEN_ADDR (full host:port) still wins if set, for
	// local dev / anywhere PORT isn't the convention.
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
	}
	log.Printf("equiptra api listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
