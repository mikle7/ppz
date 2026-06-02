package daemon

import "testing"

// senderForRequest centralises the envelope.sender precedence rule for
// `ppz send` / `ppz command` publishes. These tests pin all four
// combinations of (CLI hint set?, per-session state set?) so the GREEN
// commit that flips the stub body has a clear contract to satisfy and
// future stamp-site refactors can't silently drift back to daemon-only
// resolution. See sender_resolve.go for the wider "why".

// TestSenderForRequest_HintWinsWhenStateEmpty pins the user-observable
// bug fix: inside a `ppz terminal share` wrapped shell, the daemon's
// State.Current(session) is "" (IPCCreate skips SetCurrent for PTY-
// kind sources) but the CLI forwards PPZ_CURRENT_HANDLE as
// SendRequest.Sender. envelope.sender must be the hint, not "".
//
// RED today — stub returns stateCurrent.
func TestSenderForRequest_HintWinsWhenStateEmpty(t *testing.T) {
	got := senderForRequest("jimmy", "")
	if got != "jimmy" {
		t.Fatalf("senderForRequest(%q, %q) = %q, want %q — CLI hint must win over empty daemon state so shared-pty sends carry the wrapped handle as sender",
			"jimmy", "", got, "jimmy")
	}
}

// TestSenderForRequest_HintWinsOverState pins env-first precedence
// consistency with `ppz status` (status.go:37+43) and
// `ppz read inbox` (effectiveCurrentHandle, current.go:10-23): when
// both env and daemon state are set, env wins. Without this,
// `ppz send`'s sender stamping would diverge from how every other
// verb interprets the same env var.
//
// RED today — stub returns stateCurrent.
func TestSenderForRequest_HintWinsOverState(t *testing.T) {
	got := senderForRequest("jimmy", "bob")
	if got != "jimmy" {
		t.Fatalf("senderForRequest(%q, %q) = %q, want %q — CLI hint must override daemon state to match the env-first precedence used by `ppz status`, `ppz read inbox`, and the `--request-ack` preflight",
			"jimmy", "bob", got, "jimmy")
	}
}

// TestSenderForRequest_FallsBackToState_WhenNoHint regression-pins the
// non-share path: a normal `ppz send` from a session where the user
// has run `ppz set handle bob` (no PPZ_CURRENT_HANDLE in env) must
// still stamp sender="bob". Going GREEN must not break this.
//
// GREEN today and post-fix.
func TestSenderForRequest_FallsBackToState_WhenNoHint(t *testing.T) {
	got := senderForRequest("", "bob")
	if got != "bob" {
		t.Fatalf("senderForRequest(%q, %q) = %q, want %q — no CLI hint must fall through to daemon's per-session current; regression would break `ppz set handle bob; ppz send …`",
			"", "bob", got, "bob")
	}
}

// TestSenderForRequest_EmptyWhenBothEmpty pins anonymous-send
// semantics: with neither env nor daemon-state set, sender is "".
// Mirrors today's behaviour for a send from a fresh shell with no
// current configured. Must not regress.
//
// GREEN today and post-fix.
func TestSenderForRequest_EmptyWhenBothEmpty(t *testing.T) {
	got := senderForRequest("", "")
	if got != "" {
		t.Fatalf("senderForRequest(%q, %q) = %q, want \"\" — anonymous send (no env, no state) must stamp empty sender",
			"", "", got)
	}
}
