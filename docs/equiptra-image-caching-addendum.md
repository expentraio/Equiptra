# Equiptra — Addendum: Image Caching Performance Fix

## What's actually slow, and why

Investigated the "site feels slow" report by measuring real load timings on the Products grid
page rather than guessing. Findings:

- Page shell: 47ms to first byte, 240ms full DOM load — the frontend itself is fast.
- The `/api/products` call: ~1.1s — a little slow, but not the dominant cost, and out of scope
  for this addendum.
- **The real cost: 245 product photos on a single grid page, each triggering a fresh network
  round trip to Supabase Storage on every page load, every time, for every visitor.**

Checked the response headers on an uploaded photo directly:

```
cache-control: no-cache
```

This means the browser can never serve a previously-loaded photo from its own cache — it must
revalidate with Supabase Storage on every single visit to any page showing product photos. With
245+ images on the grid view, that's 245+ unavoidable round trips stacking up, which is what's
actually producing the sluggish feeling, not any single slow operation.

## Root cause

The photo upload code (both the one-off backfill and the ongoing `POST /api/products/:id/photo`
endpoint) uploads to Supabase Storage's REST API without setting a `Cache-Control` header,
so Supabase defaults to no caching at all. Product photos are static — they only change when
someone deliberately replaces one via the upload endpoint (`x-upsert`) — so this is pure waste.

## Fix

1. **Confirm the exact mechanism** for setting cache behavior on Supabase Storage's upload API
   before writing code — check current Supabase Storage REST API docs for how `cacheControl` is
   specified on `POST /storage/v1/object/{bucket}/{path}` (this has been a header in some API
   versions and a multipart field in others; verify against the live docs rather than assuming).
2. **Set a long cache lifetime going forward**: something like `public, max-age=31536000` (one
   year) on every upload — both the ongoing upload endpoint and any future one-off scripts.
   Since `x-upsert` already handles replacing a photo in place, a long cache lifetime is safe:
   when a photo is genuinely replaced, browsers that already cached the old version won't see
   the new one until their cache expires — acceptable at this usage scale, but worth knowing.
   If tighter correctness is wanted, a shorter cache lifetime (e.g. one day) is a reasonable
   compromise instead of a full year.
3. **Repair the existing 693 already-uploaded photos**, which all currently have `no-cache` set.
   These don't need re-uploading from scratch — the same local source files used for the
   original backfill (or a fresh export, if those are gone) can be re-pushed to the *same* object
   paths with `x-upsert: true` and the corrected cache header. URLs and `products.image_url`
   don't change, so no database work is needed for this repair — only the stored object's
   headers need fixing.

## Testing checklist

- After the fix, re-check response headers on a freshly (re-)uploaded photo and confirm
  `cache-control` reflects the new value, not `no-cache`.
- Reload the Products grid twice in the same browser session and confirm (via Network tab or the
  Performance API) that the second load serves images from cache (status `(disk cache)` /
  `(memory cache)` in DevTools, or near-zero `duration` in `performance.getEntriesByType`) rather
  than re-fetching every image.
- Confirm replacing a photo via the existing upload endpoint still works correctly end-to-end
  after the header change.

## Out of scope for this fix (noted, not urgent)

- The `/api/products` endpoint's ~1.1s response time is worth a look eventually but isn't the
  dominant cause of the reported slowness — no action needed as part of this fix.
- Image file sizes looked reasonable in spot checks (~37KB) — no evidence yet that resizing or
  compression is needed on top of fixing caching.
