package natsauth

import (
	"testing"
)

// Phase 1.5 Cycle D — minimal ACL coverage for uncollared pipes.
//
// The existing per-account wildcard `<accountID>.>` already grants pub+sub
// on every subject under the account, so uncollared pipes (which sit at
// shapes like `<accountID>.team1.room` or `<accountID>.public`) are
// already covered by the same JWT a Phase 1 user receives. No mint
// changes were needed for Phase 1.5's minimal "make uncollared work"
// requirement.
//
// This file pins that property so a future tightening of the JWT
// (the leaf-name role-asymmetry work that lands in Phase 3) can't
// silently break uncollared pipes without an explicit test update.

func TestSubjectsForOrgUser_CoversUncollaredShapes(t *testing.T) {
	const acct = "11111111-2222-3333-4444-555555555555"
	subj := SubjectsForOrgUser(acct)

	// The account wildcard must be present in both Publish and Subscribe.
	// Pattern is the bare token followed by `.>` — both uncollared
	// (acct.team1.room, acct.public) and collared (acct.cindy.inbox)
	// shapes match.
	want := acct + ".>"

	foundPub := false
	for _, p := range subj.Publish {
		if p == want {
			foundPub = true
			break
		}
	}
	if !foundPub {
		t.Errorf("SubjectsForOrgUser(%q).Publish missing %q — uncollared pipes need this wildcard for pub access. Got: %v", acct, want, subj.Publish)
	}

	foundSub := false
	for _, p := range subj.Subscribe {
		if p == want {
			foundSub = true
			break
		}
	}
	if !foundSub {
		t.Errorf("SubjectsForOrgUser(%q).Subscribe missing %q — uncollared pipes need this wildcard for sub access. Got: %v", acct, want, subj.Subscribe)
	}
}

// Negative pin: a non-member's JWT must NOT include the account
// wildcard. Today this is enforced by the account-scoped mint call
// chain (MintUserJWT is invoked with the user's own account, not
// somebody else's), so the test asserts the upstream invariant: a
// SubjectsForOrgUser call for accountA never produces patterns
// granting accountB's wildcard.
func TestSubjectsForOrgUser_DoesNotLeakOtherAccountWildcard(t *testing.T) {
	const accountA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const accountB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	subj := SubjectsForOrgUser(accountA)

	otherWildcard := accountB + ".>"
	for _, p := range subj.Publish {
		if p == otherWildcard {
			t.Errorf("SubjectsForOrgUser(%q).Publish leaks %q — JWT mint must not grant cross-account access", accountA, otherWildcard)
		}
	}
	for _, p := range subj.Subscribe {
		if p == otherWildcard {
			t.Errorf("SubjectsForOrgUser(%q).Subscribe leaks %q — JWT mint must not grant cross-account access", accountA, otherWildcard)
		}
	}
}
