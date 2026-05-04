package daemon

// Phase 3.5 — daemon JWT refresh loop. Watches a cached User JWT's
// `exp` claim and re-runs /auth/exchange before it expires, swapping
// the cached (jwt, seed) atomically so the next NATS reconnect picks
// up the new credentials.

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RefreshFn is the work the refresh loop calls when a JWT is about
// to expire. Implementations re-run POST /api/v1/auth/exchange and
// return the new (jwt, seed). orgID lets a multi-org daemon route
// to the right account.
type RefreshFn func(ctx context.Context, orgID string) (jwt, seed string, expUnix int64, err error)

// ErrUnauthorized is what RefreshFn returns when the bearer was
// revoked / expired — distinct from transient network failures
// (which the loop retries). Triggers OnUnauthorized + stop.
var ErrUnauthorized = errors.New("daemon.RefreshLoop: unauthorized")

// skewSeconds is how far before exp we re-run /auth/exchange. The
// User JWT's `nbf` claim is also set 30s before now on the server
// side, so a 30s skew here means rotation happens with a 60s
// overlap window where both old + new JWTs are valid.
const skewSeconds = 30

// retryAfter is the backoff between retries on transient (non-401)
// errors from RefreshFn.
const retryAfter = 5 * time.Second

// RefreshLoop monitors one (org, JWT) pair and refreshes it before
// expiry. Concurrency: Current() may be called from any goroutine;
// Start/Stop must be called from the same goroutine.
type RefreshLoop struct {
	OrgID          string
	Refresh        RefreshFn
	OnUnauthorized func(orgID string)

	mu      sync.Mutex
	jwt     string
	seed    string
	expUnix int64
	lastAt  time.Time
	cancel  context.CancelFunc
}

// Start begins the refresh goroutine with an initial credential.
// expUnix is the JWT's `exp` claim in unix seconds.
func (r *RefreshLoop) Start(ctx context.Context, jwt, seed string, expUnix int64) error {
	if r.Refresh == nil {
		return errors.New("RefreshLoop.Start: Refresh fn is required")
	}
	loopCtx, cancel := context.WithCancel(ctx)

	r.mu.Lock()
	r.jwt = jwt
	r.seed = seed
	r.expUnix = expUnix
	r.lastAt = time.Now()
	r.cancel = cancel
	r.mu.Unlock()

	go r.run(loopCtx)
	return nil
}

// Stop halts the refresh goroutine. Idempotent.
func (r *RefreshLoop) Stop() {
	r.mu.Lock()
	c := r.cancel
	r.mu.Unlock()
	if c != nil {
		c() // context.CancelFunc is documented as safe to call repeatedly
	}
}

// Current returns the freshest (jwt, seed) — used by nats.UserJWT()
// callbacks on every NATS (re)connect.
func (r *RefreshLoop) Current() (jwt, seed string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jwt, r.seed
}

// LastRefreshAt returns when the loop last accepted fresh credentials.
// Start counts as the first refresh because its credentials came from
// /auth/exchange immediately before the loop was started.
func (r *RefreshLoop) LastRefreshAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastAt
}

// RefreshNowIfDue refreshes synchronously when the cached credential is already
// inside its refresh window. It covers machines waking from sleep: the timer
// goroutine may not have run yet, but the next command must not continue with
// an expired JWT.
func (r *RefreshLoop) RefreshNowIfDue(ctx context.Context, now time.Time) (bool, error) {
	r.mu.Lock()
	exp := r.expUnix
	r.mu.Unlock()

	fireAt := time.Unix(exp, 0).Add(-time.Duration(skewSeconds) * time.Second)
	if now.Before(fireAt) {
		return false, nil
	}
	if err := r.refreshNow(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *RefreshLoop) run(ctx context.Context) {
	for {
		// Sleep until exp - skew, with a small floor so we never
		// busy-loop if the supplied JWT is already past its skew
		// window (unit tests pass JWTs that expire in <1s).
		r.mu.Lock()
		exp := r.expUnix
		r.mu.Unlock()

		fireAt := time.Unix(exp, 0).Add(-time.Duration(skewSeconds) * time.Second)
		delay := time.Until(fireAt)
		if delay < 100*time.Millisecond {
			delay = 100 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		if err := r.refreshNow(ctx); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				return
			}
			// Transient — back off and retry. The cached creds are
			// still the previous (still-valid until exp) values, so
			// callers' nats.UserJWT callback continues working until
			// the next reconnect.
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryAfter):
				continue
			}
		}
	}
}

func (r *RefreshLoop) refreshNow(ctx context.Context) error {
	newJWT, newSeed, newExp, err := r.Refresh(ctx, r.OrgID)
	if errors.Is(err, ErrUnauthorized) {
		if r.OnUnauthorized != nil {
			r.OnUnauthorized(r.OrgID)
		}
		return err
	}
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.jwt = newJWT
	r.seed = newSeed
	r.expUnix = newExp
	r.lastAt = time.Now()
	r.mu.Unlock()
	return nil
}
