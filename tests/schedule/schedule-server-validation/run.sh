#!/usr/bin/env bash
# RED — PR #139 review finding #1 (+ skew grace). The daemon always
# sends resolved targets, but POST /api/v1/schedules is the trust
# boundary: a bearer hitting the route directly must not be able to
# store a handle/manifold that would build a malformed or wildcard
# NATS subject at fire time (which the old loop then re-leased and
# retried every 30s forever).
#
# Also pins the skew grace: `--at` is validated strictly-future on the
# CLI's clock, so by the time the request lands server-side it may be
# slightly past (network latency / clock skew) — the server accepts up
# to 30s past and the schedule fires on the next tick.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a source create bob >/dev/null

resp=$(mktemp)
post_schedule() {
  curl -sS -o "$resp" -w '%{http_code}' \
    -X POST "$PPZ_SERVER_URL/api/v1/schedules" \
    -H "Authorization: Bearer $(key_alpha)" \
    -H "Content-Type: application/json" \
    -d "$1"
}

code=$(post_schedule '{"manifold":"","handle":"bad.handle","pipe":"inbox","payload":"x","sender":"","kind":"at","at":"2999-01-01T00:00:00Z"}')
echo "bad-handle=$code $(grep -oE 'E_[A-Z_]+' $resp | head -1)"

code=$(post_schedule '{"manifold":"","handle":"wild*card","pipe":"inbox","payload":"x","sender":"","kind":"at","at":"2999-01-01T00:00:00Z"}')
echo "wildcard-handle=$code $(grep -oE 'E_[A-Z_]+' $resp | head -1)"

code=$(post_schedule '{"manifold":"ok.BAD","handle":"","pipe":"room","payload":"x","sender":"","kind":"at","at":"2999-01-01T00:00:00Z"}')
echo "bad-manifold=$code $(grep -oE 'E_[A-Z_]+' $resp | head -1)"

# BusyBox date (test-runner image) has no relative -d; use epoch math.
past=$(date -u -d "@$(( $(date +%s) - 120 ))" +%FT%TZ)
code=$(post_schedule "{\"manifold\":\"\",\"handle\":\"bob\",\"pipe\":\"inbox\",\"payload\":\"x\",\"sender\":\"\",\"kind\":\"at\",\"at\":\"$past\"}")
echo "beyond-grace=$code $(grep -oE 'E_[A-Z_]+' $resp | head -1)"

# Nothing above may have stored a row.
echo "rows-after-rejects=$(ppz_a schedule ls | wc -l | tr -d ' ')"

# Within the 30s skew grace: accepted, fires on the next tick.
skew=$(date -u -d "@$(( $(date +%s) - 10 ))" +%FT%TZ)
code=$(post_schedule "{\"manifold\":\"\",\"handle\":\"bob\",\"pipe\":\"inbox\",\"payload\":\"skew ping\",\"sender\":\"alice\",\"kind\":\"at\",\"at\":\"$skew\"}")
echo "within-grace=$code"
wait_for 100 'ppz_a reread bob.inbox -l 1 --bare 2>/dev/null | grep -q "skew ping"'
echo "skew-fired=$?"

rm -f $resp
