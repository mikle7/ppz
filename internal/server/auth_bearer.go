package server

// Unified bearer-token middleware (Auth V2 Phase 2).
//
//   Authorization: Bearer ppz_live_<…>     → look up in api_keys
//   Authorization: Bearer ppz_oauth_<…>    → look up in oauth_tokens
//   anything else                          → 401
//
// Replaces the old requireAuth (which only handled api_keys). Both
// paths return a unified AuthedCaller via context so downstream
// handlers don't care which auth surface got us there.

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
)

const (
	// Existing v1 API keys are `ppz_<26 hex>` — not `ppz_live_…`.
	// Phase 2 keeps that shape verbatim (no migration) and uses a
	// dedicated `ppz_oauth_` prefix to distinguish the two paths.
	// Anything starting with `ppz_` other than `ppz_oauth_` is
	// treated as an API key.
	bearerPrefixAPIKey = "ppz_"
	bearerPrefixOAuth  = "ppz_oauth_"
)

// AuthedCaller is what requireBearer attaches to the request context.
// Exactly one of APIKey or TokenID is populated.
type AuthedCaller struct {
	UserID  uuid.UUID    // set on OAuth path; uuid.Nil on API-key path (V1 keys aren't user-scoped)
	APIKey  *db.APIKey   // populated when authed via api_keys
	TokenID *uuid.UUID   // populated when authed via oauth_tokens
}

type ctxKeyAuthedCallerType struct{}

var ctxKeyAuthedCaller ctxKeyAuthedCallerType

func CallerFromCtx(ctx context.Context) AuthedCaller {
	if v, ok := ctx.Value(ctxKeyAuthedCaller).(AuthedCaller); ok {
		return v
	}
	return AuthedCaller{}
}

// extractBearer pulls the token out of the Authorization header.
// Returns (plaintext, true) on a recognised-prefix token, ("", false)
// otherwise. The "recognised prefix" check is cheap and stops random
// scanner traffic from hitting the DB.
func extractBearer(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const scheme = "Bearer "
	if !strings.HasPrefix(header, scheme) {
		return "", false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(header, scheme))
	if tok == "" {
		return "", false
	}
	if !strings.HasPrefix(tok, bearerPrefixAPIKey) && !strings.HasPrefix(tok, bearerPrefixOAuth) {
		return "", false
	}
	return tok, true
}

// requireBearer is the unified middleware. Use this for any HTTP
// route that needs caller-identity but doesn't care which auth shape
// got us there (most /api/v1/* routes).
func (s *Server) requireBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := extractBearer(r)
		if !ok {
			writeErr(w, cliproto.New(cliproto.EInvalidAPIKey))
			return
		}
		caller, ok := s.resolveBearer(r.Context(), tok)
		if !ok {
			writeErr(w, cliproto.New(cliproto.EInvalidAPIKey))
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAuthedCaller, caller)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) resolveBearer(ctx context.Context, tok string) (AuthedCaller, bool) {
	// Order matters: `ppz_oauth_` is more specific than `ppz_`.
	switch {
	case strings.HasPrefix(tok, bearerPrefixOAuth):
		row, err := db.LookupBearerToken(ctx, s.Pool, tok)
		if err != nil {
			return AuthedCaller{}, false
		}
		return AuthedCaller{UserID: row.UserID, TokenID: &row.ID}, true

	case strings.HasPrefix(tok, bearerPrefixAPIKey):
		key, err := db.LookupAPIKey(ctx, s.Pool, tok)
		if err != nil {
			return AuthedCaller{}, false
		}
		return AuthedCaller{APIKey: &key}, true
	}
	return AuthedCaller{}, false
}

// requireAPIKey is the legacy authedHandler shape, kept for the
// existing org-scoped write surface (handlers_api.go) that takes a
// db.APIKey directly. Now backed by requireBearer.
//
// Org resolution for OAuth bearers (Phase 4 — multi-org):
//
//  1. If the request carries `?org=<uuid>`, validate the user is a
//     member or owner of that org and use it. Reject with 403 if
//     not — silent fallback would be a confused-deputy bug.
//  2. Otherwise fall back to FirstOwnedOrgFor (Phase 2 default — the
//     auto-org assigned to fresh GitHub signups).
//
// The daemon stamps `?org=<id>` on every API call once the user has
// switched orgs (`ppz org switch <name>`), so post-switch source
// create / broadcast / list all land in the chosen tenant.
func (s *Server) requireAPIKey(h authedHandler) http.HandlerFunc {
	return s.requireBearer(func(w http.ResponseWriter, r *http.Request) {
		caller := CallerFromCtx(r.Context())
		if caller.APIKey != nil {
			h(w, r, *caller.APIKey)
			return
		}
		// OAuth path. Stamp the caller's UserID on the synthetic APIKey
		// so downstream handlers (InsertSource, InsertPipe) attribute
		// the new row to the OAuth bearer's user — same field
		// downstream code reads on the API-key path.
		if raw := r.URL.Query().Get("org"); raw != "" {
			accountID, err := uuid.Parse(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "org is not a valid uuid"})
				return
			}
			if !db.IsMemberOrOwner(r.Context(), s.Pool, accountID, caller.UserID) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of org"})
				return
			}
			h(w, r, db.APIKey{AccountID: accountID, CreatedByUserID: caller.UserID})
			return
		}
		// Fallback: caller's default org (owned, else member). Used by
		// daemons that haven't sent an explicit ?org=. Must match the
		// /auth/exchange default so listing and the minted NATS creds agree.
		org, err := db.DefaultAccountFor(r.Context(), s.Pool, caller.UserID)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "user belongs to no org; create or join one on the GUI first",
			})
			return
		}
		h(w, r, db.APIKey{AccountID: org.ID, CreatedByUserID: caller.UserID})
	})
}
