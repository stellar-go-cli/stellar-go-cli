package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	wan "github.com/stellar-go-cli/stellar-go-cli/internal/webauthn"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
)

// decisionCtxKey carries the per-call decision ref into handlers — the audit
// record hash a signing call forwards to the custodian (E5 linkage).
type decisionCtxKey struct{}

// mintDecisionRef produces the per-call decision identifier recorded in the
// audit log and forwarded to the custodian for signing calls.
func mintDecisionRef(transport, tool string, args map[string]interface{}) string {
	canonical, merr := json.Marshal(args)
	if merr != nil {
		canonical = []byte(fmt.Sprintf("%v", args))
	}
	h := sha256.New()
	h.Write([]byte(transport))
	h.Write([]byte{0})
	h.Write([]byte(tool))
	h.Write([]byte{0})
	h.Write(canonical)
	h.Write([]byte{0})
	h.Write([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// decisionRefFrom returns the call's decision ref ("" for non-decision ctx).
func decisionRefFrom(ctx context.Context) string {
	if v := ctx.Value(decisionCtxKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// custodianSocket resolves the custodian socket path (operator override or
// the state-dir default).
func (s *Server) custodianSocket() (string, error) {
	if s.custodianSock != "" {
		return s.custodianSock, nil
	}
	return signsock.DefaultSocketPath()
}

// custodianWallet returns the sanitized wallet view over the custodian
// socket. The gatekeeper holds no wallet state or key material — all wallet
// reads go through the daemon (fail closed when it isn't running).
func (s *Server) custodianWallet() (*signsock.RemoteWallet, error) {
	path, err := s.custodianSocket()
	if err != nil {
		return nil, err
	}
	return signsock.Dial(path).RemoteWallet(), nil
}

// custodianClient returns a raw custodian client for signing calls.
func (s *Server) custodianClient() (*signsock.Client, error) {
	path, err := s.custodianSocket()
	if err != nil {
		return nil, err
	}
	return signsock.Dial(path), nil
}

// remoteSigner builds a custodian-backed signer for the account bound to the
// call's tool + decision ref. When a covering mandate exists it is attached
// with the agent's holder-binding proof — the custodian re-verifies
// everything, so a wrong selection just fails closed.
func (s *Server) remoteSigner(ctx context.Context, tool, address string) (signer.Signer, error) {
	client, err := s.custodianClient()
	if err != nil {
		return nil, err
	}
	rs := signsock.NewRemoteSigner(client, address, tool, decisionRefFrom(ctx))
	if ref, pop := s.selectMandate(tool, address); ref != "" && pop != nil {
		rs = rs.WithMandate(ref, pop)
	}
	return rs, nil
}

// selectMandate picks the first active mandate whose scope covers the tool —
// the gatekeeper's §7.1 analysis choice. The custodian independently
// re-verifies scope, holder binding, limits, and status.
func (s *Server) selectMandate(tool, account string) (string, signsock.POPSinger) {
	rw, err := s.custodianWallet()
	if err != nil {
		return "", nil
	}
	mandates, err := rw.ListMandates(account)
	if err != nil {
		return "", nil
	}
	agent, err := loadAgentKey()
	if err != nil {
		return "", nil
	}
	for _, m := range mandates {
		status, _ := m["status"].(string)
		if status != "active" {
			continue
		}
		agentDID, _ := m["agent"].(string)
		if agentDID != agent.DID() {
			continue
		}
		scope, _ := m["scope"].([]interface{})
		for _, sc := range scope {
			name, _ := sc.(string)
			if name == tool || name == "*" ||
				(len(name) > 1 && name[len(name)-1] == '*' && len(tool) >= len(name)-1 && tool[:len(name)-1] == name[:len(name)-1]) {
				id, _ := m["id"].(string)
				return id, agent
			}
		}
	}
	return "", nil
}

// activeWallet resolves the sanitized active account through the custodian.
func (s *Server) activeWallet() (*models.Account, error) {
	rw, err := s.custodianWallet()
	if err != nil {
		return nil, err
	}
	return rw.ActiveWallet()
}

// walletByAddress resolves a sanitized account through the custodian.
func (s *Server) walletByAddress(address string) (*models.Account, error) {
	rw, err := s.custodianWallet()
	if err != nil {
		return nil, err
	}
	return rw.GetWalletByAddress(address)
}

// listWallets returns the registry through the custodian.
func (s *Server) listWallets() ([]models.WalletEntry, string, error) {
	rw, err := s.custodianWallet()
	if err != nil {
		return nil, "", err
	}
	return rw.ListWallets()
}

// ─── Network helpers (pure Horizon queries — no wallet state) ──────────────

// horizonClientFor picks the Horizon client for a network.
func horizonClientFor(network models.Network) (*horizonclient.Client, error) {
	switch network {
	case models.NetworkStellarMainnet:
		return horizonclient.DefaultPublicNetClient, nil
	case models.NetworkStellarTestnet:
		return horizonclient.DefaultTestNetClient, nil
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
}

// fetchBalanceFromNetwork returns the XLM balance and funded state for an
// address — a pure Horizon query, no wallet state involved.
func fetchBalanceFromNetwork(address string, network models.Network) (string, bool, error) {
	client, err := horizonClientFor(network)
	if err != nil {
		return "0", false, err
	}
	account, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: address})
	if err != nil {
		return "0", false, nil // not found = unfunded
	}
	for _, b := range account.Balances {
		if b.Type == "native" {
			return b.Balance, true, nil
		}
	}
	return "0", true, nil
}

// accountAsset is the gatekeeper's local asset record shape (was
// wallet.AssetInfo).
type accountAsset struct {
	Code    string `json:"code"`
	Issuer  string `json:"issuer"`
	Balance string `json:"balance"`
	Type    string `json:"type"`
}

// getAccountAssets fetches all assets/trustlines for an address from Horizon.
func getAccountAssets(address string, network models.Network) ([]accountAsset, error) {
	client, err := horizonClientFor(network)
	if err != nil {
		return nil, err
	}
	account, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: address})
	if err != nil {
		return nil, nil // not found = unfunded, empty list
	}
	var assets []accountAsset
	for _, b := range account.Balances {
		if b.Type == "native" {
			assets = append(assets, accountAsset{Code: "XLM", Issuer: "Native", Balance: b.Balance, Type: "native"})
		} else {
			assets = append(assets, accountAsset{Code: b.Code, Issuer: b.Issuer, Balance: b.Balance, Type: b.Type})
		}
	}
	return assets, nil
}

// ceremonyOpener surfaces a relayed-ceremony URL to the operator: the link is
// printed to stderr (MCP protocol stays clean on stdout) and the system
// browser is asked to open it when one exists.
func ceremonyOpener(url string) error {
	fmt.Fprintf(os.Stderr, "[mcp] Open this link in your browser to continue: %s\n", url) //nolint:errcheck
	return wan.OpenBrowser(url)
}
