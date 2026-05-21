#!/usr/bin/env bash
# `ppz agent create --codex` must invoke the codex binary with
# --dangerously-bypass-approvals-and-sandbox prepended.
#
# Why: codex defaults to its seatbelt sandbox (CODEX_SANDBOX=seatbelt),
# which prevents the agent from reaching the running ppz daemon — `ppz
# status` returns `daemon: not running` from inside the sandbox even
# though the daemon is healthy on the host. Bypassing the sandbox is
# the codex analogue of `--dangerously-skip-permissions` already baked
# into the claude harness (see buildAgentArgv).
#
# The test stubs `codex` on PATH so it records its argv to a file and
# exits immediately, then runs `ppz agent create --codex` and greps the
# recorded argv for the flag. RED: current buildAgentArgv emits
# `[codex <prompt>]` with no sandbox flag, so the grep misses.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null

# Stub codex on PATH: write argv, exit. PATH is exported so ppz's
# in-process exec.Command sees the stub when it LookPaths "codex".
stub_dir=$(mktemp -d)
argv_log="$stub_dir/codex-argv.log"
cat > "$stub_dir/codex" <<EOF
#!/bin/sh
printf '%s\n' "\$@" > "$argv_log"
exit 0
EOF
chmod +x "$stub_dir/codex"
export PATH="$stub_dir:$PATH"

# Drive the codepath under test. The positional prompt keeps the stub
# argv compact (no embedded default orientation prompt). Discard
# stdout/stderr — the pty wrapper prints its own banner that we don't
# want to diff.
ppz_a agent create test-codex --codex "hi" >/dev/null 2>&1 || true

if [ -f "$argv_log" ] && grep -q -- '--dangerously-bypass-approvals-and-sandbox' "$argv_log"; then
  echo "OK: codex invoked with --dangerously-bypass-approvals-and-sandbox"
else
  echo "FAIL: codex was not invoked with --dangerously-bypass-approvals-and-sandbox"
fi
