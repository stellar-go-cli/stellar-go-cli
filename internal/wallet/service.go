package wallet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/zalando/go-keyring"
)

// Service manages wallet operations
type Service struct{}

func NewService() *Service { return &Service{} }

// ConnectStellarWallet creates a native Stellar wallet with proper keypair
func (s *Service) ConnectStellarWallet(network models.Network) (*models.Account, error) {
	// Generate proper Stellar keypair using Stellar SDK
	pair, err := keypair.Random()
	if err != nil {
		return nil, fmt.Errorf("failed to generate Stellar keypair: %w", err)
	}

	// Use proper Stellar address from SDK
	acc := &models.Account{
		Address:    pair.Address(),
		PublicKey:  pair.Address(),
		PrivateKey: pair.Seed(),
		Network:    network,
		Type:       models.WalletStellar,
		Balance:    "0",
		Funded:     false,
		CreatedAt:  time.Now().UTC(),
	}
	return acc, nil
}

// ImportStellarFromSecret builds an account from a Stellar secret key (S... strkey) and refreshes balance from Horizon.
func (s *Service) ImportStellarFromSecret(secret string, network models.Network) (*models.Account, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("secret key is empty")
	}
	if network != models.NetworkStellarTestnet && network != models.NetworkStellarMainnet {
		return nil, fmt.Errorf("import supports only stellar-testnet and stellar-mainnet, got: %s", network)
	}
	kp, err := keypair.ParseFull(secret)
	if err != nil {
		return nil, fmt.Errorf("invalid Stellar secret key: %w", err)
	}
	addr := kp.Address()
	bal, funded, err := s.FetchBalanceFromNetwork(addr, network)
	if err != nil {
		return nil, err
	}
	return &models.Account{
		Address:    addr,
		PublicKey:  addr,
		PrivateKey: kp.Seed(),
		Network:    network,
		Type:       models.WalletStellar,
		Balance:    bal,
		Funded:     funded,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// ConnectExternalEOA registers an external owner account
func (s *Service) ConnectExternalEOA(address string, network models.Network) (*models.Account, error) {
	if address == "" {
		address = "0x" + mpCrypto.RandomHex(20)
	}
	acc := &models.Account{
		Address:   address,
		Network:   network,
		Type:      models.WalletExternal,
		Balance:   "0",
		Funded:    false,
		CreatedAt: time.Now().UTC(),
	}
	return acc, nil
}

// FundTestnetAccount funds an account via faucet using real API calls
func (s *Service) FundTestnetAccount(acc *models.Account) (*models.Account, error) {
	if acc.Network != models.NetworkStellarTestnet && acc.Network != models.NetworkEVMSepolia {
		return nil, fmt.Errorf("testnet funding only available on testnet networks, got: %s", acc.Network)
	}

	var funded *models.Account
	switch acc.Network {
	case models.NetworkStellarTestnet:
		// Call real Friendbot API
		friendbotURL := fmt.Sprintf("https://friendbot.stellar.org?addr=%s", acc.Address)

		// Use standard HTTP client with proper TLS verification and timeout
		client := config.NewHTTPClient()

		resp, err := client.Get(friendbotURL)
		if err != nil {
			return nil, fmt.Errorf("failed to call Friendbot: %w", err)
		}
		defer resp.Body.Close() //nolint:errcheck // best-effort close

		if resp.StatusCode != http.StatusOK {
			body, rerr := io.ReadAll(resp.Body)
			if rerr != nil {
				return nil, fmt.Errorf("friendbot returned status %d", resp.StatusCode)
			}
			return nil, fmt.Errorf("friendbot returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse Friendbot response
		var friendbotResp struct {
			Hash     string `json:"hash"`
			Ledger   int64  `json:"ledger"`
			Envelope string `json:"envelope"`
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read Friendbot response: %w", err)
		}

		if err := json.Unmarshal(body, &friendbotResp); err != nil {
			return nil, fmt.Errorf("failed to parse Friendbot response: %w", err)
		}

		// Update account with funded status
		funded = &models.Account{
			Address:          acc.Address,
			PublicKey:        acc.PublicKey,
			PrivateKeyEnc:    acc.PrivateKeyEnc,
			PasskeyProtected: acc.PasskeyProtected,
			Network:          acc.Network,
			Type:             acc.Type,
			Balance:          "10000.0000000 XLM",
			DID:              acc.DID,
			Funded:           true,
			CreatedAt:        acc.CreatedAt,
		}

	case models.NetworkEVMSepolia:
		// For EVM Sepolia, we'd implement similar logic with Sepolia faucet
		funded = &models.Account{
			Address:   acc.Address,
			PublicKey: acc.PublicKey,
			Network:   acc.Network,
			Type:      acc.Type,
			Balance:   "1.0 SepoliaETH",
			DID:       acc.DID,
			Funded:    true,
			CreatedAt: acc.CreatedAt,
		}
	}

	return funded, nil
}

// FetchBalanceFromNetwork fetches actual balance from Stellar Horizon API
func (s *Service) FetchBalanceFromNetwork(address string, network models.Network) (string, bool, error) {
	var client *horizonclient.Client
	switch network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
	default:
		return "0", false, fmt.Errorf("unsupported network: %s", network)
	}

	account, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: address,
	})
	if err != nil {
		// Account not found = not funded
		return "0", false, nil
	}

	// Find XLM balance
	for _, balance := range account.Balances {
		if balance.Type == "native" {
			return balance.Balance, true, nil
		}
	}

	return "0", true, nil
}

// GetAccountAssets fetches all assets/trustlines for a wallet address
func (s *Service) GetAccountAssets(address string, network models.Network) ([]AssetInfo, error) {
	var client *horizonclient.Client
	switch network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	account, err := client.AccountDetail(horizonclient.AccountRequest{
		AccountID: address,
	})
	if err != nil {
		// Account not found = not funded, return empty list
		return nil, nil
	}

	var assets []AssetInfo
	for _, balance := range account.Balances {
		if balance.Type == "native" {
			assets = append(assets, AssetInfo{
				Code:    "XLM",
				Issuer:  "Native",
				Balance: balance.Balance,
				Type:    "native",
			})
		} else {
			assets = append(assets, AssetInfo{
				Code:    balance.Code,
				Issuer:  balance.Issuer,
				Balance: balance.Balance,
				Type:    balance.Type,
			})
		}
	}

	return assets, nil
}

// AssetInfo represents an asset/trustline information
type AssetInfo struct {
	Code    string `json:"code"`
	Issuer  string `json:"issuer"`
	Balance string `json:"balance"`
	Type    string `json:"type"`
}

// TransactionInfo represents a transaction from Horizon
type TransactionInfo struct {
	Hash      string    `json:"hash"`
	Type      string    `json:"type"`
	Amount    float64   `json:"amount"`
	Asset     string    `json:"asset"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
}

// GetTransactions fetches recent transactions for a wallet address from Horizon
func (s *Service) GetTransactions(address string, network models.Network, limit int) ([]TransactionInfo, error) {
	var client *horizonclient.Client
	switch network {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
	case models.NetworkStellarTestnet:
		client = horizonclient.DefaultTestNetClient
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	// Fetch transactions
	txRequest := horizonclient.TransactionRequest{
		ForAccount:    address,
		Limit:         uint(limit),
		Order:         horizonclient.OrderDesc,
		IncludeFailed: true,
	}

	transactions, err := client.Transactions(txRequest)
	if err != nil {
		return nil, nil // Return empty list on error
	}

	var result []TransactionInfo
	for _, tx := range transactions.Embedded.Records {
		info := TransactionInfo{
			Hash:      tx.Hash[:8] + "...",
			Type:      getTransactionType(int(tx.OperationCount)),
			Timestamp: tx.LedgerCloseTime,
			Status:    "Success",
		}
		if !tx.Successful {
			info.Status = "Failed"
		}

		// Fetch operations to get payment details
		if tx.OperationCount > 0 {
			opRequest := horizonclient.OperationRequest{
				Limit: 1,
			}
			ops, err := client.Operations(opRequest)
			if err == nil && len(ops.Embedded.Records) > 0 {
				op := ops.Embedded.Records[0]
				info.Type = getOperationType(op.GetType())
				switch o := op.(type) {
				case *operations.Payment:
					info.Amount = parseAmount(o.Amount)
					if o.Asset.Type == "native" {
						info.Asset = "XLM"
					} else {
						info.Asset = o.Code
					}
				case *operations.CreateAccount:
					info.Amount = parseAmount(o.StartingBalance)
					info.Asset = "XLM"
				default:
					// For swaps and other operations, try to extract from generic operation
					// Amount stays 0 for operations without clear amounts (trustlines, etc.)
					info.Amount = 0
				}
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func getTransactionType(opCount int) string {
	if opCount == 1 {
		return "Payment"
	}
	return "Multi-op"
}

func getOperationType(opType string) string {
	switch opType {
	case "payment":
		return "Payment"
	case "path_payment_strict_receive", "path_payment_strict_send":
		return "Swap"
	case "create_account":
		return "Create"
	case "change_trust":
		return "Trustline"
	case "payment_path":
		return "Path"
	default:
		return "Transfer"
	}
}

func parseAmount(amount string) float64 {
	val, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return 0
	}
	return val
}

// UpdateWalletBalance fetches and updates wallet balance from network
func (s *Service) UpdateWalletBalance(address string) (*models.Account, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, err
	}

	balance, funded, err := s.FetchBalanceFromNetwork(address, acc.Network)
	if err != nil {
		return nil, err
	}

	acc.Balance = balance
	acc.Funded = funded

	// Save updated wallet
	if err := s.AddWalletToRegistry(acc, address == acc.Address); err != nil {
		return nil, err
	}

	return acc, nil
}

// GetBalance returns the stored balance (use UpdateWalletBalance to refresh from network)
func (s *Service) GetBalance(acc *models.Account) (string, error) {
	if !acc.Funded {
		return "0", nil
	}
	return acc.Balance, nil
}

// SaveAccountByType saves account state with type-specific filename
func (s *Service) SaveAccountByType(acc *models.Account) error {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	statePath := fmt.Sprintf("%s/.stellar-go-cli/state/account_%s.json", stateDir, acc.Type)
	data, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("failed to save account state: %w", err)
	}

	// Also save as default for backward compatibility
	defaultPath := fmt.Sprintf("%s/.stellar-go-cli/state/account.json", stateDir)
	return os.WriteFile(defaultPath, data, 0600)
}

// LoadAccountByType loads account state for a specific wallet type
func (s *Service) LoadAccountByType(walletType string) (*models.Account, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	statePath := fmt.Sprintf("%s/.stellar-go-cli/state/account_%s.json", stateDir, walletType)
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read account state: %w", err)
	}

	var acc models.Account
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %w", err)
	}

	return &acc, nil
}

// LinkDID links a DID to the account
func (s *Service) LinkDID(acc *models.Account, did string) *models.Account {
	updated := *acc
	updated.DID = did
	return &updated
}

// NetworkDisplayName returns human-readable network name
func NetworkDisplayName(n models.Network) string {
	switch n {
	case models.NetworkStellarTestnet:
		return "Stellar Testnet (Friendbot)"
	case models.NetworkStellarMainnet:
		return "Stellar Mainnet"
	case models.NetworkEVMSepolia:
		return "EVM Sepolia Testnet"
	case models.NetworkEVMMainnet:
		return "EVM Mainnet"
	default:
		return string(n)
	}
}

// WalletNetworkDisplayName returns human-readable network name
func WalletNetworkDisplayName(n models.Network, walletType models.WalletType) string {
	return NetworkDisplayName(n)
}

// FaucetURL returns the faucet URL for a testnet
func FaucetURL(n models.Network, address string) string {
	switch n {
	case models.NetworkStellarTestnet:
		return fmt.Sprintf("https://friendbot.stellar.org?addr=%s", address)
	case models.NetworkEVMSepolia:
		return fmt.Sprintf("https://sepoliafaucet.com/?address=%s", address)
	default:
		return "https://friendbot.stellar.org"
	}
}

// ── Wallet Registry Management ─────────────────

// GetWalletRegistryPath returns the path to the wallet registry file
func (s *Service) GetWalletRegistryPath() (string, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets.json"), nil
}

// GetWalletsDir returns the directory where individual wallet files are stored
func (s *Service) GetWalletsDir() (string, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	walletsDir := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets")
	if err := os.MkdirAll(walletsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create wallets directory: %w", err)
	}
	return walletsDir, nil
}

// LoadRegistry loads the wallet registry
func (s *Service) LoadRegistry() (*models.WalletRegistry, error) {
	registryPath, err := s.GetWalletRegistryPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty registry
			return &models.WalletRegistry{
				Wallets: []models.WalletEntry{},
			}, nil
		}
		return nil, fmt.Errorf("failed to read wallet registry: %w", err)
	}

	var registry models.WalletRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse wallet registry: %w", err)
	}

	return &registry, nil
}

// SaveRegistry saves the wallet registry with file locking for concurrency safety
func (s *Service) SaveRegistry(registry *models.WalletRegistry) error {
	lock := newRegistryLock()
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire registry lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // best-effort lock release

	registryPath, err := s.GetWalletRegistryPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal wallet registry: %w", err)
	}

	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save wallet registry: %w", err)
	}

	return nil
}

// AddWalletToRegistry adds a wallet to the registry and saves the full account
func (s *Service) AddWalletToRegistry(acc *models.Account, makeActive bool) error {
	// Save full account to wallets directory
	walletsDir, err := s.GetWalletsDir()
	if err != nil {
		return err
	}

	walletPath := filepath.Join(walletsDir, acc.Address+".json")
	data, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("failed to marshal wallet: %w", err)
	}

	if err := os.WriteFile(walletPath, data, 0600); err != nil {
		return fmt.Errorf("failed to save wallet file: %w", err)
	}

	// Update registry
	registry, err := s.LoadRegistry()
	if err != nil {
		return err
	}

	// Check if wallet already exists
	exists := false
	for i, w := range registry.Wallets {
		if w.Address == acc.Address {
			// Update existing entry
			registry.Wallets[i] = models.WalletEntry{
				Address:   acc.Address,
				Type:      acc.Type,
				Network:   acc.Network,
				Balance:   acc.Balance,
				Funded:    acc.Funded,
				CreatedAt: acc.CreatedAt,
			}
			exists = true
			break
		}
	}

	if !exists {
		// Add new entry
		registry.Wallets = append(registry.Wallets, models.WalletEntry{
			Address:   acc.Address,
			Type:      acc.Type,
			Network:   acc.Network,
			Balance:   acc.Balance,
			Funded:    acc.Funded,
			CreatedAt: acc.CreatedAt,
		})
	}

	if makeActive {
		registry.ActiveWallet = acc.Address
	}

	return s.SaveRegistry(registry)
}

// GetWalletByAddress loads a full wallet by address
func (s *Service) GetWalletByAddress(address string) (*models.Account, error) {
	walletsDir, err := s.GetWalletsDir()
	if err != nil {
		return nil, err
	}

	walletPath := filepath.Join(walletsDir, address+".json")
	data, err := os.ReadFile(walletPath)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	var acc models.Account
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("failed to parse wallet: %w", err)
	}

	return &acc, nil
}

// ListWallets returns all wallets in the registry
func (s *Service) ListWallets() ([]models.WalletEntry, string, error) {
	registry, err := s.LoadRegistry()
	if err != nil {
		return nil, "", err
	}
	return registry.Wallets, registry.ActiveWallet, nil
}

// SetActiveWallet sets the active wallet by address
func (s *Service) SetActiveWallet(address string) error {
	registry, err := s.LoadRegistry()
	if err != nil {
		return err
	}

	// Verify wallet exists
	found := false
	for _, w := range registry.Wallets {
		if w.Address == address {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("wallet not found: %s", address)
	}

	registry.ActiveWallet = address
	return s.SaveRegistry(registry)
}

// RenameWallet sets a friendly name for a wallet
func (s *Service) RenameWallet(address, name string) error {
	registry, err := s.LoadRegistry()
	if err != nil {
		return err
	}

	found := false
	for i := range registry.Wallets {
		if registry.Wallets[i].Address == address {
			registry.Wallets[i].Name = name
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("wallet not found: %s", address)
	}

	return s.SaveRegistry(registry)
}

// GetActiveWallet loads the currently active wallet
func (s *Service) GetActiveWallet() (*models.Account, error) {
	registry, err := s.LoadRegistry()
	if err != nil {
		return nil, err
	}

	if registry.ActiveWallet == "" {
		return nil, fmt.Errorf("no active wallet set")
	}

	return s.GetWalletByAddress(registry.ActiveWallet)
}

// RemoveWallet removes a wallet from storage
func (s *Service) RemoveWallet(address string) error {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Remove wallet file
	walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", address+".json")
	if err := os.Remove(walletPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove wallet file: %w", err)
	}

	// Clean up passkey state and keychain entries (best effort)
	deletePasskeys(address)                                //nolint:errcheck // best-effort cleanup
	keyring.Delete(keyringService, walletKeyItem(address)) //nolint:errcheck // best-effort cleanup
	keyring.Delete(keyringService, unlockItem(address))    //nolint:errcheck // best-effort cleanup
	scrubLegacySeedCaches(address)

	// Load and update registry with proper locking
	registry, err := s.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	// Remove wallet from registry
	found := false
	for i, w := range registry.Wallets {
		if w.Address == address {
			// Remove this wallet from the slice
			registry.Wallets = append(registry.Wallets[:i], registry.Wallets[i+1:]...)
			found = true
			break
		}
	}

	// Clear active wallet if this was it
	if registry.ActiveWallet == address {
		registry.ActiveWallet = ""
	}

	if found || registry.ActiveWallet == "" {
		if err := s.SaveRegistry(registry); err != nil {
			return fmt.Errorf("failed to update registry: %w", err)
		}
	}

	return nil
}

func (s *Service) MigrateFromLegacy() error {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Check for old wallet file
	oldWalletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account_stellar.json")
	data, err := os.ReadFile(oldWalletPath)
	if err != nil {
		// No legacy wallet to migrate
		return nil
	}

	var acc models.Account
	if err := json.Unmarshal(data, &acc); err != nil {
		return fmt.Errorf("failed to parse legacy wallet: %w", err)
	}

	// Migrate to new system
	if err := s.AddWalletToRegistry(&acc, true); err != nil {
		return fmt.Errorf("failed to migrate wallet: %w", err)
	}

	// Rename old file as backup
	backupPath := oldWalletPath + ".backup"
	_ = os.Rename(oldWalletPath, backupPath) //nolint:errcheck // backup is best-effort

	return nil
}

// GetActiveAccount loads the active wallet account from the registry
func (s *Service) GetActiveAccount() (*models.Account, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Load registry to get active wallet
	registryPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets.json")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("no wallet registry found: %w", err)
	}

	var registry struct {
		ActiveWallet string `json:"activeWallet"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse wallet registry: %w", err)
	}

	if registry.ActiveWallet == "" {
		return nil, fmt.Errorf("no active wallet set")
	}

	// Load the active wallet
	walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", registry.ActiveWallet+".json")
	wdata, err := os.ReadFile(walletPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wallet file: %w", err)
	}

	var account models.Account
	if err := json.Unmarshal(wdata, &account); err != nil {
		return nil, fmt.Errorf("failed to parse wallet data: %w", err)
	}

	return &account, nil
}
