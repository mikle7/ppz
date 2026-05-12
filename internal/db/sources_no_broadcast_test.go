package db

import "testing"

// RED test for commit 5 of Phase 1: the auto-provisioned pipe set
// must no longer include `broadcast`. Locked decision #16 — pre-launch
// removal of the broadcast auto-pipe and the `ppz broadcast` CLI
// verb. See docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo).
//
// This test fails today (Source.Pipes() returns ["broadcast", "inbox", ...])
// and passes once commit 5 removes `broadcast` from the default lists.
func TestSourcePipes_OmitsBroadcast(t *testing.T) {
	cases := []struct {
		name string
		src  Source
	}{
		{"message", Source{Kind: SourceKindMessage}},
		{"pty", Source{Kind: SourceKindPTY}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, p := range tc.src.Pipes() {
				if p == "broadcast" {
					t.Errorf("Source{Kind:%s}.Pipes() still includes %q — pre-launch removal pending",
						tc.src.Kind, p)
				}
			}
			if tc.src.IsAutoPipe("broadcast") {
				t.Errorf("Source{Kind:%s}.IsAutoPipe(\"broadcast\") still returns true — pre-launch removal pending",
					tc.src.Kind)
			}
		})
	}
}
