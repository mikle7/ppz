# Spec: Robust session binding (Layer 1) + fail-closed send (Layer 2)

## Problem

Subprocesses of an agent's pty inherit none of its session state. `sessionID()` (`internal/cli/session.go:30`) keys per-shell state on `getsid(0)`, and subprocess managers (Claude Code's Bash tool, Monitor recipes, sub-agents) call `setsid` — each subprocess gets a fresh session id the daemon has never seen. Symptoms in prod:

1. **Lost current handle.** Subprocess `ppz status` / `read inbox` → `E_NO_CURRENT_SOURCE`.
2. **Anonymous sends.** Subprocess `ppz send david hello` → daemon stamps empty sender, david receives an untraceable message.

The current workaround (`internal/cli/agent.go:172-196`) hard-codes `PPZ_SESSION=<handle>` inline in the Monitor recipe — fragile, easy to drop, needs to be remembered by every recipe author.

## Empirical diagnosis (2026-05-20)

Probed Claude Code's subprocess spawn behavior directly before designing the fix:

| Observation | Implication |
|---|---|
| Consecutive Bash calls have **different `getsid` values** (e.g. 81786 → 81803). | Confirmed root cause. Each Bash spawn is its own session leader. |
| Subprocess `tty=??` even when the grandparent (`claude`) has `ttys007`. | TTY binding wouldn't work — there's no tty on the subprocess. |
| PPID chain is intact end-to-end across every spawn mode tested (`&`, `nohup`, sub-agent, sub-shell). | Process tree is the reliable signal. |
| Sub-agents (Agent tool) share the parent claude's process tree. | Sub-agent identity inherits automatically — no extra wiring needed. |
| `PPZ_SESSION` env propagates through `&`, `nohup`, sub-agents. | The existing env pin still works as an escape hatch. |

The `tty=??` observation is specific to subprocess spawners (Claude Code's Bash tool, and any shell that calls `setsid` on children) — not universal. A `bash -c …` launched directly from an interactive terminal preserves the parent's controlling tty. We use ppid because it covers both cases; tty would have covered neither reliably.

The original draft of this spec used tty binding as the primary mechanism. The probe data inverts that: **ppid walk is primary; tty plays no role.**

## Goals

- **G1.** Agent subprocesses resolve to their parent agent's session — no env propagation required.
- **G2.** `ppz send` with no resolvable sender returns `E_NO_CURRENT_SOURCE`. No anonymous publication.
- **G3.** All session-keyed state (current handle, cursors, namespace, future `subs`) unifies for in-pty agent contexts.
- **G4.** Daemon crash / upgrade / restart preserves bindings.
- **G5.** Wire change is additive — old CLI ↔ new daemon and new CLI ↔ old daemon both work.

## Non-goals

- Cross-machine session unification.
- SO_PEERCRED-style multi-tenant hardening. **Trust boundary: the daemon trusts CLI-supplied `ancestor_pids`.** A local user could spoof another agent's identity, but they can already read and edit `$PPZ_HOME/current.json` directly — the boundary is unchanged.
- Replacing `PPZ_SESSION` env override — kept as the highest-precedence signal and escape hatch for double-fork-detached subprocesses.
- Per-handle subscriptions (covered separately).

## Design

### Layer 1 — agent binding

```go
type AgentBinding struct {
    Handle       string
    SharePID     int       // pid of `ppz terminal share <Handle>`
    SessionKey   string    // "agent:<Handle>"
    RegisteredAt time.Time
}
```

- `ppz terminal share <H>` calls `IPCRegisterAgentBinding{Handle, SharePID=os.Getpid()}`. Idempotent for same `(SharePID, Handle)`; same SharePID + different handle returns `E_BINDING_CONFLICT`.
- Clean teardown calls `IPCUnregisterAgentBinding`. Abnormal exits handled by lazy validation on lookup.
- No tty index. No heartbeat IPC. No janitor goroutine.

### CLI ancestor chain

Each session-using IPC request gains an optional `ancestor_pids []int` field. CLI populates it by walking its own ppid chain client-side: `syscall.Getppid()` for depth 1, then `sysctl(KERN_PROC_PID)` on darwin or `/proc/<pid>/status` on linux, capped at `MaxPPIDWalkDepth = 8` or PID 1 (init).

### Resolver

```
1. ancestor_pids ∩ bindings ≠ ∅     → (binding.SessionKey, binding.Handle)
2. declaredSession != ""             → ("declaredSession", "")    // explicit env / legacy CLI
3. fallback                          → ("default", "")
```

**Precedence inversion (revised from original draft after PR #75 review):**
binding match wins over declared session. The CLI always sends
`Session: sessionID()` (legacy back-compat — old daemons still get a
usable session key) AND `AncestorPIDs`. If we let declared win, the
new daemon would never engage the resolver. By putting the binding
first, in-pty subprocesses resolve to their agent regardless of what
their inherited session id happens to be.

Cost: explicit `PPZ_SESSION=foo ppz …` from inside an agent's pty is
ignored — the binding wins. To operate under a synthetic session,
run from outside the binding's process tree. Acceptable tradeoff
given the goal is "agent's identity follows from being in their pty."

### Auto-write current

When the resolver returns `(sessionKey="agent:cindy", boundHandle="cindy")` and `State.Current("agent:cindy") == ""`, the daemon writes `current["agent:cindy"] = "cindy"`. Idempotent. Explicit `ppz set handle bob` later overrides; `ppz unset handle` clears, and the next IPC re-fires the auto-write — agents can't lock themselves out.

### Back-compat seed for legacy env pins

`RegisterAgentBinding` also writes `current["<handle>"] = "<handle>"` (gated on the key being currently empty) when a binding is created. This makes the long-standing `PPZ_SESSION=<handle> ppz …` recipe pattern continue to resolve — without it, the daemon would receive `declaredSession="<handle>"`, find nothing under that key, and fail closed on send. The seed is small, idempotent, and self-cleaning when the source is destroyed (via `ClearCurrentForHandle`). Visible side effect: `current.json` will carry one extra entry per agent handle.

### Layer 2 — fail-closed send

In `resolveSendTarget` (`internal/daemon/handlers.go`), if the resolved sender is empty, return `E_NO_CURRENT_SOURCE` and don't publish. Applies to both `handleSend` and `handleSendBatch`. No `--anonymous` opt-in.

The existing fixture `tests/send/send-uncollared-stamps-empty-without-handle/` encodes today's broken behavior and gets updated to expect the new error.

### Per-call sender override: `--from`

Persistent identity via `ppz set handle <H>` is the right primitive for humans and long-lived shells. Scripts and one-off invocations want a per-call override — analogous to `git commit --author "..."`. `ppz send` and `ppz command` gain `--from <handle>`:

- `ppz send --from cindy david "msg"` → publishes to `david.inbox` with `sender=cindy`.
- `ppz command --from cindy bob "ls"` → forwards to `bob.stdin` with `sender=cindy`.

Semantics:
- `--from` is per-call. No state mutation. Subsequent `ppz` calls in the same session see whatever current handle is set.
- The daemon validates `--from` value's shape via `natsubj.ValidateHandle` (regex check; the handle does not have to exist as a registered source).
- **No auth gate.** A local user can already publish to any handle via the IPC socket; `--from` doesn't widen the trust surface. Recipients should treat envelope.sender as informational at the same level they always have.
- `--from` overrides the auto-resolved sender from `State.Current`. If both an explicit `--from` and a current handle are set, `--from` wins for that call.
- Compatible with `--request-ack`: the ack auto-emits to `<--from-handle>.inbox`, which is correct — wherever you claim to be is where the ack goes.

After `--from` lands, the user-facing remediation set for `E_NO_CURRENT_SOURCE` collapses to two items: `ppz set handle <H>` (persistent) or `--from <H>` (per-call). The error message reflects this.

### IPC delta

| Method / Type | Change |
|---|---|
| `IPCRegisterAgentBinding` | New. `{Handle, SharePID}` → `{Handle, SharePID, SessionKey, RegisteredAt}`. |
| `IPCUnregisterAgentBinding` | New. `{SharePID}` → `{}`. |
| All session-using requests | Add `ancestor_pids []int` field, `omitempty`. |
| `E_BINDING_CONFLICT`, `E_BINDING_UNKNOWN` | New error codes. |

## Durability

Persistence file `$PPZ_HOME/agent-bindings.json`, versioned envelope, atomic tmp+rename (same pattern as `current.json`).

| Scenario | Recovery |
|---|---|
| Daemon crash/respawn/upgrade | Load + validate-on-load drops dead pids; survivors are immediately usable. |
| Share killed -9 | Lazy validation drops the binding on next lookup. |
| Persistence file corrupt or missing | Load empty. Recovery: when the share's next IPC (e.g. `terminal share` publishing wrapped-pty stdout to NATS) hits the daemon, the daemon detects the share's pid is in the caller chain but has no binding → returns `E_BINDING_UNKNOWN` → CLI re-registers. Window of brokenness is "until next IPC from the share"; idle ptys (silent harness, no output) extend the window until any output happens. Subprocesses making IPC calls during the window resolve via fallback (`E_NO_CURRENT_SOURCE` on send — fail-closed, not anonymous). |
| In-flight IPC mid-restart | `E_DAEMON_NOT_RUNNING` (existing behavior). |

## Migration

- `current.json` keyed by `sid-N`: stays. Auto-write populates new `agent:<H>` keys on first IPC; old keys are inert.
- Cursor / namespace state: same.
- Single-step CLI rollout. Old CLI hits precedence 1; new CLI talking to old daemon falls back via the optional field (old daemon ignores `ancestor_pids`).

## Tests

Tests are written first; impl pauses for review when all are failing.

| Group | IDs | File |
|---|---|---|
| Binding table CRUD | PB-1, 2, 5, 6, 9, 10, 11, 12 | `internal/daemon/agent_binding_test.go` |
| Persistence | PP-1, 2, 3, 4, 5, 6, 8, 9 | `internal/daemon/agent_binding_persist_test.go` |
| Resolver | RS-1, 2, A, B, C, 7, 8, 9, 11, 12, 13 | `internal/daemon/resolve_session_test.go` |
| Auto-write current | AC-1..6 | `internal/daemon/handlers_session_test.go` |
| Wire compat | BC-1, 2, 3 | same |
| CLI ancestor walker | AW-1..5 | `internal/cli/session_ancestors_test.go` |
| Wire shape | WP-1, 2, 3 | `internal/cliproto/types_session_binding_test.go` |
| Send fail-closed | SF-1..7 | `internal/daemon/send_failclosed_test.go` + e2e |
| Resolver perf | PF-A, PF-B | `internal/daemon/resolve_session_bench_test.go` |
| Terminal share e2e | TS-A..I | `tests/sessionbind/` |
| Recovery e2e | RC-A..E | `tests/sessionbind-recovery/` |
| State unification e2e | SI-1..6 | `tests/sessionbind-state/` |
| Anon-send repro | AS-1 (before/after) | `tests/anon-send-repro/` |

Goal coverage:

| Goal | Tests |
|---|---|
| G1 — subprocesses resolve correctly | TS-A, TS-B, TS-C, TS-D, TS-H, AS-1 |
| G2 — fail-closed send | SF-1, SF-2, SF-3, SF-5, AS-1 |
| G3 — state unification | AC-1..6, SI-1..6 |
| G4 — restart preserves bindings | RC-A, RC-B, PP-1, PP-6, PP-8 |
| G5 — wire compat | BC-1, BC-2, BC-3, WP-1, WP-2, WP-3 |

## Open questions

1. **`MaxPPIDWalkDepth = 8`.** Reasonable for typical chains. Bump if a pathological harness exceeds it.
2. **PID-reuse race.** Pid wraps in seconds on linux but takes longer on darwin. Worst case: one IPC mis-resolves before lazy validation catches it. Document; don't try to prevent.
3. **Linux PID namespaces.** Walk uses host PIDs; in a containerized client the walk terminates inside the namespace. Future work if it comes up.

## References

- `internal/cli/session.go:30` — current `sessionID()`
- `internal/cli/agent.go:172-196` — current Monitor recipe workaround
- `internal/cli/send.go:91-100` — `--request-ack` preflight pattern
- `internal/daemon/state.go:36-37` — file pattern (`current.json` / `namespace.json`)
- `internal/daemon/handlers.go:924` — `resolveSendTarget`
- `docs/WIRE.md` — IPC wire contract
