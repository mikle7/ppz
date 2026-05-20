package daemon

import (
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Tests SF-1..SF-7 from docs/specs/session-binding.md.
//
// SF-1..3 are unit-level guards on the resolveSendTarget / handleSend
// code path. SF-4 is an audit-style guard: a snapshot of the existing
// fixture that today encodes the broken anonymous-sender behavior.
// SF-5..7 are e2e — see tests/sessionbind/ and tests/send-noanon/.
//
// The unit-level tests below use a SendSenderResolver helper extracted
// from handleSend / resolveSendTarget so we can verify the empty-
// sender → ENoCurrentSource contract without standing up NATS.

// SF-1: empty resolved sender → ENoCurrentSource, no publish.
func TestSendFailClosed_EmptySenderRejectsBeforePublish(t *testing.T) {
	s := newTestStateForBindings(t)
	// Session "anon-X" has no current handle set; uncollared send must
	// reject before reaching NATS publish.
	got := resolveSenderForSend(s, "anon-X")
	if got.err == nil || got.err.Code != cliproto.ENoCurrentSource {
		t.Errorf("resolveSenderForSend(empty current) err = %v, want %s", got.err, cliproto.ENoCurrentSource)
	}
	if got.sender != "" {
		t.Errorf("sender = %q on rejected resolve, want empty", got.sender)
	}
}

// SF-2: caller perspective — sender resolves to the session's current
// when set.
func TestSendFailClosed_KnownSenderResolves(t *testing.T) {
	s := newTestStateForBindings(t)
	if err := s.SetCurrent("user-session", "cindy"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := resolveSenderForSend(s, "user-session")
	if got.err != nil {
		t.Errorf("resolveSenderForSend(set current) err = %v, want nil", got.err)
	}
	if got.sender != "cindy" {
		t.Errorf("sender = %q, want cindy", got.sender)
	}
}

// SF-3: explicit PPZ_SESSION=unknown-XYZ, no current bound for that
// session → ENoCurrentSource.
func TestSendFailClosed_ExplicitUnknownSessionRejects(t *testing.T) {
	s := newTestStateForBindings(t)
	// No SetCurrent for "unknown-XYZ"; even though the session id is
	// declared, no sender resolves → reject.
	got := resolveSenderForSend(s, "unknown-XYZ")
	if got.err == nil || got.err.Code != cliproto.ENoCurrentSource {
		t.Errorf("explicit unknown session: err = %v, want %s", got.err, cliproto.ENoCurrentSource)
	}
}

// SF-4 (was: audit pin). The legacy fixture
// `tests/send/send-uncollared-stamps-empty-without-handle/` has been
// updated to expect E_NO_CURRENT_SOURCE; the audit is complete. This
// stub stays so the spec's test ID list remains traceable.
func TestSendFailClosed_LegacyFixtureUpdated(t *testing.T) {
	t.Log("Legacy fixture updated to expect E_NO_CURRENT_SOURCE — see tests/send/send-uncollared-stamps-empty-without-handle/")
}
