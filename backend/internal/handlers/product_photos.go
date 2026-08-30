package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var allowedPhotoContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

type presignPhotoRequest struct {
	ContentType string `json:"content_type"`
}

// PresignProductPhoto issues a short-lived S3 PUT URL for a product photo
// upload (brief §7). The browser uploads directly to S3 with the returned
// URL, then calls PUT /products/{id} with image_url set to get_url to
// finish. Admin-only, and a 503 if the feature isn't configured
// (S3_BUCKET unset) rather than a confusing lower-level error.
func (a *API) PresignProductPhoto(w http.ResponseWriter, r *http.Request) {
	if a.S3 == nil {
		writeError(w, http.StatusServiceUnavailable, "photo uploads aren't configured (S3_BUCKET not set)")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req presignPhotoRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ext, ok := allowedPhotoContentTypes[strings.ToLower(req.ContentType)]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported content_type — use image/jpeg, image/png, or image/webp")
		return
	}

	key := fmt.Sprintf("products/%d-%d.%s", id, time.Now().UnixNano(), ext)
	putURL, getURL, err := a.S3.PresignPutURL(r.Context(), key, req.ContentType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not presign upload: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"put_url": putURL, "get_url": getURL})
}
