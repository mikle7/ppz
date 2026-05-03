package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/seed"
)

func main() {
	dir := flag.String("dir", "/seed", "directory to write plaintext keys + org IDs into")
	dbURL := flag.String("db", envOr("PPZ_DB_URL", ""), "postgres URL")
	flag.Parse()
	if *dbURL == "" {
		log.Fatal("ppz-seed: --db or PPZ_DB_URL required")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, *dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := seed.Run(ctx, pool, *dir); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
