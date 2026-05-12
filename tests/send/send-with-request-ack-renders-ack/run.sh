#!/usr/bin/env bash
# End-to-end: alpha sends to beta with --request-ack; after beta reads the
# message, beta's daemon auto-emits an `ack:read` envelope back to
# alpha's inbox. Alpha then reads its inbox and sees the ack rendered in
# tabular form (`ack:read → <id8>`) with sender=beta.
#
# Both daemons are in org alpha (a uses key-alpha, b uses key-alpha2 —
# same org, different api keys); their NATS streams share addressability.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

# Alpha sets up its current source so --request-ack passes the preflight
# AND so the receiver knows where to send the ack back to. Beta sets up
# its own source so it can read its inbox (and so the formatter has a
# self handle to stamp on the ack as Sender=beta).
ppz_a terminal create alpha-side >/dev/null
ppz_b terminal create beta-side  >/dev/null

# Send a request-ack message to beta-side's inbox. Discard stderr so the
# stdout of the test scenario stays clean — the assertion is on the
# downstream ack rendering, not the immediate send line.
ppz_a send beta-side "ping with ack" --request-ack 2>/dev/null

# Wait for the message to reach beta's pipe, then beta reads it. Reading
# advances the cursor; the v0.25.0 §4 auto-emit hook fires on advance and
# publishes ack:read back to alpha-side.inbox.
wait_for 20 "ppz_b ls | grep -q 'ping with ack'" >/dev/null
ppz_b read --bare inbox >/dev/null

# Wait for the ack to arrive in alpha-side's inbox, then read it back via
# alpha. Tabular default for inbox-shaped pipes renders system subjects
# (`ack:*`) as `<subject> → <last-8-hex-of-id>`.
wait_for 20 "ppz_a ls | grep alpha-side.inbox | grep -qv ' 0 '" >/dev/null
ppz_a read inbox
