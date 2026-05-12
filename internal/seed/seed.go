// Package seed provisions the deterministic test fixture: two accounts
// (alpha, beta) and three plaintext API keys (key-alpha, key-alpha2,
// key-beta). Plaintext keys + org IDs are written to <dir>/{key,org}-*.txt
// for the test runner to consume by file.
//
// Seed is idempotent: re-running on an already-seeded DB no-ops (orgs found
// by name, keys regenerated only if the org has none).
package seed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

func Run(ctx context.Context, pool *db.Pool, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir seed dir: %w", err)
	}

	// Seed test-fixture internal users. Their IDs land in
	// /seed/user-<name>.txt so e2e tests can pick them up the same
	// way they pick up alpha/beta org IDs. mode=internal — these
	// aren't OAuth identities, just deterministic targets for
	// member-management tests.
	for _, username := range []string{"foo", "bar"} {
		user, err := getOrCreateUser(ctx, pool, username)
		if err != nil {
			return fmt.Errorf("user %s: %w", username, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "user-"+username+".txt"), []byte(user.ID.String()), 0o644); err != nil {
			return fmt.Errorf("write user file: %w", err)
		}
	}

	for _, name := range []string{"alpha", "beta"} {
		org, err := getOrCreateOrg(ctx, pool, name)
		if err != nil {
			return fmt.Errorf("org %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "org-"+name+".txt"), []byte(org.ID.String()), 0o644); err != nil {
			return fmt.Errorf("write org file: %w", err)
		}
	}

	// Seed memberships: foo joins alpha, bar joins beta. Gives the
	// e2e suite stable preexisting members on the seeded orgs so
	// member-listing / removal tests don't have to add their own
	// boilerplate. AddMember is idempotent, so this re-runs cleanly.
	memberships := []struct{ org, user string }{
		{"alpha", "foo"},
		{"alpha", "bar"}, // bar is a non-owner member of alpha (used by auth e2e tests)
		{"beta", "bar"},
	}
	for _, m := range memberships {
		org, err := getOrCreateOrg(ctx, pool, m.org)
		if err != nil {
			return fmt.Errorf("seed-membership org %s: %w", m.org, err)
		}
		user, err := getOrCreateUser(ctx, pool, m.user)
		if err != nil {
			return fmt.Errorf("seed-membership user %s: %w", m.user, err)
		}
		if err := db.AddMember(ctx, pool, org.ID, user.ID); err != nil {
			return fmt.Errorf("seed-membership add %s→%s: %w", m.user, m.org, err)
		}
	}

	// Transfer alpha's ownership to foo so the auth e2e tests have
	// a clear owner/non-owner split: foo=owner, bar=member.
	// Idempotent: if alpha already owned-by-foo, this is a no-op.
	{
		org, err := getOrCreateOrg(ctx, pool, "alpha")
		if err != nil {
			return fmt.Errorf("seed-owner alpha: %w", err)
		}
		foo, err := getOrCreateUser(ctx, pool, "foo")
		if err != nil {
			return fmt.Errorf("seed-owner foo: %w", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE accounts SET owner_user_id = $1 WHERE id = $2`,
			foo.ID, org.ID); err != nil {
			return fmt.Errorf("seed-owner update alpha→foo: %w", err)
		}
	}

	// alpha gets two keys (alpha + alpha2), beta gets one (beta).
	// Each seed key is attributed to a real seeded user so `ppz ls`
	// HUMAN renders deterministic names in the e2e suite. foo owns
	// alpha; bar is a member of both alpha and beta.
	type want struct{ org, file, label, creator string }
	wants := []want{
		{"alpha", "key-alpha.txt", "alpha-primary", "foo"},
		{"alpha", "key-alpha2.txt", "alpha-secondary", "bar"},
		{"beta", "key-beta.txt", "beta-primary", "bar"},
	}
	for _, w := range wants {
		org, err := getOrCreateOrg(ctx, pool, w.org)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, w.file)
		if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 {
			// Already seeded — leave the plaintext file alone (the DB still
			// has the matching hash from a previous run).
			continue
		}
		creator, err := getOrCreateUser(ctx, pool, w.creator)
		if err != nil {
			return fmt.Errorf("seed-key creator %s: %w", w.creator, err)
		}
		_, plaintext, err := db.InsertAPIKey(ctx, pool, org.ID, creator.ID, w.label)
		if err != nil {
			return fmt.Errorf("insert key %s: %w", w.label, err)
		}
		if err := os.WriteFile(path, []byte(plaintext), 0o600); err != nil {
			return fmt.Errorf("write key file %s: %w", path, err)
		}
	}
	return nil
}

func getOrCreateOrg(ctx context.Context, pool *db.Pool, name string) (db.Account, error) {
	orgs, err := db.ListAccounts(ctx, pool)
	if err != nil {
		return db.Account{}, err
	}
	for _, o := range orgs {
		if o.Name == name {
			return o, nil
		}
	}
	// uuid.Nil tells InsertAccount to fall back to the seeded
	// "unauthenticated" user as owner — exactly what we want for
	// the alpha/beta test orgs.
	return db.InsertAccount(ctx, pool, name, uuid.Nil)
}

// getOrCreateUser is the user counterpart to getOrCreateOrg —
// returns the existing internal user or creates one with a synthetic
// `<name>@local` email. Idempotent on re-runs.
func getOrCreateUser(ctx context.Context, pool *db.Pool, username string) (db.User, error) {
	if u, err := db.GetUserByUsername(ctx, pool, username); err == nil {
		return u, nil
	}
	return db.InsertUser(ctx, pool, username, username+"@local", db.UserModeInternal)
}

// Sentinel for callers that want to detect "already done" — currently unused
// but useful when the seeder grows.
var ErrAlreadySeeded = errors.New("already seeded")
