#!/usr/bin/env bash
# `terminal share` publishes the wrapped pty's size to a new
# `<handle>.stdctrl` pipe — initial size at wrap-start + every SIGWINCH.
# The web terminal viewer reads this to size xterm.js correctly so
# bytes laid out for N cols don't render at 80 cols.
#
# stdctrl carries any control signals that don't fit on stdout/in. v1
# only emits {type: "resize", cols, rows}; the type discriminator lets
# us add exit / title / focus / etc. without breaking the wire.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
# Trivial child so wrap exits cleanly. Initial SIGWINCH-equivalent fires
# during pty setup and publishes the starting size.
ppz_a terminal share term1 -- true >/dev/null

# stdctrl must be auto-provisioned on pty sources alongside stdin/stdout.
echo "--- ppz ls includes term1.stdctrl with at least one message ---"
wait_for 20 "ppz_a ls | ls_normalize | awk '\$1 == \"term1.stdctrl\" && \$2+0 > 0'" >/dev/null
ppz_a ls | ls_normalize | awk '$1 == "term1.stdctrl" {print $1, ($2+0 > 0 ? "has-msgs" : "empty")}'

echo "--- latest stdctrl message is a JSON resize event with non-zero dims ---"
ppz_a read term1.stdctrl --json \
  | jq -r '.payload' \
  | jq -c '{type: .type, cols_set: (.cols > 0), rows_set: (.rows > 0)}' \
  | tail -1
