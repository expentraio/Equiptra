# Equiptra — Build Brief for Claude Code

Internal replacement for CurrentRMS. Sister product to Expentra (shared brand system). Built for LDMtv: 4 users, asset register + insurance, project/booking tracking with conflict detection, ATA carnet export, service history (fed from existing Monday.com fault-reporting Lambda).

Reference UI mockup: `asset-system-ui-mockup.html` (clickable, all screens sketched — treat as the visual/interaction spec for look, feel, and navigation, not literal code to reuse). **Note:** the mockup's booking board predates the two-phase reservation/allocation model in §3 — it shows one flat booking list per project. The real UI should split this into two views/states per the `booking_requests` → `booking_allocations` model: a reservation list (product + quantity + shortage status) and, per reservation, the specific asset allocation with checkout/check-in actions. Visual language (tag chips, conflict banner, colours) carries over; the exact table structure on that one screen does not.

---

## 1. Stack

**Decision (post-build-status-check):** Claude Code had already built a substantial, tested Go backend and React frontend before the Expentra-consistency question came up — a genuine two-phase booking state machine, two PDF generators verified against real reference documents, and a working photo pipeline (~3,200 lines Go, ~2,300 lines TS/TSX, 4-6 days of validated effort). Rewriting all of that in Next.js/Supabase-native form to match Expentra exactly was considered and rejected — the cost was real, not "close to free" as an earlier draft of this brief assumed. Instead: **keep the tested Go/React build, align only the hosting/database layer with Expentra where that's cheap to do.**

- **Frontend**: existing Vite/React app, deployed to **Vercel** as a static SPA — Vercel hosts plain React/Vite apps natively, not just Next.js, so this needs no rewrite. Matches Expentra's frontend host.
- **Backend**: existing Go API (chi router, pgx, JWT/bcrypt auth) — kept as-is, hosted on a small persistent service (Render or Fly.io are the natural fits for a single Go binary; avoid forcing it into Vercel's Go-serverless-function convention, which would mean restructuring all 18 handler files for little benefit).
- **Database**: **Supabase-hosted Postgres** instead of AWS RDS/Aurora — the existing schema/migration SQL (`migrations/0001_init.sql`) is Postgres-standard and needs no rewrite, just a new connection string once a Supabase project is created for Equiptra (a separate project from Expentra's — same org, own database). Gets managed backups and matches Expentra's DB provider.
- **Auth**: keep the already-built, tested JWT/bcrypt system. Not migrating to Supabase Auth — the value of doing so is low when the API layer is a custom Go backend rather than Supabase's own auto-generated API layer, and the existing auth is already tested.
- **File storage**: Supabase Storage exposes an S3-compatible API, so the existing Go S3 client (currently pointed at local MinIO) can likely point at Supabase Storage's endpoint with minimal change — avoids needing a separate AWS account for this at all. Confirm compatibility before assuming zero code change.
- **Conflict detection**: the 409-override flow is already built and tested at the application level. Worth adding a Postgres exclusion constraint (via `btree_gist`) on `booking_allocations` as defense-in-depth once hosted on real Postgres, but this is a hardening step, not a blocker — the existing check already works.
- **Monday.com Lambda integration**: stays as-is on AWS; called via HTTP from wherever the Go backend ends up running. No need to migrate this piece.
- **Immediate action, independent of any of the above**: nothing has been committed to git yet — fix this first, before anything else.

---

## 2. Brand system

Neutrals, status colours, type, icon style, and radii are shared across the Simplified Software
suite. The primary/accent colour is not — each product in the suite is locked to its own distinct
accent under the v1 brand lock; Equiptra's was provisionally teal (shared with Expentra) and is
now locked to a distinct steel blue.

| Token | Value |
|---|---|
| Steel blue (primary/action) | `#4F7693` |
| Navy (ink/text) | `#0F172A` |
| Light fill (accent bg) | `#E8EEF2` |
| Slate (secondary text) | `#64748B` |
| Off White (page bg) | `#F7F8FA` |
| Font | Inter (400/500/600/700 — no other typefaces) |
| Icon style | 2px stroke, rounded caps/joins, simple line icons (not filled) |
| Radius | 10px controls, 12px cards |

Functional-only colours (not brand, used for status semantics): amber `#B4661A` (shortage/attention), red `#A83E33` (conflict/error) — each with a light fill variant per the mockup.

Signature UI element: every asset reference renders as a small dark "tag" chip (asset number in bold tabular Inter, with a small dot/rivet) — echoes a physical asset label. Reuse this treatment anywhere an asset is referenced (tables, cards, detail panels).

Logo lockup: mark (3 stacked accent-colour bars, decreasing width) sits left of "EQUIPTRA" in uppercase, tagline "Equipment management, simplified." below — matches Expentra's own lockup pattern ("Business expenses, simplified.").

---

## 3. Data model

### `products`
Catalogue-level record — one per distinct item type. Imported from CurrentRMS Products export.

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `legacy_id` | int, nullable | CurrentRMS product id |
| `name` | text | matched 1:1 against asset export `name` field — confirmed clean join, all 733 distinct names matched exactly |
| `category` | text | Sound / Vision / Cameras / Power / Cases / Networking / Card / Grip / Lighting / Computers / Control / VTR / Other / Cable / Vehicle / Test |
| `manufacturer` | text, nullable | |
| `weight_kg` | decimal, nullable | confirmed 100% consistent with asset-level weight across all 1,397 rows — safe to treat as authoritative at product level |
| `country_of_origin_code` | text (ISO-2), nullable | **carnet-critical.** 646/743 products already populated in CurrentRMS; 94 affecting 180 assets still missing — office is backfilling directly in CurrentRMS (see §5) |
| `is_accessory` | boolean | from CurrentRMS's `Accessory Only` field (16/743 currently flagged) — used to indent/italicize accessory items on the delivery note, no strict parent-item link needed |
| `barcode` | text, nullable | |
| `image_url` | text, nullable | product thumbnail — see §7 |
| `description` | text, nullable | |
| `active` | boolean | |

### `assets`
Individual serialized unit, or a bulk-stock line. References a product.

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `legacy_id` | int, nullable | CurrentRMS asset id |
| `product_id` | FK → products | |
| `asset_number` | text, unique, nullable | null only for bulk-stock rows |
| `serial_number` | text, nullable | |
| `is_bulk` | boolean | true = tracked by quantity, not individually tagged |
| `quantity` | int, default 1 | for bulk stock, the count held; for serialized assets, always 1 |
| `location` | text, nullable | warehouse bay/shelf code |
| `purchase_price` | decimal, nullable | |
| `replacement_value` | decimal, nullable | insurance schedule |
| `purchase_date` | date, nullable | |
| `status` | enum | `active`, `written_off`, `sold`, `missing` |
| `notes` | text, nullable | |
| `created_at` / `updated_at` | timestamp | |

### `projects`

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `name` | text | |
| `client` | text, nullable | |
| `start_date` / `end_date` | date | |
| `status` | enum | `tentative`, `confirmed`, `in_progress`, `completed`, `cancelled` |
| `carnet_required` | boolean | reminder/dashboard flag only ("projects still needing a carnet") — doesn't gate the export action, which is available on any project |
| `client_reference` | text, nullable | client's own PO/job reference — printed on the delivery note as "Your Reference" |
| `order_number` | text, nullable | LDMtv's own order number (e.g. "536-244") — free text, no auto-numbering; continuity with old CurrentRMS numbering isn't required |
| `delivery_address` | text, nullable | multi-line — differs per job, so lives on the project rather than a shared client record |
| `notes` | text, nullable | |

### `booking_requests`
The product-level ask — "we need 2x Yellobrik OTT1842 for this job." Can start vague (no `product_id` yet) and get refined closer to the job. **No historic backfill** — table starts empty at go-live.

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `project_id` | FK → projects | |
| `product_id` | FK → products, nullable | null while still a placeholder idea rather than a confirmed pick |
| `placeholder_description` | text, nullable | e.g. "some XLR cable, ~10x100m" — used while `product_id` is null |
| `quantity_requested` | int | |
| `date_out` / `date_in` | date | |
| `status` | enum | `draft` (no product pinned yet), `reserved`, `partially_allocated`, `out`, `returned`, `cancelled` |
| `shortage_flag` | boolean, computed | true if `quantity_requested` exceeds available (non-allocated, non-conflicting) stock of `product_id` across the date range — only computes once a real product is attached |
| `sub_hire_notes` | text, nullable | |

### `booking_allocations`
The specific asset assigned once someone actually pulls a physical unit, plus the checkout/check-in lifecycle.

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `booking_request_id` | FK → booking_requests | |
| `asset_id` | FK → assets | |
| `status` | enum | `allocated`, `checked_out`, `returned` |
| `checked_out_at` | timestamp, nullable | |
| `checked_out_by` | FK → users, nullable | |
| `inspection_passed` | boolean, nullable | pre-checkout safety/function check. **Hard blocks checkout** — `checked_out_at` cannot be set unless this is true |
| `condition_out_notes` | text, nullable | |
| `checked_in_at` | timestamp, nullable | |
| `checked_in_by` | FK → users, nullable | |
| `condition_in_notes` | text, nullable | |
| `damage_flag` | boolean | set at check-in |
| `damage_service_record_id` | FK → service_records, nullable | auto-created when `damage_flag` is set — see below |

**Conflict logic (on allocation, not on the request):** flag — don't hard-block — when the same `asset_id` has an overlapping date range on another allocation with status `allocated` or `checked_out`. Overlap test: `date_out < other.date_in AND date_in > other.date_out` (dates taken from the parent `booking_request`). Surface clearly in the UI (mockup's conflict banner + row highlighting) and require explicit confirmation to override.

**Damage → service record:** when `damage_flag` is set on check-in, the app automatically creates a `service_records` row (which in turn feeds the existing Monday.com Lambda), linking back via `damage_service_record_id`. No separate manual reporting step needed.

**Mid-rental service/field repairs:** a fault can occur while an asset is still `checked_out`, not just get noticed on return. `service_records` references `asset_id` directly (not the allocation), so this already works without schema changes — it's just a second trigger path into the same table, alongside the existing Monday.com report path and the check-in damage path. The asset stays `checked_out` on its allocation; the service record is independent of that status.

**Extending a rental:** update `date_in` directly on the existing `booking_request` (and its allocations) rather than creating a new one. Doing so must re-run the shortage check (is the product still available for the extended range) and the conflict check on each allocated asset (is it needed elsewhere in the new date window) — same logic as initial creation, just re-triggered on edit.

**Post-return availability:** kept simple for v1 — once `status` is `returned`, the asset is immediately available again for new allocations. No intermediate cleaning/servicing state. Revisit only if this causes real problems in practice.

### `service_records`
Fed either by the existing Monday.com fault-reporting Lambda (add a third write step there, after creating the Monday item, to insert here) or automatically by a `damage_flag` set during asset check-in (see `booking_allocations` above). This app is the source of truth for service history; Monday.com stays as one of the intake paths.

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `asset_id` | FK → assets | |
| `date_reported` | date | |
| `fault_description` | text | |
| `status` | enum | `open`, `under_investigation`, `resolved` |
| `monday_item_id` | text, nullable | traceability link back to the Monday card, if reported that route |
| `source` | enum | `monday_report`, `checkin_damage` — where this record originated |
| `resolved_date` | date, nullable | |
| `resolution_notes` | text, nullable | |

### `users`

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `name` / `email` | text | |
| `role` | enum | `admin`, `standard` — admin can edit values/write off assets; standard can book/check assets in-out |
| `password_hash` | text | |

### Project documents (carnet + delivery note)
Not tables — both are generated views over `assets` + `products` + `booking_allocations` for a given project, sharing the same underlying query (allocated/checked-out assets, their weight, value, and product details), rendered through two different templates:

- **Carnet**: description, quantity, weight, value, country of origin. PDF (customs-facing) and CSV (for feeding other systems) outputs.
- **Delivery note**: item name, quantity, asset number, serial number — accessory-flagged products (`is_accessory`) indented and italicized, in the order they were added. Header pulls `client_reference`, `order_number`, `delivery_address`, and the booking date range; footer totals sum `weight_kg` and `replacement_value` across included assets. Appends a fixed T&Cs boilerplate block after the item list (static content, not modelled in the database — store the current wording as a template asset the generator appends every time). **Note:** this document carries LDMtv's own branding/letterhead, not Equiptra's — it's client-facing paperwork representing the company, not the internal tool.

---

## 4. Core workflows

1. **Asset lookup** — search by name/asset number/serial, or scan a barcode via phone camera into the search box. Filter by category. No dedicated scanner hardware needed.
2. **Reserving for a project** — create a `booking_request` against a project: pick a product (or, if not yet decided, just describe it in `placeholder_description`), set quantity and dates. `shortage_flag` computes automatically once a real product is attached, by checking available stock across the date range — no manual flag to remember.
3. **Pack-out / allocation** — closer to the job, each `booking_request` gets one or more `booking_allocations`: specific assets are picked and assigned. Conflict check runs here, at the point a specific serial number is committed, not at the vaguer reservation stage.
4. **Checkout** — requires `inspection_passed = true` first; the app blocks checkout entirely if the pre-release inspection hasn't been marked as passed. Once cleared, marking `checked_out` logs who and when, plus condition notes at the point it leaves the building.
5. **Check-in** — marking an allocation `returned` logs who and when received it back, condition notes, and a `damage_flag` if something's wrong. Setting that flag auto-creates a `service_records` entry (and the linked Monday.com card) immediately — no separate reporting step. Returned assets are immediately available again (no intermediate service/cleaning state in v1).
6. **Mid-rental faults** — a fault reported while an asset is still checked out (via the Monday.com form) writes to `service_records` independently of the allocation's status.
7. **Extending a booking** — update the date range on the existing `booking_request`; this re-runs the shortage and conflict checks against the new dates rather than assuming the extension is automatically fine.
8. **Shortage handling** — surfaced automatically on the `booking_request` (see #2); `sub_hire_notes` lets someone log what's being done about it. No full PO/supplier system.
9. **Carnet export** — available as a "Generate carnet" action on any project (not a separate carnet-only workflow), since not every job needs one. Uses the project's `carnet_required` flag mainly as a reminder/filter (e.g. a dashboard view of "confirmed projects still needing a carnet"), not a gate on the button itself — any project's allocated assets can be exported on demand. Two output formats from the same underlying data: **PDF** (the actual customs-facing general list) and **CSV** (same rows — description/qty/weight/value/origin — for feeding into other systems, e.g. a carnet-issuing body's own portal or a customs agent's tooling). Blocked/flagged in UI if any included asset's product is missing country of origin.
10. **Delivery note generation** — a "Generate delivery note" action on any project, same mechanical shape as the carnet (pull allocated assets, render PDF), but a different template: LDMtv-branded, with client reference/order number/delivery address header, an itemized list with accessories indented, computed total weight and insurance value, and the standard T&Cs boilerplate appended. This replaces the CurrentRMS-generated delivery note + rental agreement paperwork.
11. **Service history** — fault reported via the existing Monday.com form, via check-in damage flagging, or via mid-rental fault reporting — all three write to `service_records` and are visible on the asset's detail view in Equiptra.

**Scope correction on commercial functions:** billing/invoicing is confirmed to live entirely outside CurrentRMS already (Xero/QuickBooks/Sage), so Equiptra never needs to touch payments, late fees, or deposits — its role there, if any, is limited to exporting the numbers those systems need. "Contracts" turned out not to be a real e-signature workflow — it's boilerplate T&Cs printed on the back of the delivery note (see #10), which is now in scope and specified above. **Quoting and tiered pricing remain genuinely out of v1** — confirmed as fine to run manually (spreadsheet/email) short-term, since the mid-September deadline is about not losing CurrentRMS's data (already secured), not about having a fully commercial system live by then. Treat quoting as a clearly-defined next phase once the core app (asset registry, booking/allocation, carnet, delivery notes, service history) is live and stable — not an open-ended "someday."

**Consumables** — deliberately left out of v1. They need a different tracking shape (quantity-consumed rather than reserve-and-return) and are better designed properly later than bolted on now.

---

## 5. Migration plan

1. Import `products` from the CurrentRMS Products CSV export.
2. Import `assets` from the CurrentRMS Asset Listing CSV export, joining to `products` by name at import time (one-off; going forward assets reference `product_id` directly).
3. Bulk-stock rows (23 currently, blank asset number) import with `is_bulk = true` and a `quantity` value rather than being split into individual asset rows.
4. Country of origin: office is filling gaps directly in CurrentRMS against a supplied worklist (94 products / 180 assets affected). Re-export and re-diff once done; import runs again (idempotent on `legacy_id`) to pick up the fix.
5. No historic bookings/project-assignment data is migrated — `booking_requests` and `booking_allocations` start empty at go-live.

---

## 6. Legal/template content to source before build

- The delivery note's T&Cs boilerplate is provided in `LDMtv_Hire_TandC.pdf` — use this as the literal appended text, don't paraphrase or regenerate it (it's the company's actual standard terms).
- The delivery note layout/fields are provided in `536-244_CityTV_Studio_Cameras_LDM_Delivery_Note_22.pdf` as the real reference — match its header layout, item table structure, and accessory indentation exactly.

---

## 7. Product thumbnails (v1.1)

Not required for v1 launch, but source images are already secured — worth building once the core app is stable.

- **Source images**: 693 of 743 products' photos have already been pulled from CurrentRMS via its API (ahead of the Sept renewal lapsing) and saved locally, named `{product_id}_{sanitized-product-name}.{ext}` — e.g. `281_1005BAC.jpg`. A `_manifest.csv` tracks status per product (`downloaded` / `no_photo` / errors). The remaining 50 products with no photo are mostly one-off rack builds/flight cases that likely never had an image in CurrentRMS.
- **Build task**: add `products.image_url`, an S3 bucket, a presigned-upload flow for new photos going forward, and a thumbnail slot on the asset card/grid + product edit screen (mockup already reserves space for this above the product name).
- **Migration task**: bulk-upload the 693 saved images to the new S3 bucket, matching each to its product by the `product_id` embedded in the filename, and populate `image_url` accordingly.
- **Fallback**: a simple placeholder icon per category for the ~50 products without a photo.

---

## 8. Still open / worth deciding during build

- Exact carnet document template/layout — needs checking against a real past carnet for field-by-field accuracy before first live use.
- Whether `shortage_flag` needs any notification/reminder behaviour, or is purely informational for now.
- Backup/retention policy for the Postgres instance (worth a sensible default — daily automated snapshots — rather than a bespoke decision now).
- Quoting/pricing (tiered rates, tax calculation) — deliberately not speced yet; scope as its own phase once the core app above is live, working from a manual/spreadsheet stopgap until then.
