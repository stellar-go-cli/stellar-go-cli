package vc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func toDoc(t *testing.T, cred *models.VerifiableCredential) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(cred)
	if err != nil {
		t.Fatalf("marshal VC: %v", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal VC: %v", err)
	}
	return doc
}

func newService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func issueSelfSigned(t *testing.T, svc *Service, subject map[string]interface{}) map[string]interface{} {
	t.Helper()
	doc, err := svc.CreateDID(models.DIDMethodKey)
	if err != nil {
		t.Fatalf("CreateDID: %v", err)
	}
	cred, err := svc.IssueVC(doc.ID, "TestCredential", subject)
	if err != nil {
		t.Fatalf("IssueVC: %v", err)
	}
	return toDoc(t, cred)
}

func TestVerifyDocumentSelfIssuedRoundTrip(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:subject"})

	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyModelPathRoundTrip(t *testing.T) {
	svc := newService(t)
	doc, err := svc.CreateDID(models.DIDMethodKey)
	if err != nil {
		t.Fatalf("CreateDID: %v", err)
	}
	cred, err := svc.IssueVC(doc.ID, "TestCredential", map[string]interface{}{"id": doc.ID})
	if err != nil {
		t.Fatalf("IssueVC: %v", err)
	}
	valid, err := svc.Verify(cred)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentTamperedSubject(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a", "level": "KYC_LEVEL_1"})
	doc["credentialSubject"].(map[string]interface{})["level"] = "KYC_LEVEL_9"

	if _, err := VerifyDocument(doc); err == nil {
		t.Fatal("expected verification to fail for tampered subject")
	}
}

// A VC signed over fields outside the VerifiableCredential model must verify
// — the model can neither parse nor rebuild them.
func TestVerifyDocumentPreservesUnmodeledFields(t *testing.T) {
	svc := newService(t)
	issuer := mpCrypto.DIDKeyFromEd25519PublicKey(svc.ed25519Pub)
	now := time.Now().UTC()
	subject := map[string]interface{}{"id": "did:key:holder", "role": "provider"}
	doc := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			"https://www.w3.org/ns/credentials/examples/v2",
		},
		"id":                "urn:uuid:ext-1",
		"type":              []interface{}{"VerifiableCredential", "ReliabilityCredential"},
		"issuer":            issuer,
		"issuanceDate":      now.Format(time.RFC3339Nano),
		"credentialSubject": subject,
		// unmodeled field, defined in the credentials v2 context
		"credentialStatus": map[string]interface{}{
			"id":   "https://status.example/1",
			"type": "BitstringStatusListEntry",
		},
	}
	proofConfig := map[string]interface{}{
		"@context":           []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"},
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"verificationMethod": issuer + "#keys-1",
		"proofPurpose":       "assertionMethod",
	}
	sig, err := mpCrypto.SignEd25519VC(svc.ed25519Priv, doc, proofConfig)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	doc["proof"] = map[string]interface{}{
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": issuer + "#keys-1",
		"proofValue":         sig,
	}

	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentV2Dates(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})

	// Re-sign the document under v2 names so the signature still matches.
	delete(doc, "issuanceDate")
	delete(doc, "expirationDate")
	doc["validFrom"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	doc["validUntil"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	resign(t, svc, doc)

	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument with v2 dates: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentMissingDatesAllowed(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})
	delete(doc, "issuanceDate")
	delete(doc, "expirationDate")
	resign(t, svc, doc)

	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument without dates: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentExpiredAndNotYetValid(t *testing.T) {
	svc := newService(t)

	expired := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})
	delete(expired, "issuanceDate")
	delete(expired, "expirationDate")
	expired["validUntil"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	resign(t, svc, expired)
	if _, err := VerifyDocument(expired); err == nil {
		t.Fatal("expected expired credential to fail")
	}

	notYet := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})
	delete(notYet, "issuanceDate")
	delete(notYet, "expirationDate")
	notYet["validFrom"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	resign(t, svc, notYet)
	if _, err := VerifyDocument(notYet); err == nil {
		t.Fatal("expected not-yet-valid credential to fail")
	}
}

// resign re-signs a mutated document in place, preserving signature validity.
func resign(t *testing.T, svc *Service, doc map[string]interface{}) {
	t.Helper()
	issuer := mpCrypto.DIDKeyFromEd25519PublicKey(svc.ed25519Pub)
	delete(doc, "proof")
	now := time.Now().UTC()
	proofConfig := map[string]interface{}{
		"@context":           []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"},
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"verificationMethod": issuer + "#keys-1",
		"proofPurpose":       "assertionMethod",
	}
	signDoc := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		signDoc[k] = v
	}
	sig, err := mpCrypto.SignEd25519VC(svc.ed25519Priv, signDoc, proofConfig)
	if err != nil {
		t.Fatalf("resign: %v", err)
	}
	doc["proof"] = map[string]interface{}{
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": issuer + "#keys-1",
		"proofValue":         sig,
	}
}

func newECDSAService(t *testing.T) *Service {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return NewServiceWithECDSAKey(key)
}

func issueJWS(t *testing.T, svc *Service, method models.DIDMethod, subject map[string]interface{}, opts ...string) (string, map[string]interface{}) {
	t.Helper()
	doc, err := svc.CreateDID(method, opts...)
	if err != nil {
		t.Fatalf("CreateDID %s: %v", method, err)
	}
	cred, err := svc.IssueVC(doc.ID, "TestCredential", subject)
	if err != nil {
		t.Fatalf("IssueVC: %v", err)
	}
	return doc.ID, toDoc(t, cred)
}

func TestVerifyDocumentJWSEBSIRoundTrip(t *testing.T) {
	svc := newECDSAService(t)
	issuer, doc := issueJWS(t, svc, models.DIDMethodEBSI, map[string]interface{}{"id": "did:key:subject"})
	if !strings.HasPrefix(issuer, "did:ebsi:z") {
		t.Fatalf("expected did:ebsi issuer, got %s", issuer)
	}
	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentJWSEthrRoundTrip(t *testing.T) {
	svc := newECDSAService(t)
	issuer, doc := issueJWS(t, svc, models.DIDMethodEthr, map[string]interface{}{"id": "did:key:subject"})
	if !strings.HasPrefix(issuer, "did:ethr:0x04") {
		t.Fatalf("expected public-key-form did:ethr issuer, got %s", issuer)
	}
	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentJWSDIDKeyRoundTrip(t *testing.T) {
	svc := newECDSAService(t)
	_, doc := issueJWS(t, svc, models.DIDMethodKey, map[string]interface{}{"id": "did:key:subject"})
	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
}

func TestVerifyDocumentJWSTamperedSubject(t *testing.T) {
	svc := newECDSAService(t)
	_, doc := issueJWS(t, svc, models.DIDMethodEBSI, map[string]interface{}{"id": "did:key:a", "level": "KYC_LEVEL_1"})
	doc["credentialSubject"].(map[string]interface{})["level"] = "KYC_LEVEL_9"
	if _, err := VerifyDocument(doc); err == nil {
		t.Fatal("expected verification to fail for tampered subject")
	}
}

func TestVerifyDocumentJWSIssuerBinding(t *testing.T) {
	svc := newECDSAService(t)
	_, doc := issueJWS(t, svc, models.DIDMethodEBSI, map[string]interface{}{"id": "did:key:a"})
	doc["proof"].(map[string]interface{})["verificationMethod"] = "did:ebsi:zSomebodyElse#keys-1"
	if _, err := VerifyDocument(doc); err == nil {
		t.Fatal("expected verificationMethod/issuer mismatch to fail")
	}
}

func TestResolveECDSAKeyRejectsUnresolvableDIDs(t *testing.T) {
	// Address-form did:ethr carries no key material — it needs a registry.
	if _, err := ResolveECDSAKey("did:ethr:0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb0", "did:ethr:0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb0#controller"); err == nil {
		t.Fatal("expected address-form did:ethr resolution to fail")
	}
	// Hash-based did:ebsi (16-byte digest form) carries no key material.
	if _, err := ResolveECDSAKey("did:ebsi:z8aBCD2nVqZ1xKj", "did:ebsi:z8aBCD2nVqZ1xKj#keys-1"); err == nil {
		t.Fatal("expected hash-based did:ebsi resolution to fail")
	}
	// An ed25519 did:key has no ECDSA key material.
	edSvc := newService(t)
	if _, err := ResolveECDSAKey(mpCrypto.DIDKeyFromEd25519PublicKey(edSvc.ed25519Pub), mpCrypto.DIDKeyFromEd25519PublicKey(edSvc.ed25519Pub)+"#keys-1"); err == nil {
		t.Fatal("expected ed25519 did:key ECDSA resolution to fail")
	}
}

func TestVerifyDocumentJWSMalformed(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})
	doc["proof"] = map[string]interface{}{
		"type":               models.ProofTypeJWS,
		"created":            time.Now().UTC().Format(time.RFC3339Nano),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": "did:key:zfake#k1",
		"jws":                "simulated",
	}
	if _, err := VerifyDocument(doc); err == nil {
		t.Fatal("expected malformed JWS proof to fail")
	}
}

func TestVerifyDocumentIssuerBinding(t *testing.T) {
	svc := newService(t)
	doc := issueSelfSigned(t, svc, map[string]interface{}{"id": "did:key:a"})

	// Point the proof at a verification method in a different DID — the
	// signature was made over that vm too, but the binding check must reject
	// it before signature verification.
	doc["proof"].(map[string]interface{})["verificationMethod"] = "did:key:zSomebodyElse#keys-1"
	if _, err := VerifyDocument(doc); err == nil {
		t.Fatal("expected verificationMethod/issuer mismatch to fail")
	}
}

func TestVerifyDocumentDIDWebIssuer(t *testing.T) {
	svc := newService(t)
	pubMultibase := mpCrypto.Ed25519PublicKeyToMultibase(svc.ed25519Pub)
	issuerDID := "did:web:issuer.example"
	vmID := issuerDID + "#key-1"

	didDoc := fmt.Sprintf(`{
		"@context": ["https://www.w3.org/ns/did/v1"],
		"id": %q,
		"verificationMethod": [{
			"id": %q,
			"type": "Ed25519VerificationKey2020",
			"controller": %q,
			"publicKeyMultibase": %q
		}]
	}`, issuerDID, vmID, issuerDID, pubMultibase)

	orig := didWebFetch
	defer func() { didWebFetch = orig }()
	var fetchedURL string
	didWebFetch = func(url string) ([]byte, error) {
		fetchedURL = url
		return []byte(didDoc), nil
	}

	cred, err := svc.IssueVC(issuerDID, "ReliabilityCredential", map[string]interface{}{"id": "did:web:anchor.example"})
	if err != nil {
		t.Fatalf("IssueVC with did:web issuer: %v", err)
	}
	doc := toDoc(t, cred)
	// Align the verification method id with the DID doc's key id.
	doc["proof"].(map[string]interface{})["verificationMethod"] = vmID
	resignWeb(t, svc, doc, vmID)

	valid, err := VerifyDocument(doc)
	if err != nil {
		t.Fatalf("VerifyDocument did:web issuer: %v", err)
	}
	if !valid {
		t.Fatal("expected valid credential")
	}
	if fetchedURL != "https://issuer.example/.well-known/did.json" {
		t.Fatalf("unexpected did:web fetch URL %q", fetchedURL)
	}
}

func resignWeb(t *testing.T, svc *Service, doc map[string]interface{}, vmID string) {
	t.Helper()
	delete(doc, "proof")
	now := time.Now().UTC()
	signDoc := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		signDoc[k] = v
	}
	proofConfig := map[string]interface{}{
		"@context":           []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"},
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"verificationMethod": vmID,
		"proofPurpose":       "assertionMethod",
	}
	sig, err := mpCrypto.SignEd25519VC(svc.ed25519Priv, signDoc, proofConfig)
	if err != nil {
		t.Fatalf("resign: %v", err)
	}
	doc["proof"] = map[string]interface{}{
		"type":               models.ProofTypeEd25519,
		"created":            now.Format(time.RFC3339Nano),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": vmID,
		"proofValue":         sig,
	}
}

func TestDIDWebDocURL(t *testing.T) {
	cases := []struct{ did, want string }{
		{"did:web:example.com", "https://example.com/.well-known/did.json"},
		{"did:web:example.com:team:alice", "https://example.com/team/alice/did.json"},
		{"did:web:localhost%3A3000", "https://localhost:3000/.well-known/did.json"},
	}
	for _, tc := range cases {
		got, err := didWebDocURL(tc.did)
		if err != nil {
			t.Fatalf("didWebDocURL(%q): %v", tc.did, err)
		}
		if got != tc.want {
			t.Fatalf("didWebDocURL(%q) = %q, want %q", tc.did, got, tc.want)
		}
	}
}

func TestIssueVCRejectsForeignIssuer(t *testing.T) {
	svc := newService(t)
	other := newService(t)
	otherDoc, err := other.CreateDID(models.DIDMethodKey)
	if err != nil {
		t.Fatalf("CreateDID: %v", err)
	}
	if _, err := svc.IssueVC(otherDoc.ID, "TestCredential", map[string]interface{}{"id": "did:key:x"}); err == nil {
		t.Fatal("expected issuer-control check to reject a foreign did:key issuer")
	}
	if _, err := svc.IssueVC("did:ethr:0x1234", "TestCredential", map[string]interface{}{}); err == nil {
		t.Fatal("expected issuer-control check to reject did:ethr under ed25519")
	}
}

func TestCreateDIDMethodAlignment(t *testing.T) {
	svc := newService(t)
	webDoc, err := svc.CreateDID(models.DIDMethodWeb, "issuer.example")
	if err != nil {
		t.Fatalf("CreateDID web: %v", err)
	}
	if webDoc.ID != "did:web:issuer.example" {
		t.Fatalf("expected did:web:issuer.example, got %s", webDoc.ID)
	}
	if _, err := svc.CreateDID(models.DIDMethodEthr); err == nil {
		t.Fatal("expected ed25519 + did:ethr to error")
	}
	if _, err := svc.CreateDID(models.DIDMethodEBSI); err == nil {
		t.Fatal("expected ed25519 + did:ebsi to error")
	}
}

func TestECDSAIssuerControl(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	svc := NewServiceWithECDSAKey(key)
	doc, err := svc.CreateDID(models.DIDMethodKey)
	if err != nil {
		t.Fatalf("CreateDID: %v", err)
	}
	if _, err := svc.IssueVC(doc.ID, "T", map[string]interface{}{}); err != nil {
		t.Fatalf("IssueVC with own did:key should be allowed: %v", err)
	}
	if _, err := svc.IssueVC("did:key:zQmNotMine", "T", map[string]interface{}{}); err == nil {
		t.Fatal("expected issuer-control check to reject foreign did:key")
	}
}

func TestIssueVCRejectsNilKey(t *testing.T) {
	svc := &Service{keyType: "ed25519"} // no key material
	if _, err := svc.IssueVC("did:key:x", "T", map[string]interface{}{}); err == nil {
		t.Fatal("expected error for missing key")
	}
}
