#!/bin/sh
# Server entrypoint: run migrations + seed (if PPZ_SEED=true), then
# exec the server. NATS NSC/JWT credentials are expected in env at
# this point — production injects them from Pulumi-managed Secrets
# Manager; local compose generates ephemerals via ppz-natsbootstrap
# which mints fresh keys per stack lifecycle.
set -e

if [ "${PPZ_SEED:-false}" = "true" ]; then
  echo "[entrypoint] running seed..."
  /usr/local/bin/ppz-seed --dir "${PPZ_SEED_DIR:-/seed}"
fi

if [ -z "${PPZ_NATS_OPERATOR_JWT:-}" ]; then
  # Cache the bootstrap output to a file on the (persistent) seed
  # volume. Without this, a container restart (`docker stop` + `docker
  # start`) regenerates the NATS Operator key — invalidating every
  # already-issued account / user JWT. The reliability suite stops +
  # starts ppz-server to exercise NATS outage recovery; clients with
  # pre-issued JWTs would then hit "Authorization Violation" on
  # reconnect, masking real reconnect-handler signal under what's
  # actually a key-rotation problem. The seed volume is recreated by
  # `make e2e-up` so each fresh test run still gets fresh keys.
  cache="${PPZ_SEED_DIR:-/seed}/nats-bootstrap.env"
  if [ ! -s "$cache" ]; then
    echo "[entrypoint] bootstrapping ephemeral NATS NSC/JWT credentials..."
    /usr/local/bin/ppz-natsbootstrap > "$cache"
  else
    echo "[entrypoint] reusing cached NATS NSC/JWT credentials from $cache"
  fi
  eval "$(sed 's/^/export /' "$cache")"
fi

exec /usr/local/bin/ppz-server
