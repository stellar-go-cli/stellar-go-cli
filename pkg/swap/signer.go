package swap

import (
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// Signer abstracts transaction signing so services can plug in their own key
// management (KMS, hardware signer, remote signer). Implementations return
// the source account address and sign transactions for a network passphrase.
type Signer interface {
	// Address returns the signer's account address (G...).
	Address() string
	// Sign signs the transaction for the given network passphrase.
	Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error)
}

// KeypairSigner adapts a Stellar keypair to the Signer interface.
type KeypairSigner struct {
	KP *keypair.Full
}

// Address returns the keypair's public address.
func (k KeypairSigner) Address() string { return k.KP.Address() }

// Sign signs the transaction with the keypair for the given passphrase.
func (k KeypairSigner) Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error) {
	return tx.Sign(passphrase, k.KP)
}
