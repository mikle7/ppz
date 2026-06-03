package cli

import (
	"flag"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdLs: ppz ls [--json | --iso] [--watch] [PATTERN...]
//
// Default: aligned table — PIPE / TOTAL / UNREAD / LAST / PAYLOAD with
// relative time in the LAST column. Tuned for human + agent scanability.
//
//	--json    one JSONL row per (handle, pipe), full untruncated payload,
//	          ISO last_at. The agent-friendly path.
//	--iso     keep the table layout but render LAST as RFC3339 timestamps
//	          instead of relative durations. For when you want sortable /
//	          diffable timestamps without dropping into JSON.
//	--watch   block until the calling session has at least one unread
//	          message on a matching pipe, then print the snapshot and
//	          exit. Level-triggered: if unread > 0 already, returns
//	          immediately.
//	PATTERN…  optional glob(s) matched against handle, pipe, or
//	          `<handle>.<pipe>`; multiple OR-combine; no pattern means
//	          every pipe. Works for both the plain snapshot and
//	          --watch — `ppz ls clancy%` filters the table the same
//	          way `ls clancy*` would on the filesystem.
//
//	          Wildcards: `*` (standard glob, must be quoted in zsh:
//	          `'agent-*'`) or `%` (SQL-LIKE-style alias, passes through
//	          unquoted: `agent-%`). Both work the same.
//
// --json and --iso are mutually exclusive — JSON mode always emits ISO
// timestamps, so --iso would be a no-op tag.
func cmdLs(args []string) error {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit one JSON object per row (agent-friendly, full payload)")
	iso := fs.Bool("iso", false, "render last-message column as RFC3339 timestamp instead of relative duration")
	watch := fs.Bool("watch", false, "block until matching pipes have unread messages, print, then exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *asJSON && *iso {
		os.Stderr.WriteString("ppz ls: --json and --iso are mutually exclusive\n")
		os.Exit(2)
	}
	patterns := fs.Args()

	var reply cliproto.ListReply
	if *watch {
		req := cliproto.ListWatchRequest{Session: sessionID(), Patterns: patterns}
		if err := daemon.Call(ipcSocket(), cliproto.IPCListWatch, req, &reply); err != nil {
			return err
		}
	} else {
		req := cliproto.ListRequest{Session: sessionID(), Patterns: patterns}
		if err := daemon.Call(ipcSocket(), cliproto.IPCList, req, &reply); err != nil {
			return err
		}
	}

	if *asJSON {
		cliproto.PrintListJSONWithUncollared(os.Stdout, reply.Sources, reply.UncollaredPipes)
	} else {
		cliproto.PrintListWithUncollared(os.Stdout, reply.Sources, reply.UncollaredPipes, *iso)
	}
	maybeNotifyUpdate()
	return nil
}
