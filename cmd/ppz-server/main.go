package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pipescloud/ppz/internal/server"
	"github.com/pipescloud/ppz/internal/version"
)

func main() {
	// Phase 2 Cycle C: PPZ_SERVER_AUTH_MODE governs the /login route's
	// behaviour. Default (unset) is AuthModeNone; invalid values fail
	// boot loudly.
	authMode, err := server.ParseAuthMode(os.Getenv("PPZ_SERVER_AUTH_MODE"))
	if err != nil {
		log.Fatal(err)
	}
	cfg := server.Config{
		DBURL:         envOr("PPZ_DB_URL", "postgres://postgres:ppz@localhost:5432/ppz?sslmode=disable"),
		HTTPAddr:      envOr("PPZ_HTTP_ADDR", ":8080"),
		NATSAddr:      envOr("PPZ_NATS_ADDR", ":4222"),
		NATSPublicURL: envOr("PPZ_NATS_PUBLIC_URL", ""),
		SeedDir:       envOr("PPZ_SEED_DIR", "/seed"),
		// Auth V2
		SessionKey: []byte(envOr("PPZ_SESSION_KEY", "")),
		BaseURL:    envOr("PPZ_BASE_URL", "http://localhost:8080"),
		DevLogin:   envOr("PPZ_DEV_LOGIN", "") == "true",
		AuthMode:   authMode,
		// Auth V2 §Phase 3.5 — NATS NSC/JWT auth.
		NATSOperatorSeed:     envOr("PPZ_NATS_OPERATOR_SEED", ""),
		NATSOperatorJWT:      envOr("PPZ_NATS_OPERATOR_JWT", ""),
		NATSSystemAccountJWT: envOr("PPZ_NATS_SYSTEM_ACCOUNT_JWT", ""),
		Version:              version.Version,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := server.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
