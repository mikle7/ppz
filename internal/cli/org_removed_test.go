package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// RED tests for the pre-launch removal of `ppz org`. See:
//   - docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo)
//   - Locked decisions #11, #18 in OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md
//
// These tests fail today (the org verb is still wired) and pass after
// commit 3 of the Phase 1 PR removes it. Companion e2e fixtures live
// in tests/org/{list,switch,create,invite}-returns-unknown-command/.

// TestTopLevelVerbs_ExcludesOrg asserts the completion engine's
// top-level verb list no longer advertises `org`. Today `topLevelVerbs`
// includes "org" — completion test TestComplete_TopLevel_IncludesOrg
// pins the current state. After commit 3 the symmetric removal lands
// here and that opposing test goes away.
func TestTopLevelVerbs_ExcludesOrg(t *testing.T) {
	for _, v := range topLevelVerbs {
		if v == "org" {
			t.Errorf("topLevelVerbs still contains %q — pre-launch removal pending", v)
		}
	}
}

// TestSubverbs_ExcludesOrg asserts the completion subverb map has
// no `org` key. Today `subverbs["org"] = []string{"list","switch","invite"}`.
func TestSubverbs_ExcludesOrg(t *testing.T) {
	if _, ok := subverbs["org"]; ok {
		t.Errorf("subverbs still has key %q — pre-launch removal pending", "org")
	}
}

// TestUsage_OmitsOrgVerbs asserts the top-level usage text does not
// mention `ppz org`. The Sources / Setup sections may still describe
// "current source" — that wording leaves with the source-removal cycle,
// not this one.
func TestUsage_OmitsOrgVerbs(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	for _, banned := range []string{
		"ppz org ",
		"ppz org {",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — pre-launch removal pending", banned)
		}
	}
}
