// Package signsock is the client half of the custodian socket: the wire
// protocol plus RemoteSigner/RemoteWallet used by the MCP gatekeeper and the
// operator CLI. It deliberately imports no wallet internals — the gatekeeper
// process must not carry a code path to key material (C1).
package signsock

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Wire protocol: one JSON-RPC request per line over a Unix socket, one JSON
// response per line. Deliberately mirrors the MCP framing style.
type Request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Token  string          `json:"token,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Error codes on the wire. Refusal codes are contract-significant: callers
// (and agents) distinguish "policy refused" from "daemon broken".
const (
	ErrBadRequest          = "bad_request"
	ErrRefused             = "refused"
	ErrEscalationRequired  = "escalation_required"
	ErrUnimplemented       = "unimplemented"
	ErrInternal            = "internal"
	ErrNoKeyMaterialPrefix = "no_key_material"
	ErrAuth                = "unauthorized"
	ErrCeremonyRequired    = "ceremony_required"
)

// Methods — the §3.1 socket API. Phase-4 credential/mandate calls return
// ErrUnimplemented so their absence is explicit, not silent.
const (
	MethodSignTx          = "sign_tx"
	MethodSignSorobanAuth = "sign_soroban_auth"
	MethodPubkey          = "pubkey"
	MethodActiveWallet    = "active_wallet"
	MethodWalletGet       = "wallet_get"
	MethodListWallets     = "list_wallets"
	MethodUnlock          = "unlock"
	MethodLock            = "lock"
	MethodStatus          = "status"
	MethodRegister        = "passkey_register"
	MethodDisable         = "passkey_disable"
	MethodConfirm         = "confirm"
	MethodRemoveWallet    = "remove_wallet"
	MethodSignCredential  = "sign_credential" // #nosec G101 -- method-name constant, not a credential
	MethodIssueMandate    = "issue_mandate"
	MethodSetMandateState = "set_mandate_status"
	MethodListMandates    = "list_mandates"
	MethodStatusList      = "status_list"
	MethodSubmitTx        = "submit_tx"
	MethodAssert          = "assert"
	MethodRequestInfo     = "request_info"
	MethodAgentEnroll     = "agent_enroll"
	MethodAgentSetStatus  = "agent_set_status"
	MethodAgentList       = "agent_list"
)

// signTxParams is the sign_tx request. EnvelopeXDR is base64 of the unsigned
// transaction envelope.
type SignTxParams struct {
	DecisionRef string `json:"decision_ref"`
	RequestHash string `json:"request_hash"`
	Tool        string `json:"tool"`
	Account     string `json:"account"`
	Passphrase  string `json:"passphrase"`
	EnvelopeXDR string `json:"envelope_xdr"`
	// MandateRef selects a mandate VC by id; AgentPOP is the holder-binding
	// proof — an Ed25519 signature over agentPOPPreimage by the agent's
	// did:key. Both empty = grant/policy authority path.
	MandateRef string `json:"mandate_ref,omitempty"`
	AgentPOP   string `json:"agent_pop,omitempty"`
	// Untrusted marks request values sourced from third-party content (e.g.,
	// a merchant's x402 challenge). Metadata only — not bound into
	// RequestHash — so the approval display can flag it (§10.3).
	Untrusted bool `json:"untrusted,omitempty"`
}

// signAuthParams is the sign_soroban_auth request.
type SignAuthParams struct {
	DecisionRef string `json:"decision_ref"`
	RequestHash string `json:"request_hash"`
	Tool        string `json:"tool"`
	Account     string `json:"account"`
	Passphrase  string `json:"passphrase"`
	EntryXDR    string `json:"entry_xdr"`
	ValidUntil  uint32 `json:"valid_until_ledger"`
	MandateRef  string `json:"mandate_ref,omitempty"`
	AgentPOP    string `json:"agent_pop,omitempty"`
	Untrusted   bool   `json:"untrusted,omitempty"`
}

type SignResult struct {
	PayloadXDR string `json:"payload_xdr"` // base64 signed envelope / entry
}

type ConfirmParams struct {
	Account     string `json:"account"`
	RequestHash string `json:"request_hash"` // hex; becomes the ceremony challenge
	Origin      string `json:"origin"`
}

type UnlockParams struct {
	Address string `json:"address"`
	TTLSec  int64  `json:"ttl_seconds,omitempty"`
	Origin  string `json:"origin"`
}

// issueMandateParams is the issue_mandate request. The daemon assembles the
// credentialSubject, runs a bound principal ceremony, and issues the VC.
type IssueMandateParams struct {
	Account        string            `json:"account"`
	Agent          string            `json:"agent"` // agent did:key (holder)
	Scope          []string          `json:"scope"`
	PerTx          map[string]string `json:"per_tx,omitempty"`
	Cumulative     map[string]string `json:"cumulative,omitempty"`
	Counterparties []string          `json:"counterparties,omitempty"`
	Deny           []string          `json:"deny,omitempty"`
	Networks       []string          `json:"networks,omitempty"`
	ValidUntil     string            `json:"valid_until"`              // RFC3339
	PayloadDigest  string            `json:"payload_digest,omitempty"` // closed mandate
	Origin         string            `json:"origin"`
}

type SetMandateStatusParams struct {
	VCID   string `json:"vc_id"`
	Status string `json:"status"` // active | suspended | revoked
	Origin string `json:"origin"`
}

type StatusListParams struct {
	Issuer     string `json:"issuer"`      // issuer DID (empty → infer from mandate_ref)
	MandateRef string `json:"mandate_ref"` // resolve issuer from a mandate
}

type ListMandatesParams struct {
	Account string `json:"account,omitempty"`
}

// signCredentialParams asks the custodian to sign a VC — the daemon derives
// the proof key from the wallet's seed and refuses foreign issuers.
type SignCredentialParams struct {
	DecisionRef  string `json:"decision_ref,omitempty"`
	Account      string `json:"account"`
	IssuerDID    string `json:"issuer_did,omitempty"`    // exact DID — verified against wallet-derived candidates
	IssuerMethod string `json:"issuer_method,omitempty"` // did:key | did:ethr | did:ebsi — daemon derives the DID (for callers that can't derive the key, e.g. protected wallets)
	ProofType    string `json:"proof_type,omitempty"`    // ed25519 | jws
	UnsignedVC   string `json:"unsigned_vc_json"`        // VC document without proof
	MandateRef   string `json:"mandate_ref,omitempty"`
	AgentPOP     string `json:"agent_pop,omitempty"`
}

// submitTxParams asks the custodian to submit a signed envelope it issued.
type SubmitTxParams struct {
	Account   string `json:"account,omitempty"`
	SignedXDR string `json:"signed_xdr"`
}

// assertParams delivers the browser's ceremony response to finish a pending
// challenge.
type AssertParams struct {
	CeremonyID    string `json:"ceremony_id"`
	AssertionJSON string `json:"assertion_json"` // raw credential response body
}

// agentPOPPreimage is the bytes an agent signs to prove possession of the
// mandate's holder key: sha256(tool | NUL | account | NUL | payloadXDR).
// The gatekeeper computes it without decoding; the custodian recomputes it
// and verifies the signature against the mandate's agent did:key.
func AgentPOPPreimage(tool, account, payloadXDR string) []byte {
	h := sha256.New()
	h.Write([]byte(tool))
	h.Write([]byte{0})
	h.Write([]byte(account))
	h.Write([]byte{0})
	h.Write([]byte(payloadXDR))
	return h.Sum(nil)
}

// opDesc is the canonical description of one decoded operation for the
// request digest and E1 limit checks.
type OpDesc struct {
	Type        string `json:"type"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Asset       string `json:"asset,omitempty"`
	Amount      string `json:"amount,omitempty"`
}

// payloadDesc is the canonical description of a decoded signing payload —
// what the custodian actually verified (E4) and what limits apply to (E1).
type PayloadDesc struct {
	Source  string   `json:"source,omitempty"`
	Network string   `json:"network,omitempty"`
	Kind    string   `json:"kind"` // "tx" | "soroban_auth"
	Ops     []OpDesc `json:"ops"`
	Args    []string `json:"args,omitempty"` // canonical args for invocations
}

// describeEnvelopeXDR decodes a transaction envelope XDR and produces the
// canonical description. Payloads that cannot be fully decoded fail — no
// blind signing (E4).
func DescribeEnvelopeXDR(b64 string, passphrase string) (*PayloadDesc, error) {
	gtx, err := txnbuild.TransactionFromXDR(b64)
	if err != nil {
		return nil, fmt.Errorf("undecodable transaction envelope: %w", err)
	}
	tx, ok := gtx.Transaction()
	if !ok {
		return nil, fmt.Errorf("fee-bump envelopes are not supported by the custodian")
	}
	// Note: the passphrase is not encoded in the envelope — it binds only
	// through the signature hash. The custodian signs under the declared
	// passphrase and applies the network allowlist to it.

	desc := &PayloadDesc{
		Source:  tx.SourceAccount().AccountID,
		Network: passphrase,
		Kind:    "tx",
	}
	for _, op := range tx.Operations() {
		od := OpDesc{
			Type:   opTypeName(op),
			Source: op.GetSourceAccount(),
		}
		if od.Source == "" {
			od.Source = desc.Source
		}
		describeOpValue(op, &od)
		desc.Ops = append(desc.Ops, od)
	}
	return desc, nil
}

// describeAuthEntryXDR decodes a Soroban authorization entry and produces the
// canonical description — contract, function, and canonicalized args (the
// x402 validation shape, applied custodian-side).
func DescribeAuthEntryXDR(b64 string, passphrase string) (*PayloadDesc, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("bad entry encoding: %w", err)
	}
	var entry xdr.SorobanAuthorizationEntry
	if err := entry.UnmarshalBinary(raw); err != nil {
		return nil, fmt.Errorf("undecodable authorization entry: %w", err)
	}

	desc := &PayloadDesc{Kind: "soroban_auth", Network: passphrase}
	if cred := authCredentials(&entry); cred != nil {
		addr, err := cred.Address.String()
		if err != nil {
			return nil, fmt.Errorf("undecodable authorization address: %w", err)
		}
		desc.Source = addr
	}
	op := OpDesc{Type: "invoke", Source: desc.Source}
	inv := entry.RootInvocation
	switch inv.Function.Type {
	case xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn:
		cf := inv.Function.ContractFn
		if cf == nil {
			return nil, fmt.Errorf("authorization entry has no contract invocation")
		}
		addr, err := cf.ContractAddress.String()
		if err != nil {
			return nil, fmt.Errorf("undecodable contract address: %w", err)
		}
		op.Destination = addr
		op.Type = "invoke_contract:" + string(cf.FunctionName)
		for _, arg := range cf.Args {
			desc.Args = append(desc.Args, canonicalScVal(arg))
		}
	default:
		op.Type = "host_fn:" + inv.Function.Type.String()
	}
	desc.Ops = []OpDesc{op}
	return desc, nil
}

func authCredentials(entry *xdr.SorobanAuthorizationEntry) *xdr.SorobanAddressCredentials {
	switch entry.Credentials.Type {
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddress:
		return entry.Credentials.Address
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2:
		return entry.Credentials.AddressV2
	default:
		return nil
	}
}

// describeOpValue extracts destination/asset/amount from value-moving ops.
// Ops whose value semantics are not extracted carry only type+source — E1
// caps then can't apply, and policy decides (allow_unclassified_ops).
func describeOpValue(op txnbuild.Operation, od *OpDesc) {
	switch o := op.(type) {
	case *txnbuild.Payment:
		od.Destination = o.Destination
		od.Asset = canonicalAsset(o.Asset)
		od.Amount = o.Amount
	case *txnbuild.PathPaymentStrictSend:
		od.Destination = o.Destination
		od.Asset = canonicalAsset(o.SendAsset)
		od.Amount = o.SendAmount
	case *txnbuild.PathPaymentStrictReceive:
		od.Destination = o.Destination
		od.Asset = canonicalAsset(o.SendAsset)
		od.Amount = o.SendMax
	case *txnbuild.AccountMerge:
		od.Destination = o.Destination
		od.Asset = "native"
	case *txnbuild.ManageSellOffer:
		od.Asset = canonicalAsset(o.Selling)
		od.Amount = o.Amount
	case *txnbuild.ManageBuyOffer:
		od.Asset = canonicalAsset(o.Buying)
		od.Amount = o.Amount
	case *txnbuild.CreatePassiveSellOffer:
		od.Asset = canonicalAsset(o.Selling)
		od.Amount = o.Amount
	case *txnbuild.ChangeTrust:
		if xa, err := o.Line.ToXDR(); err == nil {
			a := xa.ToAsset()
			od.Asset = a.StringCanonical()
		} else {
			od.Asset = "unknown-trustline"
		}
		od.Amount = o.Limit
	case *txnbuild.CreateClaimableBalance:
		od.Asset = canonicalAsset(o.Asset)
		od.Amount = o.Amount
	}
}

// canonicalAsset renders an asset in canonical "native"/"CODE:ISSUER" form so
// policy keys and digests are unambiguous.
func canonicalAsset(a txnbuild.Asset) string {
	if a == nil {
		return ""
	}
	xa, err := a.ToXDR()
	if err != nil {
		return a.GetCode() + ":" + a.GetIssuer()
	}
	return xa.StringCanonical()
}

func opTypeName(op txnbuild.Operation) string {
	name := fmt.Sprintf("%T", op)
	return strings.TrimPrefix(name, "*txnbuild.")
}

// canonicalScVal renders an ScVal deterministically for digests/policy.
func canonicalScVal(v xdr.ScVal) string {
	switch v.Type {
	case xdr.ScValTypeScvAddress:
		if v.Address != nil {
			if s, err := v.Address.String(); err == nil {
				return "addr:" + s
			}
		}
	case xdr.ScValTypeScvSymbol:
		if v.Sym != nil {
			return "sym:" + string(*v.Sym)
		}
	case xdr.ScValTypeScvString:
		if v.Str != nil {
			return "str:" + string(*v.Str)
		}
	case xdr.ScValTypeScvI128:
		if v.I128 != nil {
			return "i128:" + i128String(*v.I128)
		}
	case xdr.ScValTypeScvU128:
		if v.U128 != nil {
			return "u128:" + u128String(*v.U128)
		}
	case xdr.ScValTypeScvI64:
		if v.I64 != nil {
			return fmt.Sprintf("i64:%d", *v.I64)
		}
	case xdr.ScValTypeScvU64:
		if v.U64 != nil {
			return fmt.Sprintf("u64:%d", *v.U64)
		}
	case xdr.ScValTypeScvI32:
		if v.I32 != nil {
			return fmt.Sprintf("i32:%d", *v.I32)
		}
	case xdr.ScValTypeScvU32:
		if v.U32 != nil {
			return fmt.Sprintf("u32:%d", *v.U32)
		}
	case xdr.ScValTypeScvBool:
		if v.B != nil {
			return fmt.Sprintf("bool:%t", *v.B)
		}
	case xdr.ScValTypeScvBytes:
		if v.Bytes != nil {
			return "bytes:" + hex.EncodeToString(*v.Bytes)
		}
	case xdr.ScValTypeScvVec:
		if v.Vec != nil && *v.Vec != nil {
			parts := make([]string, 0, len(**v.Vec))
			for _, e := range **v.Vec {
				parts = append(parts, canonicalScVal(e))
			}
			return "vec:[" + strings.Join(parts, ",") + "]"
		}
	case xdr.ScValTypeScvMap:
		if v.Map != nil && *v.Map != nil {
			parts := make([]string, 0, len(**v.Map))
			for _, e := range **v.Map {
				parts = append(parts, canonicalScVal(e.Key)+"="+canonicalScVal(e.Val))
			}
			return "map:{" + strings.Join(parts, ",") + "}"
		}
	}
	return v.Type.String()
}

func i128String(v xdr.Int128Parts) string {
	i := new(big.Int).SetInt64(int64(v.Hi))
	i.Lsh(i, 64)
	i.Or(i, new(big.Int).SetUint64(uint64(v.Lo)))
	return i.String()
}

func u128String(v xdr.UInt128Parts) string {
	i := new(big.Int).SetUint64(uint64(v.Hi))
	i.Lsh(i, 64)
	i.Or(i, new(big.Int).SetUint64(uint64(v.Lo)))
	return i.String()
}

// RequestHash computes the binding between a decided request and the exact
// payload — sha256 over tool, account, and the canonical decoded description.
// The gatekeeper computes it over intent; the custodian recomputes it over
// the decoded payload (E4) and approvals commit to it (E3).
func RequestHash(tool, account string, desc *PayloadDesc) string {
	canonical, err := json.Marshal(desc)
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(tool))
	h.Write([]byte{0})
	h.Write([]byte(account))
	h.Write([]byte{0})
	h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil))
}

// DecodeAddress validates a G... address for params. C... contract addresses
// are only valid as counterparts, never as signers.
func DecodeAddress(addr string) error {
	if !strkey.IsValidEd25519PublicKey(addr) {
		return fmt.Errorf("invalid Stellar address: %q", addr)
	}
	return nil
}

// RequestInfoParams queries the custodian's pending-request map for a hash.
type RequestInfoParams struct {
	RequestHash string `json:"request_hash"`
}

// PendingRequest describes one E3-escalated request awaiting operator action.
type PendingRequest struct {
	RequestHash string `json:"request_hash"`
	Tool        string `json:"tool"`
	Account     string `json:"account"`
	Description string `json:"description"`
	Untrusted   bool   `json:"untrusted,omitempty"`
	Timestamp   string `json:"ts"`
}

// AgentEnrollParams registers an agent identity with the custodian (§11.1).
type AgentEnrollParams struct {
	DID     string `json:"did"`
	Profile string `json:"profile,omitempty"`
	Note    string `json:"note,omitempty"`
}

// AgentSetStatusParams drives a lifecycle transition.
type AgentSetStatusParams struct {
	DID     string `json:"did"`
	Status  string `json:"status"`            // active | suspended | retired
	Account string `json:"account,omitempty"` // ceremony wallet — required for →active
	Origin  string `json:"origin,omitempty"`  // ceremony client origin — required for →active
}

// AgentRecord is one registry entry (§11.1 inventory).
type AgentRecord struct {
	DID        string `json:"did"`
	State      string `json:"state"` // enrolled | active | suspended | retired
	Profile    string `json:"profile,omitempty"`
	Note       string `json:"note,omitempty"`
	EnrolledAt string `json:"enrolled_at"`
	UpdatedAt  string `json:"updated_at"`
}
