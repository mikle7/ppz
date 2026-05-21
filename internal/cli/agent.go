package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// cmdAgentGroup dispatches `ppz agent <subverb>`. Today only `create`
// exists; the verb is grouped so we can grow it (destroy, list, etc.)
// without re-shaping the CLI surface.
func cmdAgentGroup(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz agent create <name> [<prompt>] [flags...]")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		return cmdAgentCreate(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz agent: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// cmdAgentCreate is the wired-up command. Two execution paths:
//
//  1. Default (foreground): build the harness argv and call cmdTerminalShare
//     directly with `<handle> -- <argv>`. The current shell becomes the
//     agent's controlling terminal — Ctrl-C exits the agent.
//  2. --new-window: write the prompt to a temp file, build a `ppz terminal
//     share <handle> -- <harness> ... "$(cat <file>)"` shell command, and
//     hand it to osascript so a fresh Terminal.app/iTerm window runs it.
//     Returns immediately so the parent shell stays usable.
//
// Either way, `ppz terminal share` itself creates the source as KindPTY —
// we don't pre-create it. That matches the demo (../ppz-demo-1/setup.sh)
// which just runs `ppz terminal share <name>` per agent.
func cmdAgentCreate(args []string) error {
	spec, handle, err := resolveAgentSpec(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ppz agent create:", err)
		os.Exit(2)
	}
	if spec.newWindow {
		return runAgentInNewWindow(handle, spec)
	}
	return runAgentInForeground(handle, spec)
}

// runAgentInForeground hands off to cmdTerminalShare in-process. The
// argv shape is `<handle> -- <harness-argv...>` so terminal share runs
// the harness inside the wrapped pty. Before handing off, the agent
// identity env (PPZ_AGENT_HARNESS / PPZ_AGENT_MODEL) is exported into
// the current process so terminalShareEnv picks it up via os.Environ()
// — that's how heartbeats learn what harness they're stamping.
func runAgentInForeground(handle string, spec agentSpec) error {
	argv, err := buildAgentArgv(spec)
	if err != nil {
		return err
	}
	setAgentEnv(spec)
	shareArgs := append([]string{handle, "--"}, argv...)
	return cmdTerminalShare(shareArgs)
}

// agentEnvPairs returns the agent-identity env-var assignments the pty
// wrapper reads to stamp every heartbeat. Always two keys so the wire
// schema stays consistent regardless of harness; model may be empty
// when the agent harness has no default (e.g. copilot/codex without
// --model).
func agentEnvPairs(spec agentSpec) []string {
	return []string{
		"PPZ_AGENT_HARNESS=" + spec.harness,
		"PPZ_AGENT_MODEL=" + spec.model,
	}
}

// setAgentEnv exports agentEnvPairs into the current process env. The
// foreground path relies on this — cmdTerminalShare is called in-
// process, and terminalShareEnv appends os.Environ() to the wrapped
// child's env, so the agent identity flows through transitively. The
// new-window path can't share process state with the spawned terminal,
// so it injects the same pairs as a shell `env KEY=VAL ...` prefix
// instead — see runAgentInNewWindow.
func setAgentEnv(spec agentSpec) {
	for _, kv := range agentEnvPairs(spec) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

// runAgentInNewWindow asks the host's window-manager to open a new
// terminal running `ppz terminal share <handle> -- <harness>
// [...] '<prompt>'`. The prompt is bash-single-quoted inline via
// buildHarnessSpawnArgv — see that helper for why the previous
// `"$(cat FILE)"` temp-file round-trip was abandoned.
//
// Backend per platform:
//   - darwin: osascript → Terminal.app / iTerm2 (see buildNewWindowScript)
//   - linux (WSL):   wt.exe + wsl.exe (see buildWSLNewWindowArgv)
//   - linux (native): $TERMINAL or probed emulator (see buildLinuxNewWindowArgv)
func runAgentInNewWindow(handle string, spec agentSpec) error {
	argv, err := buildHarnessSpawnArgv(spec)
	if err != nil {
		return err
	}

	// Inherit the parent shell's cwd so the spawned harness boots in
	// the folder the user already trusts. Without this, the new
	// terminal opens in $HOME and claude shows a "trust this folder?"
	// dialog on every run.
	cwd, _ := os.Getwd()
	envPairs := agentEnvPairs(spec)

	switch runtime.GOOS {
	case "darwin":
		script := buildNewWindowScript(os.Getenv("TERM_PROGRAM"), handle, cwd, envPairs, argv)
		return runWindowCmd("osascript", []string{"-e", script})
	case "linux":
		procVersion, _ := os.ReadFile("/proc/version")
		if isWSL(string(procVersion)) {
			body := buildWSLScript(handle, cwd, envPairs, argv)
			scriptPath, err := writeAgentSpawnScript("", handle, body)
			if err != nil {
				return err
			}
			cmdArgv, err := buildWSLNewWindowArgv(os.Getenv("WSL_DISTRO_NAME"), scriptPath)
			if err != nil {
				return err
			}
			return runWindowCmd(cmdArgv[0], cmdArgv[1:])
		}
		terminal, err := selectLinuxTerminal(os.Getenv("TERMINAL"), func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		})
		if err != nil {
			return err
		}
		cmdArgv, err := buildLinuxNewWindowArgv(terminal, handle, cwd, envPairs, argv)
		if err != nil {
			return err
		}
		return runWindowCmd(cmdArgv[0], cmdArgv[1:])
	}
	return fmt.Errorf("--new-window: unsupported platform %q", runtime.GOOS)
}

// runWindowCmd is a thin wrapper around exec.Command that wires
// stdout/stderr to the parent — used by all three --new-window
// backends so they handle process-spawn the same way.
func runWindowCmd(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// agentSpec is the resolved input to buildAgentArgv. The CLI flag parser
// resolves --claude/--codex/etc into harness, --opus/--sonnet/--haiku and
// --model into a single model string, and the positional prompt or
// --prompt-file into prompt.
type agentSpec struct {
	harness   string
	model     string
	prompt    string
	newWindow bool
}

// defaultAgentPrompt returns the orientation prompt sent when the user
// supplies no positional prompt and no --prompt-file. Templated on the
// handle so the inbox-watch recipe can pin PPZ_SESSION=<handle> inline,
// and dispatched on harness so each harness gets a prompt that names its
// own primitives (claude → Monitor + PushNotification; copilot → bash
// detach: true; codex/gemini/pi → foreground `ppz ls --watch` loop).
//
// Inheriting PPZ_SESSION from the parent shell is unreliable: some
// harness/Monitor combinations don't propagate env to subprocesses
// (Claude Code v2.1.143 was observed dropping it on Monitor's bash),
// and a Monitor subprocess with no PPZ_SESSION resolves a fresh
// tty-less session id the daemon has never seen — every ppz call
// inside then fails E_NO_CURRENT_SOURCE. Setting PPZ_SESSION inline
// in the recipe makes it robust to any future env-strip behavior.
func defaultAgentPrompt(handle, harness string) string {
	_ = harness // scaffold: harness branching lands in the follow-up commit
	return `You are an agent running inside a ppz (pipes) pty. Your handle is "` + handle + `". Your terminal output is published to ` + handle + `.stdout. Other agents can reach you via ` + handle + `.inbox.

Useful commands:
  ppz status                find out which source you are
  ppz who                   see which other agents are online
  ppz ls                    list sources × pipes
  ppz ls --watch            block until any pipe has unread, then print a snapshot (non-destructive)
  ppz read inbox            read new messages addressed to you
  ppz read <pipe>           read new messages from a pipe (e.g. a chat room)
  ppz send <handle> <text>  send a message to another agent
  ppz send <pipe> <text>    send a message to a pipe

Create a persistent Monitor running ` + "`while true; do PPZ_SESSION=" + handle + " ppz ls --watch 2>/dev/null; sleep 60; done`" + ` that fires a PushNotification on each new message arrival. PPZ_SESSION is set inline so the subprocess works even if env isn't inherited. The trailing sleep 60 throttles the loop: ls --watch is non-destructive, so without it the loop would re-fire immediately on every iteration as long as any pipe still has unread, flooding you with duplicate events until you ` + "`ppz read`" + ` to clear them.`
}

// buildHarnessSpawnArgv returns the harness argv for the --new-window
// spawn path. Identical to buildAgentArgv except the prompt element is
// bash-single-quoted so it inlines safely into the `bash -lc <script>`
// invocation we hand to the spawned terminal.
//
// Why not the old `"$(cat FILE)"` round-trip: when the WSL backend
// invokes wt.exe, Windows' argv parser sees our command line and
// strips the outer double quotes around the expansion before wt.exe
// hands the remainder to bash. Bash then sees unquoted `$(cat FILE)`
// and word-splits the file contents — the harness ends up receiving
// only the first word of the prompt (reproduced on this WSL2 box: a
// multi-line prompt arrived as the literal string "You").
// Single-quoting avoids the round-trip entirely: single quotes are
// inert at every layer (Windows' argv parser, AppleScript, bash) so
// any prompt content survives intact.
func buildHarnessSpawnArgv(spec agentSpec) ([]string, error) {
	quoted := spec
	if spec.prompt != "" {
		quoted.prompt = bashSingleQuote(spec.prompt)
	}
	return buildAgentArgv(quoted)
}

// buildAgentArgv returns the argv that runs *inside* the wrapped pty
// (i.e. the part after the `--` to `ppz terminal share`). It does not
// include `ppz terminal share <handle> --` — that prefix is the caller's
// responsibility (either via cmdTerminalShare directly, or as a string
// composed for osascript).
func buildAgentArgv(spec agentSpec) ([]string, error) {
	switch spec.harness {
	case "claude":
		argv := []string{"claude", "--dangerously-skip-permissions"}
		if spec.model != "" {
			argv = append(argv, "--model", spec.model)
		}
		if spec.prompt != "" {
			argv = append(argv, spec.prompt)
		}
		return argv, nil
	case "copilot":
		// copilot rejects a positional prompt with "Invalid command
		// format" — it tries to dispatch the first positional as a
		// subcommand. The initial prompt must arrive via `-i <prompt>`
		// (interactive mode with prompt). --yolo enables all
		// permissions so the unattended agent can act without per-tool
		// approval prompts.
		argv := []string{"copilot", "--yolo"}
		if spec.model != "" {
			argv = append(argv, "--model", spec.model)
		}
		if spec.prompt != "" {
			argv = append(argv, "-i", spec.prompt)
		}
		return argv, nil
	case "codex", "gemini", "pi":
		argv := []string{spec.harness}
		if spec.model != "" {
			argv = append(argv, "--model", spec.model)
		}
		if spec.prompt != "" {
			argv = append(argv, spec.prompt)
		}
		return argv, nil
	}
	return nil, fmt.Errorf("unknown harness %q", spec.harness)
}

// resolveAgentSpec parses the args passed to `ppz agent create` and
// produces (spec, handle, error). It handles:
//
//   - harness selection: --claude (default) | --copilot | --codex |
//     --gemini | --pi (mutually exclusive)
//   - model selection: claude shortcuts --opus / --sonnet / --haiku
//     (mutually exclusive, claude-only) OR --model X (any harness).
//     Combining a shortcut with --model errors.
//   - prompt selection: positional <prompt> argument OR --prompt-file
//     <path>. If neither is given, defaultAgentPrompt is used.
//
// The handle is the first positional argument; the (optional) prompt is
// the second.
func resolveAgentSpec(args []string) (agentSpec, string, error) {
	fs := flag.NewFlagSet("agent create", flag.ContinueOnError)
	fs.SetOutput(devNull{})

	var (
		fClaude, fCopilot, fCodex, fGemini, fPi bool
		fOpus, fSonnet, fHaiku                  bool
		fNewWindow                              bool
		fModel, fPromptFile                     string
	)
	fs.BoolVar(&fClaude, "claude", false, "use the claude harness (default)")
	fs.BoolVar(&fCopilot, "copilot", false, "use the copilot harness")
	fs.BoolVar(&fCodex, "codex", false, "use the codex harness")
	fs.BoolVar(&fGemini, "gemini", false, "use the gemini harness")
	fs.BoolVar(&fPi, "pi", false, "use the pi harness")
	fs.BoolVar(&fOpus, "opus", false, "claude shortcut: --model opus")
	fs.BoolVar(&fSonnet, "sonnet", false, "claude shortcut: --model sonnet")
	fs.BoolVar(&fHaiku, "haiku", false, "claude shortcut: --model haiku")
	fs.BoolVar(&fNewWindow, "new-window", false, "open a new Terminal.app/iTerm2 window via osascript")
	fs.StringVar(&fModel, "model", "", "model passed to the harness")
	fs.StringVar(&fPromptFile, "prompt-file", "", "read the prompt from a file")

	// Pre-split flags from positionals so flag order doesn't matter
	// (matches cmdCommand's pattern). --prompt-file and --model carry
	// values, so we step over those.
	valueFlags := map[string]bool{"--model": true, "-model": true, "--prompt-file": true, "-prompt-file": true}
	var flagArgs, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, a)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return agentSpec{}, "", err
	}

	// Harness mutual exclusion. Default to claude when none given.
	harnessFlags := []struct {
		set  bool
		name string
	}{
		{fClaude, "claude"}, {fCopilot, "copilot"}, {fCodex, "codex"},
		{fGemini, "gemini"}, {fPi, "pi"},
	}
	var picked []string
	for _, h := range harnessFlags {
		if h.set {
			picked = append(picked, h.name)
		}
	}
	var harness string
	switch len(picked) {
	case 0:
		harness = "claude"
	case 1:
		harness = picked[0]
	default:
		return agentSpec{}, "", fmt.Errorf("only one of --claude/--copilot/--codex/--gemini/--pi may be set; got %v", picked)
	}

	// Claude model-shortcut mutual exclusion.
	shortcutCount := 0
	var shortcutModel string
	for _, s := range []struct {
		set  bool
		name string
	}{{fOpus, "opus"}, {fSonnet, "sonnet"}, {fHaiku, "haiku"}} {
		if s.set {
			shortcutCount++
			shortcutModel = s.name
		}
	}
	if shortcutCount > 1 {
		return agentSpec{}, "", fmt.Errorf("only one of --opus/--sonnet/--haiku may be set")
	}
	if shortcutCount == 1 && fModel != "" {
		return agentSpec{}, "", fmt.Errorf("--%s and --model are mutually exclusive", shortcutModel)
	}
	if shortcutCount == 1 && harness != "claude" {
		return agentSpec{}, "", fmt.Errorf("--%s is claude-only; use --model with --%s", shortcutModel, harness)
	}

	// Resolve final model string.
	model := fModel
	if shortcutModel != "" {
		model = shortcutModel
	}
	if model == "" && harness == "claude" {
		model = "opus" // documented default per CLI design
	}

	// Positional: <handle> [<prompt>].
	if len(rest) == 0 {
		return agentSpec{}, "", fmt.Errorf("missing handle: ppz agent create <name> [<prompt>] [flags...]")
	}
	handle := rest[0]
	var positionalPrompt string
	if len(rest) > 1 {
		positionalPrompt = strings.Join(rest[1:], " ")
	}

	if positionalPrompt != "" && fPromptFile != "" {
		return agentSpec{}, "", fmt.Errorf("positional prompt and --prompt-file are mutually exclusive")
	}

	prompt := positionalPrompt
	if fPromptFile != "" {
		body, err := os.ReadFile(fPromptFile)
		if err != nil {
			return agentSpec{}, "", fmt.Errorf("--prompt-file %s: %w", fPromptFile, err)
		}
		prompt = string(body)
	}
	if prompt == "" {
		prompt = defaultAgentPrompt(handle, harness)
	}

	return agentSpec{harness: harness, model: model, prompt: prompt, newWindow: fNewWindow}, handle, nil
}

// buildNewWindowScript returns the osascript fragment that opens a new
// terminal window (Terminal.app or iTerm2 depending on TERM_PROGRAM)
// running `ppz terminal share <handle> -- <argv...>`. The argv is joined
// with spaces; multi-line/special-char prompts must be referenced via
// `$(cat /tmp/...)` to avoid shell-quoting nightmares — see
// cmdAgentCreate's --new-window path which writes a temp file.
//
// When cwd is non-empty the shell command is prefixed with
// `cd '<bash-quoted-cwd>' && ` so the spawned harness boots in the
// folder the parent shell was running in. macOS otherwise opens the new
// Terminal window in $HOME, and claude treats trust per-folder, so every
// run pops a "trust this folder?" dialog. Empty cwd is a graceful no-op.
func buildNewWindowScript(termProgram, handle, cwd string, envPairs []string, argv []string) string {
	cmd := shareInvocation(handle, envPairs, argv)
	if cwd != "" {
		cmd = "cd " + bashSingleQuote(cwd) + " && " + cmd
	}
	switch termProgram {
	case "iTerm.app":
		// iTerm2's "current session of current window" creates a new
		// session in the existing window unless we explicitly request a
		// new window. `create window with default profile` does the
		// right thing.
		return `tell application "iTerm"
    activate
    set newWindow to (create window with default profile)
    tell current session of newWindow to write text "` + escapeAppleScript(cmd) + `"
end tell`
	default:
		// Apple_Terminal and unknowns fall through to Terminal.app —
		// matches the demo's behaviour.
		return `tell application "Terminal"
    do script "` + escapeAppleScript(cmd) + `"
    activate
end tell`
	}
}

// shareInvocation builds the `[env KEY=VAL ...] ppz terminal share
// <handle> -- <argv...>` command string used by every --new-window
// backend. envPairs is the agent identity (PPZ_AGENT_HARNESS, etc.) —
// prepended via `env` so the spawned `ppz terminal share` sees the
// pairs in its environment regardless of the user's shell. Empty
// envPairs is a clean no-op for non-agent shares.
func shareInvocation(handle string, envPairs, argv []string) string {
	prefix := ""
	if len(envPairs) > 0 {
		prefix = "env " + strings.Join(envPairs, " ") + " "
	}
	return prefix + "ppz terminal share " + handle + " -- " + strings.Join(argv, " ")
}

// bashSingleQuote wraps s in bash-safe single quotes. Embedded single
// quotes are emitted as `'\''` (close-quote, escaped quote, reopen) —
// the standard pattern for shell-injecting an arbitrary string into a
// command line.
func bashSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeAppleScript quotes a string for embedding inside an AppleScript
// string literal. AppleScript only requires escaping `"` and `\`.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// devNull is an io.Writer that discards everything. Used to silence the
// flag package's default usage output during tests.
type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

// --- Linux & WSL --new-window support --------------------------------------

// linuxTerminalPriority is the probe order used by selectLinuxTerminal
// when $TERMINAL is unset. Higher-quality emulators come first; bare
// xterm is the penultimate fallback, x-terminal-emulator (Debian
// alternatives) is last so distros that ship it as a wrapper don't
// shadow a real GUI emulator that's also installed.
var linuxTerminalPriority = []string{
	"gnome-terminal", "konsole", "xfce4-terminal", "tilix",
	"wezterm", "kitty", "alacritty", "xterm", "x-terminal-emulator",
}

// selectLinuxTerminal chooses which terminal emulator to drive on Linux.
// If termEnv (typically $TERMINAL) is non-empty it wins; otherwise the
// caller-supplied availability probe is consulted in a fixed priority
// order. Returns ("", error) when no candidate is available.
func selectLinuxTerminal(termEnv string, available func(string) bool) (string, error) {
	if termEnv != "" {
		return termEnv, nil
	}
	for _, name := range linuxTerminalPriority {
		if available(name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no supported terminal emulator on PATH; tried %v", linuxTerminalPriority)
}

// buildLinuxNewWindowArgv returns the exec argv that opens a new
// <terminal> window running `ppz terminal share <handle> -- <argv...>`.
// cwd, when non-empty, is prepended as `cd '<cwd>' && ` so the spawned
// harness boots in the folder the parent shell was running in
// (matching the macOS path's claude-trust-folder fix).
func buildLinuxNewWindowArgv(terminal, handle, cwd string, envPairs []string, argv []string) ([]string, error) {
	script := shareInvocation(handle, envPairs, argv)
	if cwd != "" {
		script = "cd " + bashSingleQuote(cwd) + " && " + script
	}
	// `-lc` (login + command) makes bash source ~/.profile (and through
	// it ~/.bashrc) before running the script. Plain `-c` is non-login
	// and non-interactive, so it skips the user's PATH additions and
	// the spawned `claude` reads as "executable file not found".
	switch terminal {
	case "gnome-terminal":
		// gnome-terminal swallows trailing argv as its own options unless
		// `--` separates them from the inner command.
		return []string{"gnome-terminal", "--", "bash", "-lc", script}, nil
	case "konsole", "xfce4-terminal", "tilix", "alacritty", "xterm", "x-terminal-emulator":
		return []string{terminal, "-e", "bash", "-lc", script}, nil
	case "kitty":
		// kitty treats trailing argv as the command directly — no flag.
		return []string{"kitty", "bash", "-lc", script}, nil
	case "wezterm":
		return []string{"wezterm", "start", "--", "bash", "-lc", script}, nil
	}
	return nil, fmt.Errorf("unsupported terminal %q (supported: %v)", terminal, linuxTerminalPriority)
}

// isWSL reports whether the calling process is running under Windows
// Subsystem for Linux. The caller passes the contents of /proc/version
// so the function stays unit-testable. Both WSL1 and WSL2 tag their
// kernel string with "microsoft" (case varies between releases).
func isWSL(procVersion string) bool {
	return strings.Contains(strings.ToLower(procVersion), "microsoft")
}

// buildWSLScript returns the bash script body that boots the agent.
// Pure — no side effects. The caller writes the body to a tempfile via
// writeAgentSpawnScript and passes the path to buildWSLNewWindowArgv.
//
// We don't inline this script into the wt.exe argv anymore. wt.exe's
// argv tokenizer corrupts the script in two ways that together break
// every realistic agent prompt:
//
//  1. It treats `;` as a sub-command separator. The default agent
//     prompt's Monitor recipe (`while true; do ... ; sleep 60 ; done`)
//     contains three of them; wt.exe truncates the script at the first
//     one and launches the trailing chunks as standalone Windows
//     programs.
//  2. It collapses `''` (adjacent close-quote / open-quote) sequences,
//     which is exactly the middle of the standard bash single-quote
//     escape `'\''` (used by bashSingleQuote to embed a literal `'`
//     into a single-quoted string). A prompt containing `isn't`
//     becomes `isn'\''t` in the script, which wt.exe collapses to
//     `isn'\t'` — bash then sees an unmatched closing quote and hangs
//     at the PS2 continuation prompt. The "flashing cursor, no source
//     created" symptom on WSL.
//
// Routing the script through a tempfile means wt.exe only sees a
// benign path (no `;`, no `'`), and bash reads the script byte-for-
// byte from disk with its own quoting rules intact.
func buildWSLScript(handle, cwd string, envPairs []string, argv []string) string {
	script := shareInvocation(handle, envPairs, argv)
	if cwd != "" {
		script = "cd " + bashSingleQuote(cwd) + " && " + script
	}
	return script
}

// writeAgentSpawnScript writes the bash script body to a tempfile and
// returns its path. The written script self-cleans (rm -f) on EXIT so
// /tmp doesn't accumulate one-shot helpers. dir is the temp directory
// to use — empty means os.TempDir().
//
// The shebang isn't load-bearing (the script is invoked as `bash -l
// <path>`, which reads commands directly without honoring shebangs)
// but it's set so a user inspecting the leftover file knows what it
// is.
func writeAgentSpawnScript(dir, handle, body string) (string, error) {
	f, err := os.CreateTemp(dir, "ppz-agent-"+handle+"-*.sh")
	if err != nil {
		return "", fmt.Errorf("ppz agent spawn script: %w", err)
	}
	defer f.Close()
	content := "#!/bin/bash\ntrap 'rm -f -- " + bashSingleQuote(f.Name()) + "' EXIT\n" + body + "\n"
	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("ppz agent spawn script: %w", err)
	}
	if err := os.Chmod(f.Name(), 0o700); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("ppz agent spawn script: %w", err)
	}
	return f.Name(), nil
}

// buildWSLNewWindowArgv returns the exec argv that drives wt.exe
// (Windows Terminal) to open a new tab running `wsl.exe -d <distro>
// bash -l <scriptPath>`. The script must already be on disk — see
// writeAgentSpawnScript and the buildWSLScript comment for why the
// script can't be inlined into the argv.
//
// `-w 0` targets the current Windows Terminal window; `nt` opens a new
// tab inside it. Falls back to a new window if no WT instance exists
// yet. `bash -l <scriptPath>` runs the script as a *login* shell so it
// sources ~/.profile / ~/.bash_profile — most users' PATH additions
// for claude (npm-global, nvm, asdf, ~/.local/bin) live there and a
// plain non-login `bash <path>` would fail to find the binary.
func buildWSLNewWindowArgv(distro, scriptPath string) ([]string, error) {
	if distro == "" {
		return nil, fmt.Errorf("buildWSLNewWindowArgv: empty distro (set $WSL_DISTRO_NAME)")
	}
	if scriptPath == "" {
		return nil, fmt.Errorf("buildWSLNewWindowArgv: empty scriptPath")
	}
	return []string{"wt.exe", "-w", "0", "nt", "wsl.exe", "-d", distro, "bash", "-l", scriptPath}, nil
}
