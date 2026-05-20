#!/usr/bin/env bash
# Bug: viewing an uncollared (sourceless) pipe in the GUI returns 404.
# The org pipes tab links uncollared rows to `/orgs/<slug>/pipes/<leaf>`
# (handlers_gui.go), but no route is mounted for that shape — only the
# collared `/orgs/{id}/sources/{handle}/pipes/{pipe}` exists. Clicking
# the link lands on Go's default 404.
#
# This scenario provokes the bug: create an uncollared pipe `testroom`
# at the account root, publish three messages, fetch the GUI page at
# `/orgs/alpha/pipes/testroom`, and assert that each buffered message
# is rendered with the same `data-message="<id>:<created_at>:<payload>"`
# marker the collared channel page uses.
. /tests/lib/common.sh
auth_as_foo

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a unset handle >/dev/null 2>&1
ppz_a unset namespace >/dev/null 2>&1

ppz_a pipe create testroom >/dev/null
ppz_a send testroom "msg-1" >/dev/null
ppz_a send testroom "msg-2" >/dev/null
ppz_a send testroom "msg-3" >/dev/null
wait_for 20 "ppz_a ls | grep -q msg-3" >/dev/null

curl_server "/orgs/alpha/pipes/testroom" \
  | grep -oE 'data-message="[^"]+"' \
  | sed -E 's/data-message="([^"]+)"/\1/'
