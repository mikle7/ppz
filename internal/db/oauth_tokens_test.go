package db

import (
	"strings"
	"testing"
)

// These tests exercise the wire-format / format invariants of the
// device-flow + bearer-token helpers. DB-state behaviour (insert,
// approve, consume, look up) is covered by the e2e tests against a
// real Postgres.

func TestUserCodeFormat(t *testing.T) {
	// Spec: a user_code is a short, human-typeable string. The plan
	// says ABCD-1234 (4 alphabetic + 4 digit, hyphenated). Test that
	// any code we'd generate matches the canonical regex shape.
	got := generateUserCode()
	if len(got) == 0 {
		t.Fatal("generateUserCode returned empty")
	}
	if !looksLikeUserCode(got) {
		t.Errorf("generateUserCode returned %q; expected XXXX-XXXX shape", got)
	}
}

func TestDeviceCodeFormat(t *testing.T) {
	// device_code is opaque, long, url-safe. Just test it's long
	// enough to be unguessable.
	got := generateDeviceCode()
	if len(got) < 32 {
		t.Errorf("device_code too short: %d chars (want ≥32)", len(got))
	}
}

func TestBearerTokenFormat(t *testing.T) {
	got := generateBearerToken()
	if !strings.HasPrefix(got, "ppz_oauth_") {
		t.Errorf("bearer must start with ppz_oauth_; got %q", got)
	}
	if len(got) < len("ppz_oauth_")+32 {
		t.Errorf("bearer too short: %d chars", len(got))
	}
}

// looksLikeUserCode is a test helper, not exported. The shape is
// 8 chars + a hyphen in position 4, e.g. "ABCD-1234".
func looksLikeUserCode(s string) bool {
	if len(s) != 9 || s[4] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 {
			continue
		}
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
