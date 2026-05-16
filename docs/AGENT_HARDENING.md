# Agent Hardening Plan

Status: **planning** (2026-05-09). Plan written before any code change; phased
so we observe before we fix.

## Why this exists

ppz is positioned as the substrate for multi-agent coordination. Two team-scale
runs have surfaced a clear pattern: **the data model is good, the reliability
isn't.**

- **Salt Town** (6 agents, 2026-05-07) — drove the v0.23–v0.25 envelope / ack /
  format work. Feedback was about visibility and completeness. Mostly addressed.
- **MoltHub** (5 agents on v0.25.0, 2026-05-08) — now reports substrate
  reliability problems: daemon stability, NATS auth handling, source/pipe
  state continuity. v0.25 features ARE being used; agents have moved past the
  data-model layer and are now hitting the substrate.

This document is the plan for the next reliability push. **Observe before
changing.** Reliability fixes without diagnosis are guessing.

## Scope

In scope:

- NATS connection lifecycle (connect, disconnect, reconnect, JWT rotation)
- Daemon process resilience (panics, restart-survival of in-flight state)
- Error message accuracy where MoltHub showed they actively misled

Explicitly out of scope (own roadmap, blocked on hardening):

| Feature | Source | Status |
|---|---|---|
| `ppz read --wait --timeout` | MoltHub 5/5, Salt Town 4/6 | Backlogged. Modest implementation. |
| `ppz who` | MoltHub 5/5 | Backlogged. UI sugar over `ls`. |
| Read-side filters (`--from`, `--subject`, `--no-acks`) | MoltHub 5/5, Salt Town 3/6 | Backlogged. Half-shipped (`--subject` is on send). |
| Tabular default for custom pipes | MoltHub 1/5 | `--meta` opt-in, symmetric to `--bare`. |
| Ack correlation enrichment | MoltHub 1/5 | Daemon enriches ack at emit time. |
| Shared "room" / cross-source semantics | MoltHub 4/5 | Open design question. |
| PTY input injection for orchestrators | MoltHub 1/5 (Claude) | Open. Investigate before scoping. |
| Broadcast subscriber feedback | Salt Town 3/6 | Open. Server-side state. |

Don't open features in this list before Phase 1 lands. The point of this plan
is to fix substrate before adding things that depend on it.

---

## Signals from MoltHub

Direct quotes, agent-attributed, so future readers can re-anchor on the source:

- *"NATS connectivity was flaky — I got E_NATS_UNREACHABLE errors intermittently,
  even though the daemon was running. Had to restart the daemon (`ppz daemon
  stop && ppz daemon start`) to recover. This happened multiple times
  unpredictably."* — Bob (Engineer)
- *"The daemon kept dying or becoming unreachable between commands. I had to
  restart it ~5 times before `ppz ls` worked. The `E_NATS_UNREACHABLE` and
  `E_SERVER_UNREACHABLE` errors were intermittent and confusing."* — Eve (Designer)
- *"The daemon crashed multiple times during my session (NATS auth expiry,
  SIGSEGV panic in the daemon log)."* — Charlie (Tester)
- *"About 5 minutes into the agents' work, `ppz ls` started returning
  `E_NATS_UNREACHABLE`. Required `ppz daemon stop`, then `ppz login
  pipescloud.io` (full re-auth flow including browser approval), then
  `ppz daemon start`. This caused the `common` source to temporarily disappear,
  breaking inter-agent communication."* — Claude (Orchestrator)
- *"NATS authentication expiry was the biggest pain point. The daemon was
  running and `ppz status` showed 'logged in,' but `ppz ls` threw
  `E_NATS_UNREACHABLE`."* — Alice (PM)

Common factor: connection drops without graceful recovery. Sometimes a daemon
restart suffices, sometimes a full re-auth is required. The agent never has a
signal that something's wrong until an IPC call fails.

---

## Diagnostic posture

### What we know (from code reading)

- `nats.Connect` is invoked with **no explicit reconnect options** at
  `internal/daemon/handlers.go:38–44` (`connectNATSWithJWT`) and `:51–78`
  (`connectNATSWithRefresh`). NATS Go client defaults apply:
  `MaxReconnects=60`, `ReconnectWait=2s`, no disconnect/reconnect handler
  callbacks. After ~120s of reconnect attempts, the client gives up; the
  daemon's `d.NC.IsConnected()` returns false thereafter, and every
  NATS-touching IPC call fails with `E_NATS_UNREACHABLE`.
- The refresh loop (`internal/daemon/refresh.go`) rotates JWTs 30s before
  exp, retries transients on a 5s backoff, signals `OnUnauthorized` on
  revocation. Looks sound on its own.
- The `RefreshLoop.Current()` returns the freshest `(jwt, seed)` to the
  `nats.UserJWT` callback. **But the callback only fires on (re)connect** —
  so a freshly-rotated JWT doesn't update the active connection until the
  next disconnect-and-reconnect cycle. Window for mismatch: server expires
  the JWT-bound session, client doesn't notice until traffic.
- **No `nats.DisconnectErrHandler` / `ReconnectHandler` / `ClosedHandler`
  registered anywhere.** When the connection drops or recovers, the daemon
  is silent. Operators (and agents) have no way to see what's happening.

### What we suspect, not yet verified

- The default 60-retry cap is the **dominant cause** of "permanent
  E_NATS_UNREACHABLE until daemon restart."
- JWT expiry without a corresponding clean reconnect is a **secondary
  cause**: server boots the session, client lags noticing.
- Charlie's SIGSEGV is **a separate bug**, possibly a nil-deref under
  reconnect or in the refresh path. We need the actual stack trace.
- Claude's "common source temporarily disappeared after re-auth" is the
  daemon's `KnowsPipe` cache being reset, then lazily re-fetched. State
  loss across daemon restarts is a real but distinct problem.

### What we need to learn

- Confirmed list of triggers for the disconnect (network blip vs JWT
  expiry vs server restart vs other).
- Whether reconnect succeeds for each cause, and how long it takes.
- The SIGSEGV stack trace.
- The shape of state loss across daemon restarts.

We won't know any of these without instrumentation. Hence Phase 0.

---

## Phase 0 — Observability (instrument before changing)

**Goal:** make the daemon's NATS connection state visible. No behavior change;
just logging and a status surface.

**Why first:** every fix in Phase 1 should be verifiable against real
before/after data. We don't have that today, and asking the MoltHub team to
re-run blind doesn't give us anything new.

### Deliverables

1. **Disconnect / reconnect handlers.** Register `nats.DisconnectErrHandler`,
   `nats.ReconnectHandler`, `nats.ClosedHandler` on every `nats.Connect`
   call site. Each writes a structured line to the daemon's diagnostics log
   (timestamp, event, error if any, attempt counter).
2. **Connection-state ring buffer in `Daemon`.** Tracks last N disconnect /
   reconnect events with timestamps and reasons. Capped at, say, 32
   entries. Read-only access for `ppz status` and `ppz diagnostics`.
3. **`ppz status` shows NATS state.** New line:

   ```
   nats: connected (last drop: 14:23:01 — server unreachable, last reconnect: 14:23:03; drops in last hour: 2)
   ```

   Or when disconnected:

   ```
   nats: disconnected since 14:23:01 (server unreachable; reconnect attempts: 7)
   ```
4. **`ppz diagnostics` (new verb).** Dumps the diagnostics log tail and the connection-
   state ring buffer. Replaces the implicit "tail `/tmp/ppz-diag.log`"
   workflow MoltHub had to discover.
5. **WIRE.md and `docs/ERRORS.md` updates.** Document the new `ppz status`
   format and the `ppz diagnostics` verb. Note diagnostics log location.

Phase 0 is **one PR**. ~1 day. Behavior-preserving except for the new
status field and verb, both additive.

### RED tests for Phase 0

Yes, fully testable. Three e2e fixtures under a new `tests/reliability/`
directory (don't pollute `wan/`; reliability has its own concerns). Compose
overlay extends `compose/docker-compose.yml` with no special networking —
just controlled stop/start of the `ppz-server` container.

**`tests/reliability/disconnect-handler-fires/`** — RED: the daemon writes a
disconnect event to the diagnostics log when NATS goes away.

```bash
# pseudocode
ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"
ppz_a source create chat
ppz_a broadcast -m "before drop"

docker stop compose-ppz-server-1
sleep 3
docker start compose-ppz-server-1
wait_for 30 'ppz_a diagnostics | grep -q "nats: reconnected"'

ppz_a diagnostics | grep -E "^nats: (disconnected|reconnected)" | head -4
```

Expected lines (after normalisation): `nats: disconnected …` then
`nats: reconnected …`. Asserts that handlers are registered AND the diag
log surfaces them.

**`tests/reliability/status-reports-nats-state/`** — RED: `ppz status`
includes a `nats:` line, with state matching reality.

```bash
ppz_a daemon login …
ppz_a status | grep '^nats:' | head -1   # → "nats: connected …"

docker stop compose-ppz-server-1
wait_for 10 'ppz_a status 2>/dev/null | grep -q "^nats: disconnected"'
ppz_a status | grep '^nats:' | head -1   # → "nats: disconnected …"
```

**`tests/reliability/diagnostics-verb-dumps-events/`** — RED: `ppz diagnostics` returns
the connection-state ring + diagnostics log tail.

```bash
ppz_a daemon login …
docker stop compose-ppz-server-1
sleep 3
docker start compose-ppz-server-1
wait_for 30 'ppz_a status | grep -q "^nats: connected"'

ppz_a diagnostics | grep -c '^nats:'   # → at least 2 (one disconnect, one reconnect)
```

All three are red against `main` today (no handlers registered, no `diag`
verb, no `nats:` line in `ppz status`). They turn green when Phase 0 ships.

### Acceptance criteria

- All three RED tests pass.
- `ppz status` output remains stable for unchanged callers (the new
  `nats:` line is a single addition; existing fixtures need updating in
  lockstep — same migration shape as v0.25 tabular default).
- No behaviour change for the connection itself — same retry counts, same
  failure modes. We're observing what's there, not changing it.

### What we expect to learn

After shipping Phase 0 and re-running with MoltHub (or our own soak):

1. Whether the 60-retry cap is hit in real-world drops (and how often).
2. What the disconnect causes are (server unreachable vs JWT expiry vs
   other).
3. Whether reconnect succeeds for each cause.
4. The SIGSEGV stack (if it reproduces).

That data scopes Phase 1.

---

## Phase 1 — NATS connection hardening

**Goal:** the daemon recovers transparently from connection drops, regardless
of cause and duration, without operator intervention.

### Hypotheses to verify (using Phase 0 data)

A. The 60-retry cap is hit in real-world drops.
B. Some drops correlate with JWT expiry windows.
C. Some drops have other causes (network blips, server restarts).
D. The SIGSEGV is in a specific code path.

### Likely fixes (scoped post-Phase 0)

These are the changes I'd expect to ship; we revise as Phase 0 data comes
in. **Each is a separate PR with a corresponding RED test.**

1. **`nats.MaxReconnects(-1)`** — unlimited reconnect attempts. Catches
   any-duration outage.
2. **`nats.ReconnectWait(2 * time.Second)` + `nats.ReconnectJitter(500ms,
   2s)`** — keeps reconnect storms tame without imposing a cap.
3. **`nats.RetryOnFailedConnect(true)`** — for the initial connection too,
   so a startup-time NATS unavailability isn't fatal.
4. **`OnUnauthorized` triggers re-auth.** Today the refresh loop sets a
   flag and stops. Next IPC call discovers it. Should instead re-run
   `/auth/exchange`, get fresh creds, restart the refresh loop, and
   reconnect.
5. **Pre-flight `RefreshNowIfDue` on every NATS-touching IPC call.**
   Already exists; verify all entry points use it (search for
   `d.ensureNATS` and adjacent code).
6. **SIGSEGV bug** (whatever Phase 0 reveals it to be).

### RED tests for Phase 1

All testable; the harness is the same `tests/reliability/` compose
extension established in Phase 0.

**`tests/reliability/recovers-after-long-outage/`** — pins the
`MaxReconnects(-1)` fix. RED: with default settings, killing NATS for
>120s permanently breaks the daemon; with the fix, it recovers.

```bash
ppz_a daemon login …
ppz_a source create chat
docker stop compose-ppz-server-1
sleep 150                         # well past the 60×2s default cap
docker start compose-ppz-server-1
wait_for 60 'ppz_a status | grep -q "^nats: connected"'

ppz_a broadcast -m "after long outage"   # should succeed
```

**`tests/reliability/survives-jwt-rotation-under-traffic/`** — pins the
JWT rotation correctness. RED variant uses a server-side test mode that
issues 30s-TTL JWTs (controlled by an env var like `PPZ_TEST_JWT_TTL=30`);
daemon must survive multiple rotations under continuous traffic.

```bash
PPZ_TEST_JWT_TTL=30 docker compose up -d ppz-server   # short-TTL server mode
ppz_a daemon login …
ppz_a source create chat

# Send messages every 5s for 90s — covers ≥3 rotation cycles
for i in $(seq 1 18); do
  ppz_a broadcast -m "msg-$i"
  sleep 5
done

# Diag log shows rotations completing without disconnect/reconnect cycles
# (in-place rotation works) OR with clean reconnects (forced rotation works)
ppz_a diagnostics | awk '/^nats: (disconnected|reconnected|rotated)/ { c++ } END { print c }'
# Either count 0 (in-place) or even number (each disconnect paired with reconnect)
```

This requires a server-side test mode for short-TTL JWTs. Worth the
plumbing — without it, the test takes >1h per run.

**`tests/reliability/onunauth-triggers-reauth/`** — pins the auto-re-auth
fix.

```bash
ppz_a daemon login …
ppz_a source create chat

# Server-side: revoke and re-issue the api key (test-only endpoint, or
# direct DB manipulation in the test harness — same pattern as the
# existing seed scripts).
revoke_and_reissue_alpha_key

# Daemon's next call surfaces ENotLoggedIn? Or transparently
# re-authenticates? The fix says transparent.
ppz_a broadcast -m "after key rotation"   # should succeed
ppz_a diagnostics | grep -q "^nats: re-authenticated"
```

This requires a way to revoke + reissue server-side from the test
harness. May warrant exposing a `ppz-seed reissue` helper or just
running the SQL directly.

**`tests/reliability/reconnect-storm-tamed/`** — pins the
`ReconnectJitter` fix. RED with vanilla settings: under sustained NATS
unavailability, reconnect attempts hit a synchronous backoff that tight-
loops; with jitter, attempts spread out.

```bash
docker stop compose-ppz-server-1
sleep 30                          # daemon trying to reconnect this whole time

# Inspect daemon's reconnect-attempt timestamps from the diagnostics log.
# With jitter (0.5–2s), inter-attempt gaps should vary; without, they're
# uniform at 2s. Cheap statistical assertion: variance > threshold.
ppz_a diagnostics | awk '/nats: reconnect attempt/ { print $1 }' | …
```

Statistical assertions are unusual for ppz e2e; if this test is too
flaky, pin a simpler property (no >5 attempts in any 1s window, etc.)
and rely on unit tests for the jitter calculation itself.

**`tests/reliability/sigsegv-regression-<id>/`** — once we know what the
SIGSEGV is, a directed regression test that exercises the specific code
path that crashed.

### Acceptance criteria

- Each fix has a corresponding RED test that's red against `main` and
  green after the fix.
- Existing e2e (192/192 today) remains green — no regression.
- A 70-minute soak (past 1h JWT TTL) under continuous traffic on the WAN
  stack completes with zero `E_NATS_UNREACHABLE` user-visible failures.
- `ppz diagnostics` shows clean disconnect → reconnect → re-authenticated event
  trails for each induced fault.

### Out of scope for Phase 1

- Source / pipe state survival across daemon restarts (Phase 2).
- New error codes (e.g. splitting `E_NATS_AUTH_EXPIRED` from
  `E_NATS_UNREACHABLE`). Decide post-Phase 0 once we see the actual
  cause distribution.

---

## Phase 2 — Daemon state survival

**Goal:** restarting the daemon (or recovering from full re-auth) doesn't
break in-flight agent coordination.

### Specific scenarios from MoltHub

- After `daemon stop && daemon start`: per-session `current` map is wiped
  from memory. Any session that had `current=alpha` now sees `current=-`.
  Agents have to re-set.
- After full re-auth (Claude's case): the `KnowsPipe` cache is reset; the
  source list refreshes lazily on the next call; brief window where
  agents see "source not found" for sources they know exist.
- Sources / pipes the daemon "knew" about are forgotten on restart and
  re-discovered lazily.

### Design questions to answer in Phase 2 scoping

- Should the per-session `current` map persist to disk? (We already
  persist `current.json`; is there a session-scoping bug we missed?)
- How aggressively should the daemon prefetch source/pipe state on
  (re)connect? (Pre-fetch on connect = no lazy "source not found"
  surprises; cost = startup latency.)
- Is there a clean "graceful re-auth" path that doesn't lose in-flight
  state? (Refresh creds, re-establish NATS connection, but keep state
  caches warm — possible if we separate auth state from capability state.)

Phase 2 is its own scoping pass after Phase 1 lands. **Don't bundle into
Phase 1.** RED tests for Phase 2 will follow the same pattern: induce
the scenario via compose, assert state survives.

---

## Track B — Error message accuracy (parallel quick wins)

These don't depend on Phase 0 / 1 and ship immediately. **One PR, ~half a
day.**

### Three concrete fixes

1. **`E_INVALID_PIPE`** message (`internal/cliproto/errors.go:94`):

   Today: *"invalid pipe; target must be `<handle>.<pipe>` with pipe ∈
   `{broadcast, inbox, stdin, stdout}`"*

   Wrong — custom pipes (`ppz pipe create … chat`) are valid. Charlie and
   Bob both hit the misleading text and concluded custom pipes weren't
   supported. Rewrite to drop the false enumerated set; describe format
   constraints; mention `ppz pipe create`.

2. **`E_NATS_UNREACHABLE`** message (`internal/cliproto/errors.go:92`):

   Today: *"nats unreachable; if running ppz daemon outside docker, set
   `PPZ_NATS_URL=nats://localhost:4222` before 'ppz daemon start'"*

   Wrong — the most common MoltHub cause was expired credentials, not URL
   misconfig. Rewrite to mention both. Once Phase 0 instrumentation lands
   we may split into a distinct `E_NATS_AUTH_EXPIRED` code; until then,
   the message just needs to point at both possibilities.

3. **`ppz source destroy` output**: Claude reported contradictory
   consecutive lines (*"0 sources destroyed"* then *"Destroyed alice"*).
   Likely a printf-ordering bug. Find the call site, fix the order.

### Test changes for Track B

Trivial. Update the two e2e fixtures that pin the literal `E_INVALID_PIPE`
and `E_NATS_UNREACHABLE` strings (search: `grep -rn 'E_NATS_UNREACHABLE\|E_INVALID_PIPE' tests/`).
Add one fixture for the destroy-output ordering bug.

---

## How to use this document

If you're starting a new PR in this area:

- **Phase 0 / 1 PRs**: cite this doc; describe which hypothesis you're
  verifying or which observation you're acting on; include the RED test
  your PR turns green.
- **Track B PRs**: cite the specific MoltHub agent and quote the bullet
  you're addressing.
- **Phase 3+ work** (the out-of-scope feature list): don't open until
  Phase 1 is in. The point of this plan is to fix substrate before
  building on it.

If you're updating this document:

- Mark phases ✅ shipped when their PRs merge.
- Move discoveries into "what we know" as they're verified.
- Prune stale hypotheses.
- The Phase 1 fix list will likely change as Phase 0 data comes in —
  rewrite it, don't paper over the change.
