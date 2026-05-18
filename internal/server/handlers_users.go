package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

// handleCreateUser: POST /users (form: username, email, mode, password?).
// Phase 2 Cycle F: when password is provided, the user is created
// with users.password_hash set (bcrypt). Omitting password leaves
// password_hash NULL — the user can't sign in via auth_mode=password
// until a hash is set.
//
// Redirects to the Referer on success.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	mode := db.UserMode(strings.TrimSpace(r.FormValue("mode")))
	password := r.FormValue("password")
	if username == "" || email == "" {
		http.Error(w, "username and email required", http.StatusBadRequest)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	user, err := db.InsertUser(ctx, s.Pool, username, email, mode)
	if err != nil {
		if errors.Is(err, db.ErrInvalidUserMode) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if password != "" {
		hash, err := db.HashPassword(password)
		if err != nil {
			http.Error(w, "could not store password", http.StatusInternalServerError)
			return
		}
		if err := db.SetUserPasswordHash(ctx, s.Pool, user.ID, hash); err != nil {
			http.Error(w, "could not store password", http.StatusInternalServerError)
			return
		}
	}
	browserSubmit(w, r)
}

// handleAddMember: POST /accounts/{id}/members (form: user_id) — adds the
// user as a non-owner member of the account. Idempotent: re-adding an
// existing member is a no-op + still 303.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(r.FormValue("user_id")))
	if err != nil {
		http.Error(w, "user_id is not a valid uuid", http.StatusBadRequest)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}
	if err := db.AddMember(ctx, s.Pool, org.ID, userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	browserSubmit(w, r)
}

// handleRemoveMember: POST /accounts/{id}/members/{uid}/remove — removes
// the user from the account. Returns 409 when the target is the
// account's owner (transfer-ownership is a v2 feature).
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("uid"))
	if err != nil {
		http.Error(w, "user_id is not a valid uuid", http.StatusBadRequest)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}
	if err := db.RemoveMember(ctx, s.Pool, org.ID, userID); err != nil {
		if errors.Is(err, db.ErrCannotRemoveOwner) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "member not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	browserSubmit(w, r)
}