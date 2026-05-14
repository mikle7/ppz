package server

import (
	"errors"
	"net/http"

	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// handleGUIPipePage renders every buffered message on a (source, pipe) pair.
// Reads from the JetStream stream backing the pipe; messages are returned in
// chronological order (sequence ASC).
//
// Route: GET /accounts/{id}/sources/{handle}/pipes/{pipe}
// {id} accepts either UUID or slug via resolveOrg.
func (s *Server) handleGUIPipePage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r)
	defer cancel()

	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", 404)
		return
	}
	handle := r.PathValue("handle")
	pipeName := r.PathValue("pipe")
	if err := natsubj.ValidatePipe(pipeName); err != nil {
		http.Error(w, "unknown pipe", 404)
		return
	}

	// Verify the source exists in this org. Without this we'd happily render
	// streams from any org if you guessed the handle.
	src, err := db.GetSourceByHandle(ctx, s.Pool, org.ID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "source not found", 404)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	subject := natsubj.BuildSubject(org.ID, src.Manifold, src.Handle, pipeName)
	streamName := natsubj.BuildStreamName(org.ID, src.Manifold, src.Handle, pipeName)

	type messageView struct {
		ID        string
		CreatedAt string
		Payload   string
	}
	var messages []messageView

	js, _ := s.JSFor(ctx, org.ID)
	if js == nil {
		// Org account not provisioned yet (just-created org, no
		// streams yet). Render the page with no buffered messages.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.ExecuteTemplate(w, "channel.html", map[string]any{
			"Org": org, "Source": src, "Pipe": pipeName, "Subject": subject, "Messages": messages,
		})
		return
	}
	stream, err := js.Stream(ctx, streamName)
	if err == nil {
		info, err := stream.Info(ctx)
		if err == nil && info.State.Msgs > 0 {
			for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
				msg, err := stream.GetMsg(ctx, seq)
				if err != nil {
					// Sequence gap (expired by MaxAge / dropped by limits).
					continue
				}
				env, err := envelope.Unmarshal(msg.Data)
				if err != nil {
					continue
				}
				messages = append(messages, messageView{
					ID:        env.ID,
					CreatedAt: env.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
					Payload:   env.Payload,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "channel.html", map[string]any{
		"Org":      org,
		"Handle":   src.Handle,
		"Pipe":     pipeName,
		"Subject":  subject,
		"Messages": messages,
	}); err != nil {
		http.Error(w, err.Error(), 500)
	}
}
