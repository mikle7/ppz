package server

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
)

// org-management API — Phase 4 (multi-org support).
//
// Both endpoints require an OAuth bearer (user identity); API keys
// are org-scoped and have no user identity, so they get 403 here.
//
//   GET  /api/v1/orgs   — list orgs the caller is in (owner or member)
//   POST /api/v1/orgs   — create a new org with caller as owner
//
// The CLI wraps these in `ppz org list` / `ppz org create`.

func (s *Server) handleAPIListOrgs(w http.ResponseWriter, r *http.Request) {
	caller := CallerFromCtx(r.Context())
	if caller.UserID == uuid.Nil {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "this endpoint requires an OAuth token (user identity)",
		})
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	orgs, err := db.ListAccountsForUser(ctx, s.Pool, caller.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]cliproto.OrgInfo, 0, len(orgs))
	for _, o := range orgs {
		role := "member"
		if o.OwnerUserID == caller.UserID {
			role = "owner"
		}
		out = append(out, cliproto.OrgInfo{ID: o.ID.String(), Name: o.Name, Role: role})
	}
	writeJSON(w, http.StatusOK, cliproto.ListOrgsReply{Orgs: out})
}

func (s *Server) handleAPICreateOrg(w http.ResponseWriter, r *http.Request) {
	caller := CallerFromCtx(r.Context())
	if caller.UserID == uuid.Nil {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "this endpoint requires an OAuth token (user identity)",
		})
		return
	}
	var req cliproto.CreateOrgRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if !validOrgSlug(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be a slug: 1-40 chars, [a-z0-9-]"})
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	if _, err := db.GetAccountByName(ctx, s.Pool, name); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "name already taken"})
		return
	}
	org, err := db.InsertAccount(ctx, s.Pool, name, caller.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, cliproto.CreateOrgReply{ID: org.ID.String(), Name: org.Name})
}

// validOrgSlug enforces a conservative slug shape: 1-40 chars,
// lowercase alphanumeric and dashes only, no leading/trailing dash.
// Mirrors what the GUI org-create form would accept.
func validOrgSlug(s string) bool {
	if len(s) == 0 || len(s) > 40 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}
