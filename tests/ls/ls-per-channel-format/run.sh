#!/usr/bin/env bash
# `ppz ls` shows one line per (pipe, channel). Message pipes get one line
# (inbox); pty pipes get five (heartbeat / inbox / stdctrl / stdin /
# stdout, alphabetical). Each line:
#   <handle>.<channel>  <total>  <unread>  <last_at_or_dash>  <preview60_or_dash>
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create regular >/dev/null
ppz_a terminal share pty-pipe -- true >/dev/null

# Normalise the stdctrl + heartbeat JSON previews to stable placeholders
# so the test isn't dependent on the default cols/rows that no-tty wraps
# fall back to, nor on the heartbeat's volatile pid/hostname/started_at.
# Heartbeat JSON is longer than the preview cap and gets truncated with a
# trailing `…`, so it never ends in `}` — anchor the regex on the
# .heartbeat pipe column and consume non-whitespace through the next gap.
ppz_a ls \
  | ls_normalize \
  | sed -E 's| \{[^}]*"type":"resize"[^}]*\}| STDCTRL_JSON|' \
  | sed -E 's|^(.+\.heartbeat [0-9]+ [0-9]+ RELATIVE) \{[^ ]+|\1 HEARTBEAT_JSON|'
