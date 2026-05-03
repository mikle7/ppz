#!/usr/bin/env bash
# `ppz version` does NOT require a running daemon. Useful for diagnosing
# "what binary am I running" before the daemon is up.
. /tests/lib/common.sh

# Point at a socket that intentionally doesn't exist.
PPZ_HOME=/tmp/no-such-home PPZ_IPC_SOCKET=/tmp/no-such-home/daemon.sock \
  ppz version
