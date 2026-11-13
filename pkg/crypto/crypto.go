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
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

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
	h := sha256.Sum256([]byte("stellar-go-cli-did-key-derivation:" + seed))
	d := new(big.Int).SetBytes(h[:])
	curve := elliptic.P256()
	d.Mod(d, curve.Params().N)
	if d.Sign() == 0 {
		h[0] = 1
		d.SetBytes(h[:])
		d.Mod(d, curve.Params().N)
	}
	return ecdsa.ParseRawPrivateKey(curve, d.FillBytes(make([]byte, 32)))
}

// marshalPublicKey encodes a public key in uncompressed SEC 1 form
// (0x04 || X || Y). Callers pass internally generated keys, which are
// always valid.
func marshalPublicKey(pub *ecdsa.PublicKey) []byte {
	b, err := pub.Bytes()
	if err != nil {
		panic(fmt.Sprintf("invalid ECDSA public key: %v", err))
	}
	return b
}

// PublicKeyToHex encodes the public key as uncompressed hex
func PublicKeyToHex(pub *ecdsa.PublicKey) string {
	b := marshalPublicKey(pub)
	return hex.EncodeToString(b)
}

// PublicKeyToMultibase returns a multibase-encoded (base58btc) public key
func PublicKeyToMultibase(pub *ecdsa.PublicKey) string {
	b := marshalPublicKey(pub)
	// z prefix = base58btc multibase
	return "z" + base58Encode(b)
}

// AddressFromPublicKey derives an Ethereum-style address from a public key
func AddressFromPublicKey(pub *ecdsa.PublicKey) string {
	b := marshalPublicKey(pub)
	h := sha256.Sum256(b[1:]) // skip 0x04 prefix
	return "0x" + hex.EncodeToString(h[12:])
}

// StellarAddressFromPublicKey derives a Stellar-style address (G...)
func StellarAddressFromPublicKey(pub *ecdsa.PublicKey) string {
	b := marshalPublicKey(pub)
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

// ─────────────────────────────────────────────
// JsonWebSignature2020 (detached JWS, ES256)
// ─────────────────────────────────────────────

// jwsHeaderB64 is the base64url-encoded protected header used for VC proofs:
// ES256 over the unencoded detached payload (RFC 7797), matching the
// JsonWebSignature2020 linked-data proof format.
const jwsHeaderB64 = "eyJhbGciOiJFUzI1NiIsImI2NCI6ZmFsc2UsImNyaXQiOlsiYjY0Il19"

// jwsSigningInput builds the ES256 signing input
// b64(header) + "." + hex(sha256(c14n proofConfig)) + "." + hex(sha256(c14n doc))
// used by JsonWebSignature2020: the proof options hash precedes the document
// hash, both computed over URDNA2015-canonicalized input.
func jwsSigningInput(vcMap, proofConfig map[string]interface{}) (string, error) {
	configCanonical, err := CanonicalizeRDF(proofConfig)
	if err != nil {
		return "", fmt.Errorf("canonicalize proof config: %w", err)
	}
	vcCanonical, err := CanonicalizeRDF(vcMap)
	if err != nil {
		return "", fmt.Errorf("canonicalize VC: %w", err)
	}
	payload := hex.EncodeToString(HashData(configCanonical)) + "." + hex.EncodeToString(HashData(vcCanonical))
	return jwsHeaderB64 + "." + payload, nil
}

// SignES256VC signs a VC using the JsonWebSignature2020 approach and returns
// the detached compact JWS "b64(header)..b64(signature)" for the proof's
// jws property. The signature is over the document and proof-option hashes,
// so any tampering invalidates it.
func SignES256VC(priv *ecdsa.PrivateKey, vcMap, proofConfig map[string]interface{}) (string, error) {
	input, err := jwsSigningInput(vcMap, proofConfig)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return "", fmt.Errorf("ES256 sign: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return jwsHeaderB64 + ".." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyES256VC verifies a detached JsonWebSignature2020 proof produced by
// SignES256VC. Only the ES256 / b64=false detached profile is accepted.
func VerifyES256VC(pub *ecdsa.PublicKey, vcMap, proofConfig map[string]interface{}, jws string) (bool, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 || parts[1] != "" {
		return false, fmt.Errorf("expected detached compact JWS (header..signature)")
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false, fmt.Errorf("decode JWS header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		B64 *bool  `json:"b64"`
	}
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		return false, fmt.Errorf("parse JWS header: %w", err)
	}
	if hdr.Alg != "ES256" {
		return false, fmt.Errorf("unsupported JWS alg %q (expected ES256)", hdr.Alg)
	}
	if hdr.B64 == nil || *hdr.B64 {
		return false, fmt.Errorf("only detached unencoded payloads (b64=false) are supported")
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("decode JWS signature: %w", err)
	}
	if len(sigBytes) != 64 {
		return false, fmt.Errorf("ES256 signature must be 64 bytes, got %d", len(sigBytes))
	}
	input, err := jwsSigningInput(vcMap, proofConfig)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256([]byte(input))
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	return ecdsa.Verify(pub, digest[:], r, s), nil
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
	opts := jsonLdProcessorOpts()
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
	opts := jsonLdProcessorOpts()
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

// DIDEthrFromAddress builds a did:ethr identifier from an address. Note that
// address-form identifiers do not carry the public key and therefore cannot
// be resolved locally — prefer DIDEthrFromPublicKey.
func DIDEthrFromAddress(addr string) string {
	return "did:ethr:" + addr
}

// DIDEthrFromPublicKey builds a did:ethr identifier whose method-specific-id
// is the uncompressed SEC 1 public key (did:ethr:0x04<128 hex chars>).
// Public-key identifiers are valid per the did:ethr method spec and resolve
// without an EthrDIDRegistry lookup.
func DIDEthrFromPublicKey(pub *ecdsa.PublicKey) string {
	return "did:ethr:0x" + PublicKeyToHex(pub)
}

// DIDEBSIFromPublicKey builds a did:ebsi identifier that embeds the public
// key as base58btc multibase (did:ebsi:z<65-byte uncompressed SEC 1 point>),
// making the DID self-contained and locally resolvable.
func DIDEBSIFromPublicKey(pub *ecdsa.PublicKey) string {
	return "did:ebsi:" + PublicKeyToMultibase(pub)
}

// ─────────────────────────────────────────────
// ECDSA public key resolution
// ─────────────────────────────────────────────

// DecodeECDSAPublicKeyMultibase decodes a base58btc ("z") multibase carrying
// an uncompressed SEC 1 P-256 public key (0x04 || X || Y).
func DecodeECDSAPublicKeyMultibase(mb string) (*ecdsa.PublicKey, error) {
	if !strings.HasPrefix(mb, "z") {
		return nil, fmt.Errorf("expected base58btc multibase (z prefix)")
	}
	raw, err := base58Decode(mb[1:])
	if err != nil {
		return nil, fmt.Errorf("base58 decode: %w", err)
	}
	return decodeP256Point(raw)
}

// DecodeECDSAPublicKeyFromDIDKey resolves a did:key carrying an ECDSA P-256 key.
func DecodeECDSAPublicKeyFromDIDKey(did string) (*ecdsa.PublicKey, error) {
	msid, ok := strings.CutPrefix(did, "did:key:")
	if !ok {
		return nil, fmt.Errorf("not a did:key identifier: %q", did)
	}
	return DecodeECDSAPublicKeyMultibase(msid)
}

// DecodeECDSAPublicKeyFromDIDEBSI resolves a did:ebsi identifier produced by
// DIDEBSIFromPublicKey. Identifiers holding a key hash rather than the key
// itself cannot be resolved and are rejected.
func DecodeECDSAPublicKeyFromDIDEBSI(did string) (*ecdsa.PublicKey, error) {
	msid, ok := strings.CutPrefix(did, "did:ebsi:")
	if !ok {
		return nil, fmt.Errorf("not a did:ebsi identifier: %q", did)
	}
	return DecodeECDSAPublicKeyMultibase(msid)
}

// DecodeECDSAPublicKeyFromDIDEthr resolves a did:ethr whose
// method-specific-id is the uncompressed SEC 1 public key
// (did:ethr:0x04<128 hex chars>, optionally chain-prefixed). Address-form
// identifiers (did:ethr:0x<40 hex>) need an EthrDIDRegistry lookup, which
// this library does not perform, and are rejected.
func DecodeECDSAPublicKeyFromDIDEthr(did string) (*ecdsa.PublicKey, error) {
	msid, ok := strings.CutPrefix(did, "did:ethr:")
	if !ok {
		return nil, fmt.Errorf("not a did:ethr identifier: %q", did)
	}
	if i := strings.LastIndex(msid, ":"); i >= 0 {
		msid = msid[i+1:]
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(msid, "0x"))
	if err != nil {
		return nil, fmt.Errorf("did:ethr method-specific-id is not hex: %w", err)
	}
	if len(raw) == 20 {
		return nil, fmt.Errorf("address-form did:ethr requires EthrDIDRegistry resolution, which is unsupported")
	}
	return decodeP256Point(raw)
}

// decodeP256Point parses a 65-byte uncompressed SEC 1 P-256 public key.
func decodeP256Point(b []byte) (*ecdsa.PublicKey, error) {
	if len(b) != 65 || b[0] != 0x04 {
		return nil, fmt.Errorf("expected 65-byte uncompressed P-256 point (0x04 prefix), got %d bytes", len(b))
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), b)
	if err != nil {
		return nil, fmt.Errorf("invalid P-256 public key: %w", err)
	}
	return pub, nil
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

// Base58Decode decodes a base58 (Bitcoin alphabet) string.
func Base58Decode(input string) ([]byte, error) {
	return base58Decode(input)
}

func base58Decode(input string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(int64(len(base58Alphabet)))

	for i := 0; i < len(input); i++ {
		idx := strings.IndexByte(base58Alphabet, input[i])
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character: %c", input[i])
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
