#!/usr/bin/env bash
# `ppz send` invoked from inside a `ppz terminal share`-wrapped shell
# must stamp envelope.sender = the wrapped handle, NOT "".
#
# Repro for the bug fixed in PR #92: the daemon's IPCCreate skips
# SetCurrent for PTY-kind sources, so its own State.Current(session)
# is empty inside the share — even though terminalShareEnv exports
# PPZ_CURRENT_HANDLE=<handle> + PPZ_SESSION=<handle> into the wrapped
# child, and every other verb (status / read inbox / --request-ack
# preflight) reads env-first via effectiveCurrentHandle. Pre-fix,
# `ppz send` stamped sender="" because it only consulted the daemon's
# per-session state, ignoring the env override the rest of the CLI
# already honoured. User-visible symptom from the field report:
#
#   $ ppz terminal share jimmy                       # outer shell
#   jimmy@... % ppz send eric "Hello Eric, are you there?"
#   jimmy@... % ppz_a read eric.inbox --json
#   {"sender":"","payload":"Hello Eric, are you there?",...}
#                ^^^^^^
#
# Closes the coverage gap left by the unit tests in PR #92: those
# tests cover the CLI's PPZ_CURRENT_HANDLE → SendRequest.Sender
# forwarding AND the daemon's senderForRequest precedence helper
# SEPARATELY, but nothing exercises the full chain
#
#   cmdSend → SendRequest.Sender → resolveSendTarget
#           → senderForRequest → envelope.sender
#
# against a live daemon. A future positional-arg swap on
# resolveSendTarget (it now takes six string args) would pass every
# unit test in this PR but break this e2e — which is exactly the
# kind of regression e2e coverage exists to catch.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# eric: message-kind destination for the cross-handle send. `source
# create` auto-provisions .inbox.
ppz_a source create eric >/dev/null

# `terminal share <H>` with an explicit handle is the create-and-wrap
# path: it provisions jimmy as a fresh pty-kind source AND wraps the
# child command. IPCCreate deliberately skips SetCurrent for PTY-kind
# sources, so the daemon's State.Current("jimmy" session id) is empty
# — even though terminalShareEnv exports PPZ_CURRENT_HANDLE=jimmy +
# PPZ_SESSION=jimmy into the wrapped bash. That mismatch is exactly
# the bug surface: pre-fix, the wrapped send had no way to tell the
# daemon who it was, so envelope.sender stamped "".
#
# bash exits when `ppz send` returns; the share exits when bash
# exits. PPZ_IPC_SOCKET is inherited via terminalShareEnv's
# os.Environ() append, so plain `ppz` inside the wrapped shell
# targets the same daemon as the outer ppz_a.
ppz_a terminal share jimmy -- bash -c 'ppz send eric "Hello Eric, are you there?"' >/dev/null 2>&1

# Wait for the published envelope to land on eric.inbox.
wait_for 20 "ppz_a reread eric.inbox --json | grep -q 'Hello Eric'" >/dev/null

# Contract: envelope.sender = "jimmy" (wrapped handle), NOT "".
# Without PPZ_CURRENT_HANDLE → Sender forwarding (cli/send.go) OR
# the daemon's hint-wins precedence (daemon/sender_resolve.go), the
# sender field is empty and this assertion fails.
ppz_a reread eric.inbox --json | jq -c '{sender, payload}'
