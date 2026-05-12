package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrCannotRemoveOwner is returned by RemoveMember when the caller
// tries to remove the org's owner. v1 has no transfer-ownership
// path; the owner has to stay on the org until v2 lands transfer.
var ErrCannotRemoveOwner = errors.New("cannot remove the account's owner")

type Member struct {
	AccountID uuid.UUID
	UserID         uuid.UUID
	AddedAt        time.Time
}

// AddMember records a user as a non-owner member of an org. Idempotent
// — re-adding an existing member is a no-op (UPSERT-style).
func AddMember(ctx context.Context, p *Pool, accountID, userID uuid.UUID) error {
	_, err := p.Exec(ctx,
		`INSERT INTO account_members (account_id, user_id, added_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (account_id, user_id) DO NOTHING`,
		accountID, userID, time.Now().UTC())
	return err
}

// RemoveMember drops a non-owner from the org. Returns
// ErrCannotRemoveOwner when targetUserID matches the org's owner —
// caller surfaces it as 409. ErrNotFound when the user wasn't a
// member.
func RemoveMember(ctx context.Context, p *Pool, accountID, userID uuid.UUID) error {
	var ownerID uuid.UUID
	if err := p.QueryRow(ctx,
		`SELECT owner_user_id FROM accounts WHERE id = $1`, accountID).Scan(&ownerID); err != nil {
		return ErrNotFound
	}
	if ownerID == userID {
		return ErrCannotRemoveOwner
	}
	tag, err := p.Exec(ctx,
		`DELETE FROM account_members WHERE account_id = $1 AND user_id = $2`,
		accountID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMembers returns the non-owner members of an org, ordered by
// when they were added.
func ListMembers(ctx context.Context, p *Pool, accountID uuid.UUID) ([]User, error) {
	rows, err := p.Query(ctx,
		`SELECT u.id, u.username, u.email, u.mode, u.created_at
		   FROM account_members m
		   JOIN users u ON u.id = m.user_id
		  WHERE m.account_id = $1
		  ORDER BY m.added_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var mode string
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &mode, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Mode = UserMode(mode)
		out = append(out, u)
	}
	return out, rows.Err()
}