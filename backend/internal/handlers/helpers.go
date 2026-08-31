package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"equiptra/internal/storage"
)

type API struct {
	DB *pgxpool.Pool
	// Supabase is nil when the product-photo feature is disabled
	// (SUPABASE_PROJECT_REF/SUPABASE_SERVICE_ROLE_KEY unset) — see
	// storage.NewSupabaseClient.
	Supabase *storage.SupabaseClient
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
