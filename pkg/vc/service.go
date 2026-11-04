package vc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
)

// Service handles DID operations
type Service struct {
	keyType     string // "ed25519" or "ecdsa"
	ed25519Priv ed25519.PrivateKey
	ed25519Pub  ed25519.PublicKey

	ecdsaPriv *ecdsa.PrivateKey
}

// NewService creates a new DID service with an ephemeral Ed25519 key (default)
func NewService() (*Service, error) {
	pub, priv, err := mpCrypto.GenerateEd25519KeyPair()
	if err != nil {
		return nil, fmt.Errorf("ed25519 key generation: %w", err)
	}
	return &Service{
		keyType:     "ed25519",
		ed25519Priv: priv,
		ed25519Pub:  pub,
	}, nil
}

// NewServiceWithECDSAKey creates a DID service using an ECDSA key (for JWS proof type)
func NewServiceWithECDSAKey(key *ecdsa.PrivateKey) *Service {
	return &Service{
		keyType:   "ecdsa",
		ecdsaPriv: key,
	}
}

// NewServiceWithEd25519Key creates a DID service with an explicit Ed25519 key
func NewServiceWithEd25519Key(priv ed25519.PrivateKey) *Service {
	return &Service{
		keyType:     "ed25519",
		ed25519Priv: priv,
		ed25519Pub:  priv.Public().(ed25519.PublicKey),
	}
}

// NewServiceWithStellarSeed creates a DID service using the wallet's Stellar seed.
// The Stellar keypair uses Ed25519 natively, so the VC is signed with the same key.
func NewServiceWithStellarSeed(seed string) (*Service, error) {
	_, err := keypair.ParseFull(seed)
	if err != nil {
		return nil, fmt.Errorf("invalid Stellar seed: %w", err)
	}
	rawSeed, err := decodeStellarStrkeySeed(seed)
	if err != nil {
		return nil, fmt.Errorf("decode stellar seed: %w", err)
	}
	priv := ed25519.NewKeyFromSeed(rawSeed)
	pub := priv.Public().(ed25519.PublicKey)
	return &Service{
		keyType:     "ed25519",
		ed25519Priv: priv,
		ed25519Pub:  pub,
	}, nil
}

// NewServiceWithKey creates a DID service with an ECDSA key (backward compat alias)
func NewServiceWithKey(key *ecdsa.PrivateKey) *Service {
	return NewServiceWithECDSAKey(key)
}

// CreateDID creates a DID document for the given method
func (s *Service) CreateDID(method models.DIDMethod, opts ...string) (*models.DIDDocument, error) {
	var did string
	var keyTypeStr string
	var pubKeyMultibase string

	if s.keyType == "ed25519" {
		switch method {
		case models.DIDMethodKey:
			did = mpCrypto.DIDKeyFromEd25519PublicKey(s.ed25519Pub)
		case models.DIDMethodWeb:
			domain := "example.com"
			if len(opts) > 0 && opts[0] != "" {
				domain = opts[0]
			}
			did = mpCrypto.DIDWebFromDomain(domain)
		default:
			return nil, fmt.Errorf("DID method %s requires an ECDSA (P-256) key — Ed25519 supports did:key and did:web", method)
		}
		pubKeyMultibase = mpCrypto.Ed25519PublicKeyToMultibase(s.ed25519Pub)
		keyTypeStr = "Ed25519VerificationKey2020"
	} else {
		pub := &s.ecdsaPriv.PublicKey
		switch method {
		case models.DIDMethodKey:
			did = mpCrypto.DIDKeyFromPublicKey(pub)
		case models.DIDMethodWeb:
			domain := "example.com"
			if len(opts) > 0 && opts[0] != "" {
				domain = opts[0]
			}
			did = mpCrypto.DIDWebFromDomain(domain)
		case models.DIDMethodEthr:
			did = mpCrypto.DIDEthrFromPublicKey(pub)
		case models.DIDMethodEBSI:
			did = mpCrypto.DIDEBSIFromPublicKey(pub)
		default:
			return nil, fmt.Errorf("unsupported DID method: %s", method)
		}
		pubKeyMultibase = mpCrypto.PublicKeyToMultibase(pub)
		keyTypeStr = "JsonWebKey2020"
	}

	keyID := did + "#keys-1"

	var contexts []string
	if s.keyType == "ed25519" {
		contexts = []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		}
	} else {
		contexts = []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		}
	}

	doc := &models.DIDDocument{
		Context: contexts,
		ID:      did,
		Method:  method,
		VerificationMethod: []models.VerificationKey{
			{
				ID:           keyID,
				Type:         keyTypeStr,
				Controller:   did,
				PublicKeyHex: pubKeyMultibase,
			},
		},
		Authentication: []string{keyID},
		Created:        time.Now().UTC(),
	}

	return doc, nil
}

// IssueVC issues a Verifiable Credential anchored to the DID. The service
// signs only for identities its key controls: the issuer DID must be
// derivable from the service key (did:key, did:ethr, did:ebsi). did:web is
// exempt — domain ownership is proven by hosting the DID document, which
// cannot be checked offline; the caller asserts control and the credential
// verifies only if the document is actually published.
func (s *Service) IssueVC(did, vcType string, subject map[string]interface{}) (*models.VerifiableCredential, error) {
	if err := s.checkIssuerControl(did); err != nil {
		return nil, err
	}
	vcID := "urn:uuid:" + mpCrypto.RandomHex(16)
	now := time.Now().UTC()

	var contexts []string
	if s.keyType == "ed25519" {
		contexts = []string{
			"https://www.w3.org/ns/credentials/v2",
			"https://www.w3.org/ns/credentials/examples/v2",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		}
	} else {
		contexts = []string{
			"https://www.w3.org/ns/credentials/v2",
			"https://www.w3.org/ns/credentials/examples/v2",
			"https://w3id.org/security/suites/jws-2020/v1",
		}
	}

	vcTypes := []string{"VerifiableCredential"}
	if vcType != "" && vcType != "VerifiableCredential" {
		vcTypes = append(vcTypes, vcType)
	}

	vc := &models.VerifiableCredential{
		Context:           contexts,
		ID:                vcID,
		Type:              vcTypes,
		Issuer:            did,
		IssuanceDate:      now,
		ExpirationDate:    now.Add(365 * 24 * time.Hour),
		CredentialSubject: subject,
	}

	proofConfig := map[string]interface{}{
		"created":            now.Format(time.RFC3339Nano),
		"verificationMethod": did + "#keys-1",
		"proofPurpose":       "assertionMethod",
	}
	if s.keyType == "ed25519" {
		proofConfig["@context"] = []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"}
		proofConfig["type"] = models.ProofTypeEd25519
	} else {
		proofConfig["@context"] = []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/jws-2020/v1"}
		proofConfig["type"] = models.ProofTypeJWS
	}

	vcMap := vcDocumentMap(vc, subject)
	if err := mpCrypto.DetectDataLoss(vcMap); err != nil {
		return nil, fmt.Errorf("data loss detection: %w", err)
	}

	if s.keyType == "ed25519" {
		sig, err := mpCrypto.SignEd25519VC(s.ed25519Priv, vcMap, proofConfig)
		if err != nil {
			return nil, fmt.Errorf("sign VC: %w", err)
		}
		vc.Proof = models.VCProof{
			Type:               models.ProofTypeEd25519,
			Created:            now,
			ProofPurpose:       "assertionMethod",
			VerificationMethod: did + "#keys-1",
			ProofValue:         sig,
		}
	} else {
		jws, err := mpCrypto.SignES256VC(s.ecdsaPriv, vcMap, proofConfig)
		if err != nil {
			return nil, fmt.Errorf("sign VC: %w", err)
		}
		vc.Proof = models.VCProof{
			Type:               models.ProofTypeJWS,
			Created:            now,
			ProofPurpose:       "assertionMethod",
			VerificationMethod: did + "#keys-1",
			JWSSignature:       jws,
		}
	}

	return vc, nil
}

// vcDocumentMap renders a VerifiableCredential as the JSON-LD map the proof
// is computed over — the credential without its proof.
func vcDocumentMap(vc *models.VerifiableCredential, subject map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"@context":          toInterfaceSlice(vc.Context),
		"id":                vc.ID,
		"type":              toInterfaceSlice(vc.Type),
		"issuer":            vc.Issuer,
		"issuanceDate":      vc.IssuanceDate.Format(time.RFC3339Nano),
		"expirationDate":    vc.ExpirationDate.Format(time.RFC3339Nano),
		"credentialSubject": subject,
	}
}

// IssueNationalIDVC issues a VC representing a verified National ID.
// birthYear and level are optional — empty birthYear is omitted from the
// subject; empty level defaults to KYC_LEVEL_2.
func (s *Service) IssueNationalIDVC(did, holderDID, name, country, idNumber, birthYear, level string) (*models.VerifiableCredential, error) {
	if level == "" {
		level = "KYC_LEVEL_2"
	}
	subject := map[string]interface{}{
		"id":       holderDID,
		"name":     name,
		"country":  country,
		"idNumber": "[REDACTED-" + mpCrypto.RandomHex(4) + "]",
		"verified": true,
		"level":    level,
	}
	if birthYear != "" {
		subject["birthYear"] = birthYear
	}
	return s.IssueVC(did, "NationalIdentityCredential", subject)
}

// Verify performs cryptographic verification of a VC model. Fields the
// model does not represent are lost when the credential was parsed into it;
// for externally issued VCs prefer VerifyDocument on the raw JSON.
func (s *Service) Verify(vc *models.VerifiableCredential) (bool, error) {
	if vc == nil {
		return false, fmt.Errorf("PARSING_ERROR: VC is nil")
	}
	raw, err := json.Marshal(vc)
	if err != nil {
		return false, fmt.Errorf("marshal VC: %w", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("unmarshal VC: %w", err)
	}
	// Unset dates marshal as year-1 strings; validity dates are optional, so
	// drop them rather than treating the credential as long-expired.
	if vc.IssuanceDate.IsZero() {
		delete(doc, "issuanceDate")
	}
	if vc.ExpirationDate.IsZero() {
		delete(doc, "expirationDate")
	}
	return VerifyDocument(doc)
}

// VerifyDocument cryptographically verifies a raw VC JSON document — the map
// produced by json.Unmarshal of the credential JSON-LD. Unlike Verify, it
// preserves fields the VerifiableCredential model does not represent, so
// externally issued VCs carrying credentialStatus, evidence or extra proof
// properties still hash to what the issuer signed. It accepts both VC 1.0
// (issuanceDate/expirationDate) and VC 2.0 (validFrom/validUntil) date names,
// all optional.
func VerifyDocument(doc map[string]interface{}) (bool, error) {
	if id, _ := doc["id"].(string); id == "" {
		return false, fmt.Errorf("invalid VC: missing id")
	}
	issuer, err := issuerFromDocument(doc)
	if err != nil {
		return false, err
	}
	proof, err := proofFromDocument(doc)
	if err != nil {
		return false, err
	}
	if err := checkValidityWindow(doc); err != nil {
		return false, err
	}

	proofType, _ := proof["type"].(string)
	switch proofType {
	case models.ProofTypeEd25519:
		return verifyEd25519Document(doc, proof, issuer)
	case models.ProofTypeJWS:
		return verifyJWSDocument(doc, proof, issuer)
	case "":
		return false, fmt.Errorf("invalid VC: proof.type is missing")
	default:
		return false, fmt.Errorf("unsupported proof type: %s", proofType)
	}
}

// issuerFromDocument extracts the issuer DID from either the string or the
// object (`{"id": ...}`) form of the issuer property.
func issuerFromDocument(doc map[string]interface{}) (string, error) {
	switch iss := doc["issuer"].(type) {
	case string:
		if iss == "" {
			return "", fmt.Errorf("invalid VC: issuer is empty")
		}
		return iss, nil
	case map[string]interface{}:
		if id, _ := iss["id"].(string); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("invalid VC: issuer object missing id")
	default:
		return "", fmt.Errorf("invalid VC: missing issuer")
	}
}

// proofFromDocument extracts the first proof from either the object or the
// array form of the proof property.
func proofFromDocument(doc map[string]interface{}) (map[string]interface{}, error) {
	switch p := doc["proof"].(type) {
	case map[string]interface{}:
		return p, nil
	case []interface{}:
		if len(p) == 0 {
			return nil, fmt.Errorf("invalid VC: empty proof set")
		}
		m, ok := p[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid VC: malformed proof")
		}
		return m, nil
	default:
		return nil, fmt.Errorf("invalid VC: missing proof")
	}
}

// checkValidityWindow enforces the VC's validity dates when present. Both
// VC 1.0 (issuanceDate/expirationDate) and VC 2.0 (validFrom/validUntil)
// names are accepted; absent dates are allowed.
func checkValidityWindow(doc map[string]interface{}) error {
	now := time.Now()
	for _, key := range []string{"validFrom", "issuanceDate"} {
		s, ok := doc[key].(string)
		if !ok || s == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("invalid VC: unparsable %s %q", key, s)
		}
		if now.Before(t) {
			return fmt.Errorf("VC is not yet valid (%s %s)", key, s)
		}
		break
	}
	for _, key := range []string{"validUntil", "expirationDate"} {
		s, ok := doc[key].(string)
		if !ok || s == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("invalid VC: unparsable %s %q", key, s)
		}
		if now.After(t) {
			return fmt.Errorf("VC has expired (%s %s)", key, s)
		}
		break
	}
	return nil
}

func verifyEd25519Document(doc, proof map[string]interface{}, issuer string) (bool, error) {
	vm, _ := proof["verificationMethod"].(string)
	if proofPurpose, _ := proof["proofPurpose"].(string); proofPurpose == "" {
		return false, fmt.Errorf("invalid VC: proof.proofPurpose is missing")
	}
	proofValue, _ := proof["proofValue"].(string)
	if err := mpCrypto.ValidateProofValue(proofValue); err != nil {
		return false, fmt.Errorf("invalid proofValue: %w", err)
	}
	pub, err := ResolveEd25519Key(issuer, vm)
	if err != nil {
		return false, err
	}

	// The signed document is the credential minus its proof.
	docCanon := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		if k != "proof" {
			docCanon[k] = v
		}
	}
	// The signed proof config is the proof minus the signature itself.
	proofConfig := make(map[string]interface{}, len(proof))
	for k, v := range proof {
		if k != "proofValue" && k != "jws" {
			proofConfig[k] = v
		}
	}
	if _, ok := proofConfig["@context"]; !ok {
		// Credentials issued before the proof carried an explicit @context
		// were signed with this default context pair.
		proofConfig["@context"] = []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		}
	}

	if err := mpCrypto.DetectDataLoss(docCanon); err != nil {
		return false, fmt.Errorf("data loss detection: %w", err)
	}

	valid, err := mpCrypto.VerifyEd25519VC(pub, docCanon, proofConfig, proofValue)
	if err != nil {
		return false, fmt.Errorf("verification error: %w", err)
	}
	if !valid {
		return false, fmt.Errorf("signature verification failed: cryptographic proof is invalid (wrong canonicalization, wrong hash algorithm, or tampered data)")
	}
	return true, nil
}

func verifyJWSDocument(doc, proof map[string]interface{}, issuer string) (bool, error) {
	vm, _ := proof["verificationMethod"].(string)
	if proofPurpose, _ := proof["proofPurpose"].(string); proofPurpose == "" {
		return false, fmt.Errorf("invalid VC: proof.proofPurpose is missing")
	}
	jws, _ := proof["jws"].(string)
	if jws == "" {
		return false, fmt.Errorf("invalid VC: proof.jws is missing")
	}
	pub, err := ResolveECDSAKey(issuer, vm)
	if err != nil {
		return false, err
	}

	// The signed document is the credential minus its proof.
	docCanon := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		if k != "proof" {
			docCanon[k] = v
		}
	}
	// The signed proof config is the proof minus the signature itself.
	proofConfig := make(map[string]interface{}, len(proof))
	for k, v := range proof {
		if k != "proofValue" && k != "jws" {
			proofConfig[k] = v
		}
	}
	if _, ok := proofConfig["@context"]; !ok {
		// Credentials issued before the proof carried an explicit @context
		// were signed with this default context pair.
		proofConfig["@context"] = []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			"https://w3id.org/security/suites/jws-2020/v1",
		}
	}

	if err := mpCrypto.DetectDataLoss(docCanon); err != nil {
		return false, fmt.Errorf("data loss detection: %w", err)
	}

	valid, err := mpCrypto.VerifyES256VC(pub, docCanon, proofConfig, jws)
	if err != nil {
		return false, fmt.Errorf("verification error: %w", err)
	}
	if !valid {
		return false, fmt.Errorf("signature verification failed: cryptographic proof is invalid (wrong canonicalization, wrong hash algorithm, or tampered data)")
	}
	return true, nil
}

// checkIssuerControl ensures the service signs only for identities its key
// controls. did:web is exempt — domain ownership is proven by hosting the DID
// document, which cannot be checked offline.
func (s *Service) checkIssuerControl(did string) error {
	if strings.HasPrefix(did, "did:web:") {
		return nil
	}
	var controlled []string
	if s.keyType == "ed25519" {
		controlled = []string{mpCrypto.DIDKeyFromEd25519PublicKey(s.ed25519Pub)}
	} else {
		pub := &s.ecdsaPriv.PublicKey
		controlled = []string{
			mpCrypto.DIDKeyFromPublicKey(pub),
			mpCrypto.DIDEthrFromPublicKey(pub),
			mpCrypto.DIDEBSIFromPublicKey(pub),
		}
	}
	if !slices.Contains(controlled, did) {
		return fmt.Errorf("issuer %q is not controlled by this service's key", did)
	}
	return nil
}

// toInterfaceSlice converts []string to []interface{}
func toInterfaceSlice(s []string) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}

// decodeStellarStrkeySeed decodes a Stellar strkey seed (S...) to raw 32 bytes
func decodeStellarStrkeySeed(seed string) ([]byte, error) {
	return strkey.Decode(strkey.VersionByteSeed, seed)
}

// PrettyPrint returns a formatted JSON representation
func PrettyPrint(v interface{}) string {
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return "  " + string(b)
}
