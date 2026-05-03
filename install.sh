#!/usr/bin/env bash
set -euo pipefail

# ppz install script.
#
# Detects the local OS + arch, downloads the matching pre-built tarball
# from the latest GitHub Release of pipescloud/ppz, verifies the
# sha256 against checksums.txt, and drops the bundled binaries into a
# user-writable PATH location. No Go toolchain required.
#
# Configurable:
#   PPZ_INSTALL_DIR   target dir (default: $HOME/.local/bin)
#   PPZ_VERSION       tag to pin (default: latest release; e.g. v0.16.0)

REPO="pipescloud/ppz"
INSTALL_DIR="${PPZ_INSTALL_DIR:-$HOME/.local/bin}"
PIN_VERSION="${PPZ_VERSION:-}"
BINARIES=(ppz ppz-server ppz-desktop ppz-seed ppz-natsbootstrap)

msg() { printf '%s\n' "$*" >&2; }
die() { msg "ppz install: $*"; exit 1; }

detect_target() {
	local os arch
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	case "$os" in
		darwin | linux) ;;
		*) die "unsupported OS: $os (need darwin or linux)" ;;
	esac
	arch=$(uname -m)
	case "$arch" in
		x86_64 | amd64) arch=amd64 ;;
		arm64 | aarch64) arch=arm64 ;;
		*) die "unsupported arch: $arch (need amd64 or arm64)" ;;
	esac
	echo "$os $arch"
}

# Resolve the GitHub release tag we'll pull. Honors PPZ_VERSION; falls
# back to the latest release via the public GitHub API.
resolve_tag() {
	if [ -n "$PIN_VERSION" ]; then
		echo "$PIN_VERSION"
		return
	fi
	local tag
	tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
		| sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
	[ -n "$tag" ] || die "could not resolve latest release tag (rate-limited or no releases yet?)"
	echo "$tag"
}

sha256_of() {
	# macOS ships shasum; most Linux ships sha256sum. Either works.
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "neither sha256sum nor shasum found — can't verify download"
	fi
}

main() {
	read -r OS ARCH < <(detect_target)
	TAG=$(resolve_tag)
	VERSION="${TAG#v}"
	TARBALL="ppz_${VERSION}_${OS}_${ARCH}.tar.gz"
	BASE="https://github.com/${REPO}/releases/download/${TAG}"

	msg "Installing ppz ${TAG} (${OS}/${ARCH}) to ${INSTALL_DIR}..."

	mkdir -p "$INSTALL_DIR" || die "cannot create $INSTALL_DIR"

	TMP=$(mktemp -d)
	trap 'rm -rf "$TMP"' EXIT

	curl -fsSL "$BASE/$TARBALL"      -o "$TMP/$TARBALL"      \
		|| die "download failed: $BASE/$TARBALL"
	curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt" \
		|| die "download failed: $BASE/checksums.txt"

	expected=$(awk -v f="$TARBALL" '$2 == f { print $1 }' "$TMP/checksums.txt")
	[ -n "$expected" ] || die "no checksum entry for $TARBALL in checksums.txt"
	actual=$(sha256_of "$TMP/$TARBALL")
	[ "$expected" = "$actual" ] || die "checksum mismatch (expected $expected, got $actual)"

	tar -xzf "$TMP/$TARBALL" -C "$TMP"

	installed=()
	for b in "${BINARIES[@]}"; do
		if [ -f "$TMP/$b" ]; then
			install -m 0755 "$TMP/$b" "$INSTALL_DIR/$b"
			installed+=("$b")
		fi
	done
	[ "${#installed[@]}" -gt 0 ] || die "tarball contained no expected binaries"

	msg ""
	msg "Installed ${#installed[@]} binaries: ${installed[*]}"
	msg "  → ${INSTALL_DIR}/"
	case ":$PATH:" in
		*:"$INSTALL_DIR":*)
			msg ""
			msg "Verify:  ppz version"
			;;
		*)
			msg ""
			msg "Heads-up: ${INSTALL_DIR} is not on \$PATH. Add to your shell rc:"
			msg "    export PATH=\"${INSTALL_DIR}:\$PATH\""
			msg ""
			msg "Then verify:  ppz version"
			;;
	esac
}

main "$@"
