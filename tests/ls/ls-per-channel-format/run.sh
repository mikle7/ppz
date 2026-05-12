#!/usr/bin/env bash
# `ppz ls` shows one line per (pipe, channel). Message pipes get one line
# (broadcast); pty pipes get four (broadcast / stdctrl / stdin / stdout,
# alphabetical). Each line:
#   <handle>.<channel>  <total>  <unread>  <last_at_or_dash>  <preview60_or_dash>
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create regular >/dev/null
ppz_a terminal share pty-pipe -- true >/dev/null

# Normalise the stdctrl JSON preview to a placeholder so the test isn't
# dependent on the default cols/rows that no-tty wraps fall back to.
ppz_a ls \
  | ls_normalize \
  | sed -E 's| \{[^}]*"type":"resize"[^}]*\}| STDCTRL_JSON|'
