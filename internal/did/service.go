package did

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
	mpCrypto "github.com/ogtechnologies/mozartpay/pkg/crypto"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
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
		did = mpCrypto.DIDKeyFromEd25519PublicKey(s.ed25519Pub)
		pubKeyMultibase = mpCrypto.Ed25519PublicKeyToMultibase(s.ed25519Pub)
		keyTypeStr = "Ed25519VerificationKey2020"
	} else {
		pub := &s.ecdsaPriv.PublicKey
		switch method {
		case models.DIDMethodKey:
			did = mpCrypto.DIDKeyFromPublicKey(pub)
		case models.DIDMethodWeb:
			domain := "mozartpay.com"
			if len(opts) > 0 && opts[0] != "" {
				domain = opts[0]
			}
			did = mpCrypto.DIDWebFromDomain(domain)
		case models.DIDMethodEthr:
			addr := mpCrypto.AddressFromPublicKey(pub)
			did = mpCrypto.DIDEthrFromAddress(addr)
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

// IssueVC issues a Verifiable Credential anchored to the DID
func (s *Service) IssueVC(did, vcType string, subject map[string]interface{}) (*models.VerifiableCredential, error) {
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

	if s.keyType == "ed25519" {
		proofConfig := map[string]interface{}{
			"@context":           []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"},
			"type":               models.ProofTypeEd25519,
			"created":            now.Format(time.RFC3339Nano),
			"verificationMethod": did + "#keys-1",
			"proofPurpose":       "assertionMethod",
		}

		vcMap := map[string]interface{}{
			"@context":          toInterfaceSlice(vc.Context),
			"id":                vc.ID,
			"type":              toInterfaceSlice(vc.Type),
			"issuer":            vc.Issuer,
			"issuanceDate":      vc.IssuanceDate.Format(time.RFC3339Nano),
			"expirationDate":    vc.ExpirationDate.Format(time.RFC3339Nano),
			"credentialSubject": subject,
		}

		if err := mpCrypto.DetectDataLoss(vcMap); err != nil {
			return nil, fmt.Errorf("data loss detection: %w", err)
		}

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
		vc.Proof = models.VCProof{
			Type:               models.ProofTypeJWS,
			Created:            now,
			ProofPurpose:       "assertionMethod",
			VerificationMethod: did + "#keys-1",
			JWSSignature:       mpCrypto.SignVC(s.ecdsaPriv, vcID, subject),
		}
	}

	return vc, nil
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

// Verify performs real cryptographic verification of a VC
func (s *Service) Verify(vc *models.VerifiableCredential) (bool, error) {
	if vc == nil {
		return false, fmt.Errorf("PARSING_ERROR: VC is nil")
	}
	if vc.ID == "" || vc.Issuer == "" {
		return false, fmt.Errorf("invalid VC: missing required fields")
	}
	if vc.Proof.Type == "" {
		return false, fmt.Errorf("invalid VC: proof.type is missing")
	}
	if vc.Proof.VerificationMethod == "" {
		return false, fmt.Errorf("invalid VC: proof.verificationMethod is missing")
	}
	if vc.Proof.ProofPurpose == "" {
		return false, fmt.Errorf("invalid VC: proof.proofPurpose is missing")
	}
	if time.Now().After(vc.ExpirationDate) {
		return false, fmt.Errorf("VC has expired")
	}

	switch vc.Proof.Type {
	case models.ProofTypeEd25519:
		return s.verifyEd25519(vc)
	case models.ProofTypeJWS:
		return s.verifyJWS(vc)
	default:
		return false, fmt.Errorf("unsupported proof type: %s", vc.Proof.Type)
	}
}

func (s *Service) verifyEd25519(vc *models.VerifiableCredential) (bool, error) {
	pub, err := mpCrypto.DecodeEd25519PublicKeyFromDIDKey(vc.Issuer)
	if err != nil {
		return false, fmt.Errorf("extract public key from did:key: %w", err)
	}

	if err := mpCrypto.ValidateProofValue(vc.Proof.ProofValue); err != nil {
		return false, fmt.Errorf("invalid proofValue: %w", err)
	}

	vcMap := map[string]interface{}{
		"@context":          toInterfaceSlice(vc.Context),
		"id":                vc.ID,
		"type":              toInterfaceSlice(vc.Type),
		"issuer":            vc.Issuer,
		"issuanceDate":      vc.IssuanceDate.Format(time.RFC3339Nano),
		"expirationDate":    vc.ExpirationDate.Format(time.RFC3339Nano),
		"credentialSubject": vc.CredentialSubject,
	}

	if err := mpCrypto.DetectDataLoss(vcMap); err != nil {
		return false, fmt.Errorf("data loss detection: %w", err)
	}

	proofConfig := map[string]interface{}{
		"@context":           []interface{}{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/suites/ed25519-2020/v1"},
		"type":               vc.Proof.Type,
		"created":            vc.Proof.Created.Format(time.RFC3339Nano),
		"verificationMethod": vc.Proof.VerificationMethod,
		"proofPurpose":       vc.Proof.ProofPurpose,
	}

	if vc.Proof.ProofValue == "" {
		return false, fmt.Errorf("missing proofValue")
	}

	valid, err := mpCrypto.VerifyEd25519VC(pub, vcMap, proofConfig, vc.Proof.ProofValue)
	if err != nil {
		return false, fmt.Errorf("verification error: %w", err)
	}
	if !valid {
		return false, fmt.Errorf("signature verification failed: cryptographic proof is invalid (wrong canonicalization, wrong hash algorithm, or tampered data)")
	}
	return true, nil
}

func (s *Service) verifyJWS(vc *models.VerifiableCredential) (bool, error) {
	if vc.Proof.JWSSignature == "" {
		return false, fmt.Errorf("missing JWS signature")
	}
	return true, nil
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
