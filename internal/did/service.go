package did

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
	mpCrypto "github.com/ogtechnologies/mozartpay/pkg/crypto"
)

// Service handles DID operations
type Service struct {
	privateKey *ecdsa.PrivateKey
}

func NewService() (*Service, error) {
	key, err := mpCrypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("key generation: %w", err)
	}
	return &Service{privateKey: key}, nil
}

func NewServiceWithKey(key *ecdsa.PrivateKey) *Service {
	return &Service{privateKey: key}
}

// CreateDID creates a DID document for the given method
func (s *Service) CreateDID(method models.DIDMethod, opts ...string) (*models.DIDDocument, error) {
	pub := &s.privateKey.PublicKey
	var did string

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

	keyID := did + "#keys-1"
	doc := &models.DIDDocument{
		Context: []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		},
		ID:     did,
		Method: method,
		VerificationMethod: []models.VerificationKey{
			{
				ID:           keyID,
				Type:         "JsonWebKey2020",
				Controller:   did,
				PublicKeyHex: mpCrypto.PublicKeyToMultibase(pub),
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

	vc := &models.VerifiableCredential{
		Context: []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://www.w3.org/2018/credentials/examples/v1",
		},
		ID:             vcID,
		Type:           []string{"VerifiableCredential", vcType},
		Issuer:         did,
		IssuanceDate:   now,
		ExpirationDate: now.Add(365 * 24 * time.Hour),
		CredentialSubject: subject,
		Proof: models.VCProof{
			Type:               "JsonWebSignature2020",
			Created:            now,
			ProofPurpose:       "assertionMethod",
			VerificationMethod: did + "#keys-1",
			JWSSignature:       mpCrypto.SignVC(s.privateKey, vcID, subject),
		},
	}
	return vc, nil
}

// IssueNationalIDVC issues a VC representing a verified National ID
func (s *Service) IssueNationalIDVC(did, holderDID, name, country, idNumber string) (*models.VerifiableCredential, error) {
	subject := map[string]interface{}{
		"id":        holderDID,
		"name":      name,
		"country":   country,
		"idNumber":  "[REDACTED-" + mpCrypto.RandomHex(4) + "]",
		"birthYear": "1985",
		"verified":  true,
		"level":     "KYC_LEVEL_2",
	}
	return s.IssueVC(did, "NationalIdentityCredential", subject)
}

// Verify simulates VC verification
func (s *Service) Verify(vc *models.VerifiableCredential) (bool, error) {
	if vc.ID == "" || vc.Issuer == "" {
		return false, fmt.Errorf("invalid VC: missing required fields")
	}
	if time.Now().After(vc.ExpirationDate) {
		return false, fmt.Errorf("VC has expired")
	}
	// In production: verify JWS against public key from DID document
	return vc.Proof.JWSSignature != "", nil
}

// PrettyPrint returns a formatted JSON representation
func PrettyPrint(v interface{}) string {
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return "  " + string(b)
}
