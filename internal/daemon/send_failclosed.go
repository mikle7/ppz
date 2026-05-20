package daemon

import "github.com/pipescloud/ppz/internal/cliproto"

// resolveSenderForSend is the pure helper extracted from handleSend /
// resolveSendTarget so the empty-sender → E_NO_CURRENT_SOURCE contract
// can be unit-tested without NATS / IPC plumbing.
//
// Today's behavior (pre-Layer-2): returns the session's current handle
// or empty string. Empty string is what allows anonymous sends to slip
// through.
//
// Post-Layer-2 behavior: empty resolved sender returns ENoCurrentSource.
type senderResolution struct {
	sender string
	err    *cliproto.Error
}

func resolveSenderForSend(s *State, session string) senderResolution {
	// not implemented — stub returns the broken pre-spec behavior so
	// tests fail and drive the impl.
	return senderResolution{sender: s.Current(session), err: nil}
}
