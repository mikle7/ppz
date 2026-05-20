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
	cfg := server.Config{
		DBURL:         envOr("PPZ_DB_URL", "postgres://postgres:ppz@localhost:5432/ppz?sslmode=disable"),
		HTTPAddr:      envOr("PPZ_HTTP_ADDR", ":8080"),
		NATSAddr:      envOr("PPZ_NATS_ADDR", ":4222"),
		NATSPublicURL: envOr("PPZ_NATS_PUBLIC_URL", ""),
		SeedDir:       envOr("PPZ_SEED_DIR", "/seed"),
		// Auth V2
		SessionKey:         []byte(envOr("PPZ_SESSION_KEY", "")),
		BaseURL:            envOr("PPZ_BASE_URL", "http://localhost:8080"),
		GitHubClientID:     envOr("PPZ_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: envOr("PPZ_GITHUB_CLIENT_SECRET", ""),
		GitHubAuthorizeURL: envOr("PPZ_GITHUB_AUTHORIZE_URL", ""),
		GitHubTokenURL:     envOr("PPZ_GITHUB_TOKEN_URL", ""),
		GitHubUserURL:      envOr("PPZ_GITHUB_USER_URL", ""),
		DevLogin:           envOr("PPZ_DEV_LOGIN", "") == "true",
		// Auth V2 §Phase 3.5 — NATS NSC/JWT auth.
		NATSOperatorSeed:      envOr("PPZ_NATS_OPERATOR_SEED", ""),
		NATSOperatorJWT:       envOr("PPZ_NATS_OPERATOR_JWT", ""),
		NATSSystemAccountJWT:  envOr("PPZ_NATS_SYSTEM_ACCOUNT_JWT", ""),
		NATSJetStreamStoreDir: envOr("PPZ_JETSTREAM_STORE_DIR", ""),
		Version:               version.Version,
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
