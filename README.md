# Equiptra

Internal asset/project tracking tool for LDMtv, replacing CurrentRMS. See
[`equiptra-build-brief.md`](equiptra-build-brief.md) for the full spec —
the two-phase `booking_requests` → `booking_allocations` model in §3 is the
core of the data model.

## Stack

- Backend: Go (`backend/`), Postgres, chi router, pgx — deploying to Render
- Frontend: React + TypeScript + Tailwind v4 (`frontend/`), Vite — deploying to Vercel
- Database: Supabase-hosted Postgres (was local-only during initial build)
- File storage: Supabase Storage's native REST API (bearer token, not the S3-compatible endpoint) for product photos
- Auth: email/password, JWT session cookie, `admin`/`standard` roles — unchanged by the hosting move

## Deployment (Vercel + Render + Supabase)

- **`render.yaml`** (repo root) — Render blueprint for the Go API. Builds
  `./cmd/api` from `backend/`. Render injects `PORT` automatically; the app
  now reads it (falls back to `LISTEN_ADDR`, then `:8080` — see
  `cmd/api/main.go`). Env vars marked `sync: false` need real values set in
  the Render dashboard (or via `render.yaml` before first deploy) — none are
  committed here.
- **`frontend/vercel.json`** — rewrites `/api/:path*` to the Render backend
  URL so the browser only ever talks to the Vercel origin. This keeps the
  JWT cookie first-party (no `SameSite=None` needed, no CORS changes) —
  chosen specifically so the existing auth setup didn't need touching.
  **Placeholder URL inside — update once the backend's Render URL exists.**
- `JWT_SECRET` for production was generated and handed to you out-of-band
  (not committed anywhere in this repo) — set it in Render's dashboard.

## Local setup

Requires Go, Node, and Postgres (`brew install go node postgresql@16`).

```bash
# 1. Database
brew services start postgresql@16
createdb equiptra
psql equiptra -f backend/migrations/0001_init.sql

# 2. Seed an admin user (bcrypt hash — see backend/cmd/migrate or generate your own)
psql equiptra -c "INSERT INTO users (name, email, role, password_hash) VALUES ('Admin', 'admin@example.com', 'admin', '<bcrypt-hash>');"

# 3. Migrate the CurrentRMS CSV exports (one-off, idempotent on legacy_id)
cd backend/cmd/migrate
DATABASE_URL="postgres://localhost:5432/equiptra?sslmode=disable" go run . \
  -products /path/to/Current-Product-*.csv \
  -assets /path/to/asset_listing_report_*.csv

# 4. Run the API
cd backend/cmd/api
DATABASE_URL="postgres://localhost:5432/equiptra?sslmode=disable" \
JWT_SECRET="change-me" \
FRONTEND_ORIGIN="http://localhost:5173" \
go run .

# 5. Run the frontend (proxies /api to localhost:8080 — see frontend/vite.config.ts)
cd frontend
npm install
npm run dev
```

## Environment variables (backend)

| Var | Default | Notes |
|---|---|---|
| `DATABASE_URL` | `postgres://localhost:5432/equiptra?sslmode=disable` | point at RDS/Aurora in production |
| `JWT_SECRET` | insecure dev default | set a real secret in any deployed environment |
| `FRONTEND_ORIGIN` | `http://localhost:5173` | CORS allow-origin |
| `LISTEN_ADDR` | `:8080` | |
| `COOKIE_SECURE` | unset (`false`) | set `true` behind HTTPS |
| `SUPABASE_PROJECT_REF` | unset | product-photo uploads are disabled entirely if unset — the subdomain of `https://{ref}.supabase.co` |
| `SUPABASE_SERVICE_ROLE_KEY` | unset | server-side only — never sent to the frontend, never logged |

## Product photos

**Replaces an earlier S3-compatible-client approach** (pointing the AWS S3
SDK at Supabase Storage's S3-compatible endpoint), which failed with
`SignatureDoesNotMatch` against real Supabase Storage across multiple SDKs,
credential pairs, and regions — consistent with a SigV4 compatibility gap
in Supabase's S3 shim, not a config mistake. See
`docs/equiptra-photo-upload-addendum.md` for the full writeup. The fix:
Supabase Storage also exposes a **native REST API** that takes a plain
bearer token — no request signing, so the whole problem class goes away.
`internal/storage/supabase.go` is a small wrapper around it (no SDK
dependency at all).

The Go backend uploads on the frontend's behalf — the browser never sees
`SUPABASE_SERVICE_ROLE_KEY`:

1. `POST /api/products/{id}/photo` (admin-only, multipart) validates file
   type (jpg/png/webp) and size (5MB cap) before doing anything else.
2. The backend forwards the file to Supabase Storage's REST API
   (`POST /storage/v1/object/product-photos/{id}.{ext}`, `x-upsert: true`
   so a re-upload replaces rather than errors) using the service-role key.
3. On success, `products.image_url` is set to the public URL and the
   updated product is returned.

Requires a **`product-photos` bucket, set to public**, created manually in
the Supabase Storage dashboard — nothing in this repo creates or configures
buckets.

```bash
# Local dev/testing — needs a real SUPABASE_PROJECT_REF/SUPABASE_SERVICE_ROLE_KEY.
# DATABASE_URL is required too — db.Connect() has no local-dev fallback
# (see the backfill warning below for why).
DATABASE_URL=... SUPABASE_PROJECT_REF=... SUPABASE_SERVICE_ROLE_KEY=... go run ./cmd/api
```

**One-off backfill** of the 693 CurrentRMS photos already extracted locally
(`Photo Dump/product_photos/`, see `extract_photos.py`) to real photos in
storage:

> **`DATABASE_URL` matters here as much as the Supabase vars.** The tool
> uploads to Storage and writes `products.image_url` in two separate steps
> against two separate systems — it's entirely possible for the Storage
> upload to succeed against the real bucket while the DB write lands on
> the wrong database, because nothing ties the two together. That's
> exactly what happened running the real backfill: all 693 files uploaded
> correctly, but `DATABASE_URL` was never pointed at production Postgres
> for that run, so `image_url` only got set locally — not caught until
> someone checked the live site and found no photos. Fixed after the fact
> with a SQL `UPDATE` joining `storage.objects` back to
> `products.legacy_id`, scoped to rows where `image_url` was still empty.
> `db.Connect()` no longer has a local-dev fallback (an unset
> `DATABASE_URL` now fails immediately with a clear error instead of
> silently connecting to local Postgres), which would have caught this
> outright — but a *wrong* `DATABASE_URL` still passes silently, so before
> the **full** run (the sample run matters less), explicitly confirm it's
> the real one, not whatever's left over from local testing.

```bash
cd backend/cmd/migrate-photos
# Sample run first — this is one-time and not cleanly undoable
DATABASE_URL=... SUPABASE_PROJECT_REF=... SUPABASE_SERVICE_ROLE_KEY=... go run . -limit 3 \
  -manifest "/path/to/Photo Dump/product_photos/_manifest.csv" \
  -photos-dir "/path/to/Photo Dump/product_photos"
# Full batch once the sample looks right — DOUBLE-CHECK DATABASE_URL first
DATABASE_URL=... SUPABASE_PROJECT_REF=... SUPABASE_SERVICE_ROLE_KEY=... go run . \
  -manifest "/path/to/Photo Dump/product_photos/_manifest.csv" \
  -photos-dir "/path/to/Photo Dump/product_photos"
```

Safe to re-run — `x-upsert` on the storage side means retrying a file just
overwrites the same object, and the DB update is unconditional. Products
without a photo (the ~50 `no_photo` rows, plus anything created after) show
a generic placeholder icon — not 16 bespoke per-category icons as
originally suggested — since the category name is already shown as text
right next to every thumbnail, so a per-category icon would be redundant.

## Data model notes worth knowing

- **Bulk allocation model.** `booking_allocations` has no quantity column —
  allocating 3 units of a bulk product creates 3 rows all pointing at the same
  bulk `asset_id`. The asset-level conflict check (same asset, overlapping
  dates) is skipped for bulk assets — sharing one `asset_id` concurrently is
  normal for bulk stock, not a conflict — and an over-allocation check
  (count of active overlapping allocations vs. the asset's held quantity)
  is used instead. See `backend/internal/handlers/booking_allocations.go`.
- **`booking_requests.status`** is recomputed from its allocations on every
  mutation (`recomputeBookingRequestStatus` in `booking_requests.go`) rather
  than being settable directly, except `cancelled` which is a deliberate
  user action (`POST /booking-requests/{id}/cancel`).
- **Shortage calculation**: a request is short if its own quantity plus every
  other live (`reserved`/`partially_allocated`/`out`) request for the same
  product with an overlapping date range exceeds the product's total active
  stock. See `computeShortage`.
- **Delivery note T&Cs**: `backend/internal/assets/tandc.txt` is a
  hand-reconstructed verbatim copy of `LDMtv Hire TandC.pdf`/`.docx` — the
  source document has a numbering artifact where several sub-clause markers
  (e.g. "6.1.2 6.1.3 6.1.4 6.1.5") are bunched onto one line ahead of their
  four paragraph bodies, present identically in both the PDF and DOCX
  extraction. The stored text re-attaches each number to its correct
  paragraph (unambiguous — the numbering sequence and semicolon-terminated
  clauses make the pairing obvious) without changing a single word.
- **Delivery note logo**: `backend/internal/assets/ldm_logo.png` was
  extracted directly from the reference PDF's embedded image (RGB + soft
  mask composited into a transparent PNG) since no separate logo file was
  supplied.

## Fault reporting & Services tab

Replaces the old Monday.com relay — see `equiptra-build-brief-fault-workflow-addendum.md`
for the original spec. `service_records` (from `0001_init.sql`) was extended
rather than replaced — migration `0003_fault_reporting.sql` adds `in_progress`
to the existing `service_status` enum and `field_report` to `service_source`
(both existing enums, not new ones), plus `reporter_user_id`/`reporter_name`/
`reporter_email`/`resolved_by` columns. `under_investigation` and
`monday_report` are left in place, unused, for historic rows.

- **Two ways a fault gets logged**: automatically at check-in when
  `damage_flag` is set (`CheckinAllocation`, unchanged except it now also
  stamps `reporter_user_id` to the checking-in user), or manually via
  `POST /api/public/fault-reports` — the public, no-login form at
  `/report-fault`. That route and `GET /api/public/assets` (the form's asset
  lookup) sit outside `RequireAuth` behind `OptionalAuth` instead (best-effort
  cookie parse — auto-fills `reporter_user_id` for a staff member who happens
  to have a session, otherwise requires `reporter_name`/`reporter_email`) and
  a small in-memory per-IP rate limiter (`middleware.RateLimit`, 20 req/min) —
  the only unauthenticated write path in the app.
- **Availability**: `assetHasOpenFault` (`service_records.go`) is an inline Go
  helper, not a SQL view — matches this codebase's existing convention
  (`computeShortage`, `findAllocationConflicts`) rather than introducing the
  schema's first view. `CreateAllocation` hard-blocks (no override) any asset
  with an open/in_progress fault; `assets.has_open_fault` and
  `products.available_units` (which now subtracts faulted-but-active assets,
  same treatment as an active allocation) surface the same check for display.
  Forward-looking only — an existing allocation is never retroactively
  affected.
- **Status lifecycle** (`open → in_progress → resolved`) is driven by staff
  from the Services tab (`PUT /api/service-records/{id}`); a DB trigger
  auto-stamps the existing `resolved_date` column on the transition to
  `resolved` (no new timestamp column), while `resolved_by` is set by the
  handler (the trigger has no notion of which user is asking).
- The `chk_service_records_reporter` CHECK constraint was added `NOT VALID` —
  enforced on new/updated rows only, since pre-existing check-in-damage rows
  predate reporter tracking and would otherwise fail a validated constraint
  on migration.

## Password management

Self-service change (any logged-in user) plus admin-initiated reset for a
locked-out teammate — see `docs/equiptra-password-management-addendum.md`.
No email/SMTP infrastructure exists, so there's no "forgot password" email
link; an admin resetting someone directly is the practical equivalent at
4-user scale.

- **`users.must_change_password`** (migration `0004_must_change_password.sql`)
  drives a forced-reset session. Set to `true` by `PATCH /api/users/{id}/password`
  (admin-only — sets a temporary password the admin types in and relays
  directly, no email); cleared automatically by `PATCH /api/users/me/password`
  once the affected user sets their own new password.
- **A third auth state, enforced server-side, not just in the UI**: `Claims`
  carries `must_change_password` from the JWT issued at login, and
  `middleware.RequirePasswordSet` (chained right after `RequireAuth` on the
  whole `/api` group) blocks every route except `/api/me` and
  `/api/users/me/password` for a session in that state — mirrored on the
  frontend by `ForcedPasswordChange`, rendered instead of `<Layout>` (and
  everything inside it) whenever `user.must_change_password` is true, so
  there's no nav to escape through either.
- **Self-service change also resolves a forced reset** — same endpoint, same
  request shape. On success it clears the flag and issues a fresh session
  token directly (rather than sending the user back to `/login`), so someone
  completing a forced reset lands straight in the app.
- **Self-lockout carve-out** matches the existing deactivate/delete pattern
  in `UpdateUser`/`DeleteUser` exactly: an admin can't reset their own
  password via the admin endpoint (400, "use Settings instead") and the
  "Reset password" action is disabled on their own row in the user list —
  they already know their current password, so the self-service flow is the
  correct path.

## Still open (not built yet)

- ~~Monday.com Lambda → `service_records` write step~~ — **retired, and the
  AWS side is fully decommissioned.** Correction to an earlier assumption
  here: this wasn't unfinished — it was a real, working integration
  (`fault-report-currentrms-lookup` + `fault-report-monday-create-item`
  Lambdas behind a standalone `fault-reporting-integration-api` API Gateway,
  never wired to this app's own `/api/service-records`), just never rolled
  out past Ric's own testing. Fault reporting now goes through Equiptra
  directly instead (automatic at check-in damage, or the public
  `/report-fault` form for staff and freelancers alike — see "Fault
  reporting & Services tab" below). The old Lambdas, the API Gateway, both
  IAM roles, both CloudWatch log groups, and both Secrets Manager secrets
  (7-day deletion hold, automatic) have all been removed directly in the AWS
  console — nothing outstanding.
- **Carnet PDF template.** Functional general-list table but per the brief
  needs checking against a real past carnet before first live use.
- **Delivery note layout.** Matches the reference document's fields, table
  structure, and accessory indentation, but is a from-scratch single-column
  redesign rather than a pixel copy of the reference's two-column header —
  worth a sign-off pass against a real delivery note before first live use.
- **Quoting/pricing** — explicitly out of scope for v1 per the brief.
- Backup/retention policy for Postgres — not configured (local dev only so far).
- **Role-based permissions are deliberately loose right now.** Creating and
  editing products/assets is open to any authenticated user (`admin` or
  `standard`) — not gated the way the brief's `admin`/`standard` split
  originally implied ("admin can edit values... standard can book/check
  assets in-out"). `DELETE` on products/assets is still admin-only; nothing
  else changed. This was a deliberate, explicit decision to defer proper
  permission design until the app's been used enough to know where the
  real lines should be — revisit once that's clearer, don't treat the
  current wide-open create/edit as the intended end state.
