package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// handleListWatch is `ppz ls --watch [pattern...]`. Level-triggered: if
// the calling session already has unread on any pipe whose handle
// matches one of the patterns, it returns immediately. Otherwise it
// subscribes to the org's NATS subject space, blocks until a matching
// publish lands, then re-snapshots and returns.
//
// Patterns are filepath.Match-style globs against the handle (not
// handle.pipe). Empty Patterns slice means "any handle".
//
// Single response, not a stream. The CLI prints once and exits — same
// agent-loop pattern as `read` (without `--tail`).
func (d *Daemon) handleListWatch(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.ListWatchRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if _, ok := d.State.Credentials(); !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	orgID, err := uuid.Parse(d.State.OrgID())
	if err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: "bad org id"})
		return
	}
	if err := d.ensureNATS(ctx); err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}

	// Subscribe BEFORE the initial snapshot so a message landing between
	// "build snapshot" and "subscribe" can't make us hang on a state we
	// just observed as caught-up. NATS sub buffers in the client; we
	// drain into a 1-slot wakeup channel which is enough for "tell me
	// when something happened" semantics.
	wakeup := make(chan struct{}, 1)
	sub, err := d.NC.Subscribe(orgID.String()+".>", func(msg *nats.Msg) {
		// Subjects are "<orgID>.<handle>.<pipe>" — parse both, match
		// against the full target so `*.stdout` patterns wake correctly
		// (and not just on traffic to handles named *.stdout, which
		// don't exist).
		parts := strings.SplitN(msg.Subject, ".", 3)
		if len(parts) != 3 {
			return
		}
		// parts[0] = orgID (already filtered by subscribe), parts[1] is
		// the handle's first dot-bounded segment, parts[2] is the pipe
		// name. UUIDs contain hyphens not dots so this split is safe.
		handle, pipe := parts[1], parts[2]
		if !matchAnyTarget(handle, pipe, req.Patterns) {
			return
		}
		select {
		case wakeup <- struct{}{}:
		default:
		}
	})
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.ENATSUnreachable))
		return
	}
	defer sub.Unsubscribe()

	// Initial snapshot.
	reply, e := d.buildFilteredList(ctx, orgID, req.Session, req.Patterns)
	if e != nil {
		writeIPCErr(conn, e)
		return
	}
	if hasUnread(reply.Sources) {
		writeIPC(conn, reply)
		return
	}

	// Caught up — block until something matching arrives, or the client
	// gives up. CLI never writes after the initial request, so a Read
	// returning means the socket closed.
	clientGone := make(chan struct{})
	go func() {
		var b [1]byte
		_, _ = conn.Read(b[:])
		close(clientGone)
	}()

	select {
	case <-wakeup:
		reply, e = d.buildFilteredList(ctx, orgID, req.Session, req.Patterns)
		if e != nil {
			writeIPCErr(conn, e)
			return
		}
		writeIPC(conn, reply)
	case <-clientGone:
		return
	case <-ctx.Done():
		return
	}
}

// buildFilteredList does the same per-pipe enumeration as handleList,
// but filters sources by the patterns. Returns a ListReply whose Sources
// only include matching handles.
func (d *Daemon) buildFilteredList(ctx context.Context, orgID uuid.UUID, session string, patterns []string) (cliproto.ListReply, *cliproto.Error) {
	var lr cliproto.ListSourcesReply
	if e := d.callServer(ctx, "GET", "/api/v1/sources", nil, &lr); e != nil {
		return cliproto.ListReply{}, e
	}
	js, err := jetstream.New(d.NC)
	if err != nil {
		return cliproto.ListReply{}, cliproto.New(cliproto.ENATSUnreachable)
	}

	enriched := make([]cliproto.Source, 0)
	for _, s := range lr.Sources {
		// Same union-of-pipes logic as handleList: kind-derived
		// auto-provisioned + user-created.
		pipeSet := map[string]struct{}{}
		for _, p := range pipesForKind(s.Kind) {
			pipeSet[p] = struct{}{}
		}
		for _, p := range s.Pipes {
			pipeSet[p] = struct{}{}
		}
		pipes := make([]string, 0, len(pipeSet))
		for p := range pipeSet {
			pipes = append(pipes, p)
		}
		sort.Strings(pipes)
		infos := make([]cliproto.PipeInfo, 0, len(pipes))
		for _, p := range pipes {
			// Per-pipe filter: pattern can match handle alone (back-compat
			// with `ls --watch agent-*`) or the full `<handle>.<pipe>`
			// target (lets `*.stdout` filter to stdout pipes only).
			if !matchAnyTarget(s.Handle, p, patterns) {
				continue
			}
			info := cliproto.PipeInfo{Pipe: p}
			streamName := natsubj.StreamName(orgID, s.Handle, p)
			if stream, err := js.Stream(ctx, streamName); err == nil {
				if si, err := stream.Info(ctx); err == nil {
					info.Total = si.State.Msgs
					info.LastSeq = si.State.LastSeq
					if !si.State.LastTime.IsZero() {
						lt := si.State.LastTime.UTC()
						info.LastAt = &lt
					}
					cursor := d.Cursors.Get(session, daemonCursorKey(orgID, s.Handle, p))
					if info.LastSeq > cursor {
						info.Unread = info.LastSeq - cursor
					}
					if info.LastSeq > 0 {
						if msg, err := stream.GetMsg(ctx, info.LastSeq); err == nil {
							if env, err := envelope.Unmarshal(msg.Data); err == nil {
								info.Preview = cliproto.TruncatePayload(env.Payload)
								info.Payload = env.Payload
							}
						}
					}
				}
			}
			infos = append(infos, info)
		}
		// Skip the source entirely if none of its pipes survived the
		// filter — keeps the table from showing handle headers with
		// no pipe rows.
		if len(infos) == 0 {
			continue
		}
		s.PipeInfos = infos
		enriched = append(enriched, s)
	}
	return cliproto.ListReply{Sources: enriched}, nil
}

// matchAnyTarget returns true if any pattern matches the (handle,pipe)
// row. A pattern matches when filepath.Match succeeds against EITHER
// the handle alone (back-compat: `ls --watch agent-*` returns all pipes
// of every agent-* handle) OR the full `<handle>.<pipe>` target (lets
// `*.stdout` filter to stdout pipes only). Empty patterns slice means
// "match anything".
//
// Accepts both `*` (standard glob, requires shell-quoting in zsh) and
// `%` (SQL-LIKE-style alias, passes through unquoted in zsh/bash). The
// SQL alias is translated to `*` before delegation to filepath.Match,
// so `agent-*` and `agent-%` are interchangeable.
func matchAnyTarget(handle, pipe string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	target := handle + "." + pipe
	for _, raw := range patterns {
		p := strings.ReplaceAll(raw, "%", "*")
		if ok, _ := filepath.Match(p, handle); ok {
			return true
		}
		if ok, _ := filepath.Match(p, target); ok {
			return true
		}
	}
	return false
}

// hasUnread reports whether any pipe in any source has unread > 0 — the
// "should I wake up the watcher?" predicate.
func hasUnread(sources []cliproto.Source) bool {
	for _, s := range sources {
		for _, p := range s.PipeInfos {
			if p.Unread > 0 {
				return true
			}
		}
	}
	return false
}
