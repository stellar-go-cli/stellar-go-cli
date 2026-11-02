package vc

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// didWebFetch retrieves a DID document over HTTPS. It is a package variable so
// tests can substitute a stub transport.
var didWebFetch = func(url string) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("did:web resolution requires https, got %q", url)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url) //nolint:gosec // URL is https-enforced above
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("did:web fetch %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ResolveEd25519Key resolves the Ed25519 public key named by
// verificationMethod within the DID document of issuerDID. It binds the
// verification method to the issuer: the method's DID must equal the issuer
// DID, so a credential cannot point its proof at a key in someone else's
// document.
//
// Supported methods: did:key (decoded locally) and did:web (fetched per the
// W3C did:web method spec). Any other method is reported as unsupported.
func ResolveEd25519Key(issuerDID, verificationMethod string) (ed25519.PublicKey, error) {
	if verificationMethod == "" {
		return nil, fmt.Errorf("invalid VC: proof.verificationMethod is missing")
	}
	if vmDID := strings.SplitN(verificationMethod, "#", 2)[0]; vmDID != issuerDID {
		return nil, fmt.Errorf("proof.verificationMethod %q does not belong to issuer %q", verificationMethod, issuerDID)
	}

	switch {
	case strings.HasPrefix(issuerDID, "did:key:"):
		return mpCrypto.DecodeEd25519PublicKeyFromDIDKey(issuerDID)
	case strings.HasPrefix(issuerDID, "did:web:"):
		return resolveDIDWebKey(issuerDID, verificationMethod)
	default:
		method := strings.SplitN(issuerDID, ":", 3)
		if len(method) < 2 {
			return nil, fmt.Errorf("invalid issuer DID %q", issuerDID)
		}
		return nil, fmt.Errorf("unsupported issuer DID method %q: only did:key and did:web can be resolved", method[1])
	}
}

// didWebDocURL maps a did:web identifier to its DID document URL:
// did:web:example.com -> https://example.com/.well-known/did.json
// did:web:example.com:team:alice -> https://example.com/team/alice/did.json
// Colons in the method-specific-id encode path separators; percent-encoded
// colons (%3A) remain literal colons (e.g. host ports).
func didWebDocURL(did string) (string, error) {
	spec := strings.TrimPrefix(did, "did:web:")
	if spec == "" {
		return "", fmt.Errorf("invalid did:web: missing domain")
	}
	parts := strings.Split(spec, ":")
	host, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid did:web host: %w", err)
	}
	if host == "" {
		return "", fmt.Errorf("invalid did:web: missing domain")
	}
	if len(parts) == 1 {
		return "https://" + host + "/.well-known/did.json", nil
	}
	var segs []string
	for _, p := range parts[1:] {
		dec, err := url.PathUnescape(p)
		if err != nil {
			return "", fmt.Errorf("invalid did:web path segment: %w", err)
		}
		segs = append(segs, dec)
	}
	return "https://" + host + "/" + strings.Join(segs, "/") + "/did.json", nil
}

// resolveDIDWebKey fetches the did:web document and returns the Ed25519 key
// material of the named verification method.
func resolveDIDWebKey(issuerDID, verificationMethod string) (ed25519.PublicKey, error) {
	docURL, err := didWebDocURL(issuerDID)
	if err != nil {
		return nil, err
	}
	body, err := didWebFetch(docURL)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", issuerDID, err)
	}

	var doc struct {
		VerificationMethod []struct {
			ID                 string                 `json:"id"`
			Type               string                 `json:"type"`
			PublicKeyMultibase string                 `json:"publicKeyMultibase"`
			PublicKeyBase58    string                 `json:"publicKeyBase58"`
			PublicKeyJwk       map[string]interface{} `json:"publicKeyJwk"`
		} `json:"verificationMethod"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse DID document for %s: %w", issuerDID, err)
	}

	for _, vm := range doc.VerificationMethod {
		id := vm.ID
		if strings.HasPrefix(id, "#") {
			id = issuerDID + id
		}
		if id != verificationMethod {
			continue
		}
		return ed25519KeyFromVerificationMaterial(vm.PublicKeyMultibase, vm.PublicKeyBase58, vm.PublicKeyJwk)
	}
	return nil, fmt.Errorf("verification method %q not found in DID document of %s", verificationMethod, issuerDID)
}

// ed25519KeyFromVerificationMaterial decodes a verification method's public
// key material: publicKeyMultibase (z + 0xed01 prefix), publicKeyJwk
// (OKP/Ed25519 x coordinate) or legacy publicKeyBase58 (raw 32 bytes).
func ed25519KeyFromVerificationMaterial(multibase, base58 string, jwk map[string]interface{}) (ed25519.PublicKey, error) {
	switch {
	case multibase != "":
		if multibase[0] != 'z' {
			return nil, fmt.Errorf("publicKeyMultibase must be base-58-btc (z prefix)")
		}
		raw, err := mpCrypto.Base58Decode(multibase[1:])
		if err != nil {
			return nil, fmt.Errorf("decode publicKeyMultibase: %w", err)
		}
		if len(raw) != 34 || raw[0] != 0xed || raw[1] != 0x01 {
			return nil, fmt.Errorf("publicKeyMultibase is not an Ed25519 key (expected 0xed01 prefix)")
		}
		return ed25519.PublicKey(raw[2:]), nil
	case jwk != nil:
		crv, _ := jwk["crv"].(string)
		x, _ := jwk["x"].(string)
		if crv != "Ed25519" || x == "" {
			return nil, fmt.Errorf("publicKeyJwk is not an Ed25519 key")
		}
		raw, err := base64.RawURLEncoding.DecodeString(x)
		if err != nil {
			return nil, fmt.Errorf("decode publicKeyJwk.x: %w", err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("publicKeyJwk.x must be 32 bytes, got %d", len(raw))
		}
		return ed25519.PublicKey(raw), nil
	case base58 != "":
		raw, err := mpCrypto.Base58Decode(base58)
		if err != nil {
			return nil, fmt.Errorf("decode publicKeyBase58: %w", err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("publicKeyBase58 must be 32 bytes, got %d", len(raw))
		}
		return ed25519.PublicKey(raw), nil
	default:
		return nil, fmt.Errorf("verification method carries no public key material")
	}
}
