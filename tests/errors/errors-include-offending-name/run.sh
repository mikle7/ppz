#!/usr/bin/env bash
# Errors that name an entity must include the entity name in the message.
# "source not found" is useless without saying *which* source — likewise
# for E_PIPE_TAKEN, E_SOURCE_TAKEN, etc.
#
# Each trigger below names the entity it's complaining about; the test
# captures the printed error line and locks the wording in expected.txt.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

echo "--- E_SOURCE_NOT_FOUND ---"
ppz_a read missingsrc.broadcast 2>&1 | grep '^error:' || true

ppz_a source create foo >/dev/null
ppz_a pipe create archive >/dev/null

echo "--- E_PIPE_TAKEN ---"
ppz_a pipe create archive 2>&1 | grep '^error:' || true

echo "--- E_SOURCE_TAKEN ---"
ppz_a source create foo 2>&1 | grep '^error:' || true

echo "--- E_INVALID_PIPE (reserved name) ---"
ppz_a pipe create foo.system 2>&1 | grep '^error:' || true
