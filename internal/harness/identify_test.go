package harness

import "testing"

// Identify maps foreground processes to the canonical harness ids that
// `ppz agent --harness` already uses, so env-stamped and detected
// heartbeats speak one vocabulary. Direct binary names, including the
// aliases each harness ships under.
func TestIdentify_DirectBinaryNames(t *testing.T) {
	cases := []struct {
		comm string
		want string
	}{
		{"claude", "claude"},
		{"claude-code", "claude"},
		{"codex", "codex"},
		{"copilot", "copilot"},
		{"github-copilot", "copilot"},
		{"ghcs", "copilot"},
		{"agy", "agy"},
		{"antigravity", "agy"},
		{"pi", "pi"},
	}
	for _, c := range cases {
		if got := Identify(c.comm, []string{c.comm}); got != c.want {
			t.Errorf("Identify(%q) = %q, want %q", c.comm, got, c.want)
		}
	}
}

// Process names arrive in whatever shape the platform reports: mixed
// case, Windows-style suffixes (a harness installed via npm shows up as
// claude.js / claude.cmd under some shells), stray whitespace. All
// normalize to the same id — ported from herdr's
// normalized_agent_lookup_name.
func TestIdentify_NormalizesNames(t *testing.T) {
	cases := []struct {
		comm string
		want string
	}{
		{"CLAUDE", "claude"},
		{"claude.exe", "claude"},
		{"claude.js", "claude"},
		{"claude.cmd", "claude"},
		{" claude ", "claude"},
		{"Codex.EXE", "codex"},
	}
	for _, c := range cases {
		if got := Identify(c.comm, []string{c.comm}); got != c.want {
			t.Errorf("Identify(%q) = %q, want %q", c.comm, got, c.want)
		}
	}
}

// Harnesses installed via npm/pip run under a generic runtime — the
// foreground comm is "node" or "python3", and the harness name only
// appears in argv. Identify sees through the runtime by scanning argv
// basenames (and shell -c command strings) for a known binary name.
func TestIdentify_WrappedRuntimes(t *testing.T) {
	cases := []struct {
		name string
		comm string
		argv []string
		want string
	}{
		{"node script path", "node", []string{"/usr/local/bin/node", "/Users/x/.npm-global/bin/claude"}, "claude"},
		{"node script with .js suffix", "node", []string{"node", "/opt/claude.js", "--resume"}, "claude"},
		{"bash -c command string", "bash", []string{"bash", "-c", "claude --resume abc123"}, "claude"},
		{"zsh -c command string", "zsh", []string{"zsh", "-c", "codex"}, "codex"},
		{"python module", "python3", []string{"python3", "/home/x/.local/bin/pi"}, "pi"},
	}
	for _, c := range cases {
		if got := Identify(c.comm, c.argv); got != c.want {
			t.Errorf("%s: Identify(%q, %v) = %q, want %q", c.name, c.comm, c.argv, got, c.want)
		}
	}
}

// Anything that isn't a known harness — editors, multiplexers, bare
// shells, runtimes running unrelated scripts — must return "" so the
// heartbeat falls back to the env var (or stays blank). tmux in
// particular wraps everything and must never read as an agent.
func TestIdentify_NonHarnessReturnsEmpty(t *testing.T) {
	cases := []struct {
		name string
		comm string
		argv []string
	}{
		{"editor", "vim", []string{"vim", "main.go"}},
		{"multiplexer", "tmux", []string{"tmux", "attach"}},
		{"interactive shell", "zsh", []string{"zsh"}},
		{"node running unrelated script", "node", []string{"node", "server.js"}},
		{"bash -c unrelated", "bash", []string{"bash", "-c", "make test"}},
		{"empty comm", "", nil},
	}
	for _, c := range cases {
		if got := Identify(c.comm, c.argv); got != "" {
			t.Errorf("%s: Identify(%q, %v) = %q, want \"\"", c.name, c.comm, c.argv, got)
		}
	}
}

// The registry is the extension point: every spec row must carry a
// canonical name and at least one binary name, and every binary name
// must round-trip through Identify back to its own row. Adding a new
// harness with a typo'd alias fails here, not in production.
func TestSpecs_BinaryNamesRoundTrip(t *testing.T) {
	specs := Specs()
	if len(specs) == 0 {
		t.Fatalf("Specs() is empty; want at least the five ppz agent harnesses")
	}
	for _, s := range specs {
		if s.Name == "" || len(s.BinaryNames) == 0 {
			t.Errorf("spec %+v missing name or binary names", s)
		}
		for _, bin := range s.BinaryNames {
			if got := Identify(bin, []string{bin}); got != s.Name {
				t.Errorf("Identify(%q) = %q, want %q (registry row must round-trip)", bin, got, s.Name)
			}
		}
	}
}
