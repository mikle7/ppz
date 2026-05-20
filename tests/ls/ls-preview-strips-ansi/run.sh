#!/usr/bin/env bash
# Reproduces: a wrapped terminal's last .stdout chunk often contains ANSI
# escapes (cursor positioning, colour, the shell prompt itself, the zsh
# "%" end-of-line marker). The preview field in `ppz ls` was emitting
# those bytes verbatim, so running ls in any real terminal could clear
# the screen, set bold, change colours, or just produce visually
# mangled output mid-listing.
#
# Preview must be plain printable text only: full ANSI CSI sequences
# (ESC `[` … final-byte) are stripped, as are bare ESC bytes and all
# other C0 control bytes (< 0x20 except space) plus DEL (0x7F).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# printf renders a clear-screen + bold + "msg" + reset-bold sequence.
ppz_a terminal share term -- printf '\033[2J\033[1mmsg\033[0m' >/dev/null

wait_for 20 "ppz_a ls 2>/dev/null | ls_normalize | grep -E '^term\.stdout 1 ' >/dev/null" >/dev/null

# Print just the stdout row. cat -v makes any leaking control bytes
# visible as ^[, M-, etc. — clean preview round-trips unchanged.
ppz_a ls | ls_normalize | grep '^term\.stdout' | cat -v
