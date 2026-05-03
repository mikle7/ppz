package server

// requireSession middleware — gates GUI routes (/dashboard, /orgs/*, /me).
// On a valid session cookie, attaches the user_id to the request
// context and chains to the next handler. On miss/invalid/expired:
//
//   - Browser request (Accept does NOT include "application/json")
//     → 302 to /login?next=<original-path-and-query>
//   - API client (Accept includes "application/json")
//     → 401 with the standard error envelope
//
// The cookie is named SessionCookieName ("ppz_session"), set HttpOnly
// + Secure + SameSite=Lax by the OAuth callback.

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

const SessionCookieName = "ppz_session"

type ctxKeySessionUserType struct{}

var ctxKeySessionUser ctxKeySessionUserType

// UserIDFromCtx returns the authenticated user_id, or uuid.Nil if the
// request was not gated by requireSession.
func UserIDFromCtx(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(ctxKeySessionUser).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := s.verifyRequestSession(r)
		if !ok {
			s.unauthorized(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeySessionUser, uid)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) verifyRequestSession(r *http.Request) (uuid.UUID, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c == nil || c.Value == "" {
		return uuid.Nil, false
	}
	p, err := VerifySessionCookie(s.SessionKey, c.Value)
	if err != nil {
		return uuid.Nil, false
	}
	// Cookie HMAC is valid, but the user_id may have been deleted
	// (e.g. dev rig torn down + recreated, browser still holds the
	// stale cookie). Verify the user exists; treat missing as
	// unauthenticated so the user re-logs in and we don't trip FK
	// constraints downstream.
	//
	// Pool is nil only in middleware-only unit tests that stub a
	// Server without DB; production servers always have a Pool.
	if s.Pool != nil {
		if _, err := db.GetUser(r.Context(), s.Pool, p.UserID); err != nil {
			return uuid.Nil, false
		}
	}
	return p.UserID, true
}

// unauthorized routes anonymous requests to either the login page (GUI)
// or a 401 envelope (API), depending on Accept. Heuristic: anything
// that explicitly accepts JSON gets a 401; everything else, including
// browsers, gets a redirect.
func (s *Server) unauthorized(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
		return
	}
	next := r.URL.RequestURI()
	loginURL := "/login?next=" + url.QueryEscape(next)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func wantsJSON(r *http.Request) bool {
	a := r.Header.Get("Accept")
	return strings.Contains(a, "application/json")
}
