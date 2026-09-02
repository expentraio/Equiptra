# Equiptra — Addendum: Monday.com Project Lookup

Adds a manual "fetch from Monday" action on project creation: type the job's order number
(the `###-###` format, e.g. "536-244"), click a button, and pull `name`, `client`, `start_date`,
`end_date`, `client_reference`, and `delivery_address` in from the corresponding Monday.com item
instead of retyping everything by hand.

## Why this is low-risk

- **No schema changes.** Every target field already exists on `projects` — this is purely a
  data-entry convenience, not a new data model.
- **Reuses a proven pattern.** The retired fault-reporting integration already established the
  board-ID + column-map + server-side-token approach. This is the same shape, pointed at a
  different board.
- **Manual trigger only.** No automatic lookups, no risk of surprise API calls while someone's
  mid-typing an order number.

## Configuration

New environment variables (server-side only, same handling as any other secret in this app):
- `MONDAY_API_TOKEN`
- `MONDAY_PROJECTS_BOARD_ID` = `739667801` — confirmed, this is the "Master Sheet" board
- `MONDAY_PROJECTS_COLUMN_MAP` — confirmed against the live board:

  | Equiptra field | Monday column | Column ID | Type |
  |---|---|---|---|
  | `order_number` (lookup key) | Item | `name` | Monday's built-in item name — queried as `item.name`, not via `column_values` like the rest |
  | `name` | Fixture | `text` | Plain text |
  | `client` | Client | `tags9` | **Tags type** — API returns an array of tag objects, not a plain string. Needs a decision: join tag labels with a separator, or take the first tag only — items in practice appear to carry a single client tag, but confirm this before assuming it. |
  | `start_date` / `end_date` | Date | `date5` | **Date range/duration type** — one column holds both dates. Confirm the exact JSON shape Monday's API returns for a ranged date column (this differs from a plain single-date column) before mapping to `start_date`/`end_date`. |
  | `client_reference` | PO Number | `text8` | Plain text |
  | `delivery_address` | Location | `location` | **Location type** — structured value (address string plus lat/lng), not plain text. Extract the address portion for `delivery_address`. |

  Since three of these six are non-text column types, don't assume a uniform string-based
  mapping — verify each column's actual API response shape (e.g. via a test query against a real
  item) before writing the parsing logic.

## Backend

New endpoint, e.g. `GET /api/monday/project-lookup?order_number=536-244`:
- Same access level as project creation (no reason to restrict this further than that).
- Queries the Monday board via their GraphQL API for an item matching the given order number in
  the configured column.
- Maps the result to `{name, client, start_date, end_date, client_reference, delivery_address}`
  and returns it — no database write happens here, this only returns data for the frontend to
  populate a form with.
- **No match**: return a clean "not found" response — never block manual project creation.
- **Ambiguous match** (more than one item with that order number): return an error rather than
  guessing which one is right; surface this to the user so they can resolve it in Monday or enter
  details manually.
- **Monday API unreachable / token invalid**: fail cleanly with a clear error, not a crash —
  the user should always be able to fall back to typing project details in by hand.

## Frontend

- On the new-project screen: an "Order number" field plus a "Fetch from Monday" button.
- On click, call the lookup endpoint and populate the form fields with the result — this
  pre-fills the form for review, it doesn't auto-save. The user can adjust anything before
  submitting, same as if they'd typed it themselves.
- Show a clear inline message on no-match or error, without blocking the rest of the form.
- Re-fetching on an *existing* project (editing) is out of scope for this version — this only
  applies at creation time. Worth revisiting later if job details in Monday change after initial
  entry and people want to re-sync, but not needed for v1.

## Testing checklist

- Valid order number returns correct data and fills the form correctly.
- No match: clean error, manual entry still works.
- Ambiguous match: clean error, doesn't silently pick one.
- Misconfigured/invalid token: clean error, not a crash, manual entry still works.
- Confirm the "no re-fetch on existing projects" limitation doesn't cause confusion — e.g. if
  someone edits an existing project, the order number field being present shouldn't imply a
  re-fetch is possible if it isn't wired up.
