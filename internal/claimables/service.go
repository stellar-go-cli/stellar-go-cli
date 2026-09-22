package claimables

import (
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

// ClaimableBalance represents a claimable balance entry
type ClaimableBalance struct {
	ID            string    `json:"id"`
	Amount        string    `json:"amount"`
	AssetCode     string    `json:"asset_code"`
	AssetIssuer   string    `json:"asset_issuer"`
	Sponsor       string    `json:"sponsor"`
	CreatedAt     time.Time `json:"created_at"`
	Claimants     []string  `json:"claimants"`
	IsNativeAsset bool      `json:"is_native_asset"`
}

// Service handles claimable balance operations
type Service struct {
	client     *horizonclient.Client
	passphrase string
}

// NewService creates a new claimable balance service
func NewService(net models.Network) *Service {
	var client *horizonclient.Client
	var passphrase string

	switch net {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		passphrase = network.PublicNetworkPassphrase
	default:
		client = horizonclient.DefaultTestNetClient
		passphrase = network.TestNetworkPassphrase
	}

	return &Service{
		client:     client,
		passphrase: passphrase,
	}
}

// ListClaimableBalances fetches all claimable balances for an account
func (s *Service) ListClaimableBalances(address string) ([]ClaimableBalance, error) {
	request := horizonclient.ClaimableBalanceRequest{
		Claimant: address,
	}

	balances, err := s.client.ClaimableBalances(request)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch claimable balances: %w", err)
	}

	var result []ClaimableBalance
	for _, cb := range balances.Embedded.Records {
		claimants := make([]string, 0)
		for _, c := range cb.Claimants {
			claimants = append(claimants, c.Destination)
		}

		// Determine if native asset
		isNative := cb.Asset == "native" || cb.Asset == "XLM"

		// Parse asset code and issuer
		assetCode := "XLM"
		assetIssuer := ""
		if !isNative {
			// Asset format is "CODE:ISSUER"
			if len(cb.Asset) > 0 {
				// Try to parse asset string
				for i := 0; i < len(cb.Asset); i++ {
					if cb.Asset[i] == ':' {
						assetCode = cb.Asset[:i]
						if i+1 < len(cb.Asset) {
							assetIssuer = cb.Asset[i+1:]
						}
						break
					}
				}
				if assetIssuer == "" {
					assetCode = cb.Asset
				}
			}
		}

		createdAt := time.Now()
		if cb.LastModifiedTime != nil {
			createdAt = *cb.LastModifiedTime
		}

		result = append(result, ClaimableBalance{
			ID:            cb.BalanceID,
			Amount:        cb.Amount,
			AssetCode:     assetCode,
			AssetIssuer:   assetIssuer,
			Sponsor:       cb.Sponsor,
			CreatedAt:     createdAt,
			Claimants:     claimants,
			IsNativeAsset: isNative,
		})
	}

	return result, nil
}

// AcceptClaimableBalance claims a specific balance
func (s *Service) AcceptClaimableBalance(balanceID string) (string, error) {
	kp, err := s.loadStellarKeypair()
	if err != nil {
		return "", fmt.Errorf("no keypair found: %w", err)
	}

	// Fetch source account
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch account: %w", err)
	}

	// Build ClaimClaimableBalance operation
	claimOp := &txnbuild.ClaimClaimableBalance{
		BalanceID: balanceID,
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{claimOp},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit
	resp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// AcceptAllClaimableBalances claims all pending balances for the account
func (s *Service) AcceptAllClaimableBalances(address string) ([]string, []error) {
	balances, err := s.ListClaimableBalances(address)
	if err != nil {
		return nil, []error{err}
	}

	if len(balances) == 0 {
		return []string{}, nil
	}

	kp, err := s.loadStellarKeypair()
	if err != nil {
		return nil, []error{err}
	}

	// Fetch source account
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return nil, []error{err}
	}

	// Build batch transaction with all claims
	var operations []txnbuild.Operation
	for _, balance := range balances {
		claimOp := &txnbuild.ClaimClaimableBalance{
			BalanceID: balance.ID,
		}
		operations = append(operations, claimOp)
	}

	if len(operations) == 0 {
		return []string{}, nil
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           operations,
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to build batch transaction: %w", err)}
	}

	// Sign
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to sign transaction: %w", err)}
	}

	// Submit
	resp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to submit batch transaction: %w", err)}
	}

	// Return the single tx hash for all claims
	return []string{resp.Hash}, nil
}

// DeclineClaimableBalance claims the balance and returns it to the sponsor
func (s *Service) DeclineClaimableBalance(balanceID string, assetCode, assetIssuer, amount string) (string, error) {
	kp, err := s.loadStellarKeypair()
	if err != nil {
		return "", fmt.Errorf("no keypair found: %w", err)
	}

	// Fetch source account
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch account: %w", err)
	}

	// Get the balance details to find sponsor
	balances, err := s.ListClaimableBalances(kp.Address())
	if err != nil {
		return "", fmt.Errorf("failed to fetch balance details: %w", err)
	}

	var sponsor string
	for _, b := range balances {
		if b.ID == balanceID {
			sponsor = b.Sponsor
			break
		}
	}

	if sponsor == "" {
		return "", fmt.Errorf("claimable balance not found: %s", balanceID)
	}

	// Build operations: claim then pay back to sponsor
	claimOp := &txnbuild.ClaimClaimableBalance{
		BalanceID: balanceID,
	}

	var paymentOp txnbuild.Operation
	if assetCode == "XLM" || assetIssuer == "" {
		paymentOp = &txnbuild.Payment{
			Destination: sponsor,
			Amount:      amount,
			Asset:       txnbuild.NativeAsset{},
		}
	} else {
		paymentOp = &txnbuild.Payment{
			Destination: sponsor,
			Amount:      amount,
			Asset:       txnbuild.CreditAsset{Code: assetCode, Issuer: assetIssuer},
		}
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{claimOp, paymentOp},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit
	resp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return resp.Hash, nil
}

// DeclineAllClaimableBalances declines all pending claimable balances for an account
// Returns successfully declined balances, skipped balances (with reasons), and any errors
func (s *Service) DeclineAllClaimableBalances(address string) ([]string, []ClaimableBalance, []string, []error) {
	balances, err := s.ListClaimableBalances(address)
	if err != nil {
		return nil, nil, nil, []error{err}
	}

	if len(balances) == 0 {
		return []string{}, nil, nil, nil
	}

	kp, err := s.loadStellarKeypair()
	if err != nil {
		return nil, nil, nil, []error{err}
	}

	// Fetch source account to check existing trustlines
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return nil, nil, nil, []error{err}
	}

	// Build map of existing trustlines
	trustlines := make(map[string]bool)
	trustlines["XLM"] = true // Native asset always trusted
	for _, balance := range sourceAcct.Balances {
		if balance.Type == "native" {
			continue
		}
		key := balance.Code + ":" + balance.Issuer
		trustlines[key] = true
	}

	// Filter balances: separate claimable vs missing trustlines
	var claimableBalances []ClaimableBalance
	var skippedBalances []ClaimableBalance
	var skipReasons []string

	for _, b := range balances {
		var key string
		if b.IsNativeAsset {
			key = "XLM"
		} else {
			key = b.AssetCode + ":" + b.AssetIssuer
		}

		if trustlines[key] {
			claimableBalances = append(claimableBalances, b)
		} else {
			skippedBalances = append(skippedBalances, b)
			skipReasons = append(skipReasons, fmt.Sprintf("%s: missing trustline", b.AssetCode))
		}
	}

	if len(claimableBalances) == 0 {
		return nil, skippedBalances, skipReasons, nil
	}

	// Build operations for claimable balances: claim + payment for each
	var operations []txnbuild.Operation
	for _, balance := range claimableBalances {
		// Claim operation
		claimOp := &txnbuild.ClaimClaimableBalance{
			BalanceID: balance.ID,
		}
		operations = append(operations, claimOp)

		// Payment back to sponsor
		var paymentOp txnbuild.Operation
		if balance.AssetCode == "XLM" || balance.AssetIssuer == "" {
			paymentOp = &txnbuild.Payment{
				Destination: balance.Sponsor,
				Amount:      balance.Amount,
				Asset:       txnbuild.NativeAsset{},
			}
		} else {
			paymentOp = &txnbuild.Payment{
				Destination: balance.Sponsor,
				Amount:      balance.Amount,
				Asset:       txnbuild.CreditAsset{Code: balance.AssetCode, Issuer: balance.AssetIssuer},
			}
		}
		operations = append(operations, paymentOp)
	}

	if len(operations) == 0 {
		return []string{}, skippedBalances, skipReasons, nil
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
		return nil, skippedBalances, skipReasons, []error{fmt.Errorf("failed to build batch transaction: %w", err)}
	}

	// Sign
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return nil, skippedBalances, skipReasons, []error{fmt.Errorf("failed to sign transaction: %w", err)}
	}

	// Submit
	resp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return nil, skippedBalances, skipReasons, []error{fmt.Errorf("failed to submit batch transaction: %w", err)}
	}

	return []string{resp.Hash}, skippedBalances, skipReasons, nil
}

// loadStellarKeypair loads the keypair from wallet
func (s *Service) loadStellarKeypair() (*keypair.Full, error) {
	return wallet.LoadStellarKeypair()
}
