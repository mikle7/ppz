#!/usr/bin/env bash
# `/` is now the marketing landing — logo, "connecting agents"
# tagline, and two animated terminal demos (broadcast + terminal-
# share) that auto-cycle. The actual operator dashboard (org index)
# moves to `/dashboard`. This test pins the marketing-page contract
# at root so we don't accidentally regress to the old org-list view.
#
# `grep -m 1` (not `| head -1`) so grep itself stops after the first
# match — `head -1` closing the pipe early would SIGPIPE grep and,
# with pipefail set, propagate exit 141 to the script.
. /tests/lib/common.sh

page="$(curl_server "/")"

echo "--- landing page identifier present (so we never serve org-list at /) ---"
printf '%s' "$page" | grep -oE -m 1 'data-page="landing"'

echo "--- tagline rendered ---"
printf '%s' "$page" | grep -oE -m 1 'connecting agents'

echo "--- logo asset referenced ---"
printf '%s' "$page" | grep -oE -m 1 'src="/assets/logo\.png"'

echo "--- five paired demos: broadcast / monitor / inbox / pipes / remote ---"
printf '%s' "$page" | grep -oE -m 1 'data-pair="broadcast"'
printf '%s' "$page" | grep -oE -m 1 'data-pair="monitor"'
printf '%s' "$page" | grep -oE -m 1 'data-pair="inbox"'
printf '%s' "$page" | grep -oE -m 1 'data-pair="pipes"'
printf '%s' "$page" | grep -oE -m 1 'data-pair="remote"'

echo "--- pane counts: 7 senders (broadcast=1, monitor=2, inbox=1, pipes=1, remote=2) + 4 receivers ---"
printf '%s' "$page" | grep -oE 'data-pane="sender"'   | wc -l | tr -d ' '
printf '%s' "$page" | grep -oE 'data-pane="receiver"' | wc -l | tr -d ' '

echo "--- typewriter script loaded ---"
printf '%s' "$page" | grep -oE -m 1 'src="/assets/typewriter\.js"'

echo "--- there is a clear path into the dashboard ---"
printf '%s' "$page" | grep -oE -m 1 'href="/dashboard"'
