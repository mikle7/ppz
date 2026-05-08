# Spec: `--request-ack` end-to-end (v0.23.0 → v0.25.0)

## Context

Survey of six agents in the Salt Town multi-agent run (2026-05-07) reported coordination friction around fire-and-forget sends, missing message metadata in default output, and inability to tell who sent each message. Five of six agents asked for some form of "delivery confirmation"; review of the raw responses showed they were asking for at least three different things — two had simply missed the existing `sent id=…` line, three wanted true read receipts, and three also wanted broadcast subscriber feedback.

This spec covers the read-receipt cut. It introduces a non-blocking ack mechanism (`--request-ack` on send → daemon auto-emits an ack message back to sender's inbox when the recipient's `ppz read` advances the cursor), reworks the message envelope to carry sender identity and reply correlation, and updates default `ppz read` output for inbox-shaped pipes to surface time/sender/subject without forcing `--json`.

Originally proposed as a single v0.23.0 breaking wire change; in practice it has shipped as two minor bumps (v0.23.0, v0.24.0) for safer bisection, with a third (v0.25.0) still ahead.

Out of scope (deferred to follow-ons): broadcast `--request-ack` (one ack per consumer), explicit post-processing acks (`ack:processed` / `ppz ack <id>`), `--from` / `--subject` / `--no-acks` read filters, native gate primitives, conditional triggers, priority/DLQ, shared audit log.

---

## Status (as of 2026-05-07)

**Shipped — v0.23.0 (PR #30, `e6a8cd6`)**
- Envelope: `Handle` dropped, `Sender` added (`internal/envelope/envelope.go`)
- Daemon: sender / destination split in `handleBroadcast` — sender pulled from `d.State.Current(req.Session)`, destination still drives the NATS subject
- Server: `broadcast_subscriber.go` re-sources destination from `m.Subject` parts since the envelope no longer carries it
- WIRE.md §3 documents pre-v0.23 compat (Go drops unknown `handle`, missing fields zero-value cleanly)

**Shipped — v0.24.0 (PR #31, `3f46276`)**
- Envelope: `Subject` field added (free-form; `ack:` prefix reserved)
- IPC: `BroadcastRequest.MsgSubject` plumbed through (no CLI surface yet — laid for daemon-internal callers including ack emission)
- Read formatter: new `internal/cliproto/read_format.go` with `FormatReadMessage`, `IsTabularReadPipe`, `bodyForRow`, `lastHexOfID`
- Read CLI: `--bare` flag added on `read` and `reread`, mutex with `--json`/`--tty`/`--raw`
- Tabular default gated to `inbox` / `broadcast` pipes (pty pipes stay byte-faithful)
- Tests: 6 unit-test cases in `read_format_test.go`; new e2e fixture `tests/read/read-tabular-default-on-broadcast/`; 13 legacy fixtures opted into `--bare` rather than relying on auto-detection; `tests/lib/normalize.sh` got a BOL-anchored `HH:MM:SS` rule

**Outstanding — target v0.25.0**
- Envelope: `InReplyTo`, `AckRequested` fields (§1, deltas below)
- CLI flags: `--request-ack`, `--subject`, `--in-reply-to` on `ppz send` (§3)
- Send success output: move to stderr + add `ack=requested` token (§3) — addresses the actual Salt Town root cause for Miner-Test/Prospector ("missed the `sent id=…` line because stdout was redirected")
- Daemon: ack auto-emit on cursor advance in `read.go` (§4)
- WIRE.md §3 update for the new envelope fields; manifest bump

**Wire-shape decisions, confirmed in implementation**
- `sender` and `subject` are always-serialized (not `omitempty`) — predictable wire shape over marginal byte savings. See `internal/envelope/envelope.go:27-28`. The two new pending fields (`in_reply_to`, `ack_requested`) should follow the same convention.
- Two version bumps instead of one was deliberate (safer rollout). v0.25.0 lands the remaining pieces.

---

## 1. Envelope (`internal/envelope/envelope.go`)

**Status**: `Sender` shipped in v0.23.0; `Subject` shipped in v0.24.0. `InReplyTo` and `AckRequested` are still outstanding (target: v0.25.0).

Shipped shape:

```go
type Message struct {
    ID        string    `json:"id"`
    Sender    string    `json:"sender"`     // always-serialised, "" when no current source
    Subject   string    `json:"subject"`    // always-serialised, "" when no subject
    Payload   string    `json:"payload"`
    CreatedAt time.Time `json:"created_at"`
}
```

Target shape after v0.25.0:

```go
type Message struct {
    ID            string    `json:"id"`
    Sender        string    `json:"sender"`
    Subject       string    `json:"subject"`
    Payload       string    `json:"payload"`
    CreatedAt     time.Time `json:"created_at"`
    InReplyTo     string    `json:"in_reply_to"`     // uuid of message being replied to; "" when none
    AckRequested  bool      `json:"ack_requested"`   // request auto-ack on delivery
}
```

Wire-shape note: fields are **always serialized** (not `omitempty`). The original spec proposed `omitempty`; the implementation chose stable wire shape — receivers see every field, every time, even when empty. Apply the same convention to `InReplyTo` and `AckRequested` for consistency.

`envelope.New()` shipped as `(sender, subject, payload, now)`. v0.25.0 will likely keep that signature and set `InReplyTo`/`AckRequested` post-construction (matching how `Subject` is currently set in `handleBroadcast` after `envelope.New`).

Destination is no longer carried in the envelope — it lives only in the NATS subject (`<orgID>.<handle>.<pipe>`) where it always belonged. Confirmed in `internal/server/broadcast_subscriber.go:73`, which reads `parts[1]` of the subject for the destination handle.

Mirror every change in `cliproto.ReadMessage` (`internal/cliproto/types.go:83-88`) — daemon↔CLI IPC desyncs otherwise.

`Sender` is intentionally nullable (empty string permitted) — preserves the "headless publish" pattern (`ppz send foo "..."` with no current source set). `--request-ack` is the only operation that requires a non-empty `Sender`; enforced at the CLI level (see §3).

## 2. Daemon — separate sender from destination (`internal/daemon/handlers.go`)

**Status**: shipped in v0.23.0. The split lives at `handlers.go:651-658` — sender now pulled from `d.State.Current(req.Session)` independently of the destination resolution.

Nit (carryover): the variable that holds the *destination* is still named `current` (`handlers.go:591`). This is a holdover from when sender and destination were conflated; now that they're separate, the name misleads — a future cleanup pass should rename it to `destination`.

`v0.25.0` will need to extend this function to populate `env.InReplyTo` and `env.AckRequested` from new `BroadcastRequest` fields. The `MsgSubject` plumbing already in place (`internal/cliproto/types.go:191-207`) is the template.

## 3. CLI — `ppz send` flags and success output (`internal/cli/send.go`)

**Status**: outstanding. None of these flags have shipped yet; success output is unchanged. Target: v0.25.0.

### Delivery receipt vs. read receipt

The existing `sent id=… subject=… bytes=…` line is **already a delivery acknowledgment** — the daemon writes it after the NATS PubAck confirms the broker has durably stored the message. So `--request-ack` is *not* asking for delivery confirmation (the user has that). It's asking for a **read receipt**: confirmation that the recipient's `ppz read` has consumed the message from their queue.

Surface this distinction in `--help` text and in the WIRE.md release note so users don't conflate the two and don't expect `--request-ack` to fix any reliability question that the existing line already answers.

### Flags

Add to `cmdSend`:

| Flag | Type | Effect |
|---|---|---|
| `--subject <string>` | string | Sets `Subject`. CLI rejects values starting with `ack:` (reserved for system). |
| `--in-reply-to <id>` | string | Sets `InReplyTo`. |
| `--request-ack` | bool | Sets `AckRequested: true`. **Preflight**: requires non-empty current source; CLI exits with `ENoCurrentSource` error code if absent. |

Forward all three through new / existing fields on `cliproto.BroadcastRequest`. The `MsgSubject` field already exists from v0.24.0 — the v0.25.0 PR adds `--subject` as its CLI surface plus the `ack:` prefix rejection. Two new fields (`InReplyTo`, `AckRequested`) need to be added to `BroadcastRequest` alongside; align their JSON tags with the envelope (`in_reply_to`, `ack_requested`) rather than the `msg_subject` precedent — the new fields are 1:1 with envelope fields and should look that way in IPC.

**Scope: `cmdSend` only for v0.25.** `cmdBroadcast` shares `BroadcastRequest` with `cmdSend`, so the IPC supports threading + ack on broadcast trivially; we are nonetheless leaving the broadcast CLI surface untouched in this release. Broadcast `--request-ack` (one ack per consumer that reads) is its own design with subscriber-tracking implications and was punted to a later release with the rest of the "who heard me?" survey signal.

### Daemon-side `ack:` prefix rejection

The CLI rejection alone is not sufficient. IPC is reachable from any client (custom scripts, third-party tools, harness adapters), so the reserved-prefix invariant must be enforced at the trust boundary — `handleBroadcast` itself, not just the CLI argument parser. Concrete rule:

> Any `BroadcastRequest` whose `MsgSubject` starts with `ack:` is rejected with `E_INVALID_SUBJECT` (new error code in `cliproto/errors`). Daemon-internal ack auto-emission bypasses `handleBroadcast` entirely (see §4) so the rejection rule has no exception path.

Belt-and-suspenders: friendly early error from the CLI for users who try, hard rejection at the IPC for everyone else.

### Success output

Change destination from `os.Stdout` to `os.Stderr` and add a token to mark the ack-requested case:

```
sent id=abc12345 to=foo.inbox bytes=11
sent id=abc12345 to=foo.inbox bytes=11 ack=requested    # with --request-ack
```

`ack=requested` token only present when the flag was used. No teaching hints (those belong in `--help`, not on every send). The `id` field shown is the last 8 hex chars of the UUID for visual brevity; the full UUID stays in `--json` and the message envelope.

**Stderr is a partial fix.** Moving to stderr survives `>/dev/null` and `>file` (stdout redirection only). It does *not* survive `&>/dev/null`, `2>&1 >/dev/null`, or `>file 2>&1` (which redirect both file descriptors). For the Salt Town root cause — Miner-Test/Prospector running ppz under harnesses that captured stdout to a log — stderr is sufficient. For users actively redirecting both streams, the success line is still hidden, which is the explicit semantics of "redirect everything." Document this in `--help` so nobody re-files the same bug at a different abstraction level.

WIRE.md §8 currently documents the existing stdout success format (line 262-263); the v0.25.0 update must change that doc and any e2e fixtures asserting on the output destination.

## 4. Daemon — auto-emit ack on cursor advance (`internal/daemon/read.go`)

**Status**: outstanding. Read formatter is already prepared to render `ack:read → <id8>` rows (shipped in v0.24.0), but nothing produces them yet. Target: v0.25.0.

### Order of operations: advance-then-emit, fire-and-forget

```
for each msg about to advance cursor past:
    deliver msg to CLI
    advance cursor                                          # always; never blocks on ack publish
    if msg.AckRequested && msg.Sender != "" && !strings.HasPrefix(msg.Subject, "ack:"):
        ack := envelope.New(self, "ack:read", "", clock.Now())   # self may be ""
        ack.InReplyTo = msg.ID
        # ack.AckRequested = false (default → no recursion)
        publishEnvelope(<orgID>, msg.Sender, "inbox", ack)        # best-effort; logs on failure, never retries
```

**Cursor advances unconditionally on successful CLI delivery.** The ack publish is best-effort, fire-and-forget. This is the load-bearing decision and reverses the v0.7-of-this-spec proposal. Reasoning:

- *Why not emit-then-advance?* Coupling cursor advancement to ack-publish success means a NATS partition or any transient publish failure wedges the receiver — they cannot read any new messages until the ack lands. The blast radius of "ack publish broken" becomes "reading is broken." That's strictly worse than fire-and-forget.
- *Why not bounded retry with fall-through?* Adds complexity (retry timer, backoff schedule, where to queue) for marginal recovery on transient failures, and the failure mode after exhaustion is the same as fire-and-forget anyway. NATS's own publish-side reliability is the right layer for this.
- *Why not a background ack queue?* Same reasoning, plus introduces durable state (queue persistence across daemon restarts) we don't otherwise need.

`--request-ack` is therefore explicitly **best-effort**. Documented in `--help` and the WIRE.md release note. If a sender requires guaranteed confirmation, they should layer their own re-send-on-timeout pattern on top — which is exactly the discipline they would have applied without `--request-ack` anyway. The flag's value is *typical-case* friction reduction, not protocol-level reliability.

Operationally: if ack publish fails, log at warn level with the original message id and the recipient's sender. Sender will see no ack — indistinguishable from "not yet read," which is correct since the system can't tell whether the ack publish failure was transient or permanent. Sender-side timeout handling (whether implicit or via a future `ppz wait-ack`) makes this self-healing.

### Guards

1. **Skip emission if `msg.Subject` starts with `ack:`** — loop guard. Belt-and-suspenders since acks have `AckRequested: false` by default.
2. **Empty `msg.Sender` (headless original publish)** — emit anyway only if there's a destination. Since the destination *is* `msg.Sender`, an empty `msg.Sender` means "nowhere to send the ack." Skip in this case; log at debug level. (This case is rare — `--request-ack` requires a non-empty current source on the publish side, enforced at the CLI per §3, so reaching this branch implies a non-CLI client violated the contract.)
3. **Reader's `self == ""` is NOT a guard.** Earlier drafts skipped emission when the reader had no current source. Removing: an ack with empty `Sender` is still routable (destination is `msg.Sender`, not `self`) and still informative to the original sender ("your message was read by an agent without an attributed source"). Silently dropping the ack here would defeat the feature for senders who explicitly opted in. Sender will see `-` in the sender column, which is the correct rendering for unattributed messages.

### Ack construction must bypass `BroadcastRequest`

The ack auto-emit path constructs envelopes directly and publishes via an in-process helper (e.g. `daemon.publishEnvelope(orgID, destination, env)`), **not** by re-entering `handleBroadcast` with a `BroadcastRequest{MsgSubject: "ack:read"}`. Two reasons:

- The daemon-level `ack:` prefix rejection (see §3) blocks IPC callers from constructing ack messages — auto-emit is the legitimate exception, and the cleanest way to express "trusted internal caller" is to skip the IPC type entirely.
- Separates concerns: `handleBroadcast` is the IPC trust boundary; ack emission is an internal effect of the read path. They have different invariants and shouldn't share a code path.

Sketch:

```go
func (d *Daemon) publishEnvelope(ctx context.Context, orgID uuid.UUID, dest, pipe string, env envelope.Message) error {
    data, _ := env.Marshal()
    subject := natsubj.Subject(orgID, dest, pipe)
    return d.NC.Publish(subject, data)   // no validation, no DisallowUnknownFields, internal only
}
```

`handleBroadcast` gets refactored to call `publishEnvelope` after assembling the envelope; ack emission calls `publishEnvelope` directly with a pre-built ack envelope. One publish helper, two callers, one trust model per caller.

The reader's daemon is the right place to do this because it's where cursor state lives (`~/.ppz/cursors/<session>.json`); the server doesn't track per-session read positions.

## 5. Default `read` output format

**Status**: shipped in v0.24.0. Implementation in `internal/cliproto/read_format.go`; tested in `internal/cliproto/read_format_test.go` (6 cases) plus e2e fixture `tests/read/read-tabular-default-on-broadcast/`. 13 legacy fixtures explicitly opt into `--bare`. `tests/lib/normalize.sh` has a BOL-anchored `HH:MM:SS` rule.

For `.inbox` and `.broadcast` pipes (`.stdout`/`.stdin`/custom pipes keep the current bare default):

```
14:23:01  sheriff      [urgent] miner-test, status update?
14:23:42  -            general broadcast, no subject
14:25:00  miner-test   ack:read → 90abcdef
14:30:11  smelter      status update:
                       step 1 complete
                       step 2 in progress
```

**Rules:**

- **Time**: `HH:MM:SS` for today, `MM-DD HH:MM` for older. Absolute, stable across re-reads. Relative time would invalidate caches when the same buffer is re-ingested by an agent.
- **Sender column**: handle, or `-` for empty `Sender`.
- **User subjects** rendered inline as `[subject] payload`.
- **System subjects** (anything starting with `ack:`) rendered as `<subject> → <last-8-hex-of-uuid-without-hyphens>`. Last 8 used because UUIDv7 prefix is timestamp-derived (low entropy among co-second messages).
- **Multi-line payload**: continuation lines indented to align with the payload column.
- **Empty payload** (acks): nothing after the subject/arrow.

Add `--bare` flag (mutually exclusive with `--json` / `--tty` / `--raw`) — forces legacy payload-only output for any pipe. Stable opt-out for scripts.

Update `cliproto.PrintBroadcast` and the read formatter to consume `Sender` / `Subject` / `InReplyTo`. Remove all references to the dropped `Handle` field.

## 6. Wire / protocol

**Status**: split across two shipped releases plus one pending.

**v0.23.0 (shipped)** — `handle` removed from envelope, `sender` added. WIRE.md §3 updated. Migration: 24h JetStream retention naturally clears v0.22 messages. Verified: `encoding/json` drops unknown `handle` and zero-values missing fields cleanly (no `DisallowUnknownFields` decoder is configured on envelope payloads).

**v0.24.0 (shipped)** — `subject` field added; tabular `ppz read` default for inbox-shaped pipes; `--bare` opt-out. WIRE.md §3 and §8 (CLI pinned stdout) updated.

**v0.25.0 (pending)** — `in_reply_to`, `ack_requested` envelope fields; `--subject`/`--in-reply-to`/`--request-ack` CLI flags; send success output moved to stderr with `ack=requested` token; daemon ack auto-emission. Suggested release-note text:

> *"`ppz send --request-ack` triggers a daemon-emitted ack message back to the sender's inbox when the recipient's `ppz read` consumes the message. The flag is **best-effort** — sender sees no ack if either the recipient hasn't read yet OR the ack publish itself failed; both are indistinguishable. Senders requiring strict guarantees should layer a re-send-on-timeout on top. New `--subject`/`--in-reply-to` flags on `ppz send` populate the corresponding envelope fields. Send success line moved from stdout to stderr — scripts capturing stdout previously swallowed it. Envelope adds `in_reply_to` and `ack_requested` fields (always serialised, like `sender` and `subject`)."*

## 7. Test surface

**Shipped (v0.23.0 / v0.24.0)**:
- Envelope round-trip with `sender` and `subject` (`internal/envelope/envelope_test.go`).
- Sender correctly reports publishing agent (regression coverage for the v0.22 conflated-variable bug).
- Tabular renderer golden-file tests covering empty sender, multi-line payload, ack subject, user subject, indent alignment, timezone — `internal/cliproto/read_format_test.go` (6 cases).
- E2e: `tests/read/read-tabular-default-on-broadcast/` locks the new format. 13 legacy fixtures opted into `--bare` to keep stable assertions.
- `tests/lib/normalize.sh` has a tightly-scoped (BOL-anchored) `HH:MM:SS` substitution so other clock-like substrings in payloads aren't rewritten.

**Outstanding for v0.25.0**:
- Envelope round-trip with `in_reply_to` and `ack_requested`.
- `--subject ack:foo` rejected by CLI before daemon call.
- **Daemon rejects `MsgSubject` starting with `ack:`** — direct IPC test that bypasses the CLI, asserts `E_INVALID_SUBJECT`. Confirms the trust-boundary enforcement, not just CLI argument validation.
- `--request-ack` from no-current-source rejected by CLI before daemon call.
- Ack emission triggered by cursor advance on a `--request-ack` message — verifies advance-then-emit ordering by injecting a publish failure and asserting the cursor still advances.
- Loop guard: ack message with `Subject: "ack:read"` does not produce a second ack.
- Headless-original-publish: original `msg.Sender == ""` → daemon skips ack emission and logs at debug.
- Reader's `self == ""` does NOT skip emission: ack is published with empty `Sender` and routes correctly to `msg.Sender.inbox`.
- Send success output: stderr destination + `ack=requested` token (e2e fixture asserting both presence-with-flag and absence-without).
- E2e: send-with-ack → read-on-receiver → sender sees ack in their own inbox with correct `in_reply_to`.
- WAN variant using the framework from commit `e685516` (RED scenario for high-latency ack delivery; explicitly covers transient ack-publish failure → no retry → cursor still advanced).
- Regression: `ppz read --bare` output byte-identical to legacy default for `.inbox` pipes (already partially covered by the 13 opted-in fixtures).

## 8. Audit before merging

- `grep DisallowUnknownFields` across the repo — any decoder configured to reject unknown fields chokes on the new envelope. Remove or update.
- `grep '\.Handle\b'` — surfaces every formatter, test, and JSON path that needs to switch to `.Sender` (or be removed).
- **`grep -rn 'ppz send' docs/ README.md tests/`** — examples in docs *and* shell fixtures in `tests/` may capture/pipe `ppz send` output and break against the new format / stderr destination. Skipping `tests/` here is exactly what bit the v0.24 release (the 13-fixture `--bare` retrofit was a recovery for an analogous oversight); don't repeat it.
- `grep -rn '2>' tests/ | grep -i 'ppz send'` — narrow scan for any fixture explicitly redirecting stderr from a send call; after the stdout→stderr move, those will start capturing what they previously didn't.
- Survey-source references: Smelter's literal request was `--confirm`. Docs/changelog should explain `--request-ack` is what we shipped (non-blocking, read-receipt semantics) — and why we did not build blocking `--confirm` (deadlock risk, sender-side polling burden).

## 9. Implementation order

**Done in v0.23.0 (PR #30)**: 1, 2 (sender-only), 3.
**Done in v0.24.0 (PR #31)**: 2 (subject + `MsgSubject`), 6 (tabular formatter + `--bare`), tests for those.
**Pending for v0.25.0**: 4, 5, plus envelope additions for `InReplyTo`/`AckRequested`, plus updated `BroadcastRequest`, plus WIRE.md / manifest changes.

Original sequence (preserved for reference):

1. `internal/envelope/envelope.go` — struct + `New()` signature.
2. `internal/cliproto/types.go` — `ReadMessage` + `BroadcastRequest` fields.
3. `internal/daemon/handlers.go` — split sender / destination.
4. `internal/cli/send.go` — flags + stderr + new short-delta output.
5. `internal/daemon/read.go` (cursor-advance loop) — ack auto-emission.
6. `cliproto.PrintBroadcast` + read formatter — tabular default, `--bare` flag, pipe-type gating.
7. `docs/WIRE.md` + `update/manifest.json` + CHANGELOG.
8. Unit tests in lockstep with each step; e2e + WAN tests at the end.

## 10. Carryover nits to clean up in v0.25.0

Small items spotted during the v0.23/v0.24 review that don't justify their own PR but should be folded in opportunistically:

- **`envelope.go` package comment is stale** (`internal/envelope/envelope.go:1-2`) — *"every message published on `<org_id>.<handle>.broadcast`"* should generalise to `<pipe>` since envelopes flow on `inbox`/`stdin`/`stdout` too.
- **WIRE.md §3 line 73** references *"a session that never ran `ppz connect`"* — there's no `ppz connect` command. Should be `ppz source create` / `ppz source switch`.
- **`current` variable in `handlers.go:591`** is now semantically the *destination* (after the v0.23 sender split) but still named `current`. Misleading holdover; rename to `destination` in passing.
- **`MsgSubject` vs `Subject` naming asymmetry** — `BroadcastRequest.MsgSubject` (`msg_subject` JSON tag) vs `envelope.Message.Subject` (`subject`). The asymmetry presumably exists to differentiate from the NATS-subject routing-key sense; if intentional, add a one-line comment justifying it. Otherwise consider aligning to `Subject` / `subject` on both sides.

## 11. Rejected alternatives (for context)

- **Blocking `ppz send --confirm`**: deadlock prone (mutual A↔B sends), and just moves the polling burden from receiver to sender. The surveyed Sheriff scenario (heads-down receiver) becomes the surveyed sender scenario (blocked sender) — net-zero improvement. Asynchronous ack-as-message has none of these failure modes and reuses existing read primitives.
- **ACK as a synthetic non-message signal**: would require a new IPC event type and break the "everything is a message" invariant that makes `ppz read --tail` and `ppz ls --watch` work uniformly today.
- **Server-side ack tracking**: server doesn't currently track per-session read positions (cursors live in the daemon at `~/.ppz/cursors/<session>.json`). Adding server-side cursor state is a much larger architectural change for marginal benefit.
- **Compat alias for `handle` field**: would carry a deprecated, semantically-wrong field name forward indefinitely. ppz is pre-1.0; clean break is cheaper than perpetual aliasing.
- **Default-on `--request-ack`**: every send produces ack noise in someone's inbox. Opt-in keeps the cost on the asker.
- **Educational hint text on every `ppz send`**: noise on repeat use, breaks scripts, doesn't fix the actual root cause (output redirection swallowed the existing line). Stderr move solves that without nagging.
