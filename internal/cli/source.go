package cli

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdSourceGroup dispatches `ppz source <subverb>`.
//
// Phase B ships create + switch (carried over from the old top-level verbs).
// `destroy` lands in a later phase together with the matching server
// endpoint + cleanup story; until then, calling it returns exit 2.
func cmdSourceGroup(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz source {create|switch|clear} [HANDLE]")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		return cmdSourceCreate(args[1:])
	case "switch":
		return cmdSourceSwitch(args[1:])
	case "clear":
		return cmdSourceClear(args[1:])
	case "destroy":
		fmt.Fprintln(os.Stderr, "ppz source destroy: not implemented yet")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "ppz source: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// warnIfEnvOverrides prints a stderr warning when PPZ_CURRENT_HANDLE is
// set during a mutating "current"-changing command — the daemon-side
// mutation lands but env still wins for effective resolution. Without
// the warning, "I just ran `source switch foo` but my broadcasts go to
// bar" is a confusing afternoon.
func warnIfEnvOverrides(action string) {
	if env := os.Getenv("PPZ_CURRENT_HANDLE"); env != "" {
		fmt.Fprintf(os.Stderr,
			"warning: PPZ_CURRENT_HANDLE=%s is set; daemon current was %s but env still wins\n",
			env, action)
	}
}

// cmdSourceCreate creates a source AND sets it as current. Strict: errors
// with E_SOURCE_TAKEN if the handle already exists in the org. To point
// at a pre-existing source, use `ppz source switch HANDLE`.
func cmdSourceCreate(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz source create HANDLE")
		os.Exit(2)
	}
	var reply cliproto.CreateReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCCreate,
		cliproto.CreateRequest{Handle: args[0], Session: sessionID()}, &reply); err != nil {
		return err
	}
	cliproto.PrintCreate(os.Stdout, reply)
	warnIfEnvOverrides("updated")
	return nil
}

func cmdSourceSwitch(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz source switch HANDLE")
		os.Exit(2)
	}
	var reply cliproto.SwitchReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCSwitch,
		cliproto.SwitchRequest{Handle: args[0], Session: sessionID()}, &reply); err != nil {
		return err
	}
	cliproto.PrintSwitch(os.Stdout, reply)
	warnIfEnvOverrides("updated")
	return nil
}

// cmdSourceClear clears the daemon's current-source binding for this
// session. Reuses the IPCDisconnect handler — same daemon-side state
// change, just under the more discoverable `source clear` name.
func cmdSourceClear(args []string) error {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz source clear")
		os.Exit(2)
	}
	var reply cliproto.DisconnectReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCDisconnect,
		cliproto.DisconnectRequest{Session: sessionID()}, &reply); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "cleared")
	warnIfEnvOverrides("cleared")
	return nil
}
