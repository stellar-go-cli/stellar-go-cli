package wallet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go-stellar-sdk/keypair"
)

// LoadStellarKeypair loads the active Stellar keypair from the wallet registry or legacy paths.
// It tries multiple fallback locations in order:
// 1. Active wallet from registry (wallets.json -> wallets/<address>.json)
// 2. Legacy account_stellar.json
// 3. Legacy account.json (only if WalletStellar type)
// 4. Legacy stellar_keypair state
func LoadStellarKeypair() (*keypair.Full, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try to load from wallet registry first
	registryPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets.json")
	if data, err := os.ReadFile(registryPath); err == nil {
		var registry struct {
			ActiveWallet string `json:"activeWallet"`
		}
		if json.Unmarshal(data, &registry) == nil && registry.ActiveWallet != "" {
			walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", registry.ActiveWallet+".json")
			if wdata, err := os.ReadFile(walletPath); err == nil {
				var account models.Account
				if json.Unmarshal(wdata, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
					seed, err := ResolveSeed(&account)
					if err != nil {
						return nil, err
					}
					return keypair.ParseFull(seed)
				}
			}
		}
	}

	// Fall back to legacy paths for compatibility
	var account models.Account
	stellarAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account_stellar.json")
	if data, err := os.ReadFile(stellarAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
			seed, err := ResolveSeed(&account)
			if err != nil {
				return nil, err
			}
			return keypair.ParseFull(seed)
		}
	}

	defaultAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account.json")
	if data, err := os.ReadFile(defaultAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") && account.Type == models.WalletStellar {
			seed, err := ResolveSeed(&account)
			if err != nil {
				return nil, err
			}
			return keypair.ParseFull(seed)
		}
	}

	// Final fallback to old format
	var saved struct {
		Seed string `json:"seed"`
	}
	if err := config.LoadState("stellar_keypair", &saved); err == nil && saved.Seed != "" {
		return keypair.ParseFull(saved.Seed)
	}

	return nil, fmt.Errorf("no Stellar keypair found")
}

// LoadStellarKeypairForSwap loads a Stellar keypair with special handling for swap operations.
// It requires a wallet with a stored private key.
func LoadStellarKeypairForSwap() (*keypair.Full, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Load registry to get active wallet
	registryPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets.json")
	if data, err := os.ReadFile(registryPath); err == nil {
		var registry struct {
			ActiveWallet string `json:"activeWallet"`
		}
		if json.Unmarshal(data, &registry) == nil && registry.ActiveWallet != "" {
			walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", registry.ActiveWallet+".json")
			if wdata, err := os.ReadFile(walletPath); err == nil {
				var account models.Account
				if json.Unmarshal(wdata, &account) == nil {
					if account.PrivateKey != "" || account.PrivateKeyEnc != "" {
						seed, err := ResolveSeed(&account)
						if err != nil {
							return nil, err
						}
						return keypair.ParseFull(seed)
					}
				}
			}
		}
	}

	// Fall back to legacy paths for compatibility
	var account models.Account
	stellarAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account_stellar.json")
	if data, err := os.ReadFile(stellarAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
			seed, err := ResolveSeed(&account)
			if err != nil {
				return nil, err
			}
			return keypair.ParseFull(seed)
		}
	}

	return nil, fmt.Errorf("no Stellar keypair found")
}

// LoadStellarKeypairForAddress loads a specific Stellar keypair by address from the wallet registry.
// This ensures the correct wallet is used when cfg.ActiveAddress may differ from the registry's active wallet.
func LoadStellarKeypairForAddress(address string) (*keypair.Full, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try to load the specific wallet from registry
	walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", address+".json")
	if wdata, err := os.ReadFile(walletPath); err == nil {
		var account models.Account
		if json.Unmarshal(wdata, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
			seed, err := ResolveSeed(&account)
			if err != nil {
				return nil, err
			}
			kp, err := keypair.ParseFull(seed)
			if err != nil {
				return nil, fmt.Errorf("invalid private key for address %s: %w", address, err)
			}
			// Verify the loaded keypair matches the expected address
			if kp.Address() != address {
				return nil, fmt.Errorf("keypair address mismatch: expected %s, got %s", address, kp.Address())
			}
			return kp, nil
		}
	}

	return nil, fmt.Errorf("no Stellar keypair found for address: %s", address)
}
