package daemon

import (
	"reflect"
	"testing"
)

// TestMatchAnyTarget covers patterns that target the full
// `<handle>.<pipe>` rather than just the handle. Today the matcher
// only checks against handle, so a sensible-looking `*.stdout` pattern
// never matches anything (no handle ends in `.stdout`). The fix
// extends matching to also try `<handle>.<pipe>`.
//
// All these cases should pass once matchAnyTarget exists. RED today.
func TestMatchAnyTarget(t *testing.T) {
	cases := []struct {
		name     string
		handle   string
		pipe     string
		patterns []string
		want     bool
	}{
		{"empty patterns matches anything", "anything", "broadcast", nil, true},
		{"handle-only pattern still matches (back-compat)", "agent-one", "broadcast", []string{"agent-*"}, true},
		{"pipe-suffix pattern matches stdout pipe", "apple", "stdout", []string{"*.stdout"}, true},
		{"pipe-suffix pattern (sql alias)", "apple", "stdout", []string{"%.stdout"}, true},
		{"pipe-suffix doesn't match broadcast pipe", "apple", "broadcast", []string{"*.stdout"}, false},
		{"exact target match", "apple", "stdout", []string{"apple.stdout"}, true},
		{"exact target mismatch on pipe", "apple", "broadcast", []string{"apple.stdout"}, false},
		{"% spans handle.pipe boundary", "apple", "stdout", []string{"%stdout"}, true},
		{"multiple patterns OR — handle plus pipe", "apple", "stdout", []string{"banana", "*.stdout"}, true},

		// Uncollared pipes have handle="". A bare pattern matching the
		// pipe name alone is the natural way to address them.
		{"bare pattern matches uncollared pipe", "", "plaza", []string{"plaza"}, true},
		{"bare glob matches uncollared pipe", "", "lobby-1", []string{"lobby-*"}, true},
		{"bare pattern doesn't match different uncollared", "", "plaza", []string{"lobby"}, false},
		// Manifold-uncollared: pipe name carries dotted manifold path
		// (e.g. "team-a.chat"). Pattern must match against that whole
		// path so users can address namespaced pipes with their
		// manifold prefix.
		{"manifold.pipe pattern matches namespaced uncollared", "", "team-a.chat", []string{"team-a.chat"}, true},
		{"manifold-glob matches namespaced uncollared", "", "team-a.chat", []string{"team-*.chat"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchAnyTarget(c.handle, c.pipe, c.patterns); got != c.want {
				t.Errorf("matchAnyTarget(%q, %q, %v) = %v, want %v",
					c.handle, c.pipe, c.patterns, got, c.want)
			}
		})
	}
}

func TestPipesForKindIncludesInbox(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want []string
	}{
		{
			name: "message source",
			kind: "",
			want: []string{"inbox"},
		},
		{
			name: "pty source",
			kind: "pty",
			want: []string{"inbox", "stdctrl", "stdin", "stdout"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pipesForKind(c.kind); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("pipesForKind(%q) = %v, want %v", c.kind, got, c.want)
			}
		})
	}
}
