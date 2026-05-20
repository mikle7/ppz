#!/usr/bin/env bash
# Production scenario reported 2026-05-20: a user creates an uncollared
# pipe, sends to it, then a ppz-server deploy (systemctl restart) ships.
# Post-restart the pipe still appears in `ppz ls` (the postgres row
# survived) but `ppz send <name>` returns
#   error: E_SOURCE_NOT_FOUND: source '<name>' not found
# and the retained payload is gone.
#
# Root cause: internal/natsauth/natsauth.go:216 mints a fresh
#   os.MkdirTemp("", "ppz-jetstream-")
# on every process start. Even though the stream config says
# jetstream.FileStorage, the *path* is randomized, so a restart
# orphans the previous on-disk store and starts against an empty
# directory. JS sees no streams; the send path's uncollared lookup
# misses; it falls through to the legacy collared interpretation
# (room → room.inbox), which has no matching source and 404s.
#
# Fix: stable, configurable StoreDir (e.g. PPZ_JETSTREAM_STORE_DIR,
# default to a persistent path) so restart reattaches to the
# existing streams.
#
# Test: create + send + restart ppz-server (the container holds the
# embedded NATS + JS) + assert (a) send still succeeds against the
# bare name, and (b) the pre-restart payload is still readable.
. /tests/lib/common.sh

HANDLE="restartroom"

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle    >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1

ppz_a pipe create "$HANDLE" >/dev/null
ppz_a send --from pubsub "$HANDLE" "before-restart" >/dev/null 2>&1

# Restart ppz-server (which embeds NATS + JetStream). Same container,
# same /tmp — proves the bug is the per-process StoreDir
# randomization, not actual disk wipe. Mirrors the
# tests/reliability/nats-events-recorded-in-diagnostics pattern.
docker stop compose-ppz-server-1 >/dev/null
sleep 1
docker start compose-ppz-server-1 >/dev/null
wait_for 600 'ppz_a ls >/dev/null 2>&1'

# User's exact symptom: post-restart send to the uncollared pipe.
# Capture exit + the first verdict token (sent / error code) so the
# assertion is stable across version, id, byte-count changes.
err=$(mktemp)
ppz_a send --from pubsub "$HANDLE" "after-restart" 2>"$err"
echo "send-exit=$?"
grep -oE '^sent\b|^error: E_[A-Z_]+' "$err" | head -1
rm -f "$err"

# Stream data must survive too — not just metadata. Read both
# payloads back, sorted for ordering insensitivity.
ppz_a reread "$HANDLE" -l 2 --bare | sort
