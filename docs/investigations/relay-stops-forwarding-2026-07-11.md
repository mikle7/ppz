# Investigation: terminal-share relay forwards once, then silently stops

Tracking issue: mikle7/ppz#1
Branch: `investigate/relay-stops-forwarding`
Agent: diagnose (sonnet)
Status: `reportNATSFailure` gap confirmed, fixed, and regression-tested
(see "Fix applied" below) — **but team-gathered diagnostics show this is
probably NOT what triggered tonight's specific incidents.** See "Update"
at the bottom before treating this as closed.

## Symptom (recap)

`ppz terminal share`'s `forwardStdin` correctly forwards the first
message published to `<handle>.stdin` into the wrapped child's PTY, then
stops forwarding entirely — no error, no crash, no redial. The
daemon-side send still reports success (`sent id=... bytes=...`). Data
is not lost (confirmed via `ppz reread` against retained storage) — this
is purely a live-delivery bug in the Follow path, not the write path.
Reproduced independently on 4+ agents across 2 repos/hosts tonight.

## Ruled out

- **Envelope ID collisions** driving `seenIDRing`'s dedup to
  incorrectly skip message #2 as "already seen": envelope IDs are
  `uuid.NewString()` (`internal/envelope/envelope.go:57`) — no
  collision path. `forwardStdin`'s dedup ring is not implicated.
- **The SinceMS/backlog-replay filtering fixed tonight**
  (`internal/daemon/read.go`'s `sinceCutoff` check, both on the
  historical-drain path and the live-Consume path) is working as
  intended and is unrelated to this symptom — confirmed by re-reading
  the fix and its pinned test
  (`read_live_since_ms_test.go`).
- **`jetstream.OrderedConsumer`'s own self-heal/reset logic being
  broken in general.** Directly tested (see
  `internal/daemon/diagnose_reset_scratch_test.go`): force-delete the
  live ephemeral consumer server-side between message #1 and message
  #2 (simulating what `InactiveThreshold`-driven idle eviction does) —
  the ordered consumer's `reset()` recreates cleanly and message #2
  still arrives. So a generic "consumer got recycled" event is NOT
  what's silently killing the relay.

## Root cause (confirmed via reproducing test)

`internal/daemon/daemon.go:343-356`, `reportNATSFailure()`:

```go
func (d *Daemon) reportNATSFailure() {
	d.ncMu.Lock()
	nc := d.NC
	d.ncMu.Unlock()
	if nc == nil || !nc.IsConnected() {
		return
	}
	nc.Close()
	d.kickReconnect()
}
```

This is called from `publishWithAck`/`publishBatchWithAck`
(`internal/daemon/publish.go`) and from `handleRead`
(`internal/daemon/read.go:140`) whenever a JetStream operation times
out on a nominally-connected NC (the documented "zombie connection"
case — TCP alive, JetStream tier not responding; very plausible after a
long idle gap on a real network daemon, exactly the "agent
blocked/waiting" scenario in every repro).

**`reportNATSFailure` closes `d.NC` directly. It does NOT call
`d.Follows.closeAll()`.** Every *other* path in this codebase that
replaces or kills the shared NATS connection — `swapNCLocked`
(`daemon.go:239`) — explicitly calls `d.Follows.closeAll()` first,
specifically so any live `Follow: true` IPC connection (e.g.
`forwardStdin`'s) gets its socket closed, the CLI observes EOF, and the
CLI's outer redial loop fires. `reportNATSFailure` is the one call site
that bypasses this.

Consequence, traced through `nats.go` v1.51.0's jetstream package
(`ordered.go`, `pull.go`):

1. `nc.Close()` transitions the shared NC to `nats.CLOSED`.
2. The live `OrderedConsumer`'s underlying pull subscription observes
   this (`pull.go` connStatusChanged watcher) and emits
   `ErrConnectionClosed`.
3. `orderedConsumer.errHandler` (`ordered.go:209`) special-cases this
   error: `if errors.Is(err, ErrConnectionClosed) { subscription.Stop(); return }`
   — **no reset, no self-heal.** (Every other trigger —
   `ErrNoHeartbeat`, `ErrConsumerDeleted`, `errConnected`,
   `nats.ErrNoResponders` — goes through `doReset` and recovers, which
   is why the plain consumer-eviction test above passes. `ErrConnectionClosed`
   specifically does not, by nats.go's own design, since the ordered
   consumer instance is tied to that one NC.)
4. The daemon never set a `ConsumeErrHandler`
   (`consumer.Consume(func(msg jetstream.Msg) {...})` — no opts), so
   nothing observes step 3 happening.
5. `handleRead`'s own "block until CLI closes the socket" goroutine
   (`read.go:437-448`) is waiting on `conn.Read()` — which nobody
   triggers, because nothing closed `conn`. The IPC socket to the CLI
   stays open, delivering nothing, forever (or until something else
   closes it).
6. The CLI's `streamForwardStdinOnce` is blocked in `dec.Scan()` on
   that same still-open socket — no EOF, no error, so `forwardStdin`'s
   outer redial loop never fires. Silence, exactly as reported.
7. Meanwhile a **new** send (`ppz command --claude`, or literally
   anything else) can go through `publishWithAck`/`handleSend`, which
   opens its own fresh `jetstream.New(d.NC)` against whatever `d.NC`
   is *now* — if the background `kickReconnect()` has already installed
   a fresh working connection via `swapNC`, that send succeeds cleanly.
   This is exactly the "daemon-side send reports success" observation —
   sends and the dead Follow are on different lifecycles.
8. The dead Follow is *not* permanently stuck: the next time
   `kickReconnect` → `ensureNATS` → `swapNC` completes, `swapNCLocked`
   finally calls `Follows.closeAll()`, which closes the orphaned
   socket (confirmed empirically — see the "recovery" leg of
   `diagnose_ncclose_scratch_test.go`, using a *raw* `conn.Read`, not
   `json.Decoder`, to observe the EOF; the decoder red-herrings a
   timeout as if nothing happened). Once that fires, the real CLI
   would see EOF and redial cleanly.

So the actual window of total silence is bounded by how long it takes
`kickConnect`'s backoff loop (`reconnectInitialBackoff=2s`, doubling,
capped at 15s) to land a working `ensureNATS`. In the embedded-NATS
test that's near-instant. In production, on a real network daemon, this
window could plausibly stretch much longer if the reconnect itself
degrades repeatedly (repeated zombie-timeouts on the *new* connection
too) — which would need confirming against the actual incident's NATS
event history, not simulation.

## Confirming this against the real incidents (next step, not yet done)

Both `arthur`'s and this team's daemons keep a NATS event ring + on-disk
history exactly for this (`docs/diagnostics.md`, `internal/daemon/nats_events.go`).
Before landing any fix, whoever picks this up should run, on an affected
daemon (or from a saved `--bundle`):

```sh
ppz diagnostics --since=2h
```

and check for a `closed` / `reconnect` event (`caller=` will show
`publishWithAck`/`handleRead`-driven `reportNATSFailure` indirectly —
look for a "swap"/"reconnect" pair with no matching `Follows.closeAll`-driven
CLI redial in the same window) landing around the stall's start. If
present, this confirms `reportNATSFailure`'s missing `Follows.closeAll()`
as the actual trigger for the specific incidents tonight, not just a
theoretically-possible gap.

## Suggested fix direction (NOT applied — confirm against real incident
logs first, and only one of us should touch this given tonight's earlier
collision)

`reportNATSFailure` should not bypass the eviction contract every other
NC-closing path honors. Either:

- have it call `d.Follows.closeAll()` itself before/alongside `nc.Close()`, or
- route it through `swapNCLocked(caller, nil)`-style teardown so the
  "close now, reconnect in background" shape stays in one place.

Either way, the fix should keep the "sends still work once a new NC is
up" property intact — the bug is specifically the *absence* of a
proactive Follow eviction, not the reconnect logic itself, which is
otherwise sound (retested and confirmed self-healing for every other
trigger).

## Fix applied

`internal/daemon/daemon.go`, `reportNATSFailure()`: added the missing
`d.Follows.closeAll()` call (matching `swapNCLocked`), so a "zombie
connection" JetStream-timeout close now proactively evicts any live
Follow instead of leaving it silently orphaned. Regression test:
`internal/daemon/report_nats_failure_follows_test.go`
(`TestReportNATSFailure_EvictsLiveFollows`) — confirmed RED against the
pre-fix code, GREEN after. Full `internal/daemon` suite passes with no
regressions (the two throwaway `diagnose_*_scratch_test.go` files used
to build the hypothesis were removed once this permanent test replaced
them). `internal/cli` has 3 pre-existing, unrelated failures caused by
this *investigation session's own* `PPZ_SESSION` env var leaking into
those tests (confirmed via `git stash` against unmodified code —
identical failures either way).

## Update (2026-07-11, ~17:00): this fix is real but likely isn't tonight's trigger

Per greg's ask, herald/relay/echo/chorus each ran
`ppz diagnostics --since=2h` on their own daemons. Findings, independently
corroborating:

- **Every** `closed` event across all four daemons is attributed to
  `caller=OnRefreshed-callback` — i.e. the *routine* JWT-refresh
  `swapNCLocked` path, which already calls `Follows.closeAll()`. **None**
  show a `reportNATSFailure`-driven close. So the bug this doc fixes is
  real (and worth keeping — it's a genuine gap for whenever a zombie
  connection *does* get reported) but doesn't explain tonight's specific
  repros.
- The refresh/swap cadence measured on every daemon tonight is a
  near-metronomic **~4m30s (270s)**. That's roughly **half** the ~10min
  cadence `handleSubsWait`'s own comment
  (`internal/daemon/handlers_subs.go:175`) assumes ("a JWT-refresh swap
  (~10min cadence in production)") — worth checking why the JWT `exp`
  in use tonight is so much shorter than that comment expects.
- echo directly correlated their 3 messages to greg that never surfaced
  (16:19:07, 16:20:42, 16:22:01) against the swap/disconnect/closed
  window on **greg's** daemon (16:18:40–16:23:10) — all three fall
  inside it; sends outside any such window surfaced fine. Strong signal
  the bug is specifically about something during the swap window itself,
  not a JetStream-timeout/zombie-connection scenario.
- I read `watch_registry.go`'s `rearmAll` (used by `subs wait`, which is
  what actually backs the "nudge" mechanism) expecting to find the gap
  there, since it's the most direct candidate — but it's already
  carefully hardened for exactly this (dual-subscribe on old+new NC
  before tearing down old, per its own extensive comments referencing a
  prior "fix plan §Race analysis" and PR #115), and its tests
  (`TestWatchRegistry_RearmAll_ReplacesSubAndDeliversOnNewNC` etc.) pass.
  So the wakeup-firing mechanism itself looks sound.
- Best remaining lead, not yet investigated: `subsSnapshot` (called
  immediately after the `subs wait` wakeup fires, to build the actual
  reply) opens its own JetStream query against `d.NC` at call time — if
  that happens to land in the same narrow window where `d.NC` is
  mid-swap, a transient read failure there, if treated as "no unread"
  rather than surfaced/retried, would produce exactly echo's observed
  pattern (wakeup can fire correctly, but the snapshot built right after
  under-reports). Not yet confirmed — next person picking this up should
  start there rather than back in `read.go`/`ordered.go`.

## Artifacts

- `internal/daemon/report_nats_failure_follows_test.go` — permanent
  regression test for the fix in this doc.

The two throwaway scratch tests used while building the hypothesis
(`diagnose_reset_scratch_test.go`, `diagnose_ncclose_scratch_test.go`)
were deleted once the permanent test above superseded them; their
findings are summarized inline above ("Ruled out" / root-cause steps).
