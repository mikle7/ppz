package server

// Handlers for the OAuth 2.0 Device Authorization Grant (RFC 8628)
// flow used by the ppz CLI. See docs/AUTH-V2.md § Phase 2.
//
//   POST /oauth/device/code        CLI start  → device + user codes
//   GET  /oauth/device/verify      browser; signed-in user pastes user_code
//   POST /oauth/device/verify      browser; user clicks Approve
//   POST /oauth/device/token       CLI poll; once approved, returns bearer

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

// orgOption is one entry in the verify-page org dropdown.
type orgOption struct {
	ID       string
	Name     string
	Selected bool
}

// resolveDefaultOrg returns the account_id the verify-page dropdown should
// pre-select for userID: the org they last authorized into (if still a
// member), else their default org (owned, else member). Empty string if
// the user belongs to no org.
func (s *Server) resolveDefaultOrg(r *http.Request, userID uuid.UUID, accounts []db.Account) string {
	if last, err := db.GetLastSelectedAccount(r.Context(), s.Pool, userID); err == nil && last != nil {
		for _, a := range accounts {
			if a.ID == *last {
				return last.String()
			}
		}
	}
	if def, err := db.DefaultAccountFor(r.Context(), s.Pool, userID); err == nil {
		return def.ID.String()
	}
	return ""
}

// resolveSelectedOrg validates the account_id the user submitted on the
// verify page (must be one they own or belong to). An empty raw value
// falls back to the user's default org. Returns an error the caller can
// surface as 403.
func (s *Server) resolveSelectedOrg(r *http.Request, userID uuid.UUID, raw string) (uuid.UUID, error) {
	if raw == "" {
		def, err := db.DefaultAccountFor(r.Context(), s.Pool, userID)
		if err != nil {
			return uuid.Nil, errors.New("you do not belong to any org")
		}
		return def.ID, nil
	}
	accountID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("org is not a valid id")
	}
	if !db.IsMemberOrOwner(r.Context(), s.Pool, accountID, userID) {
		return uuid.Nil, errors.New("you are not a member of the selected org")
	}
	return accountID, nil
}

// Lifetimes — kept short so abandoned flows expire cheaply.
const (
	deviceCodeTTL = 10 * time.Minute
	bearerTTL     = 30 * 24 * time.Hour // 30 days
	pollInterval  = 5                   // seconds, what CLI should wait between polls
)

// POST /oauth/device/code (RFC 8628 §3.2).
type deviceCodeReply struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// deviceCodeRequest is the optional JSON body the CLI may POST.
// All fields optional — empty body still mints a code (back-compat).
type deviceCodeRequest struct {
	ClientName string `json:"client_name"`
}

func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	var req deviceCodeRequest
	// Body is optional: ignore decode errors so empty / malformed
	// bodies still work (early-version CLIs sent nil).
	_ = json.NewDecoder(r.Body).Decode(&req)
	dc, err := db.CreateDeviceCode(r.Context(), s.Pool, deviceCodeTTL, req.ClientName)
	if err != nil {
		http.Error(w, "create device code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	verify := s.BaseURL + "/oauth/device/verify"
	writeJSON(w, http.StatusOK, deviceCodeReply{
		DeviceCode:              dc.DeviceCode,
		UserCode:                dc.UserCode,
		VerificationURI:         verify,
		VerificationURIComplete: verify + "?user_code=" + dc.UserCode,
		Interval:                pollInterval,
		ExpiresIn:               int(deviceCodeTTL.Seconds()),
	})
}

// GET /oauth/device/verify — browser landing.
func (s *Server) handleDeviceVerifyPage(w http.ResponseWriter, r *http.Request) {
	userCode := r.URL.Query().Get("user_code")
	// Best-effort lookup so the page can name the calling client.
	// Failures (unknown / expired user_code) just mean we render the
	// generic copy — don't block the page.
	var clientName string
	if userCode != "" {
		if dc, err := db.LookupDeviceCodeByUserCode(r.Context(), s.Pool, userCode); err == nil {
			clientName = dc.ClientName
		}
	}
	// Build the org dropdown: every org the signed-in user can act in,
	// with the default (last-selected, else default org) pre-selected.
	uid := UserIDFromCtx(r.Context())
	accounts, _ := db.ListAccountsForUser(r.Context(), s.Pool, uid)
	defaultOrg := s.resolveDefaultOrg(r, uid, accounts)
	orgs := make([]orgOption, 0, len(accounts))
	for _, a := range accounts {
		orgs = append(orgs, orgOption{
			ID:       a.ID.String(),
			Name:     a.Name,
			Selected: a.ID.String() == defaultOrg,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "device_verify.html", map[string]any{
		"UserCode":   userCode,
		"ClientName": clientName,
		"approved":   r.URL.Query().Get("approved"),
		"Orgs":       orgs,
	})
}

// POST /oauth/device/verify — browser action.
func (s *Server) handleDeviceVerifySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	userCode := r.FormValue("user_code")
	if userCode == "" {
		http.Error(w, "missing user_code", http.StatusBadRequest)
		return
	}
	uid := UserIDFromCtx(r.Context())

	// Resolve the org the user is authorizing into. The dropdown posts
	// account_id; an empty value (old client / no selection) falls back to
	// the user's default org. Either way, the user must belong to it.
	accountID, err := s.resolveSelectedOrg(r, uid, r.FormValue("account_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := db.ApproveDeviceCode(r.Context(), s.Pool, userCode, uid, accountID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "user_code not found or expired", http.StatusNotFound)
			return
		}
		http.Error(w, "approve: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Remember the choice so the next login defaults to it. Best-effort —
	// a failure here doesn't invalidate the approval that just succeeded.
	_ = db.SetLastSelectedAccount(r.Context(), s.Pool, uid, accountID)
	// Render a "you can close this tab" page (or redirect with success
	// query). Keeping it simple — just a 303 to a confirmation route.
	http.Redirect(w, r, "/oauth/device/verify?user_code="+userCode+"&approved=1", http.StatusSeeOther)
}

// POST /oauth/device/token (RFC 8628 §3.5).
type deviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

type deviceTokenReply struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	// AccountID is the org the user selected on the verify page. The CLI
	// forwards it to /auth/exchange so the NATS JWT is minted there.
	AccountID string `json:"account_id,omitempty"`
}

type deviceTokenError struct {
	Error string `json:"error"`
}

func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req deviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Also accept form-encoded per RFC.
		if err := r.ParseForm(); err == nil {
			req.DeviceCode = r.FormValue("device_code")
		}
	}
	if req.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, deviceTokenError{Error: "invalid_request"})
		return
	}
	userID, accountID, err := db.ConsumeDeviceCode(r.Context(), s.Pool, req.DeviceCode)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDeviceCodePending):
			writeJSON(w, http.StatusBadRequest, deviceTokenError{Error: "authorization_pending"})
		case errors.Is(err, db.ErrDeviceCodeExpired):
			writeJSON(w, http.StatusBadRequest, deviceTokenError{Error: "expired_token"})
		case errors.Is(err, db.ErrNotFound):
			writeJSON(w, http.StatusBadRequest, deviceTokenError{Error: "invalid_grant"})
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	plaintext, _, err := db.IssueBearerToken(r.Context(), s.Pool, userID, bearerTTL)
	if err != nil {
		http.Error(w, "issue token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	reply := deviceTokenReply{
		AccessToken: plaintext,
		TokenType:   "bearer",
		ExpiresIn:   int(bearerTTL.Seconds()),
	}
	if accountID != nil {
		reply.AccountID = accountID.String()
	}
	writeJSON(w, http.StatusOK, reply)
}
