package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Organisation struct {
	ID          uuid.UUID
	Name        string
	OwnerUserID uuid.UUID
	CreatedAt   time.Time

	// Auth V2 §Phase 3.5 — per-org NATS account. NULL until the org's
	// account is provisioned (lazy on first /auth/exchange).
	NATSAccountPub         string
	NATSAccountJWT         string
	NATSAccountSigningSeed string
}

// IsMemberOrOwner returns true if userID owns orgID or is a member.
// Used by /auth/exchange (Phase 3.5) to validate that a multi-org
// user is actually entitled to the org they're requesting a JWT for.
func IsMemberOrOwner(ctx context.Context, p *Pool, orgID, userID uuid.UUID) bool {
	var n int
	err := p.QueryRow(ctx, `
		SELECT 1 FROM organisations WHERE id = $1 AND owner_user_id = $2
		UNION ALL
		SELECT 1 FROM organisation_members WHERE organisation_id = $1 AND user_id = $2
		LIMIT 1`, orgID, userID).Scan(&n)
	return err == nil
}

// SetNATSAccount persists the Operator-signed Account JWT + the
// account's signing seed for an org. Called once (lazily) on first
// /auth/exchange after Phase 3.5 — subsequent calls find the row
// already populated and skip.
func SetNATSAccount(ctx context.Context, p *Pool, orgID uuid.UUID, accountPub, accountJWT, signingSeed string) error {
	_, err := p.Exec(ctx,
		`UPDATE organisations
		    SET nats_account_pub = $2,
		        nats_account_jwt = $3,
		        nats_account_signing_seed = $4
		  WHERE id = $1`,
		orgID, accountPub, accountJWT, signingSeed)
	return err
}

// InsertOrganisation creates a new org owned by ownerUserID. If
// ownerUserID is uuid.Nil, the org defaults to the seeded
// unauthenticated user — preserves back-compat for tests + GUI
// callers that don't supply an owner yet.
func InsertOrganisation(ctx context.Context, p *Pool, name string, ownerUserID uuid.UUID) (Organisation, error) {
	if ownerUserID == uuid.Nil {
		var fallback uuid.UUID
		if err := p.QueryRow(ctx,
			`SELECT id FROM users WHERE username = 'unauthenticated'`).Scan(&fallback); err != nil {
			return Organisation{}, errors.New("no owner_user_id and no unauthenticated user to fall back on: " + err.Error())
		}
		ownerUserID = fallback
	}
	o := Organisation{
		ID:          uuid.New(),
		Name:        name,
		OwnerUserID: ownerUserID,
		CreatedAt:   time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO organisations (id, name, owner_user_id, created_at) VALUES ($1, $2, $3, $4)`,
		o.ID, o.Name, o.OwnerUserID, o.CreatedAt)
	return o, err
}

func ListOrganisations(ctx context.Context, p *Pool) ([]Organisation, error) {
	rows, err := p.Query(ctx, `SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM organisations ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Organisation
	for rows.Next() {
		var o Organisation
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt, &o.NATSAccountPub, &o.NATSAccountJWT, &o.NATSAccountSigningSeed); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListOrganisationsForUser returns the orgs userID owns or is a member
// of, ordered by name. Used by the GUI dashboard so each user sees only
// their own tenants instead of every org in the system.
func ListOrganisationsForUser(ctx context.Context, p *Pool, userID uuid.UUID) ([]Organisation, error) {
	rows, err := p.Query(ctx, `
		SELECT DISTINCT o.id, o.name, o.owner_user_id, o.created_at,
		                COALESCE(o.nats_account_pub,''),
		                COALESCE(o.nats_account_jwt,''),
		                COALESCE(o.nats_account_signing_seed,'')
		  FROM organisations o
		 WHERE o.owner_user_id = $1
		    OR EXISTS (
		        SELECT 1 FROM organisation_members m
		         WHERE m.organisation_id = o.id AND m.user_id = $1
		    )
		 ORDER BY o.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Organisation
	for rows.Next() {
		var o Organisation
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt, &o.NATSAccountPub, &o.NATSAccountJWT, &o.NATSAccountSigningSeed); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func GetOrganisation(ctx context.Context, p *Pool, id uuid.UUID) (Organisation, error) {
	var o Organisation
	err := p.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM organisations WHERE id = $1`, id).
		Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt, &o.NATSAccountPub, &o.NATSAccountJWT, &o.NATSAccountSigningSeed)
	return o, err
}

// GetOrganisationByName looks up an org by its unique name (used as a slug
// alias in the GUI: /orgs/alpha resolves the same as /orgs/<uuid>).
func GetOrganisationByName(ctx context.Context, p *Pool, name string) (Organisation, error) {
	var o Organisation
	err := p.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at, COALESCE(nats_account_pub,''), COALESCE(nats_account_jwt,''), COALESCE(nats_account_signing_seed,'') FROM organisations WHERE name = $1`, name).
		Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt, &o.NATSAccountPub, &o.NATSAccountJWT, &o.NATSAccountSigningSeed)
	return o, err
}

// FirstOwnedOrgFor returns the org owned by userID. If they own
// multiple, returns the oldest. If they own none, returns ErrNotFound.
// Used by the OAuth path of requireAPIKey to pick a default org for
// callers who haven't yet specified one (Auth V2 Phase 2 interim;
// proper org-selection UX is V3).
func FirstOwnedOrgFor(ctx context.Context, p *Pool, userID uuid.UUID) (Organisation, error) {
	var o Organisation
	err := p.QueryRow(ctx,
		`SELECT id, name, owner_user_id, created_at,
		        COALESCE(nats_account_pub,''),
		        COALESCE(nats_account_jwt,''),
		        COALESCE(nats_account_signing_seed,'')
		   FROM organisations
		  WHERE owner_user_id = $1
		  ORDER BY created_at ASC
		  LIMIT 1`, userID).
		Scan(&o.ID, &o.Name, &o.OwnerUserID, &o.CreatedAt, &o.NATSAccountPub, &o.NATSAccountJWT, &o.NATSAccountSigningSeed)
	if err != nil {
		return Organisation{}, ErrNotFound
	}
	return o, nil
}