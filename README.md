# Equiptra

Internal asset/project tracking tool for LDMtv, replacing CurrentRMS. See
[`equiptra-build-brief.md`](equiptra-build-brief.md) for the full spec —
the two-phase `booking_requests` → `booking_allocations` model in §3 is the
core of the data model.

## Stack

- Backend: Go (`backend/`), Postgres, chi router, pgx
- Frontend: React + TypeScript + Tailwind v4 (`frontend/`), Vite
- Auth: email/password, JWT session cookie, `admin`/`standard` roles

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
| `S3_BUCKET` | unset | product-photo uploads (brief §7) are disabled entirely if unset |
| `AWS_REGION` | `us-east-1` | |
| `S3_ENDPOINT_URL` | unset | set for a non-AWS S3-compatible endpoint (MinIO for local dev — see below); leave unset for real AWS S3 |
| `S3_PUBLIC_BASE_URL` | derived | override if photos are served through a CDN/custom domain rather than the bucket's own URL |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | — | standard AWS SDK credential chain (env vars, shared config, or an instance/task role in AWS) |

## Product photos (brief §7)

Fully built and tested end-to-end (presign → browser PUT → confirm) against
a **local MinIO** server standing in for S3, since this session has no real
AWS credentials. The same code talks to real AWS S3 unchanged — MinIO was
only for verification.

```bash
# Local dev / testing, using MinIO as an S3 stand-in
brew install minio/stable/minio minio-mc
minio server /tmp/minio-data --address :9000 --console-address :9001 &
mc alias set local http://127.0.0.1:9000 minioadmin minioadmin
mc mb local/equiptra-product-photos
mc anonymous set download local/equiptra-product-photos   # public GET, presigned-only PUT

# Run the API pointed at it
S3_BUCKET=equiptra-product-photos S3_ENDPOINT_URL=http://127.0.0.1:9000 \
AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
go run ./cmd/api
```

**To switch to real AWS S3**, once you can provide credentials/access:
1. Create a bucket (`aws s3 mb s3://equiptra-product-photos`), block public
   ACLs but allow public `GetObject` via a bucket policy (or front it with
   CloudFront), and set CORS to allow `PUT` from your frontend's origin.
2. Create an IAM user/role with `s3:PutObject`/`s3:GetObject` on that bucket
   and set `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` (or attach the role if
   running on ECS/EC2 — then those two vars can be left unset).
3. Set `S3_BUCKET` and `AWS_REGION`; leave `S3_ENDPOINT_URL` unset.
4. Run the one-off photo migration (idempotent, safe to re-run):
   ```bash
   cd backend/cmd/migrate-photos
   DATABASE_URL=... S3_BUCKET=... AWS_REGION=... go run . \
     -manifest "/path/to/Photo Dump/product_photos/_manifest.csv" \
     -photos-dir "/path/to/Photo Dump/product_photos"
   ```

Already verified locally: 693/693 manifest-listed photos upload and become
publicly fetchable; the 50 `no_photo` rows are correctly skipped; admin
users get a "Replace photo" control in the asset detail panel (presign →
direct browser upload → product record updated); assets without a photo
show a generic placeholder icon (not 16 bespoke per-category icons, as
originally suggested — the category name is already shown as text right
next to every thumbnail, so a per-category icon would be redundant).

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

## Still open (not built yet)

- **AWS deployment.** Built and verified against local Postgres only — no
  RDS/ECS/Lambda provisioning has been done. Needs your AWS access.
- **Monday.com Lambda → `service_records` write step.** `POST
  /api/service-records` exists (for the `monday_report` source) but
  currently needs the same user JWT as the rest of the app. The Lambda will
  need its own credential — flag your preference before wiring it up.
- **Carnet PDF template.** Functional general-list table but per the brief
  needs checking against a real past carnet before first live use.
- **Delivery note layout.** Matches the reference document's fields, table
  structure, and accessory indentation, but is a from-scratch single-column
  redesign rather than a pixel copy of the reference's two-column header —
  worth a sign-off pass against a real delivery note before first live use.
- **Quoting/pricing** — explicitly out of scope for v1 per the brief.
- Backup/retention policy for Postgres — not configured (local dev only so far).
