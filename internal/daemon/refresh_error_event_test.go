package daemon

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestRefreshFailureRecordsEventWithCause — RED for the 2026-06-11
// wake-from-sleep incident (ppz-diag-20260611-073803.tgz).
//
// During that incident the daemon's JWT expired mid-sleep, and for
// ~70 seconds after wake every refresh attempt — the background loop's
// retries AND the RefreshNowIfDue calls made by each `ppz ls` — failed
// against POST /api/v1/auth/exchange. The user saw only
// E_SERVER_UNREACHABLE; the diagnostics bundle contained NOTHING about
// those failures because ensureNATS discards the underlying error
// (handlers.go maps every non-401 refresh failure to EServerUnreachable)
// and the refresh loop doesn't record failures at all. The single most
// useful fact — WHY /auth/exchange failed (DNS? dial timeout? TLS?) —
// was unobservable.
//
// Contract pinned here: a failed refresh attempt must land in the
// daemon's NATS event stream (ring + jsonl, hence `ppz diagnostics`
// and future bundles) as a "refresh_error" event whose Reason carries
// the underlying error text.
func TestRefreshFailureRecordsEventWithCause(t *testing.T) {
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		// 127.0.0.1:1 refuses connections immediately, so the refresh
		// fails fast with a real *net.OpError — the same shape as the
		// incident's post-wake failures.
		HTTP: &http.Client{Timeout: 2 * time.Second},
	}
	creds := Credentials{
		URL:          "http://127.0.0.1:1",
		APIKey:       "pz_test_refresh_error",
		AccountID:    "00000000-0000-0000-0000-000000000001",
		NATSUserJWT:  "stale-jwt",
		NATSUserSeed: "stale-seed",
	}
	if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}

	// Expired JWT → RefreshNowIfDue is due immediately, exactly like the
	// first `ppz ls` after wake.
	d.startRefreshLoop(creds.AccountID, creds.NATSUserJWT, creds.NATSUserSeed,
		time.Now().Add(-time.Minute).Unix())
	t.Cleanup(d.Refresh.Stop)

	due, err := d.Refresh.RefreshNowIfDue(context.Background(), time.Now())
	if due {
		t.Fatalf("RefreshNowIfDue reported success against an unreachable server")
	}
	if err == nil {
		t.Fatalf("RefreshNowIfDue: expected a transport error, got nil")
	}

	var found *NATSEvent
	for _, ev := range d.NATSEvents.Snapshot() {
		if ev.Type == "refresh_error" {
			ev := ev
			found = &ev
		}
	}
	if found == nil {
		t.Fatalf("no refresh_error event recorded after a failed refresh; events: %+v",
			d.NATSEvents.Snapshot())
	}
	if found.Reason == "" {
		t.Fatalf("refresh_error event recorded with empty Reason — the underlying error must be preserved, got: %+v", *found)
	}
}

// TestRefreshErrorEventsCoalesceRepeats — during a sustained outage the
// background recovery loops (kickReconnect every 2-15s, onWake every
// 5s, the refresh loop's own 5s retry) each drive refresh attempts
// whose failures are identical. Recording every one would flood the
// bounded ring (natsEventRingCap) and evict the events that diagnose
// the outage's START — the exact evidence fix #1 exists to preserve.
// Contract: an identical consecutive reason records at most once per
// coalesce window; a CHANGED reason always records (a DNS failure
// turning into a TLS failure is signal, not spam).
func TestRefreshErrorEventsCoalesceRepeats(t *testing.T) {
	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		HTTP:       &http.Client{Timeout: 2 * time.Second},
	}
	creds := Credentials{
		URL:          "http://127.0.0.1:1",
		APIKey:       "pz_test_refresh_coalesce",
		AccountID:    "00000000-0000-0000-0000-000000000001",
		NATSUserJWT:  "stale-jwt",
		NATSUserSeed: "stale-seed",
	}
	if err := d.State.SetLogin(creds, creds.AccountID, "alpha", "pz_test"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}
	d.startRefreshLoop(creds.AccountID, creds.NATSUserJWT, creds.NATSUserSeed,
		time.Now().Add(-time.Minute).Unix())
	t.Cleanup(d.Refresh.Stop)

	// Three back-to-back failures with an identical cause (connection
	// refused against the same URL) — the sustained-outage retry shape.
	for i := 0; i < 3; i++ {
		if _, err := d.Refresh.RefreshNowIfDue(context.Background(), time.Now()); err == nil {
			t.Fatalf("RefreshNowIfDue #%d: expected a transport error, got nil", i+1)
		}
	}

	var count int
	for _, ev := range d.NATSEvents.Snapshot() {
		if ev.Type == "refresh_error" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d refresh_error events for 3 identical consecutive failures, want 1 (coalesced)", count)
	}
}
