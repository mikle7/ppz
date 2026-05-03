package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdSend: ppz send <handle>[.<pipe>] <payload>
//
// Bare handles target .inbox for direct source/agent messages. Explicit
// <handle>.<pipe> targets can write to .stdin of a running terminal pipe,
// or to .broadcast / custom pipes.
func cmdSend(args []string) error {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ppz send <handle>[.<pipe>] <payload>")
		os.Exit(2)
	}
	target := args[0]
	payload := args[1]

	idx := strings.LastIndex(target, ".")
	if idx == -1 {
		target += ".inbox"
		idx = strings.LastIndex(target, ".")
	}
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
