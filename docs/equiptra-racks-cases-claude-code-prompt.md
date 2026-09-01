# Prompt for Claude Code — Racks & Cases

Before doing anything else, read `docs/equiptra-racks-cases-addendum.md` — this adds two
container concepts to the asset model: **racks** (fixed, permanent kits) and **cases** (packed
per job, empty between bookings), plus the checkout/check-in cascading, fault-swap, and
carnet/delivery-note handling that go with them.

This is a bigger piece of work than recent additions — please read the whole addendum first
before starting, since the workflows interlock (the checkout cascade, the swap mechanism, and
the carnet/delivery-note handling all depend on the same underlying `case_contents` /
`home_rack_id` model).

Please:

1. **Resolve the open question at the end of the addendum first** — ask me directly whether
   rack membership / case content editing should be admin-only or available to standard users,
   rather than guessing.

2. **Implement the schema changes**: `container_type` and `home_rack_id` on `assets`, the new
   `case_contents` table, and `return_to_home_rack` on `booking_allocations`. Follow this repo's
   existing migration numbering/conventions.

3. **Build the cascading checkout/check-in logic** for both racks and cases, reusing the existing
   `booking_allocation` machinery rather than introducing parallel status tracking — a rack or
   case's contents should get real allocation records and show correctly in their own history,
   not just a container-level summary.

4. **Build the swap mechanism** — one underlying operation (clear old link, set new link) used in
   two contexts: permanent (`home_rack_id`) for racks, per-booking (`case_contents` row) for
   cases. The fault itself stays at the individual-item level via the existing `service_records`
   path — no new fault-handling logic needed, just make sure it works correctly on items that
   happen to be inside a container.

5. **Build the "pulled from rack" flow**: `return_to_home_rack` defaulting true when a
   rack-member asset is allocated outside its rack, surfaced as a non-blocking reminder at
   check-in, and shown in search results alongside the asset's live availability.

6. **Update carnet and delivery note generation**: carnet expands containers into itemized
   contents (weight/value/origin per item); delivery note keeps one line per container with
   pre-aggregated weight/value.

7. **Test** against the full checklist in the addendum — this touches checkout, check-in, fault
   handling, search, and both document generators, so a thorough pass matters more than usual
   here. Flag anything in the addendum that turns out to be ambiguous or conflicts with existing
   code once you're actually in there, rather than resolving it silently.

Follow existing conventions throughout. Given the scope, feel free to check in with a plan before
writing code if you think it'd help align on approach first.
