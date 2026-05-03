#!/usr/bin/env bash
# `ppz connect <handle>` is the combo verb: it ensures the source exists and
# becomes the current source for this daemon. After running it, status
# reflects the new current and ls shows the source's broadcast pipe.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create chat
echo "--- status ---"
ppz_a status | grep '^current source:'
echo "--- ls ---"
ppz_a ls | ls_normalize
