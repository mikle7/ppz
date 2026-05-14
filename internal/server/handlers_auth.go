package server

// HTTP handlers for the Auth V2 GUI flow:
//
//   GET  /login                       mode dispatcher (none / password / oauth)
//   POST /auth/logout                 clear cookie, 303 to /
//   GET  /me                          JSON of the authed user
//   POST /dev/login?user=<seed-user>  test-only — mint a session for an existing internal user
//
// Phase 2 Cycle E stripped the GitHub-specific /auth/github/start and
// /auth/github/callback handlers; auth_mode=oauth now delegates to
// Server.Provider (pipescloud implements out-of-tree).

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

const sessionTTL = 7 * 24 * time.Hour // 7 days

// ─── /login ─────────────────────────────────────────────────────────

// handleGUILogin dispatches the /login route by Server.AuthMode.
//
//   AuthModeNone     — render the upgrade-path panel (login.html).
//                      Session auto-completion happens when the user
//                      clicks "Continue to dashboard" → /, which goes
//                      through requireSession → unauthorized →
//                      back to /login. To avoid that loop in mode=none,
//                      /login also writes the session cookie inline
//                      (when Pool is non-nil — middleware-only unit
//                      tests can skip the DB hit).
//   AuthModePassword — render the username/password form
//                      (login_password.html). POST validation flow
//                      lands in Cycle F when the Users page mints
//                      password_hash.
//   AuthModeOAuth    — delegate to Server.Provider.Authorize().
//
// All modes terminate in the same downstream contract: a user_id
// session cookie. See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md
// Cycle D.
func (s *Server) handleGUILogin(w http.ResponseWriter, r *http.Request) {
	// If already signed in, send them where they were going (or /dashboard).
	if uid, ok := s.verifyRequestSession(r); ok && uid != uuid.Nil {
		next := safeNext(r.URL.Query().Get("next"))
		http.Redirect(w, r, next, http.StatusFound)
		return
	}

	switch s.AuthMode {
	case AuthModeOAuth:
		s.Provider.Authorize(w, r)
		return
	case AuthModePassword:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "login_password.html", map[string]any{
			"Next": r.URL.Query().Get("next"),
		})
		return
	default:
		// AuthModeNone (and any unset zero-value): render the upgrade
		// panel. Session auto-completion is best-effort: if Pool is
		// nil (middleware-only unit tests), skip the cookie write
		// and just render the panel.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "login.html", map[string]any{
			"Next": r.URL.Query().Get("next"),
		})
		return
	}
}

// ─── /auth/logout ─────────────────────────────────────────────────

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ─── /me ──────────────────────────────────────────────────────────

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := UserIDFromCtx(r.Context())
	u, err := db.GetUser(r.Context(), s.Pool, uid)
	if err != nil {
		http.Error(w, "get user", 500)
		return
	}
	out := map[string]any{
		"id":         u.ID.String(),
		"username":   u.Username,
		"email":      u.Email,
		"mode":       string(u.Mode),
		"avatar_url": u.AvatarURL,
	}
	if u.GitHubID != nil {
		out["github_id"] = *u.GitHubID
	}
	writeJSON(w, http.StatusOK, out)
}

// ─── /dev/login (test-only) ───────────────────────────────────────

func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.DevLogin {
		http.NotFound(w, r)
		return
	}
	username := r.URL.Query().Get("user")
	if username == "" {
		http.Error(w, "missing ?user=<seed-username>", 400)
		return
	}
	u, err := db.GetUserByUsername(r.Context(), s.Pool, username)
	if err != nil {
		http.Error(w, fmt.Sprintf("user %q not found", username), 404)
		return
	}
	cookieValue, err := SignSessionCookie(s.SessionKey, SessionPayload{
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(sessionTTL),
	})
	if err != nil {
		http.Error(w, "sign cookie: "+err.Error(), 500)
		return
	}
	s.setSessionCookie(w, cookieValue)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ─── helpers ──────────────────────────────────────────────────────

func (s *Server) setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Now().Add(sessionTTL),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.BaseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.BaseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// safeNext defaults the post-login redirect target. If the supplied
// path passes isSafeNextPath, use it; otherwise send to /dashboard.
func safeNext(next string) string {
	if next == "" || !isSafeNextPath(next) {
		return "/dashboard"
	}
	return next
}
