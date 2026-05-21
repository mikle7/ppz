package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Default: --claude --opus, prompt as last positional argument.
//
// `claude` is invoked with --dangerously-skip-permissions because the
// agent is unattended in a pty — exactly the demo's pattern in
// ../ppz-demo-1/setup.sh.
func TestBuildAgentArgv_DefaultClaudeOpusWithPrompt(t *testing.T) {
	got, err := buildAgentArgv(agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  "You are an agent.",
	})
	if err != nil {
		t.Fatalf("buildAgentArgv: %v", err)
	}
	want := []string{
		"claude",
		"--dangerously-skip-permissions",
		"--model", "opus",
		"You are an agent.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_ClaudeSonnet(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "claude", model: "sonnet", prompt: "hi"})
	if got[3] != "sonnet" {
		t.Fatalf("expected sonnet model, got %q in %q", got[3], got)
	}
}

func TestBuildAgentArgv_ClaudeHaiku(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "claude", model: "haiku", prompt: "hi"})
	if got[3] != "haiku" {
		t.Fatalf("expected haiku model, got %q in %q", got[3], got)
	}
}

// No prompt → no trailing positional arg passed to claude. The harness
// boots into its normal REPL.
func TestBuildAgentArgv_ClaudeNoPrompt(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "claude", model: "opus"})
	want := []string{"claude", "--dangerously-skip-permissions", "--model", "opus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// Codex always gets --dangerously-bypass-approvals-and-sandbox baked
// in (the codex analogue of claude's --dangerously-skip-permissions),
// because codex's default seatbelt sandbox blocks the agent from
// reaching the host ppz daemon. No model default; whatever model the
// user gave is forwarded as-is.
func TestBuildAgentArgv_CodexWithModel(t *testing.T) {
	got, err := buildAgentArgv(agentSpec{harness: "codex", model: "gpt-5", prompt: "go"})
	if err != nil {
		t.Fatalf("buildAgentArgv: %v", err)
	}
	want := []string{"codex", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-5", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_CodexNoModel(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "codex", prompt: "go"})
	want := []string{"codex", "--dangerously-bypass-approvals-and-sandbox", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestBuildAgentArgv_CodexBypassesSandboxByDefault locks in the
// behaviour against accidental removal. The agent runs unattended
// inside a pty — codex's default seatbelt sandbox (CODEX_SANDBOX=
// seatbelt) blinds it to the running ppz daemon (`ppz status`
// returns "daemon: not running" from inside the sandbox), and there
// is no human at the terminal to approve a prompt-time escalation.
// The flag must appear regardless of whether a model or prompt is
// also supplied.
func TestBuildAgentArgv_CodexBypassesSandboxByDefault(t *testing.T) {
	for _, spec := range []agentSpec{
		{harness: "codex"},
		{harness: "codex", prompt: "hi"},
		{harness: "codex", model: "gpt-5"},
		{harness: "codex", model: "gpt-5", prompt: "hi"},
	} {
		got, err := buildAgentArgv(spec)
		if err != nil {
			t.Fatalf("buildAgentArgv(%+v): %v", spec, err)
		}
		found := false
		for _, a := range got {
			if a == "--dangerously-bypass-approvals-and-sandbox" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("spec=%+v: codex argv must include --dangerously-bypass-approvals-and-sandbox (sandbox blocks ppz daemon access); got %q", spec, got)
		}
	}
}

func TestBuildAgentArgv_GeminiWithModel(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "gemini", model: "2.5-pro", prompt: "go"})
	want := []string{"gemini", "--model", "2.5-pro", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// copilot rejects a positional prompt ("Invalid command format") — the
// initial prompt must arrive via `-i <prompt>`. We also pass --yolo so
// the unattended agent can act without per-tool approval prompts.
func TestBuildAgentArgv_Copilot(t *testing.T) {
	got, err := buildAgentArgv(agentSpec{harness: "copilot", prompt: "go"})
	if err != nil {
		t.Fatalf("buildAgentArgv: %v", err)
	}
	want := []string{"copilot", "--yolo", "-i", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_CopilotWithModel(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "copilot", model: "gpt-5", prompt: "go"})
	want := []string{"copilot", "--yolo", "--model", "gpt-5", "-i", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// No prompt → no -i flag (which requires a prompt argument); copilot
// boots into its normal REPL but still with --yolo applied.
func TestBuildAgentArgv_CopilotNoPrompt(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "copilot"})
	want := []string{"copilot", "--yolo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_Pi(t *testing.T) {
	got, err := buildAgentArgv(agentSpec{harness: "pi", prompt: "go"})
	if err != nil {
		t.Fatalf("buildAgentArgv: %v", err)
	}
	if got[0] != "pi" {
		t.Fatalf("argv[0]=%q, want pi", got[0])
	}
}

func TestBuildAgentArgv_UnknownHarnessErrors(t *testing.T) {
	if _, err := buildAgentArgv(agentSpec{harness: "bogus"}); err == nil {
		t.Fatal("expected error for unknown harness, got nil")
	}
}

// resolveAgentSpec is the flag-parser-side helper. Tests pin the default
// behaviour (claude + opus) and the mutual-exclusion rules.
func TestResolveAgentSpec_DefaultsToClaudeOpus(t *testing.T) {
	spec, _, err := resolveAgentSpec([]string{"alice", "be helpful"})
	if err != nil {
		t.Fatalf("resolveAgentSpec: %v", err)
	}
	if spec.harness != "claude" {
		t.Errorf("harness=%q, want claude", spec.harness)
	}
	if spec.model != "opus" {
		t.Errorf("model=%q, want opus", spec.model)
	}
	if spec.prompt != "be helpful" {
		t.Errorf("prompt=%q, want %q", spec.prompt, "be helpful")
	}
}

func TestResolveAgentSpec_ClaudeShortcuts(t *testing.T) {
	for flag, want := range map[string]string{
		"--opus":   "opus",
		"--sonnet": "sonnet",
		"--haiku":  "haiku",
	} {
		spec, _, err := resolveAgentSpec([]string{flag, "alice"})
		if err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if spec.model != want {
			t.Errorf("%s: model=%q, want %q", flag, spec.model, want)
		}
	}
}

// --opus / --sonnet / --haiku are claude-only. Combining them with
// another harness should error rather than silently picking one.
func TestResolveAgentSpec_ClaudeShortcutsRejectedOnOtherHarness(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"--codex", "--opus", "alice"})
	if err == nil {
		t.Fatal("expected error when --opus is used with --codex")
	}
}

func TestResolveAgentSpec_MultipleHarnessFlagsError(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"--claude", "--codex", "alice"})
	if err == nil {
		t.Fatal("expected error for --claude --codex together")
	}
}

func TestResolveAgentSpec_MultipleClaudeShortcutsError(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"--opus", "--sonnet", "alice"})
	if err == nil {
		t.Fatal("expected error for --opus --sonnet together")
	}
}

func TestResolveAgentSpec_ModelFlagOverridesDefault(t *testing.T) {
	spec, _, err := resolveAgentSpec([]string{"--codex", "--model", "gpt-5", "alice"})
	if err != nil {
		t.Fatalf("resolveAgentSpec: %v", err)
	}
	if spec.model != "gpt-5" {
		t.Errorf("model=%q, want gpt-5", spec.model)
	}
}

// --model and --opus together: explicit conflict, error.
func TestResolveAgentSpec_ClaudeShortcutAndModelFlagConflict(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"--opus", "--model", "sonnet", "alice"})
	if err == nil {
		t.Fatal("expected error for --opus + --model together")
	}
}

func TestResolveAgentSpec_NoHandleErrors(t *testing.T) {
	if _, _, err := resolveAgentSpec(nil); err == nil {
		t.Fatal("expected error for missing handle")
	}
}

// Default prompt: when neither a positional prompt nor --prompt-file is
// given, the agent boots with the orientation prompt baked into the
// binary. We assert on a stable substring rather than the full text so
// minor wording tweaks don't break the test.
func TestResolveAgentSpec_DefaultPromptUsedWhenNoneProvided(t *testing.T) {
	spec, _, err := resolveAgentSpec([]string{"alice"})
	if err != nil {
		t.Fatalf("resolveAgentSpec: %v", err)
	}
	if !strings.Contains(spec.prompt, "ppz read inbox") {
		t.Errorf("default prompt missing expected ppz orientation, got: %q", spec.prompt)
	}
}

// allHarnesses is the full set of values `--claude` / `--copilot` /
// `--codex` / `--gemini` / `--pi` resolve to in resolveAgentSpec. Tests
// that assert harness-agnostic invariants (no removed verbs, handle
// substitution, cheat-sheet column alignment, …) range over this list
// so a regression in any harness branch is caught.
var allHarnesses = []string{"claude", "copilot", "codex", "gemini", "pi"}

// backgroundMonitorHarnesses are the harnesses whose prompts wrap the
// `ppz ls --watch` loop in a detached/background process (claude's
// Monitor, copilot's bash detach:true). They share two invariants
// (`sleep 60` throttle, no `ppz await` mention) that don't apply to
// the codex-family foreground-watch pattern.
var backgroundMonitorHarnesses = []string{"claude", "copilot"}

// TestDefaultAgentPrompt_OmitsRemovedBroadcastVerb keeps `ppz broadcast`
// (removed in v0.30.0 — see tests/broadcast/broadcast-returns-unknown-command)
// from creeping back into the spawn-time orientation. An agent reading
// the prompt and trying the command would hit `unknown command, exit 2`
// and either retry-loop or hallucinate a workaround.
func TestDefaultAgentPrompt_OmitsRemovedBroadcastVerb(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if strings.Contains(defaultAgentPrompt("test-handle", h), "ppz broadcast") {
				t.Errorf("defaultAgentPrompt(%q) references the removed `ppz broadcast` verb; agents will hit `unknown command` if they try it", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_MentionsLsWatch pins `ppz ls --watch` as
// the recommended inbox-awareness primitive. It blocks until any
// pipe has unread, prints a snapshot, and exits without advancing
// any cursor — which is what a watch wants. The previous
// recommendation (`ppz await`) drains as it follows, so wiring a
// monitor to await races any later `ppz read inbox` and the
// user-visible bug is "the agent claims it acted but my read shows
// nothing". Every harness branch must reference the watch verb.
func TestDefaultAgentPrompt_MentionsLsWatch(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if !strings.Contains(defaultAgentPrompt("test-handle", h), "ppz ls --watch") {
				t.Errorf("defaultAgentPrompt(%q) should reference `ppz ls --watch` — the non-destructive blocking-watch primitive", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_MentionsWho pins `ppz who` in the cheat
// sheet. Without it, an agent reading the prompt knows how to list
// pipes (`ppz ls`) and message peers (`ppz send <handle>`) but has
// no documented way to discover *which* handles exist — agents were
// observed inventing handles or asking the user, instead of running
// the verb the daemon already exposes.
func TestDefaultAgentPrompt_MentionsWho(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if !strings.Contains(defaultAgentPrompt("test-handle", h), "ppz who") {
				t.Errorf("defaultAgentPrompt(%q) should reference `ppz who` so agents can discover which peers are online before trying to `ppz send`", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_OmitsAwait — keep `ppz await` out of the
// boot prompt *for the background-Monitor harnesses*. It's still a
// valid verb when the agent actively wants to drain, but mentioning
// it in the useful-commands cheat sheet led agents to wire it into
// a persistent Monitor, where it silently ate inbox messages the
// user then asked them to `ppz read`. The codex-family prompt
// references `ppz await --tail` only as an anti-pattern callout
// ("do not use this for idle behavior"), which is the opposite
// failure mode and is covered by a separate codex test.
func TestDefaultAgentPrompt_OmitsAwait(t *testing.T) {
	for _, h := range backgroundMonitorHarnesses {
		t.Run(h, func(t *testing.T) {
			if strings.Contains(defaultAgentPrompt("test-handle", h), "ppz await") {
				t.Errorf("defaultAgentPrompt(%q) must not mention `ppz await` — destructive read races `ppz read inbox`; use `ppz ls --watch` for awareness and `ppz read` for consumption", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_SubstitutesHandle pins the handle template
// substitution across every harness branch. The prompt is built
// per-spawn with the actual handle so each branch's watch recipe
// can hard-code PPZ_SESSION=<handle> inline. A regression to a
// const prompt would leave `<handle>` as a literal placeholder in
// the recipe — the agent would then run a watch keyed by the
// string "<handle>" instead of e.g. "alice".
func TestDefaultAgentPrompt_SubstitutesHandle(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			prompt := defaultAgentPrompt("alice", h)
			if !strings.Contains(prompt, `"alice"`) {
				t.Errorf("defaultAgentPrompt(\"alice\", %q) should mention the handle literally; got: %q", h, prompt)
			}
			if strings.Contains(prompt, "<handle>.stdout") {
				t.Errorf("defaultAgentPrompt(%q) should substitute the handle into `.stdout` / `.inbox` references, not leave the `<handle>` placeholder; got: %q", h, prompt)
			}
		})
	}
}

// TestDefaultAgentPrompt_MonitorRecipeThrottlesLoop — the background-
// Monitor harnesses (claude, copilot) must include a sleep on the
// success path. `ppz ls --watch` is non-destructive: once a pipe
// has unread, every immediate re-arm returns immediately with the
// same snapshot. Without the throttle the loop spins as fast as
// the daemon can answer, flooding the agent with duplicate events
// for the same unread state until it runs `ppz read` to clear
// them. The codex-family prompt uses a foreground watch (no loop),
// so the throttle does not apply there.
func TestDefaultAgentPrompt_MonitorRecipeThrottlesLoop(t *testing.T) {
	for _, h := range backgroundMonitorHarnesses {
		t.Run(h, func(t *testing.T) {
			prompt := defaultAgentPrompt("eve", h)
			if !strings.Contains(prompt, "sleep 60") {
				t.Errorf("defaultAgentPrompt(%q) Monitor recipe must throttle the loop with `sleep 60` so non-destructive ls --watch doesn't spin on persistent unread; got: %q", h, prompt)
			}
		})
	}
}

// TestDefaultAgentPrompt_MonitorRecipePinsSession — every harness
// branch that recommends `ppz ls --watch` must set
// PPZ_SESSION=<handle> inline on that command. Inheriting the
// parent shell's PPZ_SESSION is unreliable across harnesses; we
// observed Claude Code v2.1.143 dropping it on Monitor's bash
// subprocess, which then resolved a fresh tty-less session id the
// daemon had never seen and failed every ppz call with
// E_NO_CURRENT_SOURCE. Setting PPZ_SESSION inline in the recipe
// makes the watch robust to that behaviour.
func TestDefaultAgentPrompt_MonitorRecipePinsSession(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			prompt := defaultAgentPrompt("eve", h)
			if !strings.Contains(prompt, "PPZ_SESSION=eve ppz ls --watch") {
				t.Errorf("defaultAgentPrompt(%q) recipe should set PPZ_SESSION=<handle> inline so it survives env-strip on subprocesses; got: %q", h, prompt)
			}
		})
	}
}

// TestDefaultAgentPrompt_UsesUncollaredTerminology fixes a "uncoloured"
// → "uncollared" typo. The wire vocabulary in WIRE.md §1 is "collared"
// (source-bound) vs "uncollared" (sourceless, e.g. chat-room pipes).
// Mis-spelling it leaves an agent unable to grep / Ctrl-F into the
// actual docs and tests.
func TestDefaultAgentPrompt_UsesUncollaredTerminology(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if strings.Contains(defaultAgentPrompt("test-handle", h), "uncoloured") {
				t.Errorf("defaultAgentPrompt(%q) has the `uncoloured` typo; wire vocab is `uncollared` (WIRE.md §1)", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_CommandColumnIsAligned walks every "  ppz …"
// line in the prompt and asserts that the description begins at the
// same column on every row. Mis-aligned columns aren't a correctness
// bug, but the prompt is a man-page-style cheat sheet — a drifting
// column makes it harder to scan and signals "nobody runs this through
// a check". Allowing per-row variance lets a future edit silently
// undo today's alignment work. Each harness has its own cheat sheet,
// so the column is checked per-harness rather than across harnesses.
func TestDefaultAgentPrompt_CommandColumnIsAligned(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			descCol := -1
			for i, line := range strings.Split(defaultAgentPrompt("test-handle", h), "\n") {
				if !strings.HasPrefix(line, "  ppz ") {
					continue
				}
				// Description starts after the first run of 2+ spaces past the
				// leading indent — same rule the CLI's usage-text wrapper uses
				// (cli/root.go wrapUsageText).
				idx := -1
				for j := 2; j < len(line); j++ {
					if line[j] == ' ' && j+1 < len(line) && line[j+1] == ' ' {
						k := j
						for k < len(line) && line[k] == ' ' {
							k++
						}
						idx = k
						break
					}
				}
				if idx < 0 {
					t.Errorf("line %d (%q) has no description column", i, line)
					continue
				}
				if descCol == -1 {
					descCol = idx
					continue
				}
				if idx != descCol {
					t.Errorf("line %d (%q) starts description at col %d; expected col %d (matching the first command line)", i, line, idx, descCol)
				}
			}
			if descCol == -1 {
				t.Fatalf("defaultAgentPrompt(%q) has no `  ppz …` lines to align", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_Claude_KeepsMonitorPushNotificationRecipe pins
// today's claude-specific wording. The Monitor + PushNotification
// pattern is load-bearing for claude — both are first-class harness
// tools that turn the agent push-driven rather than poll-driven — so
// any harness-branched refactor must preserve them on the `--claude`
// path verbatim. Other harnesses (no equivalent primitive) get
// different bodies and are covered by their own tests.
func TestDefaultAgentPrompt_Claude_KeepsMonitorPushNotificationRecipe(t *testing.T) {
	prompt := defaultAgentPrompt("alice", "claude")
	if !strings.Contains(prompt, "Create a persistent Monitor running") {
		t.Errorf("claude prompt must keep the `Create a persistent Monitor running` recipe — Monitor is the claude-specific harness tool that turns ppz ls --watch push-driven; got: %q", prompt)
	}
	if !strings.Contains(prompt, "PushNotification") {
		t.Errorf("claude prompt must reference PushNotification — without it the Monitor recipe has nowhere to fire on new arrivals; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_Copilot_UsesBashDetachTrue pins copilot's
// equivalent of claude's Monitor: copilot's bash tool with
// `detach: true` runs the `ppz ls --watch` loop as a background
// process. Without naming the option explicitly the agent is left
// to guess (or worse, run the loop synchronously and block the
// session). The "GitHub Copilot CLI" line up top primes copilot to
// treat inbox messages as natural-language prompts rather than
// raw shell commands — without it copilot was observed trying to
// `exec` whatever arrived in inbox.
func TestDefaultAgentPrompt_Copilot_UsesBashDetachTrue(t *testing.T) {
	prompt := defaultAgentPrompt("alice", "copilot")
	if !strings.Contains(prompt, "GitHub Copilot CLI") {
		t.Errorf("copilot prompt should self-identify as `GitHub Copilot CLI` so the agent treats inbox messages as conversational prompts, not shell commands; got: %q", prompt)
	}
	if !strings.Contains(prompt, "detach: true") {
		t.Errorf("copilot prompt should reference `detach: true` — copilot's bash-tool option for detached background processes (the analogue of claude's Monitor); got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_Codex_UsesForegroundWatchRecipe pins the
// codex-family pattern: codex has no push primitive, so the agent
// itself blocks on `ppz ls --watch` in the foreground and treats
// the return as a wake signal that must be followed by a `ppz read`
// before the next watch. The "wake signal" phrasing is what tells
// codex not to re-watch immediately after wakeup (which would
// re-trigger on the same unread snapshot indefinitely). The two
// negative assertions guard against accidental copy-paste of the
// claude- or copilot-specific tool names into the codex branch.
func TestDefaultAgentPrompt_Codex_UsesForegroundWatchRecipe(t *testing.T) {
	prompt := defaultAgentPrompt("alice", "codex")
	if !strings.Contains(prompt, "wake signal") {
		t.Errorf("codex prompt should describe `ppz ls --watch` as a wake signal so the agent reads before re-watching (codex has no push primitive); got: %q", prompt)
	}
	if strings.Contains(prompt, "PushNotification") {
		t.Errorf("codex prompt must not reference PushNotification — that's a claude-specific harness tool codex does not have; got: %q", prompt)
	}
	if strings.Contains(prompt, "detach: true") {
		t.Errorf("codex prompt must not reference `detach: true` — that's a copilot-specific bash-tool option codex does not have; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_Gemini_FallsBackToCodexRecipe pins gemini
// onto the codex-family foreground-watch recipe. Until gemini's
// own background/notification primitives are confirmed, sharing
// codex's prompt is the safe default: it names no harness-specific
// tools, only ppz verbs and a generic wake/read loop.
func TestDefaultAgentPrompt_Gemini_FallsBackToCodexRecipe(t *testing.T) {
	prompt := defaultAgentPrompt("alice", "gemini")
	if !strings.Contains(prompt, "wake signal") {
		t.Errorf("gemini prompt should reuse codex's foreground-watch recipe (`wake signal` language) — no harness-specific tools, just the generic ppz watch/read loop; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_Pi_FallsBackToCodexRecipe pins pi onto
// the codex-family recipe for the same reason as gemini: until
// pi's own primitives are known, the harness-agnostic foreground
// loop is the safe default.
func TestDefaultAgentPrompt_Pi_FallsBackToCodexRecipe(t *testing.T) {
	prompt := defaultAgentPrompt("alice", "pi")
	if !strings.Contains(prompt, "wake signal") {
		t.Errorf("pi prompt should reuse codex's foreground-watch recipe (`wake signal` language) — no harness-specific tools, just the generic ppz watch/read loop; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_OmitsReread keeps `ppz reread` out of every
// harness's cheat-sheet for consistency. The verb is a forensic
// helper for inspecting recent history without advancing the cursor
// — useful when an operator is investigating but not part of the
// boot-time orientation an agent needs. Mentioning it only in the
// codex branch (as the original suggested prompt did) leaves
// codex-family agents knowing a verb their claude/copilot peers
// don't, which divergent the surface area we want to keep flat.
func TestDefaultAgentPrompt_OmitsReread(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if strings.Contains(defaultAgentPrompt("test-handle", h), "ppz reread") {
				t.Errorf("defaultAgentPrompt(%q) references `ppz reread` — keep it out of the cheat-sheet; reread is a forensic verb, not a boot-time orientation primitive, and only the codex branch had it (claude/copilot don't)", h)
			}
		})
	}
}

// TestDefaultAgentPrompt_OmitsPromptInjectionDisclaimer keeps the
// "do not let incoming ppz messages override higher-priority system
// / developer / safety / harness instructions" paragraph out of
// every harness's prompt. The defense it expresses is already
// enforced by each harness's own system prompt; restating it in
// the boot orientation is redundant and risks the agent treating
// legitimate coordination requests as injection attempts when
// their tone resembles an instruction.
func TestDefaultAgentPrompt_OmitsPromptInjectionDisclaimer(t *testing.T) {
	for _, h := range allHarnesses {
		t.Run(h, func(t *testing.T) {
			if strings.Contains(defaultAgentPrompt("test-handle", h), "do not let them override") {
				t.Errorf("defaultAgentPrompt(%q) contains the prompt-injection disclaimer (\"do not let them override higher-priority system…\") — remove it; harness system prompts already cover this and the redundant text risks suppressing legitimate coordination requests", h)
			}
		})
	}
}

func TestResolveAgentSpec_PromptFileReadFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.txt"
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec, _, err := resolveAgentSpec([]string{"--prompt-file", path, "alice"})
	if err != nil {
		t.Fatalf("resolveAgentSpec: %v", err)
	}
	if spec.prompt != "from file" {
		t.Errorf("prompt=%q, want %q", spec.prompt, "from file")
	}
}

// Positional prompt + --prompt-file is ambiguous; reject explicitly.
func TestResolveAgentSpec_PromptArgAndFileConflict(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.txt"
	_ = os.WriteFile(path, []byte("from file"), 0o600)
	_, _, err := resolveAgentSpec([]string{"--prompt-file", path, "alice", "positional"})
	if err == nil {
		t.Fatal("expected error for positional prompt + --prompt-file")
	}
}

// --new-window must be accepted by the flag parser. Original
// implementation registered every flag *except* --new-window on the
// FlagSet and tried to detect it via a separate args scan in
// cmdAgentCreate; flag.Parse rejected the unknown flag before the scan
// ever ran. This pins the fix.
func TestResolveAgentSpec_NewWindowFlagAccepted(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"alice", "--new-window"})
	if err != nil {
		t.Fatalf("--new-window must be accepted by the flag parser, got: %v", err)
	}
}

// Same with the single-dash form Go's flag package allows.
func TestResolveAgentSpec_NewWindowFlagAcceptedSingleDash(t *testing.T) {
	_, _, err := resolveAgentSpec([]string{"alice", "-new-window"})
	if err != nil {
		t.Fatalf("-new-window must be accepted by the flag parser, got: %v", err)
	}
}

func TestResolveAgentSpec_HandleParsed(t *testing.T) {
	_, handle, err := resolveAgentSpec([]string{"alice", "hi"})
	if err != nil {
		t.Fatalf("resolveAgentSpec: %v", err)
	}
	if handle != "alice" {
		t.Errorf("handle=%q, want alice", handle)
	}
}

// Foreground osascript builder for Terminal.app / iTerm2: the new-window
// command line must include the `ppz terminal share <handle> --` prefix
// followed by the harness argv, with shell quoting that survives multi-
// line prompts.
func TestBuildNewWindowScript_TerminalAppContainsShareInvocation(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", nil, []string{"claude", "-p", "hello"})
	if !strings.Contains(cmd, "ppz terminal share alice --") {
		t.Errorf("expected ppz share prefix, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "tell application \"Terminal\"") {
		t.Errorf("expected Terminal.app osascript, got:\n%s", cmd)
	}
}

func TestBuildNewWindowScript_ITerm2Detected(t *testing.T) {
	cmd := buildNewWindowScript("iTerm.app", "alice", "", nil, []string{"claude"})
	if !strings.Contains(cmd, "tell application \"iTerm\"") {
		t.Errorf("expected iTerm osascript, got:\n%s", cmd)
	}
}

// Prompt strings can contain newlines and single/double quotes. The
// builder writes the prompt to a temp file and dereferences it with
// $(cat …) so we never have to shell-quote the content.
func TestBuildNewWindowScript_PromptFileDereferenced(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", nil,
		[]string{"claude", "$(cat /tmp/ppz-agent-alice.prompt)"})
	if !strings.Contains(cmd, "$(cat /tmp/ppz-agent-alice.prompt)") {
		t.Errorf("expected prompt-file dereference, got:\n%s", cmd)
	}
}

// macOS `do script` opens the new Terminal window in $HOME — which often
// isn't a folder claude has trusted, so claude shows a "trust this
// folder?" dialog the first time it boots. Inheriting the parent
// shell's cwd avoids that: the spawned shell runs `cd '<cwd>' &&` first,
// so claude boots in the folder the user invoked `ppz agent create`
// from (presumably already trusted).
func TestBuildNewWindowScript_PrependsCdToCallersCwd(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "/Users/jimmy/work", nil, []string{"claude"})
	if !strings.Contains(cmd, `cd '/Users/jimmy/work'`) {
		t.Errorf("expected single-quoted cd to caller's cwd, got:\n%s", cmd)
	}
	cdIdx := strings.Index(cmd, "cd '/Users/jimmy/work'")
	shareIdx := strings.Index(cmd, "ppz terminal share")
	if cdIdx < 0 || shareIdx < 0 || cdIdx > shareIdx {
		t.Errorf("cd must appear before ppz terminal share, got:\n%s", cmd)
	}
}

// iTerm2 path inherits the same cwd-preservation semantics as
// Terminal.app — the bug is dialect-agnostic.
func TestBuildNewWindowScript_ITerm2AlsoPrependsCd(t *testing.T) {
	cmd := buildNewWindowScript("iTerm.app", "alice", "/Users/jimmy/work", nil, []string{"claude"})
	if !strings.Contains(cmd, `cd '/Users/jimmy/work'`) {
		t.Errorf("iTerm path must include cd, got:\n%s", cmd)
	}
}

// Empty cwd → no cd prefix. Lets tests + callers that genuinely don't
// care about cwd opt out (and gives `os.Getwd` a graceful fallback if
// it fails).
func TestBuildNewWindowScript_EmptyCwdSkipsCd(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", nil, []string{"claude"})
	if strings.Contains(cmd, "cd ") {
		t.Errorf("empty cwd must not produce cd, got:\n%s", cmd)
	}
}

// Bash-safe single-quote handling: paths containing a single quote
// must escape it as `'\''` (close-quote, escaped quote, reopen) so the
// shell doesn't break out of the cd argument mid-path. We assert on
// the post-AppleScript-escape form (`\` doubled to `\\` in the string
// literal) — that's the literal AppleScript source the script becomes.
// AppleScript unescapes back to `'\''` at runtime, which the shell
// then interprets as the close-escape-reopen pattern.
func TestBuildNewWindowScript_CdEscapesSingleQuote(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", `/path/with'quote`, nil, []string{"claude"})
	if !strings.Contains(cmd, `'/path/with'\\''quote'`) {
		t.Errorf("expected bash-safe single-quote escape (in AppleScript form), got:\n%s", cmd)
	}
}

// --------------------------------------------------------------------------
// Linux & WSL --new-window support (RED phase)
//
// The macOS path above drives osascript. Linux and WSL need different
// dispatchers. The tests below pin the contract of four helpers:
//
//   selectLinuxTerminal     — pick which emulator to drive
//   buildLinuxNewWindowArgv — translate (terminal, handle, cwd, argv) → exec argv
//   isWSL                   — detect Windows Subsystem for Linux
//   buildWSLNewWindowArgv   — wt.exe + wsl.exe argv for the WSL path
//
// Stubs live in agent.go; bodies land in the GREEN follow-up.
// --------------------------------------------------------------------------

// $TERMINAL is the user's explicit preference. When set, it wins over
// the probe chain — even if the probe would have found something else.
func TestSelectLinuxTerminal_RespectsTerminalEnv(t *testing.T) {
	got, err := selectLinuxTerminal("konsole", func(string) bool { return false })
	if err != nil {
		t.Fatalf("selectLinuxTerminal: %v", err)
	}
	if got != "konsole" {
		t.Errorf("got %q, want konsole — $TERMINAL must win", got)
	}
}

// Empty $TERMINAL → fall back to the availability probe.
func TestSelectLinuxTerminal_FallsBackToProbe(t *testing.T) {
	available := func(name string) bool { return name == "xterm" }
	got, err := selectLinuxTerminal("", available)
	if err != nil {
		t.Fatalf("selectLinuxTerminal: %v", err)
	}
	if got != "xterm" {
		t.Errorf("got %q, want xterm", got)
	}
}

// Probe order matters: gnome-terminal beats xterm when both are
// installed. Pinning the priority avoids silently demoting a fully
// featured emulator to bare xterm on stock GNOME desktops.
func TestSelectLinuxTerminal_PrefersGnomeOverXterm(t *testing.T) {
	available := func(name string) bool { return name == "gnome-terminal" || name == "xterm" }
	got, err := selectLinuxTerminal("", available)
	if err != nil {
		t.Fatalf("selectLinuxTerminal: %v", err)
	}
	if got != "gnome-terminal" {
		t.Errorf("got %q, want gnome-terminal (higher priority than xterm)", got)
	}
}

func TestSelectLinuxTerminal_NoneAvailableErrors(t *testing.T) {
	_, err := selectLinuxTerminal("", func(string) bool { return false })
	if err == nil {
		t.Fatal("expected error when no terminal is available")
	}
}

// gnome-terminal famously requires `--` before the inner command:
//
//	gnome-terminal -- bash -c "<script>"
//
// Without the separator gnome-terminal swallows the bash invocation as
// its own argument. Pin the shape.
func TestBuildLinuxNewWindowArgv_GnomeTerminalUsesDashDashSeparator(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("gnome-terminal", "alice", "", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if argv[0] != "gnome-terminal" {
		t.Errorf("argv[0]=%q, want gnome-terminal", argv[0])
	}
	if !containsAdjacent(argv, "--", "bash") {
		t.Errorf("gnome-terminal must use `-- bash` separator, got: %q", argv)
	}
}

func TestBuildLinuxNewWindowArgv_KonsoleUsesDashE(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("konsole", "alice", "", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if argv[0] != "konsole" || !containsAdjacent(argv, "-e", "bash") {
		t.Errorf("konsole must use `-e bash`, got: %q", argv)
	}
}

func TestBuildLinuxNewWindowArgv_XtermUsesDashE(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if argv[0] != "xterm" || !containsAdjacent(argv, "-e", "bash") {
		t.Errorf("xterm must use `-e bash`, got: %q", argv)
	}
}

// Whatever the terminal, the *last* argv element is the bash -c script,
// and it must contain `ppz terminal share <handle> --` followed by the
// harness argv. Asserting on the last element keeps the test resilient
// to per-terminal flag-shape differences.
func TestBuildLinuxNewWindowArgv_IncludesShareInvocation(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", nil, []string{"claude", "-p", "hi"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, "ppz terminal share alice -- claude") {
		t.Errorf("script missing ppz share invocation, got: %q", script)
	}
}

// Same cwd-inheritance bug exists on Linux: a fresh terminal opens in
// $HOME, and claude treats trust per-folder. Prepend `cd '<cwd>' &&`.
func TestBuildLinuxNewWindowArgv_PrependsCdToCallersCwd(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "/home/jamesmiles/work", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, `cd '/home/jamesmiles/work'`) {
		t.Errorf("script missing cd to caller's cwd, got: %q", script)
	}
	cdIdx := strings.Index(script, "cd ")
	shareIdx := strings.Index(script, "ppz terminal share")
	if cdIdx < 0 || shareIdx < 0 || cdIdx > shareIdx {
		t.Errorf("cd must appear before ppz terminal share, got: %q", script)
	}
}

func TestBuildLinuxNewWindowArgv_EmptyCwdSkipsCd(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if strings.Contains(script, "cd ") {
		t.Errorf("empty cwd must not produce a cd prefix, got: %q", script)
	}
}

func TestBuildLinuxNewWindowArgv_UnknownTerminalErrors(t *testing.T) {
	if _, err := buildLinuxNewWindowArgv("nonesuch", "alice", "", nil, []string{"claude"}); err == nil {
		t.Fatal("expected error for unknown terminal")
	}
}

// ---- WSL ----

func TestIsWSL_DetectsMicrosoftKernel(t *testing.T) {
	// Matches the WSL2 kernel marker reported by /proc/version on
	// modern WSL — including this dev box.
	if !isWSL("Linux version 5.15.90.1-microsoft-standard-WSL2 (oe-user@oe-host)") {
		t.Error("isWSL must return true for microsoft-tagged kernel")
	}
}

func TestIsWSL_FalseOnNativeLinux(t *testing.T) {
	if isWSL("Linux version 6.6.0-generic (buildd@lcy02-amd64-001) (gcc) #1 SMP Ubuntu") {
		t.Error("isWSL must return false for vanilla Linux kernel")
	}
}

// On WSL the new window opens on the *Windows* host via wt.exe
// (Windows Terminal). We expect argv[0] to be the literal `wt.exe` —
// resolution to a full path is left to os/exec.LookPath.
func TestBuildWSLNewWindowArgv_UsesWtExe(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "/tmp/ppz-agent-alice.sh")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if argv[0] != "wt.exe" {
		t.Errorf("argv[0]=%q, want wt.exe", argv[0])
	}
}

// wt.exe spawns the actual WSL session via `wsl.exe -d <distro>`. The
// distro name comes from $WSL_DISTRO_NAME in the caller; without `-d`
// wt would launch the default distro, which may not match the one the
// user invoked ppz from.
func TestBuildWSLNewWindowArgv_InvokesWslExeWithDistro(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu-22.04", "/tmp/ppz-agent-alice.sh")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !containsAdjacent(argv, "wsl.exe", "-d") {
		t.Errorf("must invoke `wsl.exe -d`, got: %q", argv)
	}
	if !strings.Contains(strings.Join(argv, " "), "-d Ubuntu-22.04") {
		t.Errorf("must include `-d Ubuntu-22.04`, got: %q", argv)
	}
}

// The script-builder is the source-of-truth for what bash actually
// runs. Pin the ppz share invocation here (no wt.exe in the loop).
func TestBuildWSLScript_IncludesShareInvocation(t *testing.T) {
	script := buildWSLScript("alice", "", nil, []string{"claude", "-p", "hi"})
	if !strings.Contains(script, "ppz terminal share alice -- claude") {
		t.Errorf("script missing ppz share invocation, got: %q", script)
	}
}

// cwd is inherited from the parent shell so the spawned harness boots
// in a folder the user already trusts. Without it, claude shows a
// "trust this folder?" dialog on every run.
func TestBuildWSLScript_PrependsCdToCallersCwd(t *testing.T) {
	script := buildWSLScript("alice", "/home/jamesmiles/work", nil, []string{"claude"})
	if !strings.Contains(script, `cd '/home/jamesmiles/work'`) {
		t.Errorf("script missing cd to caller's cwd, got: %q", script)
	}
}

// Empty distro is a caller bug — $WSL_DISTRO_NAME was unset. Better to
// error loudly than to silently fall through to the user's default
// distro (which may be the wrong one).
func TestBuildWSLNewWindowArgv_EmptyDistroErrors(t *testing.T) {
	if _, err := buildWSLNewWindowArgv("", "/tmp/ppz-agent-alice.sh"); err == nil {
		t.Fatal("expected error for empty distro (caller forgot $WSL_DISTRO_NAME)")
	}
}

// Empty scriptPath is a caller bug — writeAgentSpawnScript wasn't run
// (or failed silently). Error loudly so we don't hand wt.exe a `bash
// -l` with no positional arg, which would open an empty WSL shell
// (the previously-reported "flashing cursor" symptom).
func TestBuildWSLNewWindowArgv_EmptyScriptPathErrors(t *testing.T) {
	if _, err := buildWSLNewWindowArgv("Ubuntu", ""); err == nil {
		t.Fatal("expected error for empty scriptPath")
	}
}

// TestBuildWSLNewWindowArgv_DoesNotExposeScriptToWtExe is the
// load-bearing regression test for the v0.33.4 hang on WSL.
//
// History: between v0.32 and v0.33 the default agent prompt grew a
// Monitor recipe (`while true; do … ; sleep 60 ; done`) with three
// semicolons inside a bash-single-quoted prompt argument. Inlining
// that script into wt.exe's argv broke `ppz agent create --new-window`
// on WSL in two stacked ways:
//
//  1. wt.exe treats `;` as a sub-command separator in its own argv
//     and splits the script there, launching each trailing chunk as
//     a standalone Windows program (`error 2147942402 The system
//     cannot find the file specified.`).
//  2. Even with `\;` escapes added, wt.exe collapses the `''`
//     close-quote/open-quote pair that sits in the middle of the
//     standard bash escape `'\''` (used by bashSingleQuote for any
//     `'` inside the prompt — e.g. `isn't`). The collapse turns
//     `isn'\''t` into `isn'\t'`, leaving bash with an unmatched
//     closing quote and hanging at PS2: "flashing cursor, no source
//     created, no claude UI".
//
// The fix is structural: never expose the bash script to wt.exe's
// tokenizer at all. Write the script to a tempfile and invoke `bash
// -l <path>` — wt.exe sees only a benign path. This test pins the
// invariant: no element of the wt.exe argv may contain either of
// the two metacharacters wt.exe mishandles (`;` or the `'\''` bash
// escape), regardless of how exotic the harness argv is.
func TestBuildWSLNewWindowArgv_DoesNotExposeScriptToWtExe(t *testing.T) {
	// A prompt that triggers BOTH wt.exe pathologies: a semicolon
	// (split-on-`;`) and an embedded `'` (`'\''` collapse).
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  "isn't ready; do X",
	}
	harnessArgv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	body := buildWSLScript("alice", "/home/u", agentEnvPairs(spec), harnessArgv)
	// Sanity: the body itself must contain both pathologies (otherwise
	// the assertions below pass trivially and the test is useless).
	if !strings.Contains(body, ";") {
		t.Fatalf("test setup: script body must contain `;`, got: %q", body)
	}
	if !strings.Contains(body, `'\''`) {
		t.Fatalf("test setup: script body must contain the bash quote escape `'\\''`, got: %q", body)
	}
	scriptPath := "/tmp/ppz-agent-alice-XXXXX.sh"
	argv, err := buildWSLNewWindowArgv("Ubuntu", scriptPath)
	if err != nil {
		t.Fatalf("buildWSLNewWindowArgv: %v", err)
	}
	for i, elt := range argv {
		if strings.Contains(elt, ";") {
			t.Errorf("argv[%d] contains `;` (wt.exe will split here): %q", i, elt)
		}
		if strings.Contains(elt, `'\''`) {
			t.Errorf("argv[%d] contains the bash quote escape `'\\''` (wt.exe collapses the `''` pair, breaking bash quoting): %q", i, elt)
		}
	}
}

// The spawned bash must be a *login* shell (`-lc`, not `-c`) so it
// sources ~/.profile / ~/.bash_profile. Most users' PATH additions for
// claude (npm-global, nvm, asdf, ~/.local/bin) live there — a plain
// non-login `bash -c` produces a PATH like /usr/bin:/bin:… and the
// spawned `claude` is reported as "executable file not found in $PATH".
// Verified on this WSL2 box: non-login bash sees no claude, login bash
// resolves /home/<user>/.local/bin/claude.
func TestBuildLinuxNewWindowArgv_UsesLoginShell(t *testing.T) {
	for _, terminal := range []string{"gnome-terminal", "konsole", "xterm", "kitty", "wezterm"} {
		argv, err := buildLinuxNewWindowArgv(terminal, "alice", "", nil, []string{"claude"})
		if err != nil {
			t.Fatalf("%s: build: %v", terminal, err)
		}
		if !containsAdjacent(argv, "bash", "-lc") {
			t.Errorf("%s: must invoke `bash -lc` (login shell) so ~/.profile-style PATH additions reach claude, got: %q", terminal, argv)
		}
	}
}

// Same constraint applies to the WSL path: `wsl.exe -d <distro> bash`
// spawns a bash that inherits PATH from wsl.exe (a Windows process),
// which means *only* the system minimum PATH unless we ask for login
// shell semantics. Without `-l` this reproducibly fails with
// "exec: claude: executable file not found in $PATH".
//
// Note: the script is sourced via `bash -l <path>` rather than `bash
// -lc <inline-script>` so wt.exe never sees the script content — see
// buildWSLScript's doc-comment for the rationale.
func TestBuildWSLNewWindowArgv_UsesLoginShell(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "/tmp/ppz-agent-alice.sh")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !containsAdjacent(argv, "bash", "-l") {
		t.Errorf("WSL path must invoke `bash -l`, got: %q", argv)
	}
}

// TestWriteAgentSpawnScript_SelfCleans pins the trap that removes
// the tempfile on EXIT. Without it, every `ppz agent create
// --new-window` leaks a /tmp/ppz-agent-*.sh helper that builds up
// indefinitely. The trap fires on normal exit, errors, and signals
// (e.g. user Ctrl-Cs the spawned harness) — bash's EXIT pseudo-signal
// covers all three.
func TestWriteAgentSpawnScript_SelfCleans(t *testing.T) {
	dir := t.TempDir()
	path, err := writeAgentSpawnScript(dir, "alice", "echo hello")
	if err != nil {
		t.Fatalf("writeAgentSpawnScript: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tempfile: %v", err)
	}
	if !strings.Contains(string(body), "trap ") {
		t.Errorf("script must install an EXIT trap to self-clean, got: %q", body)
	}
	if !strings.Contains(string(body), "EXIT") {
		t.Errorf("self-cleanup trap must fire on EXIT (covers normal exit, errors, and signals), got: %q", body)
	}
	if !strings.Contains(string(body), "rm -f -- '"+path+"'") {
		t.Errorf("self-cleanup must `rm -f -- <this script's path>`, got: %q", body)
	}
	if !strings.Contains(string(body), "echo hello") {
		t.Errorf("script body must be embedded verbatim, got: %q", body)
	}
}

// The tempfile must be in the requested dir (so tests can use
// t.TempDir() for hermetic cleanup) and must be named with the
// handle so a developer eyeballing /tmp can tell which agent's
// script leaked if cleanup ever breaks.
func TestWriteAgentSpawnScript_NamesByHandleInDir(t *testing.T) {
	dir := t.TempDir()
	path, err := writeAgentSpawnScript(dir, "alice", "echo hi")
	if err != nil {
		t.Fatalf("writeAgentSpawnScript: %v", err)
	}
	if !strings.HasPrefix(path, dir+string(os.PathSeparator)) {
		t.Errorf("script path %q must live under requested dir %q", path, dir)
	}
	if !strings.Contains(path, "alice") {
		t.Errorf("script path %q must include the handle so leaks are diagnosable", path)
	}
}

// Mode must be 0700: world-readable scripts are a leak surface (the
// embedded prompt may include sensitive context). Owner-only.
func TestWriteAgentSpawnScript_RestrictedMode(t *testing.T) {
	dir := t.TempDir()
	path, err := writeAgentSpawnScript(dir, "alice", "echo hi")
	if err != nil {
		t.Fatalf("writeAgentSpawnScript: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Errorf("script mode = %#o, want 0700 (owner-only — prompt may contain sensitive context)", mode)
	}
}

// TestSpawnScript_RunsUnderBashLoginShell is the bash-script-level e2e
// test for the WSL --new-window flow. It does NOT invoke wt.exe (CI
// runners aren't WSL and have no Windows Terminal) but it DOES
// exercise the layer that hung on WSL in v0.33.4: bash itself parsing
// and running the script we write to disk.
//
// Why this catches a class of bug the invariant tests miss:
//
//   - The invariant tests (DoesNotExposeScriptToWtExe etc.) prove the
//     script doesn't pass through wt.exe's tokeniser. They say nothing
//     about whether bash can actually parse the script. PR #65 shipped
//     a green-CI build that hung at PS2 the moment bash hit the bash-
//     quote-escape `'\''` — we never exec'd the script.
//   - The realistic input here is the default agent prompt, which has
//     both pathologies that previously broke WSL: `;` inside a
//     single-quoted bash arg (Monitor recipe) AND embedded `'` (bash
//     contractions like `isn't`) that produce `'\''` escapes in the
//     script. If our quoting is broken, bash hangs and the 10s
//     timeout fires; if our env-prefix or `cd` is wrong, the stubs'
//     log shows the wrong shape.
//
// Setup: two stubs in a TempDir on PATH — a `ppz` that appends its
// argv and selected env vars to a log file, and a `claude` that's a
// no-op (otherwise it'd try to launch the interactive harness and
// hang). HOME is also a TempDir so `bash -l` doesn't source the
// developer's real ~/.profile and contaminate the run.
func TestSpawnScript_RunsUnderBashLoginShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available on $PATH")
	}

	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "ppz.log")

	// `ppz` stub: append a delimited record of argv and PPZ_AGENT_*
	// env vars to logPath. `set -u` would trip on the env vars when
	// unset, so reference them with the `:-` default so an absent
	// env shows up as an empty value rather than aborting the stub.
	ppzStub := "#!/bin/bash\n" +
		"{\n" +
		"  echo '--- ppz invocation ---'\n" +
		"  echo \"argv: $*\"\n" +
		"  echo \"PPZ_AGENT_HARNESS=${PPZ_AGENT_HARNESS:-}\"\n" +
		"  echo \"PPZ_AGENT_MODEL=${PPZ_AGENT_MODEL:-}\"\n" +
		"  echo \"PWD=$PWD\"\n" +
		"} >> " + logPath + "\n"
	if err := os.WriteFile(filepath.Join(stubDir, "ppz"), []byte(ppzStub), 0o700); err != nil {
		t.Fatalf("write ppz stub: %v", err)
	}
	// `claude` stub: no-op. The real harness would try to render an
	// interactive TUI and hang under `bash -l`.
	if err := os.WriteFile(filepath.Join(stubDir, "claude"), []byte("#!/bin/bash\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write claude stub: %v", err)
	}

	// Build the spawn script via the same code path runAgentInNewWindow's
	// WSL branch uses, with the *default* agent prompt as input — that's
	// the realistic case that hung in v0.33.4.
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  defaultAgentPrompt("alice", "claude"),
	}
	harnessArgv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	body := buildWSLScript("alice", stubDir, agentEnvPairs(spec), harnessArgv)
	scriptPath, err := writeAgentSpawnScript(t.TempDir(), "alice", body)
	if err != nil {
		t.Fatalf("writeAgentSpawnScript: %v", err)
	}

	// Run with our stubs winning on PATH. A 10s timeout catches the
	// "flashing cursor" / PS2-hang failure mode — if bash can't parse
	// the script, it'll wait indefinitely for more input.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-l", scriptPath)
	// HOME=TempDir so `bash -l` doesn't source the developer's real
	// ~/.profile (which might do anything from setting unrelated env
	// to running other commands, contaminating our assertions).
	cmd.Env = []string{
		"PATH=" + stubDir + ":" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bash hung — script likely has unclosed quoting or another parse error; output so far:\n%s", out)
	}
	if err != nil {
		t.Fatalf("bash failed: %v\noutput:\n%s", err, out)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v\nbash output:\n%s", err, out)
	}
	logStr := string(log)

	// The stub should have been invoked once with the full ppz argv.
	if !strings.Contains(logStr, "--- ppz invocation ---") {
		t.Errorf("stub ppz was never invoked — bash parsed the script but didn't reach the ppz call; log was:\n%s\n\nbash output:\n%s", logStr, out)
	}
	// argv shape: `ppz terminal share alice -- claude --dangerously-skip-permissions --model opus <prompt>`
	// We assert on the discriminating subset.
	for _, want := range []string{"terminal", "share", "alice", "--", "claude"} {
		if !strings.Contains(logStr, want) {
			t.Errorf("ppz argv missing token %q; log:\n%s", want, logStr)
		}
	}
	// Env-prefix carried through `env A=... B=... ppz ...`.
	if !strings.Contains(logStr, "PPZ_AGENT_HARNESS=claude") {
		t.Errorf("PPZ_AGENT_HARNESS didn't reach ppz — `env` prefix is broken; log:\n%s", logStr)
	}
	if !strings.Contains(logStr, "PPZ_AGENT_MODEL=opus") {
		t.Errorf("PPZ_AGENT_MODEL didn't reach ppz; log:\n%s", logStr)
	}
	// cwd: `cd '<dir>' && ...` should have taken effect before ppz ran.
	if !strings.Contains(logStr, "PWD="+stubDir) {
		t.Errorf("cwd was not %q when ppz ran — `cd` is broken; log:\n%s", stubDir, logStr)
	}
}

// --------------------------------------------------------------------------
// Prompt-truncation regression (RED phase)
//
// Reproduction on this WSL2 box (Windows Terminal as front-end):
//
//   $ cat > /tmp/probe-prompt.txt <<EOF
//   You are an agent.
//   Line two has multiple words.
//   EOF
//   $ wt.exe -w 0 nt wsl.exe -d Ubuntu bash -lc \
//       '/tmp/record-argv.sh first --model opus "$(cat /tmp/probe-prompt.txt)"'
//   → recorded: argv=[first --model opus You]      ← prompt truncated to first word
//
//   $ bash -lc '/tmp/record-argv.sh ... "$(cat /tmp/probe-prompt.txt)"'
//   → recorded: argv=[first --model opus $'You are an agent.\nLine two...']  ✓
//
//   $ wsl.exe -d Ubuntu bash -lc '… same …'
//   → recorded: argv=[first --model opus $'You are an agent.\nLine two...']  ✓
//
// Conclusion: wt.exe's Windows command-line parser strips the outer
// double quotes around `"$(cat FILE)"`. Bash then sees unquoted
// `$(cat FILE)` and word-splits the file contents — the harness only
// receives the first word.
//
// Fix: stop relying on shell expansion entirely; embed the prompt
// directly as a bash-single-quoted argv element via the new
// buildHarnessSpawnArgv helper. Single quotes are inert at every layer
// of quote-mangling (Windows, AppleScript, bash) so any prompt
// content survives.
// --------------------------------------------------------------------------

// The prompt argv element must be a single bash-single-quoted token.
// Multi-line content must round-trip through `bash -lc <script>` as
// one positional arg — that's what the harness expects, and it's what
// the old $(cat) approach was meant to provide before wt.exe broke it.
func TestBuildHarnessSpawnArgv_PromptInlinedAsSingleQuotedArg(t *testing.T) {
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  "You are an agent.\nLine two has multiple words.",
	}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	want := "'You are an agent.\nLine two has multiple words.'"
	last := argv[len(argv)-1]
	if last != want {
		t.Errorf("prompt argv element = %q,\n                  want %q", last, want)
	}
}

// Hard contract: never use `$(cat FILE)` again. The whole reason this
// helper exists is to avoid shell expansion of the prompt; any
// regression to a $(cat …) shape would reintroduce the WSL truncation.
func TestBuildHarnessSpawnArgv_DoesNotUseCatExpansion(t *testing.T) {
	spec := agentSpec{harness: "claude", model: "opus", prompt: "anything"}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	for i, a := range argv {
		if strings.Contains(a, "$(cat") {
			t.Errorf("argv[%d]=%q must not use $(cat — wt.exe strips outer quotes and bash word-splits, truncating the prompt to its first word", i, a)
		}
	}
}

// Embedded single quotes need the standard close-escape-reopen
// pattern: `it's mine` → `'it'\''s mine'`. Apostrophes in prompts are
// common ("don't", "user's", contractions) so this needs to be
// boringly correct.
func TestBuildHarnessSpawnArgv_EscapesEmbeddedSingleQuote(t *testing.T) {
	spec := agentSpec{harness: "claude", model: "opus", prompt: "it's mine"}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	want := `'it'\''s mine'`
	last := argv[len(argv)-1]
	if last != want {
		t.Errorf("prompt argv element = %q,\n                  want %q", last, want)
	}
}

// Empty prompt → no trailing positional argv element. Mirrors
// buildAgentArgv's behaviour so the harness boots into its REPL with
// no canned message.
func TestBuildHarnessSpawnArgv_OmitsPromptWhenEmpty(t *testing.T) {
	spec := agentSpec{harness: "claude", model: "opus"}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	want := []string{"claude", "--dangerously-skip-permissions", "--model", "opus"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
}

// Non-claude harnesses follow buildAgentArgv's shape (no auto
// --dangerously-skip-permissions) but get the same single-quote
// treatment for the prompt.
func TestBuildHarnessSpawnArgv_CodexPromptAlsoQuoted(t *testing.T) {
	spec := agentSpec{harness: "codex", model: "gpt-5", prompt: "hello"}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	want := []string{"codex", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-5", "'hello'"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
}

// Shell metacharacters ($expansions, `backticks`) inside a prompt
// must reach the harness as literal text, not be evaluated by bash on
// the way in. Single-quoting is the mechanism — bash treats every
// character inside '...' as literal, including $ and `. Locks in the
// behaviour so a future move to double-quoting (which would expand
// $HOME and execute `pwd`) fails fast.
func TestBuildHarnessSpawnArgv_PreservesShellMetacharacters(t *testing.T) {
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  "$HOME and `pwd`",
	}
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	want := "'$HOME and `pwd`'"
	last := argv[len(argv)-1]
	if last != want {
		t.Errorf("prompt argv element = %q,\n                  want %q", last, want)
	}
}

// containsAdjacent reports whether xs contains a then b at adjacent
// indices. Used to assert flag/value pairs in the Linux/WSL argv tests
// without forcing a brittle exact-slice match.
func containsAdjacent(xs []string, a, b string) bool {
	for i := 0; i+1 < len(xs); i++ {
		if xs[i] == a && xs[i+1] == b {
			return true
		}
	}
	return false
}
