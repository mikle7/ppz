package natsubj

import (
	"testing"

	"github.com/google/uuid"
)

// Phase 1.5: BuildSubject is the new four-role subject builder per
// locked decision #18 — <account>.<manifold?>.<source?>.<pipe>.
//
// manifold is 0+ dot-separated segments; '' = root namespace.
// source ("collar") is 0 or 1 segment; '' = uncollared.
//
// Wire-level the manifold-only and source-only shapes are
// indistinguishable (acct.X.pipe could be either) — that's by
// design. Disambiguation happens by DB row at create time; the
// builder just emits the canonical dotted form.

func TestBuildSubject_FourShapes(t *testing.T) {
	acct := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	acctStr := acct.String()

	cases := []struct {
		name     string
		manifold string
		source   string
		pipe     string
		want     string
	}{
		{
			name:     "root manifold, uncollared",
			manifold: "",
			source:   "",
			pipe:     "public",
			want:     acctStr + ".public",
		},
		{
			name:     "root manifold, collared on cindy",
			manifold: "",
			source:   "cindy",
			pipe:     "inbox",
			want:     acctStr + ".cindy.inbox",
		},
		{
			name:     "single-segment manifold, uncollared",
			manifold: "team1",
			source:   "",
			pipe:     "room",
			want:     acctStr + ".team1.room",
		},
		{
			name:     "multi-segment manifold, collared",
			manifold: "proj.team",
			source:   "cindy",
			pipe:     "stdout",
			want:     acctStr + ".proj.team.cindy.stdout",
		},
		{
			name:     "multi-segment manifold, uncollared",
			manifold: "proj.team",
			source:   "",
			pipe:     "announcements",
			want:     acctStr + ".proj.team.announcements",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildSubject(acct, tc.manifold, tc.source, tc.pipe)
			if got != tc.want {
				t.Errorf("BuildSubject(_, %q, %q, %q) = %q, want %q",
					tc.manifold, tc.source, tc.pipe, got, tc.want)
			}
		})
	}
}

// Subject (the pre-Phase-1.5 3-arg builder) stays available — collared
// shape at root manifold — and BuildSubject with manifold="" collar=h
// pipe=p must produce the same wire form. Pins backward-compat for
// callers that still use the old builder until Cycle B fully threads
// manifold through the call graph.
func TestBuildSubject_RootCollaredMatchesLegacySubject(t *testing.T) {
	acct := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	legacy := Subject(acct, "cindy", "inbox")
	new4 := BuildSubject(acct, "", "cindy", "inbox")
	if legacy != new4 {
		t.Errorf("Subject(acct, cindy, inbox) = %q; BuildSubject(acct, \"\", cindy, inbox) = %q — must match for backward compat",
			legacy, new4)
	}
}

// BuildStreamName produces a unique JetStream stream name from the
// four-role shape. Manifold dots are replaced with underscores
// (NATS stream names can't contain dots). Empty manifold/source
// slots are omitted entirely rather than emitted as empty
// segments — keeps names short, and manifold/source segments can't
// contain underscores (handle regex forbids them) so no ambiguity.
func TestBuildStreamName_FourShapes(t *testing.T) {
	acct := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	// First 8 hex chars of acct UUID with hyphens stripped.
	hexShort := "00000000"

	cases := []struct {
		name     string
		manifold string
		source   string
		pipeName string
		want     string
	}{
		{"root uncollared", "", "", "public", "pipe_" + hexShort + "_public"},
		{"root collared on cindy", "", "cindy", "inbox", "pipe_" + hexShort + "_cindy_inbox"},
		{"single-segment manifold uncollared", "team1", "", "room", "pipe_" + hexShort + "_team1_room"},
		{"single-segment manifold collared", "team1", "cindy", "stdout", "pipe_" + hexShort + "_team1_cindy_stdout"},
		{"multi-segment manifold uncollared", "proj.team", "", "announcements", "pipe_" + hexShort + "_proj_team_announcements"},
		{"multi-segment manifold collared", "proj.team", "cindy", "stdout", "pipe_" + hexShort + "_proj_team_cindy_stdout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildStreamName(acct, tc.manifold, tc.source, tc.pipeName)
			if got != tc.want {
				t.Errorf("BuildStreamName(_, %q, %q, %q) = %q, want %q",
					tc.manifold, tc.source, tc.pipeName, got, tc.want)
			}
		})
	}
}
