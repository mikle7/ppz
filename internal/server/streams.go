package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/natsubj"
)

// Default JetStream stream config for newly-provisioned pipes. Each can
// be overridden per-pipe via the `retention` block on POST /sources/{h}/pipes.
//
// Sizing rationale: stdout publishes ~4 KiB chunks per pty write, so the
// previous 1000-msg cap evicted history after a few seconds of busy TUI
// traffic — long before the 64 MiB byte cap kicked in. Bumping the count
// to 5000 + dropping bytes to 16 MiB makes both caps roughly meet around
// the same point for stdout (4 KiB × 4096 ≈ 16 MiB), and keeps small-
// message pipes (broadcast/stdin/stdctrl) bounded by msg count without
// blowing storage budgets.
const (
	defaultStreamMaxAge   = 24 * time.Hour
	defaultStreamMaxMsgs  = 5000
	defaultStreamMaxBytes = 16 * 1024 * 1024 // 16 MiB
)

// ensurePipeStream creates a JetStream stream backing one (source, pipe)
// pair (source_<orgshort>_<handle>_<pipe>) with the default retention.
// Idempotent on duplicate creation.
func ensurePipeStream(ctx context.Context, js jetstream.JetStream, orgID uuid.UUID, handle, pipe string) error {
	return ensurePipeStreamWithRetention(ctx, js, orgID, handle, pipe,
		defaultStreamMaxAge, defaultStreamMaxMsgs, defaultStreamMaxBytes)
}

// ensurePipeStreamWithRetention is the override-aware variant. Used by
// `pipe create` to honour --ttl / --max-msgs / --max-bytes flags. Every
// arg is mandatory (the caller fills in defaults for any flag the user
// didn't pass) so the resulting stream config is fully deterministic.
func ensurePipeStreamWithRetention(ctx context.Context, js jetstream.JetStream, orgID uuid.UUID, handle, pipe string, maxAge time.Duration, maxMsgs int, maxBytes int64) error {
	cfg := jetstream.StreamConfig{
		Name:      natsubj.StreamName(orgID, handle, pipe),
		Subjects:  []string{natsubj.Subject(orgID, handle, pipe)},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    maxAge,
		MaxMsgs:   int64(maxMsgs),
		MaxBytes:  maxBytes,
		Storage:   jetstream.FileStorage,
		Discard:   jetstream.DiscardOld,
		Replicas:  1,
	}
	_, err := js.CreateStream(ctx, cfg)
	if err != nil {
		if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			return nil
		}
		return fmt.Errorf("create stream %s: %w", cfg.Name, err)
	}
	return nil
}

// deletePipeStream removes the JetStream stream backing one (source, pipe).
// Idempotent — already-absent stream is a success.
func deletePipeStream(ctx context.Context, js jetstream.JetStream, orgID uuid.UUID, handle, pipe string) error {
	name := natsubj.StreamName(orgID, handle, pipe)
	if err := js.DeleteStream(ctx, name); err != nil {
		if errors.Is(err, jetstream.ErrStreamNotFound) {
			return nil
		}
		return fmt.Errorf("delete stream %s: %w", name, err)
	}
	return nil
}

// jsFor returns a JetStream context bound to the server's NATS connection.
func jsFor(nc *nats.Conn) (jetstream.JetStream, error) {
	return jetstream.New(nc)
}
