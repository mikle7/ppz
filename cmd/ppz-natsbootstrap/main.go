// ppz-natsbootstrap mints an ephemeral NSC chain (Operator + Account
// + System Account + Account signing key) and prints the four env
// vars ppz-server reads at boot. Used by `compose/server-
// entrypoint.sh` for local dev / e2e — production gets the same
// values from Pulumi-managed AWS Secrets Manager.
//
// Output format is bare KEY=VALUE lines so a shell can `eval` it.
//
//   PPZ_NATS_OPERATOR_JWT=…
//   PPZ_NATS_ACCOUNT_JWT=…
//   PPZ_NATS_SYSTEM_ACCOUNT_JWT=…
//   PPZ_NATS_ACCOUNT_SIGNING_SEED=…
package main

import (
	"fmt"
	"os"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ppz-natsbootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	opKP, err := nkeys.CreateOperator()
	if err != nil {
		return err
	}
	opPub, _ := opKP.PublicKey()
	opSeed, _ := opKP.Seed()

	sysKP, err := nkeys.CreateAccount()
	if err != nil {
		return err
	}
	sysPub, _ := sysKP.PublicKey()

	opClaims := jwt.NewOperatorClaims(opPub)
	opClaims.Name = "ppz-operator"
	opClaims.SystemAccount = sysPub
	opJWT, err := opClaims.Encode(opKP)
	if err != nil {
		return err
	}

	sysClaims := jwt.NewAccountClaims(sysPub)
	sysClaims.Name = "ppz-sys"
	sysJWT, err := sysClaims.Encode(opKP)
	if err != nil {
		return err
	}

	// Phase 3.5: no longer mints a default tenants account at boot —
	// per-org accounts are created at runtime by ppz-server's
	// AccountPool, signed with the operator key it holds in env.
	fmt.Printf("PPZ_NATS_OPERATOR_JWT=%s\n", opJWT)
	fmt.Printf("PPZ_NATS_OPERATOR_SEED=%s\n", string(opSeed))
	fmt.Printf("PPZ_NATS_SYSTEM_ACCOUNT_JWT=%s\n", sysJWT)
	return nil
}
