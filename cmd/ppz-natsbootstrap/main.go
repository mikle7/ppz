// ppz-natsbootstrap mints an ephemeral NSC chain (Operator + Account
// + System Account + Account signing key) and prints the four env
// vars ppz-server reads at boot. Used by `compose/server-
// entrypoint.sh` for local dev / e2e — production gets the same
// values from Pulumi-managed AWS Secrets Manager.
//
// Output format is bare KEY=VALUE lines so a shell can `eval` it.
//
//	PPZ_NATS_OPERATOR_JWT=…
//	PPZ_NATS_OPERATOR_SEED=…
//	PPZ_NATS_SYSTEM_ACCOUNT_JWT=…
package main

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/natsauth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ppz-natsbootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	// Phase 3.5: no longer mints a default tenants account at boot —
	// per-org accounts are created at runtime by ppz-server's
	// AccountPool, signed with the operator key it holds in env.
	chain, err := natsauth.BootstrapOperator()
	if err != nil {
		return err
	}
	fmt.Printf("PPZ_NATS_OPERATOR_JWT=%s\n", chain.OperatorJWT)
	fmt.Printf("PPZ_NATS_OPERATOR_SEED=%s\n", chain.OperatorSeed)
	fmt.Printf("PPZ_NATS_SYSTEM_ACCOUNT_JWT=%s\n", chain.SystemAccountJWT)
	return nil
}
