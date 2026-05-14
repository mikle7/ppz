package server

import (
	"reflect"
	"testing"
)

// Phase 2 Cycle C: PPZ_SERVER_AUTH_MODE infrastructure.
//
// AuthMode is the parsed env-var value; valid modes are "none"
// (default), "password", and "oauth". ParseAuthMode validates input
// (case-insensitive) and returns an error for unknown values so a
// typo in the env var fails server boot loudly. Server.AuthMode is
// populated at construction time.
//
// /login behaviour wiring is Cycle D. Cycle C only adds the type +
// parser + field. See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md.

// TestAuthMode_ParseAuthMode_Defaults — empty string parses as
// AuthModeNone. The default for OSS-server deployments is unauth'd.
func TestAuthMode_ParseAuthMode_Defaults(t *testing.T) {
	got, err := ParseAuthMode("")
	if err != nil {
		t.Fatalf("ParseAuthMode(\"\"): %v, want no error", err)
	}
	if got != AuthModeNone {
		t.Errorf("ParseAuthMode(\"\") = %v, want AuthModeNone", got)
	}
}

// TestAuthMode_ParseAuthMode_AcceptsValid — lowercase + mixed-case
// inputs for each of the three modes round-trip cleanly.
func TestAuthMode_ParseAuthMode_AcceptsValid(t *testing.T) {
	cases := []struct {
		in   string
		want AuthMode
	}{
		{"none", AuthModeNone},
		{"NONE", AuthModeNone},
		{"None", AuthModeNone},
		{"password", AuthModePassword},
		{"PASSWORD", AuthModePassword},
		{"Password", AuthModePassword},
		{"oauth", AuthModeOAuth},
		{"OAUTH", AuthModeOAuth},
		{"OAuth", AuthModeOAuth},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseAuthMode(tc.in)
			if err != nil {
				t.Fatalf("ParseAuthMode(%q): %v, want no error", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseAuthMode(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestAuthMode_ParseAuthMode_RejectsInvalid — unknown values return
// a non-nil error so a typo in PPZ_SERVER_AUTH_MODE fails boot loudly
// rather than silently falling back to a default.
func TestAuthMode_ParseAuthMode_RejectsInvalid(t *testing.T) {
	for _, in := range []string{"garbage", "passwd", "oa", " none", "none ", "yes"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseAuthMode(in)
			if err == nil {
				t.Errorf("ParseAuthMode(%q): no error, want error", in)
			}
		})
	}
}

// TestServer_HasAuthModeField — reflection pin on Server.AuthMode.
// The field exists with type AuthMode so handlers (Cycle D) can
// dispatch by it.
func TestServer_HasAuthModeField(t *testing.T) {
	field, ok := reflect.TypeOf(Server{}).FieldByName("AuthMode")
	if !ok {
		t.Fatal("Server.AuthMode field missing")
	}
	authModeType := reflect.TypeOf(AuthMode(""))
	if field.Type != authModeType {
		t.Errorf("Server.AuthMode type = %v, want %v", field.Type, authModeType)
	}
}
