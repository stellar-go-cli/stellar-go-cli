// delegation.go — the custodian's delegated-authority operations:
// issue_mandate (ceremony-bound VC issuance + seed escrow), set_mandate_status
// (status registry transitions), and sign_credential (custodian-side VC proof
// for wallet-derived issuer DIDs).
package custodian

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	vc2 "github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
)

// issueMandate validates the request, builds the mandate subject, and opens
// a relayed principal ceremony bound to the exact mandate digest (§6.3). The
// client machine runs the browser; `assert` calls completeMandateIssuance.
func (d *Daemon) issueMandate(p *signsock.IssueMandateParams) (*signsock.Response, *signsock.RPCError) {
	if _, err := d.svc.GetWalletByAddress(p.Account); err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "unknown wallet " + p.Account}
	}

	until, err := time.Parse(time.RFC3339, p.ValidUntil)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "valid_until must be RFC3339: " + err.Error()}
	}
	if until.Before(time.Now()) {
		return nil, &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "valid_until is in the past"}
	}

	sub := &mandateSubject{
		Agent:          p.Agent,
		Principal:      p.Account,
		Scope:          p.Scope,
		PerTx:          p.PerTx,
		Cumulative:     p.Cumulative,
		Counterparties: p.Counterparties,
		Deny:           p.Deny,
		Networks:       p.Networks,
		PayloadDigest:  p.PayloadDigest,
		Nonce:          hex.EncodeToString(randomBytes(16)),
		Until:          until,
	}

	// Fresh principal assertion bound to the exact mandate content — a
	// compromised agent or gatekeeper cannot grant itself authority (§6.3).
	spec, err := json.Marshal(subjectToMap(sub))
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: err.Error()}
	}
	challenge := sha256.Sum256(spec)

	pend := &pendingCeremony{
		action: "issue_mandate", account: p.Account, params: mustJSON(p), sub: sub,
	}
	return d.beginCeremony(pend, challenge[:], p.Origin)
}

// completeMandateIssuance runs after the relayed assertion verified: it
// issues the VC inside the daemon and escrows the seed as a mandate-scoped
// grant for protected wallets.
func (d *Daemon) completeMandateIssuance(p *signsock.IssueMandateParams, sub *mandateSubject, seed string) (map[string]interface{}, *signsock.RPCError) {
	acc, err := d.svc.GetWalletByAddress(p.Account)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "unknown wallet " + p.Account}
	}
	until := sub.Until
	defer wipeString(&seed)

	kp, err := keypair.ParseFull(seed)
	if err != nil || kp.Address() != acc.Address {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "seed does not match wallet"}
	}

	// Issuer DID: did:key of the wallet's ed25519 key — self-contained
	// (resolves locally at verification, no external DID infra needed).
	rawSeed, derr := strkey.Decode(strkey.VersionByteSeed, seed)
	if derr != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "decode seed: " + derr.Error()}
	}
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	issuerDID := mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey))

	// Reserve the mandate's status-list positions BEFORE issuing so the
	// credentialStatus entries are inside the signed document.
	sidx, serr := d.assignStatusIndex(issuerDID, sub.Agent+":"+issuerDID+":"+subjectToMap(sub)["nonce"].(string))
	if serr != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "status index: " + serr.Error()}
	}
	extras := map[string]interface{}{"credentialStatus": statusEntries(issuerDID, sidx)}

	vcSvc := vc2.NewServiceWithEd25519Key(edPriv)
	mvc, err := vcSvc.IssueVCWithExtras(issuerDID, vcTypeMandate, subjectToMap(sub), until, extras)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "issue VC: " + err.Error()}
	}

	rec := &mandateRecord{VC: mvc, Subject: sub, Status: mandateActive}
	if err := d.mandates.put(mvc.ID, rec); err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "store mandate: " + err.Error()}
	}

	// §11.1 — mandate issuance enrols (or promotes) the agent to active: the
	// mandate is its authority; the registry tracks its lifecycle.
	if _, aerr := d.agents.enroll(sub.Agent, "", "enrolled via mandate issuance", true); aerr != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "agent enrolment: " + aerr.Error()}
	}

	// The index was reserved under a provisional id — move it to the VC id.
	d.rekeyStatusIndex(issuerDID, sub.Agent+":"+issuerDID+":"+subjectToMap(sub)["nonce"].(string), mvc.ID)
	if lerr := d.reissueStatusLists(issuerDID, seed); lerr != nil {
		d.logger.Printf("status list reissue failed: %v", lerr)
	}

	// Escrow the seed as a mandate-scoped grant for protected wallets — the
	// mandate's validity window bounds how long it stays in custodian memory.
	if acc.PasskeyProtected {
		ttl := time.Until(until)
		if ttl > 0 {
			d.grants.put("mandate:"+mvc.ID, seed, ttl)
		}
	}

	vcJSON, err := json.Marshal(mvc)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: err.Error()}
	}
	d.record(enforcementRecord{
		Timestamp:   nowRFC3339(),
		DecisionRef: "mandate-issue:" + mvc.ID,
		Tool:        signsock.MethodIssueMandate,
		KeyID:       p.Account,
		Checks:      map[string]string{"E3": "pass (bound ceremony)"},
		Result:      "mandate_issued:" + mvc.ID,
		Policy:      d.policyStamp(),
	})
	return map[string]interface{}{
		"vc_id":      mvc.ID,
		"vc":         json.RawMessage(vcJSON),
		"issuer":     issuerDID,
		"agent":      sub.Agent,
		"scope":      sub.Scope,
		"validUntil": until.Format(time.RFC3339),
		"status":     mandateActive,
	}, nil
}

// applyMandateStatus applies a status transition. Suspend/revoke are always
// allowed (P6 — privilege reduction never needs ceremony); re-activation
// goes through the relayed ceremony path (assert → action "reactivate").
// Revoking drops the escrowed grant.
func (d *Daemon) applyMandateStatus(id, status, seed string) map[string]interface{} {
	switch status {
	case mandateActive, mandateSuspended, mandateRevoked:
	default:
		return map[string]interface{}{"error": "status must be active|suspended|revoked"}
	}

	rec, err := d.mandates.load(id)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	if err := d.mandates.setStatus(id, status); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	// Re-issue the status lists BEFORE dropping the escrowed grant — the
	// escrow is the protected wallet's only signing material for the lists.
	lists := "unavailable"
	if seed != "" {
		if lerr := d.reissueStatusLists(rec.VC.Issuer, seed); lerr != nil {
			d.logger.Printf("status list reissue failed: %v", lerr)
		} else {
			lists = "updated"
		}
	}
	if status == mandateRevoked || status == mandateSuspended {
		d.grants.drop("mandate:" + id)
	}
	d.record(enforcementRecord{
		Timestamp:   nowRFC3339(),
		DecisionRef: "mandate-status:" + id + ":" + status,
		Tool:        signsock.MethodSetMandateState,
		KeyID:       rec.Subject.Principal,
		Checks:      map[string]string{"E5": "pass"},
		Result:      "mandate_" + status + ":" + id,
		Policy:      d.policyStamp(),
	})
	return map[string]interface{}{"vc_id": id, "status": status, "status_lists": lists}
}

// mandateSeedFor resolves the issuer seed for status-list re-signing:
// escrow for protected wallets, plaintext otherwise.
func (d *Daemon) mandateSeedFor(id string) string {
	rec, err := d.mandates.load(id)
	if err != nil {
		return ""
	}
	acc, err := d.svc.GetWalletByAddress(rec.Subject.Principal)
	if err != nil {
		return ""
	}
	if acc.PasskeyProtected {
		return d.grants.seed("mandate:" + id)
	}
	return acc.PrivateKey
}

// signCredential signs a VC inside the daemon — the proof key derives from
// the wallet seed and never leaves this process. Authority: a live grant
// (protected) or policy-permitted plaintext (T2-class operation).
func (d *Daemon) signCredential(p *signsock.SignCredentialParams) (map[string]interface{}, *signsock.RPCError) {
	acc, err := d.svc.GetWalletByAddress(p.Account)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "unknown wallet " + p.Account}
	}

	var seed string
	if p.MandateRef != "" {
		// Mandate authority: verify + holder binding + escrow, same as sign_tx.
		rec, merr := d.mandates.load(p.MandateRef)
		if merr != nil {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: merr.Error()}
		}
		if verr := verifyMandate(rec, p.Account, signsock.MethodSignCredential, nil, "", time.Now()); verr != nil {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "mandate: " + verr.Error()}
		}
		if herr := verifyAgentPOP(rec.Subject.Agent, p.AgentPOP, signsock.MethodSignCredential, p.Account, p.UnsignedVC); herr != nil {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: herr.Error()}
		}
		if acc.PasskeyProtected {
			seed = d.grants.seed("mandate:" + p.MandateRef)
			if seed == "" {
				return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "mandate escrow expired — re-issue the mandate or unlock the wallet"}
			}
		} else {
			if !d.policy.AllowUnprotected {
				return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "policy forbids credential signing for unprotected wallets"}
			}
			seed = acc.PrivateKey
		}
	} else if acc.PasskeyProtected {
		seed = d.grants.seed(p.Account)
		if seed == "" {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "wallet is locked — unlock via 'stellar-go-cli wallet passkey unlock'"}
		}
	} else {
		if !d.policy.AllowUnprotected {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "policy forbids credential signing for unprotected wallets"}
		}
		seed = acc.PrivateKey
		if seed == "" {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "wallet has no usable key material"}
		}
	}
	defer wipeString(&seed)

	kp, err := keypair.ParseFull(seed)
	if err != nil || kp.Address() != acc.Address {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "seed does not match wallet"}
	}

	// Foreign-issuer refusal: the issuer DID must derive from this wallet's
	// key — the custodian never signs credentials for identities it doesn't
	// custody. Candidates: ed25519 did:key and ECDSA did:key/did:ethr/did:ebsi.
	rawSeed, derr := strkey.Decode(strkey.VersionByteSeed, seed)
	if derr != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "decode seed: " + derr.Error()}
	}
	edPriv := ed25519.NewKeyFromSeed(rawSeed)
	ecdsaKey, kerr := mpCrypto.DeriveKeyFromSeed(seed)
	if kerr != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "derive ECDSA key: " + kerr.Error()}
	}
	ecdsaPub := &ecdsaKey.PublicKey
	issuerCandidates := map[string]string{
		"ed25519": mpCrypto.DIDKeyFromEd25519PublicKey(edPriv.Public().(ed25519.PublicKey)),
		"jws":     mpCrypto.DIDKeyFromPublicKey(ecdsaPub),
	}
	methodCandidates := map[string]struct{ keyType, did string }{
		"did:key":  {keyTypeFromProof(p.ProofType), ""},
		"did:ethr": {"jws", mpCrypto.DIDEthrFromPublicKey(ecdsaPub)},
		"did:ebsi": {"jws", mpCrypto.DIDEBSIFromPublicKey(ecdsaPub)},
	}

	// Resolve the intended issuer: exact IssuerDID (verified) or derived from
	// IssuerMethod for callers that can't derive the key (protected wallets).
	var issuerDID, keyType string
	if p.IssuerDID != "" {
		for kt, did := range issuerCandidates {
			if did == p.IssuerDID {
				keyType = kt
			}
		}
		// ethr/ebsi issuers are always ECDSA.
		if p.IssuerDID == mpCrypto.DIDEthrFromPublicKey(ecdsaPub) || p.IssuerDID == mpCrypto.DIDEBSIFromPublicKey(ecdsaPub) {
			keyType = "jws"
		}
		if keyType == "" {
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: fmt.Sprintf("issuer %s is not derived from wallet %s", p.IssuerDID, p.Account)}
		}
		issuerDID = p.IssuerDID
	} else {
		mc, ok := methodCandidates[p.IssuerMethod]
		if !ok {
			return nil, &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "issuer_method must be did:key | did:ethr | did:ebsi"}
		}
		keyType = mc.keyType
		if mc.did != "" {
			issuerDID = mc.did
		} else {
			issuerDID = issuerCandidates[keyType]
		}
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(p.UnsignedVC), &doc); err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "unsigned_vc_json is not valid JSON: " + err.Error()}
	}
	subject, _ := doc["credentialSubject"].(map[string]interface{})
	vcType, _ := doc["type"].([]interface{})
	vcTypeStr := ""
	for _, t := range vcType {
		if s, ok := t.(string); ok && s != "VerifiableCredential" {
			vcTypeStr = s
		}
	}
	expiry := time.Now().UTC().Add(365 * 24 * time.Hour)
	if es, ok := doc["expirationDate"].(string); ok && es != "" {
		if t, perr := time.Parse(time.RFC3339, es); perr == nil {
			expiry = t
		}
	}

	var vcSvc *vc2.Service
	if keyType == "jws" {
		vcSvc = vc2.NewServiceWithECDSAKey(ecdsaKey)
	} else {
		vcSvc = vc2.NewServiceWithEd25519Key(edPriv)
	}
	mvc, err := vcSvc.IssueVCWithValidity(issuerDID, vcTypeStr, subject, expiry)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: "sign credential: " + err.Error()}
	}
	out, err := json.Marshal(mvc)
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: err.Error()}
	}
	d.record(enforcementRecord{
		Timestamp:   nowRFC3339(),
		DecisionRef: p.DecisionRef,
		Tool:        signsock.MethodSignCredential,
		KeyID:       p.Account,
		Checks:      map[string]string{"E2": "pass (grant)"},
		Result:      "credential_signed:" + mvc.ID,
		Policy:      d.policyStamp(),
	})
	return map[string]interface{}{"signed_vc": string(out), "issuer_did": issuerDID}, nil
}

// keyTypeFromProof maps the proof-type flag to a key class.
func keyTypeFromProof(proofType string) string {
	if proofType == "jws" {
		return "jws"
	}
	return "ed25519"
}

// randomBytes returns n cryptographically random bytes.
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return b
}
