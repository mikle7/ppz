package harness

import "testing"

// Realistic Claude Code screen excerpts (bottom-of-screen region, as
// the wrapper's live screen model would hand to the detector). The
// pattern contracts are ported from herdr's claude_code.rs; ground
// truth against captured .bin fixtures lands when we have real blocked
// captures (see spec).

// Blocked: the bash permission dialog — confirmation question plus a
// selected-choice list.
const claudeBashPermission = `╭──────────────────────────────────────────────────────────╮
│ Bash command                                              │
│                                                           │
│   rm -rf build/                                           │
│   Remove stale build artifacts                            │
│                                                           │
│ Do you want to proceed?                                   │
│ ❯ 1. Yes                                                  │
│   2. Yes, and don't ask again for rm commands             │
│   3. No, and tell Claude what to do differently (esc)     │
╰──────────────────────────────────────────────────────────╯`

// Blocked: edit-approval dialog (different verb, same family).
const claudeEditPermission = `│ Edit file                                                 │
│                                                           │
│ Do you want to make this edit to terminal.go?             │
│ ❯ 1. Yes                                                  │
│   2. Yes, allow all edits during this session (shift+tab) │
│   3. No, and tell Claude what to do differently (esc)     │`

// Blocked: AskUserQuestion / interview form with live selector chrome.
const claudeQuestionForm = `│ Which approach should we take?                            │
│                                                           │
│ ❯ 1. Port herdr patterns                                  │
│   2. Write from scratch                                   │
│                                                           │
│ Enter to select · Tab/Arrow keys to navigate · Esc to cancel`

// Blocked: permission wait (MCP / connection flavors).
const claudePermissionWait = `  Waiting for permission…

  Do you want to allow this connection?`

// Blocked: plan-mode interview review screen.
const claudeReviewAnswers = `│ Review your answers                                       │
│                                                           │
│ ❯ Approach: port herdr patterns                           │
│   Scope: claude first                                     │`

// Not blocked: the idle prompt — empty input box, shortcut hint.
const claudeIdlePrompt = `╭──────────────────────────────────────────────────────────╮
│ ❯                                                         │
╰──────────────────────────────────────────────────────────╯
  ? for shortcuts`

// Not blocked: working — spinner line with interrupt hint.
const claudeWorkingSpinner = `✶ Reticulating… (esc to interrupt · 32s · ↓ 1.2k tokens)

╭──────────────────────────────────────────────────────────╮
│ ❯                                                         │
╰──────────────────────────────────────────────────────────╯`

// Not blocked: a long-quiet tool run — interrupt hint present, stale
// dialog text from an earlier (answered) permission still in view.
// Working chrome must veto the dialog text.
const claudeWorkingWithStaleDialog = `│ Do you want to proceed?
│ ❯ 1. Yes

⏺ Bash(make e2e)
  ⎿  Running… (ctrl+c to interrupt)`

// Not blocked: an answered dialog scrolled into history (or left as
// vt10x ghost cells) above a live empty input prompt box. The live
// idle prompt means any question text above it is not a live question.
const claudeAnsweredDialogAbovePrompt = `⏺ User approved Claude's plan:
  ⎿  earlier output mentioning "Do you want to proceed?"

╭──────────────────────────────────────────────────────────╮
│ ❯                                                         │
╰──────────────────────────────────────────────────────────╯
  ? for shortcuts`

func TestClaudeScreen_Blocked(t *testing.T) {
	det := ScreenDetectorFor("claude")
	if det == nil {
		t.Fatal("claude must have a screen detector in phase 3")
	}

	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"bash permission dialog", claudeBashPermission, true},
		{"edit permission dialog", claudeEditPermission, true},
		{"question form with selector chrome", claudeQuestionForm, true},
		{"permission wait", claudePermissionWait, true},
		{"review answers", claudeReviewAnswers, true},

		{"idle prompt", claudeIdlePrompt, false},
		{"working spinner", claudeWorkingSpinner, false},
		{"working chrome vetoes stale dialog", claudeWorkingWithStaleDialog, false},
		{"answered dialog above live prompt", claudeAnsweredDialogAbovePrompt, false},
		{"empty screen", "", false},
	}
	for _, c := range cases {
		if got := det.Blocked(c.content); got != c.want {
			t.Errorf("%s: Blocked = %v, want %v", c.name, got, c.want)
		}
	}
}

// Phase 3 is Claude-first: the registry hands back a detector for
// claude and nil for everything else. nil is the documented "never
// blocked" contract, so detector arbitration must tolerate it. This
// pin changes deliberately when another harness gains patterns.
func TestScreenDetectorFor_ClaudeFirst(t *testing.T) {
	if ScreenDetectorFor("claude") == nil {
		t.Error(`ScreenDetectorFor("claude") = nil, want a detector`)
	}
	for _, name := range []string{"codex", "copilot", "agy", "pi", "", "unknown"} {
		if det := ScreenDetectorFor(name); det != nil {
			t.Errorf("ScreenDetectorFor(%q) = %T, want nil (claude-first scope)", name, det)
		}
	}
}
