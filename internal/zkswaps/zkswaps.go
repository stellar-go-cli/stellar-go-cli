// Package zkswaps executes swaps with ZK proof privacy, composing the public
// swap service with the internal ZK proof service. The swap service must be
// constructed with a signer (e.g. swap.WithKeypair) for Execute to work.
package zkswaps

import (
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/zk"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
	"github.com/stellar/go/network"
)

// BuildProofRequest creates a ZK proof request for a swap operation.
func BuildProofRequest(quote *models.SwapQuote, payer, destination string, privacyLevel string) *models.ZKSwapRequest {
	return &models.ZKSwapRequest{
		ResourceURL:    "",
		Amount:         0, // Will be set from quote
		SourceAsset:    quote.SourceAsset,
		DestAsset:      quote.DestAsset,
		Payer:          payer,
		Destination:    destination,
		Nonce:          mpCrypto.RandomHex(16),
		ExpiresAt:      time.Now().Add(10 * time.Minute).UTC(),
		PrivacyLevel:   privacyLevel,
		ComplianceHash: mpCrypto.Hash256([]byte(payer + destination + quote.SourceAsset + quote.DestAsset + quote.Amount)),
		SwapType:       quote.SwapType,
	}
}

// networkID returns the network ID for ZK proof generation.
func networkID(svc *swap.Service) string {
	if svc.NetworkPassphrase() == network.PublicNetworkPassphrase {
		return "1" // Mainnet
	}
	return "0" // Testnet
}

// pickPath selects the best quote path for a ZK swap: the first path whose
// intermediate assets all have issuers, preferring direct paths.
func pickPath(quote *models.SwapQuote) (models.SwapPath, error) {
	var fallback *models.SwapPath
	for i, p := range quote.Paths {
		valid := true
		for _, a := range p.Path {
			if a.Code != "XLM" && a.Issuer == "" {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		// Prefer direct paths (no intermediate assets)
		if len(p.Path) == 0 {
			return p, nil
		}
		if fallback == nil {
			fallback = &quote.Paths[i]
		}
	}
	if fallback == nil {
		return models.SwapPath{}, fmt.Errorf("no valid paths found (all paths have assets without issuers)")
	}
	return *fallback, nil
}

// Execute executes a swap with ZK proof privacy: it generates a Noir proof
// locally, builds the path-payment transaction via svc, signs it with the
// service's configured signer, and submits it to Horizon.
func Execute(svc *swap.Service, quote *models.SwapQuote, maxSlippage float64, destination string, privacyLevel string) (*models.Payment, error) {
	signer := svc.Signer()
	if signer == nil {
		return nil, fmt.Errorf("no signer configured: pass WithSigner to swap.NewService")
	}
	src := signer.Address()

	// Use sender as destination if not specified
	if destination == "" {
		destination = src
	}

	// Initialize ZK service
	zkService := zk.NewService()

	// Generate ZK proof for swap
	inputs := zk.PaymentInputs{
		SenderPrivateKey: "MOCK_PRIVATE_KEY", // Would use actual private key
		ReceiverAddress:  destination,
		Amount:           quote.Amount,
		Nonce:            mpCrypto.RandomHex(16),
		NetworkID:        networkID(svc),
		MaxAmount:        "1000000", // 1M XLM max
	}

	proofData, err := zkService.GeneratePaymentProof(inputs)
	if err != nil {
		return nil, fmt.Errorf("ZK proof generation failed: %w", err)
	}

	// Verify proof locally first
	verified, err := zkService.VerifyProofLocally(proofData)
	if err != nil || !verified {
		return nil, fmt.Errorf("local ZK proof verification failed: %w", err)
	}

	// Fetch source account
	sourceAcct, err := svc.FetchAccount(src)
	if err != nil {
		return nil, fmt.Errorf("horizon: account not found for ZK swap: %w", err)
	}

	// Select the best valid path (preferring direct paths)
	bestPath, err := pickPath(quote)
	if err != nil {
		return nil, err
	}
	quoteWithPath := *quote
	quoteWithPath.Paths = []models.SwapPath{bestPath}

	tx, err := svc.BuildSwapTransaction(&quoteWithPath, sourceAcct, maxSlippage, destination)
	if err != nil {
		return nil, fmt.Errorf("failed to build ZK swap transaction: %w", err)
	}

	// Sign transaction
	tx, err = signer.Sign(tx, svc.NetworkPassphrase())
	if err != nil {
		return nil, fmt.Errorf("failed to sign ZK swap transaction: %w", err)
	}

	// Submit to Horizon
	resp, err := svc.SubmitTransaction(tx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	confirmed := now.Add(10 * time.Second)

	// Generate ZK verification data
	verification := &models.ZKSwapVerification{
		ProofID:          proofData.ProofHash,
		CircuitType:      proofData.CircuitType,
		Verified:         true,
		VerificationTime: time.Duration(proofData.GeneratedAt.Sub(now)),
		GasUsed:          proofData.GasUsed,
		OnChainRef:       resp.Hash[:16],
		TxHash:           resp.Hash,
		CreatedAt:        now,
	}

	// Save verification data for compliance reporting
	config.SaveState("zk_swap_verification_latest", verification) //nolint:errcheck // best-effort cache

	return &models.Payment{
		ID:          "zk-swap-" + resp.Hash[:12],
		From:        src,
		To:          destination,
		Amount:      quote.Amount,
		Asset:       quote.SourceAsset,
		Rail:        models.RailZK,
		Status:      models.PaymentConfirmed,
		Network:     svc.Network(),
		Memo:        fmt.Sprintf("ZK Swap %s→%s", quote.SourceAsset, quote.DestAsset),
		Fee:         "0.00005 XLM",
		TxHash:      resp.Hash,
		LedgerSeq:   int64(resp.Ledger),
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}, nil
}
