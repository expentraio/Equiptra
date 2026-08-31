// Package storage wraps Supabase Storage's native REST API for the
// product-photo upload flow. Replaces an earlier attempt that pointed the
// AWS S3 SDK at Supabase's S3-compatible endpoint, which failed with
// SignatureDoesNotMatch across multiple SDKs/credential pairs/regions — a
// SigV4 compatibility gap in Supabase's S3 shim, not a config mistake. The
// native REST API uses a plain bearer token, no request signing, so that
// whole problem class goes away. See
// docs/equiptra-photo-upload-addendum.md.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

type SupabaseClient struct {
	projectRef string
	serviceKey string
	httpClient *http.Client
}

// NewSupabaseClient builds a client from environment variables. Returns
// (nil, nil) if either var is unset — the photo feature is simply disabled
// rather than treated as a startup error, same as the old S3 client's
// convention.
//
//	SUPABASE_PROJECT_REF        e.g. "abcdefghijklmnop" (the subdomain of
//	                             https://{ref}.supabase.co)
//	SUPABASE_SERVICE_ROLE_KEY   server-side only — never sent to the
//	                             frontend, never logged
func NewSupabaseClient() *SupabaseClient {
	ref := os.Getenv("SUPABASE_PROJECT_REF")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if ref == "" || key == "" {
		return nil
	}
	return &SupabaseClient{
		projectRef: ref,
		serviceKey: key,
		httpClient: &http.Client{},
	}
}

// UploadObject PUTs a file to Supabase Storage's REST API with x-upsert
// set, so re-uploading to the same path replaces the existing object
// instead of erroring, and returns the object's public URL. The bucket
// must already be set to public in the Supabase dashboard — this client
// doesn't create or configure buckets.
//
// cacheControlMaxAgeSeconds sets how long the object may be cached by
// browsers/CDNs. Confirmed empirically against this project (the public
// docs/blog posts on this are inconsistent across API versions — see
// docs/equiptra-image-caching-addendum.md): the request must send a plain
// `cache-control: max-age=<seconds>` header — no "public," prefix, the
// server adds that itself when serving the object. Verified via
// Supabase's authenticated object-info endpoint (which reads the stored
// DB metadata directly, bypassing any CDN) that this is what actually
// gets persisted; a multipart `cacheControl` form field and a header
// value that already included "public," were both tried and did not
// take effect against this project.
func (c *SupabaseClient) UploadObject(ctx context.Context, bucket, path, contentType string, body []byte, cacheControlMaxAgeSeconds int) (publicURL string, err error) {
	url := fmt.Sprintf("https://%s.supabase.co/storage/v1/object/%s/%s", c.projectRef, bucket, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	// Supabase's API gateway (Kong) expects apikey on every request in
	// addition to the Authorization bearer token — omitting it produced a
	// confusing "Invalid Compact JWS" error when testing against a
	// newer-format sb_secret_... key, even though Authorization alone was
	// present and correct.
	req.Header.Set("apikey", c.serviceKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("cache-control", fmt.Sprintf("max-age=%d", cacheControlMaxAgeSeconds))
	req.Header.Set("x-upsert", "true")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("uploading to storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("storage upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return fmt.Sprintf("https://%s.supabase.co/storage/v1/object/public/%s/%s", c.projectRef, bucket, path), nil
}

// DownloadObject fetches an object's current bytes via the authenticated
// REST API (works regardless of whether the bucket is public). Used as a
// fallback by the cache-header repair script when a local source file is
// no longer available — the existing stored bytes are re-uploaded
// unchanged, just with the corrected cache-control header this time.
func (c *SupabaseClient) DownloadObject(ctx context.Context, bucket, path string) (body []byte, contentType string, err error) {
	url := fmt.Sprintf("https://%s.supabase.co/storage/v1/object/%s/%s", c.projectRef, bucket, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("apikey", c.serviceKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("downloading from storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, "", fmt.Errorf("storage download failed (%d): %s", resp.StatusCode, string(respBody))
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading response body: %w", err)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
