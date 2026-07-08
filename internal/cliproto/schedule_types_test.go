package cliproto

// RED — docs/specs/schedule.md. Wire-shape pins for the schedule IPC
// surface. Verb strings are wire compatibility: an old CLI against a
// new daemon (or vice versa) matches on these literals, so they get
// the same string-literal pins as the structs' field shapes.

import (
	"testing"
	"time"
)

func TestScheduleIPCVerbWireStrings(t *testing.T) {
	if IPCScheduleCreate != "ScheduleCreate" {
		t.Errorf("IPCScheduleCreate = %q, want %q", IPCScheduleCreate, "ScheduleCreate")
	}
	if IPCScheduleList != "ScheduleList" {
		t.Errorf("IPCScheduleList = %q, want %q", IPCScheduleList, "ScheduleList")
	}
	if IPCScheduleRemove != "ScheduleRemove" {
		t.Errorf("IPCScheduleRemove = %q, want %q", IPCScheduleRemove, "ScheduleRemove")
	}
}

// ScheduleCreateRequest mirrors SendRequest's target fields (Handle /
// Channel / BareTarget / Session / Sender) plus the schedule shape.
// Kind is "at" | "every" | "cron"; exactly one of At/Every/Cron is
// set to match. At is RFC3339 (creator's offset preserved — the CLI
// resolves relative/local forms before IPC), Every is a Go duration
// string, Cron a 5-field expression with TZ carrying the device's
// IANA zone name.
func TestScheduleCreateRequest_Shape(t *testing.T) {
	req := ScheduleCreateRequest{
		Session:    "tty-1",
		Sender:     "alice",
		Handle:     "bob",
		Channel:    "inbox",
		BareTarget: "bob",
		Payload:    "standup in 5",
		Kind:       "at",
		At:         "2026-07-08T09:55:00+01:00",
		Every:      "",
		Cron:       "",
		TZ:         "",
	}
	if req.Kind != "at" || req.At == "" {
		t.Fatalf("round-trip: %+v", req)
	}

	cron := ScheduleCreateRequest{
		Kind: "cron",
		Cron: "0 10 * * MON",
		TZ:   "Europe/London",
	}
	if cron.Cron == "" || cron.TZ == "" {
		t.Fatalf("cron shape: %+v", cron)
	}
}

func TestScheduleCreateReply_Shape(t *testing.T) {
	next := time.Date(2026, 7, 8, 8, 55, 0, 0, time.UTC)
	r := ScheduleCreateReply{ID: "a1b2c3d4", Target: "bob.inbox", NextAt: next}
	if r.ID != "a1b2c3d4" || r.Target != "bob.inbox" || !r.NextAt.Equal(next) {
		t.Fatalf("round-trip: %+v", r)
	}
}

// ScheduleInfo is one `schedule ls` row. Namespace/Handle/Pipe carry
// the same semantics as the ls tables: Namespace is the raw manifold
// ("" for root — renderers dash it), Handle is "" for uncollared
// targets, Pipe the leaf. Spec is the display spec (RFC3339 for at,
// duration for every, expression for cron); TZ is set for cron only.
// LastAt is nil when the schedule has never fired.
func TestScheduleInfo_Shape(t *testing.T) {
	next := time.Date(2026, 7, 8, 8, 55, 0, 0, time.UTC)
	last := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	info := ScheduleInfo{
		ID:        "a1b2c3d4",
		Namespace: "",
		Handle:    "bob",
		Pipe:      "inbox",
		Kind:      "every",
		Spec:      "15m",
		TZ:        "",
		NextAt:    next,
		LastAt:    &last,
		Payload:   "heartbeat check",
		Creator:   "bot-a",
	}
	if info.LastAt == nil || !info.NextAt.Equal(next) {
		t.Fatalf("round-trip: %+v", info)
	}
	info.LastAt = nil // never fired must be representable
}

func TestScheduleListAndRemove_Shapes(t *testing.T) {
	_ = ScheduleListRequest{Session: "tty-1"}
	reply := ScheduleListReply{Schedules: []ScheduleInfo{{ID: "a1b2c3d4"}}}
	if len(reply.Schedules) != 1 {
		t.Fatalf("round-trip: %+v", reply)
	}
	_ = ScheduleRemoveRequest{Session: "tty-1", ID: "a1b2c3d4"}
	rm := ScheduleRemoveReply{ID: "a1b2c3d4"}
	if rm.ID != "a1b2c3d4" {
		t.Fatalf("round-trip: %+v", rm)
	}
}

// Removing an unknown schedule id surfaces a first-class error code,
// following the E_PIPE_NOT_FOUND precedent.
func TestScheduleNotFoundErrorCode(t *testing.T) {
	if EScheduleNotFound != Code("E_SCHEDULE_NOT_FOUND") {
		t.Fatalf("EScheduleNotFound = %q, want E_SCHEDULE_NOT_FOUND", EScheduleNotFound)
	}
}
