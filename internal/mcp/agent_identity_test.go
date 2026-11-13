package mcp

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

func TestAgentIdentityPersistsAndSigns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	a1, err := loadOrGenerateAgentKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(a1.DID(), "did:key:z") {
		t.Fatalf("not a did:key: %s", a1.DID())
	}

	// Second load reads the persisted key — same DID.
	a2, err := loadOrGenerateAgentKey()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a1.DID() != a2.DID() {
		t.Fatalf("identity not persisted: %s vs %s", a1.DID(), a2.DID())
	}

	// POP signs bytes the custodian can verify against the DID.
	preimage := []byte("sha256-preimage")
	popB64, err := a1.SignPOP(preimage)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.StdEncoding.DecodeString(popB64)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := mpCrypto.DecodeEd25519PublicKeyFromDIDKey(a1.DID())
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, preimage, sig) {
		t.Fatal("POP signature does not verify against agent DID")
	}
}

func TestAgentIdentityCorruptStoreRegenerates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write garbage at the key path — must regenerate, not crash.
	dir := home + "/.stellar-go-cli/state"
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/agent-key.json", []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := loadOrGenerateAgentKey()
	if err != nil {
		t.Fatalf("regenerate after corrupt store: %v", err)
	}
	if !strings.HasPrefix(a.DID(), "did:key:z") {
		t.Fatalf("bad regenerated DID: %s", a.DID())
	}
}
