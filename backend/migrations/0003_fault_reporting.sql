-- Fault reporting & Services/Repairs tab. Extends the existing
-- service_records table/enums rather than introducing parallel ones —
-- see equiptra-build-brief-fault-workflow-addendum.md for the workflow.

-- service_status already has 'open' and 'resolved'; add the new
-- mid-lifecycle value. 'under_investigation' is left in place, unused
-- by new code, rather than dropped or renamed — there's no way from
-- this migration to confirm no live rows still use it.
ALTER TYPE service_status ADD VALUE 'in_progress';

-- service_source already has 'checkin_damage'; add the new manual-report
-- source. 'monday_report' is left in place for historic rows — nothing
-- writes it going forward (the Monday.com relay is retired).
ALTER TYPE service_source ADD VALUE 'field_report';

ALTER TABLE service_records
  ADD COLUMN reporter_user_id BIGINT REFERENCES users(id),  -- set for authenticated (staff) submissions
  ADD COLUMN reporter_name    TEXT,                          -- set for freelancer submissions (no account)
  ADD COLUMN reporter_email   TEXT,
  ADD COLUMN resolved_by      BIGINT REFERENCES users(id);

-- NOT VALID: enforced on new/updated rows only. Existing checkin_damage
-- rows predate reporter tracking entirely and would fail a validated
-- constraint; CheckinAllocation is updated alongside this migration to
-- set reporter_user_id going forward, so new rows satisfy it naturally.
ALTER TABLE service_records
  ADD CONSTRAINT chk_service_records_reporter
  CHECK (reporter_user_id IS NOT NULL OR reporter_name IS NOT NULL) NOT VALID;

-- Fast lookup for "does this asset currently have an unresolved fault" —
-- used by the allocation-time availability check.
CREATE INDEX idx_service_records_asset_open
  ON service_records (asset_id)
  WHERE status IN ('open', 'in_progress');

-- Auto-stamp resolved_date (existing column) when status flips to
-- 'resolved', rather than adding a duplicate resolved_at column.
CREATE OR REPLACE FUNCTION set_service_record_resolved_date()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'resolved' AND OLD.status IS DISTINCT FROM 'resolved' THEN
        NEW.resolved_date := CURRENT_DATE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_service_record_resolved_date
BEFORE UPDATE ON service_records
FOR EACH ROW
EXECUTE FUNCTION set_service_record_resolved_date();
