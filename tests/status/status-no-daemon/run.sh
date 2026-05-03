#!/usr/bin/env bash
. /tests/lib/common.sh
# Point at a socket path that does not exist; the CLI must report
# E_DAEMON_NOT_RUNNING (exit 11) and print the single 'daemon: not running'
# line on stdout.
PPZ_IPC_SOCKET=/tmp/does-not-exist/daemon.sock ppz status
