// Package db owns the postgres connection pool, migrations, and per-table
// repositories.
package db

import (
	"context"
	"embed"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the public handle other packages use. It wraps pgxpool.Pool so the
// rest of the codebase doesn't import pgx directly.
type Pool struct {
	*pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Pool{Pool: pool}, nil
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs every migration file in order, lexicographically by name.
// Each file uses IF NOT EXISTS / IF NOT EXISTS COLUMN clauses so re-running
// is idempotent.
func Migrate(ctx context.Context, p *Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		data, err := migrationsFS.ReadFile("migrations/" + n)
		if err != nil {
			return fmt.Errorf("read %s: %w", n, err)
		}
		if _, err := p.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", n, err)
		}
	}
	return nil
}
