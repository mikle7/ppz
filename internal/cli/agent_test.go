package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"
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

// Codex with a model passthrough — no auto-prepended permissions flag,
// no model default; whatever model the user gave is forwarded as-is.
func TestBuildAgentArgv_CodexWithModel(t *testing.T) {
	got, err := buildAgentArgv(agentSpec{harness: "codex", model: "gpt-5", prompt: "go"})
	if err != nil {
		t.Fatalf("buildAgentArgv: %v", err)
	}
	want := []string{"codex", "--model", "gpt-5", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_CodexNoModel(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "codex", prompt: "go"})
	want := []string{"codex", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_GeminiWithModel(t *testing.T) {
	got, _ := buildAgentArgv(agentSpec{harness: "gemini", model: "2.5-pro", prompt: "go"})
	want := []string{"gemini", "--model", "2.5-pro", "go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildAgentArgv_CopilotAndPi(t *testing.T) {
	for _, h := range []string{"copilot", "pi"} {
		got, err := buildAgentArgv(agentSpec{harness: h, prompt: "go"})
		if err != nil {
			t.Fatalf("%s: %v", h, err)
		}
		if got[0] != h {
			t.Fatalf("%s: argv[0]=%q, want %q", h, got[0], h)
		}
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

// TestDefaultAgentPrompt_OmitsRemovedBroadcastVerb keeps `ppz broadcast`
// (removed in v0.30.0 — see tests/broadcast/broadcast-returns-unknown-command)
// from creeping back into the spawn-time orientation. An agent reading
// the prompt and trying the command would hit `unknown command, exit 2`
// and either retry-loop or hallucinate a workaround.
func TestDefaultAgentPrompt_OmitsRemovedBroadcastVerb(t *testing.T) {
	if strings.Contains(defaultAgentPrompt("test-handle"), "ppz broadcast") {
		t.Errorf("defaultAgentPrompt references the removed `ppz broadcast` verb; agents will hit `unknown command` if they try it")
	}
}

// TestDefaultAgentPrompt_MentionsLsWatch pins `ppz ls --watch` as
// the recommended inbox-awareness primitive for the Monitor pattern.
// It blocks until any pipe has unread, prints a snapshot, and exits
// without advancing any cursor — which is what a watch wants. The
// previous recommendation (`ppz await`) drains as it follows, so a
// Monitor wired to await races any later `ppz read inbox` and the
// user-visible bug is "the agent claims it acted but my read shows
// nothing".
func TestDefaultAgentPrompt_MentionsLsWatch(t *testing.T) {
	if !strings.Contains(defaultAgentPrompt("test-handle"), "ppz ls --watch") {
		t.Errorf("defaultAgentPrompt should reference `ppz ls --watch` — the non-destructive blocking-watch primitive used by the Monitor recipe")
	}
}

// TestDefaultAgentPrompt_MentionsWho pins `ppz who` in the cheat
// sheet. Without it, an agent reading the prompt knows how to list
// pipes (`ppz ls`) and message peers (`ppz send <handle>`) but has
// no documented way to discover *which* handles exist — agents were
// observed inventing handles or asking the user, instead of running
// the verb the daemon already exposes.
func TestDefaultAgentPrompt_MentionsWho(t *testing.T) {
	if !strings.Contains(defaultAgentPrompt("test-handle"), "ppz who") {
		t.Errorf("defaultAgentPrompt should reference `ppz who` so agents can discover which peers are online before trying to `ppz send`")
	}
}

// TestDefaultAgentPrompt_OmitsAwait — keep `ppz await` out of the
// boot prompt. It's still a valid verb when the agent actively wants
// to drain, but mentioning it in the useful-commands cheat sheet led
// agents to wire it into a persistent Monitor, where it silently ate
// inbox messages the user then asked them to `ppz read`. The watch
// vs. read concerns belong on different verbs.
func TestDefaultAgentPrompt_OmitsAwait(t *testing.T) {
	if strings.Contains(defaultAgentPrompt("test-handle"), "ppz await") {
		t.Errorf("defaultAgentPrompt must not mention `ppz await` — destructive read races `ppz read inbox`; use `ppz ls --watch` for awareness and `ppz read` for consumption")
	}
}

// TestDefaultAgentPrompt_SubstitutesHandle pins the handle template
// substitution. The prompt is built per-spawn with the actual handle
// so the Monitor recipe can hard-code PPZ_SESSION=<handle> inline.
// A regression to a const prompt would leave `<handle>` as a literal
// placeholder in the recipe — the agent would then run a Monitor
// keyed by the string "<handle>" instead of e.g. "eve".
func TestDefaultAgentPrompt_SubstitutesHandle(t *testing.T) {
	prompt := defaultAgentPrompt("alice")
	if !strings.Contains(prompt, `"alice"`) {
		t.Errorf("defaultAgentPrompt(\"alice\") should mention the handle literally; got: %q", prompt)
	}
	if strings.Contains(prompt, "<handle>.stdout") {
		t.Errorf("defaultAgentPrompt should substitute the handle into `.stdout` / `.inbox` references, not leave the `<handle>` placeholder; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_MonitorRecipeThrottlesLoop — the Monitor
// recipe must include a sleep on the success path. `ppz ls --watch`
// is non-destructive: once a pipe has unread, every immediate re-arm
// returns immediately with the same snapshot. Without the throttle
// the loop spins as fast as the daemon can answer, flooding the
// agent with duplicate events for the same unread state until it
// runs `ppz read` to clear them. A trailing `sleep 60` between
// iterations keeps the duplicate-event window bounded.
func TestDefaultAgentPrompt_MonitorRecipeThrottlesLoop(t *testing.T) {
	prompt := defaultAgentPrompt("eve")
	if !strings.Contains(prompt, "sleep 60") {
		t.Errorf("defaultAgentPrompt Monitor recipe must throttle the loop with `sleep 60` so non-destructive ls --watch doesn't spin on persistent unread; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_MonitorRecipePinsSession — the Monitor
// recipe must set PPZ_SESSION=<handle> inline. Inheriting the parent
// shell's PPZ_SESSION is unreliable across Claude Code versions; we
// observed v2.1.143 dropping it on Monitor's bash subprocess, which
// then resolved a fresh tty-less session id the daemon had never
// seen and failed every ppz call with E_NO_CURRENT_SOURCE. Setting
// PPZ_SESSION inline in the recipe makes the watch robust to that
// behaviour.
func TestDefaultAgentPrompt_MonitorRecipePinsSession(t *testing.T) {
	prompt := defaultAgentPrompt("eve")
	if !strings.Contains(prompt, "PPZ_SESSION=eve ppz ls --watch") {
		t.Errorf("defaultAgentPrompt Monitor recipe should set PPZ_SESSION=<handle> inline so it survives env-strip on Monitor subprocesses; got: %q", prompt)
	}
}

// TestDefaultAgentPrompt_UsesUncollaredTerminology fixes a "uncoloured"
// → "uncollared" typo. The wire vocabulary in WIRE.md §1 is "collared"
// (source-bound) vs "uncollared" (sourceless, e.g. chat-room pipes).
// Mis-spelling it leaves an agent unable to grep / Ctrl-F into the
// actual docs and tests.
func TestDefaultAgentPrompt_UsesUncollaredTerminology(t *testing.T) {
	if strings.Contains(defaultAgentPrompt("test-handle"), "uncoloured") {
		t.Errorf("defaultAgentPrompt has the `uncoloured` typo; wire vocab is `uncollared` (WIRE.md §1)")
	}
}

// TestDefaultAgentPrompt_CommandColumnIsAligned walks every "  ppz …"
// line in the prompt and asserts that the description begins at the
// same column on every row. Mis-aligned columns aren't a correctness
// bug, but the prompt is a man-page-style cheat sheet — a drifting
// column makes it harder to scan and signals "nobody runs this through
// a check". Allowing per-row variance lets a future edit silently
// undo today's alignment work.
func TestDefaultAgentPrompt_CommandColumnIsAligned(t *testing.T) {
	descCol := -1
	for i, line := range strings.Split(defaultAgentPrompt("test-handle"), "\n") {
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
		t.Fatalf("defaultAgentPrompt has no `  ppz …` lines to align")
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
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", nil, []string{"claude"})
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
	argv, err := buildWSLNewWindowArgv("Ubuntu-22.04", "alice", "", nil, []string{"claude"})
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

func TestBuildWSLNewWindowArgv_IncludesShareInvocation(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", nil, []string{"claude", "-p", "hi"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, "ppz terminal share alice -- claude") {
		t.Errorf("script missing ppz share invocation, got: %q", script)
	}
}

func TestBuildWSLNewWindowArgv_PrependsCdToCallersCwd(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "/home/jamesmiles/work", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, `cd '/home/jamesmiles/work'`) {
		t.Errorf("script missing cd to caller's cwd, got: %q", script)
	}
}

// Empty distro is a caller bug — $WSL_DISTRO_NAME was unset. Better to
// error loudly than to silently fall through to the user's default
// distro (which may be the wrong one).
func TestBuildWSLNewWindowArgv_EmptyDistroErrors(t *testing.T) {
	if _, err := buildWSLNewWindowArgv("", "alice", "", nil, []string{"claude"}); err == nil {
		t.Fatal("expected error for empty distro (caller forgot $WSL_DISTRO_NAME)")
	}
}

// TestBuildWSLNewWindowArgv_EscapesSemicolonsForWtExe pins the wt.exe
// argv-tokenisation contract: wt.exe (Windows Terminal) uses `;` as a
// sub-command separator in its own command line, so any literal `;` we
// want bash to receive downstream must be escaped as `\;` before being
// placed in the wt.exe argv.
//
// The bug this guards against: `ppz agent create <handle> --new-window`
// on WSL invokes the default agent prompt, whose Monitor recipe is
// `while true; do PPZ_SESSION=<handle> ppz ls --watch 2>/dev/null;
// sleep 60; done` — three literal semicolons inside a bash-single-
// quoted prompt argument. Without escaping, wt.exe sees those `;`s and
// truncates the script at the first one, then tries to launch each
// subsequent chunk (` do PPZ_SESSION=…`, ` sleep 60`, ` done that fires
// a PushNotification…`) as a separate Windows program. The user sees
// `[error 2147942402 (0x80070002) when launching …] The system cannot
// find the file specified.` in four sibling tabs, plus the first tab's
// bash reports `unexpected EOF while looking for matching ` because the
// truncation severs the Monitor recipe inside its opening backtick.
//
// Realistic input: round-trip the default agent prompt through
// buildHarnessSpawnArgv (which bash-single-quotes the prompt for
// inline embedding) and then buildWSLNewWindowArgv. The final argv
// element is the bash script handed to `bash -lc`; every `;` in it
// must be backslash-escaped for wt.exe.
func TestBuildWSLNewWindowArgv_EscapesSemicolonsForWtExe(t *testing.T) {
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  defaultAgentPrompt("alice"),
	}
	harnessArgv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", agentEnvPairs(spec), harnessArgv)
	if err != nil {
		t.Fatalf("buildWSLNewWindowArgv: %v", err)
	}
	script := argv[len(argv)-1]
	// Sanity: the Monitor recipe's three semicolons should be present
	// in some form. If they aren't we've lost the prompt entirely and
	// any "no unescaped ;" assertion below is a false positive.
	if !strings.Contains(script, "while true") || !strings.Contains(script, "sleep 60") {
		t.Fatalf("script does not contain the default prompt's Monitor recipe; test setup is wrong, got: %q", script)
	}
	for i := 0; i < len(script); i++ {
		if script[i] != ';' {
			continue
		}
		if i == 0 || script[i-1] != '\\' {
			t.Errorf("script contains an unescaped `;` at byte %d — wt.exe will split the argv here and Windows will try to launch the trailing chunk as a program; got: %q", i, script)
			return
		}
	}
}

// TestBuildWSLNewWindowArgv_EscapesSemicolonsInUserPrompt covers the
// non-default-prompt case: a user runs `ppz agent create alice
// --new-window "do X; then Y"`. The `;` belongs to the user's prompt
// text — they did not type it as a shell separator — but wt.exe will
// still see it once we've inlined the prompt into the bash script, and
// will still split. The escape contract must hold regardless of where
// the `;` came from, not just for our baked-in defaultAgentPrompt.
//
// Distinct from the previous test in two ways:
//
//  1. The `;` is in a user-supplied prompt, not the orientation prompt
//     — so a future rewrite that drops `;` from defaultAgentPrompt
//     won't accidentally let this regress.
//  2. The argv is constructed via buildHarnessSpawnArgv (the same path
//     the foreground CLI uses), which bash-single-quotes the prompt.
//     We're asserting that the wt.exe escape applies *outside* the
//     bash single-quoting — bash unescapes nothing inside single
//     quotes, so without the wt.exe-layer escape, the `;` rides
//     through the single quote intact and trips wt.exe's tokeniser.
func TestBuildWSLNewWindowArgv_EscapesSemicolonsInUserPrompt(t *testing.T) {
	spec := agentSpec{
		harness: "claude",
		model:   "opus",
		prompt:  "do X; then Y; finally Z",
	}
	harnessArgv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		t.Fatalf("buildHarnessSpawnArgv: %v", err)
	}
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", agentEnvPairs(spec), harnessArgv)
	if err != nil {
		t.Fatalf("buildWSLNewWindowArgv: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, "do X") || !strings.Contains(script, "finally Z") {
		t.Fatalf("script does not contain the user prompt; test setup is wrong, got: %q", script)
	}
	for i := 0; i < len(script); i++ {
		if script[i] != ';' {
			continue
		}
		if i == 0 || script[i-1] != '\\' {
			t.Errorf("script contains an unescaped `;` at byte %d (from a user-supplied prompt) — wt.exe will split the argv here; got: %q", i, script)
			return
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
// shell semantics. Without -lc this reproducibly fails with
// "exec: claude: executable file not found in $PATH".
func TestBuildWSLNewWindowArgv_UsesLoginShell(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", nil, []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !containsAdjacent(argv, "bash", "-lc") {
		t.Errorf("WSL path must invoke `bash -lc`, got: %q", argv)
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
	want := []string{"codex", "--model", "gpt-5", "'hello'"}
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
