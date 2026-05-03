#!/usr/bin/env bash
# Repro: `ppz connect foo` creates source 'foo' as kind=message (broadcast
# only). Subsequent `ppz terminal share` against the current source
# auto-provisions stdin + stdout pipes — but the source's kind stays
# 'message', so the server GUI (which derives row-count from kind via
# `db.Source.Pipes()`) only renders foo:broadcast. The user sees stdin
# and stdout missing in the GUI even though `ppz ls` lists them.
#
# After fix: bare wrap upgrades the source to a PTY-flavoured listing,
# so the org page renders three rows (broadcast / stdin / stdout) for
# foo, matching the existing `terminal share H -- true` shape covered by
# server-gui-org-page-shows-pty-channels.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create foo >/dev/null
ppz_a terminal share -- true >/dev/null

PAGE=$(curl_server "/orgs/alpha")

echo "--- foo source-rows in GUI ---"
echo "$PAGE" \
  | grep -oE 'data-source-row="foo:[^"]+"' \
  | sed -E 's/data-source-row="([^"]+)"/\1/' \
  | sed -E 's/:(just now|[0-9]+ (seconds?|minutes?|hours?|days?) ago):/:RELATIVE:/' \
  | sed -E 's|:RELATIVE:\{&#34;.*\}$|:RELATIVE:STDCTRL_JSON|' \
  | sort
