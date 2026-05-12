package natsauth

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// newTestAccount builds an Operator + Account chain in-memory and
// returns the Account (for runtime minting) plus the Operator JWT
// (for embedding into the test NATS server's TrustedOperators).
func newTestAccount(t *testing.T) (*Account, string) {
	t.Helper()

	opKP, err := nkeys.CreateOperator()
	if err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}
	opPub, _ := opKP.PublicKey()

	opClaims := jwt.NewOperatorClaims(opPub)
	opClaims.Name = "ppz-test-op"
	opJWT, err := opClaims.Encode(opKP)
	if err != nil {
		t.Fatalf("encode operator claims: %v", err)
	}

	accKP, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	accPub, _ := accKP.PublicKey()

	signKP, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("CreateAccount (signing): %v", err)
	}
	signPub, _ := signKP.PublicKey()

	accClaims := jwt.NewAccountClaims(accPub)
	accClaims.Name = "ppz-test"
	accClaims.SigningKeys.Add(signPub)
	accJWT, err := accClaims.Encode(opKP)
	if err != nil {
		t.Fatalf("encode account claims: %v", err)
	}

	return &Account{
		AccountPub: accPub,
		SigningKey: signKP,
		AccountJWT: accJWT,
	}, opJWT
}

// TestNATSJWT_MintForUser_ContainsExpectedSubjects: the User JWT
// returned by MintUserJWT must carry pub/sub permissions scoped to
// the caller's org.
func TestNATSJWT_MintForUser_ContainsExpectedSubjects(t *testing.T) {
	acc, _ := newTestAccount(t)
	const accountID = "alpha-uuid"

	tokStr, seed, err := acc.MintUserJWT(accountID, 5*time.Minute)
	if err != nil {
		t.Fatalf("MintUserJWT: %v", err)
	}
	if tokStr == "" || seed == "" {
		t.Fatalf("expected non-empty (jwt, seed); got %q, %q", tokStr, seed)
	}

	claims, err := jwt.DecodeUserClaims(tokStr)
	if err != nil {
		t.Fatalf("DecodeUserClaims: %v", err)
	}

	wantPub := accountID + ".>"
	wantSub := accountID + ".>"

	if !containsString(claims.Pub.Allow, wantPub) {
		t.Errorf("pub allow: want %q in %v", wantPub, claims.Pub.Allow)
	}
	if !containsString(claims.Sub.Allow, wantSub) {
		t.Errorf("sub allow: want %q in %v", wantSub, claims.Sub.Allow)
	}

	// Seed must be a parseable user nkey.
	kp, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		t.Fatalf("seed parse: %v", err)
	}
	pub, _ := kp.PublicKey()
	if !strings.HasPrefix(pub, "U") {
		t.Errorf("seed public key should be a User nkey (prefix U), got %q", pub)
	}

	// JWT must reference the same user public key.
	if claims.Subject != pub {
		t.Errorf("JWT subject %q != user pub %q", claims.Subject, pub)
	}
}

// TestNATSJWT_DenyOtherOrg: the JWT minted for org alpha cannot
// publish to a foreign org's subject. We can verify this directly
// from the claims (the actual NATS-side enforcement is covered in
// the integration tests).
func TestNATSJWT_DenyOtherOrg(t *testing.T) {
	acc, _ := newTestAccount(t)
	tokStr, _, err := acc.MintUserJWT("alpha-uuid", 5*time.Minute)
	if err != nil {
		t.Fatalf("MintUserJWT: %v", err)
	}
	claims, err := jwt.DecodeUserClaims(tokStr)
	if err != nil {
		t.Fatalf("DecodeUserClaims: %v", err)
	}

	for _, p := range claims.Pub.Allow {
		if strings.HasPrefix(p, "beta-uuid.") || p == ">" {
			t.Errorf("pub allow includes a foreign-org pattern %q (claims=%v)", p, claims.Pub.Allow)
		}
	}
	for _, p := range claims.Sub.Allow {
		if strings.HasPrefix(p, "beta-uuid.") || p == ">" {
			t.Errorf("sub allow includes a foreign-org pattern %q (claims=%v)", p, claims.Sub.Allow)
		}
	}
}

// TestAccountKey_LoadFromEnv: PPZ_NATS_ACCOUNT_JWT +
// PPZ_NATS_ACCOUNT_SIGNING_SEED parse cleanly into an *Account.
// Missing/malformed values must fail fast.
func TestAccountKey_LoadFromEnv(t *testing.T) {
	// Build a real Account chain so we have valid env values.
	acc, _ := newTestAccount(t)
	signSeed, err := acc.SigningKey.Seed()
	if err != nil {
		t.Fatalf("SigningKey.Seed: %v", err)
	}

	t.Run("happy path", func(t *testing.T) {
		t.Setenv("PPZ_NATS_ACCOUNT_JWT", acc.AccountJWT)
		t.Setenv("PPZ_NATS_ACCOUNT_SIGNING_SEED", string(signSeed))
		got, err := LoadAccountFromEnv()
		if err != nil {
			t.Fatalf("LoadAccountFromEnv: %v", err)
		}
		if got.AccountJWT != acc.AccountJWT {
			t.Errorf("AccountJWT mismatch")
		}
		if got.AccountPub != acc.AccountPub {
			t.Errorf("AccountPub: got %q want %q", got.AccountPub, acc.AccountPub)
		}
		if got.SigningKey == nil {
			t.Errorf("SigningKey is nil")
		}
	})

	t.Run("missing JWT errors", func(t *testing.T) {
		t.Setenv("PPZ_NATS_ACCOUNT_JWT", "")
		t.Setenv("PPZ_NATS_ACCOUNT_SIGNING_SEED", string(signSeed))
		if _, err := LoadAccountFromEnv(); err == nil {
			t.Errorf("missing PPZ_NATS_ACCOUNT_JWT should error")
		}
	})

	t.Run("malformed seed errors", func(t *testing.T) {
		t.Setenv("PPZ_NATS_ACCOUNT_JWT", acc.AccountJWT)
		t.Setenv("PPZ_NATS_ACCOUNT_SIGNING_SEED", "not-a-real-seed")
		if _, err := LoadAccountFromEnv(); err == nil {
			t.Errorf("malformed seed should error")
		}
	})
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
