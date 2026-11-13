package contracts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// Service manages SAC (Stellar Asset Contract) operations
type Service struct{}

func NewService() *Service {
	return &Service{}
}

// DeploySAC deploys a new SAC contract for a Stellar asset.
// The SAC is deployed for real through the Soroban RPC (HostFunction
// createContract with a FromAsset preimage) — the returned contract ID is the
// deterministic strkey address of the deployed contract. The initial supply is
// then issued on the classic side: the issuer pays itself, which puts supply
// in circulation on both rails because SAC balances share the asset's
// trustlines.
func (s *Service) DeploySAC(
	ctx context.Context,
	client *horizonclient.Client,
	issuerKey string,
	assetCode string,
	assetName string,
	totalSupply string,
	net models.Network,
) (*models.Asset, error) {
	// Parse the issuer keypair using ParseFull which expects a secret seed
	trimmedKey := strings.TrimSpace(issuerKey)
	fullKP, err := keypair.ParseFull(trimmedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse issuer key (must be secret seed starting with 'S', got %d chars): %w", len(trimmedKey), err)
	}

	// Create the Stellar asset
	asset := txnbuild.CreditAsset{
		Code:   assetCode,
		Issuer: fullKP.Address(),
	}

	// Deploy the Stellar Asset Contract via Soroban RPC
	rpcClient := soroban.NewClientForNetwork(string(net))
	defer rpcClient.Close() //nolint:errcheck // best-effort close
	contractID, deployTxHash, err := rpcClient.CreateContractForAsset(ctx, signer.KeypairSigner{KP: fullKP}, asset)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy SAC contract: %w", err)
	}

	// Get sequence number for the issuer account
	accountReq := horizonclient.AccountRequest{AccountID: fullKP.Address()}
	issuerAccount, err := client.AccountDetail(accountReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get issuer account: %w", err)
	}

	// Build transaction to issue the supply
	// For the issuer, we just need to make a payment to themselves with the new asset
	paymentOp := &txnbuild.Payment{
		Destination: fullKP.Address(),
		Amount:      totalSupply,
		Asset:       asset,
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &issuerAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			Operations: []txnbuild.Operation{
				paymentOp,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	tx, err = tx.Sign(soroban.NetworkPassphrase(string(net)), fullKP)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit the transaction
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}

	// Create asset record
	assetRecord := &models.Asset{
		ID:          assetCode,
		ContractID:  contractID,
		Type:        models.AssetFungible,
		Name:        assetName,
		Symbol:      assetCode,
		Decimals:    7, // Stellar default
		TotalSupply: totalSupply,
		Issuer:      fullKP.Address(),
		Network:     net,
		Standard:    "SEP-41",
		Metadata: map[string]interface{}{
			"wrapped_asset":  assetCode,
			"contract_type":  "SAC",
			"deploy_tx_hash": deployTxHash,
			"mint_tx_hash":   resp.Hash,
		},
		CreatedAt: time.Now().UTC(),
		TxHash:    deployTxHash,
	}

	return assetRecord, nil
}

// MintSAC mints new tokens using deployed SAC contract
func (s *Service) MintSAC(
	ctx context.Context,
	client *horizonclient.Client,
	contractID string,
	issuerKey string,
	amount string,
	to string,
) (string, error) {
	// Parse the issuer keypair
	fullKP, err := keypair.ParseFull(issuerKey)
	if err != nil {
		return "", fmt.Errorf("invalid issuer key: %w", err)
	}

	// Get sequence number
	accountReq := horizonclient.AccountRequest{AccountID: fullKP.Address()}
	issuerAccount, err := client.AccountDetail(accountReq)
	if err != nil {
		return "", fmt.Errorf("failed to get issuer account: %w", err)
	}

	// Build payment transaction (minting by issuing to an account)
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &issuerAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination:   to,
					Amount:        amount,
					Asset:         txnbuild.CreditAsset{Code: contractID, Issuer: fullKP.Address()},
					SourceAccount: fullKP.Address(),
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign the transaction
	tx, err = tx.Sign(fullKP.Seed())
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit the transaction
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// TransferSAC transfers tokens using SAC contract
func (s *Service) TransferSAC(
	ctx context.Context,
	client *horizonclient.Client,
	contractID string,
	fromKey string,
	to string,
	amount string,
) (string, error) {
	// Parse the sender keypair
	fullKP, err := keypair.ParseFull(fromKey)
	if err != nil {
		return "", fmt.Errorf("invalid sender key: %w", err)
	}

	// Get sequence number
	accountReq := horizonclient.AccountRequest{AccountID: fullKP.Address()}
	senderAccount, err := client.AccountDetail(accountReq)
	if err != nil {
		return "", fmt.Errorf("failed to get sender account: %w", err)
	}

	// Build payment transaction
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &senderAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: to,
					Amount:      amount,
					Asset:       txnbuild.CreditAsset{Code: contractID, Issuer: fullKP.Address()},
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign the transaction
	tx, err = tx.Sign(fullKP.Seed())
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit the transaction
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}
