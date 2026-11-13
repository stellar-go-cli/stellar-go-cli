// mandate.go — delegated authority (spec §6): mandate VC parsing, the
// custodian's mandate store with status registry (revocation checked at
// every decision), independent §6.4 verification, and per-mandate cumulative
// spend counters.
package custodian

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	vc2 "github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
)

// Mandate statuses — transitions enforced by set_mandate_status.
const (
	mandateActive    = "active"
	mandateSuspended = "suspended"
	mandateRevoked   = "revoked"
	vcTypeMandate    = "MandateCredential"
)

// mandateSubject is the credentialSubject of a MandateCredential — the
// delegation terms the custodian independently enforces (§6.1 fields).
type mandateSubject struct {
	Agent          string            `json:"agent"`            // did:key of the presenting agent
	Principal      string            `json:"principal"`        // account address bound to the mandate
	Scope          []string          `json:"scope"`            // tool names permitted
	PerTx          map[string]string `json:"per_tx,omitempty"` // canonical asset -> max amount
	Cumulative     map[string]string `json:"cumulative,omitempty"`
	Counterparties []string          `json:"counterparties,omitempty"` // allowlist (empty = any)
	Deny           []string          `json:"deny,omitempty"`
	Networks       []string          `json:"networks,omitempty"`
	Delegation     bool              `json:"delegation,omitempty"`     // sub-delegation — refused in Phase 4
	PayloadDigest  string            `json:"payload_digest,omitempty"` // closed mandate binding
	Nonce          string            `json:"nonce"`                    // issuance entropy
	Until          time.Time         `json:"-"`                        // validity end — carried by the VC expirationDate, not the subject
}

// mandateRecord pairs a mandate VC with its custodian-side status.
type mandateRecord struct {
	VC      *models.VerifiableCredential `json:"vc"`
	Subject *mandateSubject              `json:"subject"`
	Status  string                       `json:"status"`
}

// mandateStore persists mandates under state/mandates/<id>.json. Status
// transitions are custodian-held — a gatekeeper cannot revive a revoked
// mandate.
type mandateStore struct {
	mu  sync.Mutex
	dir string
}

func newMandateStore(dir string) *mandateStore { return &mandateStore{dir: dir} }

func mandatePath(dir, id string) string {
	// VC ids are urn:uuid:… — sanitize for filesystem use.
	safe := strings.Map(func(r rune) rune {
		if r == ':' || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, id)
	return filepath.Join(dir, safe+".json")
}

func (s *mandateStore) put(id string, rec *mandateRecord) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(mandatePath(s.dir, id), data, 0600)
}

func (s *mandateStore) load(id string) (*mandateRecord, error) {
	data, err := os.ReadFile(mandatePath(s.dir, id)) // #nosec G304 -- id-sanitized path inside mandate dir
	if err != nil {
		return nil, fmt.Errorf("mandate %s not found", id)
	}
	var rec mandateRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("corrupt mandate %s: %w", id, err)
	}
	return &rec, nil
}

func (s *mandateStore) setStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch status {
	case mandateActive, mandateSuspended, mandateRevoked:
	default:
		return fmt.Errorf("invalid mandate status %q", status)
	}
	rec, err := s.load(id)
	if err != nil {
		return err
	}
	rec.Status = status
	return s.put(id, rec)
}

// list returns lightweight mandate metadata for gatekeeper analysis —
// enough to pick a covering mandate_ref, not the full VC blob.
func (s *mandateStore) list(account string) []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name())) // #nosec G304 -- dir listing of mandate dir
		if err != nil {
			continue
		}
		var rec mandateRecord
		if json.Unmarshal(data, &rec) != nil || rec.VC == nil || rec.Subject == nil {
			continue
		}
		if account != "" && !strings.EqualFold(rec.Subject.Principal, account) {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":        rec.VC.ID,
			"agent":     rec.Subject.Agent,
			"principal": rec.Subject.Principal,
			"scope":     rec.Subject.Scope,
			"status":    rec.Status,
			"until":     rec.VC.ExpirationDate.Format(time.RFC3339),
			"closed":    rec.Subject.PayloadDigest != "",
		})
	}
	return out
}

// verifyMandate runs the spec §6.4 checks the custodian performs
// independently — it never trusts the gatekeeper's selection.
func verifyMandate(rec *mandateRecord, account, tool string, desc *signsock.PayloadDesc, requestHash string, now time.Time) error {
	if rec.VC == nil || rec.Subject == nil {
		return fmt.Errorf("malformed mandate record")
	}
	sub := rec.Subject

	// Status: checked at every decision, never cached.
	switch rec.Status {
	case mandateActive:
	case mandateSuspended:
		return fmt.Errorf("mandate %s is suspended", rec.VC.ID)
	case mandateRevoked:
		return fmt.Errorf("mandate %s is revoked", rec.VC.ID)
	default:
		return fmt.Errorf("mandate %s has unknown status %q", rec.VC.ID, rec.Status)
	}

	// Credential integrity — signature + validity window.
	doc, err := json.Marshal(rec.VC)
	if err != nil {
		return err
	}
	var docMap map[string]interface{}
	if err := json.Unmarshal(doc, &docMap); err != nil {
		return err
	}
	ok, err := vc2.VerifyDocument(docMap)
	if err != nil {
		return fmt.Errorf("mandate verification failed: %w", err)
	}
	if !ok {
		return fmt.Errorf("mandate signature invalid")
	}

	// Validity window (VerifyDocument checks too, but be explicit).
	if now.After(rec.VC.ExpirationDate) {
		return fmt.Errorf("mandate %s expired", rec.VC.ID)
	}
	if now.Before(rec.VC.IssuanceDate) {
		return fmt.Errorf("mandate %s not yet valid", rec.VC.ID)
	}

	// Principal binding.
	if !strings.EqualFold(sub.Principal, account) {
		return fmt.Errorf("mandate principal %s does not match account %s", sub.Principal, account)
	}

	// Scope coverage.
	if !scopeCovers(sub.Scope, tool) {
		return fmt.Errorf("mandate scope %v does not cover %s", sub.Scope, tool)
	}

	// Sub-delegation refused in Phase 4.
	if sub.Delegation {
		return fmt.Errorf("sub-delegation is not supported")
	}

	// Constraints: networks, counterparties, per-op caps.
	if len(sub.Networks) > 0 && desc != nil && desc.Network != "" && !containsFold(sub.Networks, desc.Network) {
		return fmt.Errorf("mandate network constraint excludes %s", desc.Network)
	}
	if desc == nil {
		desc = &signsock.PayloadDesc{}
	}
	for _, op := range desc.Ops {
		if op.Destination != "" {
			if containsFold(sub.Deny, op.Destination) {
				return fmt.Errorf("counterparty %s denied by mandate", op.Destination)
			}
			if len(sub.Counterparties) > 0 && !containsFold(sub.Counterparties, op.Destination) {
				return fmt.Errorf("counterparty %s not in mandate allowlist", op.Destination)
			}
		}
		if op.Amount != "" {
			if op.Asset == "" {
				return fmt.Errorf("operation %s carries amount with no asset", op.Type)
			}
			capStr, ok := sub.PerTx[op.Asset]
			if !ok {
				return fmt.Errorf("mandate has no per-tx cap for asset %s", op.Asset)
			}
			over, err := exceeds(op.Amount, capStr)
			if err != nil {
				return err
			}
			if over {
				return fmt.Errorf("amount %s %s exceeds mandate per-tx cap %s", op.Amount, op.Asset, capStr)
			}
		}
	}

	// Closed mandate: bound to exactly one payload.
	if sub.PayloadDigest != "" && !strings.EqualFold(sub.PayloadDigest, requestHash) {
		return fmt.Errorf("closed mandate digest does not match this signsock.Request")
	}
	return nil
}

func scopeCovers(scope []string, tool string) bool {
	for _, s := range scope {
		if s == tool || s == "*" {
			return true
		}
		// Wildcard prefix: "pay_*" covers pay_send, pay_x402, ...
		if strings.HasSuffix(s, "*") && strings.HasPrefix(tool, strings.TrimSuffix(s, "*")) {
			return true
		}
	}
	return false
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// ── per-mandate cumulative counters ─────────────────────────────

// mandateCounters tracks cumulative spend per mandate (mandate-level
// cumulative caps in addition to the policy's global counters).
type mandateCounters struct {
	mu   sync.Mutex
	path string
	// mandateID -> date(YYYY-MM-DD) -> canonical asset -> summed amount
	usage map[string]map[string]map[string]string
}

func loadMandateCounters(path string) *mandateCounters {
	c := &mandateCounters{path: path, usage: map[string]map[string]map[string]string{}}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed filename inside operator state dir
	if err == nil {
		// #nosec G104 -- corrupt counters restart at zero, never widen limits
		json.Unmarshal(data, &c.usage) //nolint:errcheck
	}
	return c
}

// chargeAll atomically validates and applies a request's spend against the
// mandate's cumulative caps — all capped ops commit under one lock or none
// do (specula-lite F3).
func (c *mandateCounters) chargeAll(sub *mandateSubject, mandateID string, assets, amounts []string) error {
	capped := map[string]string{}
	for i, asset := range assets {
		if capStr, ok := sub.Cumulative[asset]; ok && capStr != "" {
			capped[asset] = addAmount(capped[asset], amounts[i])
		}
	}
	if len(capped) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	day := today()
	byDate := c.usage[mandateID]
	if byDate == nil {
		byDate = map[string]map[string]string{}
		c.usage[mandateID] = byDate
	}
	byAsset := byDate[day]
	if byAsset == nil {
		byAsset = map[string]string{}
		byDate[day] = byAsset
	}
	for asset, amt := range capped {
		capStr := sub.Cumulative[asset]
		total := addAmount(byAsset[asset], amt)
		over, err := exceeds(total, capStr)
		if err != nil {
			return err
		}
		if over {
			return fmt.Errorf("mandate cumulative cap %s %s/day exceeded (would be %s)", capStr, asset, total)
		}
	}
	for asset, amt := range capped {
		byAsset[asset] = addAmount(byAsset[asset], amt)
	}
	return c.save()
}

// refund reverses a chargeAll — used when a later commit step fails so a
// refused request leaves no spend behind.
func (c *mandateCounters) refund(sub *mandateSubject, mandateID string, assets, amounts []string) {
	capped := map[string]string{}
	for i, asset := range assets {
		if capStr, ok := sub.Cumulative[asset]; ok && capStr != "" {
			capped[asset] = addAmount(capped[asset], amounts[i])
		}
	}
	if len(capped) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	byAsset := c.usage[mandateID][today()]
	if byAsset == nil {
		return
	}
	for asset, amt := range capped {
		byAsset[asset] = subAmount(byAsset[asset], amt)
	}
	c.save() //nolint:errcheck // refund is best-effort bookkeeping
}

func (c *mandateCounters) save() error {
	data, err := json.Marshal(c.usage)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600) // #nosec G304 -- fixed filename inside operator state dir
}

// subjectToMap renders the subject for embedding in the VC.
func subjectToMap(sub *mandateSubject) map[string]interface{} {
	data, _ := json.Marshal(sub) //nolint:errcheck // fixed struct, cannot fail
	var m map[string]interface{}
	// #nosec G104 -- same
	json.Unmarshal(data, &m) //nolint:errcheck // same
	return m
}

// verifyAgentPOP checks the holder-binding proof: the agent's Ed25519
// signature over signsock.AgentPOPPreimage must verify under the mandate's agent
// did:key (§6.4 check 2 — the mandate cannot be relayed to another agent).
func verifyAgentPOP(agentDID, popB64, tool, account, payloadXDR string) error {
	pub, err := mpCrypto.DecodeEd25519PublicKeyFromDIDKey(agentDID)
	if err != nil {
		return fmt.Errorf("mandate agent is not a did:key: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(popB64)
	if err != nil {
		return fmt.Errorf("malformed agent_pop")
	}
	if !ed25519.Verify(pub, signsock.AgentPOPPreimage(tool, account, payloadXDR), sig) {
		return fmt.Errorf("agent_pop signature invalid — holder binding failed")
	}
	return nil
}
