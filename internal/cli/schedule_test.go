package cli

// RED — docs/specs/schedule.md. CLI surface for scheduled sends:
// `ppz send … --at/--every/--cron` creates a schedule over IPC
// (ScheduleCreate) instead of publishing, and the new `ppz schedule`
// noun manages the set (ls / rm). Uses the fake-daemon recorder
// pattern from send_test.go.

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// scheduleDaemon serves ScheduleCreate/ScheduleList/ScheduleRemove and
// records each request. Send requests are recorded too, so tests can
// assert a schedule flag NEVER falls through to a plain publish.
type scheduleDaemonRecorders struct {
	creates *recorder[cliproto.ScheduleCreateRequest]
	lists   *recorder[cliproto.ScheduleListRequest]
	removes *recorder[cliproto.ScheduleRemoveRequest]
	sends   *recorder[cliproto.SendRequest]
}

func serveScheduleDaemon(t *testing.T, sock string) *scheduleDaemonRecorders {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	recs := &scheduleDaemonRecorders{
		creates: &recorder[cliproto.ScheduleCreateRequest]{},
		lists:   &recorder[cliproto.ScheduleListRequest]{},
		removes: &recorder[cliproto.ScheduleRemoveRequest]{},
		sends:   &recorder[cliproto.SendRequest]{},
	}
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = conn.Close()
				continue
			}
			switch req.Method {
			case cliproto.IPCScheduleCreate:
				var cr cliproto.ScheduleCreateRequest
				_ = json.Unmarshal(req.Params, &cr)
				recs.creates.add(cr)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.ScheduleCreateReply{
						ID:     "a1b2c3d4",
						Target: cr.Handle + "." + cr.Channel,
						NextAt: time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC),
					},
				})
			case cliproto.IPCScheduleList:
				var lr cliproto.ScheduleListRequest
				_ = json.Unmarshal(req.Params, &lr)
				recs.lists.add(lr)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.ScheduleListReply{},
				})
			case cliproto.IPCScheduleRemove:
				var rr cliproto.ScheduleRemoveRequest
				_ = json.Unmarshal(req.Params, &rr)
				recs.removes.add(rr)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.ScheduleRemoveReply{ID: rr.ID},
				})
			case cliproto.IPCSend:
				var sr cliproto.SendRequest
				_ = json.Unmarshal(req.Params, &sr)
				recs.sends.add(sr)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.SendReply{ID: "test-id", Subject: "org.foo.inbox"},
				})
			}
			_ = conn.Close()
		}
	}()
	return recs
}

func scheduleTestSocket(t *testing.T) *scheduleDaemonRecorders {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ppz-schedule-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	return serveScheduleDaemon(t, sock)
}

// --- ppz send --at/--every/--cron ------------------------------------------

func TestCmdSend_AtCreatesScheduleNotSend(t *testing.T) {
	t.Setenv("PPZ_SESSION", "tty-sched-at")
	recs := scheduleTestSocket(t)

	// Trailing flag position pins splitSendArgs treating --at as a
	// value-taking flag.
	if err := cmdSend([]string{"bob", "standup in 5", "--at", "2999-01-02T03:04:05Z"}); err != nil {
		t.Fatalf("cmdSend --at: %v", err)
	}
	if recs.sends.count() != 0 {
		t.Fatal("--at must not publish immediately (IPCSend was called)")
	}
	if recs.creates.count() != 1 {
		t.Fatalf("ScheduleCreate count = %d, want 1", recs.creates.count())
	}
	got := recs.creates.at(0)
	if got.Kind != "at" || got.At != "2999-01-02T03:04:05Z" {
		t.Fatalf("kind/at = %q/%q, want at/2999-01-02T03:04:05Z (explicit RFC3339 passes through)", got.Kind, got.At)
	}
	if got.Handle != "bob" || got.Channel != "inbox" || got.BareTarget != "bob" {
		t.Fatalf("target resolution must mirror plain send (.inbox sugar + bare target): %+v", got)
	}
	if got.Payload != "standup in 5" {
		t.Fatalf("payload = %q", got.Payload)
	}
	if got.Session != "tty-sched-at" {
		t.Fatalf("Session = %q, want forwarded tty session (sender resolution parity with send)", got.Session)
	}
}

func TestCmdSend_EveryCreatesSchedule(t *testing.T) {
	recs := scheduleTestSocket(t)
	if err := cmdSend([]string{"alerts", "heartbeat check", "--every", "15m"}); err != nil {
		t.Fatalf("cmdSend --every: %v", err)
	}
	got := recs.creates.at(0)
	if got.Kind != "every" || got.Every != "15m" {
		t.Fatalf("kind/every = %q/%q, want every/15m", got.Kind, got.Every)
	}
	if got.At != "" || got.Cron != "" {
		t.Fatalf("only Every may be set for kind=every: %+v", got)
	}
}

func TestCmdSend_CronCreatesScheduleWithDeviceTZ(t *testing.T) {
	// The CLI is the only place the device zone is observable; it must
	// forward an IANA name the server can time.LoadLocation. $TZ is the
	// explicit override path and the deterministic one to pin.
	t.Setenv("TZ", "Europe/London")
	recs := scheduleTestSocket(t)
	if err := cmdSend([]string{"team.broadcast", "weekly sync", "--cron", "0 10 * * MON"}); err != nil {
		t.Fatalf("cmdSend --cron: %v", err)
	}
	got := recs.creates.at(0)
	if got.Kind != "cron" || got.Cron != "0 10 * * MON" {
		t.Fatalf("kind/cron = %q/%q", got.Kind, got.Cron)
	}
	if got.TZ != "Europe/London" {
		t.Fatalf("TZ = %q, want Europe/London ($TZ wins)", got.TZ)
	}
	if got.Handle != "team" || got.Channel != "broadcast" {
		t.Fatalf("dotted target must resolve unchanged: %+v", got)
	}
}

func TestCmdSend_CronTZFallsBackNonEmpty(t *testing.T) {
	// No $TZ: fall back to the system zone (readlink /etc/localtime)
	// or "UTC" — never an empty or unloadable name like Go's "Local".
	t.Setenv("TZ", "")
	_ = os.Unsetenv("TZ")
	recs := scheduleTestSocket(t)
	if err := cmdSend([]string{"bob", "x", "--cron", "0 10 * * MON"}); err != nil {
		t.Fatalf("cmdSend --cron: %v", err)
	}
	got := recs.creates.at(0)
	if got.TZ == "" || got.TZ == "Local" {
		t.Fatalf("TZ = %q, want a loadable IANA zone name", got.TZ)
	}
	if _, err := time.LoadLocation(got.TZ); err != nil {
		t.Fatalf("TZ %q is not loadable: %v", got.TZ, err)
	}
}

func TestCmdSend_ScheduleFlagsMutuallyExclusive(t *testing.T) {
	recs := scheduleTestSocket(t)
	err := cmdSend([]string{"bob", "x", "--at", "+1m", "--every", "5m"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want 'mutually exclusive'", err)
	}
	if recs.creates.count() != 0 || recs.sends.count() != 0 {
		t.Fatal("conflicting flags must not reach the daemon")
	}
}

func TestCmdSend_RequestAckIncompatibleWithSchedule(t *testing.T) {
	// A scheduled fire has no live session for the ack to return to.
	recs := scheduleTestSocket(t)
	err := cmdSend([]string{"bob", "x", "--request-ack", "--at", "+1m"})
	if err == nil || !strings.Contains(err.Error(), "cannot combine --request-ack") {
		t.Fatalf("err = %v, want 'cannot combine --request-ack'", err)
	}
	if recs.creates.count() != 0 || recs.sends.count() != 0 {
		t.Fatal("conflict must not reach the daemon")
	}
}

func TestCmdSend_AtRejectsPast(t *testing.T) {
	recs := scheduleTestSocket(t)
	err := cmdSend([]string{"bob", "x", "--at", "2000-01-01T00:00:00Z"})
	if err == nil || !strings.Contains(err.Error(), "--at is in the past") {
		t.Fatalf("err = %v, want '--at is in the past'", err)
	}
	if recs.creates.count() != 0 {
		t.Fatal("past --at must be rejected client-side, before IPC")
	}
}

func TestCmdSend_RejectsBadEveryAndCron(t *testing.T) {
	recs := scheduleTestSocket(t)
	if err := cmdSend([]string{"bob", "x", "--every", "nonsense"}); err == nil ||
		!strings.Contains(err.Error(), "invalid --every") {
		t.Fatalf("bad --every err = %v, want 'invalid --every'", err)
	}
	if err := cmdSend([]string{"bob", "x", "--cron", "not a cron"}); err == nil ||
		!strings.Contains(err.Error(), "invalid --cron") {
		t.Fatalf("bad --cron err = %v, want 'invalid --cron'", err)
	}
	if recs.creates.count() != 0 {
		t.Fatal("invalid specs must be rejected client-side, before IPC")
	}
}

// --- ppz schedule {ls|rm} ---------------------------------------------------

func TestCmdScheduleLs_CallsScheduleList(t *testing.T) {
	t.Setenv("PPZ_SESSION", "tty-sched-ls")
	recs := scheduleTestSocket(t)
	if err := cmdScheduleGroup([]string{"ls"}); err != nil {
		t.Fatalf("schedule ls: %v", err)
	}
	if recs.lists.count() != 1 {
		t.Fatalf("ScheduleList count = %d, want 1", recs.lists.count())
	}
	if got := recs.lists.at(0); got.Session != "tty-sched-ls" {
		t.Fatalf("Session = %q, want forwarded", got.Session)
	}
}

func TestCmdScheduleRm_CallsScheduleRemoveWithID(t *testing.T) {
	recs := scheduleTestSocket(t)
	if err := cmdScheduleGroup([]string{"rm", "a1b2c3d4"}); err != nil {
		t.Fatalf("schedule rm: %v", err)
	}
	if recs.removes.count() != 1 {
		t.Fatalf("ScheduleRemove count = %d, want 1", recs.removes.count())
	}
	if got := recs.removes.at(0); got.ID != "a1b2c3d4" {
		t.Fatalf("ID = %q, want a1b2c3d4", got.ID)
	}
}

func TestCmdScheduleRm_RequiresID(t *testing.T) {
	recs := scheduleTestSocket(t)
	if err := cmdScheduleGroup([]string{"rm"}); err == nil {
		t.Fatal("schedule rm without an id must error")
	}
	if recs.removes.count() != 0 {
		t.Fatal("missing id must not reach the daemon")
	}
}

// --- help topics --------------------------------------------------------------

func TestHelpTopics_ScheduleVerbAndSendFlags(t *testing.T) {
	sched, ok := helpTopics["schedule"]
	if !ok {
		t.Fatal(`helpTopics["schedule"] missing`)
	}
	for _, want := range []string{"ls", "rm"} {
		if !strings.Contains(sched, want) {
			t.Errorf("schedule help missing %q", want)
		}
	}
	send := helpTopics["send"]
	for _, want := range []string{"--at", "--every", "--cron"} {
		if !strings.Contains(send, want) {
			t.Errorf("send help missing %q", want)
		}
	}
}
