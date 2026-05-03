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

	fn := func(ctx context.Context, orgID string) (string, string, int64, error) {
		atomic.AddInt32(&calls, 1)
		select {
		case exchanged <- struct{}{}:
		default:
		}
		return "new-jwt", "new-seed", time.Now().Add(time.Hour).Unix(), nil
	}

	r := &RefreshLoop{OrgID: "test-org", Refresh: fn}

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

// TestRefreshTimer_HandlesUnauthorized: when RefreshFn returns
// ErrUnauthorized, the loop stops + invokes OnUnauthorized so the
// daemon can surface "session expired" via `ppz status`.
func TestRefreshTimer_HandlesUnauthorized(t *testing.T) {
	var unauthCount int32
	var unauthOrg string
	var mu sync.Mutex

	failingFn := func(ctx context.Context, orgID string) (string, string, int64, error) {
		return "", "", 0, ErrUnauthorized
	}

	r := &RefreshLoop{
		OrgID:   "acme",
		Refresh: failingFn,
		OnUnauthorized: func(orgID string) {
			atomic.AddInt32(&unauthCount, 1)
			mu.Lock()
			unauthOrg = orgID
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
		t.Errorf("OnUnauthorized got orgID=%q want %q", unauthOrg, "acme")
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
		OrgID:   "x",
		Refresh: func(ctx context.Context, orgID string) (string, string, int64, error) { return "", "", 0, errors.New("never called") },
	}
	if err := r.Start(context.Background(), "j", "s", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	r.Stop()
	r.Stop() // must not panic
}
