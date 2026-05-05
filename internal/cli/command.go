package cli

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdCommand: ppz command <handle> [instruction] [--claude|--cr|--crlf|--newline]
//
// Sends an optional instruction to <handle>.stdin, waits 100 ms, then sends a
// trailing control sequence.  Default sequence is \n (Enter).  Flags select
// alternatives:
//
//	--claude   \x1b[13u  (XTerm CSI-u "submit" — same as pty inbox alerting)
//	--cr       \r
//	--crlf     \r\n
//	--newline  \n        (explicit; same as the default)
func cmdCommand(args []string) error {
	fs := flag.NewFlagSet("command", flag.ExitOnError)
	useClaude  := fs.Bool("claude", false, "trailing sequence: XTerm CSI-u submit \\x1b[13u")
	useCR      := fs.Bool("cr", false, "trailing sequence: carriage-return \\r")
	useCRLF    := fs.Bool("crlf", false, "trailing sequence: carriage-return + newline \\r\\n")
	useNewline := fs.Bool("newline", false, "trailing sequence: newline \\n (same as default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz command <handle> [instruction] [--claude|--cr|--crlf|--newline]")
		os.Exit(2)
	}
	handle := rest[0]
	var instruction string
	if len(rest) > 1 {
		instruction = rest[1]
	}

	ctrlSeq := "\n"
	switch {
	case *useClaude:
		ctrlSeq = "\x1b[13u"
	case *useCR:
		ctrlSeq = "\r"
	case *useCRLF:
		ctrlSeq = "\r\n"
	case *useNewline:
		ctrlSeq = "\n"
	}

	send := func(payload string) error {
		var reply cliproto.BroadcastReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCBroadcast,
			cliproto.BroadcastRequest{Handle: handle, Channel: "stdin", Payload: payload},
			&reply); err != nil {
			return err
		}
		cliproto.PrintBroadcast(os.Stdout, reply)
		return nil
	}

	if instruction != "" {
		if err := send(instruction); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return send(ctrlSeq)
}
