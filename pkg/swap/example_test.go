package swap_test

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
	"github.com/stellar/go-stellar-sdk/keypair"
)

func ExampleNewService() {
	svc := swap.NewService(models.NetworkStellarTestnet)
	fmt.Println(svc != nil)
	// Output: true
}

// A quote-only service needs no signer — useful for backend services that
// only quote prices (or that return unsigned XDR for wallets to sign).
func ExampleNewService_quoteOnly() {
	svc := swap.NewService(models.NetworkStellarTestnet)
	fmt.Println(svc.Signer() == nil)
	// Output: true
}

// Attach any signer implementation — a plain keypair, or a KMS-backed
// Signer for custodial services.
func ExampleNewService_withKeypair() {
	kp := keypair.MustRandom()
	svc := swap.NewService(models.NetworkStellarTestnet, swap.WithKeypair(kp))
	fmt.Println(svc.Signer().Address() == kp.Address())
	// Output: true
}

// Arbitrary assets resolve as CODE:ISSUER — the built-in asset registry is
// only a convenience for well-known codes.
//
// No // Output: assertion — this performs a live testnet path query whose
// result depends on order-book liquidity.
func ExampleNewService_anyAsset() {
	svc := swap.NewService(models.NetworkStellarTestnet)
	quote, err := svc.GetQuote(models.SwapRequest{
		SourceAsset: "XLM",
		DestAsset:   "USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Amount:      "10",
		SwapType:    models.SwapStrictSend,
	})
	fmt.Println(quote != nil, err)
}
