// Package server runs the ppz HTTP API + GUI + embedded NATS.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"

	"github.com/pipescloud/ppz/internal/db"
)

type Config struct {
	DBURL         string
	HTTPAddr      string
	NATSAddr      string
	NATSPublicURL string // returned to clients in /auth/exchange; defaults to ns.ClientURL()
	SeedDir       string

	// SessionKey is the HMAC key used to sign session cookies. Read
	// from PPZ_SESSION_KEY at boot. 32+ bytes recommended.
	SessionKey []byte

	// BaseURL is what the server thinks its own external URL is —
	// used to construct OAuth redirect_uri. e.g.,
	// "https://pipescloud.io" or "http://localhost:8080".
	BaseURL string

	// GitHub OAuth config. Real values come from .env.local (dev) or
	// AWS Secrets Manager (prod). The three URL fields default to
	// real-GitHub if empty; the e2e stack overrides them to point at
	// the mock-github container.
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAuthorizeURL string
	GitHubTokenURL     string
	GitHubUserURL      string

	// DevLogin enables POST /dev/login?user=<seed-user> for tests.
	// MUST be false in production.
	DevLogin bool

	// Auth V2 §Phase 3.5 — NSC/JWT decentralized NATS auth.
	// Operator seed is the runtime secret (signs new per-org
	// Account JWTs as orgs are provisioned). Operator JWT is
	// public — declares the operator's existence to the embedded
	// NATS server. System Account JWT lives in /sys for JetStream.
	NATSOperatorSeed     string
	NATSOperatorJWT      string
	NATSSystemAccountJWT string
}

// Server holds the shared dependencies threaded through HTTP handlers.
type Server struct {
	Pool       *db.Pool
	NATSURL    string
	SessionKey SessionKey

	// Phase 3.5 — per-org account pool. Lazily provisions an
	// Operator-signed Account JWT per org on first use, opens a
	// per-account in-process connection, registers the JWT with
	// the live resolver. AccountPool.Get(orgID) is the path to
	// per-org NATS state. (No legacy single-account fallback —
	// every NATS operation routes through the pool.)
	AccountPool    *AccountPool
	OperatorKP     nkeys.KeyPair             // hot at runtime; signs new Account JWTs
	NATSResolver   natsserver.AccountResolver // for runtime account JWT registration
	NATSClientURL  string                    // for AccountPool.openAccount

	BaseURL            string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAuthorizeURL string
	GitHubTokenURL     string
	GitHubUserURL      string
	DevLogin           bool
}

func Run(ctx context.Context, cfg Config) error {
	pool, err := db.Open(ctx, cfg.DBURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Phase 3.5: load the Operator seed at runtime — needed to sign
	// new per-org Account JWTs as orgs are provisioned. Same trust
	// boundary as PPZ_NATS_ACCOUNT_SIGNING_SEED.
	if cfg.NATSOperatorSeed == "" {
		return fmt.Errorf("PPZ_NATS_OPERATOR_SEED is required (Auth V2 §Phase 3.5)")
	}
	operatorKP, err := nkeys.FromSeed([]byte(cfg.NATSOperatorSeed))
	if err != nil {
		return fmt.Errorf("parse PPZ_NATS_OPERATOR_SEED: %w", err)
	}

	ns, natsCleanup, err := startEmbeddedNATS(cfg)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	defer natsCleanup()

	// NATSURL is the *advertised* URL handed back by /auth/exchange. If
	// empty, the handler derives it from the request's Host header so the
	// same server works for both compose-internal and host clients.
	srv := &Server{
		Pool:               pool,
		NATSURL:            cfg.NATSPublicURL,
		SessionKey:         SessionKey(cfg.SessionKey),
		OperatorKP:         operatorKP,
		NATSResolver:       ns.AccountResolver(),
		NATSClientURL:      ns.ClientURL(),
		BaseURL:            cfg.BaseURL,
		GitHubClientID:     cfg.GitHubClientID,
		GitHubClientSecret: cfg.GitHubClientSecret,
		GitHubAuthorizeURL: defaultIfEmpty(cfg.GitHubAuthorizeURL, "https://github.com/login/oauth/authorize"),
		GitHubTokenURL:     defaultIfEmpty(cfg.GitHubTokenURL, "https://github.com/login/oauth/access_token"),
		GitHubUserURL:      defaultIfEmpty(cfg.GitHubUserURL, "https://api.github.com/user"),
		DevLogin:           cfg.DevLogin,
	}
	srv.AccountPool = newAccountPool(srv)
	defer srv.AccountPool.Close()
	// Per-org broadcast subscribers are attached when AccountPool
	// provisions each account (see subscribeBroadcasts in
	// broadcast_subscriber.go). No global subscriber is needed.

	mux := srv.Routes()
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("ppz-server listening on %s", cfg.HTTPAddr)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func defaultIfEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
