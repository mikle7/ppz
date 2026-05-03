#!/usr/bin/env bash
# Server serves a stylesheet at /assets/style.css and the GUI pages
# reference it. Locks the contract that the GUI isn't shipping default
# browser styling.
. /tests/lib/common.sh

status=$(curl -sS -o /dev/null -w '%{http_code}' "$PPZ_SERVER_URL/assets/style.css")
ctype=$(curl -sS -I "$PPZ_SERVER_URL/assets/style.css" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-type"{print tolower($2)}')
size=$(curl -sS "$PPZ_SERVER_URL/assets/style.css" | wc -c | tr -d ' ')
echo "css-status=$status"
echo "css-content-type-prefix=${ctype%%;*}"
[ "$size" -gt 0 ] && echo "css-nonempty=yes" || echo "css-nonempty=no"

if curl -sS "$PPZ_SERVER_URL/" | grep -qE '<link[^>]+href="/assets/style\.css"'; then
  echo "index-references-css=yes"
else
  echo "index-references-css=no"
fi
