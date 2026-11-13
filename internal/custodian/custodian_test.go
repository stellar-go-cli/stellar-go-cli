package custodian

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// ── store tests ─────────────────────────────

func TestGrantStore(t *testing.T) {
	g := newGrantStore()
	g.put("GABC", "SSEED", time.Minute)
	if got := g.seed("GABC"); got != "SSEED" {
		t.Fatalf("seed = %q", got)
	}
	if _, ok := g.expiry("GABC"); !ok {
		t.Fatal("expected expiry")
	}
	g.drop("GABC")
	if got := g.seed("GABC"); got != "" {
		t.Fatalf("seed after drop = %q", got)
	}
}

func TestGrantStoreExpiry(t *testing.T) {
	g := newGrantStore()
	g.put("GABC", "SSEED", -time.Second)
	if got := g.seed("GABC"); got != "" {
		t.Fatalf("expired grant returned seed %q", got)
	}
}

func TestApprovalStoreSingleUse(t *testing.T) {
	a := newApprovalStore(time.Minute)
	a.put("hash1")
	if !a.consume("hash1") {
		t.Fatal("first consume failed")
	}
	if a.consume("hash1") {
		t.Fatal("approval replayed")
	}
	if a.consume("other") {
		t.Fatal("unknown hash approved")
	}
}

func TestSeenSet(t *testing.T) {
	s := newSeenSet()
	if !s.claim("dec-1") || s.claim("dec-1") {
		t.Fatal("decision ref must be single-use")
	}
}

// ── policy tests ────────────────────────────

func testPolicy() *Policy {
	p := &Policy{Version: "test-1", AllowUnprotected: true}
	p.HardLimits.PerTx = map[string]string{"native": "75"}
	p.HardLimits.PerDay = map[string]string{"native": "10000"}
	p.HardLimits.Networks = []string{network.TestNetworkPassphrase}
	p.Escalation.T3Above = map[string]string{"native": "50"}
	return p
}

func TestPolicyChecks(t *testing.T) {
	p := testPolicy()
	if err := p.checkNetworks(network.TestNetworkPassphrase); err != nil {
		t.Fatalf("testnet allowed: %v", err)
	}
	if err := p.checkNetworks(network.PublicNetworkPassphrase); err == nil {
		t.Fatal("mainnet should be refused")
	}
	if err := p.checkPerTx("native", "5"); err != nil {
		t.Fatalf("under cap: %v", err)
	}
	if err := p.checkPerTx("native", "150"); err == nil {
		t.Fatal("over per-tx cap should refuse")
	}
	p.HardLimits.Deny = []string{"GDENY"}
	if !p.denylisted("GDENY") || p.denylisted("GOK") {
		t.Fatal("denylist mismatch")
	}
}

func TestAmountMath(t *testing.T) {
	over, err := exceeds("10.5", "10")
	if err != nil || !over {
		t.Fatalf("exceeds(10.5,10) = %v,%v", over, err)
	}
	if over, _ := exceeds("9.999", "10"); over {
		t.Fatal("under limit reported over")
	}
	if got := addAmount("1.5", "2.25"); got != "3.75" {
		t.Fatalf("addAmount = %s", got)
	}
}

// ── enforcement tests ───────────────────────

// fixture wires a temp HOME with one plaintext wallet on disk.
func fixture(t *testing.T, protected bool) (*Daemon, *keypair.Full, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	kp := keypair.MustRandom()
	acc := models.Account{
		Type:       models.WalletStellar,
		Address:    kp.Address(),
		PublicKey:  kp.Address(),
		PrivateKey: kp.Seed(),
		Network:    models.NetworkStellarTestnet,
	}
	if protected {
		acc.PasskeyProtected = true
		acc.PrivateKey = ""
		acc.PrivateKeyEnc = "deadbeef" // content irrelevant — grants hold the seed
	}
	walletsDir := filepath.Join(home, ".stellar-go-cli", "state", "wallets")
	if err := os.MkdirAll(walletsDir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(acc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(walletsDir, kp.Address()+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	var logBuf bytes.Buffer
	d := &Daemon{
		svc:             wallet.NewService(),
		policy:          testPolicy(),
		grants:          newGrantStore(),
		approvals:       newApprovalStore(5 * time.Minute),
		decisions:       newSeenSet(),
		counters:        loadCounters(filepath.Join(home, "limits-counters.json")),
		mandates:        newMandateStore(filepath.Join(home, "mandates")),
		mandateCounters: loadMandateCounters(filepath.Join(home, "mandate-counters.json")),
		parties:         loadCounterparties(filepath.Join(home, "counterparties.json")),
		pending:         newPendingCeremonies(),
		agents:          mustAgentRegistry(t, filepath.Join(home, "state")),
		pendingReq:      map[string]*signsock.PendingRequest{},
		escalations:     newEscalationLimiter(5, 10*time.Minute),
		stateDir:        filepath.Join(home, "state"),
		issued:          map[string]bool{},
		token:           "test-token",
		log:             chainlog.NewWriter(&logBuf, "", 0),
		logger:          log.New(io.Discard, "", 0),
	}
	return d, kp, &logBuf
}

func mustAgentRegistry(t *testing.T, stateDir string) *agentRegistry {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := loadAgentRegistry(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func unsignedTx(t *testing.T, kp *keypair.Full, dest, amount string) string {
	t.Helper()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: kp.Address(),
			Sequence:  1,
		},
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{Destination: dest, Amount: amount, Asset: txnbuild.NativeAsset{}},
		},
	})
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}
	b64, err := tx.Base64()
	if err != nil {
		t.Fatalf("encode tx: %v", err)
	}
	return b64
}

func TestEnforceSignsPlaintextWallet(t *testing.T) {
	d, kp, logBuf := fixture(t, false)
	dest := keypair.MustRandom().Address()
	xdr := unsignedTx(t, kp, dest, "5")

	signed, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-1", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr != nil {
		t.Fatalf("sign refused: %v", rerr)
	}
	gtx, err := txnbuild.TransactionFromXDR(signed)
	if err != nil {
		t.Fatalf("decode signed: %v", err)
	}
	tx, _ := gtx.Transaction()
	if len(tx.Signatures()) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(tx.Signatures()))
	}
	recs := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	if len(recs) != 1 || !strings.Contains(recs[0], `"signed:`) {
		t.Fatalf("enforcement record: %q", logBuf.String())
	}
}

func TestEnforceRefusesUnknownWallet(t *testing.T) {
	d, _, _ := fixture(t, false)
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", "GUNKNOWN", network.TestNetworkPassphrase, "AAAA", 0)
	if rerr == nil || !strings.Contains(rerr.Message, "unknown wallet") {
		t.Fatalf("expected unknown wallet refusal, got %v", rerr)
	}
}

func TestEnforceRefusesUndecodablePayload(t *testing.T) {
	d, kp, _ := fixture(t, false)
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, "!!!notxdr!!!", 0)
	if rerr == nil {
		t.Fatal("undecodable payload signed")
	}
}

func TestEnforceRefusesPerTxCap(t *testing.T) {
	d, kp, _ := fixture(t, false)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "150") // over 75 cap
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || rerr.Code != signsock.ErrRefused {
		t.Fatalf("expected cap refusal, got %v", rerr)
	}
}

func TestEnforceRefusesDenylisted(t *testing.T) {
	d, kp, _ := fixture(t, false)
	deny := keypair.MustRandom().Address()
	d.policy.HardLimits.Deny = []string{deny}
	xdr := unsignedTx(t, kp, deny, "1")
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || !strings.Contains(rerr.Message, "denylisted") {
		t.Fatalf("expected denylist refusal, got %v", rerr)
	}
}

func TestEnforceRefusesWrongNetwork(t *testing.T) {
	d, kp, _ := fixture(t, false)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "1")
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.PublicNetworkPassphrase, xdr, 0)
	if rerr == nil {
		t.Fatal("mainnet payload signed under testnet-only policy")
	}
}

func TestEnforceRefusesRequestHashMismatch(t *testing.T) {
	d, kp, _ := fixture(t, false)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "1")
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "deadbeef", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || !strings.Contains(rerr.Message, "request hash") {
		t.Fatalf("expected hash mismatch refusal, got %v", rerr)
	}
}

func TestEnforceEscalationRequired(t *testing.T) {
	d, kp, _ := fixture(t, true)                                   // protected wallet, no grant
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "60") // above T3Above 50
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	// No grant at all → E2 refusal first.
	if rerr == nil || !strings.Contains(rerr.Message, "passkey-protected") {
		t.Fatalf("expected locked-wallet refusal, got %v", rerr)
	}

	// Grant the seed, retry — now E3 fires (escalation threshold).
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)
	_, rerr = d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || rerr.Code != signsock.ErrEscalationRequired {
		t.Fatalf("expected escalation_required, got %v", rerr)
	}

	// Simulate bound approval — consume and sign succeeds.
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	rh := signsock.RequestHash("pay_send", kp.Address(), desc)
	d.approvals.put(rh)
	signed, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr != nil || signed == "" {
		t.Fatalf("post-approval sign failed: %v", rerr)
	}
}

func TestEnforceRefusesUnprotectedWhenPolicyDenies(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.AllowUnprotected = false
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "1")
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || !strings.Contains(rerr.Message, "unprotected") {
		t.Fatalf("expected unprotected refusal, got %v", rerr)
	}
}

func TestEnforceRefusesUnclassifiedOps(t *testing.T) {
	d, kp, _ := fixture(t, false)
	// A bump-sequence op carries no extractable amount → E1 refuse under default policy.
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &txnbuild.SimpleAccount{AccountID: kp.Address(), Sequence: 1},
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		Operations:           []txnbuild.Operation{&txnbuild.BumpSequence{BumpTo: 99}},
	})
	if err != nil {
		t.Fatal(err)
	}
	xdr, err := tx.Base64()
	if err != nil {
		t.Fatal(err)
	}
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "d", "", "pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || !strings.Contains(rerr.Message, "unclassified") {
		t.Fatalf("expected unclassified-op refusal, got %v", rerr)
	}
}

// ── socket round-trip ───────────────────────

func TestSocketRoundTripAndSanitization(t *testing.T) {
	d, kp, _ := fixture(t, false)
	sockPath := filepath.Join(os.TempDir(), "custodian-test-"+strings.ReplaceAll(t.Name(), "/", "-")+".sock")
	defer os.Remove(sockPath) //nolint:errcheck // best-effort cleanup

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup
	if err := os.Chmod(sockPath, 0600); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go d.serve(conn)
		}
	}()

	if err := os.WriteFile(filepath.Join(filepath.Dir(sockPath), "signer.token"), []byte("test-token"), 0600); err != nil {
		t.Fatal(err)
	}
	client := signsock.Dial(sockPath)
	rw := client.RemoteWallet()

	// status round-trip (empty grant state)
	st, err := rw.Status(kp.Address())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if unlocked, _ := st["unlocked"].(bool); unlocked {
		t.Fatal("expected locked")
	}

	// wallet_get sanitizes key material
	acc, err := rw.GetWalletByAddress(kp.Address())
	if err != nil {
		t.Fatalf("wallet_get: %v", err)
	}
	if acc.PrivateKey != "" || acc.PrivateKeyEnc != "" {
		t.Fatal("key material leaked through remote wallet_get")
	}
	if acc.Address != kp.Address() {
		t.Fatalf("address = %s", acc.Address)
	}

	// signsock.RemoteSigner produces a real signature over the socket.
	rs := signsock.NewRemoteSigner(client, kp.Address(), "pay_send", "")
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	tx, err := txnbuild.TransactionFromXDR(xdr)
	if err != nil {
		t.Fatal(err)
	}
	gtx, _ := tx.Transaction()
	signed, err := rs.Sign(gtx, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("remote sign: %v", err)
	}
	if len(signed.Signatures()) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(signed.Signatures()))
	}

	// Replay the same payload — the auto-derived local decision ref was
	// consumed, so E5 must refuse.
	if _, err := rs.Sign(gtx, network.TestNetworkPassphrase); err == nil {
		t.Fatal("replayed decision ref signed")
	}
}

func TestRemoteSignerFailClosed(t *testing.T) {
	rs := signsock.NewRemoteSigner(signsock.Dial(filepath.Join(t.TempDir(), "absent.sock")), "GABC", "pay_send", "")
	kp := keypair.MustRandom()
	xdrStr := unsignedTx(t, kp, keypair.MustRandom().Address(), "1")
	gtx, err := txnbuild.TransactionFromXDR(xdrStr)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := gtx.Transaction()
	if _, err := rs.Sign(tx, network.TestNetworkPassphrase); err == nil {
		t.Fatal("remote signer did not fail closed on missing socket")
	} else if !strings.Contains(err.Error(), "stellar-go-cli signer") {
		t.Fatalf("unactionable error: %v", err)
	}
}
