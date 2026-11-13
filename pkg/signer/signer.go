// Package signer abstracts Stellar key custody behind signing-oracle
// interfaces. Call sites request signatures — an unsigned envelope or
// authorization entry goes in, the signed result comes out — without ever
// holding key material. An in-process KeypairSigner satisfies the interface
// today; a remote custodian can satisfy the same interface over a socket
// without any call-site changes (see docs/audit/mcp-signer-architecture.md).
package signer

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/stellar/go-stellar-sdk/hash"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ErrNoKeyMaterial is returned by signers that hold no key material (e.g.
// address-only preview signers).
var ErrNoKeyMaterial = errors.New("signer holds no key material")

// TxSigner signs Stellar transactions — the shape of a custodian's sign_tx
// call: unsigned envelope in, signed envelope out.
type TxSigner interface {
	// Address returns the signer's account address (G...).
	Address() string
	// Sign signs the transaction for the given network passphrase.
	Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error)
}

// AuthEntrySigner signs Soroban authorization entries — the shape of a
// custodian's sign_soroban_auth call.
type AuthEntrySigner interface {
	// Address returns the signer's account address (G...).
	Address() string
	// SignAuthEntry signs the authorization entry with an expiration ledger.
	SignAuthEntry(entry xdr.SorobanAuthorizationEntry, passphrase string, validUntilLedger uint32) (xdr.SorobanAuthorizationEntry, error)
}

// Signer combines transaction and Soroban-authorization signing.
type Signer interface {
	TxSigner
	AuthEntrySigner
}

// KeypairSigner adapts an in-process Stellar keypair to the signer
// interfaces. It satisfies the interface shape but not custody separation —
// keys still live in the caller's process.
type KeypairSigner struct {
	KP *keypair.Full
}

// Address returns the keypair's public address.
func (k KeypairSigner) Address() string { return k.KP.Address() }

// Sign signs the transaction with the keypair for the given passphrase.
func (k KeypairSigner) Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error) {
	return tx.Sign(passphrase, k.KP)
}

// SignAuthEntry signs the payer's Soroban authorization payload and embeds
// the standard Ed25519 account signature ScVal shape:
// vec[{public_key, signature}]. Both credential arms are supported:
// SOROBAN_CREDENTIALS_ADDRESS signs the envelope type 9 preimage
// (SorobanAuthorization); SOROBAN_CREDENTIALS_ADDRESS_V2 (CAP-71, protocol
// 27+) signs the envelope type 10 preimage (SorobanAuthorizationWithAddress),
// which additionally binds the credential's address.
func (k KeypairSigner) SignAuthEntry(
	entry xdr.SorobanAuthorizationEntry,
	passphrase string,
	validUntilLedger uint32,
) (xdr.SorobanAuthorizationEntry, error) {
	if k.KP == nil {
		return entry, fmt.Errorf("signer keypair is required")
	}
	credentials := AddressCredentials(&entry)
	if credentials == nil {
		return entry, fmt.Errorf("unsupported Soroban credentials type %s", entry.Credentials.Type.String())
	}
	address, err := credentials.Address.String()
	if err != nil {
		return entry, fmt.Errorf("invalid authorization address: %w", err)
	}
	if !strings.EqualFold(address, k.KP.Address()) {
		return entry, fmt.Errorf("authorization entry address %s does not match signer %s", address, k.KP.Address())
	}
	if validUntilLedger == 0 {
		return entry, fmt.Errorf("signature expiration ledger is required")
	}

	encoded, err := entry.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("marshal authorization entry: %w", err)
	}
	var clone xdr.SorobanAuthorizationEntry
	if err := clone.UnmarshalBinary(encoded); err != nil {
		return entry, fmt.Errorf("clone authorization entry: %w", err)
	}

	credentials = AddressCredentials(&clone)
	credentials.SignatureExpirationLedger = xdr.Uint32(validUntilLedger)
	if credentials.Nonce == 0 {
		nonce, err := randomNonce()
		if err != nil {
			return entry, err
		}
		credentials.Nonce = xdr.Int64(nonce)
	}

	var preimage xdr.HashIdPreimage
	if entry.Credentials.Type == xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2 {
		preimage = xdr.HashIdPreimage{
			Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorizationWithAddress,
			SorobanAuthorizationWithAddress: &xdr.HashIdPreimageSorobanAuthorizationWithAddress{
				NetworkId:                 network.ID(passphrase),
				Nonce:                     credentials.Nonce,
				SignatureExpirationLedger: credentials.SignatureExpirationLedger,
				Address:                   credentials.Address,
				Invocation:                clone.RootInvocation,
			},
		}
	} else {
		preimage = xdr.HashIdPreimage{
			Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorization,
			SorobanAuthorization: &xdr.HashIdPreimageSorobanAuthorization{
				NetworkId:                 network.ID(passphrase),
				Nonce:                     credentials.Nonce,
				SignatureExpirationLedger: credentials.SignatureExpirationLedger,
				Invocation:                clone.RootInvocation,
			},
		}
	}
	preimageBytes, err := preimage.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("marshal authorization preimage: %w", err)
	}
	payload := hash.Hash(preimageBytes)

	signature, err := k.KP.Sign(payload[:])
	if err != nil {
		return entry, fmt.Errorf("sign authorization payload: %w", err)
	}
	if err := k.KP.Verify(payload[:], signature); err != nil {
		return entry, fmt.Errorf("verify authorization signature: %w", err)
	}

	publicKeyBytes, err := strkey.Decode(strkey.VersionByteAccountID, k.KP.Address())
	if err != nil {
		return entry, fmt.Errorf("decode signer public key: %w", err)
	}
	publicKey := xdr.ScBytes(publicKeyBytes)
	signatureBytes := xdr.ScBytes(signature)
	publicKeySymbol := xdr.ScSymbol("public_key")
	signatureSymbol := xdr.ScSymbol("signature")

	signatureMap := xdr.ScMap{
		{
			Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &publicKeySymbol},
			Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &publicKey},
		},
		{
			Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &signatureSymbol},
			Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &signatureBytes},
		},
	}
	mapValue, err := xdr.NewScVal(xdr.ScValTypeScvMap, &signatureMap)
	if err != nil {
		return entry, fmt.Errorf("build signature map: %w", err)
	}
	signatureVec := xdr.ScVec{mapValue}
	credentials.Signature, err = xdr.NewScVal(xdr.ScValTypeScvVec, &signatureVec)
	if err != nil {
		return entry, fmt.Errorf("build signature value: %w", err)
	}
	return clone, nil
}

// AddressSigner is an address-only signer for preview and dry-run paths: it
// reports an address but refuses to sign. Previews built through it never
// touch key material.
type AddressSigner struct {
	Addr string
}

// Address returns the configured account address.
func (a AddressSigner) Address() string { return a.Addr }

// Sign always fails — an AddressSigner holds no key material.
func (a AddressSigner) Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error) {
	return nil, ErrNoKeyMaterial
}

// SignAuthEntry always fails — an AddressSigner holds no key material.
func (a AddressSigner) SignAuthEntry(
	entry xdr.SorobanAuthorizationEntry,
	passphrase string,
	validUntilLedger uint32,
) (xdr.SorobanAuthorizationEntry, error) {
	return entry, ErrNoKeyMaterial
}

// AddressCredentials returns the SorobanAddressCredentials for the supported
// address-credentialled arms (legacy Address and CAP-71 AddressV2), or nil for
// any other credentials type.
func AddressCredentials(entry *xdr.SorobanAuthorizationEntry) *xdr.SorobanAddressCredentials {
	switch entry.Credentials.Type {
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddress:
		return entry.Credentials.Address
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2:
		return entry.Credentials.AddressV2
	default:
		return nil
	}
}

func randomNonce() (int64, error) {
	max := new(big.Int).SetInt64(int64(^uint64(0) >> 1))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, fmt.Errorf("generate authorization nonce: %w", err)
	}
	return n.Int64(), nil
}
