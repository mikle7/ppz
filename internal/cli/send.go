package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdSend: ppz send <handle>.<channel> <payload>
//
// Explicit-target publish — the same wire format as ppz broadcast, just
// without the "current handle" default. Used to write to .stdin of a
// running terminal pipe, or to .broadcast of any pipe.
func cmdSend(args []string) error {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ppz send <handle>.<channel> <payload>")
		os.Exit(2)
	}
	target := args[0]
	payload := args[1]

	idx := strings.LastIndex(target, ".")
	if idx <= 0 || idx == len(target)-1 {
		return cliproto.New(cliproto.EInvalidPipe)
	}
	handle, channel := target[:idx], target[idx+1:]

	var reply cliproto.BroadcastReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCBroadcast,
		cliproto.BroadcastRequest{Handle: handle, Channel: channel, Payload: payload},
		&reply); err != nil {
		return err
	}
	cliproto.PrintBroadcast(os.Stdout, reply)
	return nil
}
