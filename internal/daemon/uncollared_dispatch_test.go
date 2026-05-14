package daemon

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/natsubj"
)

// Phase 1.5: shouldDispatchUncollared is the pure decision function that
// resolveSendTarget uses to pick the uncollared branch. Pinned by table
// to prevent silent regression of the bare-target shape contract.

func TestShouldDispatchUncollared_BareTargetEmpty_AlwaysCollared(t *testing.T) {
	// An empty bareTarget means the CLI did not signal a bare form;
	// always collared (the legacy `<handle>.<pipe>` path or a request
	// with an explicit handle).
	cases := []struct {
		name                  string
		reqHandle, env, sess  string
	}{
		{"all empty", "", "", ""},
		{"reqHandle set", "cindy", "", ""},
		{"env set", "", "cindy", ""},
		{"session current set", "", "", "cindy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if shouldDispatchUncollared("", tc.reqHandle, tc.env, tc.sess) {
				t.Errorf("shouldDispatchUncollared(\"\", %q, %q, %q) = true, want false (no bare target → collared)",
					tc.reqHandle, tc.env, tc.sess)
			}
		})
	}
}

func TestShouldDispatchUncollared_BareTargetWithNoHandle_Uncollared(t *testing.T) {
	if !shouldDispatchUncollared("room", "", "", "") {
		t.Error("shouldDispatchUncollared(\"room\", \"\", \"\", \"\") = false, want true (bare target + no handles anywhere → uncollared)")
	}
}

func TestShouldDispatchUncollared_AnyHandlePresent_Collared(t *testing.T) {
	// Legacy `ppz send X "msg"` shorthand (X = source handle) must keep
	// working when any handle source is set, regardless of bareTarget.
	cases := []struct {
		name                 string
		reqHandle, env, sess string
	}{
		{"reqHandle wins", "cindy", "", ""},
		{"env wins", "", "alice", ""},
		{"session wins", "", "", "bob"},
		{"all three set", "cindy", "alice", "bob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if shouldDispatchUncollared("room", tc.reqHandle, tc.env, tc.sess) {
				t.Errorf("shouldDispatchUncollared(\"room\", %q, %q, %q) = true, want false (any handle present → collared shortcut wins)",
					tc.reqHandle, tc.env, tc.sess)
			}
		})
	}
}

// uncollaredCursorKey is the cursor-key namespace used by handleRead's
// uncollared branch and by handleList's uncollared enrichment. The two
// paths must agree on the exact prefix shape so cursor advance / unread
// counts stay consistent across read and ls.

func TestUncollaredCursorKey_Shape(t *testing.T) {
	acct := uuid.MustParse("11111111-2222-3333-4444-555566667777")
	cases := []struct {
		name     string
		manifold string
		pipe     string
		want     string
	}{
		{"root", "", "lobby", "uncollared:" + acct.String() + ".lobby"},
		{"single manifold", "team-a", "chat", "uncollared:" + acct.String() + ".team-a.chat"},
		{"multi manifold", "proj.team", "news", "uncollared:" + acct.String() + ".proj.team.news"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uncollaredCursorKey(natsubj.BuildSubject(acct, tc.manifold, "", tc.pipe))
			if got != tc.want {
				t.Errorf("uncollaredCursorKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// The collared and uncollared cursor-key namespaces must be disjoint —
// adding an "uncollared:" prefix is the only way the uncollared path
// declares it owns this key, and the collared path never produces a
// key that starts with that string.

func TestCursorKeys_CollaredAndUncollared_Disjoint(t *testing.T) {
	acct := uuid.MustParse("11111111-2222-3333-4444-555566667777")
	collared := daemonCursorKey(acct, "cindy", "inbox")
	uncollared := uncollaredCursorKey(natsubj.BuildSubject(acct, "", "", "cindy"))
	if collared == uncollared {
		t.Errorf("collared cursor key %q collides with uncollared %q", collared, uncollared)
	}
}
