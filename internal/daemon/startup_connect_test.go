package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// waitNCConnected polls d.NC (under ncMu) until it is connected or the
// deadline passes. Shared by the startup-connect and closed-reconnect
// tests, both of which assert on a connection that a BACKGROUND loop
// brings up — so they must poll rather than read once.
func waitNCConnected(d *Daemon, dur time.Duration) bool {
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		d.ncMu.Lock()
		ok := d.NC != nil && d.NC.IsConnected()
		d.ncMu.Unlock()
		if ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestConnectOnStartup_ConnectsLoggedInDaemon — RED for the cold-start
// gap reported after the v0.48.1 upgrade. A freshly-(re)started daemon
// that is already logged in sat at `nats: unknown` and received no
// pushed messages until the first IPC command (`ppz ls`) happened to
// call ensureNATS — the diagnostics showed the only `connect` event
// arriving 38s after daemon_start, exactly when `ls` ran. Run() must
// bring the connection up itself.
//
// Contract: connectOnStartup, given stored credentials, establishes the
// NATS connection in the background with NO IPC command.
func TestConnectOnStartup_ConnectsLoggedInDaemon(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			return nats.Connect(u)
		},
	}
	loginForWakeTests(t, d)
	d.Refresh = &RefreshLoop{
		AccountID: "00000000-0000-0000-0000-000000000001",
		Refresh: func(context.Context, string) (string, string, int64, error) {
			return "jwt", "seed", time.Now().Add(5 * time.Minute).Unix(), nil
		},
	}
	if err := d.Refresh.Start(context.Background(), "jwt", "seed", time.Now().Add(5*time.Minute).Unix()); err != nil {
		t.Fatalf("RefreshLoop.Start: %v", err)
	}
	t.Cleanup(d.Refresh.Stop)

	d.connectOnStartup(context.Background())

	if !waitNCConnected(d, 3*time.Second) {
		t.Fatalf("connectOnStartup did not establish a NATS connection without an IPC command")
	}
	t.Cleanup(func() {
		if d.NC != nil {
			d.NC.Close()
		}
	})
}

// TestConnectOnStartup_NoopWhenLoggedOut — a daemon started with no
// stored credentials must not dial NATS and must not spin: connect-on-
// startup is gated on being logged in, the same as kickReconnect.
func TestConnectOnStartup_NoopWhenLoggedOut(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	dialed := make(chan struct{}, 1)
	d := &Daemon{
		State:      NewState(t.TempDir()), // logged out — no SetLogin
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		NATSURL:    url,
		dial: func(u string, _ *RefreshLoop, _ func(NATSEvent)) (*nats.Conn, error) {
			select {
			case dialed <- struct{}{}:
			default:
			}
			return nats.Connect(u)
		},
	}

	d.connectOnStartup(context.Background())

	if waitNCConnected(d, 300*time.Millisecond) {
		t.Fatalf("connectOnStartup connected a logged-out daemon")
	}
	select {
	case <-dialed:
		t.Fatalf("connectOnStartup dialed NATS while logged out")
	default:
	}
}
