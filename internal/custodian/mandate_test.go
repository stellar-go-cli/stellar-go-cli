package custodian

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	vc2 "github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// ── helpers ─────────────────────────────────────────────

// mintMandate issues a real mandate VC inside the test and stores it — the
// same shape issueMandate produces after its (untestable) WebAuthn ceremony.
func mintMandate(t *testing.T, d *Daemon, kp *keypair.Full, agentDID string, scope []string, until time.Time) (*mandateRecord, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv // mandate VCs are issued by the principal, not the agent

	rawSeed, err := strkey.Decode(strkey.VersionByteSeed, kp.Seed())
	if err != nil {
		t.Fatal(err)
	}
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	issuerDID := mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey))

	sub := &mandateSubject{
		Agent:     agentDID,
		Principal: kp.Address(),
		Scope:     scope,
		PerTx:     map[string]string{"native": "100"},
		Nonce:     "deadbeefcafe",
	}
	svc := vc2.NewServiceWithEd25519Key(edPriv)
	mvc, err := svc.IssueVCWithValidity(issuerDID, vcTypeMandate, subjectToMap(sub), until)
	if err != nil {
		t.Fatalf("issue mandate VC: %v", err)
	}
	rec := &mandateRecord{VC: mvc, Subject: sub, Status: mandateActive}
	if err := d.mandates.put(mvc.ID, rec); err != nil {
		t.Fatalf("store mandate: %v", err)
	}
	return rec, pub
}

// newAgent creates an agent identity (did:key + signer) as the gatekeeper would.
func newAgent(t *testing.T) (string, func([]byte) (string, error)) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	did := mpCrypto.DIDKeyFromEd25519PublicKey(pub)
	signerFn := func(preimage []byte) (string, error) {
		return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, preimage)), nil
	}
	return did, signerFn
}

// signedPOP returns the base64 POP for a request, as the gatekeeper computes it.
func signPOP(t *testing.T, signerFn func([]byte) (string, error), tool, account, xdr string) string {
	t.Helper()
	pop, err := signerFn(signsock.AgentPOPPreimage(tool, account, xdr))
	if err != nil {
		t.Fatal(err)
	}
	return pop
}

// ── store tests ─────────────────────────────────────────

func TestMandateStoreRoundTrip(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	loaded, err := d.mandates.load(rec.VC.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Subject.Agent != agentDID || loaded.Status != mandateActive {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}

	list := d.mandates.list(kp.Address())
	if len(list) != 1 || list[0]["id"] != rec.VC.ID {
		t.Fatalf("list: %+v", list)
	}
	// Filter by other account excludes it.
	if got := d.mandates.list(keypair.MustRandom().Address()); len(got) != 0 {
		t.Fatalf("unfiltered list leaked mandate: %+v", got)
	}
}

func TestMandateStatusTransitions(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	if err := d.mandates.setStatus(rec.VC.ID, mandateSuspended); err != nil {
		t.Fatal(err)
	}
	loaded, _ := d.mandates.load(rec.VC.ID)
	if loaded.Status != mandateSuspended {
		t.Fatalf("status not persisted: %s", loaded.Status)
	}
	if err := d.mandates.setStatus(rec.VC.ID, "bogus"); err == nil {
		t.Fatal("bogus status accepted")
	}
}

// ── verifyMandate paths ─────────────────────────────────

func TestVerifyMandateValid(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "x", time.Now()); err != nil {
		t.Fatalf("valid mandate refused: %v", err)
	}
}

func TestVerifyMandateRejectsStatus(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	for _, status := range []string{mandateSuspended, mandateRevoked, "bogus"} {
		rec.Status = status
		if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "x", time.Now()); err == nil {
			t.Fatalf("status %q accepted", status)
		}
	}
}

func TestVerifyMandateExpired(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(-time.Minute)) // issued in past, expired

	if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "x", time.Now()); err == nil {
		t.Fatal("expired mandate accepted")
	}
}

func TestVerifyMandateWrongPrincipal(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	other := keypair.MustRandom().Address()
	if err := verifyMandate(rec, other, "pay_send", nil, "x", time.Now()); err == nil {
		t.Fatal("mandate accepted for wrong account")
	}
}

func TestVerifyMandateScopeMiss(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	if err := verifyMandate(rec, kp.Address(), "wallet_remove", nil, "x", time.Now()); err == nil {
		t.Fatal("out-of-scope tool accepted")
	}
	// Wildcard coverage.
	if !scopeCovers([]string{"pay_*"}, "pay_x402") {
		t.Fatal("wildcard scope did not cover pay_x402")
	}
}

func TestVerifyMandateSubDelegationRefused(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	rec.Subject.Delegation = true

	if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "x", time.Now()); err == nil ||
		!strings.Contains(err.Error(), "sub-delegation") {
		t.Fatalf("sub-delegated mandate not refused: %v", err)
	}
}

func TestVerifyMandatePerTxCap(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	rec.Subject.PerTx = map[string]string{"native": "5"}

	desc := &signsock.PayloadDesc{Ops: []signsock.OpDesc{{Type: "payment", Asset: "native", Amount: "10", Destination: keypair.MustRandom().Address()}}}
	if err := verifyMandate(rec, kp.Address(), "pay_send", desc, "x", time.Now()); err == nil {
		t.Fatal("over-cap amount accepted")
	}
	desc.Ops[0].Amount = "3"
	if err := verifyMandate(rec, kp.Address(), "pay_send", desc, "x", time.Now()); err != nil {
		t.Fatalf("under-cap amount refused: %v", err)
	}
}

func TestVerifyMandateCounterpartyAllowlist(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	allowed := keypair.MustRandom().Address()
	rec.Subject.Counterparties = []string{allowed}

	desc := &signsock.PayloadDesc{Ops: []signsock.OpDesc{{Type: "payment", Asset: "native", Amount: "1", Destination: keypair.MustRandom().Address()}}}
	if err := verifyMandate(rec, kp.Address(), "pay_send", desc, "x", time.Now()); err == nil {
		t.Fatal("non-allowlisted counterparty accepted")
	}
	desc.Ops[0].Destination = allowed
	if err := verifyMandate(rec, kp.Address(), "pay_send", desc, "x", time.Now()); err != nil {
		t.Fatalf("allowlisted counterparty refused: %v", err)
	}
}

func TestVerifyMandateClosedDigest(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	rec.Subject.PayloadDigest = "sha256:expected"

	if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "sha256:different", time.Now()); err == nil {
		t.Fatal("closed mandate accepted wrong digest")
	}
	if err := verifyMandate(rec, kp.Address(), "pay_send", nil, "sha256:expected", time.Now()); err != nil {
		t.Fatalf("closed mandate refused matching digest: %v", err)
	}
}

// ── holder binding ──────────────────────────────────────

func TestVerifyAgentPOP(t *testing.T) {
	did, signerFn := newAgent(t)
	pop, err := signerFn(signsock.AgentPOPPreimage("pay_send", "GACC", "PAYLOAD"))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyAgentPOP(did, pop, "pay_send", "GACC", "PAYLOAD"); err != nil {
		t.Fatalf("valid POP refused: %v", err)
	}
	// Tampered payload → wrong preimage.
	if err := verifyAgentPOP(did, pop, "pay_send", "GACC", "OTHER"); err == nil {
		t.Fatal("POP accepted for different payload")
	}
	// Wrong DID (different key).
	other, _ := newAgent(t)
	if err := verifyAgentPOP(other, pop, "pay_send", "GACC", "PAYLOAD"); err == nil {
		t.Fatal("POP accepted under a different agent DID")
	}
	// Garbage input.
	if err := verifyAgentPOP(did, "!!!", "pay_send", "GACC", "PAYLOAD"); err == nil {
		t.Fatal("garbage POP accepted")
	}
}

// ── enforcement path ────────────────────────────────────

func TestMandateSignsPlaintextWallet(t *testing.T) {
	d, kp, logBuf := fixture(t, false)
	d.policy.AllowUnprotected = true

	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	dest := keypair.MustRandom().Address()
	xdr := unsignedTx(t, kp, dest, "5")
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)

	signed, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-m1", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr != nil {
		t.Fatalf("mandated sign refused: %v", rerr)
	}
	if !strings.Contains(logBuf.String(), `"signed:`) {
		t.Fatalf("no signed record: %q", logBuf.String())
	}
	if signed == "" {
		t.Fatal("empty signed payload")
	}
}

func TestMandateRefusedWithoutPOP(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.AllowUnprotected = true
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-m2", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, "", false)
	if rerr == nil || !strings.Contains(rerr.Message, "holder binding") {
		t.Fatalf("missing POP not refused: %v", rerr)
	}
}

func TestMandateRefusedWrongAgentKey(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.AllowUnprotected = true
	agentDID, _ := newAgent(t) // mandate bound to this agent
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	// Attacker signs POP with their own key — a stolen mandate can't be replayed.
	_, attackerSigner := newAgent(t)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	pop := signPOP(t, attackerSigner, "pay_send", kp.Address(), xdr)

	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-m3", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr == nil {
		t.Fatal("POP from wrong key accepted")
	}
}

func TestMandateEscrowSignsProtectedWallet(t *testing.T) {
	d, kp, _ := fixture(t, true)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))

	// Escrow the seed as issueMandate does — no ordinary unlock grant exists.
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)

	signed, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-m4", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr != nil {
		t.Fatalf("protected-wallet mandate sign refused: %v", rerr)
	}
	if signed == "" {
		t.Fatal("empty signed payload")
	}
}

func TestMandateProtectedWalletWithoutEscrow(t *testing.T) {
	d, kp, _ := fixture(t, true)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	// No escrow — mandate alone can't conjure the seed.
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)

	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-m5", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr == nil || !strings.Contains(rerr.Message, "escrow") {
		t.Fatalf("protected wallet signed without escrow: %v", rerr)
	}
}

func TestMandateEscrowDroppedOnRevoke(t *testing.T) {
	d, kp, _ := fixture(t, true)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	if d.grants.seed("mandate:"+rec.VC.ID) == "" {
		t.Fatal("escrow missing before revoke")
	}
	res := d.applyMandateStatus(rec.VC.ID, mandateRevoked, kp.Seed())
	if e, _ := res["error"].(string); e != "" {
		t.Fatalf("revoke failed: %s", e)
	}
	if d.grants.seed("mandate:"+rec.VC.ID) != "" {
		t.Fatal("escrow survived revocation")
	}
}

func TestMandateCumulativeCapEnforced(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.AllowUnprotected = true
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_send"}, time.Now().Add(time.Hour))
	rec.Subject.Cumulative = map[string]string{"native": "7"}
	if err := d.mandates.put(rec.VC.ID, rec); err != nil {
		t.Fatal(err)
	}

	dest := keypair.MustRandom().Address()
	for i, amount := range []string{"5", "5"} {
		xdr := unsignedTx(t, kp, dest, amount)
		pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
		_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, fmt.Sprintf("dec-c%d", i), "", "pay_send",
			kp.Address(), network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
		if i == 0 && rerr != nil {
			t.Fatalf("first sign refused: %v", rerr)
		}
		if i == 1 {
			if rerr == nil || !strings.Contains(rerr.Message, "mandate") {
				t.Fatalf("cumulative cap not enforced: %v", rerr)
			}
		}
	}
}

// ── sign_credential ─────────────────────────────────────

func TestSignCredentialPlaintextWallet(t *testing.T) {
	d, kp, _ := fixture(t, false)

	rawSeed, err := strkey.Decode(strkey.VersionByteSeed, kp.Seed())
	if err != nil {
		t.Fatal(err)
	}
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	issuerDID := mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey))

	unsigned, _ := json.Marshal(map[string]interface{}{
		"type":              []string{"VerifiableCredential", "NationalIdentityCredential"},
		"credentialSubject": map[string]interface{}{"id": "did:key:zSubject", "name": "Ada"},
	})
	sres, rerr := d.signCredential(&signsock.SignCredentialParams{
		Account: kp.Address(), IssuerDID: issuerDID, UnsignedVC: string(unsigned),
	})
	if rerr != nil {
		t.Fatalf("sign_credential refused: %v", rerr)
	}
	signedVC, _ := sres["signed_vc"].(string)
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(signedVC), &doc); err != nil {
		t.Fatal(err)
	}
	ok, err := vc2.VerifyDocument(doc)
	if err != nil || !ok {
		t.Fatalf("signed credential does not verify: %v ok=%v", err, ok)
	}
}

func TestSignCredentialForeignIssuerRefused(t *testing.T) {
	d, kp, _ := fixture(t, false)
	foreign := mpCrypto.DIDKeyFromEd25519PublicKey(func() ed25519.PublicKey {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		return pub
	}())
	unsigned, _ := json.Marshal(map[string]interface{}{
		"type":              []string{"VerifiableCredential"},
		"credentialSubject": map[string]interface{}{"id": "x"},
	})
	_, rerr := d.signCredential(&signsock.SignCredentialParams{
		Account: kp.Address(), IssuerDID: foreign, UnsignedVC: string(unsigned),
	})
	if rerr == nil || !strings.Contains(rerr.Message, "not derived") {
		t.Fatalf("foreign issuer not refused: %v", rerr)
	}
}

// ── residuals: token auth, submit_tx, ceremony relay, ECDSA ──────────────

func TestDispatchRefusesWithoutToken(t *testing.T) {
	d, kp, _ := fixture(t, false)
	sockPath := filepath.Join(t.TempDir(), "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck // test cleanup
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go d.serve(conn)
		}
	}()

	// signsock.Client without token file/env → refused.
	client := signsock.NewClient(sockPath, "")
	var out map[string]interface{}
	err = client.Call(signsock.MethodStatus, map[string]string{"address": kp.Address()}, &out)
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("unauthenticated call not refused: %v", err)
	}

	// Wrong token → refused.
	client2 := signsock.NewClient(sockPath, "wrong")
	if err = client2.Call(signsock.MethodStatus, map[string]string{"address": kp.Address()}, &out); err == nil {
		t.Fatal("wrong token accepted")
	}

	// Right token → works.
	client3 := signsock.NewClient(sockPath, "test-token")
	if err = client3.Call(signsock.MethodStatus, map[string]string{"address": kp.Address()}, &out); err != nil {
		t.Fatalf("token auth failed: %v", err)
	}
}

func TestSubmitTxRefusesUnknownPayload(t *testing.T) {
	d, kp, _ := fixture(t, false)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	_, rerr := d.submitTx(&signsock.SubmitTxParams{SignedXDR: xdr})
	if rerr == nil || !strings.Contains(rerr.Message, "unsigned") {
		t.Fatalf("unsigned payload not refused: %v", rerr)
	}
}

func TestSubmitTxRefusesNotIssued(t *testing.T) {
	d, kp, _ := fixture(t, false)
	// Sign locally with the keypair directly — not via the daemon.
	gtx, derr := txnbuild.TransactionFromXDR(unsignedTx(t, kp, keypair.MustRandom().Address(), "5"))
	if derr != nil {
		t.Fatal(derr)
	}
	utx, _ := gtx.Transaction()
	signed, err := signer.KeypairSigner{KP: kp}.Sign(utx, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	b64, err := signed.Base64()
	if err != nil {
		t.Fatal(err)
	}
	_, rerr := d.submitTx(&signsock.SubmitTxParams{SignedXDR: b64})
	if rerr == nil || !strings.Contains(rerr.Message, "not signed by this custodian") {
		t.Fatalf("foreign-signed payload not refused: %v", rerr)
	}
}

func TestPendingCeremonySingleUse(t *testing.T) {
	p := newPendingCeremonies()
	p.put(&pendingCeremony{id: "c1", action: "unlock", deadline: time.Now().Add(time.Minute)})
	if p.take("c1") == nil {
		t.Fatal("pending ceremony missing")
	}
	if p.take("c1") != nil {
		t.Fatal("ceremony replayed — must be single-use")
	}
	p.put(&pendingCeremony{id: "c2", action: "unlock", deadline: time.Now().Add(-time.Minute)})
	if p.take("c2") != nil {
		t.Fatal("expired ceremony accepted")
	}
}

func TestFinishCeremonyUnknownID(t *testing.T) {
	d, _, _ := fixture(t, false)
	resp := d.finishCeremony(&signsock.AssertParams{CeremonyID: "nope", AssertionJSON: "{}"})
	if resp.Error == nil {
		t.Fatal("unknown ceremony id accepted")
	}
}

func TestSignCredentialECDSAJWS(t *testing.T) {
	d, kp, _ := fixture(t, false)
	unsigned, _ := json.Marshal(map[string]interface{}{
		"type":              []string{"VerifiableCredential", "TestCredential"},
		"credentialSubject": map[string]interface{}{"id": "did:example:x"},
	})
	res, rerr := d.signCredential(&signsock.SignCredentialParams{
		Account: kp.Address(), IssuerMethod: "did:key", ProofType: "jws", UnsignedVC: string(unsigned),
	})
	if rerr != nil {
		t.Fatalf("jws sign_credential refused: %v", rerr)
	}
	issuerDID, _ := res["issuer_did"].(string)
	signedVC, _ := res["signed_vc"].(string)
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(signedVC), &doc); err != nil {
		t.Fatal(err)
	}
	// Issuer must be the ECDSA-derived did:key.
	ecdsaKey, _ := mpCrypto.DeriveKeyFromSeed(kp.Seed())
	want := mpCrypto.DIDKeyFromPublicKey(&ecdsaKey.PublicKey)
	if issuerDID != want {
		t.Fatalf("issuer %s != ECDSA-derived %s", issuerDID, want)
	}
	proof, _ := doc["proof"].(map[string]interface{})
	if jws, _ := proof["jws"].(string); jws == "" {
		t.Fatalf("expected JWS proof, got %+v", proof)
	}
}

func TestSignCredentialEthrIssuer(t *testing.T) {
	d, kp, _ := fixture(t, false)
	unsigned, _ := json.Marshal(map[string]interface{}{
		"type":              []string{"VerifiableCredential"},
		"credentialSubject": map[string]interface{}{"id": "x"},
	})
	res, rerr := d.signCredential(&signsock.SignCredentialParams{
		Account: kp.Address(), IssuerMethod: "did:ethr", ProofType: "jws", UnsignedVC: string(unsigned),
	})
	if rerr != nil {
		t.Fatalf("did:ethr refused: %v", rerr)
	}
	issuerDID, _ := res["issuer_did"].(string)
	if !strings.HasPrefix(issuerDID, "did:ethr:") {
		t.Fatalf("expected did:ethr issuer, got %s", issuerDID)
	}
}

// ── status lists ─────────────────────────────────────────────────────────

func TestStatusListIssuanceAndExport(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	until := time.Now().Add(time.Hour)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, until)

	rawSeed, _ := strkey.Decode(strkey.VersionByteSeed, kp.Seed())
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	issuerDID := mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey))

	idx, err := d.assignStatusIndex(issuerDID, rec.VC.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.reissueStatusLists(issuerDID, kp.Seed()); err != nil {
		t.Fatalf("reissue: %v", err)
	}

	lists, err := d.exportStatusLists(issuerDID)
	if err != nil {
		t.Fatal(err)
	}
	revDoc, ok := lists["revocation"].(map[string]interface{})
	if !ok {
		t.Fatalf("no revocation list: %v", lists)
	}
	// List VC verifies cryptographically.
	if ok, verr := vc2.VerifyDocument(revDoc); !ok || verr != nil {
		t.Fatalf("status list VC doesn't verify: %v %v", ok, verr)
	}
	subject, _ := revDoc["credentialSubject"].(map[string]interface{})
	enc, _ := subject["encodedList"].(string)
	set, derr := vc2.DecodeBitstring(enc, idx)
	if derr != nil || set {
		t.Fatalf("active mandate appears revoked: set=%v err=%v", set, derr)
	}

	// Revoke → bit flips on next reissue.
	res := d.applyMandateStatus(rec.VC.ID, mandateRevoked, kp.Seed())
	if res["status_lists"] != "updated" {
		t.Fatalf("lists not updated: %v", res)
	}
	lists, _ = d.exportStatusLists(issuerDID)
	revDoc = lists["revocation"].(map[string]interface{})
	subject = revDoc["credentialSubject"].(map[string]interface{})
	set, derr = vc2.DecodeBitstring(subject["encodedList"].(string), idx)
	if derr != nil || !set {
		t.Fatalf("revoked mandate bit not set: set=%v err=%v", set, derr)
	}
}

func TestStatusListIndexStable(t *testing.T) {
	d, kp, _ := fixture(t, false)
	rawSeed, _ := strkey.Decode(strkey.VersionByteSeed, kp.Seed())
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	issuerDID := mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey))

	i1, _ := d.assignStatusIndex(issuerDID, "mandate-1")
	i2, _ := d.assignStatusIndex(issuerDID, "mandate-2")
	i1b, _ := d.assignStatusIndex(issuerDID, "mandate-1")
	if i1 != i1b || i1 == i2 {
		t.Fatalf("index allocation unstable: %d %d %d", i1, i2, i1b)
	}
}

func TestBitstringRoundTrip(t *testing.T) {
	set := map[int]bool{0: true, 5: true, 131071: true}
	enc := vc2.EncodeBitstring(set, 0)
	for i := 0; i < 100; i++ {
		got, err := vc2.DecodeBitstring(enc, i)
		if err != nil {
			t.Fatal(err)
		}
		want := set[i]
		if got != want {
			t.Fatalf("bit %d: got %v want %v", i, got, want)
		}
	}
	got, err := vc2.DecodeBitstring(enc, 131071)
	if err != nil || !got {
		t.Fatalf("max bit: %v %v", got, err)
	}
}
