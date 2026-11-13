// agent_identity.go — the gatekeeper's DID:key agent identity for mandate
// holder binding. The key is an agent credential, NOT wallet material: it
// proves which gatekeeper instance a mandate was issued to, so a stolen
// mandate cannot be replayed by a different process (spec §6.4 check 2).
package mcp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// agentKeyFile is the persisted agent identity under the operator state dir.
const agentKeyFile = "agent-key.json"

// agentIdentity is the gatekeeper's persistent did:key signing identity.
type agentIdentity struct {
	priv ed25519.PrivateKey
	did  string
}

var (
	agentOnce sync.Once
	agentInst *agentIdentity
	agentErr  error
)

// loadAgentKey returns the process-shared agent identity, generating and
// persisting it on first use. Persistence is best-effort: an ephemeral
// identity still works — mandates bound to it just die with the process.
func loadAgentKey() (*agentIdentity, error) {
	agentOnce.Do(func() {
		agentInst, agentErr = loadOrGenerateAgentKey()
	})
	return agentInst, agentErr
}

func loadOrGenerateAgentKey() (*agentIdentity, error) {
	dir, err := config.StateDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, agentKeyFile)

	if data, err := os.ReadFile(path); err == nil { // #nosec G304 -- fixed filename inside operator state dir
		var stored struct {
			Priv string `json:"priv"` // base64 ed25519 private key (agent identity only)
			DID  string `json:"did"`
		}
		if json.Unmarshal(data, &stored) == nil && stored.Priv != "" {
			priv, derr := base64.StdEncoding.DecodeString(stored.Priv)
			if derr == nil && len(priv) == ed25519.PrivateKeySize {
				return &agentIdentity{priv: ed25519.PrivateKey(priv), did: stored.DID}, nil
			}
		}
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id := &agentIdentity{priv: priv, did: mpCrypto.DIDKeyFromEd25519PublicKey(pub)}

	// Persist for mandate continuity across restarts (0600).
	stored := struct {
		Priv string `json:"priv"`
		DID  string `json:"did"`
	}{Priv: base64.StdEncoding.EncodeToString(priv), DID: id.did}
	if data, err := json.Marshal(stored); err == nil {
		if err := os.MkdirAll(dir, 0700); err == nil {
			// #nosec G104 G304 -- best-effort persistence; fixed path under state dir
			os.WriteFile(path, data, 0600) //nolint:errcheck // best-effort persistence
		}
	}
	return id, nil
}

// DID returns the agent's did:key identifier — the value to bind mandates to.
func (a *agentIdentity) DID() string { return a.did }

// SignPOP produces the holder-binding signature over the given preimage.
func (a *agentIdentity) SignPOP(preimage []byte) (string, error) {
	if a == nil || a.priv == nil {
		return "", fmt.Errorf("agent identity unavailable")
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(a.priv, preimage)), nil
}
