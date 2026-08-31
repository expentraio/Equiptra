# Equiptra Build Brief — Addendum: Fault Reporting & Services Tab

*This addendum supersedes any earlier reference to the Monday.com Lambda relay for fault/damage reporting. It should be merged into the main build brief before handoff.*

## Change summary

The previous design routed damage/fault reports through a standalone public form into a Monday.com board via an existing Lambda integration. That integration was providing minimal value beyond acting as an inbox, so it is being retired. Equiptra becomes the sole system of record for asset faults, via a new **Services/Repairs tab** and an extended `service_records` table.

## Workflow

1. **Report** — A fault is logged one of two ways:
   - **Automatically**, when damage is recorded at asset check-in (existing behaviour, unchanged).
   - **Manually**, via a lightweight public-facing form (no login required), used by both internal staff and freelancers. Internal staff submissions capture their user ID; freelancer submissions capture name + email as free text since they have no Equiptra account.
2. **Block** — The moment a `service_record` exists against an asset with status `open` or `in_progress`, that asset is excluded from the bookable pool for **future** allocations only. Any booking the asset is *currently* allocated to is unaffected — the block is forward-looking, not retroactive.
3. **Track** — A new **Services/Repairs** tab (alongside Products, Bookings, Assets) lists all service records with: asset, fault description, source (check-in damage / field report), reporter, date logged, and status.
4. **Resolve** — Staff update status from `open` → `in_progress` → `resolved` from within the tab. The moment an asset has no remaining open/in-progress records, it automatically becomes bookable again — no separate manual "make available" step.

## Data model changes

See `equiptra-service-records-schema-update.sql`:
- `service_records` gains `source`, `status`, `reporter_user_id`, `reporter_name`, `reporter_email`, `resolved_at`, `resolved_by`.
- New `asset_availability` view is the single source of truth for whether an asset can be allocated — both the Services tab and the booking/shortage-detection logic should read from this view rather than each maintaining their own flag.

## UI scope (v1)

- Services/Repairs tab: filterable table (by status), click-through to full record detail including linked asset history.
- Public fault-report form: asset lookup/select, description field, reporter name + email (freelancer path) or auto-filled from session (staff path).
- Booking/allocation screens: assets with an open fault should show a visual indicator and be excluded from the assignable list, consistent with how out-for-hire assets are already handled.

## Explicitly deferred to v2

- Notifications when a new fault is logged (email/Slack alert to whoever manages repairs). The schema's `status` and `source` fields are designed so this can be layered on later without a redesign — no need to build it now with 4 users checking the tab directly.

## Retired from scope

- Monday.com Lambda relay for fault reporting. No replacement integration needed; this can be decommissioned once the new form/tab is live.

## AWS housekeeping — resources to remove alongside the code change

The Monday.com relay was a live AWS integration, not just application code, so removing the Go handler alone will leave orphaned infrastructure behind. Claude Code should locate and remove:

- **The Lambda function itself** that received the check-in damage call and created the Monday.com card.
- **Any trigger wiring** to that Lambda — API Gateway route, EventBridge rule, or direct invoke permissions granted to the Go backend's IAM role.
- **IAM policy statements** scoped specifically to invoking that Lambda (on the backend's execution role) — don't remove the role itself if it's shared with other permissions, just the now-unused statement.
- **Secrets Manager / Parameter Store entries** holding the Monday.com API token/board ID used by the Lambda.
- **CloudWatch log group** for that Lambda (`/aws/lambda/<function-name>`) — safe to delete once you've confirmed nothing else logs there.
- **The `monday_item_id` column's role** in `service_records` — the column can stay for historical records already linked to old Monday cards, but nothing should write to it going forward. Consider marking it deprecated in a comment rather than dropping it, so historic references aren't lost.

Separately, worth noting for completeness rather than action: the **CurrentRMS API** was never a live AWS dependency — it was only called from the one-off local migration scripts (`extract_photos.py`, CSV imports), not from any deployed AWS resource. There's nothing to decommission there beyond archiving those scripts once the final ~50 images and the country-of-origin backfill are done; it was never wired into the running app or its AWS footprint.

**Suggested approach for Claude Code:** before deleting anything, grep the Go codebase and any Terraform/CDK/CloudFormation/SAM templates for references to `monday` (case-insensitive) to get a complete list of what's wired to what, rather than removing resources based on assumption. Confirm the Lambda isn't shared with any other trigger path before deleting it outright.

## Open items to confirm before build

- Exact table/column names in the original schema file (this addendum was written without access to the original file in this session — reconcile enum/column names against whatever naming convention the rest of the schema uses before running the migration).
