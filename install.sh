#!/usr/bin/env bash
set -euo pipefail

# Temporary ppz install script.
#
# The "real" distribution path (goreleaser → pre-built binaries on
# GitHub Releases → Homebrew tap) is in flight; until it lands, this
# script falls back to `go install` from source. It's intentionally
# tiny and side-effect-free — readable from the homepage's
# curl-pipe-bash one-liner without anything sneaky going on.
#
# Replace this with a proper installer once distribution v1 lands;
# the curl-pipe-bash URL on pipescloud.io stays the same so the user
# experience doesn't churn.

require_go() {
	if command -v go >/dev/null 2>&1; then
		return 0
	fi
	cat >&2 <<'EOF'
ppz install: Go (>= 1.22) is required for the moment.

Install Go via your package manager or https://go.dev/dl/, then re-run:

    curl -fsSL https://raw.githubusercontent.com/pipescloud/ppz/main/install.sh | bash

A pre-built binary distribution (no Go required) is on the roadmap —
track progress at https://github.com/pipescloud/ppz.
EOF
	exit 1
}

main() {
	require_go

	echo "Installing ppz via 'go install' (compiles from source — may take a minute)…"
	go install github.com/pipescloud/ppz/cmd/ppz@latest

	# `go install` drops the binary into $(go env GOBIN), falling back
	# to $(go env GOPATH)/bin. Either is a valid PATH location on a
	# normal Go setup.
	dest="$(go env GOBIN || true)"
	if [ -z "$dest" ]; then
		dest="$(go env GOPATH)/bin"
	fi

	echo
	echo "Installed: $dest/ppz"
	case ":$PATH:" in
		*:"$dest":*)
			echo "Verify: ppz version"
			;;
		*)
			echo
			echo "Heads-up: $dest is not on \$PATH. Add it to your shell rc:"
			echo "    export PATH=\"$dest:\$PATH\""
			echo
			echo "Then verify: ppz version"
			;;
	esac
}

main "$@"
