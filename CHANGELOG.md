# Changelog

## v0.30.0 — Pre-launch surface strip (Phase 1)

**Breaking release.** Removes three concepts from the user-facing CLI
before launch — they were OSS pre-release surface that didn't survive
field-signal review or didn't match how teams use the tool. Pipescloud
will layer org/team/project management above the OSS account primitive
in its closed-source control plane.

### Removed

- **`ppz org`** — `ppz org list/switch/create/invite` are gone. Multi-org
  tenancy moves to pipescloud's control plane; OSS keeps single-tenant
  accounts as the default deployment shape. The HTTP endpoints
  `GET /api/v1/orgs` and `POST /api/v1/orgs` are also removed.
- **`ppz broadcast`** — both the CLI verb and the `<handle>.broadcast`
  auto-provisioned pipe are gone. Teams overwhelmingly use shared "room"
  pipes (e.g. `ppz pipe create team1.room` with implicit `--writers=anyone`),
  not one-to-many announce.
- **`ppz source switch / clear`** — gone (cleanly replaced; see migration
  table below). `ppz source create` and `ppz source destroy` *survive*
  the strip — their semantics aren't covered by other verbs.

### Renamed (schema + Go types)

- `organisations` table → `accounts`
- `organisation_members` table → `account_members`
- `organisation_id` columns → `account_id` (api_keys, sources, invites,
  account_members)
- `db.Organisation` Go type → `db.Account`; methods follow
  (`InsertAccount`, `ListAccounts`, etc.)
- `OrganisationID` Go fields and `OrgID`/`OrgName` JSON fields →
  `AccountID` / `account_id` / `account_name` everywhere (`StatusReply`,
  `LoginReply`, `AuthExchangeRequest`, `Credentials`, `Invite`).

### New

- **`ppz set [key] [value]`** — daemon-state CLI pattern. Day-one keys:
  `handle`.
  - `ppz set handle HANDLE` switches the daemon's current handle
    (replaces `ppz source switch HANDLE`).
- **`ppz unset [key]`** — clears state.
  - `ppz unset handle` (replaces `ppz source clear`).
- **`ppz get [key]`** — reads state. Single-line stdout; exits 1 if
  empty so `$(ppz get handle) || handle=` is scriptable.
- **`ppz pipe destroy --recursive HANDLE`** — bulk destroys every pipe
  under a handle, plus the handle row itself. Replaces
  `ppz source destroy HANDLE`.
- **`ppz terminal create HANDLE`** — provisions a pty-kind handle
  (inbox + stdin/stdout/stdctrl pipes) and sets it as current. Direct
  replacement for `ppz source create HANDLE` when you want the full
  pty-style pipe set. (`ppz agent create HANDLE` already existed since
  v0.29.)

### Migration

- **Schema is destructive**: the `organisations` → `accounts` rename
  cannot be applied to existing pre-launch installs as a no-op.
  Self-hosters on v0.29 or earlier must **drop and reinitialise the
  database**. Pre-launch with no production users this is acceptable.
- **CLI verb replacements**: at-a-glance migration table—

  | Pre-Phase 1 (v0.29) | Post-Phase 1 (v0.30) |
  |---|---|
  | `ppz org list/switch/create/invite` | (web UI — pipescloud only) |
  | `ppz broadcast HANDLE MSG` | `ppz pipe create HANDLE.room` once, then `ppz send HANDLE.room MSG` |
  | `ppz source create HANDLE` | unchanged — `ppz source create HANDLE` (bare actor identity; auto-pipe set is just inbox). For richer pipe bundles, use `ppz terminal create HANDLE` (pty) or `ppz agent create HANDLE` (agent harness). |
  | `ppz source switch HANDLE` | `ppz set handle HANDLE` |
  | `ppz source clear` | `ppz unset handle` |
  | `ppz source destroy PATTERN` | unchanged — `ppz source destroy PATTERN` (glob across handles and pipes). For per-handle recursive destroy, `ppz pipe destroy --recursive HANDLE` also works. |

### Internal / not user-visible

- IPC verb constants `IPCBroadcast` / `IPCBroadcastBatch` retained as
  the publish-IPC path (`ppz send`, `ppz command`, terminal stdin
  forwarding still use them); commented as such.
- IPC verb constants `IPCSwitch` / `IPCDisconnect` / `IPCSourceDestroy`
  retained as the daemon-state mutation path; `ppz set handle` /
  `ppz unset handle` / `ppz pipe destroy --recursive` route through
  them.
- `db.Source` Go type, `sources` table, and `db.Source.Pipes()` /
  `IsAutoPipe()` retained — terminal/agent create still go through
  them. Pipes table's `LastBroadcastAt` / `LastBroadcastPayload`
  columns dead but harmless until the schema fully collapses.
- The "drop sources table; subject grammar collapses to
  `<account>.<path>`" architectural step is deferred to a follow-up
  release (would be Phase 1.5 or fall out of Phase 3 ACL work).
