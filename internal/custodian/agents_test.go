package custodian

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
)

// §11.1 lifecycle FSM.

func TestAgentEnrollAndTransitions(t *testing.T) {
	d, _, _ := fixture(t, false)

	rec, err := d.agents.enroll("did:key:z6MkAgent1", "transact", "test", false)
	if err != nil || rec.State != agentEnrolled {
		t.Fatalf("enroll: %v state=%s", err, rec.State)
	}
	// enrolled → active
	if err := d.setAgentStatus(rec.DID, agentActive); err != nil {
		t.Fatalf("enrolled→active: %v", err)
	}
	// active → suspended
	if err := d.setAgentStatus(rec.DID, agentSuspended); err != nil {
		t.Fatalf("active→suspended: %v", err)
	}
	// suspended → active
	if err := d.setAgentStatus(rec.DID, agentActive); err != nil {
		t.Fatalf("suspended→active: %v", err)
	}
	// active → enrolled is not a legal move
	if err := d.setAgentStatus(rec.DID, agentEnrolled); err == nil {
		t.Fatal("active→enrolled accepted")
	}
	// active → retired (terminal)
	if err := d.setAgentStatus(rec.DID, agentRetired); err != nil {
		t.Fatalf("active→retired: %v", err)
	}
	if err := d.setAgentStatus(rec.DID, agentActive); err == nil {
		t.Fatal("retired→active accepted — retirement must be terminal")
	}
	if _, err := d.agents.enroll(rec.DID, "", "", false); err == nil {
		t.Fatal("re-enrolling a retired agent accepted")
	}
	// Unknown DID refuses.
	if err := d.setAgentStatus("did:key:z6MkNobody", agentSuspended); err == nil {
		t.Fatal("status on unknown agent accepted")
	}
}

func TestAgentRegistryPersists(t *testing.T) {
	d, _, _ := fixture(t, false)
	if _, err := d.agents.enroll("did:key:z6MkPersist", "admin", "", false); err != nil {
		t.Fatal(err)
	}
	r2, err := loadAgentRegistry(d.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if st := r2.stateOf("did:key:z6MkPersist"); st != agentEnrolled {
		t.Fatalf("reloaded state = %q", st)
	}
}

// §11.1 enforcement: suspended/retired agents cannot exercise mandate
// authority; retirement revokes dependent mandates.

func TestSuspendedAgentRefused(t *testing.T) {
	d, kp, _ := fixture(t, false)
	d.policy.AllowUnprotected = true
	agentDID, signerFn := newAgent(t)
	rec, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))

	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	pop := signPOP(t, signerFn, "pay_send", kp.Address(), xdr)

	// First sign auto-enrols the agent as active and succeeds.
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-a1",
		signsock.RequestHash("pay_send", kp.Address(), desc), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false); rerr != nil {
		t.Fatalf("active agent refused: %v", rerr)
	}
	if st := d.agents.stateOf(agentDID); st != agentActive {
		t.Fatalf("auto-enrol state = %q", st)
	}

	// Suspend → next sign refused even though the mandate is still active.
	if err := d.setAgentStatus(agentDID, agentSuspended); err != nil {
		t.Fatal(err)
	}
	desc2, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-a2",
		signsock.RequestHash("pay_send", kp.Address(), desc2), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false); rerr == nil || !strings.Contains(rerr.Message, "suspended") {
		t.Fatalf("suspended agent not refused: %v", rerr)
	}

	// Reactivate → signs again.
	if err := d.setAgentStatus(agentDID, agentActive); err != nil {
		t.Fatal(err)
	}
	if _, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-a3",
		signsock.RequestHash("pay_send", kp.Address(), desc2), "pay_send", kp.Address(),
		network.TestNetworkPassphrase, xdr, 0, rec.VC.ID, pop, false); rerr != nil {
		t.Fatalf("reactivated agent refused: %v", rerr)
	}
}

func TestRetireAgentRevokesMandates(t *testing.T) {
	d, kp, _ := fixture(t, false)
	agentDID, _ := newAgent(t)
	rec1, _ := mintMandate(t, d, kp, agentDID, []string{"pay_*"}, time.Now().Add(time.Hour))
	rec2, _ := mintMandate(t, d, kp, agentDID, []string{"swap_*"}, time.Now().Add(time.Hour))
	otherDID, _ := newAgent(t)
	rec3, _ := mintMandate(t, d, kp, otherDID, []string{"pay_*"}, time.Now().Add(time.Hour))

	if _, err := d.agents.enroll(agentDID, "", "", true); err != nil {
		t.Fatal(err)
	}
	if err := d.setAgentStatus(agentDID, agentRetired); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{rec1.VC.ID, rec2.VC.ID} {
		r, err := d.mandates.load(id)
		if err != nil || r.Status != mandateRevoked {
			t.Fatalf("mandate %s status = %v (want revoked)", id, r.Status)
		}
	}
	// The other agent's mandate is untouched.
	r3, _ := d.mandates.load(rec3.VC.ID)
	if r3.Status != mandateActive {
		t.Fatalf("unrelated mandate status = %s", r3.Status)
	}
}

// §10.3 — escalation rate limiting.

func TestEscalationRateLimited(t *testing.T) {
	d, kp, _ := fixture(t, true)
	d.escalations = newEscalationLimiter(2, time.Minute)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute) // unlock so we reach E3
	// Trigger escalations: amount above the T3 threshold on a protected wallet.
	d.policy.Escalation.T3Above = map[string]string{"native": "1"}
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")

	for i := 0; i < 2; i++ {
		_, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-esc", "", "pay_send",
			kp.Address(), network.TestNetworkPassphrase, xdr, 0)
		if rerr == nil || !strings.Contains(rerr.Message, "operator approval required") {
			t.Fatalf("escalation %d: got %v", i, rerr)
		}
	}
	_, rerr := d.enforceAndSign(signsock.MethodSignTx, "dec-esc", "", "pay_send",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0)
	if rerr == nil || !strings.Contains(rerr.Message, "rate limited") {
		t.Fatalf("third escalation not rate-limited: %v", rerr)
	}
}

// §10.3 — pending request tracking + untrusted flag.

func TestPendingRequestTracksEscalation(t *testing.T) {
	d, kp, logBuf := fixture(t, true)
	d.grants.put(kp.Address(), kp.Seed(), time.Minute) // unlock so we reach E3
	d.policy.Escalation.T3Above = map[string]string{"native": "1"}
	xdr := unsignedTx(t, kp, keypair.MustRandom().Address(), "5")
	desc, _ := signsock.DescribeEnvelopeXDR(xdr, network.TestNetworkPassphrase)
	rh := signsock.RequestHash("pay_x402", kp.Address(), desc)

	_, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, "dec-u1", rh, "pay_x402",
		kp.Address(), network.TestNetworkPassphrase, xdr, 0, "", "", true)
	if rerr == nil || !strings.Contains(rerr.Message, "operator approval required") {
		t.Fatalf("expected escalation, got %v", rerr)
	}
	pr, ok := d.pendingRequest(rh)
	if !ok {
		t.Fatal("escalation not tracked")
	}
	if !pr.Untrusted {
		t.Fatal("untrusted flag lost")
	}
	if pr.Tool != "pay_x402" || pr.Description == "" {
		t.Fatalf("bad pending record: %+v", pr)
	}

	// The refusal record carries the flag too.
	var last map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(logBuf.String()), "\n") {
		var rec map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["request_hash"] == rh {
			last = rec
		}
	}
	if last["untrusted"] != true {
		t.Fatalf("record missing untrusted flag: %v", last)
	}
}
