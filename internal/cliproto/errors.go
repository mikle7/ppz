// Package cliproto holds the contracts shared by the CLI, daemon, server,
// and desktop: error/exit codes, stdout printers, and IPC types.
//
// Everything in this package mirrors docs/ERRORS.md and docs/WIRE.md. If you
// change a string here, change those docs too — and the test fixtures.
package cliproto

import "fmt"

// Code is a stable error identifier. Strings are part of the wire contract.
type Code string

const (
	ENotLoggedIn       Code = "E_NOT_LOGGED_IN"
	EDaemonNotRunning  Code = "E_DAEMON_NOT_RUNNING"
	EInvalidAPIKey     Code = "E_INVALID_API_KEY"
	ESourceTaken       Code = "E_SOURCE_TAKEN"
	ESourceNotFound    Code = "E_SOURCE_NOT_FOUND"
	EInvalidHandle     Code = "E_INVALID_HANDLE"
	ENoCurrentSource   Code = "E_NO_CURRENT_SOURCE"
	EPayloadTooLarge   Code = "E_PAYLOAD_TOO_LARGE"
	EServerUnreachable Code = "E_SERVER_UNREACHABLE"
	ENATSUnreachable   Code = "E_NATS_UNREACHABLE"
	EInvalidPipe       Code = "E_INVALID_PIPE"
	EPipeTaken         Code = "E_PIPE_TAKEN"
	EPipeNotFound      Code = "E_PIPE_NOT_FOUND"
	// EInvalidSubject is returned when a caller (CLI flag parser or IPC
	// client) tries to set a Subject value that violates the reserved-
	// prefix invariant. Daemon-emitted protocol messages own the `ack:`
	// prefix; users cannot stamp it themselves.
	EInvalidSubject Code = "E_INVALID_SUBJECT"
)

// ExitCode returns the integer the CLI exits with for a given Code. Unknown
// codes map to 1 (generic error).
func ExitCode(c Code) int {
	switch c {
	case ENotLoggedIn:
		return 10
	case EDaemonNotRunning:
		return 11
	case EInvalidAPIKey:
		return 12
	case ESourceTaken:
		return 13
	case ESourceNotFound:
		return 14
	case EInvalidHandle:
		return 15
	case ENoCurrentSource:
		return 16
	case EPayloadTooLarge:
		return 17
	case EServerUnreachable:
		return 18
	case ENATSUnreachable:
		return 19
	case EInvalidPipe:
		return 20
	case EPipeTaken:
		return 21
	case EPipeNotFound:
		return 22
	case EInvalidSubject:
		return 23
	}
	return 1
}

// Message is the standard human-readable message for a Code, per ERRORS.md.
func Message(c Code) string {
	switch c {
	case ENotLoggedIn:
		return "not logged in; run 'ppz daemon login URL -apikey K'"
	case EDaemonNotRunning:
		return "daemon not running; run 'ppz daemon start'"
	case EInvalidAPIKey:
		return "invalid api key"
	case ESourceTaken:
		return "source already exists in this org"
	case ESourceNotFound:
		return "source not found"
	case EInvalidHandle:
		return "invalid handle: must match [a-z0-9-] (max 32, no leading/trailing -, not reserved)"
	case ENoCurrentSource:
		return "no current source; run 'ppz source create <handle>' (or 'ppz source switch <handle>' to point at an existing one)"
	case EPayloadTooLarge:
		return "payload too large; max 64KiB encoded"
	case EServerUnreachable:
		return "server unreachable"
	case ENATSUnreachable:
		return "nats unreachable; if running ppz daemon outside docker, set PPZ_NATS_URL=nats://localhost:4222 before 'ppz daemon start'"
	case EInvalidPipe:
		return "invalid pipe; target must be <handle>.<pipe> with pipe ∈ {broadcast, inbox, stdin, stdout}"
	case EPipeTaken:
		return "pipe with this name already exists on this source"
	case EPipeNotFound:
		return "pipe not found on this source"
	case EInvalidSubject:
		return "invalid subject; the 'ack:' prefix is reserved for system-emitted protocol messages"
	}
	return "unknown error"
}

// Error is a wire-level error carrying a Code and an arbitrary message.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// New builds an *Error using the standard Message for the code.
func New(c Code) *Error {
	return &Error{Code: c, Message: Message(c)}
}

// Constructors that thread the offending entity name into the message —
// "source not found" is useless without saying which source. Prefer these
// over plain New(...) wherever the caller has the name.

// NewSourceNotFound: source 'foo' not found
func NewSourceNotFound(handle string) *Error {
	return &Error{Code: ESourceNotFound, Message: fmt.Sprintf("source '%s' not found", handle)}
}

// NewSourceTaken: source 'foo' already exists in this org
func NewSourceTaken(handle string) *Error {
	return &Error{Code: ESourceTaken, Message: fmt.Sprintf("source '%s' already exists in this org", handle)}
}

// NewPipeTaken: pipe 'archive' already exists on source 'foo'
func NewPipeTaken(pipe, handle string) *Error {
	return &Error{Code: EPipeTaken, Message: fmt.Sprintf("pipe '%s' already exists on source '%s'", pipe, handle)}
}

// NewPipeNotFound: pipe 'archive' not found on source 'foo'
func NewPipeNotFound(pipe, handle string) *Error {
	return &Error{Code: EPipeNotFound, Message: fmt.Sprintf("pipe '%s' not found on source '%s'", pipe, handle)}
}

// NewInvalidPipeReserved: pipe name 'system' is reserved
func NewInvalidPipeReserved(name string) *Error {
	return &Error{Code: EInvalidPipe, Message: fmt.Sprintf("pipe name '%s' is reserved", name)}
}

// NewInvalidPipeName: pipe name 'X' is invalid (regex rejection / etc.)
func NewInvalidPipeName(name string) *Error {
	return &Error{Code: EInvalidPipe, Message: fmt.Sprintf("pipe name '%s' is invalid: must match [a-z0-9-] (max 32, no leading/trailing -)", name)}
}

// NewInvalidHandle: invalid handle 'BAD'
func NewInvalidHandle(handle string) *Error {
	return &Error{Code: EInvalidHandle, Message: fmt.Sprintf("invalid handle '%s': must match [a-z0-9-] (max 32, no leading/trailing -, not reserved)", handle)}
}

// HTTPStatus returns the HTTP status code for a given Code, or 500 if unknown.
func HTTPStatus(c Code) int {
	switch c {
	case EInvalidAPIKey:
		return 401
	case ESourceTaken:
		return 409
	case ESourceNotFound:
		return 404
	case EInvalidHandle:
		return 400
	case EPayloadTooLarge:
		return 413
	case EServerUnreachable:
		return 502
	case EInvalidPipe:
		return 400
	case EPipeTaken:
		return 409
	case EPipeNotFound:
		return 404
	case EInvalidSubject:
		return 400
	}
	return 500
}
