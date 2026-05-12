package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// InviteStatus values mirror the CHECK constraint on invites.status.
type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusDeclined InviteStatus = "declined"
	InviteStatusRevoked  InviteStatus = "revoked"
)

// ErrInviteNotPending is returned when a transition (accept/decline/
// revoke) targets an invite that is no longer pending. Caller surfaces
// it as 409.
var ErrInviteNotPending = errors.New("invite is not pending")

// ErrDuplicatePendingInvite is returned by CreateInvite when an
// active pending invite for the same (org, username) already exists.
// The unique partial index enforces this server-side; we surface a
// typed sentinel so handlers can return a clean 409.
var ErrDuplicatePendingInvite = errors.New("a pending invite for this user already exists")

// ErrAlreadyMember is returned by CreateInvite when the invitee is
// already a member or the owner of the org. Same intent as the
// duplicate-pending case but a different cause — surface separately.
var ErrAlreadyMember = errors.New("user is already a member of this account")

type Invite struct {
	ID              uuid.UUID
	AccountID  uuid.UUID
	InviteeUsername string
	InviterUserID   uuid.UUID
	Status          InviteStatus
	CreatedAt       time.Time
	DecidedAt       *time.Time
}

// InviteWithAccount is the dashboard projection — invite plus org name so
// the user can see where the invite came from without a second query.
type InviteWithAccount struct {
	Invite
	AccountName string
}

// CreateInvite inserts a pending invite for inviteeUsername into accountID,
// recording inviterUserID as the sender. Pre-flights two error
// conditions before the insert:
//
//   - inviteeUsername is already a member or the owner of accountID
//     → ErrAlreadyMember
//   - a pending invite for the same (org, username) already exists
//     → ErrDuplicatePendingInvite
//
// Pre-flighting the duplicate case in addition to relying on the
// partial unique index gives us a typed error without inspecting pg
// error codes.
func CreateInvite(ctx context.Context, p *Pool, accountID uuid.UUID, inviteeUsername string, inviterUserID uuid.UUID) (Invite, error) {
	// Already a member or the owner?
	var n int
	err := p.QueryRow(ctx, `
		SELECT 1
		  FROM accounts o
		  JOIN users u ON u.id = o.owner_user_id
		 WHERE o.id = $1 AND u.username = $2
		UNION ALL
		SELECT 1
		  FROM account_members m
		  JOIN users u ON u.id = m.user_id
		 WHERE m.account_id = $1 AND u.username = $2
		 LIMIT 1`, accountID, inviteeUsername).Scan(&n)
	if err == nil {
		return Invite{}, ErrAlreadyMember
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, err
	}

	// Existing pending invite?
	err = p.QueryRow(ctx, `
		SELECT 1 FROM invites
		 WHERE account_id = $1 AND invitee_username = $2 AND status = 'pending'
		 LIMIT 1`, accountID, inviteeUsername).Scan(&n)
	if err == nil {
		return Invite{}, ErrDuplicatePendingInvite
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, err
	}

	inv := Invite{
		ID:              uuid.New(),
		AccountID:  accountID,
		InviteeUsername: inviteeUsername,
		InviterUserID:   inviterUserID,
		Status:          InviteStatusPending,
		CreatedAt:       time.Now().UTC(),
	}
	if _, err := p.Exec(ctx,
		`INSERT INTO invites (id, account_id, invitee_username, inviter_user_id, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		inv.ID, inv.AccountID, inv.InviteeUsername, inv.InviterUserID, string(inv.Status), inv.CreatedAt); err != nil {
		return Invite{}, err
	}
	return inv, nil
}

// GetInvite fetches a single invite by id. Returns ErrNotFound if the
// row doesn't exist.
func GetInvite(ctx context.Context, p *Pool, id uuid.UUID) (Invite, error) {
	var inv Invite
	var status string
	err := p.QueryRow(ctx,
		`SELECT id, account_id, invitee_username, inviter_user_id, status, created_at, decided_at
		   FROM invites WHERE id = $1`, id).
		Scan(&inv.ID, &inv.AccountID, &inv.InviteeUsername, &inv.InviterUserID, &status, &inv.CreatedAt, &inv.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	inv.Status = InviteStatus(status)
	return inv, nil
}

// ListPendingInvitesForUsername returns all pending invites whose
// invitee_username matches. Joined with accounts so callers can
// show the org name on the dashboard.
func ListPendingInvitesForUsername(ctx context.Context, p *Pool, username string) ([]InviteWithAccount, error) {
	rows, err := p.Query(ctx, `
		SELECT i.id, i.account_id, i.invitee_username, i.inviter_user_id,
		       i.status, i.created_at, i.decided_at, o.name
		  FROM invites i
		  JOIN accounts o ON o.id = i.account_id
		 WHERE i.invitee_username = $1 AND i.status = 'pending'
		 ORDER BY i.created_at ASC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InviteWithAccount
	for rows.Next() {
		var iw InviteWithAccount
		var status string
		if err := rows.Scan(&iw.ID, &iw.AccountID, &iw.InviteeUsername, &iw.InviterUserID,
			&status, &iw.CreatedAt, &iw.DecidedAt, &iw.AccountName); err != nil {
			return nil, err
		}
		iw.Status = InviteStatus(status)
		out = append(out, iw)
	}
	return out, rows.Err()
}

// ListInvitesForOrg returns every invite (any status) for accountID,
// newest first. Used by the org page to show the invite history.
func ListInvitesForOrg(ctx context.Context, p *Pool, accountID uuid.UUID) ([]Invite, error) {
	rows, err := p.Query(ctx, `
		SELECT id, account_id, invitee_username, inviter_user_id, status, created_at, decided_at
		  FROM invites
		 WHERE account_id = $1
		 ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var inv Invite
		var status string
		if err := rows.Scan(&inv.ID, &inv.AccountID, &inv.InviteeUsername, &inv.InviterUserID,
			&status, &inv.CreatedAt, &inv.DecidedAt); err != nil {
			return nil, err
		}
		inv.Status = InviteStatus(status)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// AcceptInvite transitions a pending invite to accepted and adds the
// accepting user as a non-owner member of the org, atomically. The
// caller passes the accepting user's id; the function checks that
// their username matches the invite's invitee_username (so a logged-in
// user can't accept someone else's invite even if they know the id).
//
// Returns:
//   - ErrNotFound          if the invite id doesn't exist
//   - ErrInviteNotPending  if the invite is no longer pending
//   - errors.New("forbidden") if userID's username != invitee_username
func AcceptInvite(ctx context.Context, p *Pool, inviteID, acceptingUserID uuid.UUID) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inv Invite
	var status string
	err = tx.QueryRow(ctx,
		`SELECT id, account_id, invitee_username, inviter_user_id, status, created_at
		   FROM invites WHERE id = $1 FOR UPDATE`, inviteID).
		Scan(&inv.ID, &inv.AccountID, &inv.InviteeUsername, &inv.InviterUserID, &status, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if InviteStatus(status) != InviteStatusPending {
		return ErrInviteNotPending
	}

	var acceptingUsername string
	if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, acceptingUserID).Scan(&acceptingUsername); err != nil {
		return err
	}
	if acceptingUsername != inv.InviteeUsername {
		return errors.New("forbidden")
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`UPDATE invites SET status = 'accepted', decided_at = $2 WHERE id = $1`,
		inviteID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO account_members (account_id, user_id, added_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (account_id, user_id) DO NOTHING`,
		inv.AccountID, acceptingUserID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeclineInvite transitions a pending invite to declined. Same
// permission check as Accept: caller's username must match the
// invite's invitee_username.
func DeclineInvite(ctx context.Context, p *Pool, inviteID, decliningUserID uuid.UUID) error {
	inv, err := GetInvite(ctx, p, inviteID)
	if err != nil {
		return err
	}
	if inv.Status != InviteStatusPending {
		return ErrInviteNotPending
	}

	var username string
	if err := p.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, decliningUserID).Scan(&username); err != nil {
		return err
	}
	if username != inv.InviteeUsername {
		return errors.New("forbidden")
	}

	tag, err := p.Exec(ctx,
		`UPDATE invites SET status = 'declined', decided_at = $2 WHERE id = $1 AND status = 'pending'`,
		inviteID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInviteNotPending
	}
	return nil
}

// RevokeInvite transitions a pending invite to revoked. Owner-only:
// caller is expected to be the org's owner; this function does not
// re-check the role (handlers gate via owner-only middleware), but it
// does require the invite be in the pending state.
func RevokeInvite(ctx context.Context, p *Pool, inviteID uuid.UUID) error {
	inv, err := GetInvite(ctx, p, inviteID)
	if err != nil {
		return err
	}
	if inv.Status != InviteStatusPending {
		return ErrInviteNotPending
	}
	tag, err := p.Exec(ctx,
		`UPDATE invites SET status = 'revoked', decided_at = $2 WHERE id = $1 AND status = 'pending'`,
		inviteID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInviteNotPending
	}
	return nil
}
