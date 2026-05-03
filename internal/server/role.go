package server

// Role check helpers — used by the owner-only gates on destructive
// org operations (revoke key, remove member, transfer ownership).

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OrgRole is the role the user has in an org. "" = not a member.
type OrgRole string

const (
	OrgRoleOwner  OrgRole = "owner"
	OrgRoleMember OrgRole = "member"
	OrgRoleNone   OrgRole = ""
)

// RoleInOrg returns the calling user's role in the given org.
//
//   - OrgRoleOwner  if users.id == organisations.owner_user_id
//   - OrgRoleMember if listed in organisation_members (and not owner)
//   - OrgRoleNone   otherwise
func (s *Server) RoleInOrg(ctx context.Context, userID, orgID uuid.UUID) (OrgRole, error) {
	if userID == uuid.Nil || orgID == uuid.Nil {
		return OrgRoleNone, nil
	}
	var ownerID uuid.UUID
	err := s.Pool.QueryRow(ctx,
		`SELECT owner_user_id FROM organisations WHERE id = $1`, orgID).
		Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgRoleNone, nil
	}
	if err != nil {
		return OrgRoleNone, err
	}
	if ownerID == userID {
		return OrgRoleOwner, nil
	}
	var ignored uuid.UUID
	err = s.Pool.QueryRow(ctx,
		`SELECT user_id FROM organisation_members
		  WHERE organisation_id = $1 AND user_id = $2`, orgID, userID).
		Scan(&ignored)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgRoleNone, nil
	}
	if err != nil {
		return OrgRoleNone, err
	}
	return OrgRoleMember, nil
}
