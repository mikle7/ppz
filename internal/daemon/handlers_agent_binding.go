package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// handleRegisterAgentBinding records the calling `ppz terminal share`
// process as the identity anchor for its handle. CLI sends
// {Handle, SharePID}; the daemon trusts the SharePID (local trust
// boundary — see docs/specs/session-binding.md §Non-goals).
func (d *Daemon) handleRegisterAgentBinding(_ context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.RegisterAgentBindingRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if req.Handle == "" || req.SharePID <= 0 {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: "RegisterAgentBinding: handle and share_pid required"})
		return
	}
	b, err := d.State.RegisterAgentBinding(req.Handle, req.SharePID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[debug] RegisterAgentBinding handle=%s pid=%d FAILED: %v\n", req.Handle, req.SharePID, err)
		if errors.Is(err, ErrBindingConflict) {
			writeIPCErr(conn, &cliproto.Error{Code: cliproto.EBindingConflict, Message: err.Error()})
			return
		}
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	fmt.Fprintf(os.Stderr, "[debug] RegisterAgentBinding handle=%s pid=%d sessionKey=%s OK\n", b.Handle, b.SharePID, b.SessionKey)
	writeIPC(conn, cliproto.RegisterAgentBindingReply{
		Handle:       b.Handle,
		SharePID:     b.SharePID,
		SessionKey:   b.SessionKey,
		RegisteredAt: b.RegisteredAt,
	})
}

// handleUnregisterAgentBinding drops the binding for the given share pid.
// Idempotent — unknown pid is success.
func (d *Daemon) handleUnregisterAgentBinding(_ context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.UnregisterAgentBindingRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if req.SharePID > 0 {
		d.State.UnregisterAgentBinding(req.SharePID)
	}
	writeIPC(conn, cliproto.UnregisterAgentBindingReply{})
}
