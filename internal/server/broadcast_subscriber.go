package server

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"
)

// subscribeBroadcasts attaches a Core NATS subscriber to the org's
// in-process connection that mirrors every broadcast into Postgres
// (last_broadcast_* columns the GUI uses for "last message" display).
//
// One subscriber per account — Phase 3.5 puts each org in its own
// NATS account, so a single shared subscriber can't see all orgs'
// traffic. AccountPool.openAccount calls this when provisioning.
func (s *Server) subscribeBroadcasts(oa *OrgAccount) error {
	sub, err := oa.NC.Subscribe("*.*.broadcast", func(m *nats.Msg) {
		s.handleBroadcastMsg(context.Background(), m)
	})
	if err != nil {
		return err
	}
	// Hold a reference to the subscription on the OrgAccount so its
	// cleanup func also unsubscribes; otherwise on org delete we'd
	// orphan the subscription.
	oa.attachCleanup(func() { _ = sub.Unsubscribe() })
	return nil
}

// startBroadcastSubscriber is the legacy single-connection variant,
// retained for the boot-time s.NC connection (currently unused by
// any tenant traffic — every per-org account has its own subscriber
// via subscribeBroadcasts). Will be removed once s.NC is fully
// retired.
func (s *Server) startBroadcastSubscriber(ctx context.Context, nc *nats.Conn) error {
	sub, err := nc.Subscribe("*.*.broadcast", func(m *nats.Msg) {
		s.handleBroadcastMsg(ctx, m)
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
	return nil
}

func (s *Server) handleBroadcastMsg(ctx context.Context, m *nats.Msg) {
	parts := strings.Split(m.Subject, ".")
	if len(parts) != 3 || parts[2] != "broadcast" {
		return
	}
	orgID, err := uuid.Parse(parts[0])
	if err != nil {
		return
	}
	env, err := envelope.Unmarshal(m.Data)
	if err != nil {
		return
	}
	if err := db.UpdateLastBroadcast(ctx, s.Pool, orgID, env.Handle, env.CreatedAt, env.Payload); err != nil {
		log.Printf("ppz-server: update last_broadcast for %s/%s: %v", orgID, env.Handle, err)
	}
}
