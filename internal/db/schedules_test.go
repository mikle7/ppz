package db

// RED — docs/specs/schedule.md. Durable schedule rows live in
// postgres (migration 0004_schedules.sql): the resolved target,
// payload, sender, and the schedule shape, keyed by account. The
// firing loop claims due rows with FOR UPDATE SKIP LOCKED so multiple
// server replicas never double-fire.
//
// Struct/signature pins follow the pipes_creator_test.go pattern;
// live row behaviour is covered by tests/schedule/ e2e.

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSchedule_Fields(t *testing.T) {
	now := time.Now()
	s := Schedule{
		ID:              uuid.New(),
		AccountID:       uuid.New(),
		Manifold:        "",
		SourceHandle:    "bob",
		Pipe:            "inbox",
		Payload:         "standup in 5",
		Sender:          "alice",
		Kind:            "at",
		Spec:            "2026-07-08T09:55:00+01:00",
		TZ:              "",
		NextFireAt:      now,
		LastFiredAt:     nil,
		CreatedByUserID: uuid.New(),
	}
	if s.Kind != "at" || !s.NextFireAt.Equal(now) {
		t.Fatalf("round-trip: %+v", s)
	}

	typ := reflect.TypeOf(Schedule{})
	if f, ok := typ.FieldByName("LastFiredAt"); !ok || f.Type != reflect.TypeOf((*time.Time)(nil)) {
		t.Error("Schedule.LastFiredAt must be *time.Time (nil = never fired)")
	}
	if f, ok := typ.FieldByName("NextFireAt"); !ok || f.Type != reflect.TypeOf(time.Time{}) {
		t.Error("Schedule.NextFireAt must be time.Time (always set while the row lives)")
	}
	// PR #139 finding #2: consecutive failed-fire counter — the
	// scheduler drops the row when it crosses the threshold instead of
	// re-leasing and retrying every 30s forever.
	if f, ok := typ.FieldByName("FailCount"); !ok || f.Type.Kind() != reflect.Int {
		t.Error("Schedule.FailCount must be int (consecutive failed fires; reset on success)")
	}
}

// Compile-time signature pins for the repo functions the handlers and
// the firing loop consume.
func TestScheduleRepoSignatures(t *testing.T) {
	_ = func(ctx context.Context, pool *Pool, accountID uuid.UUID, s Schedule, now time.Time) {
		// Create — returns the stored row (id populated).
		_, _ = InsertSchedule(ctx, pool, s)
		// List for `schedule ls` — one org, next_fire_at ascending.
		_, _ = ListSchedules(ctx, pool, accountID)
		// Remove by the CLI-facing short id (last-8-hex). The bool
		// reports whether a row matched — false maps to
		// E_SCHEDULE_NOT_FOUND at the handler.
		_, _ = DeleteScheduleByShortID(ctx, pool, accountID, "a1b2c3d4")
		// Firing loop: claim due rows (next_fire_at <= now) with
		// FOR UPDATE SKIP LOCKED, bounded per tick.
		_, _ = ClaimDueSchedules(ctx, pool, now, 100)
		// Post-fire: advance next (recurring) or delete (next == nil,
		// one-off done), stamping last_fired_at. A successful fire also
		// resets fail_count.
		_ = CompleteFire(ctx, pool, s.ID, nil, now)
		// PR #139 finding #2: bump the consecutive-failure counter after
		// a failed publish; returns the post-bump count the scheduler
		// compares against its drop threshold.
		_, _ = BumpScheduleFailCount(ctx, pool, s.ID)
	}
}

// PR #139 finding #4: a short id (last-8-hex) can in principle match
// two rows in one account. `schedule rm` must never silently delete
// both — DeleteScheduleByShortID returns ErrScheduleIDAmbiguous
// instead, and InsertSchedule avoids minting a colliding suffix in the
// first place (regenerates the uuid on a per-account suffix
// collision).
func TestScheduleShortIDAmbiguityGuard(t *testing.T) {
	var e error = ErrScheduleIDAmbiguous
	if e == nil || !strings.Contains(e.Error(), "ambiguous") {
		t.Fatalf("ErrScheduleIDAmbiguous = %v, want an error mentioning ambiguity", e)
	}
}

// Migration 0004 is embedded and creates the table plus the index the
// due-row poll depends on.
func TestSchedulesMigrationEmbedded(t *testing.T) {
	raw, err := migrationsFS.ReadFile("migrations/0004_schedules.sql")
	if err != nil {
		t.Fatalf("migrations/0004_schedules.sql not embedded: %v", err)
	}
	sql := strings.ToLower(string(raw))
	// The runner re-applies migrations on every boot, so the repo
	// convention is idempotent DDL (IF NOT EXISTS) — accept it.
	if !strings.Contains(sql, "create table if not exists schedules") &&
		!strings.Contains(sql, "create table schedules") {
		t.Error("0004_schedules.sql must create table schedules")
	}
	if !strings.Contains(sql, "next_fire_at") {
		t.Error("schedules must carry next_fire_at")
	}
	if !strings.Contains(sql, "index") {
		t.Error("schedules needs an index on next_fire_at — the firing loop polls it every tick")
	}
	if !strings.Contains(sql, "fail_count") {
		t.Error("schedules must carry fail_count (PR #139 finding #2 — bounded retries for failed fires)")
	}
}
