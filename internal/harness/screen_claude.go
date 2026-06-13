package harness

import (
	"strings"
	"unicode"
)

// claudeScreen recognizes Claude Code's blocked states from visible
// screen text. The pattern vocabulary (which UI strings mean what) is
// a fact of Claude Code's UI; the precedence is what keeps it honest:
//
//  1. Live selector-chrome form footer ("Enter to select · … · Esc to
//     cancel") — the strongest signal, blocked.
//  2. The dynamic-workflow prompt — blocked.
//  3. Working chrome ("esc to interrupt" / "ctrl+c to interrupt")
//     vetoes everything below: a long-running tool can sit quietly
//     with stale dialog text still in view.
//  4. Strong blocker phrases (permission waits, plan-interview
//     screens) — blocked.
//  5. A live input prompt box (a ❯ line that isn't a numbered
//     selector) vetoes confirmation matching: when the idle prompt is
//     accepting input, any question text above it is history — or a
//     vt10x ghost cell — not a live question.
//  6. Confirmation dialogs ("Do you want to …?" with a yes/❯ choice).
type claudeScreen struct{}

func (claudeScreen) Blocked(content string) bool {
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)

	if claudeLiveBlockedForm(lower) {
		return true
	}
	if strings.Contains(lower, "run a dynamic workflow?") && strings.Contains(lower, "esc to cancel") {
		return true
	}
	if strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "ctrl+c to interrupt") {
		return false
	}
	for _, phrase := range []string{
		"waiting for permission",
		"do you want to allow this connection?",
		"review your answers",
		"skip interview and plan immediately",
		"tab to amend",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	if claudeLiveInputPromptBox(content) {
		return false
	}
	return claudeConfirmationPrompt(lower)
}

// claudeLiveBlockedForm matches the form footer Claude renders under
// live AskUserQuestion / interview widgets: one line carrying select +
// cancel + navigate chrome together.
func claudeLiveBlockedForm(lower string) bool {
	for _, line := range strings.Split(lower, "\n") {
		if !strings.Contains(line, "enter to select") || !strings.Contains(line, "esc to cancel") {
			continue
		}
		for _, nav := range []string{
			"tab/arrow keys to navigate",
			"arrow keys to navigate",
			"arrows to navigate",
			"↑/↓ to navigate",
			"↑↓ to navigate",
		} {
			if strings.Contains(line, nav) {
				return true
			}
		}
	}
	return false
}

// claudeLiveInputPromptBox reports a ❯ line that is the input prompt
// (empty or with typed text) rather than a selection row: numbered
// options ("❯ 1. Yes") and selector chrome don't count.
func claudeLiveInputPromptBox(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimFunc(line, func(r rune) bool {
			return r == '│' || unicode.IsSpace(r)
		})
		rest, ok := strings.CutPrefix(trimmed, "❯")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if claudeSelectorRow(rest) {
			continue
		}
		return true
	}
	return false
}

// claudeSelectorRow reports option-row content after a ❯ marker:
// "1. Yes"-style numbered choices.
func claudeSelectorRow(rest string) bool {
	num, _, found := strings.Cut(rest, ".")
	if !found {
		return false
	}
	num = strings.TrimSpace(num)
	if num == "" {
		return false
	}
	for _, r := range num {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// claudeConfirmationPrompt matches the permission-dialog family: a
// "do you want to …" / "would you like to …" question followed by a
// yes option or a ❯ selection.
func claudeConfirmationPrompt(lower string) bool {
	pos := strings.Index(lower, "do you want to")
	if pos < 0 {
		pos = strings.Index(lower, "would you like to")
	}
	if pos < 0 {
		return false
	}
	after := lower[pos:]
	return strings.Contains(after, "yes") || strings.Contains(after, "❯")
}
