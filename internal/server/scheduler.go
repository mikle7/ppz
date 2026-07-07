package server

// The scheduled-send firing loop (docs/specs/schedule.md) — the
// server's data-plane publisher. Every tick it claims due rows
// (SKIP LOCKED + a short lease, so replicas never double-fire and a
// crashed claimer re-offers the row), applies the missfire policy
// (schedule.Decide), publishes fired envelopes over the org's pooled
// NATS connection, and settles each row: recurring rows advance
// next_fire_at, spent one-offs delete.
//
// Envelope-shape parity with the daemon's live-send path matters:
// receivers must not be able to tell a scheduled message apart except
// by the schedule_id marker.

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
	"github.com/pipescloud/ppz/internal/schedule"
)

const (
	schedulerTick  = time.Second
	schedulerBatch = 100
	// maxScheduleFailures bounds retries for a failing fire: after this
	// many CONSECUTIVE failed publishes (counter resets on success) the
	// schedule is dropped. Without it an unforeseen permanent error —
	// anything we don't classify below — would re-lease and retry every
	// scheduleClaimLease forever (PR #139 finding #2).
	maxScheduleFailures = 5
)

// dropAfterFailure is the loop's verdict on one failed publish.
// failCount is the post-bump consecutive-failure count.
func dropAfterFailure(err error, failCount int) bool {
	if errors.Is(err, jetstream.ErrNoStreamResponse) {
		// Target stream is gone (source/pipe destroyed after the
		// schedule was created) — permanent, drop immediately.
		return true
	}
	return failCount >= maxScheduleFailures
}

func (s *Server) runScheduler(ctx context.Context) {
	t := time.NewTicker(schedulerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.fireDueSchedules(ctx)
		}
	}
}

func (s *Server) fireDueSchedules(ctx context.Context) {
	now := time.Now().UTC()
	claimed, err := db.ClaimDueSchedules(ctx, s.Pool, now, schedulerBatch)
	if err != nil {
		log.Printf("scheduler: claim due: %v", err)
		return
	}
	for _, row := range claimed {
		spec, err := rowSpec(row)
		if err != nil {
			// Poison row (unparseable spec should be impossible past the
			// create-time validation) — delete rather than re-claim it
			// every lease expiry forever.
			log.Printf("scheduler: dropping schedule %s: %v", row.ShortID(), err)
			_ = db.CompleteFire(ctx, s.Pool, row.ID, nil, time.Time{})
			continue
		}
		d := schedule.Decide(spec, row.NextFireAt, now)
		if d.Fire {
			if err := s.publishScheduled(ctx, row); err != nil {
				// Count the consecutive failure; the pre-claim row carries
				// the previous count, so fall back to it if the bump
				// itself fails (db hiccup) rather than losing the verdict.
				failCount := row.FailCount + 1
				if n, berr := db.BumpScheduleFailCount(ctx, s.Pool, row.ID); berr == nil {
					failCount = n
				}
				if dropAfterFailure(err, failCount) {
					log.Printf("scheduler: dropping schedule %s after %d failed fire(s): %v", row.ShortID(), failCount, err)
					_ = db.CompleteFire(ctx, s.Pool, row.ID, nil, time.Time{})
					continue
				}
				// Presumed transient (NATS hiccup): leave the row leased;
				// it re-offers when the claim lease expires and the fire
				// retries. One-off lateness is covered by the missfire
				// policy; recurring rows just skip per Decide next round.
				log.Printf("scheduler: publish schedule %s (failure %d/%d, will retry): %v", row.ShortID(), failCount, maxScheduleFailures, err)
				continue
			}
		}
		var next *time.Time
		if !d.Done {
			n := d.Next.UTC()
			next = &n
		}
		firedAt := time.Time{}
		if d.Fire {
			firedAt = now
		}
		if err := db.CompleteFire(ctx, s.Pool, row.ID, next, firedAt); err != nil {
			log.Printf("scheduler: settle schedule %s: %v", row.ShortID(), err)
		}
	}
}

// rowSpec rebuilds the pure schedule.Spec from a stored row. CreatedAt
// anchors kind=every's grid; TZ resolves kind=cron's wall-clock times.
func rowSpec(row db.Schedule) (schedule.Spec, error) {
	spec := schedule.Spec{Kind: schedule.Kind(row.Kind)}
	switch spec.Kind {
	case schedule.KindAt:
		t, err := time.Parse(time.RFC3339, row.Spec)
		if err != nil {
			return schedule.Spec{}, err
		}
		spec.At = t
	case schedule.KindEvery:
		d, err := time.ParseDuration(row.Spec)
		if err != nil {
			return schedule.Spec{}, err
		}
		spec.At = row.CreatedAt
		spec.Every = d
	case schedule.KindCron:
		if err := schedule.ParseCron(row.Spec); err != nil {
			return schedule.Spec{}, err
		}
		loc, err := time.LoadLocation(row.TZ)
		if err != nil {
			// Should be impossible past create-time validation (e.g. the
			// host lost its tzdata) — falling back silently would shift
			// fire times without a trace, so leave a trail.
			log.Printf("scheduler: schedule %s tz %q not loadable, falling back to UTC: %v", row.ShortID(), row.TZ, err)
			loc = time.UTC
		}
		spec.Cron = row.Spec
		spec.Loc = loc
	default:
		return schedule.Spec{}, errors.New("unknown kind " + row.Kind)
	}
	return spec, nil
}

// publishScheduled emits one fired envelope, blocking for the
// JetStream PubAck — same durable-write contract as a live send.
func (s *Server) publishScheduled(ctx context.Context, row db.Schedule) error {
	js, err := s.JSFor(ctx, row.AccountID)
	if err != nil {
		return err
	}
	env := envelope.New(row.Sender, "", row.Payload, time.Now().UTC())
	env.ScheduleID = row.ShortID()
	data, err := env.Marshal()
	if err != nil {
		return err
	}
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	subject := natsubj.BuildSubject(row.AccountID, row.Manifold, row.SourceHandle, row.Pipe)
	_, err = js.Publish(pubCtx, subject, data)
	return err
}
