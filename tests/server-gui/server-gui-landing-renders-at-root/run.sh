#!/usr/bin/env bash
# `/` is the OSS-server landing — a thin hero with the Pipes logo,
# tagline, and two CTAs (Login → /login, GitHub → the OSS repo). The
# marketing demos that used to live here have moved to pipescloud.io's
# site repo (pipes-internal). This test pins the slim contract so
# /dashboard never starts being served at / again, and so the page
# keeps the bare minimum a fresh visitor needs to find login.
. /tests/lib/common.sh

page="$(curl_server "/")"

echo "--- landing page identifier present (never serve org-list at /) ---"
printf '%s' "$page" | grep -oE -m 1 'data-page="landing"'

echo "--- tagline rendered ---"
printf '%s' "$page" | grep -oE -m 1 'connecting agents'

echo "--- logo asset referenced ---"
printf '%s' "$page" | grep -oE -m 1 'src="/assets/logo\.png"'

echo "--- Login CTA points at /login ---"
printf '%s' "$page" | grep -oE -m 1 'href="/login"'

echo "--- GitHub CTA points at the OSS repo ---"
printf '%s' "$page" | grep -oE -m 1 'href="https://github.com/pipescloud/ppz"'

echo "--- marketing demos no longer served from / ---"
# `grep -c` exits 1 when zero matches even though it prints 0; wrap so
# the run.sh harness doesn't treat the absence as a script-level failure.
printf '%s' "$page" | { grep -cE 'data-pair=' || true; } | tr -d ' '
