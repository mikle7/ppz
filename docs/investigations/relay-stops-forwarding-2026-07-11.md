# Investigation: terminal-share relay forwards once, then silently stops

Tracking issue: mikle7/ppz#1
Branch: `investigate/relay-stops-forwarding`
Agent: diagnose (sonnet)
Status: OPEN, unresolved as of 2026-07-12. `reportNATSFailure` gap is a
real, fixed, regression-tested bug (see "Fix applied") but confirmed
NOT to be the trigger for either night's incidents. Recurred fresh on
4 more sessions (echo/herald/chorus/relay) on 2026-07-12 despite that
fix plus two more (`5ebf59c`, `463a7c8`) already being on `main`. Three
more candidate mechanisms tested and ruled out empirically in "Update 3"
— read that section first; it has the current best lead (an untested,
unconfirmed error-path gap) and the recommended next step
(instrumentation, not another blind fix).

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

## Update 2 (2026-07-11, ~17:10): git archaeology + subsSnapshot lead ruled out

Per Michael's question (relayed by greg): is either bug pre-existing
upstream, or introduced by this fork's own work (terminal attach,
heartbeat-project, muster integration, etc.)? Answer, via git
archaeology (both remotes already configured — `origin`=fork,
`upstream`=pipescloud/ppz):

- `reportNATSFailure`: `git blame upstream/main` attributes it to James
  Miles (the original author) in `e986007d` (2026-06-09) and
  `48bf9253` (2026-06-16) — both predate this fork's divergence point
  (`git merge-base origin/main upstream/main` = `bc23e16`).
  `git log bc23e16..origin/main -- internal/daemon/daemon.go` shows the
  *only* fork-side commit touching that file since divergence is this
  investigation's own fix. **Pre-existing upstream bug, not
  fork-introduced.**
- `internal/daemon/watch_registry.go`, `handlers_subs.go`,
  `list_watch.go`: byte-for-byte identical between `origin/main` and
  `upstream/main` (`diff` exit 0 on all three). Untouched by any fork
  feature work. Whatever the swap-cycle mechanism turns out to be, it's
  pre-existing too.

Then chased the `subsSnapshot` lead one step further: wrote a stress
test (`internal/daemon/watch_registry_swap_race_test.go`,
`TestPublishDuringSwapWindowStillWakesWatch`) that fires a publish on an
*independent* connection concurrently with each of 40 `swapNC` calls in
a tight loop, checking whether the armed watch entry (the exact
mechanism behind `subs wait` / the nudge pump) ever misses one.
**Result: 0/40 misses.** Combined with the already-solid
`rearmAll`/`swapNCLocked` dual-subscribe-then-flush-then-close-old
design (extensive existing comments reference a prior "fix plan §Race
analysis" + PR #115) and its passing test suite, this rules out
`watch_registry`'s rearm as the mechanism behind echo's precisely
timestamp-correlated lost messages.

Remaining open question: since the actual wakeup-delivery mechanism
tests airtight under direct adversarial racing, either (a) the real
bug is somewhere I haven't checked yet (the unsynchronized `d.NC` reads
in `buildFilteredList`/most other handlers are a real, confirmable
latent data race — `go test -race` doesn't currently catch it, so
inconclusive — but this pattern is pervasive across nearly every
handler in the package, not unique to this path, so it's a weak signal
on its own), or (b) the swap-window timestamp correlation the team
found tonight is coincidental rather than causal — plausible given how
frequently swaps recur (~4m30s), a decent fraction of any busy
window's message timestamps will land near *some* swap boundary by
chance. Not resolved either way; flagging both possibilities for
whoever continues.

**Since this doc was written**: chud found and fixed the two concrete
bugs this thread was chasing — a transient `subs read` race
(`buildFilteredList` reading `d.NC` unlocked; PR mikle7/ppz#2,
`fix/reqreply-swap-race`) and a duplicate/replay bug for long-lived
`forwardStdin` wrappers (SinceMS not degrading with process age; PR
mikle7/ppz#3, `fix/stdin-relay-replay`). **Both PRs are still OPEN,
unmerged, as of this update** (confirmed via `gh pr list`) — held
pending Michael per the release-process finding below. Neither fixes
the symptom this doc opened with (`forwardStdin` under-*delivering*
after message #1); #2 is a different subsystem (`subs wait`) and #3
fixes over-delivery (duplicates), not under-delivery. Separately: ppz's
auto-upgrade only ships tagged releases, and none of that night's
fixes (including the `reportNATSFailure` fix in *this* doc) were ever
released — so no daemon restart that night could have picked any of
this up regardless of root cause. Audit for "is a box actually
patched" should check for a git-describe `vX.Y.Z-N-g<sha>` shape (N>0
commits past the tag), not just `ppz status` self-reporting "latest".

## Update 3 (2026-07-12): symptom recurs on 4 fresh sessions; three more
hypotheses tested and ruled out empirically

Reopened by fresh, independent reports from echo/herald/chorus/relay —
same signature as the original symptom (`forwardStdin` relays message
#1 into the wrapped pty correctly, then goes permanently silent; no
error either side; a subsequent send still reports success) — despite
`5ebf59c` (backlog-replay-on-fresh-process fix), `463a7c8` (SinceMS
filter on the live-follow path), `a134977` (`reportNATSFailure`
Follows eviction), and `764557a` (swap-race stress coverage) all
already being on `main`. This confirms the "Ruled out" list above is
real but incomplete — the actual trigger for repeated real-world
recurrence is still unidentified.

Rather than re-derive the same static-analysis leads, this round built
three more targeted reproductions against the real embedded-NATS
daemon harness (`startEmbeddedJS`, same pattern as
`read_live_since_ms_test.go`), each driving the exact wire protocol
`forwardStdin`/`streamForwardStdinOnce` use (`IPCRead{Follow:true,
NoAdvance:true, SinceMS:<elapsed-since-process-start>+1}`, redial with
a *grown* SinceMS anchored to the same start time — never reset per
reconnect, matching the real code). All three **passed** — genuine
negative results, not gaps in coverage:

1. **Idle past the pull consumer's Expires window.** Hypothesis: after
   1 message out of nats.go's default 500-message batch, `checkPending`
   only re-arms a new pull once `pending.msgCount` drops below
   `ThresholdMessages` (250) — nowhere near after just 1 delivery — so
   if the batch's 30s `Expires` lapses with true silence in between (the
   normal shape of an interactive ping, one message then a human/agent-
   paced gap), delivery could stall until *something else* forces a
   reset. Real wall-clock test: publish message #1, confirm delivery,
   wait 35s of genuine silence (past both the 10s `ErrNoHeartbeat`
   threshold and the 30s `Expires`), publish message #2. **Message #2
   arrived cleanly** — the ordered consumer's heartbeat-loss self-heal
   (`ErrNoHeartbeat` is one of the 5 sentinel errors `orderedConsumer`'s
   `errHandler` resets on) genuinely works end-to-end in this exact
   configuration. Rules out plain idle-timeout mishandling as a
   single-cycle cause.
2. **Single routine-swap eviction + client redial.** The team's own
   `ppz diagnostics` corroboration (see "Update" above) already showed
   every `closed` event that night was a routine JWT-refresh
   `swapNCLocked`, which *does* call `Follows.closeAll()` (unlike the
   separately-fixed `reportNATSFailure` gap) — so this forcibly closes
   `forwardStdin`'s live Follow roughly every 4m30s as a matter of
   normal operation, not a bug. Untested until now: does the *client*
   side actually recover from that closure and keep receiving? Test:
   deliver message #1, force a real `swapNC` (fresh `*nats.Conn`,
   `Follows.closeAll()` fires), confirm the pre-swap conn observes
   EOF, redial exactly like `forwardStdin`'s outer loop (fresh conn,
   fresh Follow request, SinceMS grown from the same anchor), publish
   message #2. **Delivered cleanly** on the first redial.
3. **8 repeated swap+redial cycles against the same long-lived
   stream/session** (reconnecting to the *same* embedded server each
   time — the real shape of a JWT refresh, not a different backend),
   to check for accumulation/drift a single-cycle test can't see
   (consumer-name collisions across many resets, growing-SinceMS
   interaction with a real growing sequence number, fd/goroutine
   exhaustion). All 8 cycles delivered their expected message,
   including the by-design retained-backlog replay each redial's
   historical drain re-surfaces (confirming `forwardStdin`'s real
   `seenIDRing` dedup — not replicated by a naive version of this test,
   which initially mis-fired on cycle 2 by grabbing the replayed
   message #1 instead of draining to the new one — is genuinely
   load-bearing for correctness under this protocol's by-design
   NoAdvance-redelivers-everything contract, worth remembering if
   anyone re-derives this test).

**Net effect on the hypothesis space**: every mechanism that goes
through the OrderedConsumer's *documented* recovery paths (heartbeat
loss, connection-closed-then-redial, repeated eviction) has now been
directly tested and confirmed working. What remains untested (couldn't
be forced through the public `jetstream` API in a reasonable test):
any status/error the underlying pull consumer receives that *isn't*
one of the 5 sentinel types `orderedConsumer.errHandler`
(`nats.go@v1.51.0/jetstream/ordered.go:209`) recognizes as reset-worthy
(`ErrNoHeartbeat`, `errOrderedSequenceMismatch`, `ErrConsumerDeleted`,
`errConnected`, `nats.ErrNoResponders`). Concretely, `pull.go`'s
`handleStatusMsg` treats a handful of other server responses as
*terminal* — e.g. a malformed `Nats-Pending-Messages`/`Nats-Pending-Bytes`
header on an otherwise-ordinary 408 timeout (`message.go:468/476`), or
a `409` conflict whose `Description` doesn't match any of the five
hardcoded substrings `checkMsg` looks for (`message.go:427-458`, falls
through to a generic `fmt.Errorf`) — which call the underlying
`pullSubscription.Stop()` with **no reset**. Combined with
`internal/daemon/read.go:384`'s `consumer.Consume(handler)` call
passing **no `jetstream.ConsumeErrHandler`**, any such error is
currently invisible on both sides: nothing logs it, nothing resets,
the IPC socket stays open (so the CLI never redials), and a later send
still succeeds (publish is decoupled from any one reader's broken
pull). This is a real, confirmed-by-code-reading gap in error-path
*coverage and observability* — but unlike the three mechanisms above,
it could not be forced to fire against the embedded test server (its
error responses are well-formed), so it remains an unconfirmed
hypothesis, not a demonstrated root cause. A public nats-server issue
(`nats-io/nats-server#5839`, still open) describes the same class
of symptom independent of this codebase ("consumer status shows active
but not pulling, requires client restart") — consistent with, but not
proof of, this being the mechanism here.

**Recommendation — instrumentation before another fix attempt.**
Per the standing rule from earlier tonight (two half-understood fixes
collided once already), do not patch this blind. The single highest-
value next step is to add a temporary `jetstream.ConsumeErrHandler`
to `internal/daemon/read.go`'s live-follow `consumer.Consume(...)` call
that just logs (via `d.Diag`/whatever `nats_events.go` already uses) —
not resets, not changes behaviour, purely observability — and get it
onto one daemon that's actually likely to reproduce this, then wait for
the next natural recurrence. That single log line will say definitively
whether an unrecognized status/error precedes the silence (confirming
this hypothesis and pointing at the exact fix: recognize more error
types, or stop trusting the ordered consumer's internal self-heal
entirely and add an application-level idle-watchdog redial in
`streamForwardStdinOnce` instead — the CLI side currently has *zero*
idle-based liveness check; it relies entirely on the connection
erroring, which this bug class may never do) — or rules it out too, in
which case the search moves to a dimension not considered yet.

## Artifacts

- `internal/daemon/report_nats_failure_follows_test.go` — permanent
  regression test for the fix in this doc.

The two throwaway scratch tests used while building the hypothesis
(`diagnose_reset_scratch_test.go`, `diagnose_ncclose_scratch_test.go`)
were deleted once the permanent test above superseded them; their
findings are summarized inline above ("Ruled out" / root-cause steps).

Three more throwaway scratch tests from Update 3
(`diagnose_idle_expiry_scratch_test.go`,
`diagnose_swap_redial_scratch_test.go`,
`diagnose_repeated_swap_scratch_test.go`) were deleted after their
(all-negative) findings were folded into Update 3 above — no fix
resulted from this round, so there is nothing for a permanent
regression test to pin. Re-derive from the descriptions in Update 3 if
needed; all three used the `read_live_since_ms_test.go`/
`startEmbeddedJS` pattern already established in this package.

## Update 4 (2026-07-12): diagnostic-only instrumentation shipped, per
greg's go-ahead

Greg approved converting the unconfirmed lead from Update 3 into
observability, explicitly scoped as log-only/no-behavior-change: "Go
ahead and add the diagnostic-only ConsumeErrHandler ... that's a
sound, low-risk way to convert an unconfirmed hypothesis into a
confirmed one."

Shipped: `internal/daemon/read.go`'s live-follow `consumer.Consume(...)`
call now also passes `jetstream.ConsumeErrHandler(...)`, which records a
`warn`/`handleRead-liveFollowConsume` `NATSEvent` (existing ring +
on-disk sink, `d.recordNATSEvent` — the same mechanism `ppz
diagnostics` already reads) with nats.go's raw error string in
`Reason`. It does not reset, retry, close, or otherwise touch delivery
— purely observational. Documented as a new `warn` source in
`docs/diagnostics.md` §4/§5, per that file's own extend-with-care
contract.

Permanent regression test:
`internal/daemon/read_live_follow_consume_err_test.go`
(`TestHandleRead_Follow_ConsumeErrIsRecordedAndSelfHealStillWorks`) —
forces a real `ErrConsumerDeleted` (deletes the live OrderedConsumer's
underlying ephemeral consumer server-side, same technique as the
original "Ruled out: generic consumer recycling" test) and asserts
both halves of the contract: message #2 still arrives (self-heal
unaffected) AND the new warn event was recorded with a non-empty
reason. Full `internal/daemon` suite green, `go vet` clean, `gofmt`
clean.

**Next step, not yet done**: this needs to actually be running on a
daemon likely to hit the real recurrence (echo/herald/chorus/relay's
sessions, or whichever is next to reproduce) — a deploy decision, not
just a code change, left to greg per his message. Once it's live and
this fires for real, `ppz diagnostics` will show whether the `Reason`
string names one of the "doesn't self-heal" shapes hypothesized in
Update 3 (a malformed pending-count header, an unrecognized 409
description) or something not considered yet. Report back here and to
the team either way — this doc stays open until a confirmed root cause
lands a real fix.
