package cli

import (
	"os"
	"runtime"
	"testing"

	"github.com/creack/pty"
)

// TestSessionID_DerivesFromInteractiveTty exposes a bug where sessionID()
// runs `tty` without inheriting our stdin, so the subprocess sees /dev/null
// (Go's exec.Command default) and reports "not a tty" — collapsing every
// interactive terminal window into the "default" session and defeating the
// per-session current-source isolation.
//
// The unit test the e2e harness already covers (PPZ_SESSION=tab-a vs tab-b)
// only exercises the env-var branch and silently passes despite the
// fallback path being broken. This test pins the actual behaviour: when
// stdin is a real pty and PPZ_SESSION is unset, sessionID MUST derive a
// per-tty id, not collapse to "default".
func TestSessionID_DerivesFromInteractiveTty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty.Open not supported on windows")
	}
	t.Setenv("PPZ_SESSION", "")

	ptmx, slave, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed (not available in this environment): %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = slave.Close()
	})

	// Replace os.Stdin with the pty slave so sessionID()'s `tty` subprocess
	// — once it correctly inherits stdin — sees a real terminal.
	oldStdin := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = oldStdin })

	got := sessionID()
	if got == "default" {
		t.Errorf("sessionID() = %q; expected a tty-derived id "+
			"(the `tty` subprocess isn't inheriting our stdin, "+
			"so it sees /dev/null and reports 'not a tty')", got)
	}
}

// TestDeriveSessionID_SidDistinguishesShells pins the recycled-tty fix:
// macOS hands new terminal windows tty paths that previous (now-closed)
// windows used. Without a per-shell discriminator, the new window
// inherits the previous window's "current" source.
//
// We use the Unix session id (getsid(0)) for the discriminator — it's
// stable across pipeline subshells and forks (`ppz X | grep ...` and
// `ppz X >/dev/null` agree on the session id, since both processes
// inherit the shell's session leader), but unique per terminal window
// because each shell start gets its own session leader. Earlier
// implementations used getppid() and broke pipelines: the subshell's
// fork has a fresh PID, so each pipe stage saw a different session id
// and current-source state didn't carry across them.
//
// Sid alone is sufficient as the discriminator — tty was previously
// folded in for "human-readable" session names but introduced a real
// bug (TestDeriveSessionID_PipedStdinSameSession). Now we use only
// sid when it's available; tty is the last-resort fallback.
func TestDeriveSessionID_SidDistinguishesShells(t *testing.T) {
	cases := []struct {
		name       string
		tty1, tty2 string
		sid1       int
		sid2       int
		want       string // "same" or "different"
	}{
		{"same tty + same sid → same session", "ttys036", "ttys036", 1000, 1000, "same"},
		{"same tty + different sid → different sessions", "ttys036", "ttys036", 1000, 2000, "different"},
		// Different tty + same sid: both processes are in the same kernel
		// session (sid is the authoritative shell-session marker). Treat
		// as one logical session; the tty is incidental.
		{"different tty + same sid → same session", "ttys036", "ttys037", 1000, 1000, "same"},
		{"no tty + same sid → same session", "", "", 1000, 1000, "same"},
		{"no tty + different sid → different sessions", "", "", 1000, 2000, "different"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := deriveSessionID(tc.tty1, tc.sid1)
			b := deriveSessionID(tc.tty2, tc.sid2)
			if tc.want == "same" && a != b {
				t.Errorf("expected same session id; got %q vs %q", a, b)
			}
			if tc.want == "different" && a == b {
				t.Errorf("expected different session ids; both got %q", a)
			}
			// Both must be non-empty — empty is a footgun (collapses to
			// "default" via normSession on the daemon side).
			if a == "" || b == "" {
				t.Errorf("session id must be non-empty; got %q / %q", a, b)
			}
		})
	}
}

// TestDeriveSessionID_PipedStdinSameSession pins the bug exposed when
// a user pipes into ppz from the same shell where they ran a non-piped
// command moments earlier. tty(1) reports "not a tty" when stdin is a
// pipe, so currentTty() returns "" — but getsid(0) is the same as the
// interactive case. The session id MUST be the same so the daemon's
// per-session current-source binding still applies:
//
//	$ ppz source switch foo            # tty=ttys036, sid=1000
//	$ echo X | ppz broadcast           # tty="",       sid=1000
//
// Both should resolve to the same session.
func TestDeriveSessionID_PipedStdinSameSession(t *testing.T) {
	interactive := deriveSessionID("ttys036", 1000)
	piped := deriveSessionID("", 1000)
	if interactive != piped {
		t.Errorf("piped invocation got a different session id than interactive: "+
			"interactive=%q piped=%q (the user ran them in the same shell, sid=%d)",
			interactive, piped, 1000)
	}
}
