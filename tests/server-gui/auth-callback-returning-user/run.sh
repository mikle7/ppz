#!/usr/bin/env bash
# Returning user: same OAuth flow twice. Second login must NOT create
# a duplicate users row — the upsert keys on github_id and updates
# the existing row in place.
. /tests/lib/common.sh

COOKIE_1=$(mktemp); COOKIE_2=$(mktemp)
trap "rm -f $COOKIE_1 $COOKIE_2" EXIT

echo "--- first login (creates the user) ---"
curl_server "/auth/github/start" -L -c "$COOKIE_1" -o /dev/null -s -w "status=%{http_code}\n"
UID_1=$(curl_server "/me" -b "$COOKIE_1" -s | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
echo "user_id_first=$UID_1"

echo ""
echo "--- second login (separate session, same GH user) ---"
curl_server "/auth/github/start" -L -c "$COOKIE_2" -o /dev/null -s -w "status=%{http_code}\n"
UID_2=$(curl_server "/me" -b "$COOKIE_2" -s | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
echo "user_id_second=$UID_2"

echo ""
echo "--- same user_id (no duplicate row) ---"
[[ -n "$UID_1" && "$UID_1" == "$UID_2" ]] && echo "same_user=true" || echo "same_user=false"

echo ""
echo "--- exactly one 'gh-test-user' org on the dashboard ---"
curl_server "/dashboard" -b "$COOKIE_2" -s | grep -oE 'data-org="gh-test-user"' | wc -l | tr -d ' '
