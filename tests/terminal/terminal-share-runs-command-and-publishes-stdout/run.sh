#!/usr/bin/env bash
# `ppz terminal share <handle> -- <cmd>` allocates a PTY for the child,
# captures its byte stream, and publishes chunks verbatim to <handle>.stdout.
# Foreground mode: blocks until the child exits.
#
# .stdout is byte-faithful (no line splitting, no \n insertion); cooked-mode
# OPOST is on, so the child's two `\n` come back as `\r\n`. We concatenate
# every .stdout chunk's payload, strip the CRs, and assert "hello/world"
# survive intact.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal share term1 -- sh -c 'echo hello && echo world' >/dev/null

wait_for 20 "ppz_a reread term1.stdout --json | jq -r '.payload' | tr -d '\r' | grep -q world" >/dev/null
ppz_a reread term1.stdout --json | jq -r '.payload' | tr -d '\r' | sed '/^$/d'
