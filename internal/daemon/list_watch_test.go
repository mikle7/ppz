package daemon

import "testing"

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
