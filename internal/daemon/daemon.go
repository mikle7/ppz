package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

// Daemon is the singleton process per $PPZ_HOME.
type Daemon struct {
	Home    string
	Sock    string
	State   *State
	HTTP    *http.Client
	NC      *nats.Conn // nil until Login
	NATSURL string
	Cursors *cursors

	// Phase 3.5 — JWT refresh loop. Started on Login (and restored
	// in ensureNATS for daemon restarts), holds the latest minted
	// (jwt, seed) for the current org, and re-runs /auth/exchange
	// at exp-30s. nats.Connect's UserJWT callback reads from
	// Refresh.Current() so reconnects pick up fresh credentials.
	// Stopped on Logout / replaced on Login.
	Refresh *RefreshLoop

	// Phase 0 (agent hardening) — short tail of NATS connection-state
	// transitions. Surfaced by `ppz status` (latest state) and
	// `ppz diagnostics` (full ring). Initialised in New() so the handlers
	// registered on the very first nats.Connect have a non-nil ring
	// to append to.
	NATSEvents *NATSEventRing

	// Heartbeats holds the most recent <handle>.heartbeat payload per
	// source, populated by handleSend / handleSendBatch on the fly.
	// Read by `ppz who`. Memory-only — cleared on daemon restart.
	Heartbeats *HeartbeatCache

	// Follows tracks live IPC conns that handleRead is streaming
	// JetStream events on (Follow: true). Used by swapNC to evict
	// stale follows when the NATS connection is replaced — the old
	// consumers go silent and the CLI needs to redial against the
	// new NC. See follow_registry.go.
	Follows *followRegistry
}

func New(home, sock string) *Daemon {
	return &Daemon{
		Home:       home,
		Sock:       sock,
		State:      NewState(home),
		HTTP:       &http.Client{Timeout: 5 * time.Second},
		Cursors:    newCursors(home),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Heartbeats: NewHeartbeatCache(),
		Follows:    newFollowRegistry(),
	}
}

// swapNC replaces d.NC, first evicting every live follow conn so the
// CLI redials against the fresh NATS connection. Centralizing this
// pattern guarantees that watcher / handleLogin / ensureNATS all
// invalidate stale follows identically; any future NC-replacement
// path picks up the same behaviour for free.
//
// `newNC` may be nil — used by the watcher when credentials disappear
// and there's no replacement to install. Callers are responsible for
// any nats.Conn lifecycle (connectNATSWithRefresh ownership) outside
// of swapping the pointer.
//
// caller is the originating function name ("ensureNATS",
// "OnRefreshed-callback", "handleLogin", ...), recorded on the
// emitted "swap" event so readers can attribute every NC transition.
// The swap event captures both the old and new NC IDs in Reason so a
// single line tells the full story; the inevitable nats.go-driven
// disconnect+closed pair (from closing the old NC) lands separately
// with Caller="nats.go" — that's the noise the burst-swap-storm
// pattern detects.
func (d *Daemon) swapNC(caller string, newNC *nats.Conn) {
	oldID := ncID(d.NC)
	newID := ncID(newNC)
	if d.NATSEvents != nil {
		d.recordNATSEvent(NATSEvent{
			Type:   "swap",
			At:     time.Now(),
			Caller: caller,
			NCID:   newID,
			JWTExp: d.Refresh.JWTExp(),
			Reason: "old=" + oldID + " new=" + newID,
		})
	}
	if d.Follows != nil {
		d.Follows.closeAll()
	}
	if d.NC != nil && d.NC != newNC {
		d.NC.Close()
	}
	d.NC = newNC
}

// recordNATSEvent is the single sink for connection-state events: it
// stamps the in-memory ring (hot tail for `ppz status` /
// `ppz diagnostics`) AND appends to the on-disk jsonl (full history,
// surviving restarts and burst-rate aging). Failures on the disk path
// are silent — observability is best-effort.
func (d *Daemon) recordNATSEvent(ev NATSEvent) {
	if d.NATSEvents != nil {
		d.NATSEvents.Append(ev)
	}
	if d.Home != "" {
		_ = appendNATSEventLog(d.Home, ev)
	}
}

// Run runs the daemon in the foreground. Returns when ctx is cancelled or
// the listener fails.
func (d *Daemon) Run(ctx context.Context) error {
	if err := os.MkdirAll(d.Home, 0o700); err != nil {
		return fmt.Errorf("mkdir home: %w", err)
	}
	// PID file (also used by reset.sh for SIGHUP).
	if err := os.WriteFile(filepath.Join(d.Home, filePID), []byte(fmt.Sprint(os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer os.Remove(filepath.Join(d.Home, filePID))

	// Diagnostics ring: prime from BOTH the on-disk lifecycle log (so
	// the previous daemon's stop event is observable here) AND the
	// connection-events tail (so a fresh process's `ppz diagnostics`
	// shows the burst that prompted the operator to restart it).
	// Then record our own start; the stop counterpart fires on
	// shutdown below.
	if d.NATSEvents != nil {
		for _, ev := range loadLifecycleLog(d.Home) {
			d.NATSEvents.Append(ev)
		}
	}
	d.loadNATSEventLogTail()
	d.recordDaemonLifecycle("daemon_start", "")
	defer d.recordDaemonLifecycle("daemon_stop", "graceful")

	if err := d.State.LoadFromDisk(); err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// State reload: file-mtime poller + SIGHUP. The poller is what makes
	// out-of-band resets reliable — the test harness runs in a different
	// container (different PID namespace) so kill -HUP can't reach this
	// process. SIGHUP is still honoured for local dev.
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)
	go d.watchState(ctx, hupCh)

	// Remove any stale socket file from a prior crash.
	_ = os.Remove(d.Sock)
	ln, err := net.Listen("unix", d.Sock)
	if err != nil {
		return fmt.Errorf("listen %s: %w", d.Sock, err)
	}
	defer ln.Close()
	defer os.Remove(d.Sock)
	if err := os.Chmod(d.Sock, 0o600); err != nil {
		return fmt.Errorf("chmod sock: %w", err)
	}

	go d.serveIPC(ctx, ln)

	<-ctx.Done()
	return nil
}

// PIDFromHome reads $PPZ_HOME/daemon.pid. Returns 0 if absent.
func PIDFromHome(home string) int {
	data, err := os.ReadFile(filepath.Join(home, filePID))
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid
}

// IsAlive returns true if the PID-bearing process exists.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

var ErrCredsRequired = errors.New("credentials required")
