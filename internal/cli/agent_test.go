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
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", []string{"claude", "-p", "hello"})
	if !strings.Contains(cmd, "ppz terminal share alice --") {
		t.Errorf("expected ppz share prefix, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "tell application \"Terminal\"") {
		t.Errorf("expected Terminal.app osascript, got:\n%s", cmd)
	}
}

func TestBuildNewWindowScript_ITerm2Detected(t *testing.T) {
	cmd := buildNewWindowScript("iTerm.app", "alice", "", []string{"claude"})
	if !strings.Contains(cmd, "tell application \"iTerm\"") {
		t.Errorf("expected iTerm osascript, got:\n%s", cmd)
	}
}

// Prompt strings can contain newlines and single/double quotes. The
// builder writes the prompt to a temp file and dereferences it with
// $(cat …) so we never have to shell-quote the content.
func TestBuildNewWindowScript_PromptFileDereferenced(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "",
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
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "/Users/jimmy/work", []string{"claude"})
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
	cmd := buildNewWindowScript("iTerm.app", "alice", "/Users/jimmy/work", []string{"claude"})
	if !strings.Contains(cmd, `cd '/Users/jimmy/work'`) {
		t.Errorf("iTerm path must include cd, got:\n%s", cmd)
	}
}

// Empty cwd → no cd prefix. Lets tests + callers that genuinely don't
// care about cwd opt out (and gives `os.Getwd` a graceful fallback if
// it fails).
func TestBuildNewWindowScript_EmptyCwdSkipsCd(t *testing.T) {
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", []string{"claude"})
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
	cmd := buildNewWindowScript("Apple_Terminal", "alice", `/path/with'quote`, []string{"claude"})
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
	argv, err := buildLinuxNewWindowArgv("gnome-terminal", "alice", "", []string{"claude"})
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
	argv, err := buildLinuxNewWindowArgv("konsole", "alice", "", []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if argv[0] != "konsole" || !containsAdjacent(argv, "-e", "bash") {
		t.Errorf("konsole must use `-e bash`, got: %q", argv)
	}
}

func TestBuildLinuxNewWindowArgv_XtermUsesDashE(t *testing.T) {
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", []string{"claude"})
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
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", []string{"claude", "-p", "hi"})
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
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "/home/jamesmiles/work", []string{"claude"})
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
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", []string{"claude"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if strings.Contains(script, "cd ") {
		t.Errorf("empty cwd must not produce a cd prefix, got: %q", script)
	}
}

func TestBuildLinuxNewWindowArgv_UnknownTerminalErrors(t *testing.T) {
	if _, err := buildLinuxNewWindowArgv("nonesuch", "alice", "", []string{"claude"}); err == nil {
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
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", []string{"claude"})
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
	argv, err := buildWSLNewWindowArgv("Ubuntu-22.04", "alice", "", []string{"claude"})
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
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", []string{"claude", "-p", "hi"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	script := argv[len(argv)-1]
	if !strings.Contains(script, "ppz terminal share alice -- claude") {
		t.Errorf("script missing ppz share invocation, got: %q", script)
	}
}

func TestBuildWSLNewWindowArgv_PrependsCdToCallersCwd(t *testing.T) {
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "/home/jamesmiles/work", []string{"claude"})
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
	if _, err := buildWSLNewWindowArgv("", "alice", "", []string{"claude"}); err == nil {
		t.Fatal("expected error for empty distro (caller forgot $WSL_DISTRO_NAME)")
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
