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
  echo "[entrypoint] bootstrapping ephemeral NATS NSC/JWT credentials..."
  eval "$(/usr/local/bin/ppz-natsbootstrap | sed 's/^/export /')"
fi

exec /usr/local/bin/ppz-server
