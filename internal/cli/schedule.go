package cli

// Scheduled sends (docs/specs/schedule.md). Creation is a switch on
// `ppz send` (--at/--every/--cron, dispatched from cmdSend into
// sendSchedule below); management lives under the `ppz schedule` noun:
//
//	ppz schedule ls [--json|--iso]   list the account's live schedules
//	ppz schedule rm <id>             remove by the short id `ls` shows
//
// The schedule itself is durable server-side state — it fires with
// this machine asleep or the daemon stopped. Validation is client-
// side-first: a bad spec never reaches the daemon.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
	"github.com/pipescloud/ppz/internal/schedule"
)

// sendSchedule is cmdSend's scheduled branch: exactly one of
// atArg/everyArg/cronArg is non-empty (cmdSend enforced exclusivity).
// Target resolution (bare-name .inbox sugar) already happened —
// handle/channel/bareTarget arrive in SendRequest shape.
func sendSchedule(handle, channel, bareTarget, payload, atArg, everyArg, cronArg string) error {
	req := cliproto.ScheduleCreateRequest{
		Session:    sessionID(),
		Sender:     os.Getenv("PPZ_CURRENT_HANDLE"),
		Handle:     handle,
		Channel:    channel,
		BareTarget: bareTarget,
		Payload:    payload,
	}
	switch {
	case atArg != "":
		// Resolve relative/local forms here — the device zone is only
		// observable CLI-side. The RFC3339 sent up preserves the
		// creator's offset so `schedule ls` renders the time as typed.
		t, err := schedule.ParseAt(atArg, time.Now(), deviceLocation())
		if errors.Is(err, schedule.ErrPast) {
			return fmt.Errorf("ppz send: --at is in the past (%s)", atArg)
		}
		if err != nil {
			return fmt.Errorf("ppz send: invalid --at %q: %v", atArg, err)
		}
		req.Kind = string(schedule.KindAt)
		req.At = t.Format(time.RFC3339)
	case everyArg != "":
		if _, err := schedule.ParseEvery(everyArg); err != nil {
			return fmt.Errorf("ppz send: invalid --every %q: %v", everyArg, err)
		}
		req.Kind = string(schedule.KindEvery)
		req.Every = everyArg
	case cronArg != "":
		if err := schedule.ParseCron(cronArg); err != nil {
			return fmt.Errorf("ppz send: invalid --cron %q: %v", cronArg, err)
		}
		req.Kind = string(schedule.KindCron)
		req.Cron = cronArg
		req.TZ = deviceTZName()
	}
	var reply cliproto.ScheduleCreateReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCScheduleCreate, req, &reply); err != nil {
		return err
	}
	// Same stream as `sent id=…` — scripts capturing stdout for
	// payloads keep seeing the confirmation.
	cliproto.PrintScheduleCreate(sendErr, reply)
	return nil
}

// deviceTZName returns the IANA zone name schedules created with
// --cron resolve their wall-clock times in. $TZ wins when loadable;
// otherwise the /etc/localtime symlink; otherwise UTC. Never returns
// Go's unloadable "Local" pseudo-name — the server must be able to
// time.LoadLocation the result.
func deviceTZName() string {
	if tz := os.Getenv("TZ"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(link, "zoneinfo/"); i >= 0 {
			name := link[i+len("zoneinfo/"):]
			// Containers commonly link Etc/UTC; render the canonical name.
			if name == "Etc/UTC" {
				return "UTC"
			}
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "UTC"
}

func deviceLocation() *time.Location {
	loc, err := time.LoadLocation(deviceTZName())
	if err != nil {
		return time.UTC
	}
	return loc
}

// cmdScheduleGroup: ppz schedule {ls|rm}
func cmdScheduleGroup(args []string) error {
	if len(args) == 0 {
		printHelp(os.Stderr, "schedule")
		os.Exit(2)
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "ls":
		return cmdScheduleLs(rest)
	case "rm":
		return cmdScheduleRm(rest)
	case "-h", "--help", "help":
		printHelp(os.Stdout, "schedule")
		return nil
	}
	fmt.Fprintf(os.Stderr, "ppz schedule: unknown verb %q\n", verb)
	printHelp(os.Stderr, "schedule")
	os.Exit(2)
	return nil
}

// cmdScheduleLs renders the account's live schedules with the `ppz ls`
// table conventions (fired one-offs and removed schedules have no row).
func cmdScheduleLs(args []string) error {
	fs := flag.NewFlagSet("schedule ls", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit one JSON object per schedule (full payload)")
	iso := fs.Bool("iso", false, "render NEXT/LAST as RFC3339 UTC instead of relative time")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *asJSON && *iso {
		os.Stderr.WriteString("ppz schedule ls: --json and --iso are mutually exclusive\n")
		os.Exit(2)
	}
	var reply cliproto.ScheduleListReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCScheduleList,
		cliproto.ScheduleListRequest{Session: sessionID()}, &reply); err != nil {
		return err
	}
	if *asJSON {
		cliproto.PrintScheduleListJSON(os.Stdout, reply.Schedules)
	} else {
		cliproto.PrintScheduleList(os.Stdout, reply.Schedules, *iso)
	}
	return nil
}

func cmdScheduleRm(args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: ppz schedule rm <id> (ids from 'ppz schedule ls')")
	}
	var reply cliproto.ScheduleRemoveReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCScheduleRemove,
		cliproto.ScheduleRemoveRequest{Session: sessionID(), ID: args[0]}, &reply); err != nil {
		return err
	}
	cliproto.PrintScheduleRemove(os.Stdout, reply)
	return nil
}
