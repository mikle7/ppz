package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// the harness inside the wrapped pty.
func runAgentInForeground(handle string, spec agentSpec) error {
	argv, err := buildAgentArgv(spec)
	if err != nil {
		return err
	}
	shareArgs := append([]string{handle, "--"}, argv...)
	return cmdTerminalShare(shareArgs)
}

// runAgentInNewWindow writes the prompt to $TMPDIR/ppz-agent-<handle>-
// prompt.txt and asks osascript to open a new Terminal/iTerm window
// running `ppz terminal share <handle> -- <harness> [...] "$(cat
// FILE)"`. We use a temp file (not direct shell quoting) so prompts
// containing newlines, quotes, $-expansions, and backticks all survive
// untouched.
func runAgentInNewWindow(handle string, spec agentSpec) error {
	promptPath := filepath.Join(os.TempDir(), "ppz-agent-"+handle+"-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(spec.prompt), 0o600); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}

	// Replace the literal prompt in the spec with a shell expansion of
	// the temp file. The harness binary receives the dereferenced
	// content as a single argv element after the shell expands it.
	specForShell := spec
	if spec.prompt != "" {
		specForShell.prompt = `"$(cat ` + promptPath + `)"`
	}
	argv, err := buildAgentArgv(specForShell)
	if err != nil {
		return err
	}

	script := buildNewWindowScript(os.Getenv("TERM_PROGRAM"), handle, argv)
	cmd := exec.Command("osascript", "-e", script)
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

// defaultAgentPrompt is sent when the user supplies no positional prompt
// and no --prompt-file. Keep this short and ppz-aware so the agent has
// orientation as soon as the harness boots.
const defaultAgentPrompt = `You are an agent running inside a ppz (pipes) pty. Your terminal output is published to <handle>.stdout. Other agents can reach you via <handle>.inbox.

Useful commands:
  ppz status                find out which source you are
  ppz ls                    list sources × pipes
  ppz read inbox            read new messages addressed to you
  ppz read inbox --tail     follow your inbox live
  ppz send <handle> <text>  send a message to another agent
  ppz broadcast -m <text>   broadcast to your source's broadcast pipe

Poll your inbox periodically while waiting for instructions.`

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
	case "codex", "copilot", "gemini", "pi":
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
		prompt = defaultAgentPrompt
	}

	return agentSpec{harness: harness, model: model, prompt: prompt, newWindow: fNewWindow}, handle, nil
}

// buildNewWindowScript returns the osascript fragment that opens a new
// terminal window (Terminal.app or iTerm2 depending on TERM_PROGRAM)
// running `ppz terminal share <handle> -- <argv...>`. The argv is joined
// with spaces; multi-line/special-char prompts must be referenced via
// `$(cat /tmp/...)` to avoid shell-quoting nightmares — see
// cmdAgentCreate's --new-window path which writes a temp file.
func buildNewWindowScript(termProgram, handle string, argv []string) string {
	cmd := "ppz terminal share " + handle + " -- " + strings.Join(argv, " ")
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
