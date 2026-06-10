// Package harness detects which agent harness (claude/codex/...) is
// running in a wrapped PTY and whether it is working or idle, so the
// terminal-share heartbeat can stamp live agent info instead of relying
// on launch-time env vars. Detection is pure and platform-free: the
// wrapper feeds in foreground-process snapshots and byte-activity
// timestamps; this package never touches the PTY itself.
//
// The design (and the timing constants) are ported from herdr's pane
// detection: identification is process-tree based, working/idle is PTY
// byte-causality based. See docs/specs/agent-detection.md.
package harness

import (
	"path/filepath"
	"strings"
)

// State of a wrapped agent harness, as stamped into heartbeat payloads.
type State string

const (
	StateUnknown State = ""        // no harness identified in the PTY foreground
	StateIdle    State = "idle"    // harness at rest, prompt waiting
	StateWorking State = "working" // harness actively producing output
	StateBlocked State = "blocked" // harness waiting on human input (phase 3)
)

// Spec describes one known harness. Extending detection to a new
// harness is one new row here. Names are the canonical ids `ppz agent
// --harness` already uses, so heartbeat consumers see one vocabulary
// regardless of whether the value came from env or detection.
type Spec struct {
	Name        string
	BinaryNames []string
}

// Specs returns the registry of known harnesses.
func Specs() []Spec {
	return []Spec{
		{Name: "claude", BinaryNames: []string{"claude", "claude-code"}},
		{Name: "codex", BinaryNames: []string{"codex"}},
		{Name: "copilot", BinaryNames: []string{"copilot", "github-copilot", "ghcs"}},
		{Name: "agy", BinaryNames: []string{"agy", "antigravity", "antigravity-cli"}},
		{Name: "pi", BinaryNames: []string{"pi"}},
	}
}

// genericRuntimes are interpreters/shells that frequently host a
// harness installed via npm/pip: the foreground comm is the runtime,
// the harness name only appears in argv. tmux is deliberately absent —
// it wraps everything and must never read as an agent.
var genericRuntimes = map[string]bool{
	"node": true, "bun": true,
	"python": true, "python3": true,
	"sh": true, "bash": true, "zsh": true, "fish": true,
	"pwsh": true, "powershell": true,
}

// Identify maps a foreground process (comm + full argv, argv[0]
// included) to a canonical harness name, or "" when the process is not
// a known harness. It normalizes names (case, .exe/.cmd/.bat/.ps1/.js
// suffixes, whitespace) and sees through generic runtimes (node,
// python, sh/bash/zsh, pwsh) by scanning argv for a known binary name.
func Identify(comm string, argv []string) string {
	if name := lookupBinary(comm); name != "" {
		return name
	}
	if !genericRuntimes[normalizeBinaryName(comm)] {
		return ""
	}
	if len(argv) < 2 {
		return ""
	}
	for _, arg := range argv[1:] {
		if name := lookupBinary(filepath.Base(arg)); name != "" {
			return name
		}
		// Shell -c / node -e style: the harness invocation hides inside
		// a single command-string token ("claude --resume abc").
		if strings.ContainsRune(arg, ' ') {
			for _, field := range strings.Fields(arg) {
				if name := lookupBinary(filepath.Base(field)); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

// normalizeBinaryName lowercases, trims, and strips the wrapper
// suffixes a harness ships under on various platforms — ported from
// herdr's normalized_agent_lookup_name.
func normalizeBinaryName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1", ".js"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// lookupBinary resolves a (possibly unnormalized) binary name to its
// canonical harness id, or "".
func lookupBinary(name string) string {
	name = normalizeBinaryName(name)
	if name == "" {
		return ""
	}
	for _, s := range Specs() {
		for _, bin := range s.BinaryNames {
			if name == bin {
				return s.Name
			}
		}
	}
	return ""
}
