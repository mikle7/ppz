package daemon

import "github.com/pipescloud/ppz/internal/cliproto"

// resolveSenderForSend resolves the envelope.sender for a send IPC.
// Returns a non-nil error when the resolved sender is empty — Layer 2
// fail-closed (docs/specs/session-binding.md). Callers stamp
// `envelope.sender = result.sender` only when err is nil.
type senderResolution struct {
	sender string
	err    *cliproto.Error
}

func resolveSenderForSend(s *State, session string) senderResolution {
	sender := s.Current(session)
	if sender == "" {
		return senderResolution{err: cliproto.New(cliproto.ENoCurrentSource)}
	}
	return senderResolution{sender: sender}
}
