# Equiptra Build Brief — Addendum: Product Photo Uploads (v1.1)

*This addendum should be merged into the main build brief, replacing the S3-compatible-client
approach described in the original §7 (Product thumbnails).*

## Root cause (for context, not action)

The original plan pointed the existing Go S3 client (built against local MinIO) at Supabase
Storage's S3-compatible endpoint. This failed with `SignatureDoesNotMatch` across multiple SDKs,
credential pairs, and regions — a pattern consistent with an AWS SigV4 compatibility gap in
Supabase's S3 shim (newer SDKs add checksum/chunked-signing headers the shim doesn't fully
support), not a configuration mistake. Free-tier Supabase support couldn't help within a useful
timeframe. Rather than keep debugging signature compatibility, this addendum switches to
Supabase Storage's **native REST API**, which uses a plain bearer token — no request signing
involved, so the whole problem class goes away.

## Approach: server-side proxy upload

The Go backend uploads on the frontend's behalf, using a service-role key that never reaches
the browser:

1. Frontend sends the photo as a multipart file to a new Go endpoint.
2. The Go backend forwards it to Supabase Storage's REST API (`POST /storage/v1/object/{bucket}/{path}`)
   with `Authorization: Bearer <service_role_key>`.
3. On success, the backend constructs the public URL and saves it to `products.image_url`.
4. The backend returns the URL to the frontend, which updates the UI immediately.

This avoids needing presigned upload URLs or exposing any Supabase credential to the browser —
simpler to build and secure by construction for a 4-user internal tool with modest upload volume.

## Configuration

- New bucket in Supabase Storage: `product-photos`, set to public (matches how these images are
  used — displayed directly in the app, no access control needed on the images themselves).
- New environment variables for the Go backend: `SUPABASE_PROJECT_REF`, `SUPABASE_SERVICE_ROLE_KEY`.
  The service role key must only ever live server-side — never sent to the frontend, never logged.

## Backend

- **`POST /api/products/:id/photo`** — admin-only (matches the existing pattern for
  product-editing actions). Accepts a multipart file upload.
  - Validate file type (jpg/png/webp) and a reasonable size cap (e.g. 5MB) before uploading.
  - Upload path convention: `{product_id}.{ext}` in the `product-photos` bucket — simpler than
    the migration's `{product_id}_{sanitized-name}` convention since this is a fresh upload, not
    a filename inherited from CurrentRMS.
  - Use `x-upsert: true` on the upload request so re-uploading a photo for the same product
    replaces the existing one rather than erroring.
  - On success, update `products.image_url` to the public URL
    (`https://{project_ref}.supabase.co/storage/v1/object/public/product-photos/{path}`) and
    return the updated product.

## Frontend

- Upload control on the product edit screen — the mockup already reserves thumbnail space above
  the product name; wire a file picker there.
- Thumbnail rendering on the product grid/list and asset detail view, using `image_url` when set.
- Fallback: a simple placeholder icon per category (already specified in the original brief)
  for any product without a photo — most of these are one-off rack builds/flight cases that
  likely never had a CurrentRMS photo either.

## One-off backfill (separate from the ongoing feature)

693 of 743 product photos were already extracted from CurrentRMS before its subscription lapsed,
saved locally as `{product_id}_{sanitized-product-name}.{ext}`, with a `_manifest.csv` tracking
status per product. These need to be uploaded once, using the same native REST API technique
(proving the approach works before relying on it for the live upload button):

- A standalone script (doesn't need to touch the Go backend) iterates the local photo folder,
  uploads each file to the `product-photos` bucket via the same REST API endpoint and auth
  approach described above, and updates `products.image_url` by matching the `product_id`
  embedded in each filename.
- Log successes/failures the same way `extract_photos.py` did, so any failures during backfill
  are easy to spot and retry individually rather than needing to rerun the whole batch.
- This is a one-time operation — once run, the remaining ~50 products without a photo get the
  category placeholder as normal; there's no need to re-run this script again.

## Testing checklist

- Upload succeeds and `image_url` is set correctly on the product record.
- Re-uploading a photo for a product with an existing one replaces it (via `x-upsert`), rather
  than erroring or creating a duplicate.
- Non-image files and oversized files are rejected with a clear error before any upload attempt.
- Backfill script completes, and the number of successful uploads matches the manifest's
  `downloaded` count (693).
- Thumbnails render correctly across the product grid, product edit screen, and asset detail view.
- Products without a photo show the category placeholder, not a broken image.
