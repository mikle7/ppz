package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// `ppz completion <bash|zsh>` emits a shell script the user sources from
// their rc file. The script registers a completion function that calls
// the hidden `ppz __complete` subcommand on every tab. The subcommand
// receives the partial word list and prints one candidate per line.
//
// Two-stage design (script ↔ __complete) keeps the heavy lifting in Go
// where we already have access to verb metadata + the daemon's pipe
// list. The shell script stays tiny and stable; rules can change in
// `__complete` without users re-sourcing.
//
// Setup (printed by `cmdCompletion` itself):
//
//	# bash:
//	eval "$(ppz completion bash)"
//	# zsh:
//	eval "$(ppz completion zsh)"
//
// Add to ~/.bashrc / ~/.zshrc to persist.

const completionBash = `# ppz bash completion
_ppz_complete() {
    local words=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    local IFS=$'\n'
    COMPREPLY=( $(ppz __complete -- "${words[@]}" 2>/dev/null) )
    return 0
}
complete -F _ppz_complete ppz
`

const completionZsh = `# ppz zsh completion
_ppz() {
    local -a completions
    local words=("${(@)words[2,$CURRENT]}")
    completions=("${(@f)$(ppz __complete -- "${words[@]}" 2>/dev/null)}")
    compadd -- $completions
}
compdef _ppz ppz
`

// cmdCompletion handles `ppz completion <shell>`.
func cmdCompletion(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz completion <bash|zsh>")
		os.Exit(2)
	}
	switch args[0] {
	case "bash":
		fmt.Print(completionBash)
	case "zsh":
		fmt.Print(completionZsh)
	default:
		fmt.Fprintf(os.Stderr, "ppz completion: unsupported shell %q (bash, zsh)\n", args[0])
		os.Exit(2)
	}
	return nil
}

// Top-level verbs offered by the completion engine. Order is for the
// shell to display when the user hits tab on `ppz <tab>` — keep
// alphabetical so it scans easily; "completion" / "__complete" are
// intentionally omitted (operator-internal, not for everyday use).
var topLevelVerbs = []string{
	"broadcast",
	"daemon",
	"login",
	"ls",
	"pipe",
	"read",
	"reread",
	"send",
	"source",
	"status",
	"terminal",
	"upgrade",
	"version",
}

var subverbs = map[string][]string{
	"source":   {"create", "switch", "clear", "destroy"},
	"daemon":   {"start", "stop", "login", "logout"},
	"pipe":     {"create", "destroy"},
	"terminal": {"share", "watch", "read"},
}

// Verbs that take a `<handle>.<pipe>` target as their first positional.
// Tab on the target slot completes against everything the daemon's
// IPCList knows about.
var targetTakingVerbs = map[string]bool{
	"read":      true,
	"reread":    true,
	"send":      true,
	"broadcast": true,
}

// Terminal subverbs whose first positional is a handle (no `.pipe`).
// `share` creates a new pty source — completing existing handles is
// still useful as a "remember what I called the other one" hint.
var terminalHandleSubverbs = map[string]bool{
	"share": true,
	"watch": true,
	"read":  true,
}

// cmdComplete is the hidden completion engine. The shell script invokes
// it as `ppz __complete -- <word1> <word2> …` where the LAST word is
// the partial currently being typed (may be empty). Earlier words give
// context.
//
// Output: one candidate per line, no formatting, no errors on stdout.
// Failures (e.g. daemon down for dynamic completion) silently fall back
// to nothing rather than spamming the user's tab key with errors.
func cmdComplete(args []string) error {
	// Strip a leading "--" if the shell script left it in argv. We use
	// `--` in the script so words starting with `-` aren't mis-parsed
	// as flags — but Go's args slice has already split on it.
	for len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		// No word at all — e.g. "ppz" then immediately tab. Treat as
		// completing an empty top-level verb.
		args = []string{""}
	}
	cur := args[len(args)-1]
	prior := args[:len(args)-1]

	// Position 1: top-level verb.
	if len(prior) == 0 {
		emitMatching(topLevelVerbs, cur)
		return nil
	}

	verb := prior[0]

	// Position 2: subverb for grouped verbs.
	if subs, ok := subverbs[verb]; ok && len(prior) == 1 {
		emitMatching(subs, cur)
		return nil
	}

	// Target-taking verbs: complete `<handle>.<pipe>` from daemon.
	if targetTakingVerbs[verb] && len(prior) == 1 {
		emitTargets(cur)
		return nil
	}

	// `terminal {share|watch|read} <handle>` — complete handles.
	if verb == "terminal" && len(prior) == 2 && terminalHandleSubverbs[prior[1]] {
		emitHandles(cur)
		return nil
	}

	// Anything else — no completion. Quietly succeed.
	return nil
}

// emitMatching prints each candidate that prefix-matches `cur`.
func emitMatching(candidates []string, cur string) {
	for _, c := range candidates {
		if strings.HasPrefix(c, cur) {
			fmt.Println(c)
		}
	}
}

// listSourcesForCompletion talks to the daemon. Errors are swallowed —
// completion must never fail loudly, and "daemon's down so suggest
// nothing" is the right fallback.
func listSourcesForCompletion() []cliproto.Source {
	var reply cliproto.ListReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCList,
		cliproto.ListRequest{Session: sessionID()}, &reply); err != nil {
		return nil
	}
	return reply.Sources
}

// emitHandles prints handles from the daemon's known sources.
func emitHandles(cur string) {
	seen := map[string]bool{}
	for _, s := range listSourcesForCompletion() {
		if seen[s.Handle] {
			continue
		}
		seen[s.Handle] = true
		if strings.HasPrefix(s.Handle, cur) {
			fmt.Println(s.Handle)
		}
	}
}

// emitTargets prints `<handle>.<pipe>` for every (source, pipe) the
// daemon knows about.
func emitTargets(cur string) {
	for _, s := range listSourcesForCompletion() {
		for _, p := range s.PipeInfos {
			t := s.Handle + "." + p.Pipe
			if strings.HasPrefix(t, cur) {
				fmt.Println(t)
			}
		}
	}
}
