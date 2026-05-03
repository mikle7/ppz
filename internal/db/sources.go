package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SourceKind enumerates the supported source shapes.
//   "message" — default; one pipe: broadcast.
//   "pty"     — terminal source; three pipes: broadcast, stdin, stdout.
type SourceKind string

const (
	SourceKindMessage SourceKind = "message"
	SourceKindPTY     SourceKind = "pty"
)

type Source struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	Handle               string
	Kind                 SourceKind
	CreatedAt            time.Time
	LastBroadcastAt      *time.Time
	LastBroadcastPayload *string
}

// Pipes returns the pipe set for a source based on its kind.
// pty sources have three pipes:
//   - broadcast: user-level messages (same as message-kind sources)
//   - stdin: input fed to the wrapped child via `ppz send`
//   - stdout: byte-faithful capture of the PTY master's output (ANSI
//     escapes intact); both `ppz read` and `ppz terminal view` consume
//     this pipe.
func (s Source) Pipes() []string {
	switch s.Kind {
	case SourceKindPTY:
		return []string{"broadcast", "stdin", "stdout", "stdctrl"}
	default:
		return []string{"broadcast"}
	}
}

// ErrHandleTaken is returned when a (org, handle) row already exists.
var ErrHandleTaken = errors.New("handle taken")

func InsertSource(ctx context.Context, p *Pool, orgID uuid.UUID, handle string, kind SourceKind) (Source, error) {
	if kind == "" {
		kind = SourceKindMessage
	}
	src := Source{
		ID:             uuid.New(),
		OrganisationID: orgID,
		Handle:         handle,
		Kind:           kind,
		CreatedAt:      time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO sources (id, organisation_id, handle, kind, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		src.ID, src.OrganisationID, src.Handle, string(src.Kind), src.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Source{}, ErrHandleTaken
		}
		return Source{}, err
	}
	return src, nil
}

func GetSourceByHandle(ctx context.Context, p *Pool, orgID uuid.UUID, handle string) (Source, error) {
	var src Source
	var kind string
	err := p.QueryRow(ctx,
		`SELECT id, organisation_id, handle, kind, created_at, last_broadcast_at, last_broadcast_payload
		   FROM sources WHERE organisation_id = $1 AND handle = $2`, orgID, handle).
		Scan(&src.ID, &src.OrganisationID, &src.Handle, &kind, &src.CreatedAt,
			&src.LastBroadcastAt, &src.LastBroadcastPayload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	src.Kind = SourceKind(kind)
	return src, err
}

func ListSourcesForOrg(ctx context.Context, p *Pool, orgID uuid.UUID) ([]Source, error) {
	rows, err := p.Query(ctx,
		`SELECT id, organisation_id, handle, kind, created_at, last_broadcast_at, last_broadcast_payload
		   FROM sources WHERE organisation_id = $1 ORDER BY handle ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src Source
		var kind string
		if err := rows.Scan(&src.ID, &src.OrganisationID, &src.Handle, &kind, &src.CreatedAt,
			&src.LastBroadcastAt, &src.LastBroadcastPayload); err != nil {
			return nil, err
		}
		src.Kind = SourceKind(kind)
		out = append(out, src)
	}
	return out, rows.Err()
}

// UpdateLastBroadcast records the most recent broadcast for this source.
// Called by the server-side subscriber on every message. Idempotent on
// identical inputs.
func UpdateLastBroadcast(ctx context.Context, p *Pool, orgID uuid.UUID, handle string, at time.Time, payload string) error {
	_, err := p.Exec(ctx,
		`UPDATE sources SET last_broadcast_at = $1, last_broadcast_payload = $2
		   WHERE organisation_id = $3 AND handle = $4`,
		at, payload, orgID, handle)
	return err
}
