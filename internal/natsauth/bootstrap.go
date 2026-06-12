package natsauth

import (
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// OperatorChain is the NSC trust root ppz-server needs at boot: the
// operator JWT + seed it signs per-org Account JWTs with, and the system
// account JWT the embedded NATS runs under.
type OperatorChain struct {
	OperatorJWT      string
	OperatorSeed     string
	SystemAccountJWT string
}

// BootstrapOperator mints a fresh operator + system-account chain.
// Shared by cmd/ppz-natsbootstrap (env-var output for compose/Pulumi)
// and ppz-server --ephemeral (in-process). Regenerating invalidates
// every previously issued JWT — callers own that decision.
func BootstrapOperator() (OperatorChain, error) {
	opKP, err := nkeys.CreateOperator()
	if err != nil {
		return OperatorChain{}, err
	}
	opPub, _ := opKP.PublicKey()
	opSeed, _ := opKP.Seed()

	sysKP, err := nkeys.CreateAccount()
	if err != nil {
		return OperatorChain{}, err
	}
	sysPub, _ := sysKP.PublicKey()

	opClaims := jwt.NewOperatorClaims(opPub)
	opClaims.Name = "ppz-operator"
	opClaims.SystemAccount = sysPub
	opJWT, err := opClaims.Encode(opKP)
	if err != nil {
		return OperatorChain{}, err
	}

	sysClaims := jwt.NewAccountClaims(sysPub)
	sysClaims.Name = "ppz-sys"
	sysJWT, err := sysClaims.Encode(opKP)
	if err != nil {
		return OperatorChain{}, err
	}

	return OperatorChain{
		OperatorJWT:      opJWT,
		OperatorSeed:     string(opSeed),
		SystemAccountJWT: sysJWT,
	}, nil
}
