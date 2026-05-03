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

	"github.com/pipescloud/ppz/internal/db"
)

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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "device_verify.html", map[string]any{
		"UserCode":   userCode,
		"ClientName": clientName,
		"approved":   r.URL.Query().Get("approved"),
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
	if err := db.ApproveDeviceCode(r.Context(), s.Pool, userCode, uid); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "user_code not found or expired", http.StatusNotFound)
			return
		}
		http.Error(w, "approve: "+err.Error(), http.StatusInternalServerError)
		return
	}
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
	userID, err := db.ConsumeDeviceCode(r.Context(), s.Pool, req.DeviceCode)
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
	writeJSON(w, http.StatusOK, deviceTokenReply{
		AccessToken: plaintext,
		TokenType:   "bearer",
		ExpiresIn:   int(bearerTTL.Seconds()),
	})
}
