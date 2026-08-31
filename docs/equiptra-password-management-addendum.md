# Equiptra Build Brief — Addendum: Password Management

*This addendum should be merged into the main build brief before handoff. It adds two related
capabilities the original brief missed: self-service password change, and admin-initiated
password reset for a locked-out user.*

## Why both are needed

There's no email/SMTP infrastructure in Equiptra, so a traditional "forgot password" email link
isn't available. For a 4-user internal tool, the practical equivalent is: users can change their
own password when logged in, and an admin can reset a teammate's password directly when they're
locked out (forgotten password, left it written down somewhere insecure, etc.).

## Workflow

### Self-service change
1. Logged-in user goes to a Settings/Profile page and enters their current password, a new
   password, and a confirmation.
2. Backend verifies the current password against the stored hash before allowing the change.
3. Password is re-hashed and saved. No session invalidation needed — the user is already
   authenticated and doesn't need to re-log-in.

### Admin-initiated reset
1. From the existing user management screen, an admin selects "Reset password" on a user's row
   (alongside the existing deactivate/reactivate actions).
2. Admin sets a temporary password for that user (typed by the admin, or system-generated and
   shown once — admin's choice which is simpler to build first; typed is simplest for v1).
3. The affected user's account is flagged `must_change_password = true`.
4. Next time that user logs in, instead of landing in the normal app shell, they're routed to a
   forced "set a new password" screen. They must set a new password (which they alone know)
   before they can access anything else. Once set, `must_change_password` clears and normal
   access resumes.

This means the admin never ends up as the long-term holder of a teammate's real password — the
temporary one is single-use by design.

## Data model change

Add one column to `users`:

| Field | Type | Notes |
|---|---|---|
| `must_change_password` | boolean, default `false` | Set to `true` by an admin reset; cleared automatically when the user successfully sets their own new password |

No new table needed — this rides on the existing `users` table.

## Backend changes

- **`PATCH /api/users/me/password`** — self-service. Requires `currentPassword` + `newPassword`
  in the request body. Verifies `currentPassword` against the stored bcrypt hash before accepting
  the change; rejects with a clear error if it doesn't match.
- **`PATCH /api/users/:id/password`** — admin-only. Sets a new password directly (no current
  password check, since the admin is resetting on someone else's behalf) and sets
  `must_change_password = true` on that user's row.
- **Login handler change**: after successful authentication, check `must_change_password`. If
  true, issue a restricted-scope token that only permits calling the self-service password
  change endpoint, rather than a normal full-access session token. Once the password is changed
  via that endpoint, clear the flag and the user can log in normally on their next attempt (or
  the endpoint can issue a full session token directly on success — simpler UX, worth deciding
  during build).
- **Password policy**: keep simple for v1 — minimum 8 characters, no additional complexity rules.
  Reuse whatever bcrypt cost factor the existing auth system already uses.

## Frontend changes

- New Settings/Profile page (or a section of an existing one, if one already exists) with a
  change-password form: current password, new password, confirm new password.
- User management screen: add a "Reset password" action per user row, alongside the existing
  deactivate/reactivate controls. Prompts the admin for a new temporary password, submits it,
  and shows a confirmation the admin can relay to the affected user directly (Slack, in person,
  however LDMtv normally communicates internally).
- A forced "set new password" screen, shown instead of the normal authenticated shell whenever
  the logged-in user has `must_change_password = true`. No navigation away from this screen until
  a new password is successfully set.

## Interaction with existing self-lockout protection

The existing admin user management screen already has self-lockout protection (an admin can't
deactivate/delete their own account). Worth extending the same principle here: an admin resetting
their *own* password should probably go through the normal self-service flow (they already know
their current password), not the admin-reset flow — the admin-reset action should likely be
disabled or hidden on the admin's own row in the user list, consistent with how deactivate/delete
already behave there.

## Open items to confirm before build

- Temporary password: admin types one in, or system generates one and displays it once? (Typed
  is simpler to build; generated avoids weak/predictable temp passwords — worth a quick call.)
- On successful forced password change, should the user land straight in the app, or be sent
  back to the login screen to log in fresh with their new password? Either is fine at this scale;
  landing straight in is slightly better UX.
