package server

import "testing"

// splitManifoldName recovers (manifold, name) from the dotted pipe path
// the GUI puts in the URL — the leaf is always the segment after the
// last dot. Cheap pure function, several branches; pinning behaviour
// here keeps the e2e suite from being the only place a future "split
// on first dot" regression would surface.
func TestSplitManifoldName(t *testing.T) {
	for _, tc := range []struct {
		in       string
		manifold string
		name     string
	}{
		{"testroom", "", "testroom"},        // root manifold
		{"foo.testroom", "foo", "testroom"}, // single-segment manifold
		{"foo.bar.room", "foo.bar", "room"}, // multi-segment manifold
		{".room", "", "room"},               // leading dot — manifold collapses to empty
		{"foo.", "foo", ""},                 // trailing dot — name empty (downstream validation rejects)
		{"", "", ""},                        // empty path — both empty (downstream validation rejects)
	} {
		t.Run(tc.in, func(t *testing.T) {
			manifold, name := splitManifoldName(tc.in)
			if manifold != tc.manifold || name != tc.name {
				t.Errorf("splitManifoldName(%q) = (%q, %q), want (%q, %q)",
					tc.in, manifold, name, tc.manifold, tc.name)
			}
		})
	}
}
