#!/usr/bin/env bash
# `ppz send --subject 'ack:read' ...` is rejected with E_INVALID_SUBJECT.
# The `ack:` prefix is reserved for daemon-emitted protocol messages.
# CLI rejects belt; daemon (handlers.go handleBroadcast) rejects suspenders
# — this scenario exercises the CLI path.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat >/dev/null

# Capture stderr + stdout — error line lands on stderr.
ppz_a send chat "hi" --subject "ack:read" 2>&1 | grep -oE '^error: E_[A-Z_]+' || true
