package daemon

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/natsubj"
)

// Phase 1.5.1: shouldTryUncollaredFirst pins the runtime-fallback
// dispatch contract. Any bare target triggers an uncollared
// resolution attempt; the daemon falls back to legacy collared if
// the uncollared stream doesn't exist (handled at the call site, not
// in this pure helper).

func TestShouldTryUncollaredFirst_BareTargetEmpty_False(t *testing.T) {
	if shouldTryUncollaredFirst("") {
		t.Error("shouldTryUncollaredFirst(\"\") = true, want false (no bare target → no uncollared attempt)")
	}
}

func TestShouldTryUncollaredFirst_BareTargetSet_True(t *testing.T) {
	if !shouldTryUncollaredFirst("room") {
		t.Error("shouldTryUncollaredFirst(\"room\") = false, want true (bare target → always attempt uncollared)")
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
