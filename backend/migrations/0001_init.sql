-- Equiptra schema — two-phase booking model (booking_requests -> booking_allocations)
-- Enums

CREATE TYPE asset_status AS ENUM ('active', 'written_off', 'sold', 'missing');
CREATE TYPE project_status AS ENUM ('tentative', 'confirmed', 'in_progress', 'completed', 'cancelled');
CREATE TYPE booking_request_status AS ENUM ('draft', 'reserved', 'partially_allocated', 'out', 'returned', 'cancelled');
CREATE TYPE booking_allocation_status AS ENUM ('allocated', 'checked_out', 'returned');
CREATE TYPE service_status AS ENUM ('open', 'under_investigation', 'resolved');
CREATE TYPE service_source AS ENUM ('monday_report', 'checkin_damage');
CREATE TYPE user_role AS ENUM ('admin', 'standard');

-- users (created early: referenced by booking_allocations)

CREATE TABLE users (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL,
    email               TEXT NOT NULL UNIQUE,
    role                user_role NOT NULL DEFAULT 'standard',
    password_hash       TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- products

CREATE TABLE products (
    id                      BIGSERIAL PRIMARY KEY,
    legacy_id               INTEGER UNIQUE,
    name                    TEXT NOT NULL,
    category                TEXT,
    manufacturer            TEXT,
    weight_kg               NUMERIC(10,3),
    country_of_origin_code  TEXT,
    is_accessory            BOOLEAN NOT NULL DEFAULT FALSE,
    barcode                 TEXT,
    image_url               TEXT,
    description             TEXT,
    active                  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_name ON products (name);
CREATE INDEX idx_products_category ON products (category);

-- assets

CREATE TABLE assets (
    id                  BIGSERIAL PRIMARY KEY,
    legacy_id           INTEGER UNIQUE,
    product_id          BIGINT NOT NULL REFERENCES products(id),
    asset_number        TEXT UNIQUE,
    serial_number       TEXT,
    is_bulk             BOOLEAN NOT NULL DEFAULT FALSE,
    quantity            INTEGER NOT NULL DEFAULT 1,
    location            TEXT,
    purchase_price      NUMERIC(12,2),
    replacement_value   NUMERIC(12,2),
    purchase_date       DATE,
    status              asset_status NOT NULL DEFAULT 'active',
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_asset_number_or_bulk CHECK (is_bulk OR asset_number IS NOT NULL)
);

CREATE INDEX idx_assets_product_id ON assets (product_id);
CREATE INDEX idx_assets_asset_number ON assets (asset_number);
CREATE INDEX idx_assets_serial_number ON assets (serial_number);
CREATE INDEX idx_assets_status ON assets (status);

-- projects

CREATE TABLE projects (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL,
    client              TEXT,
    start_date          DATE NOT NULL,
    end_date            DATE NOT NULL,
    status              project_status NOT NULL DEFAULT 'tentative',
    carnet_required     BOOLEAN NOT NULL DEFAULT FALSE,
    client_reference    TEXT,
    order_number        TEXT,
    delivery_address    TEXT,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_project_dates CHECK (end_date >= start_date)
);

CREATE INDEX idx_projects_status ON projects (status);
CREATE INDEX idx_projects_dates ON projects (start_date, end_date);

-- service_records (created before booking_allocations: allocations reference it)

CREATE TABLE service_records (
    id                  BIGSERIAL PRIMARY KEY,
    asset_id            BIGINT NOT NULL REFERENCES assets(id),
    date_reported       DATE NOT NULL,
    fault_description   TEXT NOT NULL,
    status              service_status NOT NULL DEFAULT 'open',
    monday_item_id      TEXT,
    source              service_source NOT NULL,
    resolved_date       DATE,
    resolution_notes    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_service_records_asset_id ON service_records (asset_id);
CREATE INDEX idx_service_records_status ON service_records (status);

-- booking_requests: the product-level ask against a project

CREATE TABLE booking_requests (
    id                      BIGSERIAL PRIMARY KEY,
    project_id              BIGINT NOT NULL REFERENCES projects(id),
    product_id              BIGINT REFERENCES products(id),
    placeholder_description TEXT,
    quantity_requested      INTEGER NOT NULL,
    date_out                DATE NOT NULL,
    date_in                 DATE NOT NULL,
    status                  booking_request_status NOT NULL DEFAULT 'draft',
    shortage_flag           BOOLEAN NOT NULL DEFAULT FALSE,
    sub_hire_notes          TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_booking_request_dates CHECK (date_in >= date_out),
    CONSTRAINT chk_booking_request_quantity CHECK (quantity_requested > 0),
    CONSTRAINT chk_booking_request_has_description CHECK (product_id IS NOT NULL OR placeholder_description IS NOT NULL)
);

CREATE INDEX idx_booking_requests_project_id ON booking_requests (project_id);
CREATE INDEX idx_booking_requests_product_id ON booking_requests (product_id);
CREATE INDEX idx_booking_requests_dates ON booking_requests (date_out, date_in);
CREATE INDEX idx_booking_requests_status ON booking_requests (status);

-- booking_allocations: the specific asset pulled for a booking_request, plus
-- its checkout/check-in lifecycle. For bulk products, one row = one unit —
-- several allocation rows may reference the same bulk asset_id concurrently
-- (see conflict-check note in the Go handler: the asset_id-overlap conflict
-- test only applies to non-bulk assets; bulk over-allocation is checked by
-- counting active rows against the asset's held quantity instead).

CREATE TABLE booking_allocations (
    id                          BIGSERIAL PRIMARY KEY,
    booking_request_id          BIGINT NOT NULL REFERENCES booking_requests(id),
    asset_id                    BIGINT NOT NULL REFERENCES assets(id),
    status                      booking_allocation_status NOT NULL DEFAULT 'allocated',
    checked_out_at              TIMESTAMPTZ,
    checked_out_by              BIGINT REFERENCES users(id),
    inspection_passed           BOOLEAN,
    condition_out_notes         TEXT,
    checked_in_at               TIMESTAMPTZ,
    checked_in_by               BIGINT REFERENCES users(id),
    condition_in_notes          TEXT,
    damage_flag                 BOOLEAN NOT NULL DEFAULT FALSE,
    damage_service_record_id    BIGINT REFERENCES service_records(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- inspection_passed hard-blocks checkout: cannot be checked out unless true
    CONSTRAINT chk_checkout_requires_inspection CHECK (checked_out_at IS NULL OR inspection_passed = TRUE)
);

CREATE INDEX idx_booking_allocations_request_id ON booking_allocations (booking_request_id);
CREATE INDEX idx_booking_allocations_asset_id ON booking_allocations (asset_id);
CREATE INDEX idx_booking_allocations_status ON booking_allocations (status);
