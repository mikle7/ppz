package cli

import (
	"sort"
	"testing"
)

// Phase 1.5 Cycle C: `ppz set namespace PATH` / `ppz unset namespace`
// — daemon-state CLI pattern (locked decision #20) extended to
// the new namespace (manifold) slot.
//
// No `ppz get namespace` — `ppz status` is the read interface
// (per the in-conversation decision to keep status as the canonical
// place to display daemon state).

func TestSubverbs_SetIncludesNamespace(t *testing.T) {
	got := append([]string{}, subverbs["set"]...)
	sort.Strings(got)
	want := []string{"handle", "namespace"}
	if len(got) != len(want) {
		t.Fatalf("subverbs[set] = %v, want %v", subverbs["set"], want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subverbs[set] = %v, want %v", subverbs["set"], want)
		}
	}
}

func TestSubverbs_UnsetIncludesNamespace(t *testing.T) {
	got := append([]string{}, subverbs["unset"]...)
	sort.Strings(got)
	want := []string{"handle", "namespace"}
	if len(got) != len(want) {
		t.Fatalf("subverbs[unset] = %v, want %v", subverbs["unset"], want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subverbs[unset] = %v, want %v", subverbs["unset"], want)
		}
	}
}

func TestSubverbs_GetDoesNotIncludeNamespace(t *testing.T) {
	for _, sub := range subverbs["get"] {
		if sub == "namespace" {
			t.Fatalf("subverbs[get] still has %q — Phase 1.5 omits `ppz get namespace` (status is the read interface)", "namespace")
		}
	}
}

