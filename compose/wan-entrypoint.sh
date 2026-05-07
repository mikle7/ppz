#!/bin/sh
# WAN-mode daemon entrypoint. Used only by the e2e-wan suite.
#
# Applies a `tc qdisc netem` to eth0 so all egress from this container
# carries an artificial delay before forwarding the original argv on
# to the daemon. Lets us assert that hot paths (read, follow, etc.)
# stay batched / pipelined under WAN-style RTT — bugs that look fine
# in sub-ms compose RTT (e.g. one-round-trip-per-message loops) are
# very visible at 200ms.
#
# Tunables via env (compose-injected):
#   PPZ_WAN_DELAY   default "200ms"
#   PPZ_WAN_JITTER  optional, e.g. "20ms" — appended to netem
#
# Requires NET_ADMIN cap on the container and iproute2 in the image.

set -e

DELAY="${PPZ_WAN_DELAY:-200ms}"
JITTER="${PPZ_WAN_JITTER:-}"
NETEM="delay $DELAY"
if [ -n "$JITTER" ]; then
  NETEM="$NETEM $JITTER"
fi

if ! tc qdisc add dev eth0 root netem $NETEM 2>/dev/null; then
  # On container restart the qdisc may already be present — tolerate
  # by replacing rather than adding.
  tc qdisc replace dev eth0 root netem $NETEM
fi
echo "wan-tc: applied netem '$NETEM' on eth0" >&2

exec "$@"
