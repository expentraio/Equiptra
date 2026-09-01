# Equiptra — Addendum: Racks & Cases

Equiptra currently treats every asset as an independent, individually-booked item. In practice,
a meaningful chunk of inventory travels as a unit: **racks** (fixed, permanent kits — a specific
flight case that always holds the same set of gear) and **cases** (containers packed differently
per job, empty between bookings). This addendum adds both concepts.

## Data model

### `assets` — two new columns
| Field | Type | Notes |
|---|---|---|
| `container_type` | enum, nullable | `rack` \| `case` \| null. Marks an asset as a container and which kind. |
| `home_rack_id` | FK → assets, nullable | The asset's *permanent* rack membership, if any. Set/cleared only via manual edit — this is metadata, not live booking state. Applies to non-container assets that live in a rack. |

### New table: `case_contents`
Tracks per-job packing for **cases only** (racks don't need this — `home_rack_id` already
represents rack membership, since it's stable rather than repacked each time).

| Field | Type | Notes |
|---|---|---|
| `id` | PK | |
| `case_asset_id` | FK → assets | the case |
| `content_asset_id` | FK → assets | item packed inside it |
| `booking_allocation_id` | FK → booking_allocations | ties the packing to a specific job |

Rows are created at pack-out, deleted at check-in. A case has no contents between jobs — a case
appearing in inventory with no active booking is simply empty.

### `booking_allocations` — one new column
| Field | Type | Notes |
|---|---|---|
| `return_to_home_rack` | boolean, default true | Set when an asset with a `home_rack_id` is allocated to a booking *other than via its rack* (i.e. pulled out individually for a different job instead of sub-hiring). Surfaces as a check-in reminder until confirmed. |

## Workflows

### 1. Checking out a rack or case
When a rack or case is checked out, the system auto-creates a `booking_allocation` for every
current content item too:
- **Rack**: contents = every asset with `home_rack_id` pointing at this rack.
- **Case**: contents = every asset currently linked via `case_contents` for this booking.

Each content item's own allocation record and history shows it as checked out on this booking —
this reuses the existing allocation machinery rather than introducing separate container-level
status tracking. Check-in cascades the same way: checking in the rack/case checks in its
contents' allocations too. For cases, the `case_contents` rows are then deleted (empty again for
next time). For racks, `home_rack_id` is untouched — the kit stays defined.

### 2. Swapping a faulty item — same mechanism for both racks and cases
If an item in a rack or a case develops a fault (whether mid-job or discovered at inspection),
it gets swapped for a replacement via a manual edit:
- **Rack**: clear `home_rack_id` on the faulty item, set it on the replacement. Permanent until
  changed again — if a repaired item doesn't go back in, that's a staff decision, nothing for
  the system to enforce automatically.
- **Case**: edit the relevant `case_contents` row for this booking — same swap, but scoped to
  just this job. Naturally disappears at check-in along with the rest of the case's contents,
  no separate cleanup needed.

The fault itself only affects the individual item (via the existing `service_records`
mechanism) — not the container as a whole. This matches how mid-rental faults already work
elsewhere in the app; no new fault-handling logic needed, just applying it at the item level
within a container.

### 3. Pulling a rack item for another job (instead of sub-hiring)
An asset with a `home_rack_id` can be allocated directly to a different booking, independent of
its rack. When this happens, `return_to_home_rack` defaults to `true` on that allocation.
- **Search**: this asset shows up in search normally (with its real availability), plus a note
  of its home rack (e.g. "Home: Rack R-4") regardless of whether it's currently there.
- **Check-in**: if `return_to_home_rack` is true, check-in surfaces a reminder — "this item
  needs to go back into Rack R-4" — until someone confirms it's been returned. Non-blocking,
  informational only, consistent with the app's existing flag-don't-block philosophy.

### 4. Carnet vs. delivery note
- **Carnet**: expands any rack/case into its individual contents as separate line items —
  description, weight, value, origin per item — since customs needs the real itemized list.
- **Delivery note**: stays one line per rack/case (e.g. "1x Camera Rack R-4"), with weight and
  value pre-aggregated from current contents. Matches what the client actually needs to see,
  with far less setup than full itemization.

## Testing checklist
- Checking out a rack cascades correctly to all its current contents' allocations and history.
- Checking out a case with packed contents (via `case_contents`) does the same.
- Checking in either clears cascaded allocations correctly; case contents are deleted after
  check-in, rack membership (`home_rack_id`) is untouched.
- Swapping a faulty rack item: old item's `home_rack_id` cleared, new item's set, change persists
  across future bookings until edited again.
- Swapping a faulty case item: change only affects the current booking's `case_contents`; doesn't
  persist to the next time that case is packed.
- Pulling a rack item for a different job: `return_to_home_rack` defaults true, search shows both
  live availability and home rack, check-in surfaces the reminder correctly.
- Carnet export on a project with a rack/case correctly itemizes contents individually.
- Delivery note on the same project shows the rack/case as a single line with correct aggregated
  weight/value.

## Still open
- Whether non-admin (standard) users can create/edit rack membership and case contents, or
  whether this should be admin-only given it's closer to inventory structure than day-to-day
  booking — worth a quick decision before build, consistent with how other edit actions in the
  app are scoped by role.
