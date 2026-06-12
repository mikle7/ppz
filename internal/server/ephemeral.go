package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"

	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/natsauth"
	"github.com/pipescloud/ppz/internal/seed"
)

// RunEphemeral is `ppz-server --ephemeral`: a zero-config, throwaway
// server for local development and tools like h2oslide. It provisions
// everything a normal deployment gets from ops:
//
//   - Postgres: embedded (binaries auto-downloaded to
//     ~/.embedded-postgres on first use), data in a temp dir, gone on
//     exit.
//   - NATS trust root: minted in-process (natsauth.BootstrapOperator) —
//     every credential dies with the server, which is the point.
//   - Session key: random.
//   - Account: the standard seed fixture (alpha/beta, foo/bar).
//   - Ports: free ports unless PPZ_HTTP_ADDR / PPZ_NATS_ADDR are set.
//
// It prints two machine-readable lines on stdout for spawning tools:
//
//	PPZ_EPHEMERAL_URL=http://127.0.0.1:<port>
//	PPZ_EPHEMERAL_API_KEY=<key-alpha>
//
// then blocks until ctx is cancelled. Fields already set on cfg (env
// overrides) win over provisioning.
func RunEphemeral(ctx context.Context, cfg Config) error {
	sweepStaleEphemeralDirs()

	work, err := os.MkdirTemp("", "ppz-ephemeral-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	// Pid marker lets the next --ephemeral run sweep this dir if we die
	// without running defers (SIGKILL, OOM).
	_ = os.WriteFile(filepath.Join(work, "pid"), []byte(fmt.Sprint(os.Getpid())), 0o644)

	httpAddr, httpPort, err := pickAddr(os.Getenv("PPZ_HTTP_ADDR"))
	if err != nil {
		return err
	}
	natsAddr, natsPort, err := pickAddr(os.Getenv("PPZ_NATS_ADDR"))
	if err != nil {
		return err
	}
	pgPort, err := freePort()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "ppz-server: ephemeral mode — starting embedded postgres (first run downloads binaries)\n")
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(uint32(pgPort)).
		Username("postgres").Password("ppz").Database("ppz").
		DataPath(filepath.Join(work, "pgdata")).
		RuntimePath(filepath.Join(work, "pgrun")).
		Logger(os.Stderr).
		StartTimeout(120 * time.Second))
	if err := pg.Start(); err != nil {
		return fmt.Errorf("embedded postgres: %w", err)
	}
	defer func() {
		if err := pg.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "ppz-server: stop embedded postgres: %v\n", err)
		}
	}()

	cfg.DBURL = fmt.Sprintf("postgres://postgres:ppz@127.0.0.1:%d/ppz?sslmode=disable", pgPort)
	cfg.HTTPAddr = httpAddr
	cfg.NATSAddr = natsAddr
	cfg.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	if cfg.NATSPublicURL == "" {
		// auth/exchange's Host-derived fallback assumes :4222; with a
		// random ephemeral port the URL must be advertised explicitly.
		cfg.NATSPublicURL = fmt.Sprintf("nats://127.0.0.1:%d", natsPort)
	}

	if cfg.NATSOperatorSeed == "" {
		chain, err := natsauth.BootstrapOperator()
		if err != nil {
			return fmt.Errorf("bootstrap nats chain: %w", err)
		}
		cfg.NATSOperatorSeed = chain.OperatorSeed
		cfg.NATSOperatorJWT = chain.OperatorJWT
		cfg.NATSSystemAccountJWT = chain.SystemAccountJWT
	}
	if len(cfg.SessionKey) == 0 {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		cfg.SessionKey = []byte(hex.EncodeToString(key))
	}
	if cfg.NATSJetStreamStoreDir == "" {
		cfg.NATSJetStreamStoreDir = filepath.Join(work, "jetstream")
	}
	seedDir := filepath.Join(work, "seed")
	cfg.SeedDir = seedDir

	// Migrate + seed before the server runs so the API key exists the
	// moment the URL is printed (Run migrates again — idempotent).
	if err := migrateAndSeed(ctx, cfg.DBURL, seedDir); err != nil {
		return err
	}
	apiKey, err := os.ReadFile(filepath.Join(seedDir, "key-alpha.txt"))
	if err != nil {
		return fmt.Errorf("read seeded api key: %w", err)
	}

	// The printed credentials are the readiness contract for spawning
	// tools (h2oslide --ephemeral): they must not appear until the HTTP
	// listener accepts connections, so start the server first and poll.
	runErr := make(chan error, 1)
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	go func() { runErr <- Run(runCtx, cfg) }()

	if err := waitTCP(runCtx, cfg.HTTPAddr, 60*time.Second, runErr); err != nil {
		return fmt.Errorf("ephemeral server never became ready: %w", err)
	}
	fmt.Printf("PPZ_EPHEMERAL_URL=%s\n", cfg.BaseURL)
	fmt.Printf("PPZ_EPHEMERAL_API_KEY=%s\n", string(apiKey))
	fmt.Fprintf(os.Stderr, "ppz-server: ephemeral server ready on %s (account alpha; state dies with this process)\n", cfg.BaseURL)

	return <-runErr
}

// sweepStaleEphemeralDirs removes ppz-ephemeral-* temp dirs whose owning
// process is gone — each holds a ~100MB postgres tree, so leaks from
// unclean deaths add up fast. A dir without a pid marker is from a run
// that died before writing it (or a pre-marker build): also stale.
func sweepStaleEphemeralDirs() {
	dirs, _ := filepath.Glob(filepath.Join(os.TempDir(), "ppz-ephemeral-*"))
	for _, dir := range dirs {
		raw, err := os.ReadFile(filepath.Join(dir, "pid"))
		if err == nil {
			var pid int
			if _, err := fmt.Sscanf(string(raw), "%d", &pid); err == nil && pid > 0 {
				if proc, err := os.FindProcess(pid); err == nil && proc.Signal(syscall.Signal(0)) == nil {
					continue // owner still alive
				}
			}
		}
		fmt.Fprintf(os.Stderr, "ppz-server: sweeping stale ephemeral dir %s\n", dir)
		stopOrphanPostgres(dir)
		_ = os.RemoveAll(dir)
	}
}

// stopOrphanPostgres terminates a postgres left running by an unclean
// death (its postmaster would otherwise outlive the swept data dir).
// First line of pgdata/postmaster.pid is the postmaster's pid.
func stopOrphanPostgres(dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "pgdata", "postmaster.pid"))
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil || pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 50 && proc.Signal(syscall.Signal(0)) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

// waitTCP polls addr until it accepts a connection, the server errors
// out, or the timeout lapses.
func waitTCP(ctx context.Context, addr string, timeout time.Duration, runErr <-chan error) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-runErr:
			return fmt.Errorf("server exited during startup: %w", err)
		default:
		}
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

func migrateAndSeed(ctx context.Context, dbURL, seedDir string) error {
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := seed.Run(ctx, pool, seedDir); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

// pickAddr returns the configured addr unchanged, or a 127.0.0.1 addr on
// a free port when unset. Also reports the port for URL construction.
func pickAddr(configured string) (addr string, port int, err error) {
	if configured != "" {
		_, p, err := net.SplitHostPort(configured)
		if err != nil {
			return "", 0, fmt.Errorf("parse addr %q: %w", configured, err)
		}
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
			return "", 0, fmt.Errorf("parse port %q: %w", p, err)
		}
		return configured, n, nil
	}
	p, err := freePort()
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("127.0.0.1:%d", p), p, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
