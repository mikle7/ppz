#!/usr/bin/env bash
# v0.25.0 (§3): `ppz send` writes its `sent id=…` success line to stderr
# instead of stdout. Salt Town root cause for "missed the sent line": the
# harness captured stdout to a log; stderr survived. The assertion here:
#   - stdout produced by send is empty
#   - stderr contains a `sent id=…` line
#   - with --request-ack, the line gains an ` ack=requested` token
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null

err_plain=$(mktemp)
out_plain=$(mktemp)
ppz_a send chat "hi" >"$out_plain" 2>"$err_plain"

err_ack=$(mktemp)
out_ack=$(mktemp)
ppz_a send chat "hi-ack" --request-ack >"$out_ack" 2>"$err_ack"

# Plain send: stdout empty; stderr has `sent id=` line.
echo "stdout-empty=$([[ ! -s "$out_plain" ]] && echo yes || echo no)"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+$' "$err_plain" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

# --request-ack: stdout still empty; stderr line gains ack=requested token.
echo "stdout-empty-ack=$([[ ! -s "$out_ack" ]] && echo yes || echo no)"
grep -oE '^sent id=[a-f0-9]{8} to=[^ ]+ bytes=[0-9]+ ack=requested$' "$err_ack" | head -1 \
  | sed -E 's/id=[a-f0-9]{8}/id=ID8/; s/bytes=[0-9]+/bytes=N/'

rm -f "$out_plain" "$err_plain" "$out_ack" "$err_ack"
