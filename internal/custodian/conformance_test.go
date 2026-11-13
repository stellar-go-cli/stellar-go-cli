package custodian

// Conformance vectors C3–C6, C9–C12, C14 (spec §15, Class T) — the
// custodian-side checks. Gatekeeper-side vectors (C1, C2, C7, C8, C13) live
// in internal/mcp/conformance_test.go; C1's static dependency check there
// asserts internal/mcp carries no code path to key material.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
)

// signOnce runs one grant-authorized sign through the real decision point.
func signOnce(t *testing.T, d *Daemon, kp *keypair.Full, tool string) (string, *signsock.RPCError) {
	t.Helper()
	return signRef(t, d, kp, tool, "dec-"+tool+"-"+randomHex(4))
}

func signRef(t *testing.T, d *Daemon, kp *keypair.Full, tool, ref string) (string, *signsock.RPCError) {
	t.Helper()
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, err := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	return d.enforceAndSign(signsock.MethodSignTx, ref,
		signsock.RequestHash(tool, kp.Address(), desc), tool, kp.Address(),
		network.TestNetworkPassphrase, xdr, 0)
}

// C3 — every request produces an enforcement record; the chain verifies.
func TestC3_RecordsAndChainVerify(t *testing.T) {
	d, kp, logBuf := fixture(t, false)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)

	if _, rerr := signRef(t, d, kp, "pay_send", "dec-c3"); rerr != nil {
		t.Fatalf("sign refused: %v", rerr)
	}
	// Replay the same decision ref — E5 refuses, and refusals record too.
	if _, rerr := signRef(t, d, kp, "pay_send", "dec-c3"); rerr == nil {
		t.Fatal("expected refusal")
	}

	var lines int
	sc := bufio.NewScanner(bytes.NewReader(logBuf.Bytes()))
	for sc.Scan() {
		lines++
		var rec map[string]interface{}
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			t.Fatal("unparseable record")
		}
		for _, f := range []string{"seq", "prev", "hash", "decision_ref", "tool", "key_id", "result"} {
			if _, ok := rec[f]; !ok {
				t.Fatalf("record %d missing %s", lines, f)
			}
		}
	}
	if lines != 2 {
		t.Fatalf("expected 2 records, got %d", lines)
	}
	// VerifyFile over the buffer — write it to a temp log.
	path := writeLog(t, logBuf.Bytes())
	if n, err := chainlog.VerifyFile(path); err != nil || n != 2 {
		t.Fatalf("chain verify: n=%d err=%v", n, err)
	}
}

// C4 — T3 without authority is never signed: escalation fires instead.
func TestC4_T3WithoutAuthorityNeverSigns(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.Escalation.T3Above = map[string]string{"native": "1"}
	d.grants.put(kp.Address(), kp.Seed(), time.Minute) // grant exists but value exceeds T3 → escalate

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "50")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	signed, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-c4", signsock.RequestHash("pay_send", kp.Address(), desc),
		"pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || signed != "" {
		t.Fatalf("T3 above threshold signed without bound approval")
	}
	if rerr.Code != signsock.ErrEscalationRequired {
		t.Fatalf("expected escalation_required, got %s", rerr.Code)
	}
}

// C5 — within-mandate signs; outside-mandate refuses.
func TestC5_MandateBounds(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
	signed, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c5",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr != nil || signed == "" {
		t.Fatalf("within-mandate sign refused: %v", rerr)
	}

	// Outside scope: tool not covered by the mandate.
	rerr2 := func() *signsock.RPCError {
		pop2 := signPOP(t, signerFn, "asset_trust", kp.Address(), xdr)
		_, e := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c5b",
			signsock.RequestHash("asset_trust", kp.Address(), desc), "asset_trust", kp.Address(),
			network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop2, false)
		return e
	}()
	if rerr2 == nil {
		t.Fatal("out-of-scope mandate sign succeeded")
	}
}

// C6 — revocation takes effect within the declared latency bound.
func TestC6_RevocationLatency(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	t0 := time.Now()
	res := d.applyMandateStatus(rec.VC.ID, mandateRevoked, kp.Seed())
	if e, _ := res["error"].(string); e != "" {
		t.Fatalf("revoke failed: %s", e)
	}

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c6",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	elapsed := time.Since(t0)
	if rerr == nil {
		t.Fatal("revoked mandate still signs")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("revocation latency %s exceeds declared bound", elapsed)
	}
}

// C9 — expired grants and suspended/revoked mandates cannot produce a signature.
func TestC9_NoSignatureWithoutLiveAuthority(t *testing.T) {
	d, kp, _ := fixture(t, true)
	agentDID, signerFn := newAgent(t)
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)

	// Expired grant on a protected wallet.
	d.grants.put(kp.Address(), kp.Seed(), -time.Second)
	if _, rerr := signOnce(t, d, kp, "pay_send"); rerr == nil {
		t.Fatal("expired grant signed")
	}
	// A live grant on the same wallet signs — the refusal above was the
	// expiry, not a missing path.
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)
	if _, rerr := signOnce(t, d, kp, "pay_send"); rerr != nil {
		t.Fatalf("live grant refused: %v", rerr)
	}
	d.grants.drop(kp.Address())

	// Suspended mandate.
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	d.applyMandateStatus(rec.VC.ID, mandateSuspended, kp.Seed())
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c9",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false); rerr == nil {
		t.Fatal("suspended mandate signed")
	}

	// Expired mandate.
	rec2, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(-time.Hour))
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c9b",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec2.VC.ID, pop, false); rerr == nil {
		t.Fatal("expired mandate signed")
	}
}

// C10 — payloads whose decoded contents differ from the decided context are
// refused; undecodable payloads are refused.
func TestC10_PayloadTruth(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	wrongHash := signsock.RequestHash("pay_send", keypair.MustRandom().Address(), desc)
	if _, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-c10", wrongHash,
		"pay_send", kp.Address(), network.TestNetworkPassphrase, xdr, 0); rerr == nil {
		t.Fatal("mismatched request hash signed")
	}
	if _, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-c10b", "deadbeef",
		"pay_send", kp.Address(), network.TestNetworkPassphrase, "!!!not-xdr!!!", 0); rerr == nil {
		t.Fatal("undecodable payload signed")
	}
}

// C11 — every enforcement record carries attribution: decision_ref, tool,
// key_id, result — and mandate-derived signs carry mandate_ref.
func TestC11_Attribution(t *testing.T) {
	d, kp, logBuf := fixture(t, false)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-c11",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false); rerr != nil {
		t.Fatalf("mandate sign refused: %v", rerr)
	}

	var last map[string]interface{}
	sc := bufio.NewScanner(bytes.NewReader(logBuf.Bytes()))
	for sc.Scan() {
		var m map[string]interface{}
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			last = m
		}
	}
	if last == nil {
		t.Fatal("no record written")
	}
	if last["mandate_ref"] != rec.VC.ID {
		t.Fatalf("record missing mandate attribution: %v", last)
	}
	if last["decision_ref"] != "dec-c11" || last["key_id"] != kp.Address() {
		t.Fatalf("record attribution wrong: %v", last)
	}
}

// C12 — lock/revoke/suspend remain usable while everything else is denied.
func TestC12_PrivilegeReductionAlwaysWorks(t *testing.T) {
	d, kp, _ := fixture(t, true)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	// Wallet is locked (no grant) — signing is denied, but lifecycle ops work.
	if _, rerr := signOnce(t, d, kp, "pay_send"); rerr == nil {
		t.Fatal("locked wallet signed")
	}
	d.grants.drop(kp.Address()) // the lock operation drops the grant
	res := d.applyMandateStatus(rec.VC.ID, mandateSuspended, kp.Seed())
	if e, _ := res["error"].(string); e != "" {
		t.Fatalf("suspend denied on locked wallet: %s", e)
	}
	res = d.applyMandateStatus(rec.VC.ID, mandateRevoked, kp.Seed())
	if e, _ := res["error"].(string); e != "" {
		t.Fatalf("revoke denied on locked wallet: %s", e)
	}
}

// C14 — the custodian artefact is integrity-anchored in the enforcement
// chain: recordSelfIntegrity writes a self:<sha256> record.
func TestC14_SelfIntegrityRecord(t *testing.T) {
	d, _, logBuf := fixture(t, false)
	d.recordSelfIntegrity()
	var last map[string]interface{}
	sc := bufio.NewScanner(bytes.NewReader(logBuf.Bytes()))
	for sc.Scan() {
		var m map[string]interface{}
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			last = m
		}
	}
	if last == nil || last["tool"] != "self_integrity" {
		t.Fatalf("no self-integrity record: %v", last)
	}
	r, _ := last["result"].(string)
	if !strings.HasPrefix(r, "self:") || len(r) != 5+64 {
		t.Fatalf("malformed self record: %q", r)
	}
}

// writeLog persists a log buffer to a temp file for chainlog.VerifyFile.
func writeLog(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "enforcement.log")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
