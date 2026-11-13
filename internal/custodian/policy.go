package custodian

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Policy is the operator-owned custodian policy (spec §5.4). The custodian
// holds the authoritative copy; nothing the gatekeeper says can widen it.
type Policy struct {
	Version    string `json:"version"`
	HardLimits struct {
		PerTx    map[string]string `json:"per_tx"`   // canonical asset -> max amount per op
		PerDay   map[string]string `json:"per_day"`  // canonical asset -> rolling UTC-day cap
		Networks []string          `json:"networks"` // allowed network passphrases (empty = any)
		Deny     []string          `json:"deny"`     // denied G.../C... counterparties
	} `json:"hard_limits"`
	Escalation struct {
		T3Above         map[string]string `json:"t3_above"` // canonical asset -> threshold needing bound approval
		NewCounterparty bool              `json:"new_counterparty"`
		// Rate bounds escalation-request generation per account (§10.3 —
		// escalation fatigue is an attack). Zero = default 5 per 10 minutes.
		Rate struct {
			Count     int   `json:"count"`
			WindowSec int64 `json:"window_sec"`
		} `json:"rate"`
	} `json:"escalation"`
	AllowUnprotected     bool `json:"allow_unprotected"`      // sign for plaintext wallets
	AllowUnclassifiedOps bool `json:"allow_unclassified_ops"` // ops with unextractable value semantics
	Audit                struct {
		Path string `json:"path"` // enforcement log path (default state/enforcement.log)
	} `json:"audit"`
}

// policyHash is the short digest stamped on enforcement records.
func (p *Policy) policyHash() string {
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// defaultPolicy is conservative: testnet only, small caps, protected wallets
// required for signing above zero-config defaults.
func defaultPolicy() *Policy {
	p := &Policy{Version: "pme-2026-11", AllowUnprotected: true, AllowUnclassifiedOps: false}
	p.HardLimits.PerTx = map[string]string{"native": "500"}
	p.HardLimits.PerDay = map[string]string{"native": "2000"}
	p.HardLimits.Networks = []string{
		"Test SDF Network ; September 2015", // stellar-testnet
		"Test SDF Future Network ; October 2022",
	}
	p.Escalation.T3Above = map[string]string{"native": "50"}
	p.Escalation.NewCounterparty = false
	return p
}

// loadPolicy reads the operator policy file. A missing file is seeded with
// the restrictive default; a malformed file fails closed — the daemon
// refuses to run without legible policy.
func loadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-selected policy path
	if err != nil {
		if os.IsNotExist(err) {
			p := defaultPolicy()
			out, merr := json.MarshalIndent(p, "", "  ")
			if merr != nil {
				return nil, fmt.Errorf("encode default policy: %w", merr)
			}
			if werr := os.WriteFile(path, out, 0600); werr != nil {
				return nil, fmt.Errorf("seed default policy: %w", werr)
			}
			return p, nil
		}
		return nil, err
	}
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("policy file is malformed (refusing to start): %w", err)
	}
	return &p, nil
}

// checkNetworks enforces the network allowlist (E1).
func (p *Policy) checkNetworks(passphrase string) error {
	if len(p.HardLimits.Networks) == 0 {
		return nil
	}
	for _, n := range p.HardLimits.Networks {
		if n == passphrase {
			return nil
		}
	}
	return fmt.Errorf("network %q is not in the policy allowlist", passphrase)
}

// denylisted reports whether a counterparty is denied (E1).
func (p *Policy) denylisted(addr string) bool {
	for _, d := range p.HardLimits.Deny {
		if d == addr {
			return true
		}
	}
	return false
}

// perTxCap returns the per-transaction cap for an asset, or an error naming
// the first violation. Assets absent from per_tx are uncapped at this level —
// escalation/day caps still apply.
func (p *Policy) checkPerTx(asset, amount string) error {
	capStr, ok := p.HardLimits.PerTx[asset]
	if !ok {
		return nil
	}
	over, err := exceeds(amount, capStr)
	if err != nil {
		return err
	}
	if over {
		return fmt.Errorf("amount %s %s exceeds per-transaction cap %s", amount, asset, capStr)
	}
	return nil
}

// escalationThreshold returns the asset's escalation threshold ("" if none).
func (p *Policy) escalationThreshold(asset string) string {
	return p.Escalation.T3Above[asset]
}

// exceeds compares decimal strings: amount > limit.
func exceeds(amount, limit string) (bool, error) {
	a, ok := new(big.Rat).SetString(amount)
	if !ok {
		return false, fmt.Errorf("unparseable amount %q", amount)
	}
	l, ok := new(big.Rat).SetString(limit)
	if !ok {
		return false, fmt.Errorf("unparseable limit %q", limit)
	}
	return a.Cmp(l) > 0, nil
}

// addAmount returns amount+delta as a decimal string.
func addAmount(amount, delta string) string {
	a, _ := new(big.Rat).SetString(amount)
	d, _ := new(big.Rat).SetString(delta)
	if a == nil {
		a = new(big.Rat)
	}
	if d == nil {
		return amount
	}
	sum := new(big.Rat).Add(a, d)
	if sum.IsInt() {
		return sum.RatString()
	}
	// Strip trailing zeros from a 7-decimal rendering (Stellar's max
	// precision) — RatString would emit a fraction, which is exact but
	// harder to read in counters/logs.
	s := strings.TrimRight(sum.FloatString(7), "0")
	return strings.TrimSuffix(s, ".")
}

func subAmount(amount, delta string) string {
	a, _ := new(big.Rat).SetString(amount)
	d, _ := new(big.Rat).SetString(delta)
	if a == nil || d == nil {
		return amount
	}
	diff := new(big.Rat).Sub(a, d)
	if diff.IsInt() {
		return diff.RatString()
	}
	s := strings.TrimRight(diff.FloatString(7), "0")
	return strings.TrimSuffix(s, ".")
}

// spendCounters tracks cumulative daily spend per asset (E1 rolling cap).
// Persisted so daemon restarts don't reset the window.
type spendCounters struct {
	mu    sync.Mutex
	path  string
	Day   string            `json:"day"`
	Spend map[string]string `json:"spend"`
}

func loadCounters(path string) *spendCounters {
	c := &spendCounters{path: path, Spend: map[string]string{}}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed filename inside the operator state dir
	if err == nil && json.Unmarshal(data, c) == nil && c.Day == today() {
		return c
	}
	c.Day = today()
	c.Spend = map[string]string{}
	return c
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

// charge adds spend and checks the daily cap; on cap breach it rolls back
// and returns an error.
func (c *spendCounters) charge(p *Policy, asset, amount string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Day != today() {
		c.Day = today()
		c.Spend = map[string]string{}
	}
	capStr, ok := p.HardLimits.PerDay[asset]
	if ok {
		over, err := exceeds(addAmount(c.Spend[asset], amount), capStr)
		if err != nil {
			return err
		}
		if over {
			return fmt.Errorf("daily spend cap %s %s would be exceeded", capStr, asset)
		}
	}
	c.Spend[asset] = addAmount(c.Spend[asset], amount)
	return c.save()
}

// chargeAll atomically validates and applies a request's full spend — all
// ops commit under one lock or none do (specula-lite F3).
func (c *spendCounters) chargeAll(p *Policy, assets, amounts []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Day != today() {
		c.Day = today()
		c.Spend = map[string]string{}
	}
	totals := map[string]string{}
	for i, asset := range assets {
		totals[asset] = addAmount(totals[asset], amounts[i])
	}
	for asset, amt := range totals {
		capStr, ok := p.HardLimits.PerDay[asset]
		if !ok {
			continue
		}
		over, err := exceeds(addAmount(c.Spend[asset], amt), capStr)
		if err != nil {
			return err
		}
		if over {
			return fmt.Errorf("daily spend cap %s %s would be exceeded", capStr, asset)
		}
	}
	for asset, amt := range totals {
		c.Spend[asset] = addAmount(c.Spend[asset], amt)
	}
	return c.save()
}

func (c *spendCounters) save() error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}

// counterparties is the persisted set of previously-signed destinations —
// the basis for the new_counterparty escalation rule.
type counterparties struct {
	mu   sync.Mutex
	path string
	set  map[string]bool
}

func loadCounterparties(path string) *counterparties {
	c := &counterparties{path: path, set: map[string]bool{}}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed filename inside the operator state dir
	if err == nil {
		var list []string
		if json.Unmarshal(data, &list) == nil {
			for _, a := range list {
				c.set[a] = true
			}
		}
	}
	return c
}

// mark records a counterparty after a successful sign.
func (c *counterparties) mark(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if addr == "" || c.set[addr] {
		return
	}
	c.set[addr] = true
	list := make([]string, 0, len(c.set))
	for a := range c.set {
		list = append(list, a)
	}
	if data, err := json.Marshal(list); err == nil {
		// #nosec G104 -- best-effort persistence; counters are advisory
		os.WriteFile(c.path, data, 0600) //nolint:errcheck
	}
}

func (c *counterparties) seen(addr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set[addr]
}

// counterpartyPath locates the counterparty store under the state dir.
func counterpartyPath(stateDir string) string { return filepath.Join(stateDir, "counterparties.json") }

// ── policy approval gate (spec §11.3) ────────────────────────────────────
//
// Policy is a versioned artefact. Widening changes SHALL require recorded
// operator approval; narrowing changes apply immediately. The gate compares
// policy.json against the last approved snapshot (policy.approved.json);
// approvals are logged in policy-approvals.json.

// policyApproval records an operator's review of a policy version.
type policyApproval struct {
	Version    string `json:"version"`
	Hash       string `json:"hash"`
	ApprovedAt string `json:"approved_at"`
	Approver   string `json:"approver"`
	Automatic  bool   `json:"automatic,omitempty"` // narrowing/self-approval, not an operator act
}

func approvalsPath(stateDir string) string { return filepath.Join(stateDir, "policy-approvals.json") }
func approvedPolicyPath(stateDir string) string {
	return filepath.Join(stateDir, "policy.approved.json")
}

func loadApprovals(stateDir string) ([]policyApproval, error) {
	data, err := os.ReadFile(approvalsPath(stateDir)) // #nosec G304 -- fixed name inside state dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []policyApproval
	return list, json.Unmarshal(data, &list)
}

func saveApproval(stateDir string, p *Policy, approver string, automatic bool) error {
	list, lerr := loadApprovals(stateDir)
	if lerr != nil {
		return lerr
	}
	list = append(list, policyApproval{
		Version:    p.Version,
		Hash:       p.policyHash(),
		ApprovedAt: time.Now().UTC().Format(time.RFC3339),
		Approver:   approver,
		Automatic:  automatic,
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(approvalsPath(stateDir), data, 0600); err != nil {
		return err
	}
	// Snapshot the approved policy for the next widening diff.
	snap, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(approvedPolicyPath(stateDir), snap, 0600)
}

// ApprovePolicy records operator approval for the policy file at path —
// `stellar-go-cli signer policy approve`.
func ApprovePolicy(policyPath, stateDir, approver string) (*policyApproval, error) {
	p, err := loadPolicy(policyPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, err
	}
	if err := saveApproval(stateDir, p, approver, false); err != nil {
		return nil, err
	}
	return &policyApproval{Version: p.Version, Hash: p.policyHash(), Approver: approver}, nil
}

// ApprovedPolicy returns the last approved policy snapshot and stamp.
func ApprovedPolicy(stateDir string) (*Policy, *policyApproval) {
	data, err := os.ReadFile(approvedPolicyPath(stateDir)) // #nosec G304 -- fixed name inside state dir
	if err != nil {
		return nil, nil
	}
	var p Policy
	if json.Unmarshal(data, &p) != nil {
		return nil, nil
	}
	list, lerr := loadApprovals(stateDir)
	var last *policyApproval
	if lerr == nil && len(list) > 0 {
		last = &list[len(list)-1]
	}
	return &p, last
}

// checkPolicyApproval enforces §11.3 at daemon start: widening vs the last
// approved snapshot without approval → refuse. Narrowing auto-approves;
// first boot self-approves the baseline.
func checkPolicyApproval(p *Policy, stateDir string) error {
	approved, _ := ApprovedPolicy(stateDir)
	if approved == nil {
		return saveApproval(stateDir, p, "self-approval:first-boot", true)
	}
	if approved.policyHash() == p.policyHash() {
		return nil
	}
	widening := PolicyWidening(approved, p)
	if len(widening) > 0 {
		return fmt.Errorf("policy widened without operator approval — run `stellar-go-cli signer policy approve`: %s",
			strings.Join(widening, "; "))
	}
	// Narrowing applies immediately (P6) — record it as an automatic approval.
	return saveApproval(stateDir, p, "auto:narrowing", true)
}

// PolicyWidening diffs two policies and lists the authority-widening changes
// in the new one. Anything not listed is a narrowing or neutral change.
func PolicyWidening(old, cur *Policy) []string {
	var out []string
	rat := func(s string) *big.Rat {
		r, _ := new(big.Rat).SetString(s)
		return r
	}
	widenCaps := func(name string, oldCaps, curCaps map[string]string) {
		for asset, curLim := range curCaps {
			oldLim, ok := oldCaps[asset]
			if !ok {
				out = append(out, name+" cap added for "+asset+" ("+curLim+")")
				continue
			}
			if rat(curLim).Cmp(rat(oldLim)) > 0 {
				out = append(out, name+" cap raised for "+asset+" ("+oldLim+"→"+curLim+")")
			}
		}
	}
	widenCaps("per_tx", old.HardLimits.PerTx, cur.HardLimits.PerTx)
	widenCaps("per_day", old.HardLimits.PerDay, cur.HardLimits.PerDay)
	widenCaps("t3_above", old.Escalation.T3Above, cur.Escalation.T3Above)
	// Removing a T3 threshold also widens (no escalation needed).
	for asset := range old.Escalation.T3Above {
		if _, ok := cur.Escalation.T3Above[asset]; !ok {
			out = append(out, "t3_above threshold removed for "+asset)
		}
	}
	oldNets := map[string]bool{}
	for _, n := range old.HardLimits.Networks {
		oldNets[n] = true
	}
	for _, n := range cur.HardLimits.Networks {
		if !oldNets[n] {
			out = append(out, "network allowed: "+n)
		}
	}
	// Empty list = "any network" — the widest case.
	if len(old.HardLimits.Networks) > 0 && len(cur.HardLimits.Networks) == 0 {
		out = append(out, "network allowlist removed (any network permitted)")
	}
	curDeny := map[string]bool{}
	for _, cp := range cur.HardLimits.Deny {
		curDeny[cp] = true
	}
	for _, cp := range old.HardLimits.Deny {
		if !curDeny[cp] {
			out = append(out, "denylisted counterparty removed: "+cp)
		}
	}
	if old.Escalation.NewCounterparty && !cur.Escalation.NewCounterparty {
		out = append(out, "new-counterparty escalation disabled")
	}
	if !old.AllowUnprotected && cur.AllowUnprotected {
		out = append(out, "allow_unprotected enabled")
	}
	if !old.AllowUnclassifiedOps && cur.AllowUnclassifiedOps {
		out = append(out, "allow_unclassified_ops enabled")
	}
	return out
}
