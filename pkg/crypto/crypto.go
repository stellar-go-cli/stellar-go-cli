package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// GenerateKeyPair generates an ECDSA P-256 key pair
func GenerateKeyPair() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
