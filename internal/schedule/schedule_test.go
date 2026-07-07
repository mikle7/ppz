package schedule

// RED — docs/specs/schedule.md. The schedule package is the pure,
// clock-injected core of scheduled sends: argument parsing for
// --at/--every/--cron, next-fire computation, and the missfire
// decision the server's firing loop applies to a claimed row.
// Everything here is deterministic — callers inject `now`.

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%s): %v", name, err)
	}
	return loc
}

func ts(t *testing.T, v string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("parse %q: %v", v, err)
	}
	return parsed
}

// --- ParseAt ------------------------------------------------------------

func TestParseAt_RFC3339PassesThrough(t *testing.T) {
	now := ts(t, "2026-07-07T12:00:00Z")
	got, err := ParseAt("2026-07-08T09:55:00Z", now, time.UTC)
	if err != nil {
		t.Fatalf("ParseAt: %v", err)
	}
	if !got.Equal(ts(t, "2026-07-08T09:55:00Z")) {
		t.Fatalf("got %v, want 2026-07-08T09:55:00Z", got)
	}
}

func TestParseAt_RFC3339KeepsOffset(t *testing.T) {
	// The typed offset must survive — `schedule ls` renders the SCHEDULE
	// cell in the creator's offset, so ParseAt must not normalise to UTC.
	now := ts(t, "2026-07-07T12:00:00Z")
	got, err := ParseAt("2026-07-08T09:55:00+01:00", now, time.UTC)
	if err != nil {
		t.Fatalf("ParseAt: %v", err)
	}
	if !got.Equal(ts(t, "2026-07-08T08:55:00Z")) {
		t.Fatalf("instant: got %v, want 08:55Z", got)
	}
	if got.Format(time.RFC3339) != "2026-07-08T09:55:00+01:00" {
		t.Fatalf("offset lost: renders as %s, want +01:00 preserved", got.Format(time.RFC3339))
	}
}

func TestParseAt_LocalDateTimeUsesDeviceZone(t *testing.T) {
	// "YYYY-MM-DD HH:MM" is interpreted in the device zone. July in
	// New York is EDT (UTC-4).
	now := ts(t, "2026-07-07T12:00:00Z")
	got, err := ParseAt("2026-07-08 09:55", now, mustLoc(t, "America/New_York"))
	if err != nil {
		t.Fatalf("ParseAt: %v", err)
	}
	if !got.Equal(ts(t, "2026-07-08T13:55:00Z")) {
		t.Fatalf("got %v, want 2026-07-08T13:55:00Z (09:55 EDT)", got)
	}
}

func TestParseAt_RelativePlusDuration(t *testing.T) {
	now := ts(t, "2026-07-07T12:00:00Z")
	got, err := ParseAt("+5m", now, time.UTC)
	if err != nil {
		t.Fatalf("ParseAt: %v", err)
	}
	if !got.Equal(ts(t, "2026-07-07T12:05:00Z")) {
		t.Fatalf("got %v, want now+5m", got)
	}
}

func TestParseAt_RejectsPastAndPresent(t *testing.T) {
	now := ts(t, "2026-07-07T12:00:00Z")
	for _, arg := range []string{
		"2000-01-01T00:00:00Z",   // long past
		"2026-07-07T12:00:00Z",   // exactly now — must be strictly future
		"+0s",                    // relative zero
	} {
		if _, err := ParseAt(arg, now, time.UTC); err == nil {
			t.Errorf("ParseAt(%q) accepted a non-future instant", arg)
		}
	}
}

func TestParseAt_RejectsGarbage(t *testing.T) {
	now := ts(t, "2026-07-07T12:00:00Z")
	for _, arg := range []string{"", "tomorrow", "2026-13-40T99:99:99Z", "+nonsense"} {
		if _, err := ParseAt(arg, now, time.UTC); err == nil {
			t.Errorf("ParseAt(%q) accepted garbage", arg)
		}
	}
}

// --- ParseEvery ---------------------------------------------------------

func TestParseEvery_AcceptsGoDurations(t *testing.T) {
	cases := map[string]time.Duration{
		"1s":    time.Second,
		"15m":   15 * time.Minute,
		"1h30m": 90 * time.Minute,
		"24h":   24 * time.Hour,
	}
	for arg, want := range cases {
		got, err := ParseEvery(arg)
		if err != nil {
			t.Errorf("ParseEvery(%q): %v", arg, err)
			continue
		}
		if got != want {
			t.Errorf("ParseEvery(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestParseEvery_RejectsSubSecondZeroNegativeGarbage(t *testing.T) {
	for _, arg := range []string{"500ms", "0s", "-5m", "nonsense", ""} {
		if _, err := ParseEvery(arg); err == nil {
			t.Errorf("ParseEvery(%q) accepted, want error (minimum 1s, positive, valid)", arg)
		}
	}
}

// --- ParseCron ----------------------------------------------------------

func TestParseCron_AcceptsStandardFiveField(t *testing.T) {
	for _, expr := range []string{
		"0 10 * * MON",
		"30 17 * * MON-FRI",
		"0 9 1 * *",
		"*/15 * * * *",
	} {
		if err := ParseCron(expr); err != nil {
			t.Errorf("ParseCron(%q): %v", expr, err)
		}
	}
}

func TestParseCron_RejectsInvalid(t *testing.T) {
	for _, expr := range []string{
		"",
		"not a cron",
		"* * *",        // too few fields
		"61 0 * * *",   // minute out of range
		"0 25 * * *",   // hour out of range
	} {
		if err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) accepted, want error", expr)
		}
	}
}

// --- NextAfter ----------------------------------------------------------

func TestNextAfter_OneOff(t *testing.T) {
	at := ts(t, "2026-07-08T09:55:00Z")
	s := Spec{Kind: KindAt, At: at}

	next, ok := NextAfter(s, ts(t, "2026-07-07T12:00:00Z"))
	if !ok || !next.Equal(at) {
		t.Fatalf("before the instant: got (%v,%v), want (%v,true)", next, ok, at)
	}
	if _, ok := NextAfter(s, at); ok {
		t.Fatal("at/after the instant: a one-off has no next fire")
	}
}

func TestNextAfter_EveryStaysOnAnchorGrid(t *testing.T) {
	// Anchor (creation instant) 12:00, every 15m. First fire is
	// anchor+interval; a late `after` skips forward on the anchor grid
	// rather than drifting.
	anchor := ts(t, "2026-07-07T12:00:00Z")
	s := Spec{Kind: KindEvery, At: anchor, Every: 15 * time.Minute}

	cases := []struct{ after, want string }{
		{"2026-07-07T12:00:00Z", "2026-07-07T12:15:00Z"}, // first fire
		{"2026-07-07T12:34:00Z", "2026-07-07T12:45:00Z"}, // mid-grid
		{"2026-07-07T12:45:00Z", "2026-07-07T13:00:00Z"}, // exactly on a slot → strictly after
		{"2026-07-07T14:03:00Z", "2026-07-07T14:15:00Z"}, // hours of missed slots skipped
	}
	for _, c := range cases {
		next, ok := NextAfter(s, ts(t, c.after))
		if !ok {
			t.Errorf("after %s: no next fire, want %s", c.after, c.want)
			continue
		}
		if !next.Equal(ts(t, c.want)) {
			t.Errorf("after %s: next = %v, want %s", c.after, next, c.want)
		}
	}
}

func TestNextAfter_CronHonoursTimezoneAcrossDST(t *testing.T) {
	// "0 10 * * *" in Europe/London: 10:00 BST (=09:00Z) in July,
	// 10:00 GMT (=10:00Z) in January.
	s := Spec{Kind: KindCron, Cron: "0 10 * * *", Loc: mustLoc(t, "Europe/London")}

	next, ok := NextAfter(s, ts(t, "2026-07-01T00:00:00Z"))
	if !ok || !next.Equal(ts(t, "2026-07-01T09:00:00Z")) {
		t.Fatalf("summer: got (%v,%v), want 2026-07-01T09:00:00Z", next, ok)
	}
	next, ok = NextAfter(s, ts(t, "2026-01-15T00:00:00Z"))
	if !ok || !next.Equal(ts(t, "2026-01-15T10:00:00Z")) {
		t.Fatalf("winter: got (%v,%v), want 2026-01-15T10:00:00Z", next, ok)
	}
}

func TestNextAfter_CronWeekly(t *testing.T) {
	// 2026-07-08 is a Wednesday; next MON 10:00 UTC is 2026-07-13.
	s := Spec{Kind: KindCron, Cron: "0 10 * * MON", Loc: time.UTC}
	next, ok := NextAfter(s, ts(t, "2026-07-08T12:00:00Z"))
	if !ok || !next.Equal(ts(t, "2026-07-13T10:00:00Z")) {
		t.Fatalf("got (%v,%v), want 2026-07-13T10:00:00Z", next, ok)
	}
}

// --- Decide (missfire policy) --------------------------------------------

func TestDecide_NotDueYet(t *testing.T) {
	s := Spec{Kind: KindEvery, At: ts(t, "2026-07-07T12:00:00Z"), Every: 15 * time.Minute}
	next := ts(t, "2026-07-07T12:15:00Z")
	d := Decide(s, next, ts(t, "2026-07-07T12:10:00Z"))
	if d.Fire || d.Done {
		t.Fatalf("not due: got %+v, want no fire, not done", d)
	}
	if !d.Next.Equal(next) {
		t.Fatalf("not due must leave next unchanged: got %v", d.Next)
	}
}

func TestDecide_OneOffFiresWhenDue(t *testing.T) {
	at := ts(t, "2026-07-07T12:00:00Z")
	s := Spec{Kind: KindAt, At: at}
	d := Decide(s, at, ts(t, "2026-07-07T12:00:01Z"))
	if !d.Fire || !d.Done {
		t.Fatalf("due one-off: got %+v, want Fire=true Done=true", d)
	}
}

func TestDecide_OneOffFiresHoursLate(t *testing.T) {
	// Missfire policy: a one-off fires once immediately, however late —
	// "server was down at fire time" must not swallow the message.
	at := ts(t, "2026-07-07T12:00:00Z")
	s := Spec{Kind: KindAt, At: at}
	d := Decide(s, at, ts(t, "2026-07-07T18:30:00Z"))
	if !d.Fire || !d.Done {
		t.Fatalf("late one-off: got %+v, want Fire=true Done=true", d)
	}
}

func TestDecide_RecurringFiresWithinGrace(t *testing.T) {
	anchor := ts(t, "2026-07-07T12:00:00Z")
	s := Spec{Kind: KindEvery, At: anchor, Every: 15 * time.Minute}
	next := ts(t, "2026-07-07T12:15:00Z")

	// 30s late — normal loop jitter, fires.
	d := Decide(s, next, next.Add(30*time.Second))
	if !d.Fire || d.Done {
		t.Fatalf("30s overdue: got %+v, want Fire=true Done=false", d)
	}
	if !d.Next.Equal(ts(t, "2026-07-07T12:30:00Z")) {
		t.Fatalf("next after fire = %v, want 12:30:00Z", d.Next)
	}

	// Exactly MissfireGrace late — boundary is inclusive, still fires.
	d = Decide(s, next, next.Add(MissfireGrace))
	if !d.Fire {
		t.Fatalf("overdue == MissfireGrace: got %+v, want Fire=true", d)
	}
}

func TestDecide_RecurringSkipsBeyondGrace(t *testing.T) {
	// >grace overdue (downtime): drop the missed occurrences, no
	// catch-up burst, advance next past now.
	anchor := ts(t, "2026-07-07T12:00:00Z")
	s := Spec{Kind: KindEvery, At: anchor, Every: 15 * time.Minute}
	next := ts(t, "2026-07-07T12:15:00Z")
	now := ts(t, "2026-07-07T13:07:00Z") // 52m overdue, 3 slots missed

	d := Decide(s, next, now)
	if d.Fire || d.Done {
		t.Fatalf("beyond grace: got %+v, want Fire=false Done=false", d)
	}
	if !d.Next.Equal(ts(t, "2026-07-07T13:15:00Z")) {
		t.Fatalf("next = %v, want first grid slot after now (13:15:00Z)", d.Next)
	}
}

func TestDecide_CronSkipsBeyondGrace(t *testing.T) {
	s := Spec{Kind: KindCron, Cron: "0 10 * * *", Loc: time.UTC}
	next := ts(t, "2026-07-07T10:00:00Z")
	now := ts(t, "2026-07-07T14:00:00Z") // 4h overdue

	d := Decide(s, next, now)
	if d.Fire || d.Done {
		t.Fatalf("cron beyond grace: got %+v, want Fire=false Done=false", d)
	}
	if !d.Next.Equal(ts(t, "2026-07-08T10:00:00Z")) {
		t.Fatalf("next = %v, want tomorrow 10:00Z", d.Next)
	}
}

func TestMissfireGraceIsOneMinute(t *testing.T) {
	if MissfireGrace != 60*time.Second {
		t.Fatalf("MissfireGrace = %v, want 60s (docs/specs/schedule.md)", MissfireGrace)
	}
}
