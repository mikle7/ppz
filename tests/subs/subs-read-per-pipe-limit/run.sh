#!/usr/bin/env bash
# Flood protection for the agent drain loop: `ppz subs read` applies the
# same head-N cap as `ppz read`, PER PIPE (default 10, -l N override,
# -l 0 = unbounded). A spammy pipe can no longer dominate the output —
# it yields at 10 with a "N more unread" trailer inside its banner
# block, and the other pipes still get their turn in the same run.
#
# Flow:
#   1. bar.inbox gets 12 messages (the spammer), foo.inbox gets 2.
#   2. subs read: bar block = bar-01..bar-10 + "2 more unread" trailer,
#      foo block = both messages, no trailer.
#   3. subs read -l 0: drains bar's remaining 2 (foo now empty → no
#      banner, pinned by the separator listing).
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
export PPZ_SESSION=mysh
ppz_a source create foo >/dev/null
ppz_a source create bar >/dev/null
ppz_a subs add foo.inbox bar.inbox >/dev/null
for i in $(seq -w 1 12); do
  ppz_a send bar.inbox "bar-$i" >/dev/null
done
ppz_a send foo.inbox "foo-1" >/dev/null
ppz_a send foo.inbox "foo-2" >/dev/null
wait_for 20 "ppz_a subs ls | grep -q foo-2" >/dev/null
wait_for 20 "ppz_a subs ls | grep -q bar-12" >/dev/null

OUT=$(mktemp)
ppz_a subs read >"$OUT" 2>/dev/null
echo "--- separators: both pipes visited ---"
grep '^=== ' "$OUT"
echo "--- bar delivers only its next 10 ---"
grep -oE 'bar-[0-9]+' "$OUT"
echo "--- bar trailer names the remainder ---"
grep -oE '[0-9]+ more unread' "$OUT"
echo "--- foo delivers everything, uncapped ---"
grep -oE 'foo-[0-9]+' "$OUT"

ppz_a subs read -l 0 >"$OUT" 2>/dev/null
echo "--- second pass -l 0: only bar has unread ---"
grep '^=== ' "$OUT"
grep -oE 'bar-[0-9]+' "$OUT"
echo "--- no trailer once drained ---"
grep -c 'more unread' "$OUT" || true
