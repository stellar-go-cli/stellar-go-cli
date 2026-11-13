package custodian

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// enforcementRecord is the custodian-side append-only evidence record
// (spec §12.1). Every decided signing request produces one, signed or
// refused, hash-chained into state/enforcement.log.
type enforcementRecord struct {
	chainlog.Link
	Timestamp   string            `json:"ts"`
	DecisionRef string            `json:"decision_ref"`
	RequestHash string            `json:"request_hash,omitempty"`
	Tool        string            `json:"tool"`
	KeyID       string            `json:"key_id"`
	MandateRef  string            `json:"mandate_ref,omitempty"`
	AgentDID    string            `json:"agent_did,omitempty"`
	Untrusted   bool              `json:"untrusted,omitempty"`
	GrantAgeS   int64             `json:"grant_age_s,omitempty"`
	Checks      map[string]string `json:"checks"`                 // E1..E5 → pass/fail reason
	PayloadHash string            `json:"payload_hash,omitempty"` // submit_tx: consumed issued payload (rebuild tombstone)
	Approval    string            `json:"approval,omitempty"`
	Result      string            `json:"result"` // signed:<hash> | refused:<reason>
	Policy      string            `json:"policy"` // version:hash
}

// enforceAndSign is the custodian's decision point: decode → verify E1–E5 →
// sign → record. It never trusts the caller's description; every check runs
// against the decoded payload.
func (d *Daemon) enforceAndSign(kind, decisionRef, requestHash, tool, account, passphrase, payloadXDR string, validUntil uint32) (string, *signsock.RPCError) {
	return d.enforceAndSignMandated(kind, decisionRef, requestHash, tool, account, passphrase, payloadXDR, validUntil, "", "", false)
}

// enforceAndSignMandated is enforceAndSign with an optional mandate —
// mandateRef != "" takes the delegated-authority E2 path (§6.4).
func (d *Daemon) enforceAndSignMandated(kind, decisionRef, requestHash, tool, account, passphrase, payloadXDR string, validUntil uint32, mandateRef, agentPOP string, untrusted bool) (string, *signsock.RPCError) {
	checks := map[string]string{"E1": "-", "E2": "-", "E3": "-", "E4": "-", "E5": "-"}
	var agentDID string
	var grantAgeS int64
	refuse := func(check, reason string) (string, *signsock.RPCError) {
		checks[check] = "fail: " + reason
		d.record(enforcementRecord{
			Timestamp:   nowRFC3339(),
			DecisionRef: decisionRef,
			RequestHash: requestHash,
			Tool:        tool,
			KeyID:       account,
			MandateRef:  mandateRef,
			AgentDID:    agentDID,
			Untrusted:   untrusted,
			GrantAgeS:   grantAgeS,
			Checks:      checks,
			Result:      "refused:" + reason,
			Policy:      d.policyStamp(),
		})
		code := signsock.ErrRefused
		if check == "E3" {
			code = signsock.ErrEscalationRequired
		}
		return "", &signsock.RPCError{Code: code, Message: reason}
	}

	// Account resolution — signer identity must be a wallet we custody.
	acc, err := d.svc.GetWalletByAddress(account)
	if err != nil {
		return refuse("E2", fmt.Sprintf("unknown wallet %s", account))
	}

	// E4 — payload truth: fully decode, recompute the request hash over the
	// decoded description, verify the caller's hash matches.
	var desc *signsock.PayloadDesc
	if kind == signsock.MethodSignTx {
		desc, err = signsock.DescribeEnvelopeXDR(payloadXDR, passphrase)
	} else {
		desc, err = signsock.DescribeAuthEntryXDR(payloadXDR, passphrase)
	}
	if err != nil {
		return refuse("E4", err.Error())
	}
	if desc.Source != "" && !strings.EqualFold(desc.Source, account) {
		return refuse("E4", fmt.Sprintf("payload source %s does not match declared account %s", desc.Source, account))
	}
	computed := signsock.RequestHash(tool, account, desc)
	if requestHash != "" && requestHash != computed {
		return refuse("E4", "request hash does not match decoded payload")
	}
	requestHash = computed
	checks["E4"] = "pass"

	// E1 — hard limits: network allowlist, denied counterparties, per-tx caps.
	if err := d.policy.checkNetworks(passphrase); err != nil {
		return refuse("E1", err.Error())
	}
	var spendAssets, spendAmounts []string
	for _, op := range desc.Ops {
		if op.Destination != "" && d.policy.denylisted(op.Destination) {
			return refuse("E1", "counterparty "+op.Destination+" is denylisted")
		}
		if op.Amount != "" {
			if op.Asset == "" {
				return refuse("E1", "operation "+op.Type+" carries amount with no asset")
			}
			if err := d.policy.checkPerTx(op.Asset, op.Amount); err != nil {
				return refuse("E1", err.Error())
			}
			spendAssets = append(spendAssets, op.Asset)
			spendAmounts = append(spendAmounts, op.Amount)
		} else if !d.policy.AllowUnclassifiedOps && op.Amount == "" && op.Type != "" {
			return refuse("E1", "operation "+op.Type+" has no extractable amount and policy forbids unclassified ops")
		}
	}
	checks["E1"] = "pass"

	// E2 — authority: a valid covering mandate, a live grant, or policy
	// permission for plaintext wallets (§7.3 outcomes).
	var seed string
	var mandateSub *mandateSubject
	if mandateRef != "" {
		rec, merr := d.mandates.load(mandateRef)
		if merr != nil {
			return refuse("E2", merr.Error())
		}
		if verr := verifyMandate(rec, account, tool, desc, requestHash, time.Now()); verr != nil {
			return refuse("E2", "mandate: "+verr.Error())
		}
		if herr := verifyAgentPOP(rec.Subject.Agent, agentPOP, tool, account, payloadXDR); herr != nil {
			return refuse("E2", herr.Error())
		}
		// §11.1 — lifecycle gate: only an active agent may exercise mandate
		// authority. Unknown DIDs auto-enrol active — the verified mandate is
		// the authority; the registry is the operator's control layer, and a
		// lost registry must not strand valid mandates.
		agentDID = rec.Subject.Agent
		switch st := d.agents.stateOf(agentDID); st {
		case "":
			if _, aerr := d.agents.enroll(agentDID, "", "auto-enrolled on mandate use", true); aerr != nil {
				return refuse("E2", "agent enrolment failed: "+aerr.Error())
			}
		case agentActive:
		default:
			return refuse("E2", "agent "+agentDID+" is "+st)
		}
		mandateSub = rec.Subject
		if acc.PasskeyProtected {
			// Escrowed at issuance — the mandate's own validity window bounds
			// how long the seed stays in custodian memory.
			seed = d.grants.seed("mandate:" + mandateRef)
			if seed == "" {
				return refuse("E2", "mandate escrow expired (daemon restart or expiry) — re-issue the mandate or unlock the wallet")
			}
			if acc.PrivateKeyEnc == "" {
				d.grants.drop("mandate:" + mandateRef)
				return refuse("E2", "mandate escrow stale: wallet record no longer protected")
			}
		} else {
			if !d.policy.AllowUnprotected {
				return refuse("E2", "policy forbids signing for unprotected wallets")
			}
			seed = acc.PrivateKey
			if seed == "" {
				return refuse("E2", "wallet has no usable key material")
			}
		}
		checks["E2"] = "pass (mandate:" + mandateRef + ")"
	} else if acc.PasskeyProtected {
		seed = d.grants.seed(account)
		if age, ok := d.grants.age(account); ok {
			grantAgeS = int64(age.Seconds())
		}
		if seed == "" {
			return refuse("E2", fmt.Sprintf("wallet %s is passkey-protected and locked — operator must unlock via 'stellar-go-cli wallet passkey unlock'", account))
		}
		// Revalidate against the wallet record: if protection was disabled
		// after unlock, the grant is stale.
		if acc.PrivateKeyEnc == "" {
			d.grants.drop(account)
			return refuse("E2", "grant stale: wallet record no longer protected")
		}
	} else {
		if !d.policy.AllowUnprotected {
			return refuse("E2", "policy forbids signing for unprotected wallets")
		}
		seed = acc.PrivateKey
		if seed == "" {
			return refuse("E2", "wallet has no usable key material")
		}
	}
	checks["E2"] = "pass"

	// E3 — approval binding: escalation-triggered ops need a bound approval
	// whose ceremony challenge committed to this request hash.
	escalate := false
	for _, op := range desc.Ops {
		if op.Amount == "" {
			continue
		}
		if threshold := d.policy.escalationThreshold(op.Asset); threshold != "" {
			if over, err := exceeds(op.Amount, threshold); err == nil && over {
				escalate = true
			}
		}
	}
	if d.policy.Escalation.NewCounterparty {
		for _, op := range desc.Ops {
			if op.Destination != "" && !d.parties.seen(op.Destination) {
				escalate = true
			}
		}
	}
	var approvalNote string
	if escalate {
		if !acc.PasskeyProtected {
			return refuse("E3", "escalation required but wallet has no passkey to bind an approval")
		}
		if !d.approvals.consume(requestHash) {
			// §10.3 — escalation requests are rate-limited; bursts are a
			// security event (the refusal record carries the distinct reason).
			if !d.limiterFor().allow(account) {
				return refuse("E3", "escalation rate limited — too many approval requests")
			}
			descJSON, merr := json.Marshal(desc)
			if merr != nil {
				descJSON = nil
			}
			d.trackPending(&signsock.PendingRequest{
				RequestHash: requestHash,
				Tool:        tool,
				Account:     account,
				Description: string(descJSON),
				Untrusted:   untrusted,
				Timestamp:   nowRFC3339(),
			})
			return refuse("E3", fmt.Sprintf("operator approval required — run 'stellar-go-cli wallet passkey approve %s'", requestHash))
		}
		approvalNote = "bound:" + requestHash
	}
	checks["E3"] = "pass"

	// E5 — freshness: decision ref single-use (claimed inside the commit
	// boundary below so a refused attempt doesn't burn the decision).
	if decisionRef == "" {
		decisionRef = "local:" + requestHash
	}
	checks["E5"] = "pass"

	// Authority was sampled at E2 — re-validate the mutable inputs (grant,
	// escrow, mandate status, agent state) immediately before signing so a
	// revocation landing mid-request cannot produce a post-revocation
	// signature (specula-lite F9).
	if reason := d.authorityRevoked(mandateRef, agentDID, acc); reason != "" {
		return refuse("E2", reason)
	}

	// Sign before committing — the signature is pure output, so a failure
	// below still leaves no residue. Seed buffers are zeroed on all paths.
	signed, sigHash, serr := d.sign(kind, seed, acc, passphrase, payloadXDR, validUntil)
	wipeString(&seed)
	if serr != nil {
		return refuse("E4", "signing failed: "+serr.Error())
	}

	// Commit boundary — all-or-nothing: claim the decision, charge counters
	// atomically, then mark the payload submittable. A refused request
	// leaves no issued mark, no burned ref, and no partial spend
	// (specula-lite F1/F2/F3).
	if !d.decisions.claim(decisionRef) {
		return refuse("E5", "decision "+decisionRef+" already consumed")
	}
	if err := d.chargeSpend(mandateSub, mandateRef, spendAssets, spendAmounts); err != nil {
		d.decisions.release(decisionRef)
		d.record(enforcementRecord{
			Timestamp: nowRFC3339(), DecisionRef: decisionRef, RequestHash: requestHash,
			Tool: tool, KeyID: account, MandateRef: mandateRef, Checks: checks,
			Result: "refused:" + err.Error(), Policy: d.policyStamp(),
		})
		return "", &signsock.RPCError{Code: signsock.ErrRefused, Message: err.Error()}
	}
	for _, op := range desc.Ops {
		if op.Destination != "" {
			d.parties.mark(op.Destination)
		}
	}
	d.issuedMark(sigHash)

	d.record(enforcementRecord{
		Timestamp:   nowRFC3339(),
		DecisionRef: decisionRef,
		RequestHash: requestHash,
		Tool:        tool,
		KeyID:       account,
		MandateRef:  mandateRef,
		AgentDID:    agentDID,
		Untrusted:   untrusted,
		GrantAgeS:   grantAgeS,
		Checks:      checks,
		Approval:    approvalNote,
		Result:      "signed:" + sigHash,
		Policy:      d.policyStamp(),
	})
	return signed, nil
}

// chargeSpend applies a request's spend atomically: mandate cumulative
// counter first, then the global daily counter. A global failure refunds
// the mandate charge so refused requests commit nothing (specula-lite F3).
func (d *Daemon) chargeSpend(sub *mandateSubject, mandateRef string, assets, amounts []string) error {
	if sub != nil {
		if err := d.mandateCounters.chargeAll(sub, mandateRef, assets, amounts); err != nil {
			return err
		}
	}
	if err := d.counters.chargeAll(d.policy, assets, amounts); err != nil {
		if sub != nil {
			d.mandateCounters.refund(sub, mandateRef, assets, amounts)
		}
		return err
	}
	return nil
}

// authorityRevoked re-validates the mutable authority inputs sampled at E2 —
// grant/escrow liveness, mandate status, agent state — immediately before
// signing, so a revocation mid-request cannot yield a signature
// (specula-lite F9). Returns "" while authority still holds.
func (d *Daemon) authorityRevoked(mandateRef, agentDID string, acc *models.Account) string {
	if mandateRef == "" {
		if acc.PasskeyProtected && d.grants.seed(acc.Address) == "" {
			return fmt.Sprintf("grant for %s was locked or expired mid-request", acc.Address)
		}
		return ""
	}
	rec, err := d.mandates.load(mandateRef)
	if err != nil || rec.Status != mandateActive {
		return fmt.Sprintf("mandate %s is no longer active", mandateRef)
	}
	if st := d.agents.stateOf(agentDID); st != agentActive {
		return fmt.Sprintf("agent %s is no longer active (%s)", agentDID, st)
	}
	if acc.PasskeyProtected && d.grants.seed("mandate:"+mandateRef) == "" {
		return fmt.Sprintf("mandate %s escrow revoked or expired mid-request", mandateRef)
	}
	return ""
}

// sign performs the actual signature with the resolved seed.
func (d *Daemon) sign(kind, seed string, acc *models.Account, passphrase, payloadXDR string, validUntil uint32) (string, string, error) {
	kp, err := keypair.ParseFull(seed)
	if err != nil {
		return "", "", fmt.Errorf("stored seed is invalid: %w", err)
	}
	if kp.Address() != acc.Address {
		return "", "", fmt.Errorf("stored seed produces %s, not wallet %s", kp.Address(), acc.Address)
	}
	ks := signer.KeypairSigner{KP: kp}

	switch kind {
	case signsock.MethodSignTx:
		gtx, err := txnbuild.TransactionFromXDR(payloadXDR)
		if err != nil {
			return "", "", err
		}
		tx, ok := gtx.Transaction()
		if !ok {
			return "", "", fmt.Errorf("fee-bump envelopes not supported")
		}
		signed, err := ks.Sign(tx, passphrase)
		if err != nil {
			return "", "", err
		}
		b64, err := signed.Base64()
		if err != nil {
			return "", "", err
		}
		return b64, payloadHash(b64), nil

	case signsock.MethodSignSorobanAuth:
		raw, err := base64.StdEncoding.DecodeString(payloadXDR)
		if err != nil {
			return "", "", fmt.Errorf("bad entry encoding: %w", err)
		}
		var entry xdr.SorobanAuthorizationEntry
		if err := entry.UnmarshalBinary(raw); err != nil {
			return "", "", fmt.Errorf("undecodable authorization entry: %w", err)
		}
		signed, err := ks.SignAuthEntry(entry, passphrase, validUntil)
		if err != nil {
			return "", "", err
		}
		out, err := signed.MarshalBinary()
		if err != nil {
			return "", "", err
		}
		b64 := base64.StdEncoding.EncodeToString(out)
		return b64, payloadHash(b64), nil
	}
	return "", "", fmt.Errorf("unknown sign kind %q", kind)
}

func payloadHash(b64 string) string {
	sum := sha256.Sum256([]byte(b64))
	return hex.EncodeToString(sum[:16])
}

func (d *Daemon) policyStamp() string {
	return d.policy.Version + ":" + d.policy.policyHash()
}

func (d *Daemon) record(rec enforcementRecord) {
	if d.log == nil {
		return
	}
	if _, err := d.log.Append(&rec, &rec.Link); err != nil {
		d.logger.Printf("enforcement record write failed: %v", err)
	}
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }
