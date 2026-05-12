package daemon

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRefreshTimer_FiresBeforeExp: given a JWT that expires in ~1s,
// the refresh goroutine calls RefreshFn before that deadline (the
// 30s skew buffer documented in §Phase 3.5 means in production
// refresh fires at exp-30s; tests use shorter windows).
func TestRefreshTimer_FiresBeforeExp(t *testing.T) {
	var calls int32
	exchanged := make(chan struct{}, 1)

	fn := func(ctx context.Context, accountID string) (string, string, int64, error) {
		atomic.AddInt32(&calls, 1)
		select {
		case exchanged <- struct{}{}:
		default:
		}
		return "new-jwt", "new-seed", time.Now().Add(time.Hour).Unix(), nil
	}

	r := &RefreshLoop{AccountID: "test-org", Refresh: fn}

	// Initial JWT expires in 1s — refresh should fire before that.
	expSoon := time.Now().Add(1 * time.Second).Unix()
	if err := r.Start(context.Background(), "init-jwt", "init-seed", expSoon); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	select {
	case <-exchanged:
		// good
	case <-time.After(2 * time.Second):
		t.Fatalf("RefreshFn was not called within 2s (expected before exp=1s); calls=%d", atomic.LoadInt32(&calls))
	}

	// After refresh, Current() should return the new credential.
	gotJWT, gotSeed := r.Current()
	if gotJWT != "new-jwt" || gotSeed != "new-seed" {
		t.Errorf("Current after refresh: got (%q, %q) want (new-jwt, new-seed)", gotJWT, gotSeed)
	}
}

func TestRefreshLoop_TracksLastRefreshAt(t *testing.T) {
	refreshed := make(chan struct{}, 1)
	fn := func(ctx context.Context, accountID string) (string, string, int64, error) {
		select {
		case refreshed <- struct{}{}:
		default:
		}
		return "new-jwt", "new-seed", time.Now().Add(time.Hour).Unix(), nil
	}

	r := &RefreshLoop{AccountID: "test-org", Refresh: fn}

	beforeStart := time.Now()
	expSoon := time.Now().Add(500 * time.Millisecond).Unix()
	if err := r.Start(context.Background(), "init-jwt", "init-seed", expSoon); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	initialRefresh := r.LastRefreshAt()
	if initialRefresh.IsZero() || initialRefresh.Before(beforeStart) {
		t.Fatalf("LastRefreshAt after Start = %v, want non-zero at or after %v", initialRefresh, beforeStart)
	}

	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatalf("RefreshFn was not called within 2s")
	}

	afterRefresh := r.LastRefreshAt()
	if !afterRefresh.After(initialRefresh) {
		t.Fatalf("LastRefreshAt after refresh = %v, want after initial %v", afterRefresh, initialRefresh)
	}
}

func TestEnsureRefreshLoopFromCredsStartsLoopFromPersistedCredentials(t *testing.T) {
	d := New(t.TempDir(), "")
	creds := Credentials{
		URL:          "http://127.0.0.1:1",
		APIKey:       "ppz_test_key",
		AccountID:        "test-org",
		NATSUserJWT:  "persisted-jwt",
		NATSUserSeed: "persisted-seed",
	}

	d.ensureRefreshLoopFromCreds(&creds)
	if d.Refresh == nil {
		t.Fatalf("ensureRefreshLoopFromCreds did not start refresh loop")
	}
	t.Cleanup(d.Refresh.Stop)

	if gotJWT, gotSeed := d.Refresh.Current(); gotJWT != "persisted-jwt" || gotSeed != "persisted-seed" {
		t.Fatalf("refresh loop credentials = (%q, %q), want persisted credentials", gotJWT, gotSeed)
	}
	if got := d.Refresh.LastRefreshAt(); got.IsZero() {
		t.Fatalf("LastRefreshAt is zero after starting from persisted credentials")
	}
}

func TestRefreshLoopRefreshNowIfDueRefreshesExpiredCredentialSynchronously(t *testing.T) {
	var calls int32
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	fn := func(ctx context.Context, accountID string) (string, string, int64, error) {
		atomic.AddInt32(&calls, 1)
		if accountID != "test-org" {
			t.Fatalf("RefreshFn accountID = %q, want test-org", accountID)
		}
		return "fresh-jwt", "fresh-seed", now.Add(time.Hour).Unix(), nil
	}

	r := &RefreshLoop{AccountID: "test-org", Refresh: fn}
	if err := r.Start(context.Background(), "expired-jwt", "expired-seed", now.Add(-time.Hour).Unix()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	refreshed, err := r.RefreshNowIfDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RefreshNowIfDue: %v", err)
	}
	if !refreshed {
		t.Fatal("RefreshNowIfDue refreshed=false, want true for expired credential")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("RefreshFn calls = %d, want 1", got)
	}
	gotJWT, gotSeed := r.Current()
	if gotJWT != "fresh-jwt" || gotSeed != "fresh-seed" {
		t.Fatalf("Current after RefreshNowIfDue = (%q, %q), want fresh credentials", gotJWT, gotSeed)
	}
	if got := r.LastRefreshAt(); got.IsZero() {
		t.Fatal("LastRefreshAt after RefreshNowIfDue is zero")
	}
}

func TestRefreshLoopRefreshNowIfDueSkipsFreshCredential(t *testing.T) {
	var calls int32
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	fn := func(ctx context.Context, accountID string) (string, string, int64, error) {
		atomic.AddInt32(&calls, 1)
		return "unexpected", "unexpected", now.Add(time.Hour).Unix(), nil
	}

	r := &RefreshLoop{AccountID: "test-org", Refresh: fn}
	if err := r.Start(context.Background(), "current-jwt", "current-seed", now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	refreshed, err := r.RefreshNowIfDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RefreshNowIfDue: %v", err)
	}
	if refreshed {
		t.Fatal("RefreshNowIfDue refreshed=true, want false for fresh credential")
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("RefreshFn calls = %d, want 0", got)
	}
}

// TestRefreshTimer_HandlesUnauthorized: when RefreshFn returns
// ErrUnauthorized, the loop stops + invokes OnUnauthorized so the
// daemon can surface "session expired" via `ppz status`.
func TestRefreshTimer_HandlesUnauthorized(t *testing.T) {
	var unauthCount int32
	var unauthOrg string
	var mu sync.Mutex

	failingFn := func(ctx context.Context, accountID string) (string, string, int64, error) {
		return "", "", 0, ErrUnauthorized
	}

	r := &RefreshLoop{
		AccountID:   "acme",
		Refresh: failingFn,
		OnUnauthorized: func(accountID string) {
			atomic.AddInt32(&unauthCount, 1)
			mu.Lock()
			unauthOrg = accountID
			mu.Unlock()
		},
	}

	expSoon := time.Now().Add(500 * time.Millisecond).Unix()
	if err := r.Start(context.Background(), "init-jwt", "init-seed", expSoon); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	// Wait for the failure path to fire.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&unauthCount) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if atomic.LoadInt32(&unauthCount) == 0 {
		t.Fatalf("OnUnauthorized was not called within 3s")
	}
	mu.Lock()
	defer mu.Unlock()
	if unauthOrg != "acme" {
		t.Errorf("OnUnauthorized got accountID=%q want %q", unauthOrg, "acme")
	}

	// After unauth, Current() should still return the last known good
	// credential (callers can decide whether to keep using it). The
	// contract is the loop *stops* — it doesn't *forget*.
	if jwt, _ := r.Current(); jwt != "init-jwt" {
		t.Errorf("Current after unauth: got %q want init-jwt (last-known)", jwt)
	}
}

// TestRefreshTimer_StopIsIdempotent: calling Stop twice is safe.
// Catches the "send on closed channel" panic class.
func TestRefreshTimer_StopIsIdempotent(t *testing.T) {
	r := &RefreshLoop{
		AccountID: "x",
		Refresh: func(ctx context.Context, accountID string) (string, string, int64, error) {
			return "", "", 0, errors.New("never called")
		},
	}
	if err := r.Start(context.Background(), "j", "s", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	r.Stop()
	r.Stop() // must not panic
}
