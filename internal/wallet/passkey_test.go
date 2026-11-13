package wallet

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/zalando/go-keyring"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestEncryptDecryptSeedRoundTrip(t *testing.T) {
	key := testKey(t)
	seed := "SABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ABCDEFGHIJKLMNOPQRSTU"

	enc, err := encryptSeed(seed, key)
	if err != nil {
		t.Fatalf("encryptSeed: %v", err)
	}
	if enc == seed {
		t.Fatal("ciphertext equals plaintext")
	}

	got, err := decryptSeed(enc, key)
	if err != nil {
		t.Fatalf("decryptSeed: %v", err)
	}
	if got != seed {
		t.Fatalf("round trip mismatch: got %q", got)
	}
}

func TestDecryptSeedWrongKey(t *testing.T) {
	key := testKey(t)
	wrong := make([]byte, 32)

	enc, err := encryptSeed("SXYZ", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptSeed(enc, wrong); err == nil {
		t.Fatal("decryption with wrong key succeeded")
	}
}

func TestDecryptSeedRejectsGarbage(t *testing.T) {
	key := testKey(t)
	if _, err := decryptSeed("not!base64!!!", key); err == nil {
		t.Fatal("bad base64 accepted")
	}
	if _, err := decryptSeed(base64.StdEncoding.EncodeToString([]byte("short")), key); err == nil {
		t.Fatal("short ciphertext accepted")
	}
	// Tampered ciphertext must fail GCM auth
	enc, _ := encryptSeed("SXYZ", key)
	raw, _ := base64.StdEncoding.DecodeString(enc)
	raw[len(raw)-1] ^= 0xFF
	if _, err := decryptSeed(base64.StdEncoding.EncodeToString(raw), key); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestPasskeyStoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr := "GTESTSTORE"

	creds, err := LoadPasskeys(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 0 {
		t.Fatal("expected no credentials for new address")
	}

	cred := &webauthn.Credential{
		ID:        []byte("credential-id-1"),
		PublicKey: []byte{1, 2, 3},
	}
	if err := SavePasskey(addr, cred); err != nil {
		t.Fatalf("SavePasskey: %v", err)
	}

	creds, err = LoadPasskeys(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 1 || string(creds[0].ID) != "credential-id-1" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}

	// File must be 0600
	path, err := passkeyPath(addr)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("passkey file perm %v, want 0600", perm)
	}
	if filepath.Dir(path) == "" {
		t.Fatal("bad path")
	}
}

func TestPasskeyStoreUpsert(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr := "GTESTUPSERT"

	c1 := &webauthn.Credential{ID: []byte("id1"), PublicKey: []byte{1}}
	if err := SavePasskey(addr, c1); err != nil {
		t.Fatal(err)
	}
	c1Updated := &webauthn.Credential{ID: []byte("id1"), PublicKey: []byte{9, 9}}
	if err := SavePasskey(addr, c1Updated); err != nil {
		t.Fatal(err)
	}
	creds, err := LoadPasskeys(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 1 {
		t.Fatalf("upsert duplicated credential: %d entries", len(creds))
	}
	if creds[0].PublicKey[0] != 9 {
		t.Fatal("upsert did not replace credential")
	}

	if err := DeletePasskeys(addr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path2(t, addr)); !os.IsNotExist(err) {
		t.Fatal("passkey file not deleted")
	}
	// delete of missing file is not an error
	if err := DeletePasskeys(addr); err != nil {
		t.Fatal(err)
	}
}

func path2(t *testing.T, addr string) string {
	t.Helper()
	p, err := passkeyPath(addr)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveSeedPlaintext(t *testing.T) {
	keyring.MockInit()
	acc := &models.Account{Address: "GPLAIN", PrivateKey: "SSEED"}
	seed, err := ResolveSeed(acc)
	if err != nil {
		t.Fatal(err)
	}
	if seed != "SSEED" {
		t.Fatalf("got %q", seed)
	}
}

func TestResolveSeedProtectedLocked(t *testing.T) {
	keyring.MockInit()
	acc := &models.Account{
		Address:          "GLOCKED",
		PasskeyProtected: true,
		PrivateKeyEnc:    "AAAA",
	}
	if _, err := ResolveSeed(acc); err == nil {
		t.Fatal("locked protected wallet resolved a seed")
	}
}

func TestResolveSeedProtectedRoutesToCustodian(t *testing.T) {
	keyring.MockInit()
	// Grants live in the custodian daemon's memory — local ResolveSeed must
	// never produce a protected wallet's seed, regardless of keyring state.
	addr := "GUNLOCKED"
	if err := keyring.Set(keyringService, unlockItem(addr), `{"seed":"STALE","expires":9999999999}`); err != nil {
		t.Fatal(err)
	}

	acc := &models.Account{Address: addr, PasskeyProtected: true, PrivateKeyEnc: "AAAA"}
	if _, err := ResolveSeed(acc); err == nil {
		t.Fatal("protected wallet resolved a seed locally")
	} else if !strings.Contains(err.Error(), "custodian") {
		t.Fatalf("expected custodian-directed error, got %v", err)
	}
}

func TestResolveSeedNoKey(t *testing.T) {
	keyring.MockInit()
	acc := &models.Account{Address: "GEMPTY"}
	if _, err := ResolveSeed(acc); err == nil {
		t.Fatal("empty account resolved a seed")
	}
}

func TestDecryptWalletSeedViaKeyring(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	addr := "GDECRYPT"

	key := testKey(t)
	enc, err := encryptSeed("SREAL", key)
	if err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set(keyringService, walletKeyItem(addr), base64.StdEncoding.EncodeToString(key)); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	acc := &models.Account{Address: addr, PrivateKeyEnc: enc}
	seed, err := svc.DecryptWalletSeed(acc)
	if err != nil {
		t.Fatal(err)
	}
	if seed != "SREAL" {
		t.Fatalf("got %q", seed)
	}

	// Missing keychain key → descriptive error
	if _, err := svc.DecryptWalletSeed(&models.Account{Address: "GNOKEY", PrivateKeyEnc: enc}); err == nil {
		t.Fatal("decrypt without keychain key succeeded")
	}
}
