// Package desktop is the headless / dump-state half of ppz-desktop. The Wails
// windowed app would consume the same state via DumpState; the windowed
// frontend lands post-Phase-1.
package desktop

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// DumpState writes a single JSON snapshot of the daemon's source view (per
// WIRE.md §8a) and returns. Sorted by handle ASC.
func DumpState(ipcSock string, w io.Writer) error {
	var st cliproto.StatusReply
	if err := daemon.Call(ipcSock, cliproto.IPCStatus, struct{}{}, &st); err != nil {
		return err
	}

	out := snapshot{
		LoggedIn: st.LoggedIn,
		AccountID:    nilStringIfEmpty(st.AccountID),
		Sources:  []sourceView{},
	}

	if st.LoggedIn {
		var lr cliproto.ListReply
		if err := daemon.Call(ipcSock, cliproto.IPCList, struct{}{}, &lr); err != nil {
			return err
		}
		for _, src := range lr.Sources {
			out.Sources = append(out.Sources, sourceView{
				Handle:               src.Handle,
				LastBroadcastAt:      formatTime(src),
				LastBroadcastPayload: src.LastBroadcastPayload,
			})
		}
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// RunHeadless: stays alive until SIGINT/SIGTERM. When httpAddr is non-empty,
// also serves the embedded HTML viewer at /  and the JSON snapshot at
// /api/state — that's what gives the user a browser-accessible "desktop GUI"
// in docker (where a real Wails window can't be drawn without X11
// forwarding).
func RunHeadless(ipcSock, httpAddr string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Touch the daemon at startup just to surface obvious misconfiguration.
	var st cliproto.StatusReply
	_ = daemon.Call(ipcSock, cliproto.IPCStatus, struct{}{}, &st)

	if httpAddr != "" {
		go func() {
			if err := ServeHTTP(httpAddr, ipcSock); err != nil {
				log.Printf("ppz-desktop http: %v", err)
			}
		}()
	}

	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			_ = daemon.Call(ipcSock, cliproto.IPCStatus, struct{}{}, &st)
		}
	}
}

type snapshot struct {
	LoggedIn bool         `json:"logged_in"`
	AccountID    *string      `json:"account_id"`
	Sources  []sourceView `json:"sources"`
}

type sourceView struct {
	Handle               string  `json:"handle"`
	LastBroadcastAt      *string `json:"last_broadcast_at"`
	LastBroadcastPayload *string `json:"last_broadcast_payload"`
}

func nilStringIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatTime(s cliproto.Source) *string {
	if s.LastBroadcastAt == nil {
		return nil
	}
	str := s.LastBroadcastAt.UTC().Format("2006-01-02T15:04:05Z")
	return &str
}

// Suppress unused warnings on os import path until anything new uses it.
var _ = os.Getenv
