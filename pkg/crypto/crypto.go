package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/piprate/json-gold/ld"
)

// GenerateKeyPair generates an ECDSA P-256 key pair
func GenerateKeyPair() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// DeriveKeyFromSeed derives a deterministic ECDSA P-256 private key from a
// Stellar seed (S...) using SHA-256 as a KDF. The same seed always produces
// the same key pair, linking the VC signature to the wallet owner.
func DeriveKeyFromSeed(seed string) (*ecdsa.PrivateKey, error) {
	h := sha256.Sum256([]byte("mozartpay-did-key-derivation:" + seed))
	d := new(big.Int).SetBytes(h[:])
	curve := elliptic.P256()
	d.Mod(d, curve.Params().N)
	if d.Sign() == 0 {
		h[0] = 1
		d.SetBytes(h[:])
		d.Mod(d, curve.Params().N)
	}
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: curve},
		D:         d,
	}
	priv.PublicKey.X, priv.PublicKey.Y = curve.ScalarBaseMult(d.Bytes())
	return priv, nil
}

// PublicKeyToHex encodes the public key as uncompressed hex
func PublicKeyToHex(pub *ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	return hex.EncodeToString(b)
}

// PublicKeyToMultibase returns a multibase-encoded (base58btc) public key
func PublicKeyToMultibase(pub *ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	// z prefix = base58btc multibase
	return "z" + base58Encode(b)
}

// AddressFromPublicKey derives an Ethereum-style address from a public key
func AddressFromPublicKey(pub *ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	h := sha256.Sum256(b[1:]) // skip 0x04 prefix
	return "0x" + hex.EncodeToString(h[12:])
}

// StellarAddressFromPublicKey derives a Stellar-style address (G...)
func StellarAddressFromPublicKey(pub *ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	encoded := strings.ToUpper(base32Encode(b[:32]))
	if len(encoded) > 54 {
		encoded = encoded[:54]
	}
	return "G" + encoded
}

// Hash256 returns hex-encoded SHA-256
func Hash256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// RandomHex returns n random bytes as hex
func RandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// RandomBase64 returns n random bytes as base64url
func RandomBase64(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// SimulatedSign creates a deterministic pseudo-signature for MVP
func SimulatedSign(privateKey *ecdsa.PrivateKey, data []byte) string {
	h := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, h[:])
	if err != nil {
		return RandomHex(64)
	}
	sig := append(r.Bytes(), s.Bytes()...)
	return base64.RawURLEncoding.EncodeToString(sig)
}

// TxHash generates a realistic-looking transaction hash
func TxHash() string {
	return "0x" + RandomHex(32)
}

// StellarTxHash generates a Stellar-style transaction hash
func StellarTxHash() string {
	return strings.ToUpper(RandomHex(32))
}

// ContractID generates a Stellar SAC contract ID
func ContractID() string {
	b := make([]byte, 32)
	rand.Read(b)
	encoded := strings.ToUpper(base32Encode(b))
	if len(encoded) > 54 {
		encoded = encoded[:54]
	}
	return "C" + encoded
}

// ─────────────────────────────────────────────
// Ed25519 key generation and helpers
// ─────────────────────────────────────────────

// GenerateEd25519KeyPair generates an Ed25519 key pair
func GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// Ed25519PublicKeyToMultibase returns a multibase-encoded (base58btc) Ed25519 public key
// with the 0xed01 multicodec prefix per the did:key spec.
func Ed25519PublicKeyToMultibase(pub ed25519.PublicKey) string {
	b := append([]byte{0xed, 0x01}, pub...)
	return "z" + base58Encode(b)
}

// DIDKeyFromEd25519PublicKey builds a did:key identifier for an Ed25519 public key
func DIDKeyFromEd25519PublicKey(pub ed25519.PublicKey) string {
	return "did:key:" + Ed25519PublicKeyToMultibase(pub)
}

// DecodeEd25519PublicKeyFromDIDKey extracts the Ed25519 public key from a did:key identifier
func DecodeEd25519PublicKeyFromDIDKey(didKey string) (ed25519.PublicKey, error) {
	// did:key:z... → strip prefix
	encoded := strings.TrimPrefix(didKey, "did:key:")
	if !strings.HasPrefix(encoded, "z") {
		return nil, fmt.Errorf("invalid did:key: expected z prefix")
	}
	encoded = encoded[1:]
	decoded, err := base58Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid did:key: base58 decode: %w", err)
	}
	// Must be 0xed01 + 32 bytes
	if len(decoded) != 34 || decoded[0] != 0xed || decoded[1] != 0x01 {
		return nil, fmt.Errorf("invalid did:key: expected 0xed01 prefix for Ed25519")
	}
	return ed25519.PublicKey(decoded[2:]), nil
}

// SignEd25519 signs data with Ed25519 and returns a multibase-encoded (base58btc) signature
func SignEd25519(priv ed25519.PrivateKey, data []byte) string {
	sig := ed25519.Sign(priv, data)
	return "z" + base58Encode(sig)
}

// VerifyEd25519 verifies a multibase-encoded Ed25519 signature
func VerifyEd25519(pub ed25519.PublicKey, data []byte, sig string) bool {
	if !strings.HasPrefix(sig, "z") {
		return false
	}
	decoded, err := base58Decode(sig[1:])
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, data, decoded)
}

// ─────────────────────────────────────────────
// URDNA2015 RDF Dataset Canonicalization
// ─────────────────────────────────────────────

// CanonicalizeRDF normalizes a JSON-LD document using URDNA2015
// and returns canonical N-Quads as bytes.
func CanonicalizeRDF(doc map[string]interface{}) ([]byte, error) {
	proc := ld.NewJsonLdProcessor()
	opts := ld.NewJsonLdOptions("")
	opts.Format = "application/n-quads"
	opts.Algorithm = ld.AlgorithmURDNA2015
	normalized, err := proc.Normalize(doc, opts)
	if err != nil {
		return nil, fmt.Errorf("URDNA2015 normalize: %w", err)
	}
	nquads, ok := normalized.(string)
	if !ok {
		return nil, fmt.Errorf("URDNA2015: unexpected normalized type")
	}
	return []byte(nquads), nil
}

// DetectDataLoss expands a JSON-LD document and checks for undefined terms.
// If any property is dropped during expansion (i.e. not in the context),
// it returns an error indicating data loss.
func DetectDataLoss(doc map[string]interface{}) error {
	proc := ld.NewJsonLdProcessor()
	opts := ld.NewJsonLdOptions("")
	opts.ProcessingMode = ld.JsonLd_1_1
	expanded, err := proc.Expand(doc, opts)
	if err != nil {
		return fmt.Errorf("JSON-LD expansion failed: %w", err)
	}
	if err := checkExpandedForDataLoss(doc, expanded); err != nil {
		return err
	}
	return nil
}

// checkExpandedForDataLoss recursively compares the original document keys
// against the expanded form to detect dropped (undefined) properties.
func checkExpandedForDataLoss(original map[string]interface{}, expanded []interface{}) error {
	if len(expanded) == 0 {
		return fmt.Errorf("DATA_LOSS_DETECTION_ERROR: expansion produced empty result")
	}
	expandedMap, ok := expanded[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("DATA_LOSS_DETECTION_ERROR: expanded form is not a map")
	}
	return compareKeys(original, expandedMap)
}

func compareKeys(original, expanded map[string]interface{}) error {
	for key := range original {
		if key == "@context" {
			continue
		}
		if !keyExistsInExpanded(key, expanded) {
			return fmt.Errorf("DATA_LOSS_DETECTION_ERROR: property %q is not defined in the JSON-LD context", key)
		}
	}
	return nil
}

func keyExistsInExpanded(key string, expanded map[string]interface{}) bool {
	switch key {
	case "id":
		_, ok := expanded["@id"]
		return ok
	case "type":
		_, ok := expanded["@type"]
		return ok
	}
	if strings.HasPrefix(key, "@") {
		_, ok := expanded[key]
		return ok
	}
	for expKey := range expanded {
		if strings.HasSuffix(expKey, "#"+key) || strings.HasSuffix(expKey, "/"+key) {
			return true
		}
	}
	return false
}

// ValidateProofValue decodes a multibase base-58-btc proofValue and verifies
// its raw byte length is 64 (for 32-byte Ed25519 keys).
func ValidateProofValue(proofValue string) error {
	if proofValue == "" {
		return fmt.Errorf("proofValue is empty")
	}
	if proofValue[0] != 'z' {
		return fmt.Errorf("proofValue must use multibase base-58-btc (z prefix)")
	}
	decoded, err := base58Decode(proofValue[1:])
	if err != nil {
		return fmt.Errorf("decode proofValue: %w", err)
	}
	if len(decoded) != 64 {
		return fmt.Errorf("proofValue must be 64 bytes for 32-byte Ed25519 key, got %d", len(decoded))
	}
	return nil
}

// HashData returns SHA-256 raw bytes
func HashData(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// SignEd25519VC signs a VC using the Ed25519Signature2020 approach:
// 1. Canonicalize VC (without proof) → SHA-256
// 2. Canonicalize proof config (type, created, verificationMethod, proofPurpose) → SHA-256
// 3. Concatenate hashes and sign with Ed25519
func SignEd25519VC(priv ed25519.PrivateKey, vcMap, proofConfig map[string]interface{}) (string, error) {
	vcCanonical, err := CanonicalizeRDF(vcMap)
	if err != nil {
		return "", fmt.Errorf("canonicalize VC: %w", err)
	}
	configCanonical, err := CanonicalizeRDF(proofConfig)
	if err != nil {
		return "", fmt.Errorf("canonicalize proof config: %w", err)
	}
	vcHash := HashData(vcCanonical)
	configHash := HashData(configCanonical)
	combined := append(vcHash, configHash...)
	return SignEd25519(priv, combined), nil
}

// VerifyEd25519VC verifies an Ed25519Signature2020 proof
func VerifyEd25519VC(pub ed25519.PublicKey, vcMap, proofConfig map[string]interface{}, sig string) (bool, error) {
	vcCanonical, err := CanonicalizeRDF(vcMap)
	if err != nil {
		return false, fmt.Errorf("canonicalize VC: %w", err)
	}
	configCanonical, err := CanonicalizeRDF(proofConfig)
	if err != nil {
		return false, fmt.Errorf("canonicalize proof config: %w", err)
	}
	vcHash := HashData(vcCanonical)
	configHash := HashData(configCanonical)
	combined := append(vcHash, configHash...)
	return VerifyEd25519(pub, combined, sig), nil
}

// ─────────────────────────────────────────────
// DID helpers
// ─────────────────────────────────────────────

// DIDKeyFromPublicKey builds a did:key identifier
func DIDKeyFromPublicKey(pub *ecdsa.PublicKey) string {
	mb := PublicKeyToMultibase(pub)
	return "did:key:" + mb
}

// DIDWebFromDomain builds a did:web identifier
func DIDWebFromDomain(domain string) string {
	return "did:web:" + domain
}

// DIDEthrFromAddress builds a did:ethr identifier
func DIDEthrFromAddress(addr string) string {
	return "did:ethr:" + addr
}

// DIDEBSIFromPublicKey builds a did:ebsi identifier
func DIDEBSIFromPublicKey(pub *ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	h := sha256.Sum256(b)
	enc := base58Encode(h[:16])
	return "did:ebsi:z" + enc
}

// ─────────────────────────────────────────────
// Verifiable Credential JWT-like signing
// ─────────────────────────────────────────────

func SignVC(privateKey *ecdsa.PrivateKey, vcID string, subject map[string]interface{}) string {
	payload := fmt.Sprintf(`{"vcId":"%s","iat":%d}`, vcID, time.Now().Unix())
	return SimulatedSign(privateKey, []byte(payload))
}

// ─────────────────────────────────────────────
// Base encodings (stdlib only)
// ─────────────────────────────────────────────

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(input []byte) string {
	var result []byte
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(int64(len(base58Alphabet)))
	zero := big.NewInt(0)
	mod := new(big.Int)

	for x.Cmp(zero) != 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, base58Alphabet[0])
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

func base58Decode(input string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(int64(len(base58Alphabet)))

	for _, c := range input {
		idx := strings.IndexByte(base58Alphabet, byte(c))
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character: %c", c)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}

	decoded := result.Bytes()

	// Count leading '1' characters (which represent leading zero bytes)
	leadingZeros := 0
	for _, c := range input {
		if c == '1' {
			leadingZeros++
		} else {
			break
		}
	}

	return append(bytes.Repeat([]byte{0}, leadingZeros), decoded...), nil
}

const base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

func base32Encode(data []byte) string {
	var result strings.Builder
	buf := 0
	bufLen := 0
	for _, b := range data {
		buf = (buf << 8) | int(b)
		bufLen += 8
		for bufLen >= 5 {
			bufLen -= 5
			result.WriteByte(base32Alphabet[(buf>>bufLen)&0x1F])
		}
	}
	if bufLen > 0 {
		result.WriteByte(base32Alphabet[(buf<<(5-bufLen))&0x1F])
	}
	return result.String()
}
