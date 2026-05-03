#!/usr/bin/env bash
# `ppz version` prints "ppz <version> (<sha>)" and exits 0. The exact
# version + sha are build-time, normalized to "VERSION (SHA)" by
# tests/lib/normalize.sh so dev / tagged / dirty builds all diff
# against the same expected.txt.
. /tests/lib/common.sh
ppz_a version
