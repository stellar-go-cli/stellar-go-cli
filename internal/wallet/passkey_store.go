package wallet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-webauthn/webauthn/webauthn"
)

// passkeysDir returns the directory holding per-wallet passkey credential records.
func passkeysDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	dir := filepath.Join(home, ".stellar-go-cli", "state", "passkeys")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create passkeys directory: %w", err)
	}
	return dir, nil
}

func passkeyPath(addr string) (string, error) {
	dir, err := passkeysDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, addr+".json"), nil
}

// loadPasskeys returns the WebAuthn credentials registered for a wallet.
// A missing file yields an empty slice, not an error.
func loadPasskeys(addr string) ([]webauthn.Credential, error) {
	path, err := passkeyPath(addr)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read passkeys: %w", err)
	}
	var creds []webauthn.Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse passkeys: %w", err)
	}
	return creds, nil
}

// savePasskey upserts a credential (matched by ID) for a wallet.
func savePasskey(addr string, cred *webauthn.Credential) error {
	creds, err := loadPasskeys(addr)
	if err != nil {
		return err
	}

	replaced := false
	for i := range creds {
		if string(creds[i].ID) == string(cred.ID) {
			creds[i] = *cred
			replaced = true
			break
		}
	}
	if !replaced {
		creds = append(creds, *cred)
	}

	path, err := passkeyPath(addr)
	if err != nil {
		return err
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal passkeys: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write passkeys: %w", err)
	}
	return nil
}

// deletePasskeys removes the passkey file for a wallet (best effort on missing).
func deletePasskeys(addr string) error {
	path, err := passkeyPath(addr)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove passkeys: %w", err)
	}
	return nil
}
