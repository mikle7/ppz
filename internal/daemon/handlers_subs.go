package daemon

import (
	"context"
	"encoding/json"
	"net"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// handleSubsAdd appends targets to the caller's session subscription list.
// Idempotent (see subscriptions.Add). Targets are stored verbatim — a
// dotless target stays an uncollared pipe (read-style); the daemon does no
// `.inbox` sugaring.
func (d *Daemon) handleSubsAdd(_ context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SubsAddRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if err := d.Subs.Add(req.Session, req.Targets...); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeIPC(conn, cliproto.SubsAddReply{Subs: d.Subs.List(req.Session)})
}

// handleSubsRemove drops targets from the session list. Removing an agent's
// OWN inbox — the auto-subscribed <session>.inbox — is guarded: refused
// (E_OWN_INBOX) unless Force, so an agent doesn't accidentally opt out of
// its own monitor. Removing any other subject (including a DIFFERENT
// handle's inbox) is allowed and idempotent.
func (d *Daemon) handleSubsRemove(_ context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SubsRemoveRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	// "self" is defined by the session key: inside `ppz terminal share H`,
	// PPZ_SESSION=H, so H.inbox is the agent's own inbox. From a personal
	// shell (session sid-N), no target is "self".
	ownInbox := session(req.Session) + ".inbox"
	if !req.Force {
		for _, t := range req.Targets {
			if strings.TrimSpace(t) == ownInbox {
				writeIPCErr(conn, &cliproto.Error{
					Code:    "E_OWN_INBOX",
					Message: "refusing to remove own inbox " + ownInbox + " (use --force to override)",
				})
				return
			}
		}
	}
	if err := d.Subs.Remove(req.Session, req.Targets...); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeIPC(conn, cliproto.SubsRemoveReply{Subs: d.Subs.List(req.Session)})
}

// handleSubsList replies with a ListReply scoped to the session's
// subscription set (the source of truth), enriched with live JetStream
// stats where the pipe exists and synthetic zero-rows where it doesn't.
func (d *Daemon) handleSubsList(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SubsListRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	reply, e := d.subsSnapshot(ctx, req.Session)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

// handleSubsWait is `ppz subs wait` — `ls --watch` scoped to the
// subscription set. Level-triggered: returns immediately if any subscribed
// pipe already has unread; otherwise subscribes to the org subject space
// and blocks until a publish on a subscribed subject lands.
//
// The reply carries ONLY the unread row(s) — token-light for an agent
// monitor loop; the full set is `ppz subs ls`.
func (d *Daemon) handleSubsWait(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.SubsWaitRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	subjects := d.Subs.List(req.Session)
	// No subscriptions → nothing can ever wake us; return an empty snapshot
	// immediately rather than block forever.
	if len(subjects) == 0 {
		writeIPC(conn, cliproto.ListReply{})
		return
	}
	accountID, err := uuid.Parse(d.State.AccountID())
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"})
		return
	}
	if err := d.ensureNATS(ctx); err != nil {
		if e, ok := err.(*cliproto.Error); ok {
			writeIPCErr(conn, e)
		} else {
			writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		}
		return
	}

	// Subscribe BEFORE the initial snapshot (same ordering rationale as
	// handleListWatch) so an arrival between snapshot and subscribe can't
	// strand the caller. Wake on any publish matching a subscribed subject;
	// false positives are harmless — the next snapshot decides.
	wakeup := make(chan struct{}, 1)
	sub, err := d.NC.Subscribe(accountID.String()+".>", func(msg *nats.Msg) {
		for _, iv := range parseSubjectInterpretations(msg.Subject) {
			if matchAnyTarget(iv.Handle, iv.Pipe, subjects) {
				select {
				case wakeup <- struct{}{}:
				default:
				}
				return
			}
		}
	})
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	defer sub.Unsubscribe()

	reply, e := d.subsSnapshot(ctx, req.Session)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	if unread := unreadOnly(reply); hasUnread(unread) {
		writeIPC(conn, unread)
		return
	}

	clientGone := make(chan struct{})
	go func() {
		var b [1]byte
		_, _ = conn.Read(b[:])
		close(clientGone)
	}()

	select {
	case <-wakeup:
		reply, e = d.subsSnapshot(ctx, req.Session)
		if e != nil {
			writeIPCErr(conn, e)
			return
		}
		writeIPC(conn, unreadOnly(reply))
	case <-clientGone:
		return
	case <-ctx.Done():
		return
	}
}

// subsSnapshot builds a ListReply for the session's subscription set. It
// reuses buildFilteredList to enrich subscribed pipes that currently exist
// (correct unread / buffered / creator), then appends synthetic zero-rows
// for subscribed subjects with no backing pipe yet (e.g. a room nobody has
// posted to). The subscription list is the source of truth: every
// subscribed subject appears as a row.
func (d *Daemon) subsSnapshot(ctx context.Context, sessionID string) (cliproto.ListReply, *cliproto.Error) {
	subjects := d.Subs.List(sessionID)
	if len(subjects) == 0 {
		return cliproto.ListReply{}, nil
	}
	if _, ok := d.State.Credentials(); !ok {
		return cliproto.ListReply{}, cliproto.New(cliproto.ENotLoggedIn)
	}
	accountID, err := uuid.Parse(d.State.AccountID())
	if err != nil {
		return cliproto.ListReply{}, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"}
	}
	if err := d.ensureNATS(ctx); err != nil {
		if e, ok := err.(*cliproto.Error); ok {
			return cliproto.ListReply{}, e
		}
		return cliproto.ListReply{}, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()}
	}

	reply, e := d.buildFilteredList(ctx, accountID, sessionID, subjects)
	if e != nil {
		return cliproto.ListReply{}, e
	}

	// Which subjects did buildFilteredList already surface (existing pipes)?
	covered := map[string]bool{}
	for _, s := range reply.Sources {
		for _, p := range s.PipeInfos {
			covered[s.Handle+"."+p.Pipe] = true
		}
	}
	for _, u := range reply.UncollaredPipes {
		covered[cliproto.FormatPipePath(u.Manifold, "", u.Name)] = true
	}

	// Append synthetic zero-rows for subscribed-but-nonexistent subjects so
	// `subs ls` shows the full subscription list. Dotted → collared (grouped
	// into the handle's Source row); dotless → uncollared room/lobby.
	for _, subj := range subjects {
		if covered[subj] {
			continue
		}
		if handle, pipe, ok := splitCollared(subj); ok {
			reply.Sources = appendSyntheticPipe(reply.Sources, handle, pipe)
		} else {
			reply.UncollaredPipes = append(reply.UncollaredPipes, cliproto.UncollaredPipe{
				Name: subj,
				Info: cliproto.PipeInfo{Pipe: subj},
			})
		}
	}
	sortListReply(&reply)
	return reply, nil
}

// unreadOnly returns a copy of reply keeping only rows with unread > 0 —
// the token-light payload `subs wait` emits on wake.
func unreadOnly(in cliproto.ListReply) cliproto.ListReply {
	var out cliproto.ListReply
	for _, s := range in.Sources {
		kept := make([]cliproto.PipeInfo, 0, len(s.PipeInfos))
		for _, p := range s.PipeInfos {
			if p.Unread > 0 {
				kept = append(kept, p)
			}
		}
		if len(kept) > 0 {
			s.PipeInfos = kept
			out.Sources = append(out.Sources, s)
		}
	}
	for _, u := range in.UncollaredPipes {
		if u.Info.Unread > 0 {
			out.UncollaredPipes = append(out.UncollaredPipes, u)
		}
	}
	return out
}

// splitCollared mirrors `ppz read` target parsing: a dotted target is
// collared (`<handle>.<pipe>`, split on the last dot); a dotless target is
// an uncollared pipe and returns ok=false.
func splitCollared(subject string) (handle, pipe string, ok bool) {
	idx := strings.LastIndex(subject, ".")
	if idx <= 0 || idx == len(subject)-1 {
		return "", "", false
	}
	return subject[:idx], subject[idx+1:], true
}

// appendSyntheticPipe adds a zero-stat PipeInfo for (handle, pipe), grouping
// it into an existing Source row for that handle when present.
func appendSyntheticPipe(sources []cliproto.Source, handle, pipe string) []cliproto.Source {
	for i := range sources {
		if sources[i].Handle == handle {
			sources[i].PipeInfos = append(sources[i].PipeInfos, cliproto.PipeInfo{Pipe: pipe})
			return sources
		}
	}
	return append(sources, cliproto.Source{
		Handle:    handle,
		PipeInfos: []cliproto.PipeInfo{{Pipe: pipe}},
	})
}

// sortListReply orders the reply deterministically: Sources by handle (each
// source's pipes by name), uncollared by rendered path.
func sortListReply(r *cliproto.ListReply) {
	sort.Slice(r.Sources, func(i, j int) bool { return r.Sources[i].Handle < r.Sources[j].Handle })
	for i := range r.Sources {
		pis := r.Sources[i].PipeInfos
		sort.Slice(pis, func(a, b int) bool { return pis[a].Pipe < pis[b].Pipe })
	}
	sort.Slice(r.UncollaredPipes, func(i, j int) bool {
		return cliproto.FormatPipePath(r.UncollaredPipes[i].Manifold, "", r.UncollaredPipes[i].Name) <
			cliproto.FormatPipePath(r.UncollaredPipes[j].Manifold, "", r.UncollaredPipes[j].Name)
	})
}
