package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Pipe is one user-creatable sub-bucket on a source. Auto-provisioned pipes
// (broadcast, stdin, stdout) are NOT stored here — they're derived from the
// source's kind and joined in at API response time.
type Pipe struct {
	ID         uuid.UUID
	SourceID   uuid.UUID
	Name       string
	TTLSeconds *int   // nil = use server default (86400 s)
	MaxMsgs    *int   // nil = use server default (1000)
	MaxBytes   *int64 // nil = use server default (64 MiB)
	CreatedAt  time.Time
}

// ErrPipeNameTaken — (source_id, name) collision on insert.
var ErrPipeNameTaken = errors.New("pipe name taken")

// InsertPipe inserts a row. Retention overrides are NULL when the pointer
// arg is nil — the server provisions the JetStream stream with default
// values for any nil fields.
func InsertPipe(ctx context.Context, p *Pool, sourceID uuid.UUID, name string, ttl *int, maxMsgs *int, maxBytes *int64) (Pipe, error) {
	pipe := Pipe{
		ID:         uuid.New(),
		SourceID:   sourceID,
		Name:       name,
		TTLSeconds: ttl,
		MaxMsgs:    maxMsgs,
		MaxBytes:   maxBytes,
		CreatedAt:  time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO pipes (id, source_id, name, ttl_seconds, max_msgs, max_bytes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pipe.ID, pipe.SourceID, pipe.Name, pipe.TTLSeconds, pipe.MaxMsgs, pipe.MaxBytes, pipe.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Pipe{}, ErrPipeNameTaken
		}
		return Pipe{}, err
	}
	return pipe, nil
}

// ListPipesForSource returns the user-creatable pipes for one source,
// sorted by name. Excludes auto-provisioned pipes (those aren't stored).
func ListPipesForSource(ctx context.Context, p *Pool, sourceID uuid.UUID) ([]Pipe, error) {
	rows, err := p.Query(ctx,
		`SELECT id, source_id, name, ttl_seconds, max_msgs, max_bytes, created_at
		   FROM pipes WHERE source_id = $1 ORDER BY name ASC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pipe
	for rows.Next() {
		var pipe Pipe
		if err := rows.Scan(&pipe.ID, &pipe.SourceID, &pipe.Name,
			&pipe.TTLSeconds, &pipe.MaxMsgs, &pipe.MaxBytes, &pipe.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, pipe)
	}
	return out, rows.Err()
}

// GetPipeByName returns one pipe row or ErrNotFound.
func GetPipeByName(ctx context.Context, p *Pool, sourceID uuid.UUID, name string) (Pipe, error) {
	var pipe Pipe
	err := p.QueryRow(ctx,
		`SELECT id, source_id, name, ttl_seconds, max_msgs, max_bytes, created_at
		   FROM pipes WHERE source_id = $1 AND name = $2`, sourceID, name).
		Scan(&pipe.ID, &pipe.SourceID, &pipe.Name,
			&pipe.TTLSeconds, &pipe.MaxMsgs, &pipe.MaxBytes, &pipe.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pipe{}, ErrNotFound
	}
	return pipe, err
}

// DeletePipe removes the row. Returns ErrNotFound when (source, name) doesn't
// exist. Stream cleanup is the caller's responsibility (server-side).
func DeletePipe(ctx context.Context, p *Pool, sourceID uuid.UUID, name string) error {
	tag, err := p.Exec(ctx,
		`DELETE FROM pipes WHERE source_id = $1 AND name = $2`, sourceID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
