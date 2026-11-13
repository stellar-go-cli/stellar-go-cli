package custodian

// specula-lite regression tests — each test encodes a TLC counterexample that
// mapped to an implementation defect, now asserting the FIXED behavior. The
// original defect-asserting versions are in git history; see
// .specula-lite/report.md for trace dispositions and
// docs/audit/specula-lite-custodian-findings.md for the finding → fix map.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
)

// S1a (F1) — a request refused at the decision-claim step must leave NO
// residue: signing now happens before the commit boundary but the issued
// mark is applied only inside it.
func TestRepro_RefusedClaimLeavesNoResidue(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)

	if !d.decisions.claim("dec-burned") {
		t.Fatal("setup claim failed")
	}
	if _, rerr := signRef(t, d, kp, "pay_send", "dec-burned"); rerr == nil {
		t.Fatal("expected claim refusal")
	}
	if len(d.issued) != 0 {
		t.Fatalf("refused request left %d issued entr(ies)", len(d.issued))
	}
}

// S1b (F2) — a sign refused at the charge step must NOT consume the
// decision ref: the ref is released on charge failure, so the operator can
// retry the same decision.
func TestRepro_RefSurvivesChargeRefusal(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)

	if err := d.counters.charge(d.policy, "native", "10000"); err != nil {
		t.Fatal(err)
	}
	if _, rerr := signRef(t, d, kp, "pay_send", "dec-charge-fail"); rerr == nil {
		t.Fatal("expected charge refusal")
	}
	if !d.decisions.claim("dec-charge-fail") {
		t.Fatal("decision ref burned by a request that never delivered a signature")
	}
	if len(d.issued) != 0 {
		t.Fatalf("refused request left %d issued entr(ies)", len(d.issued))
	}
}

// S1c (F3) — charge atomicity: a multi-op request refused mid-charge must
// not leave partial spend committed. Here the mandate path fails the global
// counter; mandate and global spend must both be unchanged and the ref free.
func TestRepro_ChargeIsAtomic(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	if err := d.counters.charge(d.policy, "native", "10000"); err != nil {
		t.Fatal(err)
	}
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)
	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-partial",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false)
	if rerr == nil {
		t.Fatal("expected charge refusal")
	}
	if len(d.issued) != 0 {
		t.Fatalf("refused request left %d issued entr(ies)", len(d.issued))
	}
	if !d.decisions.claim("dec-partial") {
		t.Fatal("decision ref burned by a charge-step refusal")
	}
	if got := d.counters.Spend["native"]; got != "10000" {
		t.Fatalf("global spend changed by refused request: %s", got)
	}
}

// S1c-unit — spendCounters.chargeAll commits all ops or none.
func TestRepro_ChargeAllAtomicUnit(t *testing.T) {
	d, _, _ := fixture(t, false)
	// 2 ops of 6000 native each: under per-op sight but 12000 > the 10000/day
	// test cap — chargeAll must refuse BOTH, leaving zero committed spend.
	err := d.counters.chargeAll(d.policy, []string{"native", "native"}, []string{"6000", "6000"})
	if err == nil {
		t.Fatal("expected cumulative-cap refusal")
	}
	if got := d.counters.Spend["native"]; got != "" && got != "0" {
		t.Fatalf("partial spend committed: %s", got)
	}
	// Under-cap charge commits atomically.
	if err := d.counters.chargeAll(d.policy, []string{"native", "native"}, []string{"30", "30"}); err != nil {
		t.Fatalf("chargeAll: %v", err)
	}
	if got := d.counters.Spend["native"]; got != "60" {
		t.Fatalf("spend = %s, want 60", got)
	}
}

// S1c-unit — mandateCounters.chargeAll + refund round-trip.
func TestRepro_MandateChargeAllAndRefund(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	sub := &mandateSubject{Agent: agentDID, Principal: kp.Address(), Cumulative: map[string]string{"native": "100"}}

	assets := []string{"native", "native"}
	amounts := []string{"40", "40"}
	if err := d.mandateCounters.chargeAll(sub, rec.VC.ID, assets, amounts); err != nil {
		t.Fatalf("chargeAll: %v", err)
	}
	if got := d.mandateCounters.usage[rec.VC.ID][today()]["native"]; got != "80" {
		t.Fatalf("mandate usage = %s, want 80", got)
	}
	// Over-cap charge commits nothing.
	if err := d.mandateCounters.chargeAll(sub, rec.VC.ID, []string{"native"}, []string{"50"}); err == nil {
		t.Fatal("expected cap refusal")
	}
	if got := d.mandateCounters.usage[rec.VC.ID][today()]["native"]; got != "80" {
		t.Fatalf("partial mandate spend committed: %s", got)
	}
	// Refund restores the counter (chargeSpend's rollback path).
	d.mandateCounters.refund(sub, rec.VC.ID, assets, amounts)
	if got := d.mandateCounters.usage[rec.VC.ID][today()]["native"]; got != "0" {
		t.Fatalf("usage after refund = %s, want 0", got)
	}
}

// S4a (F4) — the configured escalation limiter is built once and reused:
// hits now accumulate across calls, so count=1 refuses the second attempt.
func TestRepro_ConfiguredLimiterEngages(t *testing.T) {
	d, _, _ := fixture(t, false)
	d.policy.Escalation.Rate.Count = 1
	d.policy.Escalation.Rate.WindowSec = 600

	if !d.limiterFor().allow("acct") {
		t.Fatal("first escalation refused")
	}
	if d.limiterFor().allow("acct") {
		t.Fatal("second escalation allowed — configured limiter not persisting hits")
	}
	l1, l2 := d.limiterFor(), d.limiterFor()
	if l1 != l2 {
		t.Fatal("limiterFor returned a fresh limiter — hits cannot accumulate")
	}
}

// S5 (F5) — agent_set_status activation now requires a bound ceremony:
// Status "active"/"reactivate" without a ceremony account refuses outright.
// Privilege reductions stay ceremony-free.
func TestRepro_AgentActivationRequiresCeremony(t *testing.T) {
	d, _, _ := fixture(t, false)
	did, _ := newAgent(t)
	if _, err := d.agents.enroll(did, "", "", true); err != nil {
		t.Fatal(err)
	}

	call := func(status, account string) *signsock.Response {
		p, _ := json.Marshal(signsock.AgentSetStatusParams{DID: did, Status: status, Account: account})
		return d.dispatch(&signsock.Request{Method: signsock.MethodAgentSetStatus, Params: p, Token: d.token})
	}

	for _, st := range []string{"active", "reactivate"} {
		resp := call(st, "")
		if resp.Error == nil {
			t.Fatalf("%s without ceremony account succeeded", st)
		}
		if resp.Error.Code != signsock.ErrBadRequest {
			t.Fatalf("%s: error = %v, want ErrBadRequest", st, resp.Error)
		}
	}
	// suspend works bare.
	if resp := call("suspended", ""); resp.Error != nil {
		t.Fatalf("suspend refused: %v", resp.Error)
	}
	if st := d.agents.stateOf(did); st != agentSuspended {
		t.Fatalf("state = %s, want suspended", st)
	}
}

// F9 — authority re-check: the helper re-validates grants/mandate/agent
// state at sign time. A mid-request revocation must surface a refusal reason.
func TestRepro_AuthorityRecheck(t *testing.T) {
	d, kp, _ := fixture(t, true) // protected wallet — grant is the authority
	acc, err := d.svc.GetWalletByAddress(kp.Address())
	if err != nil {
		t.Fatal(err)
	}

	if reason := d.authorityRevoked("", "", acc); reason == "" {
		t.Fatal("expected revocation reason for protected wallet with no grant")
	}
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)
	if reason := d.authorityRevoked("", "", acc); reason != "" {
		t.Fatalf("unexpected reason with live grant: %s", reason)
	}
	d.grants.drop(kp.Address())
	if reason := d.authorityRevoked("", "", acc); reason == "" {
		t.Fatal("grant drop mid-request not detected")
	}

	// Mandated path: mandate record + agent + escrow all re-validated.
	agentDID, _ := newAgent(t)
	if _, err := d.agents.enroll(agentDID, "", "", true); err != nil {
		t.Fatal(err)
	}
	if err := d.setAgentStatus(agentDID, agentActive); err != nil {
		t.Fatal(err)
	}
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)
	if reason := d.authorityRevoked(rec.VC.ID, agentDID, acc); reason != "" {
		t.Fatalf("unexpected reason for live mandate: %s", reason)
	}
	if err := d.setAgentStatus(agentDID, agentSuspended); err != nil {
		t.Fatal(err)
	}
	if reason := d.authorityRevoked(rec.VC.ID, agentDID, acc); !strings.Contains(reason, "agent") {
		t.Fatalf("agent suspension not detected, got %q", reason)
	}
}

// S2b (F6) — restart tombstoning: a submitted: record carrying payload_hash
// deletes the issued entry; a re-sign after submission re-adds it.
func TestRepro_SubmittedTombstonedOnRestart(t *testing.T) {
	d, _, _ := fixture(t, false)

	write := func(recs []enforcementRecord) string {
		path := filepath.Join(t.TempDir(), "enforcement.log")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recs {
			b, _ := json.Marshal(r)
			f.Write(append(b, '\n')) //nolint:errcheck
		}
		f.Close() //nolint:errcheck
		return path
	}

	// signed then submitted → not resurrected.
	d.rebuildIssued(write([]enforcementRecord{
		{Timestamp: nowRFC3339(), Tool: "sign_tx", Result: "signed:PAYLOADHASH"},
		{Timestamp: nowRFC3339(), Tool: "submit_tx", Result: "submitted:TXHASH", PayloadHash: "PAYLOADHASH"},
	}))
	if d.issued["PAYLOADHASH"] {
		t.Fatal("submitted payload resurrected into issued")
	}

	// Re-sign after submit → legitimately submittable again.
	d.rebuildIssued(write([]enforcementRecord{
		{Timestamp: nowRFC3339(), Tool: "sign_tx", Result: "signed:P2"},
		{Timestamp: nowRFC3339(), Tool: "submit_tx", Result: "submitted:TX2", PayloadHash: "P2"},
		{Timestamp: nowRFC3339(), Tool: "sign_tx", Result: "signed:P2"},
	}))
	if !d.issued["P2"] {
		t.Fatal("re-signed payload not restored")
	}
}

// S2a (F7) — issuedTake is atomic: one claim per issued payload, even under
// concurrent submit_tx calls.
func TestRepro_IssuedTakeIsAtomic(t *testing.T) {
	d, _, _ := fixture(t, false)
	d.issuedMark("h1")
	if !d.issuedTake("h1") {
		t.Fatal("first take failed")
	}
	if d.issuedTake("h1") {
		t.Fatal("second take passed — double submit possible")
	}

	// Concurrent hammer: exactly one winner per payload.
	for round := 0; round < 20; round++ {
		h := "hh"
		d.issuedMark(h)
		wins := make(chan bool, 8)
		for i := 0; i < 8; i++ {
			go func() { wins <- d.issuedTake(h) }()
		}
		n := 0
		for i := 0; i < 8; i++ {
			if <-wins {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("round %d: %d winners, want exactly 1", round, n)
		}
	}
}

// S3 (F11 — accepted bound, documented) — seenSet still ages entries past
// 24h once the map exceeds 10000. This test documents the bound so a future
// change to persistence can revisit it deliberately.
func TestRepro_SeenSetAgingBound(t *testing.T) {
	s := newSeenSet()
	s.seen["old-ref"] = time.Now().Add(-25 * time.Hour)
	for i := 0; i < 10001; i++ {
		s.claim("filler-" + string(rune(i)))
	}
	s.claim("trigger")
	if !s.claim("old-ref") {
		t.Log("aging eviction changed — update the documented bound in the findings doc")
	}
}

// C1 (F10) — passkey_confirm now requires the request hash to be a tracked
// pending escalation. Blind pre-approval of arbitrary hashes refuses.
func TestRepro_ConfirmRequiresPendingEscalation(t *testing.T) {
	d, kp, _ := fixture(t, false)

	rh := strings.Repeat("ab", 32) // valid 32-byte hex, never escalated
	p, _ := json.Marshal(signsock.ConfirmParams{Account: kp.Address(), RequestHash: rh})
	resp := d.dispatch(&signsock.Request{Method: signsock.MethodConfirm, Params: p, Token: d.token})
	if resp.Error == nil {
		t.Fatal("confirm of un-escalated hash succeeded")
	}
	if resp.Error.Code != signsock.ErrRefused {
		t.Fatalf("error = %v, want ErrRefused", resp.Error)
	}

	// A tracked escalation passes the gate and proceeds to the ceremony
	// (which fails downstream without a passkey — that's fine; the refusal
	// must NOT be the pending-escalation one).
	d.trackPending(&signsock.PendingRequest{RequestHash: rh, Timestamp: nowRFC3339()})
	resp = d.dispatch(&signsock.Request{Method: signsock.MethodConfirm, Params: p, Token: d.token})
	if resp.Error != nil && strings.Contains(resp.Error.Message, "no tracked escalation") {
		t.Fatal("tracked escalation still refused at the pending gate")
	}
}

// T2/C8 (F8) — pendingReq and issued are mutex-guarded now; this hammer is a
// permanent race-detector target (run the package with -race).
func TestRepro_RaceHammer(t *testing.T) {
	d, _, _ := fixture(t, false)
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func(id int) {
			for j := 0; j < 500; j++ {
				h := "h" + string(rune('a'+id))
				d.trackPending(&signsock.PendingRequest{RequestHash: h, Timestamp: nowRFC3339()})
				d.pendingRequest(h)
				d.issuedMark(h)
				d.issuedTake(h)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}
}

// Hygiene — dropWalletGrants revokes the wallet grant AND every mandate
// escrow for the wallet, so disable/remove leaves no seed material behind.
func TestRepro_DropWalletGrantsSweepsEscrows(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))

	d.grants.put(kp.Address(), kp.Seed(), time.Minute)
	d.grants.put("mandate:"+rec.VC.ID, kp.Seed(), time.Hour)

	d.dropWalletGrants(kp.Address())

	if seed := d.grants.seed(kp.Address()); seed != "" {
		t.Fatal("wallet grant survived dropWalletGrants")
	}
	if seed := d.grants.seed("mandate:" + rec.VC.ID); seed != "" {
		t.Fatal("mandate escrow survived dropWalletGrants — disabled wallet leaves usable seed")
	}
}

// sanity — the same helpers still drive a clean sign end-to-end.
func TestRepro_FixtureSane(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute)
	if _, rerr := signRef(t, d, kp, "pay_send", "dec-sane"); rerr != nil {
		t.Fatalf("baseline sign refused: %v", rerr)
	}
	// Replays are refused — E5 works within one daemon lifetime.
	if _, rerr := signRef(t, d, kp, "pay_send", "dec-sane"); rerr == nil {
		t.Fatal("replay signed")
	}
}
