#!/usr/bin/env bash
# `ppz set FOO BAR` for any FOO other than the known keys (currently
# just `handle`) must exit 2 with a clear "unknown key" message on
# stderr. Same for `ppz unset` and `ppz get`. Locked decision #20:
# the dispatcher is future-proof for additional settings, but
# day-one only `handle` is wired.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

for verb in set unset get; do
  echo "--- $verb foo bar (unknown key) ---"
  if [[ "$verb" == "set" ]]; then
    err=$(ppz_a $verb foo bar 2>&1 1>/dev/null)
  else
    err=$(ppz_a $verb foo 2>&1 1>/dev/null)
  fi
  rc=$?
  echo "rc=$rc"
  echo "$err" | grep -oE 'unknown key' | head -1
done
