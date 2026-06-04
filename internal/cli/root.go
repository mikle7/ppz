// Package cli is the entrypoint for `ppz`. Each verb either talks to the
// daemon over IPC or IS the daemon (`ppz daemon start --foreground`).
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Run dispatches argv[1:] to the appropriate verb. Returns a *cliproto.Error
// when there is one — main turns that into the standard exit code + stderr.
//
// Verb hierarchy (Phase B):
//
//	ppz daemon {start|stop|login|logout}
//	(source verbs removed in Phase 1 — see ppz terminal/agent create
//	for replacements; current-handle state managed via ppz set/unset)
//	ppz terminal {wrap|watch|peek}     (terminal verbs are reshaped in Phase D)
//	ppz {status|ls|read|send}
//
// Old top-level verbs (`ppz create`, `ppz switch`, `ppz kill`, `ppz login`)
// are removed without aliases — fresh MVP, no users to migrate.
func Run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		os.Exit(2)
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "daemon":
		return cmdDaemonGroup(rest)
	case "source":
		return cmdSourceGroup(rest)
	case "pipe":
		return cmdPipeGroup(rest)
	case "terminal":
		return cmdTerminal(rest)
	case "agent":
		return cmdAgentGroup(rest)
	case "login":
		// Top-level shortcut for `ppz daemon login` — matches the
		// `gh login` / `kubectl login` / `az login` muscle memory.
		return cmdDaemonLogin(rest)
	case "status":
		return cmdStatus(rest)
	case "diagnostics":
		return cmdDiagnostics(rest)
	case "who":
		return cmdWho(rest)
	case "set":
		return cmdSet(rest)
	case "unset":
		return cmdUnset(rest)
	case "get":
		return cmdGet(rest)
	case "version":
		return cmdVersion(rest)
	case "upgrade":
		return cmdUpgrade(rest)
	case "ls":
		return cmdLs(rest)
	case "subs":
		return cmdSubsGroup(rest)
	case "read":
		return cmdRead(rest)
	case "reread":
		return cmdReread(rest)
	case "send":
		return cmdSend(rest)
	case "command":
		return cmdCommand(rest)
	case "completion":
		return cmdCompletion(rest)
	case "__complete":
		// Hidden — invoked by the shell's tab handler. Not in usage.
		return cmdComplete(rest)
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	}
	fmt.Fprintf(os.Stderr, "ppz: unknown command %q\n", verb)
	usage(os.Stderr)
	os.Exit(2)
	return nil
}

func usage(w *os.File) {
	fmt.Fprintln(w, wrapUsageText(usageText, cliproto.TerminalWidth()))
}

// wrapUsageText reflows the static usage block to fit `width` columns.
// It runs two passes:
//
//  1. Normalize: join paragraph lines so the wrap pass can reflow them at
//     the actual terminal width. Two join cases:
//
//     Case A (inline desc): verb line has a 2+ space gap at descCol.
//     Subsequent lines at indent==descCol with no internal gap are pure
//     description prose — join them into the verb line.
//
//     Case B (deep run): consecutive lines at the same deep indent (≥10)
//     with no internal gap are description prose for a verb whose signature
//     was too long to share the line — join them together.
//
//     Sub-items that are indented more than the active descCol/runIndent are
//     left on their own lines so structured blocks (e.g. flag tables) are
//     preserved.
//
//  2. Wrap: each logical line is word-wrapped at width, preserving the
//     verb-signature column and re-indenting continuation runs under the
//     description column.
func wrapUsageText(text string, width int) string {
	if width <= 0 {
		return text
	}

	// findDescCol returns (leadingIndent, descCol) for a line.
	// descCol is the column after the first 2+ space gap past the verb
	// signature, or leadingIndent when no such gap exists.
	findDescCol := func(line string) (leadingIndent, descCol int) {
		for leadingIndent < len(line) && line[leadingIndent] == ' ' {
			leadingIndent++
		}
		descCol = leadingIndent
		for i := leadingIndent; i < len(line); {
			if line[i] != ' ' {
				i++
				continue
			}
			j := i
			for j < len(line) && line[j] == ' ' {
				j++
			}
			if j-i >= 2 {
				descCol = j
				break
			}
			i = j
		}
		return
	}

	// Pass 1: normalize.
	raw := strings.Split(text, "\n")
	joined := make([]string, 0, len(raw))
	pending := ""
	// pendingDescCol and runIndent are mutually exclusive: flushPending resets
	// both, and the switch below sets at most one per new pending line.
	pendingDescCol := 0 // Case A: description column of the current verb line
	runIndent := 0      // Case B: indent shared by a deep description-only run

	flushPending := func() {
		if pending != "" {
			joined = append(joined, pending)
			pending = ""
		}
		pendingDescCol = 0
		runIndent = 0
	}

	for _, line := range raw {
		if line == "" {
			flushPending()
			joined = append(joined, "")
			continue
		}
		indent, dc := findDescCol(line)

		// Case A: continuation at the active description column.
		if pendingDescCol > 0 && indent == pendingDescCol && dc == indent {
			pending += " " + strings.TrimLeft(line, " ")
			continue
		}
		// Case B: continuation within a deep same-indent run.
		if runIndent > 0 && indent == runIndent && dc == indent {
			pending += " " + strings.TrimLeft(line, " ")
			continue
		}

		flushPending()
		pending = line
		switch {
		case dc > indent:
			pendingDescCol = dc
		case indent >= 10 && dc == indent:
			// Threshold of 10 rules out top-level verb lines (indent ≤ 4)
			// and shallow section text, leaving only description-column prose.
			runIndent = indent
		}
	}
	flushPending()

	// Pass 2: word-wrap each logical line.
	out := make([]string, 0, len(joined))
	for _, line := range joined {
		if len([]rune(line)) <= width {
			out = append(out, line)
			continue
		}
		_, descCol := findDescCol(line)
		contentBudget := width - descCol
		if contentBudget <= 4 {
			out = append(out, line)
			continue
		}
		contPrefix := strings.Repeat(" ", descCol)
		firstPrefix := line[:descCol]
		words := strings.Fields(line[descCol:])
		if len(words) == 0 {
			out = append(out, line)
			continue
		}
		var cur strings.Builder
		first := true
		flush := func() {
			if first {
				out = append(out, firstPrefix+cur.String())
				first = false
			} else {
				out = append(out, contPrefix+cur.String())
			}
			cur.Reset()
		}
		for _, word := range words {
			if cur.Len() == 0 {
				cur.WriteString(word)
				continue
			}
			if cur.Len()+1+len([]rune(word)) <= contentBudget {
				cur.WriteByte(' ')
				cur.WriteString(word)
				continue
			}
			flush()
			cur.WriteString(word)
		}
		if cur.Len() > 0 {
			flush()
		}
	}
	return strings.Join(out, "\n")
}

const usageText = `ppz — pipes for agents

Messaging (the verbs you use most):
  ppz status                       daemon state, current handle, last token refresh
  ppz ls [--watch] [PATTERN...]    list handles × pipes; --watch blocks until
                                   unread arrives on a matching pipe.
                                   PATTERNs glob the full <handle>.<pipe>
                                   (e.g. '*.inbox', 'alice.*'); a literal
                                   that matches no pipe warns (use a glob).
                                   '*' quoted or % unquoted.
  ppz read TGT [--tail --json --tty --raw --bare]
                                   read NEW messages from <handle>.<pipe>;
                                   'ppz read inbox' reads <current>.inbox.
                                   Default for inbox pipes is the
                                   v0.23 tabular format —
                                     HH:MM:SS  <sender|->  <body>
                                   where <body> is "[subject] payload" for
                                   user subjects, "ack:read → <id8>" for
                                   system ack messages, or just the payload.
                                   --bare forces legacy payload-only output
                                   (script-stable opt-out).
                                   --tail keeps streaming live until SIGINT.
                                   --tty / --raw / --json: shared with reread.
  ppz send TGT PAYLOAD [--subject S] [--in-reply-to ID] [--request-ack]
                                   publish PAYLOAD to <handle>.<pipe>;
                                   bare handle → <handle>.inbox.
                                   Success line goes to STDERR (since v0.25 —
                                   was stdout; scripts redirecting stdout
                                   no longer swallow it).
                                   --subject S        envelope-level header;
                                                      'ack:' prefix reserved.
                                   --in-reply-to ID   thread / reply linkage.
                                   --request-ack      receiver's daemon emits
                                                      'ack:read' back to YOUR
                                                      inbox when their cursor
                                                      advances (best-effort,
                                                      non-blocking — see Acks).
  ppz reread TGT [-l N --skip N --since DUR --json --tty --raw --bare]
                                   forensic / replay: every retained message;
                                   ignores and never advances the cursor.
  ppz command H [INSTR]            send INSTR to H.stdin (100 ms delay),
                                   then send a trailing control sequence.
                                   Default: \\r (carriage-return Enter, the
                                   byte every non-claude harness accepts).
                                   --claude (\\x1b[13u, kitty Enter) /
                                   --cr (\\r) / --crlf / --newline (\\n) /
                                   --none

Acks (read receipts, v0.25):
  Use 'ppz send … --request-ack' when you need to know the recipient saw
  your message. Their daemon auto-emits an 'ack:read' envelope back to your
  inbox carrying in_reply_to=<your-msg-id>. The tabular read formatter
  renders these as 'ack:read → <id8>' so you can correlate at a glance.
  Best-effort: a missing ack is indistinguishable from "not yet read".
  If you need strict guarantees, layer your own re-send-on-timeout.
  The 'ack:' subject prefix is reserved — the CLI and daemon both reject
  user attempts to set it (E_INVALID_SUBJECT).

Setup (once per workstation):
  ppz daemon start                 start the local daemon
  ppz daemon stop                  stop the local daemon (idempotent)
  ppz daemon restart               stop+start; use after 'ppz upgrade' when
                                   'ppz status' reports the daemon out of
                                   sync with the CLI
  ppz login URL -apikey K          shortcut for 'ppz daemon login'
  ppz daemon login URL -apikey K   log the daemon into a server with an api key
  ppz daemon logout                clear the stored credential

  Agents / subprocess-per-call: each shell session has its own current
  handle (keyed off the calling tty). Subprocesses with no shared tty
  get a fresh session id per invocation, so 'ppz terminal create' in
  one call won't be visible to the next — and 'ppz send --request-ack'
  will reject with E_NO_CURRENT_SOURCE. Pin a stable id by exporting
  PPZ_SESSION=<id> at the agent's lifecycle level so all subsequent
  ppz calls share session state.

Handles (your addressable identities):
  ppz source create HANDLE         claim a bare message-kind handle
                                   (auto-pipe: inbox). Use when you want a
                                   named actor identity without committing
                                   to a terminal or agent role.
  ppz terminal create HANDLE       create a pty-backed handle (auto-pipes:
                                   inbox/stdin/stdout/stdctrl) and set as
                                   current.
  ppz agent create HANDLE          create an agent handle and run an AI
                                   harness in it.
  ppz source destroy PATTERN       glob-destroy sources or pipes.
                                   bare pattern → matching sources
                                   handle.pipe pattern → matching pipes
                                   glob wildcards: * ? [abc] (path.Match rules)
                                   examples: destroy '*'  destroy 'agent-*'
                                             destroy '*.stdout'  destroy apple

Pipes:
  ppz pipe create [HANDLE.]NAME [--ttl=DUR --max-msgs=N --max-bytes=B]
  ppz pipe destroy [HANDLE.]NAME [--recursive]

Daemon state (current handle, future settings):
  ppz set handle HANDLE            switch the daemon's current handle for
                                   this session. The current handle is
                                   what gets stamped as 'sender' on
                                   outgoing envelopes and used as the
                                   implicit target for bare 'ppz read
                                   inbox' / 'ppz send TARGET' invocations.
  ppz unset handle                 clear the daemon's current handle for
                                   this session. The source row stays —
                                   only the per-session pointer is cleared.
  ppz get handle                   print the current handle to stdout.
                                   Exits 1 with empty output when no
                                   current is set, so $(ppz get handle)
                                   can detect "not set" via rc.
  ppz set namespace PATH           set the daemon's current namespace
                                   (manifold) for this session. New
                                   pipes created with 'ppz pipe create
                                   LEAF' inherit this manifold. (Phase
                                   1.5; locked decision #18.)
  ppz unset namespace              clear the daemon's namespace. New
                                   pipes are created at the root
                                   manifold. View current namespace via
                                   'ppz status'.

Terminal:
  ppz terminal share H [-- CMD ...] run CMD (or $SHELL) in a pty bound to H —
                                    bidirectional: stdout published, stdin
                                    subscribed
  ppz terminal watch H              follow H.stdout in alt-screen TUI
                                    (interactive — agents prefer terminal read)
  ppz terminal read H [reread-flags] wrapper for 'ppz reread H.stdout' with
                                    --tty as the default output mode (vt10x
                                    screen render — rebuild cumulative state)

Agents:
  ppz agent create NAME [PROMPT]    create pty source NAME and run an AI
                                    harness in it. Default: --claude --opus.
                                    Switches:
                                      --claude | --copilot | --codex
                                              | --agy | --pi
                                      --opus | --sonnet | --haiku  (claude)
                                      --model X                    (any harness)
                                      --prompt-file PATH           (instead of
                                                                    positional)
                                      --new-window                 (open a fresh
                                                                    Terminal.app /
                                                                    iTerm2 window)

Other:
  ppz version                      print the binary's version + build sha
  ppz upgrade                      install the latest ppz CLI release
  ppz diagnostics [--json]         introspect the daemon: NATS connection
                                   state + recent disconnect / reconnect
                                   events + daemon_start / daemon_stop
                                   lifecycle events (the latter persist
                                   across daemon restarts). Works without
                                   login — useful when 'ppz status' shows
                                   "not running" or "authentication error".
  ppz who [--json] [--online]      list every agent the local daemon has
         [--stale] [--offline]     seen a heartbeat from, with online /
         [--harness=X] [--owner=X] stale / offline status, harness, model,
                                   host, os/arch, CREATED (uptime as a
                                   relative duration, e.g. "5 minutes ago")
                                   and OWNER (the source's creator,
                                   resolved at query time so transfer of
                                   ownership reflects immediately).
                                   Filters combine OR for status, AND for
                                   harness and owner.
  ppz completion {bash|zsh}        tab-completion script
                                   add 'eval "$(ppz completion bash)"' to
                                   your shell rc`

// home + sock resolution. Order: PPZ_IPC_SOCKET env, then $PPZ_HOME/daemon.sock,
// then ~/.ppz/daemon.sock.
func ipcSocket() string {
	if v := os.Getenv("PPZ_IPC_SOCKET"); v != "" {
		return v
	}
	return filepath.Join(home(), "daemon.sock")
}

func home() string {
	if v := os.Getenv("PPZ_HOME"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".ppz")
	}
	return ".ppz"
}
