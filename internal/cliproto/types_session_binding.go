package cliproto

import "time"

// IPC method names for the agent binding lifecycle. See
// docs/specs/session-binding.md for the design.
const (
	// IPCRegisterAgentBinding — issued by `ppz terminal share <H>`
	// immediately after pty open and before exec'ing the harness. The
	// share's own pid is the binding's stable identity anchor.
	IPCRegisterAgentBinding = "RegisterAgentBinding"

	// IPCUnregisterAgentBinding — issued at clean share teardown.
	// Abnormal exits are handled by lazy validation on lookup.
	IPCUnregisterAgentBinding = "UnregisterAgentBinding"
)

// RegisterAgentBindingRequest carries the handle and share pid. The
// CLI sends its own pid via os.Getpid(); local-only trust boundary —
// see spec §Non-goals.
type RegisterAgentBindingRequest struct {
	Handle   string `json:"handle"`
	SharePID int    `json:"share_pid"`
}

// RegisterAgentBindingReply confirms the binding was recorded.
type RegisterAgentBindingReply struct {
	Handle       string    `json:"handle"`
	SharePID     int       `json:"share_pid"`
	SessionKey   string    `json:"session_key"`
	RegisteredAt time.Time `json:"registered_at"`
}

// UnregisterAgentBindingRequest carries the share pid to drop.
type UnregisterAgentBindingRequest struct {
	SharePID int `json:"share_pid"`
}

// UnregisterAgentBindingReply is empty on success.
type UnregisterAgentBindingReply struct{}

// Error codes for the session-binding subsystem. Keep in sync with
// docs/ERRORS.md.
const (
	// EBindingConflict — RegisterAgentBinding called with the same
	// SharePID against a different handle. Caller must unregister first.
	EBindingConflict = "E_BINDING_CONFLICT"

	// EBindingUnknown — the daemon lost the binding (e.g. after a
	// persistence corruption restart) and the share process is
	// expected to re-issue RegisterAgentBinding lazily.
	EBindingUnknown = "E_BINDING_UNKNOWN"
)
