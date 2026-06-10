package harness

// ScreenDetector is the phase-3 extension point: a per-harness matcher
// over the visible terminal screen that recognizes "waiting on human
// input" (permission dialogs, forms, choice prompts). It is consulted
// only when byte causality already says the harness is not working —
// PTY activity remains the authority for working, the screen only
// arbitrates idle vs blocked (same division of authority as herdr).
type ScreenDetector interface {
	// Blocked reports whether the visible screen content (the bottom
	// lines of the live screen model) shows the harness waiting on
	// human input.
	Blocked(content string) bool
}

// ScreenDetectorFor returns the screen detector for a canonical
// harness name, or nil when that harness has no screen patterns yet —
// nil simply means the harness never reports blocked. Phase 3 ships
// Claude Code first; adding another harness is one case here plus one
// screen_<name>.go pattern file (herdr's per-agent module layout).
func ScreenDetectorFor(name string) ScreenDetector {
	return nil // RED skeleton — implemented after test review
}
