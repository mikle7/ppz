package server

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/pipescloud/ppz/internal/natsauth"
)

// startEmbeddedNATS launches an embedded nats-server with JetStream +
// decentralized auth. The Operator declares the trust root; the
// System Account JWT is preloaded into the in-memory resolver. Per-
// org Account JWTs are added at runtime by AccountPool as orgs are
// provisioned — there is no longer a "default tenants" account at
// boot.
func startEmbeddedNATS(cfg Config) (*natsserver.Server, func(), error) {
	if cfg.NATSOperatorJWT == "" || cfg.NATSSystemAccountJWT == "" {
		return nil, nil, errors.New("missing PPZ_NATS_OPERATOR_JWT / PPZ_NATS_SYSTEM_ACCOUNT_JWT")
	}

	host, portStr, err := splitAddr(cfg.NATSAddr)
	if err != nil {
		return nil, nil, err
	}
	port, _ := strconv.Atoi(portStr)

	ns, err := natsauth.StartEmbeddedNATSWithAuth(natsauth.EmbeddedConfig{
		Host:             host,
		Port:             port,
		OperatorJWT:      cfg.NATSOperatorJWT,
		SystemAccountJWT: cfg.NATSSystemAccountJWT,
		JetStream:        true,
		StoreDir:         cfg.NATSJetStreamStoreDir,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start embedded nats: %w", err)
	}
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		return nil, nil, errors.New("nats not ready")
	}

	cleanup := func() { ns.Shutdown() }
	return ns, cleanup, nil
}

func splitAddr(a string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(a)
	if err != nil {
		return "", "", err
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if port == "" {
		port = "4222"
	}
	return host, port, nil
}
