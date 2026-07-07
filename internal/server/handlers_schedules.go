package server

// Scheduled sends (docs/specs/schedule.md) — the REST surface backing
// `ppz send --at/--every/--cron` and `ppz schedule {ls|rm}`:
//
//	POST   /api/v1/schedules        create (daemon forwards the resolved target)
//	GET    /api/v1/schedules        list the org's live schedules
//	DELETE /api/v1/schedules/{id}   remove by short id
//
// The daemon already ran send-grade target resolution (source/stream
// existence); the server re-validates shape (names, kind, spec) since
// it is the trust boundary for durable rows.

import (
	"net/http"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
	"github.com/pipescloud/ppz/internal/schedule"
)

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	var req cliproto.ScheduleServerCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "malformed json"})
		return
	}
	if err := natsubj.ValidatePipe(req.Pipe); err != nil {
		writeErr(w, cliproto.New(cliproto.EInvalidPipe))
		return
	}
	// Size gate: the fired envelope must obey the same cap as a live
	// send — reject at creation, not at fire time when nobody's looking.
	now := time.Now().UTC()
	probe := envelope.New(req.Sender, "", req.Payload, now)
	if data, err := probe.Marshal(); err != nil || len(data) > envelope.MaxBytes {
		writeErr(w, cliproto.New(cliproto.EPayloadTooLarge))
		return
	}

	row := db.Schedule{
		AccountID:       key.AccountID,
		Manifold:        req.Manifold,
		SourceHandle:    req.Handle,
		Pipe:            req.Pipe,
		Payload:         req.Payload,
		Sender:          req.Sender,
		Kind:            req.Kind,
		TZ:              req.TZ,
		CreatedByUserID: key.CreatedByUserID,
		CreatedAt:       now,
	}
	switch schedule.Kind(req.Kind) {
	case schedule.KindAt:
		t, err := time.Parse(time.RFC3339, req.At)
		if err != nil {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "invalid at: " + err.Error()})
			return
		}
		if !t.After(now) {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "at is in the past"})
			return
		}
		row.Spec = req.At // display-shaped: the creator's offset survives
		row.NextFireAt = t.UTC()
	case schedule.KindEvery:
		d, err := schedule.ParseEvery(req.Every)
		if err != nil {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "invalid every: " + err.Error()})
			return
		}
		row.Spec = req.Every
		row.NextFireAt = now.Add(d) // grid anchor = created_at
	case schedule.KindCron:
		if err := schedule.ParseCron(req.Cron); err != nil {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "invalid cron: " + err.Error()})
			return
		}
		loc, err := time.LoadLocation(req.TZ)
		if err != nil {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "invalid tz: " + req.TZ})
			return
		}
		next, ok := schedule.NextAfter(schedule.Spec{Kind: schedule.KindCron, Cron: req.Cron, Loc: loc}, now)
		if !ok {
			writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "cron expression never fires"})
			return
		}
		row.Spec = req.Cron
		row.NextFireAt = next.UTC()
	default:
		writeErr(w, &cliproto.Error{Code: cliproto.EInvalidSchedule, Message: "kind must be at, every, or cron"})
		return
	}

	ctx, cancel := withTimeout(r)
	defer cancel()
	ins, err := db.InsertSchedule(ctx, s.Pool, row)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cliproto.ScheduleCreateReply{
		ID:     ins.ShortID(),
		Target: cliproto.FormatPipePath(ins.Manifold, ins.SourceHandle, ins.Pipe),
		NextAt: ins.NextFireAt,
	})
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	ctx, cancel := withTimeout(r)
	defer cancel()
	rows, err := db.ListSchedules(ctx, s.Pool, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	reply := cliproto.ScheduleListReply{Schedules: make([]cliproto.ScheduleInfo, 0, len(rows))}
	for _, row := range rows {
		reply.Schedules = append(reply.Schedules, cliproto.ScheduleInfo{
			ID:        row.ShortID(),
			Namespace: row.Manifold,
			Handle:    row.SourceHandle,
			Pipe:      row.Pipe,
			Kind:      row.Kind,
			Spec:      row.Spec,
			TZ:        row.TZ,
			NextAt:    row.NextFireAt,
			LastAt:    row.LastFiredAt,
			Payload:   row.Payload,
			Creator:   row.CreatorUsername,
		})
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	id := strings.ToLower(r.PathValue("id"))
	ctx, cancel := withTimeout(r)
	defer cancel()
	ok, err := db.DeleteScheduleByShortID(ctx, s.Pool, key.AccountID, id)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	if !ok {
		writeErr(w, cliproto.NewScheduleNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, cliproto.ScheduleRemoveReply{ID: id})
}
