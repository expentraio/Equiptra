# Equiptra — Addendum: Asset Editing & Project Status Changes

Two gaps found in testing: neither is a new feature exactly — both fields already exist in the
schema, there's just no way to change them after creation.

## 1. Asset editing

Currently there's no way to edit an existing asset at all. Real scenario that surfaced this:
a physical asset label gets damaged and needs its `asset_number` corrected — right now that's
not possible without going through the database directly.

### Fields that should be editable
- `asset_number` (the physical tag/label number)
- `serial_number`
- `location` (warehouse bay/shelf code)
- `notes`
- `status` (`active` / `written_off` / `sold` / `missing`) — already has *some* path via
  write-off flows if those exist; confirm whether this addendum's edit screen should be the only
  way to change status, or whether a dedicated "write off" action should remain separate for
  clarity (a write-off is a more significant event than a location correction).

Fields probably **not** worth exposing on this screen: `purchase_price`, `replacement_value`,
`purchase_date` — these are usually set once at intake and rarely corrected. Worth confirming
this assumption, but keeping the edit form focused on the fields that actually get corrected in
practice (label damage, moving shelves, typos) keeps it simple.

### Access
Admin-only, matching the pattern already used for other value-editing actions (per the existing
role split: admin can edit values/write off assets, standard can book/check in-out).

### Not in scope for this addendum
No audit history of what changed/when/by whom — acceptable for a 4-user internal tool at this
stage. Worth flagging as a possible future addition if `asset_number` corrections ever need to
be traceable for insurance purposes, but not blocking this fix.

## 2. Project status changes

Projects are currently created and stay locked at `tentative` — there's no way to move them
through `confirmed` → `in_progress` → `completed`, or mark one `cancelled`. The enum already
exists in the schema; nothing surfaces it.

### Approach
A status control on the project detail screen (dropdown or button group) allowing any authorized
user to change status. Consistent with the app's existing "flag, don't block" philosophy used
elsewhere (conflict detection, shortage handling):

- No hard restrictions on which transitions are allowed (e.g. going straight from `tentative` to
  `completed` should be permitted, even if unusual — real workflows don't always move linearly).
- **Soft warnings only** where it matters: if a user marks a project `completed` or `cancelled`
  while it still has assets in `allocated` or `checked_out` status, show a warning ("3 assets are
  still checked out on this project") but allow the change to go through if confirmed. Don't
  hard-block — matches how conflict overrides already work elsewhere in the app.

### Access
Doesn't need to be admin-only — this is closer to day-to-day booking management than a
value-editing action, so standard users should likely be able to change project status. Worth
confirming this matches expectations, but defaulting to "same access as creating/editing a
booking" seems right.

## Testing checklist
- Asset edit: change `asset_number`, confirm it's reflected everywhere the old number would have
  shown (booking history, tags/chips throughout the UI, carnet/delivery note generation if
  already built).
- Project status: move a project through several transitions, including an unusual one (e.g.
  `tentative` → `cancelled` directly), and confirm the warning appears (but doesn't block) when
  active allocations exist.
