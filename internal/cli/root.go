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

Daemon (desktop-wide lifecycle):
  ppz daemon start                 start the local daemon
  ppz daemon stop                  stop the local daemon (idempotent)
  ppz daemon login URL -apikey K   log the daemon into a server with an api key
                                   (or use the 'ppz login' shortcut)
  ppz daemon logout                clear the stored credential
  ppz login URL -apikey K          shortcut for 'ppz daemon login'

Sources (the addressable top-level entity):
  ppz source create HANDLE         create a new source + set as current
                                   (errors if HANDLE already exists)
  ppz source switch HANDLE         set an existing HANDLE as the current source
  ppz source clear                 clear the current source (source stays)

Pipes (sub-buckets on a source):
  ppz pipe create [HANDLE.]NAME [--ttl=DUR --max-msgs=N --max-bytes=B]
  ppz pipe destroy [HANDLE.]NAME

Terminal:
  ppz terminal share H [-- CMD ...] run CMD (or $SHELL) in a pty bound to source H —
                                    bidirectional: stdout published, stdin subscribed
  ppz terminal watch H              follow H.stdout in the alt-screen TUI
                                    (interactive — renders to your local terminal,
                                    not capturable; agents should use terminal read)
  ppz terminal read H [reread-flags] wrapper for 'ppz reread H.stdout' with --tty
                                    as the default output mode (vt10x screen
                                    render — rebuild cumulative screen state).
                                    All reread flags work: --raw, --json, -l N,
                                    --skip N, --since DUR.

Operations:
  ppz status                       show daemon, login, and current source
  ppz version                      print the binary's version + build sha
  ppz ls [--watch [PATTERN...]]    list sources × pipes (--watch blocks until
                                   unread on a matching handle; patterns use
                                   '*' quoted or % unquoted as the wildcard)
  ppz broadcast [-m TEXT]          publish to <current_source>.broadcast
  ppz read TGT [--tail --json --tty --raw]
                                   read NEW messages from <handle>.<pipe>
                                   since the session cursor; advances the
                                   cursor as it goes (agent inbox poll).
                                   --tail keeps streaming live until SIGINT.
                                   --tty / --raw / --json: output modes
                                   shared with reread.
  ppz reread TGT [-l N --skip N --since DUR --json --tty --raw]
                                   forensic / replay: every retained message,
                                   ignoring the cursor. Carries the filter
                                   flags. Never advances the cursor.
  ppz send TGT PAYLOAD             publish PAYLOAD to <handle>.<pipe>

Shell integration:
  ppz completion bash              tab-completion script for bash
  ppz completion zsh               tab-completion script for zsh
                                   add 'eval "$(ppz completion bash)"' to your shell rc`)
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
