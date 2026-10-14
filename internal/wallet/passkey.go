package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	wan "github.com/stellar-go-cli/stellar-go-cli/internal/webauthn"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/zalando/go-keyring"
)

const (
	keyringService   = "stellar-go-cli"
	ceremonyTimeout  = 2 * time.Minute
	defaultUnlockTTL = 15 * time.Minute
)

// unlockGrant is the short-lived session record stored in the OS keyring after
// a successful passkey assertion.
type unlockGrant struct {
	Seed    string `json:"seed"`
	Expires int64  `json:"expires"`
}

func walletKeyItem(addr string) string { return "wallet-key-" + addr }
func unlockItem(addr string) string    { return "unlock-" + addr }

// encryptSeed encrypts a seed with AES-256-GCM and returns base64(nonce‖ciphertext).
func encryptSeed(seed string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(seed), nil)), nil
}

// decryptSeed reverses encryptSeed.
func decryptSeed(enc string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("bad encrypted seed encoding: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted seed too short")
	}
	seed, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("seed decryption failed: %w", err)
	}
	return string(seed), nil
}

// openCeremony opens the browser at the ceremony URL and always prints a
// clickable fallback link so the flow survives browsers that fail to launch.
func openCeremony(url string) error {
	if err := wan.OpenBrowser(url); err != nil {
		ui.Warn("Could not open a browser automatically.")
	}
	ui.Link(url, "Open this link in your browser to continue")
	return nil
}

// RegisterPasskey runs a real WebAuthn registration ceremony for the wallet and
// enables passkey protection: the seed is re-stored AES-256-GCM-encrypted with
// the key kept in the OS keychain.
func (s *Service) RegisterPasskey(address string) (*webauthn.Credential, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}
	if acc.PasskeyProtected {
		return nil, fmt.Errorf("wallet %s is already passkey-protected", address)
	}
	if acc.PrivateKey == "" {
		return nil, fmt.Errorf("wallet %s has no private key to protect", address)
	}

	creds, err := loadPasskeys(address)
	if err != nil {
		return nil, err
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Starting passkey registration — a browser window will open.")
	cred, err := wan.Register(user, openCeremony, ceremonyTimeout)
	if err != nil {
		return nil, fmt.Errorf("passkey registration failed: %w", err)
	}

	// Generate the encryption key and store it in the OS keychain before
	// touching the wallet file, so a later failure leaves no half-protected state.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}
	if err := keyring.Set(keyringService, walletKeyItem(address), base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("could not store encryption key in OS keychain: %w", err)
	}

	enc, err := encryptSeed(acc.PrivateKey, key)
	if err != nil {
		_ = keyring.Delete(keyringService, walletKeyItem(address))
		return nil, err
	}

	acc.PrivateKeyEnc = enc
	acc.PrivateKey = ""
	acc.PasskeyProtected = true
	if err := s.AddWalletToRegistry(acc, false); err != nil {
		_ = keyring.Delete(keyringService, walletKeyItem(address))
		return nil, fmt.Errorf("failed to save protected wallet: %w", err)
	}

	if err := savePasskey(address, cred); err != nil {
		return nil, fmt.Errorf("passkey registered but could not be stored: %w", err)
	}

	// Scrub plaintext seed copies from legacy state caches so the protected
	// seed cannot be recovered or used to bypass the lock.
	scrubLegacySeedCaches(address)

	return cred, nil
}

// scrubLegacySeedCaches removes the plaintext seed from legacy account caches
// (account.json, account_stellar.json) when they describe the given address.
// Display fields are preserved; only the secret is stripped.
func scrubLegacySeedCaches(address string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	stateDir := filepath.Join(home, ".stellar-go-cli", "state")
	for _, name := range []string{"account.json", "account_stellar.json"} {
		path := filepath.Join(stateDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var acc models.Account
		if err := json.Unmarshal(data, &acc); err != nil || acc.Address != address || acc.PrivateKey == "" {
			continue
		}
		acc.PrivateKey = ""
		if out, err := json.Marshal(acc); err == nil {
			_ = os.WriteFile(path, out, 0600)
		}
	}
}

// Unlock runs a WebAuthn assertion ceremony; on success the decrypted seed is
// placed in the OS keychain as a session grant valid for ttl (default 15m).
func (s *Service) Unlock(address string, ttl time.Duration) error {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return fmt.Errorf("wallet not found: %w", err)
	}
	if !acc.PasskeyProtected {
		return fmt.Errorf("wallet %s is not passkey-protected", address)
	}

	creds, err := loadPasskeys(address)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		return fmt.Errorf("no passkey registered for wallet %s", address)
	}
	if ttl <= 0 {
		ttl = defaultUnlockTTL
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Starting passkey authentication — a browser window will open.")
	cred, err := wan.Assert(user, openCeremony, ceremonyTimeout)
	if err != nil {
		return fmt.Errorf("passkey authentication failed: %w", err)
	}

	// Persist updated sign counter (clone detection).
	if err := savePasskey(address, cred); err != nil {
		ui.Warn("Could not update passkey counter: " + err.Error())
	}

	seed, err := s.decryptWalletSeed(acc)
	if err != nil {
		return err
	}

	grant := unlockGrant{Seed: seed, Expires: time.Now().Add(ttl).Unix()}
	data, err := json.Marshal(grant)
	if err != nil {
		return fmt.Errorf("grant encoding failed: %w", err)
	}
	if err := keyring.Set(keyringService, unlockItem(address), string(data)); err != nil {
		return fmt.Errorf("could not store unlock grant in OS keychain: %w", err)
	}
	return nil
}

// Lock drops the unlock grant for a wallet.
func (s *Service) Lock(address string) error {
	if err := keyring.Delete(keyringService, unlockItem(address)); err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("could not remove unlock grant: %w", err)
	}
	return nil
}

// DisablePasskey requires a successful assertion, then restores the plaintext
// seed and removes all passkey state for the wallet.
func (s *Service) DisablePasskey(address string) error {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return fmt.Errorf("wallet not found: %w", err)
	}
	if !acc.PasskeyProtected {
		return fmt.Errorf("wallet %s is not passkey-protected", address)
	}

	creds, err := loadPasskeys(address)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		return fmt.Errorf("no passkey registered for wallet %s", address)
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Verify your passkey to remove protection — a browser window will open.")
	if _, err := wan.Assert(user, openCeremony, ceremonyTimeout); err != nil {
		return fmt.Errorf("passkey authentication failed: %w", err)
	}

	seed, err := s.decryptWalletSeed(acc)
	if err != nil {
		return err
	}

	acc.PrivateKey = seed
	acc.PrivateKeyEnc = ""
	acc.PasskeyProtected = false
	if err := s.AddWalletToRegistry(acc, false); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	_ = s.Lock(address)
	_ = keyring.Delete(keyringService, walletKeyItem(address))
	_ = deletePasskeys(address)
	return nil
}

// PasskeyStatus reports protection and unlock state for a wallet.
type PasskeyStatus struct {
	Address       string           `json:"address"`
	Protected     bool             `json:"protected"`
	Unlocked      bool             `json:"unlocked"`
	UnlockedUntil *time.Time       `json:"unlockedUntil,omitempty"`
	Credentials   []PasskeySummary `json:"credentials,omitempty"`
}

// PasskeySummary is a display-friendly view of a stored credential.
type PasskeySummary struct {
	CredentialID string   `json:"credentialId"`
	Transports   []string `json:"transports,omitempty"`
	SignCount    uint32   `json:"signCount"`
}

// GetPasskeyStatus returns the passkey status for a wallet.
func (s *Service) GetPasskeyStatus(address string) (*PasskeyStatus, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	st := &PasskeyStatus{Address: address, Protected: acc.PasskeyProtected}

	creds, err := loadPasskeys(address)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		var transports []string
		for _, t := range c.Transport {
			transports = append(transports, string(t))
		}
		st.Credentials = append(st.Credentials, PasskeySummary{
			CredentialID: base64.RawURLEncoding.EncodeToString(c.ID),
			Transports:   transports,
			SignCount:    c.Authenticator.SignCount,
		})
	}

	if grant, err := readUnlockGrant(address); err == nil && time.Now().Unix() < grant.Expires {
		st.Unlocked = true
		exp := time.Unix(grant.Expires, 0)
		st.UnlockedUntil = &exp
	}
	return st, nil
}

// decryptWalletSeed decrypts the at-rest encrypted seed using the keychain key.
func (s *Service) decryptWalletSeed(acc *models.Account) (string, error) {
	if acc.PrivateKeyEnc == "" {
		return "", fmt.Errorf("wallet %s has no encrypted seed", acc.Address)
	}
	keyB64, err := keyring.Get(keyringService, walletKeyItem(acc.Address))
	if err != nil {
		return "", fmt.Errorf("encryption key not found in OS keychain: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", fmt.Errorf("bad keychain key encoding: %w", err)
	}
	return decryptSeed(acc.PrivateKeyEnc, key)
}

// readUnlockGrant loads the session grant for an unlocked wallet.
func readUnlockGrant(address string) (*unlockGrant, error) {
	data, err := keyring.Get(keyringService, unlockItem(address))
	if err != nil {
		return nil, err
	}
	var grant unlockGrant
	if err := json.Unmarshal([]byte(data), &grant); err != nil {
		return nil, fmt.Errorf("bad unlock grant: %w", err)
	}
	return &grant, nil
}

// unlockedSeed returns the decrypted seed for a wallet with a live unlock grant,
// or an error if locked/expired.
func unlockedSeed(address string) (string, error) {
	grant, err := readUnlockGrant(address)
	if err != nil {
		return "", fmt.Errorf("no unlock grant")
	}
	if time.Now().Unix() >= grant.Expires {
		_ = keyring.Delete(keyringService, unlockItem(address)) // clean expired grant
		return "", fmt.Errorf("unlock grant expired")
	}
	return grant.Seed, nil
}

// ResolveSeed returns the plaintext seed for an account: directly if stored,
// or from a live passkey unlock grant for protected wallets.
// All seed readers (keypair loaders, exports, signers) must go through this.
func ResolveSeed(acc *models.Account) (string, error) {
	if acc.PrivateKey != "" {
		return acc.PrivateKey, nil
	}
	if !acc.PasskeyProtected || acc.PrivateKeyEnc == "" {
		return "", fmt.Errorf("no private key for wallet %s", acc.Address)
	}
	seed, err := unlockedSeed(acc.Address)
	if err != nil {
		return "", fmt.Errorf(
			"wallet %s is passkey-protected and locked — run 'stellar-go-cli wallet passkey unlock %s'",
			acc.Address, acc.Address)
	}
	return seed, nil
}
