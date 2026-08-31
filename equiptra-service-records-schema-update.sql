-- ============================================================
-- Equiptra — Service Records Schema Update
-- Fault-reporting workflow addendum
-- ============================================================
-- NOTE: This assumes the existing `service_records` table (already
-- created via the check-in damage trigger) and an `assets` table with
-- an `id` primary key. Column/table names should be reconciled against
-- your original schema file before running — the original file wasn't
-- available in this session, so this is written to be dropped in or
-- adapted rather than run blind.
-- ============================================================

-- New enums to support field-reported faults alongside check-in damage
CREATE TYPE service_record_source AS ENUM ('checkin_damage', 'field_report');
CREATE TYPE service_record_status AS ENUM ('open', 'in_progress', 'resolved');

-- Extend service_records to support:
--   - who/what triggered the record (check-in damage vs public field report)
--   - a lifecycle status (replaces any prior implicit "always resolved" assumption)
--   - reporter details for non-authenticated submitters (freelancers)
--   - resolution attribution
ALTER TABLE service_records
  ADD COLUMN source service_record_source NOT NULL DEFAULT 'checkin_damage',
  ADD COLUMN status service_record_status NOT NULL DEFAULT 'open',
  ADD COLUMN reporter_user_id INTEGER REFERENCES users(id),   -- set when an internal staff member is logged in
  ADD COLUMN reporter_name TEXT,                              -- set when reporter has no Equiptra account (freelancers)
  ADD COLUMN reporter_email TEXT,
  ADD COLUMN resolved_at TIMESTAMPTZ,
  ADD COLUMN resolved_by INTEGER REFERENCES users(id);

-- A record must identify its reporter one way or another
ALTER TABLE service_records
  ADD CONSTRAINT chk_service_records_reporter
  CHECK (reporter_user_id IS NOT NULL OR reporter_name IS NOT NULL);

-- Fast lookup for "does this asset currently have an unresolved fault"
CREATE INDEX idx_service_records_asset_open
  ON service_records (asset_id)
  WHERE status IN ('open', 'in_progress');

-- ============================================================
-- Availability view
-- Single source of truth for whether an asset is bookable.
-- Booking allocation logic (and the shortage-detection query)
-- should filter against this rather than maintaining a separate
-- flag on the assets table, so the Service tab and the booking
-- engine can never drift out of sync.
-- ============================================================
CREATE OR REPLACE VIEW asset_availability AS
SELECT
    a.id AS asset_id,
    NOT EXISTS (
        SELECT 1
        FROM service_records sr
        WHERE sr.asset_id = a.id
          AND sr.status IN ('open', 'in_progress')
    ) AS is_bookable
FROM assets a;

-- ============================================================
-- Resolution trigger (optional convenience)
-- Auto-stamps resolved_at when status flips to 'resolved',
-- so the app layer doesn't have to set it manually every time.
-- ============================================================
CREATE OR REPLACE FUNCTION set_service_record_resolved_at()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'resolved' AND OLD.status IS DISTINCT FROM 'resolved' THEN
        NEW.resolved_at := now();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_service_record_resolved_at
BEFORE UPDATE ON service_records
FOR EACH ROW
EXECUTE FUNCTION set_service_record_resolved_at();

-- ============================================================
-- Behaviour notes for the build brief:
-- 1. A fault reported mid-hire (asset currently on an active
--    booking_allocation) is NOT retroactive — it does not affect
--    the current allocation, only future ones. No extra logic
--    needed here since asset_availability is only consulted at
--    allocation time, not against existing allocations.
-- 2. The Monday.com Lambda relay is retired. This table + the
--    Services/Repairs tab become the sole system of record for
--    fault tracking.
-- ============================================================
