package custodian

import (
	"strings"
	"testing"
)

func approvedBase() *Policy {
	p := testPolicy()
	p.HardLimits.PerTx = map[string]string{"native": "100"}
	p.HardLimits.PerDay = map[string]string{"native": "500"}
	p.HardLimits.Networks = []string{"testnet"}
	p.HardLimits.Deny = []string{"GDENY"}
	p.Escalation.T3Above = map[string]string{"native": "10"}
	p.AllowUnprotected = false
	return p
}

func TestPolicyWideningDetects(t *testing.T) {
	old := approvedBase()

	cur := approvedBase()
	cur.HardLimits.PerTx["native"] = "200"                               // raise cap → widen
	cur.HardLimits.Networks = append(cur.HardLimits.Networks, "mainnet") // add network → widen
	cur.HardLimits.Deny = nil                                            // remove deny → widen
	cur.AllowUnprotected = true                                          // enable unprotected → widen
	delete(cur.Escalation.T3Above, "native")                             // remove threshold → widen

	diffs := PolicyWidening(old, cur)
	if len(diffs) < 5 {
		t.Fatalf("expected ≥5 widening diffs, got %v", diffs)
	}
	joined := strings.Join(diffs, ";")
	for _, want := range []string{"per_tx", "network", "denylisted", "allow_unprotected", "t3_above"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing widening diff for %s in %v", want, diffs)
		}
	}
}

func TestPolicyNarrowingPasses(t *testing.T) {
	old := approvedBase()

	cur := approvedBase()
	cur.HardLimits.PerTx["native"] = "50"                      // lower cap → narrow
	cur.HardLimits.Deny = append(cur.HardLimits.Deny, "GMORE") // add deny → narrow
	cur.HardLimits.Networks = []string{"testnet"}              // same
	cur.Escalation.T3Above["native"] = "5"                     // lower threshold → narrow

	if diffs := PolicyWidening(old, cur); len(diffs) > 0 {
		t.Fatalf("narrowing reported as widening: %v", diffs)
	}
}

func TestPolicyApprovalGate(t *testing.T) {
	dir := t.TempDir()
	old := approvedBase()

	// First boot: self-approves.
	if err := checkPolicyApproval(old, dir); err != nil {
		t.Fatalf("first boot refused: %v", err)
	}
	// Same policy passes.
	if err := checkPolicyApproval(old, dir); err != nil {
		t.Fatalf("approved policy refused: %v", err)
	}
	// Narrowing auto-approves.
	narrow := approvedBase()
	narrow.HardLimits.PerTx["native"] = "10"
	if err := checkPolicyApproval(narrow, dir); err != nil {
		t.Fatalf("narrowing refused: %v", err)
	}
	// Widening refuses.
	wide := approvedBase()
	wide.HardLimits.PerTx["native"] = "9999"
	wide.AllowUnprotected = true
	err := checkPolicyApproval(wide, dir)
	if err == nil || !strings.Contains(err.Error(), "widened") {
		t.Fatalf("widening not refused: %v", err)
	}
	// Operator approval unblocks.
	if _, err := saveApprovalForTest(dir, wide); err != nil {
		t.Fatal(err)
	}
	if err := checkPolicyApproval(wide, dir); err != nil {
		t.Fatalf("approved widening still refused: %v", err)
	}
}

func saveApprovalForTest(dir string, p *Policy) (*policyApproval, error) {
	if err := saveApproval(dir, p, "test-operator", false); err != nil {
		return nil, err
	}
	return &policyApproval{Version: p.Version, Hash: p.policyHash()}, nil
}
