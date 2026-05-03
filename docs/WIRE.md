# ppz Wire Contracts

This document is **authoritative**. Implementation, tests, and clients must
match it byte-for-byte. If reality drifts, fix reality, not the doc — unless
the doc is being deliberately revised in the same commit alongside the
implementation and the test fixtures.

All timestamps are RFC3339 UTC (e.g. `2026-04-25T12:34:56Z`). All IDs are
UUID v7 unless noted. All JSON request/response bodies use
`application/json; charset=utf-8`.

## 0. Vocabulary

- **source** — the top-level addressable entity (formerly "pipe"). Each source
  has a unique `handle` within an organisation and a `kind` (`message` or
  `pty`).
- **pipe** — a named sub-bucket on a source where messages flow (formerly
  "channel"). A `message` source has one pipe (`broadcast`); a `pty` source
  has three (`broadcast`, `stdin`, `stdout`).

A target on the wire is `<source-handle>.<pipe-name>`.

## 1. Subject grammar (NATS)

```
<org_id>.<handle>.<pipe>
```

- `org_id` — UUID of the organisation (lowercase, hyphenated form).
- `handle` — source handle, regex `^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`, max 32.
- `pipe` — pipe name. Today: `broadcast`, `stdin`, `stdout`. Reserved: `inbox`,
  `system`, `db` (rejected if user attempts to create).

Reserved source handles (rejected at create): `system`, `db`.

## 2. JetStream stream config (per pipe)

| Field | Default |
|---|---|
| Name | `source_<orgshort>_<handle>_<pipe>` (orgshort = first 8 hex chars of org UUID, hyphens stripped) |
| Subjects | `<org_id>.<handle>.<pipe>` |
| Retention | Limits |
| MaxAge | 24h |
| MaxMsgs | 5000 |
| MaxBytes | 16 777 216 (16 MiB) |
| Storage | File |
| Discard | Old |
| Replicas | 1 |

## 3. Envelope

Published payload on every `<org_id>.<handle>.<pipe>`:

```json
{
  "id": "<uuid v7>",
  "handle": "<source-handle>",
  "payload": "<utf-8 string>",
  "created_at": "<rfc3339>"
}
```

Constraints:
- `payload` is always a UTF-8 string. Binary data must be base64-encoded by the
  caller.
- Encoded envelope must not exceed 65 536 bytes after JSON marshalling. The
  daemon enforces this *before* publishing and returns `E_PAYLOAD_TOO_LARGE`.
- The `handle` field carries the **source** handle, not the pipe name. The
  pipe name is only encoded in the subject.

## 4. NATS auth

- Single account `PPZ` on the embedded NATS server.
- Server signs short-lived user JWTs scoped to a single org.
- JWT permissions: `pub.allow=["<org_id>.>"]`, `sub.allow=["<org_id>.>"]`.
- TTL: 1 hour. Daemon refreshes at `exp − 5 min`.

## 5. HTTP API (`/api/v1`)

Auth: `Authorization: Bearer <api_key_plaintext>` on every endpoint **except**
the GUI HTML routes (which are unauthenticated).

`/auth/exchange` is the only endpoint where the API key is sent in the body
rather than the header (this allows the daemon to pre-validate the key during
`ppz login` before storing it). All subsequent calls use the header.

Error response shape (any non-2xx):
```json
{"error": {"code": "E_*", "message": "<human readable>"}}
```

### 5.1 POST /api/v1/auth/exchange
Body: `{"api_key": "<plaintext>"}`

200:
```json
{
  "jwt": "<nats user jwt>",
  "nats_url": "nats://<host>:4222",
  "org_id": "<uuid>",
  "expires_at": "<rfc3339>"
}
```

Errors: 401 `E_INVALID_API_KEY`.

### 5.2 POST /api/v1/sources
Body: `{"handle": "foo", "kind": "message" | "pty"}`

201:
```json
{
  "id": "<uuid>",
  "handle": "foo",
  "kind": "message",
  "created_at": "<rfc3339>"
}
```

Errors: 401, 400 `E_INVALID_HANDLE`, 409 `E_SOURCE_TAKEN`.

### 5.3 GET /api/v1/sources

200 (sorted by handle ASC):
```json
{
  "sources": [
    {
      "handle": "alpha",
      "kind": "message",
      "last_broadcast_at": "2026-04-25T12:34:56Z",
      "last_broadcast_payload": "hello"
    },
    {
      "handle": "beta",
      "kind": "pty",
      "last_broadcast_at": null,
      "last_broadcast_payload": null
    }
  ]
}
```

Errors: 401.

### 5.4 GET /api/v1/sources/{handle}
200: same shape as a single element of `/api/v1/sources`.
Errors: 401, 404 `E_SOURCE_NOT_FOUND`.

## 6. Server GUI (HTML, unauthenticated)

| Method | Path | Behaviour |
|---|---|---|
| GET | `/` | Lists orgs (id, name, created_at). Form posts to `/orgs`. |
| POST | `/orgs` | Form field `name`. Creates org. Redirects to `/orgs/{slug}`. |
| GET | `/orgs/{slug}` | Shows api keys (label, prefix, created_at) and sources as a table: handle, pipe, last_broadcast_at, payload (truncated to 60 chars). Pipe cell links to the pipe detail page. Form posts to `/orgs/{slug}/keys`. |
| POST | `/orgs/{slug}/keys` | Form field `label`. Creates api key. Renders the **plaintext key once** on the response page. Subsequent visits to `/orgs/{slug}` show only the prefix. |
| GET | `/orgs/{slug}/sources/{handle}/pipes/{pipe}` | Lists every buffered message on this pipe from the JetStream stream, in chronological order. Honors stream retention (defaults: 24 h / 1000 msgs / 64 MiB, whichever first). |

All HTML pages are server-rendered (Go `html/template`). No JS framework.

### 6.1 Stable HTML markers (for tests)

Tests scrape these via `grep -oP 'data-…="\K[^"]+'`. They are part of the
contract — do not rename:

| Page | Marker | Value |
|---|---|---|
| `GET /` | `data-org="<name>"` | one per org listed |
| `GET /orgs/{slug}` | `data-key-prefix="<prefix>"` | one per api key |
| `GET /orgs/{slug}` | `data-source="<handle>"` | one per source row in the table |
| `GET /orgs/{slug}` | `data-source-row="<handle>:<pipe>:<rfc3339-or-empty>:<payload60-or-empty>"` | one per (source, pipe) row |
| `GET /orgs/{slug}` | `data-source-pipe-link="/orgs/<slug>/sources/<handle>/pipes/<pipe>"` | one per pipe cell |
| `GET /orgs/{slug}/sources/{handle}/pipes/{pipe}` | `data-message="<id>:<rfc3339>:<payload>"` | one per buffered message, chronological |
| `POST /orgs/{slug}/keys` (response page) | `data-new-key="<plaintext>"` | exactly one |

## 7. CLI ↔ daemon IPC

Unix domain socket at `$PPZ_IPC_SOCKET` (default `$PPZ_HOME/daemon.sock`,
which itself defaults to `~/.ppz/daemon.sock`).

Wire format: newline-delimited JSON-RPC 2.0. One request per connection, the
daemon writes one response and closes (simple half-duplex; long-running reads
keep the connection open and stream `ReadEvent` lines until the client closes).

Methods (Phase 1.5 reality — verbs unchanged in this phase):

| Method | Params | Result |
|---|---|---|
| `Status` | `{}` | `{"daemon_pid":int,"logged_in":bool,"url":str?,"key_prefix":str?,"org_id":str?,"current":str?}` |
| `Login` | `{"url":str,"api_key":str}` | `{"url":str,"key_prefix":str,"org_id":str}` |
| `Create` | `{"handle":str,"kind":str}` | `{"handle":str,"subject":str,"kind":str}` |
| `Switch` | `{"handle":str}` | `{"handle":str}` |
| `Broadcast` | `{"handle":str?,"channel":str?,"payload":str}` | `{"id":str,"subject":str,"bytes":int}` |
| `List` | `{"session":str?}` | `{"sources":[{"handle":str,"kind":str,"pipe_infos":[{"pipe":str,"total":int,"unread":int,"last_at":str?,"preview":str},…],"last_broadcast_at":str?,"last_broadcast_payload":str?},…]}` |
| `Read` | `{"handle":str,"channel":str,"limit":int?,"skip":int?,"since_ms":int?,"json":bool?,"follow":bool?,"session":str?,"no_advance":bool?}` | streaming `ReadEvent` JSON lines |

Errors are returned as JSON-RPC errors with `code` = the integer exit code from
ERRORS.md and `message` = `"E_FOO: human readable"`.

(Note: legacy field names `channel` on `Broadcast`/`Read` requests still carry
the pipe name; this is preserved for IPC backward-compat within the Phase A
rename. Phase B reorganises these.)

## 8. Pinned stdout (CLI)

The harness diffs stdout byte-for-byte after normalisation
(`tests/lib/normalize.sh`). The exact bytes are the contract.

### `ppz daemon`
First invocation: `daemon started pid=PID`
Already running:  `daemon already running pid=PID`
Foreground (`--foreground`): same first line, then blocks.

### `ppz status` — exactly four lines
Logged in with a current source:
```
daemon: running pid=PID
login: <URL> key=KEYPREFIX
org: <org_id>
current: <handle>
```
Logged in, no current source: same with `current: -`.
Logged in, no auth: `login: not logged in`, `org: -`, `current: -`.
Daemon not running (exit 11): `daemon: not running`.

### `ppz login URL -apikey K`
```
logged in url=<URL> key=KEYPREFIX org=<org_id>
```

### `ppz create HANDLE` — two lines
```
created handle=<handle> subject=<org_id>.<handle>.broadcast
current handle=<handle>
```

### `ppz switch HANDLE`
```
current handle=<handle>
```

### `ppz broadcast` (`-m TEXT`, positional, or stdin)
```
sent id=UUID subject=<org_id>.<handle>.<pipe> bytes=<n>
```
Default pipe is `broadcast`. Stdin form strips a single trailing newline.

### `ppz send HANDLE.PIPE "PAYLOAD"`
Same line shape as broadcast, with the explicit pipe.

### `ppz read HANDLE.PIPE [-l N --skip N --since DUR --json -f]`
Default: prints `evt.Message.Payload` followed by `\n` per message.
`--json`: prints the full envelope JSON, one per line.

### `ppz kill`
`daemon stopped pid=PID` if running, `daemon not running` if not. Exit 0 either way.

### `ppz ls`
One line per (source, pipe), sorted by `<handle>.<pipe>` ASC. Fields separated
by single spaces. Missing values `-`. Preview is the most recent payload
truncated to 60 chars (UTF-8 safe), with ANSI CSI sequences and C0 controls
stripped.

```
<handle>.<pipe> <total> <unread> <last_at|-> <preview60|->
```

Empty list: zero output, exit 0.

### `ppz terminal create HANDLE [-- CMD ...]`
Wraps a shell (or `<cmd>`) in a PTY tied to source `HANDLE` (kind=pty), with
`PPZ_CURRENT_HANDLE=<handle>` exported to the child. Stdout chunks publish
verbatim to `<handle>.stdout`; subscribed `<handle>.stdin` messages forward to
the PTY master. Foreground; blocks until child exits. Exit 0 on clean child
exit.

### `ppz terminal view HANDLE`
TUI viewer: enters alt-screen, follows `<handle>.stdout` until SIGINT/Ctrl-C,
exits alt-screen, exit 0.

## 8a. Desktop `--dump-state` (test mode)

`ppz-desktop --dump-state --ipc=<sock>` connects to the named daemon, asks for
its source list (via `List`), and prints a JSON snapshot of exactly the state
the GUI would render, then exits 0.

```json
{
  "logged_in": true,
  "org_id": "<uuid>",
  "sources": [
    {"handle":"chat","last_broadcast_at":"<rfc3339>|null","last_broadcast_payload":"hello|null"}
  ]
}
```

If not logged in: `org_id` is `null`, `sources` is `[]`.

## 9. Daemon on-disk state

Under `$PPZ_HOME` (default `~/.ppz`):

| File | Contents |
|---|---|
| `daemon.pid` | The daemon process PID, written at startup. |
| `daemon.sock` | The unix-domain IPC socket. |
| `credentials` | JSON `{"url":"…","api_key":"…"}`, mode 0600. Absent ⇒ not logged in. |
| `current` | Plain text: the current source handle, no trailing newline. Absent ⇒ no current source. |
| `cursors/<session>.json` | Per-session map `{"<orgID>.<handle>.<pipe>": <stream_seq>, …}` — highest delivered JetStream sequence per pipe per session. Used for unread counts in `ls` and to resume `read`. Session id resolves from `$PPZ_SESSION` → `tty(1)` → `"default"`. |

The daemon does NOT cache cursor state in memory across calls — every Get/Advance
re-reads/writes the file. (The harness wipes `cursors/` between scenarios; an
in-memory cache would mask that wipe and cause false-negative unread counts.)

## 10. Test-only knobs

| Env var | Purpose |
|---|---|
| `PPZ_TEST_CLOCK` | If set to an RFC3339 timestamp, server- and daemon-issued `created_at` fields use this value (frozen clock). Honoured by `internal/clock`. |
| `PPZ_IPC_SOCKET` | Override unix socket path (test-runner uses two: `/tmp/a/daemon.sock`, `/tmp/b/daemon.sock`). |
| `PPZ_HOME` | Override credential storage dir (default `~/.ppz`). |
| `PPZ_SESSION` | Override session id used to key cursor state (otherwise derived from `tty(1)`, falling back to `"default"`). |
| `PPZ_TEST_FILTER` | Glob filter for `tests/run.sh`. |
| `PPZ_NATS_URL` | Override the NATS URL the daemon dials, regardless of what `/auth/exchange` returned. Useful when running daemon outside compose against an in-compose NATS. |
| `PPZ_CURRENT_HANDLE` | Override the daemon's `current` for one CLI invocation (used by `terminal create` so the wrapped child's `broadcast` targets the wrap's source). |
