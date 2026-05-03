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
}

func New(home, sock string) *Daemon {
	return &Daemon{
		Home:    home,
		Sock:    sock,
		State:   NewState(home),
		HTTP:    &http.Client{Timeout: 5 * time.Second},
		Cursors: newCursors(home),
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
