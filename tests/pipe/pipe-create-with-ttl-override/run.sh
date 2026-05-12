#!/usr/bin/env bash
# `--ttl=DUR` overrides the JetStream MaxAge on the provisioned stream.
# We assert the configured TTL is reported in the printer's output (the
# wire contract). Behavioural retention is too time-dependent to assert
# in a fixed-duration e2e — that's a server-internals concern.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null
ppz_a pipe create archive --ttl=168h
