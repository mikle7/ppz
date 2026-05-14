#!/usr/bin/env bash
# Users tab specifics: the /accounts/<id>/users page renders the Owner
# and Members subregions and stamps `data-todo` placeholders on the
# v2-only buttons (Invite, Transfer ownership). Lets the spec stay
# visible while the buttons sit disabled until v2 lands.
#
# (The "all three sections exist" assertion lives in
# server-gui-org-sub-nav-tabs — this test focuses on the users-tab
# internals.)
. /tests/lib/common.sh
auth_as_foo

org_id="$(cat /seed/org-alpha.txt)"
page="$(curl_server "/accounts/$org_id/users")"

echo "--- users section is present ---"
printf '%s' "$page" | grep -oE 'id="section-users"' | head -1

echo "--- users section: owner subregion ---"
printf '%s' "$page" | grep -oE 'data-users-subsection="owner"' | head -1

echo "--- users section: members subregion ---"
printf '%s' "$page" | grep -oE 'data-users-subsection="members"' | head -1

echo "--- users section: invites subregion (Phase 4) ---"
printf '%s' "$page" | grep -oE 'data-users-subsection="invites"' | head -1

echo "--- v2-only buttons are placeholders (data-todo) ---"
printf '%s' "$page" | grep -oE 'data-todo="transfer-ownership"' | head -1
