# Prompt for Claude Code — Fault Reporting / Services Tab feature

I'm extending the existing Equiptra codebase (Go backend, React frontend, Postgres) with a
fault-reporting workflow, and retiring the current Monday.com Lambda integration in the same
change. Three reference documents are attached:

1. `equiptra-build-brief-fault-workflow-addendum.md` — the spec for this feature: the workflow,
   data model changes, UI scope, and an AWS housekeeping checklist for what to remove.
2. `equiptra-service-records-schema-update.sql` — a draft migration extending `service_records`.
   This was written without access to the live schema, so column/table names are assumptions —
   reconcile against the actual schema in this repo before applying, don't run it blind.
3. `equiptra-workflow-updated.mermaid` — the updated end-to-end workflow diagram, showing where
   the new fault-reporting branch replaces the old Monday.com relay step.

Please:

1. **Read the existing codebase first** — find the current `service_records` table/migration,
   the check-in damage handler, and the Monday.com Lambda integration (grep for `monday`,
   case-insensitive, across Go source and any Terraform/CDK/CloudFormation/SAM templates) before
   changing anything. Confirm your understanding of the current wiring matches the addendum's
   assumptions, and flag any discrepancies before proceeding.

2. **Implement the schema change** — adapt the draft migration to match this repo's actual
   naming conventions and migration tooling, rather than applying it as-is.

3. **Build the feature**:
   - Public fault-report form (no auth required) accepting either an authenticated staff
     submission or a freelancer's name/email
   - New Services/Repairs tab in the frontend, following existing UI patterns/components
   - Booking/allocation logic updated to exclude assets with an open or in-progress
     service record from future allocations (existing bookings unaffected — this is
     forward-looking only, not retroactive)
   - Status lifecycle (open → in_progress → resolved) with automatic return to the
     bookable pool on resolution

4. **Remove the Monday.com integration** — the Go handler/call, and the AWS resources listed
   in the addendum's housekeeping section (Lambda function, trigger wiring, IAM policy
   statements, Secrets Manager entries, CloudWatch log group). Don't drop the `monday_item_id`
   column — leave it in place for historic records, just stop writing to it.

5. **Follow existing conventions** throughout — same handler style as the other Go files, same
   React component patterns as the rest of the app. This is an addition to a tested,
   working codebase, not a rewrite.

Ask me before deleting any AWS resource if you're not fully certain it's isolated to the
Monday.com path and not shared with anything else still in use.
