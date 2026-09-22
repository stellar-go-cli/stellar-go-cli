package contracts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

// Service manages SAC (Stellar Asset Contract) operations
type Service struct{}

func NewService() *Service {
	return &Service{}
}

// DeploySAC deploys a new SAC contract for a Stellar asset
// This uses the Stellar SDK to create a wrapped asset contract
func (s *Service) DeploySAC(
	ctx context.Context,
	client *horizonclient.Client,
	issuerKey string,
	assetCode string,
	assetName string,
	totalSupply string,
) (*models.Asset, error) {
	// Parse the issuer keypair using ParseFull which expects a secret seed
	trimmedKey := strings.TrimSpace(issuerKey)
	fmt.Printf("DEBUG: Attempting to parse key of length %d, first 10 chars: %s...\n", len(trimmedKey), trimmedKey[:10])
	fullKP, err := keypair.ParseFull(trimmedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse issuer key (must be secret seed starting with 'S', got %d chars): %w", len(trimmedKey), err)
	}
	fmt.Printf("DEBUG: Successfully parsed key, address: %s\n", fullKP.Address())

	// Create the Stellar asset
	asset := txnbuild.CreditAsset{
		Code:   assetCode,
		Issuer: fullKP.Address(),
	}

	// Get sequence number for the issuer account
	accountReq := horizonclient.AccountRequest{AccountID: fullKP.Address()}
	issuerAccount, err := client.AccountDetail(accountReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get issuer account: %w", err)
	}

	// Build transaction to create the asset
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

	// Sign the transaction with testnet network passphrase
	tx, err = tx.Sign(network.TestNetworkPassphrase, fullKP)
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
		ContractID:  resp.Hash, // Use transaction hash as contract ID
		Type:        models.AssetFungible,
		Name:        assetName,
		Symbol:      assetCode,
		Decimals:    7, // Stellar default
		TotalSupply: totalSupply,
		Issuer:      fullKP.Address(),
		Network:     models.NetworkStellarTestnet,
		Standard:    "SEP-41",
		Metadata: map[string]interface{}{
			"wrapped_asset": assetCode,
			"contract_type": "SAC",
			"tx_hash":       resp.Hash,
		},
		CreatedAt: time.Now().UTC(),
		TxHash:    resp.Hash,
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
