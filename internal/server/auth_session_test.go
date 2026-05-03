package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// stubServerWithSessionKey returns a *Server with just enough
// dependencies wired up to test the requireSession middleware in
// isolation. Pool and NATS are left nil — tests must not exercise
// code paths that touch them.
//
// (Implementation will provide an injectable SessionKey on Server.
// During RED, the test references the field shape; the stub may
// choose how to expose it.)
func stubServerWithSessionKey(t *testing.T) *Server {
	t.Helper()
	return &Server{SessionKey: testSessionKey}
}

// helper that makes a request, optionally with a session cookie.
func doRequireSession(t *testing.T, srv *Server, cookie *http.Cookie, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	srv.requireSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})(rec, req)
	return rec
}

func TestRequireSession_AnonymousBrowserRedirectsToLogin(t *testing.T) {
	srv := stubServerWithSessionKey(t)

	rec := doRequireSession(t, srv, nil, "text/html")
	if rec.Code != http.StatusFound {
		t.Errorf("status: got %d want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login") {
		t.Errorf("Location: got %q want /login...", loc)
	}
	if !strings.Contains(loc, "next=") {
		t.Errorf("Location must include ?next=<original-path>, got %q", loc)
	}
}

func TestRequireSession_AnonymousJSONReturns401(t *testing.T) {
	srv := stubServerWithSessionKey(t)
	rec := doRequireSession(t, srv, nil, "application/json")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
	// No Location header on a 401 — only redirects use it.
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("401 must not set Location, got %q", loc)
	}
}

func TestRequireSession_ValidCookieAttachesUserAndChains(t *testing.T) {
	srv := stubServerWithSessionKey(t)
	uid := uuid.New()

	cookieValue, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uid,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookieValue})
	rec := httptest.NewRecorder()

	var seenUID uuid.UUID
	srv.requireSession(func(w http.ResponseWriter, r *http.Request) {
		seenUID = UserIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d want 200 (next handler should run)", rec.Code)
	}
	if seenUID != uid {
		t.Errorf("UserIDFromCtx: got %v want %v (middleware must populate ctx)", seenUID, uid)
	}
}

func TestRequireSession_ExpiredCookieRedirects(t *testing.T) {
	srv := stubServerWithSessionKey(t)
	cookieValue, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rec := doRequireSession(t, srv, &http.Cookie{Name: SessionCookieName, Value: cookieValue}, "text/html")
	if rec.Code != http.StatusFound {
		t.Errorf("expired cookie should 302; got %d", rec.Code)
	}
}

func TestRequireSession_TamperedCookieRedirects(t *testing.T) {
	srv := stubServerWithSessionKey(t)
	cookieValue, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Tamper the first byte of the signature (right after the '.'),
	// not the last byte of the cookie. The last char of a 43-char
	// base64 encoding of a 32-byte HMAC carries only 4 data bits;
	// the low 2 bits are padding that Go's non-strict
	// RawURLEncoding decoder ignores. Flipping a last-char that
	// shares its top 4 bits with the replacement (~6.25% of random
	// cookies, e.g. 'A'/'B'/'C'/'D' ↔ 'A') produces a tampered
	// cookie that still verifies — caused intermittent CI failures
	// before this fix. The signature's first char has no padding
	// bits, so tampering it always changes the decoded signature.
	dot := strings.IndexByte(cookieValue, '.')
	if dot < 0 || dot == len(cookieValue)-1 {
		t.Fatalf("malformed cookie from SignSessionCookie: %q", cookieValue)
	}
	flipped := byte('A')
	if cookieValue[dot+1] == 'A' {
		flipped = 'B'
	}
	tampered := cookieValue[:dot+1] + string(flipped) + cookieValue[dot+2:]
	rec := doRequireSession(t, srv, &http.Cookie{Name: SessionCookieName, Value: tampered}, "text/html")
	if rec.Code != http.StatusFound {
		t.Errorf("tampered cookie should 302; got %d", rec.Code)
	}
}
