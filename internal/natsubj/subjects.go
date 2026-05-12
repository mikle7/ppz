// Package natsubj builds and parses the ppz subject grammar
//
//	<org_id>.<handle>.<pipe>
//
// where {pipe} ∈ {broadcast, inbox, stdin, stdout}. The optional workspace slot
// between org and handle is reserved by the long-term grammar but unused.
package natsubj

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// HandleRegex per WIRE.md §1: lowercase alnum + hyphens, max 32, no leading
// or trailing hyphen.
var HandleRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// Reserved handles that cannot be used as pipe names. They overlap with the
// channel names so a bare-token target stays unambiguous.
var Reserved = map[string]bool{
	"broadcast": true,
	"inbox":     true,
	"stdin":     true,
	"stdout":    true,
	"stdctrl":   true,
	"system":    true,
	"db":        true,
}

// ValidPipes are the pipe names recognised on the wire — both auto-
// provisioned (broadcast / stdin / stdout / stdctrl) and any future user-
// creatable names. The user-creatable paths add additional validation on
// top (regex, reserved-name check).
//
// stdctrl carries control signals that don't fit on stdin/out — initially
// just JSON resize events, later potentially exit codes, title changes,
// focus events, etc. The web terminal viewer consumes it to keep its
// xterm.js viewport sized to match the source pty.
var ValidPipes = map[string]bool{
	"broadcast": true,
	"inbox":     true,
	"stdin":     true,
	"stdout":    true,
	"stdctrl":   true,
}

// AutoProvisionedPipes covers pipes the server creates automatically for
// sources. Some are also allowed via user pipe creation (`stdin`, `stdout`,
// `stdctrl` for terminal sharing); `inbox` remains reserved from manual
// creation even though it is provisioned automatically. The set is here for
// documentation + the daemon's ls dedupe.
var AutoProvisionedPipes = map[string]bool{
	"broadcast": true,
	"inbox":     true,
	"stdin":     true,
	"stdout":    true,
	"stdctrl":   true,
}

// ReservedPipeNames are names blocked from user pipe creation. Reserved
// for future system use (e.g. an inbox routing scheme, db-backed pipes).
var ReservedPipeNames = map[string]bool{
	"system": true,
	"db":     true,
	"inbox":  true,
}

// ValidChannels is a deprecated alias kept during the Phase A rename for any
// caller still phrasing the check as "channel". Will be removed in Phase B.
var ValidChannels = ValidPipes

func ValidateHandle(h string) error {
	if !HandleRegex.MatchString(h) {
		return errors.New("invalid handle")
	}
	if Reserved[h] {
		return errors.New("reserved handle")
	}
	return nil
}

// ValidatePipe checks a pipe name's *syntax* — regex only, no auto/reserved
// restrictions. Used by `read` / `send` / `broadcast` targets, where any
// existing pipe is a legitimate destination (auto-provisioned or user-
// created). The "does this stream actually exist" check is deferred to
// the publish/read path against JetStream.
//
// User pipe creation goes through ValidateUserPipeName, which adds the
// auto/reserved restrictions on top of the regex.
func ValidatePipe(name string) error {
	if !HandleRegex.MatchString(name) {
		return errors.New("invalid pipe")
	}
	return nil
}

// ValidateUserPipeName validates a name passed to `ppz pipe create`.
// Same handle regex (lowercase alnum + hyphens, max 32, no leading/trailing
// hyphen). Reserved names are rejected. Auto-provisioned names (broadcast,
// stdin, stdout) ARE allowed: any source can have arbitrary pipes added
// to it, and the pipe-create path is idempotent against an already-existing
// stream.
func ValidateUserPipeName(name string) error {
	if !HandleRegex.MatchString(name) {
		return errors.New("invalid pipe name")
	}
	if ReservedPipeNames[name] {
		return errors.New("name is reserved")
	}
	return nil
}

// ValidateChannel is the Phase A backward-compat alias for ValidatePipe.
// Removed in Phase B.
func ValidateChannel(c string) error { return ValidatePipe(c) }

// Subject builds <org>.<handle>.<pipe>.
func Subject(accountID uuid.UUID, handle, pipe string) string {
	return fmt.Sprintf("%s.%s.%s", accountID.String(), handle, pipe)
}

// Broadcast is the canonical helper for the broadcast channel — preserved
// for callers that don't think in terms of channels yet.
func Broadcast(accountID uuid.UUID, handle string) string {
	return Subject(accountID, handle, "broadcast")
}

// StreamName produces the JetStream stream name per WIRE.md §2:
//
//	source_<orgshort>_<handle>_<pipe>
//
// where orgshort is the first 8 hex chars of the org UUID, hyphens stripped.
func StreamName(accountID uuid.UUID, handle, pipe string) string {
	hex := strings.ReplaceAll(accountID.String(), "-", "")
	if len(hex) > 8 {
		hex = hex[:8]
	}
	return "source_" + hex + "_" + handle + "_" + pipe
}

// OrgSubscription is the wildcard subscription used by the server-side
// subscriber and by the daemon's NATS user JWT permission set.
func OrgSubscription(accountID uuid.UUID) string {
	return accountID.String() + ".>"
}
