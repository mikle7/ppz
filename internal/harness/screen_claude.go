package harness

// claudeScreen recognizes Claude Code's blocked states from visible
// screen text. Patterns are ported from herdr's
// src/detect/agents/claude_code.rs, with the same precedence:
//
//  1. A live blocked form (the "Enter to select · … · Esc to cancel"
//     selector chrome) is the strongest signal — blocked.
//  2. The dynamic-workflow prompt ("run a dynamic workflow?" + "esc to
//     cancel") — blocked.
//  3. Working chrome ("esc to interrupt" / "ctrl+c to interrupt")
//     vetoes everything below — a long-running tool can sit quietly
//     with stale dialog text still on screen.
//  4. Confirmation prompts ("Do you want to proceed?", permission
//     waits, plan-interview chrome) — blocked, but only when no live
//     empty input prompt box is visible: an idle prompt box means any
//     dialog text above it is history (or a vt10x ghost cell), not a
//     live question.
type claudeScreen struct{}

func (claudeScreen) Blocked(content string) bool {
	return false // RED skeleton — implemented after test review
}
