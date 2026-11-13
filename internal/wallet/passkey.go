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
	keyringService  = "stellar-go-cli"
	ceremonyTimeout = 2 * time.Minute
)

func walletKeyItem(addr string) string { return "wallet-key-" + addr }

// unlockItem names the legacy keyring item that held unlock grants. Grants
// moved into custodian memory; the name remains so stale items can be deleted.
func unlockItem(addr string) string { return "unlock-" + addr }

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

// OpenCeremony opens the browser at the ceremony URL and always prints a
// clickable fallback link so the flow survives browsers that fail to launch.
func OpenCeremony(url string) error {
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

	creds, err := LoadPasskeys(address)
	if err != nil {
		return nil, err
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Starting passkey registration — a browser window will open.")
	cred, err := wan.Register(user, OpenCeremony, ceremonyTimeout)
	if err != nil {
		return nil, fmt.Errorf("passkey registration failed: %w", err)
	}
	return s.applyPasskeyProtection(acc, cred)
}

// applyPasskeyProtection runs the post-ceremony part of registration:
// encryption key to OS keychain, seed re-stored encrypted, passkey persisted.
// Shared by the local ceremony path and the relayed (client-side) ceremony.
func (s *Service) applyPasskeyProtection(acc *models.Account, cred *webauthn.Credential) (*webauthn.Credential, error) {
	// Generate the encryption key and store it in the OS keychain before
	// touching the wallet file, so a later failure leaves no half-protected state.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}
	if err := keyring.Set(keyringService, walletKeyItem(acc.Address), base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("could not store encryption key in OS keychain: %w", err)
	}

	enc, err := encryptSeed(acc.PrivateKey, key)
	if err != nil {
		keyring.Delete(keyringService, walletKeyItem(acc.Address)) //nolint:errcheck // best-effort cleanup on failure
		return nil, err
	}

	acc.PrivateKeyEnc = enc
	acc.PrivateKey = ""
	acc.PasskeyProtected = true
	if err := s.AddWalletToRegistry(acc, false); err != nil {
		keyring.Delete(keyringService, walletKeyItem(acc.Address)) //nolint:errcheck // best-effort cleanup on failure
		return nil, fmt.Errorf("failed to save protected wallet: %w", err)
	}

	if err := SavePasskey(acc.Address, cred); err != nil {
		return nil, fmt.Errorf("passkey registered but could not be stored: %w", err)
	}

	// Scrub plaintext seed copies from legacy state caches so the protected
	// seed cannot be recovered or used to bypass the lock.
	scrubLegacySeedCaches(acc.Address)

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
			os.WriteFile(path, out, 0600) //nolint:errcheck // best-effort cache scrub
		}
	}
}

// AssertAndDecrypt runs a WebAuthn assertion ceremony and returns the
// wallet's decrypted seed. The caller owns the returned seed — the custodian
// daemon uses it to place an unlock grant in its own memory (grants no longer
// live in the OS keychain, where any same-UID process could read them).
func (s *Service) AssertAndDecrypt(address string) (string, error) {
	return s.assertAndDecrypt(address, nil)
}

// AssertBoundAndDecrypt runs a challenge-bound assertion ceremony and returns
// the decrypted seed — operator paths that reveal key material (wallet
// export) bind the ceremony to the exact intent.
func (s *Service) AssertBoundAndDecrypt(address string, challenge []byte) (string, error) {
	return s.assertAndDecrypt(address, challenge)
}

func (s *Service) assertAndDecrypt(address string, challenge []byte) (string, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return "", fmt.Errorf("wallet not found: %w", err)
	}
	if !acc.PasskeyProtected {
		return "", fmt.Errorf("wallet %s is not passkey-protected", address)
	}

	creds, err := LoadPasskeys(address)
	if err != nil {
		return "", err
	}
	if len(creds) == 0 {
		return "", fmt.Errorf("no passkey registered for wallet %s", address)
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Starting passkey authentication — a browser window will open.")
	var cred *webauthn.Credential
	if len(challenge) > 0 {
		cred, err = wan.AssertBound(user, challenge, OpenCeremony, ceremonyTimeout)
	} else {
		cred, err = wan.Assert(user, OpenCeremony, ceremonyTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("passkey authentication failed: %w", err)
	}

	// Persist updated sign counter (clone detection).
	if err := SavePasskey(acc.Address, cred); err != nil {
		ui.Warn("Could not update passkey counter: " + err.Error())
	}

	return s.DecryptWalletSeed(acc)
}

// AssertBound runs a WebAuthn assertion whose challenge binds to the given
// request hash — the approval mechanism for custodian escalation (spec §10.2
// dynamic linking). Returns the updated credential on success.
func (s *Service) AssertBound(address string, challenge []byte) (*webauthn.Credential, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}
	creds, err := LoadPasskeys(address)
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no passkey registered for wallet %s", address)
	}
	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Confirm the requested operation with your passkey — a browser window will open.")
	cred, err := wan.AssertBound(user, challenge, OpenCeremony, ceremonyTimeout)
	if err != nil {
		return nil, fmt.Errorf("passkey authentication failed: %w", err)
	}
	if err := SavePasskey(acc.Address, cred); err != nil {
		ui.Warn("Could not update passkey counter: " + err.Error())
	}
	return cred, nil
}

// Lock drops any legacy keyring unlock grant for a wallet. Grants now live in
// the custodian daemon's memory; this stays for cleanup of pre-daemon grants
// and as the operator's best-effort keyring scrub.
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

	creds, err := LoadPasskeys(address)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		return fmt.Errorf("no passkey registered for wallet %s", address)
	}

	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	ui.Info("Verify your passkey to remove protection — a browser window will open.")
	if _, err := wan.Assert(user, OpenCeremony, ceremonyTimeout); err != nil {
		return fmt.Errorf("passkey authentication failed: %w", err)
	}

	seed, err := s.DecryptWalletSeed(acc)
	if err != nil {
		return err
	}

	acc.PrivateKey = seed
	acc.PrivateKeyEnc = ""
	acc.PasskeyProtected = false
	if err := s.AddWalletToRegistry(acc, false); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	s.Lock(address)                                            //nolint:errcheck // best-effort cleanup
	keyring.Delete(keyringService, walletKeyItem(acc.Address)) //nolint:errcheck // best-effort cleanup
	DeletePasskeys(address)                                    //nolint:errcheck // best-effort cleanup
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

	creds, err := LoadPasskeys(address)
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

	// Unlock state lives in the custodian daemon's memory — the local view
	// always reports locked. Query the daemon for grant state.
	return st, nil
}

// DecryptWalletSeed decrypts the at-rest encrypted seed using the keychain key.
func (s *Service) DecryptWalletSeed(acc *models.Account) (string, error) {
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

// ResolveSeed returns the plaintext seed for an account. Only unprotected
// (plaintext) wallets resolve locally — protected-wallet seeds resolve inside
// the custodian daemon's grant store, never in the caller's process.
// All local seed readers (keypair loaders, exports) must go through this.
func ResolveSeed(acc *models.Account) (string, error) {
	if acc.PrivateKey != "" {
		return acc.PrivateKey, nil
	}
	if acc.PasskeyProtected && acc.PrivateKeyEnc != "" {
		return "", fmt.Errorf(
			"wallet %s is passkey-protected — sign through the custodian daemon ('stellar-go-cli signer' + 'wallet passkey unlock %s')",
			acc.Address, acc.Address)
	}
	return "", fmt.Errorf("no private key for wallet %s", acc.Address)
}

// ─── Relayed ceremonies ──────────────────────────────────────────────────
// The custodian daemon builds the WebAuthn session; the CLIENT machine runs
// the browser ceremony and relays the raw credential response back. The
// daemon host needs no browser (headless deployments).

// BeginAssertRelay starts a challenge-bound assertion for a relayed ceremony:
// returns the daemon-side RelaySession (kept for Finish) and the options JSON
// the client's browser needs.
func (s *Service) BeginAssertRelay(address string, challenge []byte, origin string) (*wan.RelaySession, []byte, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}
	if !acc.PasskeyProtected {
		return nil, nil, fmt.Errorf("wallet %s is not passkey-protected", address)
	}
	creds, err := LoadPasskeys(address)
	if err != nil {
		return nil, nil, err
	}
	if len(creds) == 0 {
		return nil, nil, fmt.Errorf("no passkey registered for wallet %s", address)
	}
	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	return wan.BeginRelayFor(wan.ModeAssert, user, challenge, origin)
}

// FinishAssertRelay verifies the relayed assertion, updates the passkey sign
// counter, and returns the decrypted seed (caller owns it — custodian memory
// only).
func (s *Service) FinishAssertRelay(rs *wan.RelaySession, address string, assertionJSON []byte) (string, error) {
	cred, err := wan.FinishRelayAssert(rs, assertionJSON)
	if err != nil {
		return "", err
	}
	if err := SavePasskey(address, cred); err != nil {
		ui.Warn("Could not update passkey counter: " + err.Error())
	}
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return "", err
	}
	return s.DecryptWalletSeed(acc)
}

// VerifyAssertRelay is FinishAssertRelay without decryption — for actions
// that bind approval without revealing key material (approve, reactivate).
func (s *Service) VerifyAssertRelay(rs *wan.RelaySession, address string, assertionJSON []byte) error {
	cred, err := wan.FinishRelayAssert(rs, assertionJSON)
	if err != nil {
		return err
	}
	if err := SavePasskey(address, cred); err != nil {
		ui.Warn("Could not update passkey counter: " + err.Error())
	}
	return nil
}

// BeginRegisterRelay starts a passkey registration for a relayed ceremony.
func (s *Service) BeginRegisterRelay(address, origin string) (*wan.RelaySession, []byte, error) {
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}
	if acc.PasskeyProtected {
		return nil, nil, fmt.Errorf("wallet %s is already passkey-protected", address)
	}
	if acc.PrivateKey == "" {
		return nil, nil, fmt.Errorf("wallet %s has no private key to protect", address)
	}
	creds, err := LoadPasskeys(address)
	if err != nil {
		return nil, nil, err
	}
	user := &wan.User{Address: acc.Address, Name: acc.Address[:12], Credentials: creds}
	return wan.BeginRelayFor(wan.ModeRegister, user, nil, origin)
}

// FinishRegisterRelay verifies the relayed registration and applies passkey
// protection to the wallet.
func (s *Service) FinishRegisterRelay(rs *wan.RelaySession, address string, creationJSON []byte) (*webauthn.Credential, error) {
	cred, err := wan.FinishRelayRegister(rs, creationJSON)
	if err != nil {
		return nil, err
	}
	acc, err := s.GetWalletByAddress(address)
	if err != nil {
		return nil, err
	}
	return s.applyPasskeyProtection(acc, cred)
}
