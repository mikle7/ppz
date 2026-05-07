// Package cli is the entrypoint for `ppz`. Each verb either talks to the
// daemon over IPC or IS the daemon (`ppz daemon start --foreground`).
package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run dispatches argv[1:] to the appropriate verb. Returns a *cliproto.Error
// when there is one — main turns that into the standard exit code + stderr.
//
// Verb hierarchy (Phase B):
//
//	ppz daemon {start|stop|login|logout}
//	ppz source {create|destroy|switch}
//	ppz terminal {wrap|watch|peek}     (terminal verbs are reshaped in Phase D)
//	ppz {status|ls|broadcast|read|send}
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
	case "login":
		// Top-level shortcut for `ppz daemon login` — matches the
		// `gh login` / `kubectl login` / `az login` muscle memory.
		return cmdDaemonLogin(rest)
	case "status":
		return cmdStatus(rest)
	case "version":
		return cmdVersion(rest)
	case "upgrade":
		return cmdUpgrade(rest)
	case "broadcast":
		return cmdBroadcast(rest)
	case "ls":
		return cmdLs(rest)
	case "read":
		return cmdRead(rest)
	case "reread":
		return cmdReread(rest)
	case "send":
		return cmdSend(rest)
	case "command":
		return cmdCommand(rest)
	case "org", "orgs":
		return cmdOrg(rest)
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
	fmt.Fprintln(w, `ppz — pipes for agents

Messaging (the verbs you use most):
  ppz status                       daemon state, current source, last token refresh
  ppz ls [--watch [PATTERN...]]    list sources × pipes; --watch blocks until
                                   unread arrives on a matching handle
                                   (patterns use '*' quoted or % unquoted)
  ppz read TGT [--tail --json --tty --raw --bare]
                                   read NEW messages from <handle>.<pipe>;
                                   'ppz read inbox' reads <current>.inbox.
                                   Default for inbox / broadcast pipes is the
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
  ppz broadcast [-m TEXT]          publish to <current>.broadcast
                                   (stdin form streams one message per line)
  ppz reread TGT [-l N --skip N --since DUR --json --tty --raw --bare]
                                   forensic / replay: every retained message;
                                   ignores and never advances the cursor.
  ppz command H [INSTR]            send INSTR to H.stdin (100 ms delay),
                                   then send a trailing control sequence
                                   (--claude (\\x1b[13u) / --cr / --crlf / --newline / --none)

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
  ppz login URL -apikey K          shortcut for 'ppz daemon login'
  ppz daemon login URL -apikey K   log the daemon into a server with an api key
  ppz daemon logout                clear the stored credential

Sources (your addressable identities):
  ppz source create HANDLE         create + set as current
                                   (errors if HANDLE already exists)
  ppz source switch HANDLE         set an existing HANDLE as current
  ppz source clear                 clear the current source (source stays)
  ppz source destroy PATTERN       glob-destroy sources or pipes
                                   bare pattern → matching sources
                                   handle.pipe pattern → matching pipes
                                   glob wildcards: * ? [abc] (path.Match rules)
                                   examples: destroy '*'  destroy 'agent-*'
                                             destroy '*.stdout'  destroy apple

Pipes (custom sub-buckets on a source):
  ppz pipe create [HANDLE.]NAME [--ttl=DUR --max-msgs=N --max-bytes=B]
  ppz pipe destroy [HANDLE.]NAME

Terminal:
  ppz terminal share H [-- CMD ...] run CMD (or $SHELL) in a pty bound to H —
                                    bidirectional: stdout published, stdin
                                    subscribed
  ppz terminal watch H              follow H.stdout in alt-screen TUI
                                    (interactive — agents prefer terminal read)
  ppz terminal read H [reread-flags] wrapper for 'ppz reread H.stdout' with
                                    --tty as the default output mode (vt10x
                                    screen render — rebuild cumulative state)

Other:
  ppz version                      print the binary's version + build sha
  ppz upgrade                      install the latest ppz CLI release
  ppz org {list|switch|create|invite}
                                   multi-org operations
  ppz completion {bash|zsh}        tab-completion script
                                   add 'eval "$(ppz completion bash)"' to
                                   your shell rc`)
}

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
