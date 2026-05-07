package cliproto

import "testing"

// v0.25.0: a new error code E_INVALID_SUBJECT for subject-rule violations
// (the `ack:` prefix is reserved for daemon-emitted protocol messages).
// CLI-side and daemon-side IPC both use it — wire string MUST be stable.
func TestEInvalidSubject_StableSurface(t *testing.T) {
	if string(EInvalidSubject) != "E_INVALID_SUBJECT" {
		t.Fatalf("EInvalidSubject wire string = %q, want E_INVALID_SUBJECT", EInvalidSubject)
	}
	if got := ExitCode(EInvalidSubject); got == 1 {
		t.Errorf("ExitCode(EInvalidSubject) = 1 (generic); want a dedicated non-1 exit code")
	}
	if msg := Message(EInvalidSubject); msg == "" || msg == "unknown error" {
		t.Errorf("Message(EInvalidSubject) is missing or generic: %q", msg)
	}
	// Constructor returns a populated *Error.
	e := New(EInvalidSubject)
	if e == nil || e.Code != EInvalidSubject {
		t.Fatalf("New(EInvalidSubject) returned %+v", e)
	}
}
