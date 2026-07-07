package server

// RED — PR #139 review follow-ups (findings #1, #2, #3 + skew grace).
//
// resolveScheduleRow is the pure request→row step of
// handleCreateSchedule: validation (names, kind, spec, tz, payload
// cap) and next-fire computation, with `now` injected. Extracted so
// the REST trust boundary is unit-testable without postgres — the
// daemon always sends resolved targets, but any bearer can hit the
// route directly, so the server must re-validate what it stores.
//
// dropAfterFailure is the scheduler loop's publish-failure verdict:
// permanent failures (target stream gone) drop immediately; anything
// else retries via lease expiry but gives up after
// maxScheduleFailures consecutive failed fires, so an unforeseen
// permanent error can't re-lease and retry every 30s forever.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"

	"github.com/google/uuid"
)

func scheduleReq(mut func(*cliproto.ScheduleServerCreateRequest)) cliproto.ScheduleServerCreateRequest {
	req := cliproto.ScheduleServerCreateRequest{
		Manifold: "",
		Handle:   "bob",
		Pipe:     "inbox",
		Payload:  "standup in 5",
		Sender:   "alice",
		Kind:     "at",
		At:       "2999-01-02T03:04:05Z",
	}
	if mut != nil {
		mut(&req)
	}
	return req
}

func testKey() db.APIKey {
	return db.APIKey{AccountID: uuid.New(), CreatedByUserID: uuid.New()}
}

func resolveNow(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
}

// --- finding #1: server-side target re-validation ---------------------------

func TestResolveScheduleRow_AcceptsValidCollaredTarget(t *testing.T) {
	row, e := resolveScheduleRow(scheduleReq(nil), testKey(), resolveNow(t))
	if e != nil {
		t.Fatalf("valid request rejected: %v", e)
	}
	if row.SourceHandle != "bob" || row.Pipe != "inbox" || row.Kind != "at" {
		t.Fatalf("row: %+v", row)
	}
	if !row.NextFireAt.Equal(time.Date(2999, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("NextFireAt = %v", row.NextFireAt)
	}
}

func TestResolveScheduleRow_RejectsInvalidHandle(t *testing.T) {
	for _, handle := range []string{"a.b", "UPPER", "has*wild", "has>gt", "-lead", strings.Repeat("x", 40)} {
		_, e := resolveScheduleRow(scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
			r.Handle = handle
		}), testKey(), resolveNow(t))
		if e == nil || e.Code != cliproto.EInvalidHandle {
			t.Errorf("handle %q: got %v, want E_INVALID_HANDLE — a stored bad handle becomes a malformed/wildcard NATS subject at fire time", handle, e)
		}
	}
}

func TestResolveScheduleRow_RejectsInvalidManifoldSegment(t *testing.T) {
	for _, manifold := range []string{"ok.BAD", "*", "a..b", "sp ace"} {
		_, e := resolveScheduleRow(scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
			r.Manifold = manifold
			r.Handle = "" // uncollared shape — manifold is the only prefix
		}), testKey(), resolveNow(t))
		if e == nil || e.Code != cliproto.EInvalidManifold {
			t.Errorf("manifold %q: got %v, want E_INVALID_MANIFOLD", manifold, e)
		}
	}
}

func TestResolveScheduleRow_AcceptsUncollaredWithManifold(t *testing.T) {
	row, e := resolveScheduleRow(scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
		r.Manifold = "team-a.sub"
		r.Handle = ""
		r.Pipe = "room"
	}), testKey(), resolveNow(t))
	if e != nil {
		t.Fatalf("valid uncollared request rejected: %v", e)
	}
	if row.Manifold != "team-a.sub" || row.SourceHandle != "" || row.Pipe != "room" {
		t.Fatalf("row: %+v", row)
	}
}

// --- finding #3: payload cap must include the fired schedule_id -------------

// The fired envelope carries `"schedule_id":"<id8>"` that the old
// create-time probe omitted — a payload within those ~25 bytes of the
// cap passed creation and then exceeded MaxBytes on every fire
// (feeding the infinite-retry loop of finding #2).
func TestResolveScheduleRow_PayloadCapCountsScheduleID(t *testing.T) {
	now := resolveNow(t)
	probe := envelope.New("alice", "", "", now)
	probe.ScheduleID = "aaaaaaaa"
	base, err := probe.Marshal()
	if err != nil {
		t.Fatalf("marshal probe: %v", err)
	}

	// One byte over the cap once schedule_id is counted: must reject.
	over := scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
		r.Payload = strings.Repeat("x", envelope.MaxBytes-len(base)+1)
	})
	if _, e := resolveScheduleRow(over, testKey(), now); e == nil || e.Code != cliproto.EPayloadTooLarge {
		t.Errorf("payload 1 byte over the fired-envelope cap: got %v, want E_PAYLOAD_TOO_LARGE", e)
	}

	// Exactly at the cap including schedule_id: must accept.
	at := scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
		r.Payload = strings.Repeat("x", envelope.MaxBytes-len(base))
	})
	if _, e := resolveScheduleRow(at, testKey(), now); e != nil {
		t.Errorf("payload exactly at the fired-envelope cap rejected: %v", e)
	}
}

// --- skew grace: --at validated on the CLI clock must survive latency -------

// The CLI validates strictly-future against ITS clock; by the time the
// request reaches the server, network latency + clock skew can put the
// instant in the server's past (`--at +2s` is a legitimate call — our
// own e2e uses it). Mirror the JWT nbf-backdating precedent: accept up
// to atSkewGrace in the past, reject beyond.
func TestResolveScheduleRow_AtWithinSkewGraceAccepted(t *testing.T) {
	now := resolveNow(t)
	row, e := resolveScheduleRow(scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
		r.At = now.Add(-10 * time.Second).Format(time.RFC3339)
	}), testKey(), now)
	if e != nil {
		t.Fatalf("at 10s in the past (skew/latency) rejected: %v", e)
	}
	// Slightly-past instants fire on the next tick.
	if !row.NextFireAt.Equal(now.Add(-10 * time.Second)) {
		t.Fatalf("NextFireAt = %v, want the requested instant", row.NextFireAt)
	}
}

func TestResolveScheduleRow_AtBeyondSkewGraceRejected(t *testing.T) {
	now := resolveNow(t)
	_, e := resolveScheduleRow(scheduleReq(func(r *cliproto.ScheduleServerCreateRequest) {
		r.At = now.Add(-2 * time.Minute).Format(time.RFC3339)
	}), testKey(), now)
	if e == nil || e.Code != cliproto.EInvalidSchedule {
		t.Fatalf("at 2min in the past: got %v, want E_INVALID_SCHEDULE", e)
	}
}

func TestAtSkewGraceIsThirtySeconds(t *testing.T) {
	if atSkewGrace != 30*time.Second {
		t.Fatalf("atSkewGrace = %v, want 30s (JWT nbf-backdate precedent)", atSkewGrace)
	}
}

// --- existing spec validation moves with the extraction ---------------------

func TestResolveScheduleRow_RejectsBadSpecs(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*cliproto.ScheduleServerCreateRequest)
		want cliproto.Code
	}{
		{"unknown kind", func(r *cliproto.ScheduleServerCreateRequest) { r.Kind = "someday" }, cliproto.EInvalidSchedule},
		{"bad every", func(r *cliproto.ScheduleServerCreateRequest) { r.Kind = "every"; r.At = ""; r.Every = "nonsense" }, cliproto.EInvalidSchedule},
		{"bad cron", func(r *cliproto.ScheduleServerCreateRequest) { r.Kind = "cron"; r.At = ""; r.Cron = "not a cron"; r.TZ = "UTC" }, cliproto.EInvalidSchedule},
		{"bad tz", func(r *cliproto.ScheduleServerCreateRequest) { r.Kind = "cron"; r.At = ""; r.Cron = "0 10 * * MON"; r.TZ = "Nowhere/Nope" }, cliproto.EInvalidSchedule},
		{"bad pipe", func(r *cliproto.ScheduleServerCreateRequest) { r.Pipe = "Bad Pipe!" }, cliproto.EInvalidPipe},
	}
	for _, c := range cases {
		if _, e := resolveScheduleRow(scheduleReq(c.mut), testKey(), resolveNow(t)); e == nil || e.Code != c.want {
			t.Errorf("%s: got %v, want %s", c.name, e, c.want)
		}
	}
}

// --- finding #2: bounded retries for failed fires ----------------------------

func TestDropAfterFailure_StreamGoneDropsImmediately(t *testing.T) {
	if !dropAfterFailure(jetstream.ErrNoStreamResponse, 1) {
		t.Fatal("target stream gone is permanent — drop regardless of fail count")
	}
}

func TestDropAfterFailure_TransientRetriesUntilThreshold(t *testing.T) {
	err := errors.New("nats: timeout")
	// failCount is the post-bump consecutive-failure count.
	if dropAfterFailure(err, maxScheduleFailures-1) {
		t.Fatalf("failCount %d (< %d): must retry via lease expiry", maxScheduleFailures-1, maxScheduleFailures)
	}
	if !dropAfterFailure(err, maxScheduleFailures) {
		t.Fatalf("failCount %d: must drop — no infinite 30s retry loops", maxScheduleFailures)
	}
}

func TestMaxScheduleFailuresIsFive(t *testing.T) {
	if maxScheduleFailures != 5 {
		t.Fatalf("maxScheduleFailures = %d, want 5", maxScheduleFailures)
	}
}
