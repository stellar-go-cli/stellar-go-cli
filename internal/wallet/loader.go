package wallet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/keypair"
)

// FindStellarAccount locates the active Stellar wallet record (key fields
// intact — trusted-surface callers only). Lookup order:
// 1. Active wallet from registry (wallets.json -> wallets/<address>.json)
// 2. Legacy account_stellar.json
// 3. Legacy account.json (only if WalletStellar type)
func FindStellarAccount() (*models.Account, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

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
					return &account, nil
				}
			}
		}
	}

	var account models.Account
	stellarAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account_stellar.json")
	if data, err := os.ReadFile(stellarAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
			return &account, nil
		}
	}

	defaultAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account.json")
	if data, err := os.ReadFile(defaultAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") && account.Type == models.WalletStellar {
			return &account, nil
		}
	}

	return nil, fmt.Errorf("no Stellar wallet found")
}

// FindStellarAccountForAddress locates a specific wallet record by address.
func FindStellarAccountForAddress(address string) (*models.Account, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	walletPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets", address+".json")
	if wdata, err := os.ReadFile(walletPath); err == nil {
		var account models.Account
		if json.Unmarshal(wdata, &account) == nil && (account.PrivateKey != "" || account.PrivateKeyEnc != "") {
			return &account, nil
		}
	}
	return nil, fmt.Errorf("no Stellar wallet found for address: %s", address)
}

// FindStellarAccountForSwap locates a wallet for swap operations — active
// registry wallet or legacy account_stellar.json.
func FindStellarAccountForSwap() (*models.Account, error) {
	return FindStellarAccount()
}

// LoadStellarKeypair loads the active Stellar keypair for a plaintext wallet.
// Protected wallets fail here — they sign through the custodian daemon
// (ResolveSeed enforces this).
func LoadStellarKeypair() (*keypair.Full, error) {
	account, err := FindStellarAccount()
	if err != nil {
		// Final fallback to old format
		var saved struct {
			Seed string `json:"seed"`
		}
		if loadErr := config.LoadState("stellar_keypair", &saved); loadErr == nil && saved.Seed != "" {
			return keypair.ParseFull(saved.Seed)
		}
		return nil, fmt.Errorf("no Stellar keypair found")
	}
	seed, err := ResolveSeed(account)
	if err != nil {
		return nil, err
	}
	return keypair.ParseFull(seed)
}

// LoadStellarKeypairForSwap loads a Stellar keypair for swap operations
// (plaintext wallets only — see LoadStellarKeypair).
func LoadStellarKeypairForSwap() (*keypair.Full, error) {
	account, err := FindStellarAccountForSwap()
	if err != nil {
		return nil, fmt.Errorf("no Stellar keypair found")
	}
	seed, err := ResolveSeed(account)
	if err != nil {
		return nil, err
	}
	return keypair.ParseFull(seed)
}

// LoadStellarKeypairForAddress loads a specific Stellar keypair by address
// (plaintext wallets only — see LoadStellarKeypair).
func LoadStellarKeypairForAddress(address string) (*keypair.Full, error) {
	account, err := FindStellarAccountForAddress(address)
	if err != nil {
		return nil, err
	}
	seed, err := ResolveSeed(account)
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

// LoadStellarSigner loads the active wallet's keypair wrapped in the shared
// signer abstraction — plaintext wallets only. Callers needing a signer for
// any wallet (including passkey-protected) go through the signer resolver,
// which returns a custodian RemoteSigner for protected wallets.
func LoadStellarSigner() (signer.KeypairSigner, error) {
	kp, err := LoadStellarKeypair()
	if err != nil {
		return signer.KeypairSigner{}, err
	}
	return signer.KeypairSigner{KP: kp}, nil
}

// LoadStellarSignerForSwap is LoadStellarKeypairForSwap wrapped as a signer.
func LoadStellarSignerForSwap() (signer.KeypairSigner, error) {
	kp, err := LoadStellarKeypairForSwap()
	if err != nil {
		return signer.KeypairSigner{}, err
	}
	return signer.KeypairSigner{KP: kp}, nil
}

// LoadStellarSignerForAddress is LoadStellarKeypairForAddress wrapped as a signer.
func LoadStellarSignerForAddress(address string) (signer.KeypairSigner, error) {
	kp, err := LoadStellarKeypairForAddress(address)
	if err != nil {
		return signer.KeypairSigner{}, err
	}
	return signer.KeypairSigner{KP: kp}, nil
}

// LoadActiveStellarAddress returns the active wallet's public address without
// loading any key material — used for key-free dry-run previews.
func LoadActiveStellarAddress() (string, error) {
	stateDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	registryPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "wallets.json")
	if data, err := os.ReadFile(registryPath); err == nil {
		var registry struct {
			ActiveWallet string `json:"activeWallet"`
		}
		if json.Unmarshal(data, &registry) == nil && registry.ActiveWallet != "" {
			return registry.ActiveWallet, nil
		}
	}

	var account models.Account
	stellarAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account_stellar.json")
	if data, err := os.ReadFile(stellarAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && account.Address != "" {
			return account.Address, nil
		}
	}

	defaultAccountPath := filepath.Join(stateDir, ".stellar-go-cli", "state", "account.json")
	if data, err := os.ReadFile(defaultAccountPath); err == nil {
		if json.Unmarshal(data, &account) == nil && account.Address != "" && account.Type == models.WalletStellar {
			return account.Address, nil
		}
	}

	return "", fmt.Errorf("no active Stellar wallet found")
}
