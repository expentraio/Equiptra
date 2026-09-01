-- Racks & cases — see docs/equiptra-racks-cases-addendum.md. Two container
-- concepts on top of the existing asset model: racks (fixed, permanent
-- kits) and cases (packed per job, empty between bookings).

CREATE TYPE container_type AS ENUM ('rack', 'case');

ALTER TABLE assets
  ADD COLUMN container_type container_type,
  ADD COLUMN home_rack_id BIGINT REFERENCES assets(id);

-- A rack can't be its own member — cheap sanity guard, not a full
-- containment-cycle check (racks aren't nested per the addendum).
ALTER TABLE assets
  ADD CONSTRAINT chk_home_rack_not_self CHECK (home_rack_id IS NULL OR home_rack_id != id);

CREATE INDEX idx_assets_home_rack_id ON assets (home_rack_id) WHERE home_rack_id IS NOT NULL;

-- Per-job packing for cases only — racks don't need this since home_rack_id
-- already represents membership (stable, not repacked per job). Rows are
-- created at pack-out and deleted at check-in (see CheckinAllocation).
CREATE TABLE case_contents (
    id                     BIGSERIAL PRIMARY KEY,
    case_asset_id          BIGINT NOT NULL REFERENCES assets(id),
    content_asset_id       BIGINT NOT NULL REFERENCES assets(id),
    booking_allocation_id  BIGINT NOT NULL REFERENCES booking_allocations(id),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (booking_allocation_id, content_asset_id)
);

CREATE INDEX idx_case_contents_booking_allocation ON case_contents (booking_allocation_id);
CREATE INDEX idx_case_contents_content_asset ON case_contents (content_asset_id);

-- Set true when an asset with a home_rack_id is allocated to a booking
-- other than via its own rack cascade (i.e. pulled out individually).
-- Defaults true at the column level; the rack-checkout cascade explicitly
-- sets it false on the allocations it creates for the rack's own members,
-- since those are travelling with their rack, not pulled solo.
ALTER TABLE booking_allocations
  ADD COLUMN return_to_home_rack BOOLEAN NOT NULL DEFAULT true;
