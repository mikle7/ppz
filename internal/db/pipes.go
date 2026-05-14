package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Pipe is one user-creatable channel. Phase 1.5: pipes carry an explicit
// manifold (hierarchical-grouping segment, '' = root) and an account_id
// (denormalised from source.account_id for collared rows, explicit for
// uncollared ones). SourceID is nullable — uncollared (sourceless) pipes
// are symmetric many-to-many channels under a manifold.
//
// Auto-provisioned pipes (stdin, stdout, stdctrl, inbox) are NOT stored
// here — they're derived from the source's kind and joined in at API
// response time.
type Pipe struct {
	ID              uuid.UUID
	AccountID       uuid.UUID  // tenancy anchor (denormalised; matches source.account_id when SourceID is set)
	Manifold        string     // hierarchical-grouping segment; '' = root (NOT NULL on the DB column)
	SourceID        *uuid.UUID // nil for uncollared (sourceless) pipes
	CreatedByUserID uuid.UUID  // user that created the pipe (NOT NULL)
	Name            string
	TTLSeconds      *int   // nil = use server default (86400 s)
	MaxMsgs         *int   // nil = use server default (1000)
	MaxBytes        *int64 // nil = use server default (64 MiB)
	CreatedAt       time.Time
}

// ErrPipeNameTaken — uniqueness collision on insert. The partial UNIQUE
// indexes split this by collared/uncollared shape, but both surface here.
var ErrPipeNameTaken = errors.New("pipe name taken")

// InsertPipe inserts a row. sourceID nil = uncollared (symmetric many-to-many
// pipe under the manifold); non-nil = collared under that source. The caller
// passes accountID explicitly because uncollared rows can't derive it from
// source.account_id. Retention overrides are NULL when the pointer arg is
// nil — the server provisions the JetStream stream with defaults.
func InsertPipe(ctx context.Context, p *Pool, accountID uuid.UUID, manifold string, sourceID *uuid.UUID, createdBy uuid.UUID, name string, ttl *int, maxMsgs *int, maxBytes *int64) (Pipe, error) {
	pipe := Pipe{
		ID:              uuid.New(),
		AccountID:       accountID,
		Manifold:        manifold,
		SourceID:        sourceID,
		CreatedByUserID: createdBy,
		Name:            name,
		TTLSeconds:      ttl,
		MaxMsgs:         maxMsgs,
		MaxBytes:        maxBytes,
		CreatedAt:       time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO pipes (id, account_id, manifold, source_id, created_by_user_id, name, ttl_seconds, max_msgs, max_bytes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		pipe.ID, pipe.AccountID, pipe.Manifold, pipe.SourceID, pipe.CreatedByUserID, pipe.Name, pipe.TTLSeconds, pipe.MaxMsgs, pipe.MaxBytes, pipe.CreatedAt)
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
		`SELECT id, account_id, manifold, source_id, created_by_user_id, name, ttl_seconds, max_msgs, max_bytes, created_at
		   FROM pipes WHERE source_id = $1 ORDER BY name ASC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pipe
	for rows.Next() {
		var pipe Pipe
		if err := rows.Scan(&pipe.ID, &pipe.AccountID, &pipe.Manifold, &pipe.SourceID, &pipe.CreatedByUserID, &pipe.Name,
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
		`SELECT id, account_id, manifold, source_id, created_by_user_id, name, ttl_seconds, max_msgs, max_bytes, created_at
		   FROM pipes WHERE source_id = $1 AND name = $2`, sourceID, name).
		Scan(&pipe.ID, &pipe.AccountID, &pipe.Manifold, &pipe.SourceID, &pipe.CreatedByUserID, &pipe.Name,
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

// DeleteUncollaredPipe removes an uncollared pipe row by (account, manifold,
// name). Stream cleanup is the caller's responsibility. Phase 1.5.
func DeleteUncollaredPipe(ctx context.Context, p *Pool, accountID uuid.UUID, manifold, name string) error {
	tag, err := p.Exec(ctx,
		`DELETE FROM pipes WHERE account_id = $1 AND manifold = $2 AND name = $3 AND source_id IS NULL`,
		accountID, manifold, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUncollaredPipesForAccount returns every uncollared pipe row in the
// account, sorted (manifold, name). Used by `ppz ls` to surface the
// sourceless rows that walking sources alone misses. Phase 1.5.
func ListUncollaredPipesForAccount(ctx context.Context, p *Pool, accountID uuid.UUID) ([]Pipe, error) {
	rows, err := p.Query(ctx,
		`SELECT id, account_id, manifold, source_id, created_by_user_id, name, ttl_seconds, max_msgs, max_bytes, created_at
		   FROM pipes WHERE account_id = $1 AND source_id IS NULL
		   ORDER BY manifold ASC, name ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pipe
	for rows.Next() {
		var pipe Pipe
		if err := rows.Scan(&pipe.ID, &pipe.AccountID, &pipe.Manifold, &pipe.SourceID, &pipe.CreatedByUserID, &pipe.Name,
			&pipe.TTLSeconds, &pipe.MaxMsgs, &pipe.MaxBytes, &pipe.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, pipe)
	}
	return out, rows.Err()
}
