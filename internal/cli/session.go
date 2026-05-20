package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// sessionID resolves the current "session" used to key per-shell daemon
// state (current source, cursor positions). Order:
//
//  1. $PPZ_SESSION  — explicit override, used by detached agents / scripts
//     that need a stable identity without a controlling tty.
//  2. tty(1) + Unix session id  — interactive default. tty alone isn't
//     enough because macOS recycles tty paths across windows: open a new
//     window, the OS hands you the same /dev/ttys036 a previous closed
//     window used. The Unix session id (getsid(0)) is stable across
//     pipeline subshells and forks but unique per terminal window
//     (each shell launch gets its own session leader), so it
//     distinguishes recycled-tty cases without breaking pipelines —
//     `ppz X | grep ...` and `ppz X >/dev/null` agree on the id.
//  3. session id alone  — non-interactive fallback (no tty). Keeps each
//     shell process / script invocation its own session.
//  4. "default"  — last-resort fallback if Getsid fails (it shouldn't
//     on any supported platform).
func sessionID() string {
	if v := os.Getenv("PPZ_SESSION"); v != "" {
		return v
	}
	// stdlib's syscall.Getsid is darwin-only; x/sys/unix has it on every
	// supported posix.
	sid, err := unix.Getsid(0)
	if err != nil {
		sid = -1
	}
	return deriveSessionID(currentTty(), sid)
}

// sessionFromEnv returns the PPZ_SESSION env value, or empty if unset.
// Used by callers that want to let the new daemon-side resolver
// (Layer 1, docs/specs/session-binding.md) bind via the process
// ancestor chain rather than committing to a locally-computed
// session id. Old daemons see the empty Session field, fall back to
// "default" — acceptable degradation for the rollout window.
func sessionFromEnv() string {
	return os.Getenv("PPZ_SESSION")
}

// currentTty returns the path of the calling tty (with `/dev/` stripped
// and `/` replaced with `-`), or "" if stdin isn't connected to a tty.
//
// `tty(1)` reads its stdin to identify the controlling terminal — without
// explicitly passing our stdin through, Go's exec.Command attaches
// /dev/null and `tty` reports "not a tty", silently collapsing every
// interactive terminal window into the same session id.
func currentTty() string {
	cmd := exec.Command("tty")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" || s == "not a tty" {
		return ""
	}
	s = strings.TrimPrefix(s, "/dev/")
	s = strings.ReplaceAll(s, string(filepath.Separator), "-")
	return s
}

// deriveSessionID composes the session id from a tty path and a Unix
// session id (getsid(0)). Pure function so the logic is unit-testable
// without spawning real shells or ptys.
//
// sid alone is the authoritative key when available: stable per-shell
// (each shell window is its own session leader so each has a unique
// sid) AND consistent across pipeline subshells, redirects, and piped
// stdin (sid is inherited through forks; tty isn't — `echo X | ppz Y`
// has no controlling tty inside the right-hand process even though
// it's the same shell session).
//
// Earlier versions composed `<tty>-<sid>` for "human readable" session
// names but that diverged the key for `ppz cmd` (interactive) vs
// `echo … | ppz cmd` (piped) in the same shell, breaking per-session
// state lookups. tty is now only used as the last-resort fallback
// when sid is unavailable (Getsid syscall failure — practically
// never on supported posix platforms).
func deriveSessionID(tty string, sid int) string {
	if sid > 0 {
		return fmt.Sprintf("sid-%d", sid)
	}
	if tty != "" {
		return tty
	}
	return "default"
}
