# Prompt for Claude Code — Asset Editing & Project Status Changes

Before doing anything else, read `docs/equiptra-editing-gaps-addendum.md` — two gaps found in
testing: no way to edit an existing asset (e.g. correcting `asset_number` after a damaged label),
and no way to change a project's status past `tentative`.

Please:

1. **Confirm two open questions from the addendum before building**, rather than guessing:
   - Should asset status changes go through this new edit screen, or should write-offs stay a
     separate, more deliberate action given they're a bigger event than a location correction?
   - Should project status changes be admin-only, or available to standard users (my instinct is
     standard users, since it's day-to-day booking management, not value-editing — but confirm
     this matches how the rest of the role split is applied elsewhere in the app)?

2. **Asset editing**: add an edit action (admin-only) covering `asset_number`, `serial_number`,
   `location`, `notes`, and (pending the question above) `status`. Check everywhere an asset's
   number/serial is displayed — tags/chips, booking history, any generated documents — and
   confirm an edit is reflected consistently, not just on the asset's own detail view.

3. **Project status**: add a status control on the project detail screen allowing any valid
   transition (no hard-blocked paths). When marking a project `completed` or `cancelled` while it
   still has `allocated` or `checked_out` assets against it, show a warning listing the affected
   assets but allow the change to proceed if confirmed — matching the existing conflict-override
   pattern elsewhere in the app rather than introducing a new blocking pattern.

4. **Test**: asset number correction propagates everywhere it's shown; project status moves
   through an unusual transition (e.g. `tentative` → `cancelled` directly) and the warning
   appears correctly when active allocations exist, without blocking the change.

Follow existing conventions throughout — this is filling in missing CRUD on schema that already
exists, not new architecture.
