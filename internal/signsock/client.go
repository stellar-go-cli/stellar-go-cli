package signsock

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	wan "github.com/stellar-go-cli/stellar-go-cli/internal/webauthn"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// DefaultSocketName is the custodian socket inside the operator state dir.
const DefaultSocketName = "signer.sock"

// Client is the gatekeeper/CLI side of the custodian socket. Each call dials
// a fresh connection — cheap on Unix sockets, and keeps no state that could
// wedge.
type Client struct {
	sockPath string
	token    string
	nextID   atomic.Int64
}

// TokenEnvName lets a cross-principal client (a different OS user, a Docker
// gatekeeper without the state dir) carry the socket token via environment.
const TokenEnvName = "STELLAR_SIGNER_TOKEN" // #nosec G101 -- env var name, not a credential

// NewClient returns a client with an explicit token — for tests and callers
// that carry the token out-of-band rather than via signer.token/env.
func NewClient(sockPath, token string) *Client {
	return &Client{sockPath: sockPath, token: token}
}

// Dial returns a client for the socket path. The auth token comes from
// STELLAR_SIGNER_TOKEN or the signer.token file next to the socket — absent
// both, calls will be refused by the daemon.
func Dial(sockPath string) *Client {
	c := &Client{sockPath: sockPath, token: os.Getenv(TokenEnvName)}
	if c.token == "" {
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(sockPath), "signer.token")); err == nil { // #nosec G304 -- fixed filename next to socket
			c.token = strings.TrimSpace(string(data))
		}
	}
	return c
}

// ErrUnavailable reports a daemon that cannot be reached — callers surface
// an actionable message, they never fall back to local key access.
func ErrUnavailable(sockPath string) error {
	return fmt.Errorf("custodian daemon unreachable at %s — start it with 'stellar-go-cli signer'", sockPath)
}

// Call invokes one method. Result unmarshals into out when non-nil.
func (c *Client) Call(method string, params, out interface{}) error {
	var raw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		raw = data
	}
	req := Request{ID: c.nextID.Add(1), Method: method, Token: c.token, Params: raw}

	conn, err := net.Dial("unix", c.sockPath)
	if err != nil {
		return ErrUnavailable(c.sockPath)
	}
	defer conn.Close() //nolint:errcheck // Request-scoped connection

	if err := json.NewEncoder(conn).Encode(&req); err != nil {
		return fmt.Errorf("send %s: %w", method, err)
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return fmt.Errorf("read %s Response: %w", method, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// RemoteSigner signs through the custodian over the socket — it holds an
// address and a decision context, never key material. Implements
// signer.Signer (TxSigner + AuthEntrySigner).
type RemoteSigner struct {
	client      *Client
	addr        string
	tool        string
	decisionRef string
	mandateRef  string
	popSigner   POPSinger
	untrusted   bool
}

// POPSinger signs the holder-binding preimage — the gatekeeper's agent key.
type POPSinger interface {
	// SignPOP returns a base64 Ed25519 signature over the preimage bytes.
	SignPOP(preimage []byte) (string, error)
}

var _ signer.Signer = (*RemoteSigner)(nil)

// NewRemoteSigner builds a remote signer bound to a decision context —
// decisionRef is the gatekeeper's audit record hash (or empty for CLI use,
// where the custodian mints a local ref).
func NewRemoteSigner(client *Client, address, tool, decisionRef string) *RemoteSigner {
	return &RemoteSigner{client: client, addr: address, tool: tool, decisionRef: decisionRef}
}

// WithMandate binds a mandate reference + the agent's proof-of-possession
// signer — E2 then takes the delegated-authority path.
func (r *RemoteSigner) WithMandate(mandateRef string, pop POPSinger) *RemoteSigner {
	r.mandateRef = mandateRef
	r.popSigner = pop
	return r
}

// WithUntrusted marks request values as sourced from third-party content
// (§10.3 — the approval display flags them and the record carries it).
func (r *RemoteSigner) WithUntrusted() *RemoteSigner {
	r.untrusted = true
	return r
}

// popProof computes the holder-binding signature for a payload, when a
// mandate is bound.
func (r *RemoteSigner) popProof(payloadXDR string) (string, error) {
	if r.popSigner == nil || r.mandateRef == "" {
		return "", nil
	}
	return r.popSigner.SignPOP(AgentPOPPreimage(r.tool, r.addr, payloadXDR))
}

// Address returns the signer's account address.
func (r *RemoteSigner) Address() string { return r.addr }

// Sign sends the unsigned envelope to the custodian and returns the signed
// transaction.
func (r *RemoteSigner) Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error) {
	b64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	desc, err := DescribeEnvelopeXDR(b64, passphrase)
	if err != nil {
		return nil, err
	}
	pop, err := r.popProof(b64)
	if err != nil {
		return nil, fmt.Errorf("agent proof-of-possession: %w", err)
	}
	var res SignResult
	if err := r.client.Call(MethodSignTx, SignTxParams{
		DecisionRef: r.decisionRef,
		RequestHash: RequestHash(r.tool, r.addr, desc),
		Tool:        r.tool,
		Account:     r.addr,
		Passphrase:  passphrase,
		EnvelopeXDR: b64,
		MandateRef:  r.mandateRef,
		AgentPOP:    pop,
		Untrusted:   r.untrusted,
	}, &res); err != nil {
		return nil, err
	}
	gtx, err := txnbuild.TransactionFromXDR(res.PayloadXDR)
	if err != nil {
		return nil, fmt.Errorf("decode signed envelope: %w", err)
	}
	signed, ok := gtx.Transaction()
	if !ok {
		return nil, fmt.Errorf("custodian returned a fee-bump envelope")
	}
	return signed, nil
}

// SignAuthEntry sends the unsigned Soroban authorization entry to the
// custodian and returns the signed entry.
func (r *RemoteSigner) SignAuthEntry(
	entry xdr.SorobanAuthorizationEntry,
	passphrase string,
	validUntilLedger uint32,
) (xdr.SorobanAuthorizationEntry, error) {
	raw, err := entry.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("marshal auth entry: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(raw)
	desc, err := DescribeAuthEntryXDR(b64, passphrase)
	if err != nil {
		return entry, err
	}
	pop, err := r.popProof(b64)
	if err != nil {
		return entry, fmt.Errorf("agent proof-of-possession: %w", err)
	}
	var res SignResult
	if err := r.client.Call(MethodSignSorobanAuth, SignAuthParams{
		DecisionRef: r.decisionRef,
		RequestHash: RequestHash(r.tool, r.addr, desc),
		Tool:        r.tool,
		Account:     r.addr,
		Passphrase:  passphrase,
		EntryXDR:    b64,
		ValidUntil:  validUntilLedger,
		MandateRef:  r.mandateRef,
		AgentPOP:    pop,
		Untrusted:   r.untrusted,
	}, &res); err != nil {
		return entry, err
	}
	out, err := base64.StdEncoding.DecodeString(res.PayloadXDR)
	if err != nil {
		return entry, fmt.Errorf("decode signed entry: %w", err)
	}
	var signed xdr.SorobanAuthorizationEntry
	if err := signed.UnmarshalBinary(out); err != nil {
		return entry, fmt.Errorf("unmarshal signed entry: %w", err)
	}
	return signed, nil
}

// RemoteWallet gives the gatekeeper sanitized wallet state over the socket —
// the account structs never carry key material across.
type RemoteWallet struct {
	client *Client
}

// RemoteWallet returns a wallet-view client.
func (c *Client) RemoteWallet() *RemoteWallet { return &RemoteWallet{client: c} }

// ActiveWallet returns the sanitized active account (no key fields).
func (w *RemoteWallet) ActiveWallet() (*models.Account, error) {
	var acc models.Account
	if err := w.client.Call(MethodActiveWallet, nil, &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetWalletByAddress returns a sanitized account (no key fields).
func (w *RemoteWallet) GetWalletByAddress(address string) (*models.Account, error) {
	var acc models.Account
	if err := w.client.Call(MethodWalletGet, map[string]string{"address": address}, &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

// ListWallets returns the registry entries and the active address.
func (w *RemoteWallet) ListWallets() ([]models.WalletEntry, string, error) {
	var res struct {
		Wallets []models.WalletEntry `json:"wallets"`
		Active  string               `json:"active"`
	}
	if err := w.client.Call(MethodListWallets, nil, &res); err != nil {
		return nil, "", err
	}
	return res.Wallets, res.Active, nil
}

// Status returns the daemon-side status for a wallet.
func (w *RemoteWallet) Status(address string) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.Call(MethodStatus, map[string]string{"address": address}, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Unlock runs the passkey ceremony on THIS machine (relayed — the daemon
// never opens a browser); the grant lives in custodian memory only.
// openURL receives the ceremony URL; nil prints it to stderr.
func (w *RemoteWallet) Unlock(address string, ttlSeconds int64, openURL func(string) error) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.runCeremony(MethodUnlock, UnlockParams{Address: address, TTLSec: ttlSeconds}, openURL, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Lock drops the daemon's grant for a wallet.
func (w *RemoteWallet) Lock(address string) error {
	return w.client.Call(MethodLock, map[string]string{"address": address}, nil)
}

// RegisterPasskey runs the registration ceremony on this machine (relayed).
func (w *RemoteWallet) RegisterPasskey(address string, openURL func(string) error) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.runCeremony(MethodRegister, map[string]string{"address": address}, openURL, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// DisablePasskey runs the disable ceremony daemon-side.
func (w *RemoteWallet) DisablePasskey(address string) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.Call(MethodDisable, map[string]string{"address": address}, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Confirm runs the bound-approval ceremony on this machine (relayed, E3):
// the passkey assertion challenge commits to requestHash.
func (w *RemoteWallet) Confirm(address, requestHash string, openURL func(string) error) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.runCeremony(MethodConfirm, ConfirmParams{Account: address, RequestHash: requestHash}, openURL, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// RemoveWallet deletes a wallet record daemon-side (K12: file + keyring +
// credentials + grant).
func (w *RemoteWallet) RemoveWallet(address string) error {
	return w.client.Call(MethodRemoveWallet, map[string]interface{}{"address": address, "confirm": true}, nil)
}

// ── delegated authority (Phase 4) ─────────────────────────────────────────

// IssueMandate requests a mandate VC — the daemon runs a bound principal
// ceremony and returns the issued credential.
func (w *RemoteWallet) IssueMandate(account, agent string, scope []string, opts map[string]interface{}, openURL func(string) error) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"account": account, "agent": agent, "scope": scope,
	}
	for k, v := range opts {
		params[k] = v
	}
	var out map[string]interface{}
	if err := w.client.runCeremony(MethodIssueMandate, params, openURL, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetMandateStatus transitions a mandate: active | suspended | revoked.
func (w *RemoteWallet) SetMandateStatus(vcID, status string, openURL func(string) error) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := w.client.runCeremony(MethodSetMandateState, SetMandateStatusParams{VCID: vcID, Status: status}, openURL, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// StatusList returns the issuer's signed BitstringStatusListCredentials —
// {revocation: <vc>, suspension: <vc>}. issuerOrMandate may be an issuer DID
// or a mandate VC id (the issuer is resolved from the record).
func (w *RemoteWallet) StatusList(issuerOrMandate string) (map[string]interface{}, error) {
	p := StatusListParams{}
	if strings.HasPrefix(issuerOrMandate, "did:") {
		p.Issuer = issuerOrMandate
	} else {
		p.MandateRef = issuerOrMandate
	}
	var out map[string]interface{}
	if err := w.client.Call(MethodStatusList, p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RequestInfo returns the pending-escalation details for a request hash —
// what `wallet passkey approve` displays before the bound ceremony (§10.3).
func (w *RemoteWallet) RequestInfo(requestHash string) (*PendingRequest, error) {
	var out PendingRequest
	if err := w.client.Call(MethodRequestInfo, RequestInfoParams{RequestHash: requestHash}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AgentEnroll registers an agent DID with the custodian's lifecycle registry
// (§11.1). State starts at enrolled; mandate issuance promotes to active.
func (w *RemoteWallet) AgentEnroll(did, profile, note string) (*AgentRecord, error) {
	var out AgentRecord
	if err := w.client.Call(MethodAgentEnroll, AgentEnrollParams{DID: did, Profile: profile, Note: note}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AgentSetStatus drives a lifecycle transition: active | suspended | retired.
// Retire additionally revokes every mandate naming the agent. Activation
// (active|reactivate) grants signing authority and therefore requires a
// bound passkey ceremony on account — suspend/retire return directly.
func (w *RemoteWallet) AgentSetStatus(did, status, account string, openURL func(string) error) error {
	var out map[string]interface{}
	return w.client.runCeremony(MethodAgentSetStatus,
		AgentSetStatusParams{DID: did, Status: status, Account: account}, openURL, &out)
}

// AgentList returns the lifecycle registry for the operator inventory (§11.1).
func (w *RemoteWallet) AgentList() ([]*AgentRecord, error) {
	var out []*AgentRecord
	if err := w.client.Call(MethodAgentList, struct{}{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListMandates returns mandate metadata for an account (or all, if empty).
func (w *RemoteWallet) ListMandates(account string) ([]map[string]interface{}, error) {
	var out []map[string]interface{}
	if err := w.client.Call(MethodListMandates, ListMandatesParams{Account: account}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SignCredential asks the custodian to sign a VC for a wallet-derived issuer.
// issuerDID is the exact DID for callers that can derive it (plaintext path);
// issuerMethod ("did:key"|"did:ethr"|"did:ebsi") lets the daemon derive it —
// required for protected wallets where the caller never sees the key.
func (w *RemoteWallet) SignCredential(account, issuerDID, issuerMethod, proofType, unsignedVC string) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := w.client.Call(MethodSignCredential, SignCredentialParams{
		Account: account, IssuerDID: issuerDID, IssuerMethod: issuerMethod, ProofType: proofType, UnsignedVC: unsignedVC,
	}, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// RemoteSigner accessors — submit through the same daemon that signed.
func (r *RemoteSigner) Client() *Client { return r.client }

// Submit asks the custodian to submit an envelope it signed (submit_tx).
func (r *RemoteSigner) Submit(signedXDR string) (map[string]interface{}, error) {
	return r.client.Submit(signedXDR, r.addr)
}

// ─── relayed ceremonies ──────────────────────────────────────────────────
// The daemon builds the WebAuthn session; the browser ceremony runs HERE —
// on the client's machine — so a headless daemon still gets full passkey
// flows. runCeremony handles the whole round trip.

// ceremonyTimeout is the browser-ceremony window.
const ceremonyTimeout = 2 * time.Minute

// runCeremony calls a ceremony-bearing method, relays the browser ceremony
// locally, and returns the action result. openURL receives the ceremony URL
// (nil → stderr print only).
func (c *Client) runCeremony(method string, params interface{}, openURL func(string) error, out interface{}) error {
	relay, err := wan.StartRelay()
	if err != nil {
		return fmt.Errorf("ceremony listener: %w", err)
	}
	defer relay.Close()

	// Inject the relay origin so the daemon binds its RP to our page.
	var pm map[string]interface{}
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &pm); err != nil {
		return err
	}
	pm["origin"] = relay.Origin

	var first map[string]interface{}
	if err := c.Call(method, pm, &first); err != nil {
		return err
	}
	if cer, _ := first["ceremony"].(bool); !cer {
		// Non-ceremony path (e.g. suspend/revoke) — return the result as-is.
		if out != nil {
			data, merr := json.Marshal(first)
			if merr != nil {
				return merr
			}
			return json.Unmarshal(data, out)
		}
		return nil
	}

	options, _ := first["options_json"].(string)
	mode, _ := first["mode"].(string)
	cid, _ := first["ceremony_id"].(string)
	url := relay.Serve(mode, []byte(options))
	if openURL != nil {
		// #nosec G104 -- browser open is best-effort
		openURL(url) //nolint:errcheck // browser open is best-effort
	} else {
		// #nosec G104 -- stderr fallback is best-effort
		fmt.Fprintf(os.Stderr, "Open this link in your browser to continue: %s\n", url) //nolint:errcheck
	}
	body, err := relay.Wait(ceremonyTimeout)
	if err != nil {
		return err
	}
	return c.Call(MethodAssert, AssertParams{CeremonyID: cid, AssertionJSON: string(body)}, out)
}

// Submit sends a custodian-signed envelope to Horizon through the daemon —
// the caller never touches the network for submission.
func (c *Client) Submit(signedXDR, account string) (map[string]interface{}, error) {
	var res map[string]interface{}
	if err := c.Call(MethodSubmitTx, SubmitTxParams{SignedXDR: signedXDR, Account: account}, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// DefaultSocketPath resolves the default custodian socket path.
func DefaultSocketPath() (string, error) {
	dir, err := config.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultSocketName), nil
}
