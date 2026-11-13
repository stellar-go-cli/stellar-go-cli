package swap

import (
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
)

// Signer abstracts transaction signing so services can plug in their own key
// management (KMS, hardware signer, remote signer). Implementations return
// the source account address and sign transactions for a network passphrase.
// It is an alias of signer.TxSigner — the shared signing-oracle interface
// (pkg/signer), so swap services can be backed by any key custody model.
type Signer = signer.TxSigner

// KeypairSigner adapts a Stellar keypair to the Signer interface.
type KeypairSigner = signer.KeypairSigner
