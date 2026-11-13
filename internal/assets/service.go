package assets

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/contracts"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// Service manages SAC/SEP-41 asset lifecycle
type Service struct {
	// ResolveSigner resolves signing authority for an address — nil uses
	// the local plaintext loader (trusted-surface call sites only).
	ResolveSigner func(address string) (signer.TxSigner, error)
}

func NewService() *Service { return &Service{} }

// signerFor resolves the signer for an address via ResolveSigner, falling
// back to the local plaintext loader.
func (s *Service) signerFor(address string) (signer.TxSigner, error) {
	if s.ResolveSigner != nil {
		return s.ResolveSigner(address)
	}
	sg, err := wallet.LoadStellarSigner()
	if err != nil {
		return nil, err
	}
	return sg, nil
}

// CreateFungibleAsset creates a SEP-41 fungible token using SAC contract
func (s *Service) CreateFungibleAsset(
	name, symbol string,
	decimals int,
	totalSupply string,
	issuer string,
	issuerKey string,
	network models.Network,
	metadata map[string]interface{},
) (*models.Asset, error) {
	if name == "" || symbol == "" {
		return nil, fmt.Errorf("name and symbol are required")
	}
	if len(symbol) > 12 {
		return nil, fmt.Errorf("symbol must be ≤12 characters (Stellar limit)")
	}

	// Use symbol as asset code (already validated to be ≤12 chars)
	assetCode := symbol

	// Connect to Horizon for the target network
	client := horizonclient.DefaultTestNetClient
	if network == models.NetworkStellarMainnet {
		client = horizonclient.DefaultPublicNetClient
	}

	// Create SAC service for contract deployment
	sacService := contracts.NewService()

	// Deploy SAC contract for the asset
	ctx := context.Background()
	ui.PrintStep(1, "Deploying SAC contract to Stellar")
	spin := ui.NewSpinner(fmt.Sprintf("Deploying %s (%s) contract...", name, symbol))
	spin.Start()

	asset, err := sacService.DeploySAC(ctx, client, issuerKey, assetCode, name, totalSupply, network)
	if err != nil {
		spin.Stop(false, err.Error())
		return nil, err
	}
	spin.Stop(true, "Contract deployed")

	// Update asset with additional metadata, keeping the deploy/mint tx hashes
	// DeploySAC recorded
	asset.ID = mpCrypto.RandomHex(16)
	asset.Type = models.AssetFungible
	asset.Name = name
	asset.Symbol = symbol
	asset.Decimals = decimals
	asset.Issuer = issuer
	asset.Network = network
	asset.Standard = "SEP-41"
	for k, v := range asset.Metadata {
		if _, ok := metadata[k]; !ok {
			metadata[k] = v
		}
	}
	asset.Metadata = metadata
	asset.CreatedAt = time.Now().UTC()

	return asset, nil
}

// CreateNonFungibleAsset creates a SEP-41 non-fungible asset
func (s *Service) CreateNonFungibleAsset(
	name string,
	issuer string,
	network models.Network,
	metadata map[string]interface{},
) (*models.Asset, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for NFA")
	}

	// NFAs have totalSupply=1, decimals=0
	asset := &models.Asset{
		ID:          mpCrypto.RandomHex(16),
		ContractID:  mpCrypto.ContractID(),
		Type:        models.AssetNonFungible,
		Name:        name,
		Symbol:      "NFA",
		Decimals:    0,
		TotalSupply: "1",
		Issuer:      issuer,
		Network:     network,
		Standard:    "SEP-41",
		Metadata:    metadata,
		CreatedAt:   time.Now().UTC(),
		TxHash:      mpCrypto.StellarTxHash(),
	}

	return asset, nil
}

// AttachCarbonCredit attaches a StellarCarbon offset to an asset
func (s *Service) AttachCarbonCredit(asset *models.Asset, amount float64, vintage int, standard string) (*models.Asset, error) {
	credit := &models.CarbonCredit{
		Provider: "StellarCarbon",
		TokenID:  "CARBON-" + mpCrypto.RandomHex(8),
		Amount:   amount,
		Vintage:  vintage,
		Standard: standard,
		Retired:  false,
		TxHash:   mpCrypto.StellarTxHash(),
	}
	updated := *asset
	updated.CarbonOffset = credit
	return &updated, nil
}

// TransferAsset simulates SEP-41 transfer
func (s *Service) TransferAsset(asset *models.Asset, from, to, amount string) (string, error) {
	txHash := mpCrypto.StellarTxHash()
	return txHash, nil
}

// ─────────────────────────────────────────────
// CreateTrustline creates a trustline for an asset using ChangeTrust operation
func (s *Service) CreateTrustline(accountAddress, code, issuer, limit string) (string, error) {
	// Load keypair from wallet
	sg, err := s.signerFor(accountAddress)
	if err != nil {
		return "", fmt.Errorf("no signer found: %w", err)
	}

	// Verify the account address matches the keypair
	if sg.Address() != accountAddress {
		return "", fmt.Errorf("keypair address %s does not match provided address %s", sg.Address(), accountAddress)
	}

	// Get active wallet to determine network
	walletSvc := wallet.NewService()
	activeWallet, err := walletSvc.GetActiveWallet()
	if err != nil {
		return "", fmt.Errorf("failed to get active wallet: %w", err)
	}

	// Connect to appropriate Horizon
	var client *horizonclient.Client
	var networkPassphrase string
	switch activeWallet.Network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		networkPassphrase = network.PublicNetworkPassphrase
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
		networkPassphrase = network.TestNetworkPassphrase
	default:
		return "", fmt.Errorf("unsupported network: %s", activeWallet.Network)
	}

	// Fetch source account
	sourceAcct, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: sg.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch account: %w", err)
	}

	// Build ChangeTrust operation
	changeTrustOp := &txnbuild.ChangeTrust{
		Line: txnbuild.ChangeTrustAssetWrapper{
			Asset: txnbuild.CreditAsset{Code: code, Issuer: issuer},
		},
		Limit: "922337203685.4775807", // Max int64
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{changeTrustOp},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign
	tx, err = sg.Sign(tx, networkPassphrase)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// AssetIdentifier represents an asset to remove
type AssetIdentifier struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
}

// RemoveTrustline removes a single trustline by setting limit to 0
func (s *Service) RemoveTrustline(accountAddress, code, issuer string) (string, error) {
	// Load keypair from wallet
	sg, err := s.signerFor(accountAddress)
	if err != nil {
		return "", fmt.Errorf("no signer found: %w", err)
	}

	// Verify the account address matches the keypair
	if sg.Address() != accountAddress {
		return "", fmt.Errorf("keypair address %s does not match provided address %s", sg.Address(), accountAddress)
	}

	// Get active wallet to determine network
	walletSvc := wallet.NewService()
	activeWallet, err := walletSvc.GetActiveWallet()
	if err != nil {
		return "", fmt.Errorf("failed to get active wallet: %w", err)
	}

	// Connect to appropriate Horizon
	var client *horizonclient.Client
	var networkPassphrase string
	switch activeWallet.Network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		networkPassphrase = network.PublicNetworkPassphrase
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
		networkPassphrase = network.TestNetworkPassphrase
	default:
		return "", fmt.Errorf("unsupported network: %s", activeWallet.Network)
	}

	// Fetch source account
	sourceAcct, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: sg.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch account: %w", err)
	}

	// Build ChangeTrust operation with limit 0 to remove trustline
	changeTrustOp := &txnbuild.ChangeTrust{
		Line: txnbuild.ChangeTrustAssetWrapper{
			Asset: txnbuild.CreditAsset{Code: code, Issuer: issuer},
		},
		Limit: "0",
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{changeTrustOp},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign
	tx, err = sg.Sign(tx, networkPassphrase)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// RemoveTrustlinesBatch removes multiple trustlines in a single transaction
func (s *Service) RemoveTrustlinesBatch(accountAddress string, assets []AssetIdentifier) (string, error) {
	if len(assets) == 0 {
		return "", fmt.Errorf("no assets provided for batch removal")
	}

	// Load keypair from wallet
	sg, err := s.signerFor(accountAddress)
	if err != nil {
		return "", fmt.Errorf("no signer found: %w", err)
	}

	// Verify the account address matches the keypair
	if sg.Address() != accountAddress {
		return "", fmt.Errorf("keypair address %s does not match provided address %s", sg.Address(), accountAddress)
	}

	// Get active wallet to determine network
	walletSvc := wallet.NewService()
	activeWallet, err := walletSvc.GetActiveWallet()
	if err != nil {
		return "", fmt.Errorf("failed to get active wallet: %w", err)
	}

	// Connect to appropriate Horizon
	var client *horizonclient.Client
	var networkPassphrase string
	switch activeWallet.Network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		networkPassphrase = network.PublicNetworkPassphrase
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
		networkPassphrase = network.TestNetworkPassphrase
	default:
		return "", fmt.Errorf("unsupported network: %s", activeWallet.Network)
	}

	// Fetch source account
	sourceAcct, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: sg.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch account: %w", err)
	}

	// Build ChangeTrust operations for each asset (limit 0 = removal)
	var operations []txnbuild.Operation
	for _, asset := range assets {
		if asset.Code == "" || asset.Issuer == "" {
			continue // Skip invalid entries
		}
		changeTrustOp := &txnbuild.ChangeTrust{
			Line: txnbuild.ChangeTrustAssetWrapper{
				Asset: txnbuild.CreditAsset{Code: asset.Code, Issuer: asset.Issuer},
			},
			Limit: "0",
		}
		operations = append(operations, changeTrustOp)
	}

	if len(operations) == 0 {
		return "", fmt.Errorf("no valid assets to remove")
	}

	// Build transaction with all operations
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           operations,
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign
	tx, err = sg.Sign(tx, networkPassphrase)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// Helpers
// ─────────────────────────────────────────────

// SupportedStandards lists carbon credit standards
func SupportedStandards() []string {
	return []string{"VCS", "Gold Standard", "CAR", "ACR", "Plan Vivo"}
}
