package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Account is the ppz-side tenancy boundary. Maps 1:1 to a NATS account
// (see SetNATSAccount below for the credential plumbing). Pre-launch this
// was called "Organisation"; the rename is part of the Phase 1 surface
// strip (see strategy doc OSS-PIPESCLOUD-ARCHITECTURE-SPLIT, private,
// locked decisions #11 and #18).
type Account struct {
	ID          uuid.UUID
	Name        string
	OwnerUserID uuid.UUID
	CreatedAt   time.Time

	// Auth V2 §Phase 3.5 — per-account NATS credential. NULL until the
	// account's NATS credential is provisioned (lazy on first /auth/exchange).
	NATSAccountPub         string
	NATSAccountJWT         string
	NATSAccountSigningSeed string
}

// IsMemberOrOwner returns true if userID owns accountID or is a member.
// Used by /auth/exchange (Phase 3.5) to validate that a multi-account
// user is actually entitled to the account they're requesting a JWT for.
func IsMemberOrOwner(ctx context.Context, p *Pool, accountID, userID uuid.UUID) bool {
	var n int
	err := p.QueryRow(ctx, `
		SELECT 1 FROM accounts WHERE id = $1 AND owner_user_id = $2
		UNION ALL
		SELECT 1 FROM account_members WHERE account_id = $1 AND user_id = $2
		LIMIT 1`, accountID, userID).Scan(&n)
	return err == nil
}

// SetNATSAccount persists the Operator-signed Account JWT + the
// account's signing seed for an account row. Called once (lazily) on
// first /auth/exchange after Phase 3.5 — subsequent calls find the row
// already populated and skip.
func SetNATSAccount(ctx context.Context, p *Pool, accountID uuid.UUID, accountPub, accountJWT, signingSeed string) error {
	_, err := p.Exec(ctx,
		`UPDATE accounts
		    SET nats_account_pub = $2,
		        nats_account_jwt = $3,
		        nats_account_signing_seed = $4
		  WHERE id = $1`,
		accountID, accountPub, accountJWT, signingSeed)
	return err
}

// InsertAccount creates a new account owned by ownerUserID. If
// ownerUserID is uuid.Nil, the account defaults to the seeded
// unauthenticated user — preserves back-compat for tests + GUI
// callers that don't supply an owner yet.
func InsertAccount(ctx context.Context, p *Pool, name string, ownerUserID uuid.UUID) (Account, error) {
	if ownerUserID == uuid.Nil {
		var fallback uuid.UUID
		if err := p.QueryRow(ctx,
			`SELECT id FROM users WHERE username = 'unauthenticated'`).Scan(&fallback); err != nil {
			return Account{}, errors.New("no owner_user_id and no unauthenticated user to fall back on: " + err.Error())
		}
		ownerUserID = fallback
	}
	a := Account{
		ID:          uuid.New(),
		Name:        name,
		OwnerUserID: ownerUserID,
		CreatedAt:   time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO accounts (id, name, owner_user_id, created_at) VALUES ($1, $2, $3, $4)`,
		a.ID, a.Name, a.OwnerUserID, a.CreatedAt)
	return a, err
}

func ListAccounts(ctx context.Context, p *Pool) ([]Account, error) {
	rows, err := p.Query(ctx, `SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM accounts ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.OwnerUserID, &a.CreatedAt, &a.NATSAccountPub, &a.NATSAccountJWT, &a.NATSAccountSigningSeed); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAccountsForUser returns the accounts userID owns or is a member
// of, ordered by name. Used by the GUI dashboard so each user sees only
// their own tenants instead of every account in the system.
func ListAccountsForUser(ctx context.Context, p *Pool, userID uuid.UUID) ([]Account, error) {
	rows, err := p.Query(ctx, `
		SELECT DISTINCT a.id, a.name, a.owner_user_id, a.created_at,
		                COALESCE(a.nats_account_pub,''),
		                COALESCE(a.nats_account_jwt,''),
		                COALESCE(a.nats_account_signing_seed,'')
		  FROM accounts a
		 WHERE a.owner_user_id = $1
		    OR EXISTS (
		        SELECT 1 FROM account_members m
		         WHERE m.account_id = a.id AND m.user_id = $1
		    )
		 ORDER BY a.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.OwnerUserID, &a.CreatedAt, &a.NATSAccountPub, &a.NATSAccountJWT, &a.NATSAccountSigningSeed); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func GetAccount(ctx context.Context, p *Pool, id uuid.UUID) (Account, error) {
	var a Account
	err := p.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM accounts WHERE id = $1`, id).
		Scan(&a.ID, &a.Name, &a.OwnerUserID, &a.CreatedAt, &a.NATSAccountPub, &a.NATSAccountJWT, &a.NATSAccountSigningSeed)
	return a, err
}

// GetAccountByName looks up an account by its unique name (used as a slug
// alias in the GUI: /accounts/alpha resolves the same as /accounts/<uuid>).
func GetAccountByName(ctx context.Context, p *Pool, name string) (Account, error) {
	var a Account
	err := p.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM accounts WHERE name = $1`, name).
		Scan(&a.ID, &a.Name, &a.OwnerUserID, &a.CreatedAt, &a.NATSAccountPub, &a.NATSAccountJWT, &a.NATSAccountSigningSeed)
	return a, err
}

// DefaultAccountFor returns the account to use for a caller who hasn't
// explicitly selected one: their oldest OWNED account, or — if they own
// none — their oldest account by MEMBERSHIP. Returns ErrNotFound only
// when the user neither owns nor belongs to any account.
//
// This is the account-selection default for the OAuth path (device flow
// + /auth/exchange) and the requireAPIKey ?org fallback. It must consider
// membership: a user invited into an org (a member, not owner) otherwise
// has no usable default and every OAuth call 403s / errors — and listing
// (?org=, membership-aware) would disagree with the minted NATS creds.
func DefaultAccountFor(ctx context.Context, p *Pool, userID uuid.UUID) (Account, error) {
	var a Account
	err := p.QueryRow(ctx,
		`SELECT a.id, a.name, a.owner_user_id, a.created_at,
		        COALESCE(a.nats_account_pub,''),
		        COALESCE(a.nats_account_jwt,''),
		        COALESCE(a.nats_account_signing_seed,'')
		   FROM accounts a
		  WHERE a.owner_user_id = $1
		     OR EXISTS (
		         SELECT 1 FROM account_members m
		          WHERE m.account_id = a.id AND m.user_id = $1
		     )
		  ORDER BY (a.owner_user_id = $1) DESC, a.created_at ASC
		  LIMIT 1`, userID).
		Scan(&a.ID, &a.Name, &a.OwnerUserID, &a.CreatedAt, &a.NATSAccountPub, &a.NATSAccountJWT, &a.NATSAccountSigningSeed)
	if err != nil {
		return Account{}, ErrNotFound
	}
	return a, nil
}
