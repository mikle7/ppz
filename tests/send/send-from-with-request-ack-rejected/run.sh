#!/usr/bin/env bash
# `--from` and `--request-ack` don't compose. The ack:read auto-emit
# always routes to <sender>.inbox; --from lets the caller stamp any
# handle as the sender, so combining them is asking the recipient's
# daemon to send an ack to an inbox the caller may or may not control.
# Silent dead letter at best, surprising delivery to a third party at
# worst.
#
# Resolution: reject the combination at parse time with a clear
# message. If the user genuinely wants the ack routed to a specific
# handle, they should `ppz set handle <H>` first — that makes their
# session current and the ack routing matches their reading inbox.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null
ppz_a source create receiver >/dev/null
ppz_a unset handle >/dev/null

err=$(mktemp)
ppz_a send --from somebody receiver.inbox "hi" --request-ack 2>"$err"
rc=$?

# Token "--from and --request-ack" pins the helpful framing without
# requiring exact wording. Anchored to "and" so we don't match the
# full message body (which mentions both tokens again).
grep -oE -- '--from and --request-ack' "$err" | head -1
echo "exit=$rc"
rm -f "$err"
