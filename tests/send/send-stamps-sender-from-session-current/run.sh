#!/usr/bin/env bash
# `ppz send` MUST stamp envelope.sender from the calling shell's session
# current — distinct from the destination handle. Pre-fix, the CLI didn't
# forward the session id, so the daemon resolved current against the
# "default" session and stamped sender="" on the wire (regression that
# slipped past v0.23/v0.24 because no e2e covered the send → sender path).
#
# This fixture pins the contract: sender = current source of the
# publishing session, NOT the destination.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# Create both sources. handleCreate sets current to whichever was made
# most recently, so order matters: caller created last → current=caller.
# Result: caller is the publisher's identity; mailbox is the destination.
ppz_a source create mailbox >/dev/null
ppz_a source create caller >/dev/null
# Sanity-check the daemon's view of current matches our expectation.
ppz_a status | grep "^current source:"

ppz_a send mailbox "hello from caller" >/dev/null
wait_for 20 "ppz_a ls | grep -q 'hello from caller'" >/dev/null

# `reread --json` projects the full envelope; jq strips out only the
# fields we care about so UUID / timestamp don't need normalization.
ppz_a reread mailbox.inbox --json | jq -c '{sender, payload}'
