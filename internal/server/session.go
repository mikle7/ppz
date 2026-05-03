package server

// HMAC-signed session cookie. Stateless — the cookie value carries the
// user_id + expiry, signed with a per-deployment secret (PPZ_SESSION_KEY).
//
// Format: base64url(payload-json) + "." + base64url(hmac-sha256(payload, key))
//
// Two halves separated by ".":
//   - payload: a JSON object {"uid":"<uuid>","exp":<unix-seconds>}
//   - sig:     hmac-sha256 of the payload bytes, keyed by SessionKey
//
// Verification is constant-time. A wrong-key, tampered, or expired
// cookie all produce ErrInvalidSession; callers treat them identically
// (redirect to /login).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidSession is returned by VerifySessionCookie for any failure
// — malformed, wrong key, tampered, expired. Callers don't get to
// distinguish on purpose; "what went wrong" is leaky info.
var ErrInvalidSession = errors.New("invalid session")

// SessionKey is the HMAC signing key. 32+ bytes recommended.
type SessionKey []byte

// SessionPayload is what we encode into the cookie value (and what
// VerifySessionCookie returns on success).
type SessionPayload struct {
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// sessionWire is the on-disk JSON shape. Field names short on purpose
// — every byte travels over the wire to clients.
type sessionWire struct {
	UID string `json:"uid"`
	Exp int64  `json:"exp"`
}

func SignSessionCookie(key SessionKey, p SessionPayload) (string, error) {
	if len(key) == 0 {
		return "", errors.New("session key is empty")
	}
	body, err := json.Marshal(sessionWire{
		UID: p.UserID.String(),
		Exp: p.ExpiresAt.Unix(),
	})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(body) +
		"." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func VerifySessionCookie(key SessionKey, cookieValue string) (SessionPayload, error) {
	if cookieValue == "" || len(key) == 0 {
		return SessionPayload{}, ErrInvalidSession
	}
	dot := strings.IndexByte(cookieValue, '.')
	if dot <= 0 || dot == len(cookieValue)-1 {
		return SessionPayload{}, ErrInvalidSession
	}
	body, err := base64.RawURLEncoding.DecodeString(cookieValue[:dot])
	if err != nil {
		return SessionPayload{}, ErrInvalidSession
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(cookieValue[dot+1:])
	if err != nil {
		return SessionPayload{}, ErrInvalidSession
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	wantSig := mac.Sum(nil)
	if !hmac.Equal(gotSig, wantSig) {
		return SessionPayload{}, ErrInvalidSession
	}

	var w sessionWire
	if err := json.Unmarshal(body, &w); err != nil {
		return SessionPayload{}, ErrInvalidSession
	}
	uid, err := uuid.Parse(w.UID)
	if err != nil {
		return SessionPayload{}, ErrInvalidSession
	}
	exp := time.Unix(w.Exp, 0).UTC()
	if time.Now().After(exp) {
		return SessionPayload{}, ErrInvalidSession
	}
	return SessionPayload{UserID: uid, ExpiresAt: exp}, nil
}
