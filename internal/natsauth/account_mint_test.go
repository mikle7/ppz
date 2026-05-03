package natsauth

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// TestMintAccountJWT_SignedByOperator: an account JWT minted by
// MintAccountJWT decodes cleanly, names the supplied org, and lists
// the supplied signing key in its authorized-signers set.
func TestMintAccountJWT_SignedByOperator(t *testing.T) {
	opKP, err := nkeys.CreateOperator()
	if err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}

	accKP, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	signKP, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("CreateAccount (signing): %v", err)
	}
	signPub, _ := signKP.PublicKey()
	accPub, _ := accKP.PublicKey()

	jwtStr, err := MintAccountJWT(opKP, "tenant-acme", accKP, signKP)
	if err != nil {
		t.Fatalf("MintAccountJWT: %v", err)
	}

	claims, err := jwt.DecodeAccountClaims(jwtStr)
	if err != nil {
		t.Fatalf("DecodeAccountClaims: %v", err)
	}

	if claims.Subject != accPub {
		t.Errorf("subject: got %q want account pub %q", claims.Subject, accPub)
	}
	if !strings.Contains(claims.Name, "acme") {
		t.Errorf("name should mention org: got %q", claims.Name)
	}
	if !claims.SigningKeys.Contains(signPub) {
		t.Errorf("signing keys missing %q (set: %v)", signPub, claims.SigningKeys)
	}
}

// TestMintUserJWT_DifferentAccounts_DifferentSigners: minting the
// same user in two distinct accounts produces JWTs with different
// issuers (the account signing key) and different IssuerAccount
// claims. Validates that one user can hold two valid JWTs, one per
// (user, org) pair.
func TestMintUserJWT_DifferentAccounts_DifferentSigners(t *testing.T) {
	// Build two accounts (acme + globex), each with its own signing key.
	opKP, _ := nkeys.CreateOperator()

	accmeKP, _ := nkeys.CreateAccount()
	accmePub, _ := accmeKP.PublicKey()
	accmeSign, _ := nkeys.CreateAccount()
	accmeSignPub, _ := accmeSign.PublicKey()
	accmeJWT := mintAccountForTest(t, opKP, "tenant-acme", accmeKP, accmeSign)

	globKP, _ := nkeys.CreateAccount()
	globPub, _ := globKP.PublicKey()
	globSign, _ := nkeys.CreateAccount()
	globSignPub, _ := globSign.PublicKey()
	globJWT := mintAccountForTest(t, opKP, "tenant-globex", globKP, globSign)

	_ = accmeJWT
	_ = globJWT

	exp := time.Now().Add(5 * time.Minute).Unix()
	acmeUserJWT, _, err := MintUserJWTInAccount(accmePub, accmeSign, "alice", []string{">"}, []string{">"}, exp)
	if err != nil {
		t.Fatalf("acme user mint: %v", err)
	}
	globUserJWT, _, err := MintUserJWTInAccount(globPub, globSign, "alice", []string{">"}, []string{">"}, exp)
	if err != nil {
		t.Fatalf("globex user mint: %v", err)
	}

	a, err := jwt.DecodeUserClaims(acmeUserJWT)
	if err != nil {
		t.Fatalf("decode acme user: %v", err)
	}
	g, err := jwt.DecodeUserClaims(globUserJWT)
	if err != nil {
		t.Fatalf("decode globex user: %v", err)
	}

	if a.Issuer == g.Issuer {
		t.Errorf("issuers should differ between accounts (signing keys are per-account); both = %q", a.Issuer)
	}
	if a.Issuer != accmeSignPub {
		t.Errorf("acme issuer: got %q want %q", a.Issuer, accmeSignPub)
	}
	if g.Issuer != globSignPub {
		t.Errorf("globex issuer: got %q want %q", g.Issuer, globSignPub)
	}
	if a.IssuerAccount != accmePub {
		t.Errorf("acme IssuerAccount: got %q want %q", a.IssuerAccount, accmePub)
	}
	if g.IssuerAccount != globPub {
		t.Errorf("globex IssuerAccount: got %q want %q", g.IssuerAccount, globPub)
	}
}

// mintAccountForTest is a thin helper so the integration tests can
// build account chains. Once MintAccountJWT is implemented this
// reuses it; with the stub it still tries (and fails the test loud).
func mintAccountForTest(t *testing.T, opKP nkeys.KeyPair, name string, accKP, signKP nkeys.KeyPair) string {
	t.Helper()
	jwtStr, err := MintAccountJWT(opKP, name, accKP, signKP)
	if err != nil {
		t.Fatalf("mintAccountForTest: %v", err)
	}
	return jwtStr
}
