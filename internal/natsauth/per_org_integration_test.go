package natsauth

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// startTestNATSWithTwoAccounts boots an embedded NATS with the
// operator + sys + two tenant accounts pre-loaded into its
// resolver. Returns the URL plus the two tenants' (acc, sign)
// keypairs so callers can mint user JWTs for either.
type tenant struct {
	name        string
	accKP       nkeys.KeyPair
	signKP      nkeys.KeyPair
	accPub      string
	accountJWT  string
}

func startTestNATSWithTwoAccounts(t *testing.T) (url string, stop func(), acme, globex tenant) {
	t.Helper()

	opKP, _ := nkeys.CreateOperator()
	opPub, _ := opKP.PublicKey()
	sysKP, _ := nkeys.CreateAccount()
	sysPub, _ := sysKP.PublicKey()

	opClaims := jwt.NewOperatorClaims(opPub)
	opClaims.Name = "test-op"
	opClaims.SystemAccount = sysPub
	opJWT, _ := opClaims.Encode(opKP)

	sysClaims := jwt.NewAccountClaims(sysPub)
	sysClaims.Name = "test-sys"
	sysJWT, _ := sysClaims.Encode(opKP)

	mkTenant := func(name string) tenant {
		accKP, _ := nkeys.CreateAccount()
		accPub, _ := accKP.PublicKey()
		signKP, _ := nkeys.CreateAccount()
		j := mintAccountForTest(t, opKP, name, accKP, signKP)
		return tenant{name: name, accKP: accKP, signKP: signKP, accPub: accPub, accountJWT: j}
	}
	acme = mkTenant("tenant-acme")
	globex = mkTenant("tenant-globex")

	resolver := &natsserver.MemAccResolver{}
	if err := resolver.Store(sysPub, sysJWT); err != nil {
		t.Fatalf("store sys acct: %v", err)
	}
	if err := resolver.Store(acme.accPub, acme.accountJWT); err != nil {
		t.Fatalf("store acme acct: %v", err)
	}
	if err := resolver.Store(globex.accPub, globex.accountJWT); err != nil {
		t.Fatalf("store globex acct: %v", err)
	}

	opsClaims, _ := jwt.DecodeOperatorClaims(opJWT)
	opts := &natsserver.Options{
		Host:             "127.0.0.1",
		Port:             0,
		TrustedOperators: []*jwt.OperatorClaims{opsClaims},
		AccountResolver:  resolver,
		SystemAccount:    sysPub,
	}

	ns, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("nats new: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatalf("nats not ready")
	}
	return ns.ClientURL(), func() { ns.Shutdown() }, acme, globex
}

// connectAs is a small helper that establishes a NATS conn using a
// minted user JWT in the given tenant. Subjects allowed: ">" by
// default for these isolation tests — wide-open inside-account
// permissions, since the *account boundary* is what we're testing.
func connectAs(t *testing.T, url string, tn tenant) *nats.Conn {
	t.Helper()
	exp := time.Now().Add(5 * time.Minute).Unix()
	jwtStr, seedStr, err := MintUserJWTInAccount(
		tn.accPub, tn.signKP, "test-user", []string{">"}, []string{">"}, exp)
	if err != nil {
		t.Fatalf("mint user (%s): %v", tn.name, err)
	}
	kp, err := nkeys.FromSeed([]byte(seedStr))
	if err != nil {
		t.Fatalf("user seed parse: %v", err)
	}
	nc, err := nats.Connect(url,
		nats.UserJWT(
			func() (string, error) { return jwtStr, nil },
			func(nonce []byte) ([]byte, error) { return kp.Sign(nonce) },
		),
		nats.Timeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("nats connect (%s): %v", tn.name, err)
	}
	return nc
}

// TestEmbeddedNATS_PerOrgAccount_DataPlaneIsolation: two users in
// two different accounts using the *same* subject string don't see
// each other's messages. The account boundary scopes the namespace
// without needing a subject prefix.
func TestEmbeddedNATS_PerOrgAccount_DataPlaneIsolation(t *testing.T) {
	url, stop, acme, globex := startTestNATSWithTwoAccounts(t)
	defer stop()

	ncA := connectAs(t, url, acme)
	defer ncA.Close()
	ncG := connectAs(t, url, globex)
	defer ncG.Close()

	subA, err := ncA.SubscribeSync("shared.subject")
	if err != nil {
		t.Fatalf("acme sub: %v", err)
	}
	if err := ncA.Flush(); err != nil {
		t.Fatalf("acme flush: %v", err)
	}

	if err := ncG.Publish("shared.subject", []byte("from-globex")); err != nil {
		t.Fatalf("globex publish: %v", err)
	}
	_ = ncG.Flush()

	if msg, err := subA.NextMsg(500 * time.Millisecond); err == nil {
		t.Errorf("acme should NOT have received globex's message; got %q", msg.Data)
	}
	// And the converse — within the same account, msgs flow normally.
	if err := ncA.Publish("shared.subject", []byte("from-acme")); err != nil {
		t.Fatalf("acme publish: %v", err)
	}
	_ = ncA.Flush()
	if msg, err := subA.NextMsg(2 * time.Second); err != nil {
		t.Errorf("acme should have received its own message: %v", err)
	} else if string(msg.Data) != "from-acme" {
		t.Errorf("got %q want %q", msg.Data, "from-acme")
	}
}

// TestEmbeddedNATS_PerOrgAccount_JSAPIControlPlaneIsolation: closes
// the gap empirically demonstrated against prod in Phase 3. A user
// JWT in account-A cannot manipulate streams in account-B even via
// the JS API. The smoking-gun: STREAM.PURGE against an existing
// foreign stream returns "stream not found" because each account
// has its own JS API namespace — NOT because the target stream
// happens to be absent.
func TestEmbeddedNATS_PerOrgAccount_JSAPIControlPlaneIsolation(t *testing.T) {
	t.Skip("requires JetStream-enabled per-account NATS server boot — implement in 3.5")
	// Sketch: enable JS on both accounts, create stream `streamX` in
	// globex via globex's user, then have acme's user attempt
	// STREAM.PURGE on `streamX`. Verify the JS API response says
	// "stream not found in this account" (or equivalent) — proving
	// account-scoped routing rather than shared-account auth bypass.
	//
	// Distinguishing flag: in shared-account mode, prod returned
	// "stream not found" too — but the call REACHED the JS API.
	// In account-scoped mode the call shouldn't even route to the
	// other account's JS instance.
}

// TestEmbeddedNATS_AccountDeletedMidConnection: when a tenant
// account is deleted (org removed), connections under that account
// must be terminated and new ones rejected. Tests the cleanup path.
func TestEmbeddedNATS_AccountDeletedMidConnection(t *testing.T) {
	url, stop, acme, _ := startTestNATSWithTwoAccounts(t)
	defer stop()

	ncA := connectAs(t, url, acme)
	defer ncA.Close()

	// Verify connection is live.
	if err := ncA.Publish("ping", []byte("x")); err != nil {
		t.Fatalf("pre-deletion publish: %v", err)
	}
	_ = ncA.Flush()

	// TODO Phase 3.5: delete the acme account JWT from the resolver
	// here and verify ncA is disconnected within ~1s, with subsequent
	// publishes returning ErrConnectionClosed or auth-violation.
	t.Skip("requires resolver.Delete() + connection-eviction wiring — implement in 3.5")
	_ = strings.Contains // silence unused import in stub
}
