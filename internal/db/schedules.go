package db

// Scheduled sends (docs/specs/schedule.md) — repo functions over the
// `schedules` table (0004_schedules.sql). The firing loop's claim
// path uses FOR UPDATE SKIP LOCKED plus a short lease bump so multiple
// server replicas never double-fire the same occurrence.

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Schedule is one live scheduled send. SourceHandle=="" targets an
// uncollared pipe at Manifold. Spec is display-shaped (RFC3339 for
// at, Go duration for every, cron expression for cron); CreatedAt
// anchors the interval grid for kind=every. LastFiredAt is nil until
// the first fire. CreatorUsername is join-filled by ListSchedules for
// the `schedule ls` CREATOR column (not a table column).
type Schedule struct {
	ID              uuid.UUID
	AccountID       uuid.UUID
	Manifold        string
	SourceHandle    string
	Pipe            string
	Payload         string
	Sender          string
	Kind            string
	Spec            string
	TZ              string
	NextFireAt      time.Time
	LastFiredAt     *time.Time
	CreatedByUserID uuid.UUID
	CreatedAt       time.Time
	CreatorUsername string
}

// ShortID returns the CLI-facing 8-char id: the last 8 hex of the
// row's uuid, matching `sent id=…`'s lastHex8 convention.
func (s Schedule) ShortID() string {
	stripped := strings.ReplaceAll(s.ID.String(), "-", "")
	return stripped[len(stripped)-8:]
}

const scheduleCols = `id, account_id, manifold, source_handle, pipe, payload, sender,
	kind, spec, tz, next_fire_at, last_fired_at, created_by_user_id, created_at`

// InsertSchedule stores a new schedule. ID is stamped here; CreatedAt
// is honoured when set (it anchors kind=every's interval grid, so the
// handler passes the same instant it computed next_fire_at from) and
// stamped otherwise.
func InsertSchedule(ctx context.Context, p *Pool, s Schedule) (Schedule, error) {
	s.ID = uuid.New()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	_, err := p.Exec(ctx,
		`INSERT INTO schedules (id, account_id, manifold, source_handle, pipe, payload, sender,
		                        kind, spec, tz, next_fire_at, last_fired_at, created_by_user_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		s.ID, s.AccountID, s.Manifold, s.SourceHandle, s.Pipe, s.Payload, s.Sender,
		s.Kind, s.Spec, s.TZ, s.NextFireAt, s.LastFiredAt, s.CreatedByUserID, s.CreatedAt)
	if err != nil {
		return Schedule{}, err
	}
	return s, nil
}

// ListSchedules returns one account's live schedules, soonest
// next_fire_at first, with CreatorUsername join-filled.
func ListSchedules(ctx context.Context, p *Pool, accountID uuid.UUID) ([]Schedule, error) {
	rows, err := p.Query(ctx,
		`SELECT s.id, s.account_id, s.manifold, s.source_handle, s.pipe, s.payload, s.sender,
		        s.kind, s.spec, s.tz, s.next_fire_at, s.last_fired_at, s.created_by_user_id, s.created_at,
		        COALESCE(u.username, '')
		   FROM schedules s
		   LEFT JOIN users u ON u.id = s.created_by_user_id
		  WHERE s.account_id = $1
		  ORDER BY s.next_fire_at ASC, s.id ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var s Schedule
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Manifold, &s.SourceHandle, &s.Pipe, &s.Payload, &s.Sender,
			&s.Kind, &s.Spec, &s.TZ, &s.NextFireAt, &s.LastFiredAt, &s.CreatedByUserID, &s.CreatedAt,
			&s.CreatorUsername); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteScheduleByShortID removes one schedule addressed by the
// CLI-facing short id (last 8 hex of the uuid). Returns whether a row
// matched — false maps to E_SCHEDULE_NOT_FOUND at the handler.
func DeleteScheduleByShortID(ctx context.Context, p *Pool, accountID uuid.UUID, shortID string) (bool, error) {
	tag, err := p.Exec(ctx,
		`DELETE FROM schedules
		  WHERE account_id = $1
		    AND RIGHT(REPLACE(id::text, '-', ''), 8) = $2`,
		accountID, strings.ToLower(shortID))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// scheduleClaimLease is how far ClaimDueSchedules pushes a claimed
// row's next_fire_at into the future. It makes the claim visible
// across replicas (a row mid-fire stops matching `next_fire_at <=
// now`) without holding a transaction open through the NATS publish.
// CompleteFire overwrites it with the real next (or deletes the row)
// as soon as the fire settles; if the claimer crashes mid-fire, the
// lease expiring re-offers the row rather than losing it.
const scheduleClaimLease = 30 * time.Second

// ClaimDueSchedules atomically claims up to limit due rows
// (next_fire_at <= now): SKIP LOCKED against concurrent claimers,
// lease-bumped so the claim survives past the transaction. Returns
// the rows with their PRE-claim next_fire_at — the value the missfire
// policy (schedule.Decide) judges lateness against.
func ClaimDueSchedules(ctx context.Context, p *Pool, now time.Time, limit int) ([]Schedule, error) {
	rows, err := p.Query(ctx,
		`WITH due AS (
		    SELECT `+scheduleCols+`
		      FROM schedules
		     WHERE next_fire_at <= $1
		     ORDER BY next_fire_at ASC
		     LIMIT $2
		       FOR UPDATE SKIP LOCKED
		 )
		 UPDATE schedules s
		    SET next_fire_at = $1 + make_interval(secs => $3)
		   FROM due
		  WHERE s.id = due.id
		 RETURNING due.id, due.account_id, due.manifold, due.source_handle, due.pipe, due.payload, due.sender,
		           due.kind, due.spec, due.tz, due.next_fire_at, due.last_fired_at, due.created_by_user_id, due.created_at`,
		now, limit, scheduleClaimLease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var s Schedule
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Manifold, &s.SourceHandle, &s.Pipe, &s.Payload, &s.Sender,
			&s.Kind, &s.Spec, &s.TZ, &s.NextFireAt, &s.LastFiredAt, &s.CreatedByUserID, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CompleteFire settles a claimed row: next==nil deletes it (spent
// one-off), otherwise next_fire_at is set to *next. A non-zero
// firedAt stamps last_fired_at (a beyond-grace skip advances without
// stamping).
func CompleteFire(ctx context.Context, p *Pool, id uuid.UUID, next *time.Time, firedAt time.Time) error {
	if next == nil {
		_, err := p.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, id)
		return err
	}
	if firedAt.IsZero() {
		_, err := p.Exec(ctx, `UPDATE schedules SET next_fire_at = $2 WHERE id = $1`, id, *next)
		return err
	}
	_, err := p.Exec(ctx,
		`UPDATE schedules SET next_fire_at = $2, last_fired_at = $3 WHERE id = $1`, id, *next, firedAt)
	return err
}
