#!/usr/bin/env bash
# read requires a logged-in daemon → E_NOT_LOGGED_IN.
. /tests/lib/common.sh

ppz_a read foo.broadcast
