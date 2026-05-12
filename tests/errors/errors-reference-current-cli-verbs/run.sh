#!/usr/bin/env bash
# Error messages must reference the *current* CLI surface. Several were
# written when the verbs were top-level (`ppz login`, `ppz daemon`,
# `ppz switch HANDLE`, `ppz create HANDLE`); the surface has since
# nested under `ppz daemon ...` / `ppz source ...`, but the error
# hints still send users to commands that don't exist.
#
# Triggers below produce one error each; the assertions pin the new
# wording so future drift gets caught.
. /tests/lib/common.sh

echo "--- E_NOT_LOGGED_IN: should reference 'ppz daemon login' ---"
# Fresh daemon home, no credentials — ls is the cleanest trigger.
ppz_a ls 2>&1 | grep '^error:' || true

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

echo "--- E_NO_CURRENT_SOURCE: should reference 'ppz source create' / 'ppz source switch' ---"
# `ppz read inbox` (bare alias for `<current>.inbox`) requires a current source.
ppz_a read inbox 2>&1 | grep '^error:' || true
