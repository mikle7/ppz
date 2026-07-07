// Package schedule is the pure core of scheduled sends
// (docs/specs/schedule.md): argument parsing for --at/--every/--cron,
// next-fire computation, and the missfire decision the server's firing
// loop applies to a claimed row. Deterministic — callers inject `now`;
// nothing here reads the wall clock.
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// MissfireGrace bounds how late a recurring fire may run. A recurring
// occurrence more than MissfireGrace overdue (server downtime, stalled
// loop) is dropped — next_fire_at advances past now with no catch-up
// burst. One-offs are exempt: they fire once however late.
const MissfireGrace = 60 * time.Second

// MinEvery is the smallest accepted --every interval. It matches the
// firing loop's tick resolution.
const MinEvery = time.Second

// Kind discriminates the three schedule shapes. The strings are wire
// values (cliproto.ScheduleCreateRequest.Kind, schedules.kind column).
type Kind string

const (
	KindAt    Kind = "at"
	KindEvery Kind = "every"
	KindCron  Kind = "cron"
)

// Spec is one schedule's shape. At doubles as the fire instant for
// KindAt and the anchor (creation instant) for KindEvery's interval
// grid. Loc is the IANA zone cron wall-clock times resolve in.
type Spec struct {
	Kind  Kind
	At    time.Time
	Every time.Duration
	Cron  string
	Loc   *time.Location
}

// ErrPast marks an --at instant that isn't strictly in the future.
// Callers branch on it (errors.Is) to word the user-facing message
// separately from parse failures.
var ErrPast = errors.New("instant is not in the future")

// ParseAt resolves an --at argument to a concrete instant. Accepted
// forms: RFC3339 (offset preserved — `schedule ls` renders the
// SCHEDULE cell as typed), "YYYY-MM-DD HH:MM" in loc, and relative
// "+<goduration>". The result must be strictly after now.
func ParseAt(arg string, now time.Time, loc *time.Location) (time.Time, error) {
	if arg == "" {
		return time.Time{}, fmt.Errorf("empty --at")
	}
	var t time.Time
	switch {
	case strings.HasPrefix(arg, "+"):
		d, err := time.ParseDuration(arg[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("relative offset %q: %w", arg, err)
		}
		t = now.Add(d)
	default:
		var err error
		if t, err = time.Parse(time.RFC3339, arg); err != nil {
			if t, err = time.ParseInLocation("2006-01-02 15:04", arg, loc); err != nil {
				return time.Time{}, fmt.Errorf("unrecognised timestamp %q (want RFC3339, \"YYYY-MM-DD HH:MM\", or +duration)", arg)
			}
		}
	}
	if !t.After(now) {
		return time.Time{}, ErrPast
	}
	return t, nil
}

// ParseEvery validates an --every argument: a positive Go duration of
// at least MinEvery.
func ParseEvery(arg string) (time.Duration, error) {
	d, err := time.ParseDuration(arg)
	if err != nil {
		return 0, err
	}
	if d < MinEvery {
		return 0, fmt.Errorf("interval %q below the %s minimum", arg, MinEvery)
	}
	return d, nil
}

// ParseCron validates a standard 5-field cron expression
// (minute hour dom month dow).
func ParseCron(expr string) error {
	_, err := cron.ParseStandard(expr)
	return err
}

// NextAfter computes the first fire instant strictly after `after`,
// or ok=false when the schedule has no future fire (a spent one-off).
func NextAfter(s Spec, after time.Time) (time.Time, bool) {
	switch s.Kind {
	case KindAt:
		if after.Before(s.At) {
			return s.At, true
		}
		return time.Time{}, false
	case KindEvery:
		if s.Every <= 0 {
			return time.Time{}, false
		}
		// Stay on the anchor grid: fires are At+n·Every, never drifting
		// with claim latency. n is the first slot strictly after `after`.
		n := int64(1)
		if diff := after.Sub(s.At); diff > 0 {
			n = int64(diff/s.Every) + 1
		}
		return s.At.Add(time.Duration(n) * s.Every), true
	case KindCron:
		sched, err := cron.ParseStandard(s.Cron)
		if err != nil {
			return time.Time{}, false
		}
		loc := s.Loc
		if loc == nil {
			loc = time.UTC
		}
		next := sched.Next(after.In(loc))
		if next.IsZero() {
			return time.Time{}, false
		}
		return next, true
	}
	return time.Time{}, false
}

// Decision is the firing loop's verdict on one claimed row.
type Decision struct {
	Fire bool      // publish now
	Done bool      // schedule is spent — delete the row
	Next time.Time // next_fire_at to store (meaningless when Done)
}

// Decide applies the missfire policy to a schedule whose stored
// next-fire is `next`, at wall-clock `now`:
//
//   - not yet due            → no fire, next unchanged
//   - one-off due            → fire (however late), done
//   - recurring ≤ grace late → fire, advance to the next occurrence
//   - recurring > grace late → drop the missed occurrences, advance
func Decide(s Spec, next, now time.Time) Decision {
	if next.After(now) {
		return Decision{Next: next}
	}
	if s.Kind == KindAt {
		return Decision{Fire: true, Done: true, Next: next}
	}
	nn, ok := NextAfter(s, now)
	if !ok {
		return Decision{Done: true, Next: next}
	}
	return Decision{Fire: now.Sub(next) <= MissfireGrace, Next: nn}
}
