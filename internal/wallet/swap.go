package wallet

import (
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

// NewSwapService builds a swap service for the network with the given
// transaction signer attached. A nil signer yields a quote-only service
// (no execution capability). A custodian-backed signer also routes
// submission through the daemon — swap.SubmitTransaction auto-detects the
// signer's Submit method.
func NewSwapService(net models.Network, sg signer.Signer) *swap.Service {
	if sg == nil {
		return swap.NewService(net)
	}
	return swap.NewService(net, swap.WithSigner(sg))
}
