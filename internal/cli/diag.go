package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdDiag implements `ppz diag` — Phase 0 of the agent hardening plan
// (docs/WIRE.md §8). Prints the daemon's NATS connection state plus
// the recent tail of connection-state events (disconnect / reconnect /
// closed). Useful for catching transient outages "a few minutes ago"
// that aren't visible in `ppz status` because the connection has
// since recovered.
//
// The verb deliberately does NOT require login. An operator hitting
// a sick daemon (login fails, NATS unreachable) needs `ppz diag` to
// work — that's the entire point. We surface whatever ring buffer
// state is available + the daemon's reachability over IPC.
//
// Output format (one event per line):
//
//	<type> <RFC3339-timestamp> reason="<text>"
//
// Where type is one of disconnect / reconnect / closed. Reason is
// quoted to keep the line shape parseable when it contains spaces.
//
// --json emits a single JSON object matching cliproto.DiagReply.
func cmdDiag(args []string) error {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}

	var reply cliproto.DiagReply
	err := daemon.Call(ipcSocket(), cliproto.IPCDiag, cliproto.DiagRequest{}, &reply)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(reply)
	}

	state := reply.NATSState
	if state == "" {
		state = "unknown"
	}
	fmt.Fprintf(os.Stdout, "nats: %s drops_last_hour=%d events=%d\n",
		state, reply.NATSDropsLastHour, len(reply.NATSEvents))
	for _, ev := range reply.NATSEvents {
		fmt.Fprintf(os.Stdout, "%s %s reason=%q\n",
			ev.Type, ev.At.UTC().Format(time.RFC3339), ev.Reason)
	}
	return nil
}
