package cli

import "testing"

// The one bit of real logic in `terminal attach`: split a raw stdin read at
// the Ctrl-\ detach byte, forwarding the prefix and signalling teardown.
func TestAttachStdinChunk(t *testing.T) {
	cc := string(rune(attachDetachByte)) // Ctrl-\ (0x1c)
	cases := []struct {
		name     string
		in       string
		wantFwd  string
		wantStop bool
	}{
		{"plain bytes forwarded whole", "ls -la\n", "ls -la\n", false},
		{"ctrl-c passes through (not detach)", "\x03", "\x03", false},
		{"detach at end forwards prefix", "exit\n" + cc, "exit\n", true},
		{"lone detach forwards nothing", cc, "", true},
		{"detach mid-chunk drops the tail", "ab" + cc + "cd", "ab", true},
		{"empty read", "", "", false},
	}
	for _, c := range cases {
		fwd, stop := attachStdinChunk([]byte(c.in))
		if string(fwd) != c.wantFwd || stop != c.wantStop {
			t.Errorf("%s: attachStdinChunk(%q) = %q,%v; want %q,%v",
				c.name, c.in, fwd, stop, c.wantFwd, c.wantStop)
		}
	}
}

// Guards the self-attach footgun (wren's repro): attaching to your own handle
// loops stdout into stdin. The empty-self case is the important edge — a bare
// shell with no PPZ_SESSION must NOT be blocked from a legitimate attach.
func TestIsSelfAttach(t *testing.T) {
	cases := []struct {
		target, self string
		want         bool
	}{
		{"wren", "wren", true},  // attaching to self
		{"wren", "chud", false}, // different agent — fine
		{"wren", "", false},     // unknown self — never block
		{"", "", false},         // both empty
	}
	for _, c := range cases {
		if got := isSelfAttach(c.target, c.self); got != c.want {
			t.Errorf("isSelfAttach(%q, %q) = %v; want %v", c.target, c.self, got, c.want)
		}
	}
}
