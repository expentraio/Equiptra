package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const productPhotosBucket = "product-photos"

var allowedPhotoContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

const maxPhotoSizeBytes = 5 << 20 // 5MB

// UploadProductPhoto proxies a multipart file upload to Supabase Storage's
// REST API using the service-role key (server-side only, never exposed to
// the browser) and saves the resulting public URL on the product. Admin-only,
// matching the existing pattern for product-editing actions that aren't
// open to any authenticated user (delete, this).
//
// Upload path is {product_id}.{ext} — simpler than the backfill's
// {product_id}_{sanitized-name} convention, since this is a fresh upload,
// not a filename inherited from CurrentRMS. x-upsert on the storage
// request means re-uploading for the same product replaces the existing
// photo rather than erroring or leaving an orphaned object behind.
func (a *API) UploadProductPhoto(w http.ResponseWriter, r *http.Request) {
	if a.Supabase == nil {
		writeError(w, http.StatusServiceUnavailable, "photo uploads aren't configured (SUPABASE_PROJECT_REF/SUPABASE_SERVICE_ROLE_KEY not set)")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var exists bool
	if err := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE id = $1)`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSizeBytes+1<<20) // headroom for multipart overhead
	if err := r.ParseMultipartForm(maxPhotoSizeBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "photo must be under 5MB")
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "no photo file provided (expected multipart field \"photo\")")
		return
	}
	defer file.Close()

	if header.Size > maxPhotoSizeBytes {
		writeError(w, http.StatusBadRequest, "photo must be under 5MB")
		return
	}

	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedPhotoContentTypes[strings.ToLower(contentType)]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported file type — use JPEG, PNG, or WEBP")
		return
	}

	body, err := io.ReadAll(io.LimitReader(file, maxPhotoSizeBytes+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read uploaded file")
		return
	}
	if len(body) > maxPhotoSizeBytes {
		writeError(w, http.StatusBadRequest, "photo must be under 5MB")
		return
	}

	path := strconv.FormatInt(id, 10) + "." + ext
	publicURL, err := a.Supabase.UploadObject(r.Context(), productPhotosBucket, path, contentType, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed: "+err.Error())
		return
	}

	p, err := scanProduct(a.DB.QueryRow(r.Context(), `
		UPDATE products SET image_url = $1, updated_at = now()
		WHERE id = $2
		RETURNING `+productSelectCols,
		publicURL, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		// Rare: the product was deleted between the existence check above and
		// this update. The photo's already in storage at this point (upsert
		// means a retry against the same id just overwrites it), so 404 is
		// the honest answer rather than pretending the upload didn't happen.
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo uploaded but saving image_url failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}
