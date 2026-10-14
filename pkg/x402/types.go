package x402

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	Version2 = 2

	maxPaymentTimeoutSeconds = 24 * 60 * 60

	SchemeExact = "exact"

	NetworkStellarTestnet = "stellar:testnet"
	NetworkStellarPubnet  = "stellar:pubnet"

	HeaderPaymentRequired  = "PAYMENT-REQUIRED"
	HeaderPaymentSignature = "PAYMENT-SIGNATURE"
	HeaderPaymentResponse  = "PAYMENT-RESPONSE"
)

// ResourceInfo describes the protected resource advertised in an x402 v2
// PaymentRequired challenge.
type ResourceInfo struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	MimeType    string   `json:"mimeType,omitempty"`
	ServiceName string   `json:"serviceName,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IconURL     string   `json:"iconUrl,omitempty"`
}

// PaymentRequirements is one entry in the x402 v2 accepts array.
type PaymentRequirements struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	Asset             string                 `json:"asset"`
	Amount            string                 `json:"amount"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
}

// PaymentRequired is the x402 v2 HTTP 402 challenge.
type PaymentRequired struct {
	X402Version int                    `json:"x402Version"`
	Error       string                 `json:"error,omitempty"`
	Resource    *ResourceInfo          `json:"resource,omitempty"`
	Accepts     []PaymentRequirements  `json:"accepts"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// PaymentPayload is the x402 v2 object encoded in PAYMENT-SIGNATURE.
type PaymentPayload struct {
	X402Version int                    `json:"x402Version"`
	Payload     map[string]interface{} `json:"payload"`
	Accepted    PaymentRequirements    `json:"accepted"`
	Resource    *ResourceInfo          `json:"resource,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// SettlementResponse is decoded from the PAYMENT-RESPONSE header.
type SettlementResponse struct {
	Success      bool                   `json:"success"`
	ErrorReason  string                 `json:"errorReason,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Payer        string                 `json:"payer,omitempty"`
	Transaction  string                 `json:"transaction,omitempty"`
	Network      string                 `json:"network,omitempty"`
	Amount       string                 `json:"amount,omitempty"`
	Extensions   map[string]interface{} `json:"extensions,omitempty"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// DecodePaymentRequiredHeader decodes the base64-encoded v2 PAYMENT-REQUIRED
// header.
func DecodePaymentRequiredHeader(value string) (*PaymentRequired, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", HeaderPaymentRequired, err)
	}
	var required PaymentRequired
	if err := json.Unmarshal(decoded, &required); err != nil {
		return nil, fmt.Errorf("parse %s JSON: %w", HeaderPaymentRequired, err)
	}
	return &required, nil
}

// DecodePaymentResponseHeader decodes the base64-encoded PAYMENT-RESPONSE
// settlement header.
func DecodePaymentResponseHeader(value string) (*SettlementResponse, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", HeaderPaymentResponse, err)
	}
	var response SettlementResponse
	if err := json.Unmarshal(decoded, &response); err != nil {
		return nil, fmt.Errorf("parse %s JSON: %w", HeaderPaymentResponse, err)
	}
	return &response, nil
}

// DecodePaymentSignatureHeader decodes the base64-encoded v2 PAYMENT-SIGNATURE
// header sent by a paying client. Facilitators and resource servers use this
// to recover the PaymentPayload.
func DecodePaymentSignatureHeader(value string) (*PaymentPayload, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", HeaderPaymentSignature, err)
	}
	var payload PaymentPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("parse %s JSON: %w", HeaderPaymentSignature, err)
	}
	return &payload, nil
}

// EncodePaymentSignatureHeader returns the PAYMENT-SIGNATURE header value for a
// v2 PaymentPayload.
func EncodePaymentSignatureHeader(payload *PaymentPayload) (string, error) {
	if payload == nil {
		return "", fmt.Errorf("payment payload is nil")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payment payload: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// CanonicalNetwork maps CLI and CAIP-2 network names to the Stellar
// network identifiers required by the x402 Stellar exact scheme.
func CanonicalNetwork(network string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "stellar:testnet", "stellar-testnet", "testnet":
		return NetworkStellarTestnet, nil
	case "stellar:pubnet", "stellar-mainnet", "stellar:mainnet", "mainnet", "pubnet":
		return NetworkStellarPubnet, nil
	default:
		return "", fmt.Errorf("unsupported x402 network %q (expected stellar:testnet or stellar:pubnet)", network)
	}
}

// InternalNetwork maps an x402 Stellar CAIP-2 identifier back to the CLI's
// internal network name.
func InternalNetwork(network string) (string, error) {
	canonical, err := CanonicalNetwork(network)
	if err != nil {
		return "", err
	}
	if canonical == NetworkStellarPubnet {
		return "stellar-mainnet", nil
	}
	return "stellar-testnet", nil
}

// SelectStellarRequirement chooses the first usable Stellar exact-scheme
// requirement for the requested CAIP-2 network.
func SelectStellarRequirement(required *PaymentRequired, network string) (*PaymentRequirements, error) {
	if required == nil {
		return nil, fmt.Errorf("payment required response is empty")
	}
	if required.X402Version != Version2 {
		return nil, fmt.Errorf("unsupported x402 version %d (expected %d)", required.X402Version, Version2)
	}
	if len(required.Accepts) == 0 {
		return nil, fmt.Errorf("payment required response contains no accepted payment options")
	}

	canonical, err := CanonicalNetwork(network)
	if err != nil {
		return nil, err
	}

	var rejected []string
	for i := range required.Accepts {
		req := &required.Accepts[i]
		if req.Scheme != SchemeExact {
			rejected = append(rejected, fmt.Sprintf("scheme %q", req.Scheme))
			continue
		}
		if req.Network != canonical {
			rejected = append(rejected, fmt.Sprintf("network %q (need %s)", req.Network, canonical))
			continue
		}
		if err := ValidateStellarRequirement(req); err != nil {
			rejected = append(rejected, err.Error())
			continue
		}
		return req, nil
	}

	if len(rejected) == 0 {
		return nil, fmt.Errorf("no Stellar exact payment requirement found for %s", canonical)
	}
	return nil, fmt.Errorf("no usable Stellar exact payment requirement for %s: %s", canonical, strings.Join(rejected, "; "))
}

// ValidateStellarRequirement checks the Stellar-specific fields used by the
// exact scheme. The asset must be a SEP-41 token contract (C...); classic
// Stellar CODE:ISSUER assets are intentionally rejected.
func ValidateStellarRequirement(req *PaymentRequirements) error {
	if req == nil {
		return fmt.Errorf("payment requirement is empty")
	}
	if req.Scheme != SchemeExact {
		return fmt.Errorf("unsupported scheme %q", req.Scheme)
	}
	if _, err := CanonicalNetwork(req.Network); err != nil {
		return err
	}
	if _, err := soroban.ParseContractID(req.Asset); err != nil {
		return fmt.Errorf("invalid SEP-41 token contract asset %q: %w", req.Asset, err)
	}
	if req.PayTo == "" {
		return fmt.Errorf("payTo is required")
	}
	if _, err := ParseDestinationAddress(req.PayTo); err != nil {
		return fmt.Errorf("invalid payTo address %q: %w", req.PayTo, err)
	}
	if req.MaxTimeoutSeconds <= 0 || req.MaxTimeoutSeconds > maxPaymentTimeoutSeconds {
		return fmt.Errorf("maxTimeoutSeconds must be between 1 and %d", maxPaymentTimeoutSeconds)
	}
	amount, err := ParseAtomicAmount(req.Amount)
	if err != nil {
		return err
	}
	if amount.Sign() <= 0 {
		return fmt.Errorf("amount must be a positive integer")
	}
	if amount.BitLen() > 127 {
		return fmt.Errorf("amount exceeds signed i128 range")
	}
	sponsored, ok := req.Extra["areFeesSponsored"].(bool)
	if !ok || !sponsored {
		return fmt.Errorf("stellar exact payments require extra.areFeesSponsored=true")
	}
	return nil
}

// ParseAtomicAmount parses the atomic i128 amount required by the Stellar
// exact scheme.
func ParseAtomicAmount(value string) (*big.Int, error) {
	if value == "" {
		return nil, fmt.Errorf("amount is required")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("amount %q must be an integer in atomic units", value)
		}
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount %q", value)
	}
	return amount, nil
}

// ParseDestinationAddress parses a G..., M..., or C... Stellar address into
// the ScAddress used by a SEP-41 transfer. M-addresses resolve to their
// underlying Ed25519 account, matching SEP-41 address semantics.
func ParseDestinationAddress(address string) (xdr.ScAddress, error) {
	if strings.HasPrefix(address, "C") {
		return soroban.ParseContractID(address)
	}
	muxed, err := xdr.AddressToMuxedAccount(address)
	if err != nil {
		return xdr.ScAddress{}, err
	}
	accountID := muxed.ToAccountId()
	return xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &accountID,
	}, nil
}
