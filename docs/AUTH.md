# Auth — admin web UI authentication

ppz-server's admin web UI authentication is controlled by the
`PPZ_SERVER_AUTH_MODE` environment variable, read once at boot.
Three modes are supported:

| Mode | Behaviour | Use case |
|---|---|---|
| `none` (default) | Admin UI is unauthenticated. Session auto-completes via `/login`. | Trusted-network deploys — firewall yourself. |
| `password` | Username + password form against `users.password_hash` (bcrypt). | Self-hosters who want auth but no SSO. |
| `oauth` | Delegates to an out-of-tree `auth.Provider` implementation. OSS ships a stub returning "not configured". | Pipescloud's hosted product (GitHub OAuth, SAML, etc.). |

All three modes terminate in the same downstream contract: a `user_id`
session cookie that authenticated routes consume identically.

## Bootstrap flow

To set up password-mode auth on a fresh server:

1. Boot in `auth_mode=none` (the default).
2. Visit the admin UI and create user(s) via the Users tab on an
   account page. Set a password during creation.
3. Set `PPZ_SERVER_AUTH_MODE=password` in the server environment.
4. Restart `ppz-server`.

The next visit to `/login` renders the password form. Users who were
created without a password (i.e. `password_hash IS NULL`) cannot sign
in until an operator sets a hash via the Users page.

## Break-glass: forgotten password

The same env-var flip is the recovery path:

1. Stop `ppz-server`.
2. Set `PPZ_SERVER_AUTH_MODE=none`.
3. Start `ppz-server`. Admin UI is now open.
4. Reset the user's password via the GUI.
5. Set `PPZ_SERVER_AUTH_MODE=password` and restart.

## OAuth mode and the Provider interface

`auth_mode=oauth` makes `/login` delegate to `Server.Provider.Authorize()`.
OSS ships `*auth.StubProvider`, which returns HTTP 500 with a
"provider not configured" message — a deployment that sets
`PPZ_SERVER_AUTH_MODE=oauth` without installing a real provider fails
visibly rather than silently.

Pipescloud (and any other out-of-tree distributor) implements
`internal/auth.Provider` and wires their implementation into the
`Server.Provider` field. The `internal/auth.Provider` interface in OSS
is the stable contract.

## Daemon authentication is unaffected

`ppz daemon login <server>` continues to use API keys (`-apikey`) or
the device-code flow (`/oauth/device/code`, `/oauth/device/verify`,
`/oauth/device/token`). `PPZ_SERVER_AUTH_MODE` only governs the admin
web UI; daemon auth is unchanged across all three modes.

What the device-verify page does at user-click time depends on
`auth_mode`: under `none` it auto-approves, under `password` it
renders a form, under `oauth` it routes through the configured
Provider.

## What's stored

- `users.password_hash` — bcrypt hash (cost 10). Nullable; NULL means
  "this user can't sign in via password" (e.g. an OAuth-only user
  or a row created pre-Phase-2).
- Session cookie — HMAC-signed payload `{user_id, expires_at}`. Set
  by every mode after a successful authentication.

## See also

- `pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md` — the
  RED→GREEN cycle log for the work that produced this state.
- `pipes-internal/docs/OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md` —
  the strategic OSS-vs-SaaS split this doc serves.
