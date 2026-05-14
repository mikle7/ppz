package server

// AuthMode controls how the admin web UI authenticates users. Set at
// server boot via the PPZ_SERVER_AUTH_MODE env var; valid values are
// "none" (default — admin UI unauth'd, trusted-network deploys),
// "password" (username/password form against users.password_hash),
// and "oauth" (delegates to an out-of-tree Provider implementation,
// e.g. pipescloud's GitHub OAuth).
//
// All three modes terminate in the same downstream contract: a
// user_id session cookie that authenticated routes consume identically.
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle C.

import (
	"fmt"
	"strings"
)

type AuthMode string

const (
	AuthModeNone     AuthMode = "none"
	AuthModePassword AuthMode = "password"
	AuthModeOAuth    AuthMode = "oauth"
)

// ParseAuthMode parses a PPZ_SERVER_AUTH_MODE value. Empty input
// defaults to AuthModeNone. Comparison is case-insensitive. Unknown
// values return a non-nil error so a typo fails server boot loudly
// rather than silently defaulting.
func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToLower(s) {
	case "":
		return AuthModeNone, nil
	case "none":
		return AuthModeNone, nil
	case "password":
		return AuthModePassword, nil
	case "oauth":
		return AuthModeOAuth, nil
	default:
		return "", fmt.Errorf("PPZ_SERVER_AUTH_MODE=%q is not a valid mode (want none, password, or oauth)", s)
	}
}
