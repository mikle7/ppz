package daemon

// Scheduled sends (docs/specs/schedule.md). The daemon is a thin
// forwarder here: it runs the same target-resolution pre-flight as a
// live send (login check, .inbox sugar / uncollared fallback, source
// and stream existence, sender resolution) and then hands the resolved
// schedule to the server over REST — the durable state and the firing
// loop live server-side, so the schedule outlives this process.

import (
	"context"
	"encoding/json"
	"net"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func (d *Daemon) handleScheduleCreate(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ScheduleCreateRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	target, e := d.resolveSendTarget(ctx, req.Handle, req.Channel, req.BareTarget, req.Session, req.Sender)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	body := cliproto.ScheduleServerCreateRequest{
		Manifold: target.manifold,
		Handle:   target.source,
		Pipe:     target.pipe,
		Payload:  req.Payload,
		Sender:   target.sender,
		Kind:     req.Kind,
		At:       req.At,
		Every:    req.Every,
		Cron:     req.Cron,
		TZ:       req.TZ,
	}
	var reply cliproto.ScheduleCreateReply
	if e := d.callServer(ctx, "POST", "/api/v1/schedules", body, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

func (d *Daemon) handleScheduleList(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ScheduleListRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	var reply cliproto.ScheduleListReply
	if e := d.callServer(ctx, "GET", "/api/v1/schedules", nil, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

func (d *Daemon) handleScheduleRemove(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ScheduleRemoveRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if req.ID == "" {
		writeIPCErr(conn, cliproto.NewScheduleNotFound(req.ID))
		return
	}
	var reply cliproto.ScheduleRemoveReply
	if e := d.callServer(ctx, "DELETE", "/api/v1/schedules/"+req.ID, nil, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}
