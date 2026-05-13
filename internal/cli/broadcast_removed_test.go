package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// RED tests for the pre-launch removal of `ppz broadcast`. See:
//   - docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo)
//   - Locked decision #16 in OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md
//
// Field signal: teams use shared "room" pipes (e.g. `team1.room`) far
// more than one-to-many announce. The `broadcast` opinionated default
// doesn't match how the tool is actually used; a custom pipe with
// `--writers=anyone` (or `--writers=owner` for the rare announce
// case) expresses both patterns with more flexibility.

// TestTopLevelVerbs_ExcludesBroadcast asserts the completion engine
// no longer advertises `broadcast`.
func TestTopLevelVerbs_ExcludesBroadcast(t *testing.T) {
	for _, v := range topLevelVerbs {
		if v == "broadcast" {
			t.Errorf("topLevelVerbs still contains %q — pre-launch removal pending", v)
		}
	}
}

// TestTargetTakingVerbs_ExcludesBroadcast asserts the dynamic-target
// completion path no longer treats `broadcast` as a target-taking verb.
func TestTargetTakingVerbs_ExcludesBroadcast(t *testing.T) {
	if targetTakingVerbs["broadcast"] {
		t.Errorf("targetTakingVerbs still flags %q — pre-launch removal pending", "broadcast")
	}
}

// TestUsage_OmitsBroadcastVerb asserts the top-level usage text does
// not mention `ppz broadcast`. Comments / explanation text about
// broadcast-pattern message flows may remain — the rule is no live
// CLI verb invocation example.
func TestUsage_OmitsBroadcastVerb(t *testing.T) {
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
		"ppz broadcast ",
		"ppz broadcast\n",
		"ppz broadcast[",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — pre-launch removal pending", banned)
		}
	}
}
