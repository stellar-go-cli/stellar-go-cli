package wallet

import (
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
	"github.com/stellar/go-stellar-sdk/keypair"
)

// NewSwapService builds a swap service for the network, attaching the
// configured Stellar wallet as the transaction signer when available.
// The returned keypair is nil when no swap-compatible wallet is configured —
// the service still works for quotes and unsigned transaction building.
func NewSwapService(net models.Network) (*swap.Service, *keypair.Full) {
	kp, err := LoadStellarKeypairForSwap()
	if err != nil {
		return swap.NewService(net), nil
	}
	return swap.NewService(net, swap.WithKeypair(kp)), kp
}
