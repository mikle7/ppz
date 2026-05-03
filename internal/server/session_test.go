package server

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Random key fixture; in production this comes from PPZ_SESSION_KEY.
var testSessionKey = SessionKey([]byte("test-session-key-thirty-two-bytes-x"))

func TestSessionCookie_SignVerifyRoundTrip(t *testing.T) {
	uid := uuid.New()
	exp := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

	cookie, err := SignSessionCookie(testSessionKey, SessionPayload{UserID: uid, ExpiresAt: exp})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if cookie == "" {
		t.Fatal("expected non-empty cookie")
	}

	got, err := VerifySessionCookie(testSessionKey, cookie)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.UserID != uid {
		t.Errorf("UserID round-trip: got %v want %v", got.UserID, uid)
	}
	if !got.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt round-trip: got %v want %v", got.ExpiresAt, exp)
	}
}

func TestSessionCookie_TamperedRejected(t *testing.T) {
	cookie, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Flip the first char of the signature half. Tampering the LAST
	// char is unsafe: with RawURLEncoding the trailing char of a 32-byte
	// HMAC encodes only 4 significant bits, so swapping e.g. 'A' (0)
	// for 'B' (1) doesn't change any decoded byte — verify still passes
	// and the test flakes. The first sig char encodes the top 6 bits of
	// byte 0, so any swap there is guaranteed to change a real byte.
	dot := strings.IndexByte(cookie, '.')
	if dot < 0 || dot+1 >= len(cookie) {
		t.Fatalf("malformed cookie %q: no signature half", cookie)
	}
	flip := byte('A')
	if cookie[dot+1] == 'A' {
		flip = 'B'
	}
	tampered := cookie[:dot+1] + string(flip) + cookie[dot+2:]

	if _, err := VerifySessionCookie(testSessionKey, tampered); err == nil {
		t.Fatal("verify must reject tampered cookie")
	}
}

func TestSessionCookie_ExpiredRejected(t *testing.T) {
	cookie, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifySessionCookie(testSessionKey, cookie); err == nil {
		t.Fatal("verify must reject expired cookie")
	}
}

func TestSessionCookie_WrongKeyRejected(t *testing.T) {
	cookie, err := SignSessionCookie(testSessionKey, SessionPayload{
		UserID:    uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	otherKey := SessionKey([]byte("different-key-aaaaaaaaaaaaaaaaaaaa"))
	if _, err := VerifySessionCookie(otherKey, cookie); err == nil {
		t.Fatal("verify must reject cookie signed with a different key")
	}
}

func TestSessionCookie_MalformedRejected(t *testing.T) {
	cases := []string{
		"",                  // empty
		"no-dot-separator",  // missing the "." between payload and sig
		"only.payload.parts", // 3 parts
		".empty-payload",    // empty payload half
		"empty-sig.",        // empty sig half
	}
	for _, c := range cases {
		if _, err := VerifySessionCookie(testSessionKey, c); err == nil {
			t.Errorf("verify must reject malformed cookie %q", c)
		}
	}
}
