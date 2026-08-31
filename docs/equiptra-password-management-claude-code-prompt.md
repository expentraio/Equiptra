# Prompt for Claude Code — Password Management (self-service + admin reset)

Before doing anything else, read:

1. `docs/equiptra-password-management-addendum.md` — the spec for this feature: both the
   self-service password change flow and the admin-initiated reset flow for locked-out users,
   including the data model change and the two open questions at the bottom.
2. The existing auth code in this repo — the JWT/bcrypt login handler, the `RequireAuth`
   middleware, and (from the recent fault-reporting work) the `OptionalAuth` middleware, since
   the forced-password-change flow needs a third auth state: "authenticated, but restricted to
   only the password-change endpoint until they set a new password."
3. The existing user management screen/handlers (deactivate/reactivate, self-lockout
   protection) — this feature's admin-reset action and self-lockout carve-out should match that
   screen's existing patterns exactly rather than introducing new ones.

Please:

1. **Resolve the two open questions in the addendum before writing code** — ask me directly
   rather than guessing: (a) should the admin type a temporary password or should the system
   generate one, and (b) after a forced password change, should the user land straight in the
   app or be sent back to login. Both are quick calls but change what you build.

2. **Implement the schema change** — add `must_change_password` (boolean, default false) to
   `users`, following this repo's existing migration numbering/conventions.

3. **Build the two flows**:
   - Self-service: `PATCH /api/users/me/password`, requiring and verifying the current password
     before accepting a new one.
   - Admin reset: `PATCH /api/users/:id/password`, admin-only, sets a new password and flips
     `must_change_password` to true. Disable/hide this action on the admin's own row in the user
     list — an admin resetting their own password should use the self-service flow instead,
     matching how the existing deactivate/delete self-lockout protection already behaves.
   - Login handler: check `must_change_password` after successful auth and issue the
     restricted-scope outcome instead of a normal session (exact shape depends on your answer
     to point 1b above).

4. **Frontend**: a Settings/Profile page for self-service change; a "Reset password" action on
   the user management screen; a forced "set new password" screen shown whenever
   `must_change_password` is true, with no way to navigate elsewhere until it's cleared.

5. **Password policy**: minimum 8 characters for v1, nothing more elaborate. Reuse the existing
   bcrypt cost factor already in use elsewhere in the codebase.

6. **Test**: current-password verification actually rejects wrong passwords; admin reset works
   and sets the flag; the forced-change screen blocks all other navigation until resolved; the
   self-lockout carve-out actually hides/disables the admin-reset action on the admin's own row;
   and confirm the existing login flow for users *without* `must_change_password` set is
   completely unaffected.

Follow existing conventions throughout — same handler style, same React patterns — this is an
addition to a tested codebase, not a rewrite.
