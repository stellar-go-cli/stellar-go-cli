package x402

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
)

func testContractID(t *testing.T) string {
	t.Helper()
	var raw [32]byte
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	encoded, err := strkey.Encode(strkey.VersionByteContract, raw[:])
	if err != nil {
		t.Fatalf("encode contract ID: %v", err)
	}
	return encoded
}

func validRequirement(t *testing.T, network string) PaymentRequirements {
	t.Helper()
	return PaymentRequirements{
		Scheme:            SchemeExact,
		Network:           network,
		Asset:             testContractID(t),
		Amount:            "1000000",
		PayTo:             keypair.MustRandom().Address(),
		MaxTimeoutSeconds: 60,
		Extra: map[string]interface{}{
			"areFeesSponsored": true,
		},
	}
}

func TestPaymentRequiredHeaderRoundTrip(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Resource:    &ResourceInfo{URL: "https://example.test/protected"},
		Accepts: []PaymentRequirements{
			validRequirement(t, NetworkStellarTestnet),
		},
	}
	data, err := json.Marshal(required)
	if err != nil {
		t.Fatalf("marshal challenge: %v", err)
	}

	decoded, err := DecodePaymentRequiredHeader(base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if decoded.X402Version != Version2 || len(decoded.Accepts) != 1 {
		t.Fatalf("unexpected decoded challenge: %+v", decoded)
	}
	if decoded.Accepts[0].Network != NetworkStellarTestnet {
		t.Fatalf("unexpected network: %s", decoded.Accepts[0].Network)
	}
}

func TestPaymentSignatureHeaderRoundTrip(t *testing.T) {
	payload := &PaymentPayload{
		X402Version: Version2,
		Payload:     map[string]interface{}{"transaction": "AAAA"},
		Accepted:    validRequirement(t, NetworkStellarPubnet),
	}
	encoded, err := EncodePaymentSignatureHeader(payload)
	if err != nil {
		t.Fatalf("encode payment signature: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode payment signature: %v", err)
	}
	var decoded PaymentPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payment payload: %v", err)
	}
	if decoded.Payload["transaction"] != "AAAA" || decoded.Accepted.Network != NetworkStellarPubnet {
		t.Fatalf("unexpected payment payload: %+v", decoded)
	}
}

func TestCanonicalNetwork(t *testing.T) {
	cases := map[string]string{
		"stellar-testnet": NetworkStellarTestnet,
		"stellar:testnet": NetworkStellarTestnet,
		"testnet":         NetworkStellarTestnet,
		"stellar-mainnet": NetworkStellarPubnet,
		"stellar:pubnet":  NetworkStellarPubnet,
		"stellar:mainnet": NetworkStellarPubnet,
		"mainnet":         NetworkStellarPubnet,
		"pubnet":          NetworkStellarPubnet,
	}
	for input, expected := range cases {
		actual, err := CanonicalNetwork(input)
		if err != nil {
			t.Fatalf("CanonicalNetwork(%q): %v", input, err)
		}
		if actual != expected {
			t.Fatalf("CanonicalNetwork(%q) = %q, expected %q", input, actual, expected)
		}
	}
	if _, err := CanonicalNetwork("base-sepolia"); err == nil {
		t.Fatal("expected unsupported network error")
	}
}

func TestSelectStellarRequirement(t *testing.T) {
	valid := validRequirement(t, NetworkStellarPubnet)
	required := &PaymentRequired{
		X402Version: Version2,
		Accepts: []PaymentRequirements{
			{Scheme: "unsupported", Network: NetworkStellarPubnet},
			validRequirement(t, NetworkStellarTestnet),
			valid,
		},
	}
	selected, err := SelectStellarRequirement(required, "stellar-mainnet")
	if err != nil {
		t.Fatalf("select requirement: %v", err)
	}
	if selected.Asset != valid.Asset || selected.PayTo != valid.PayTo {
		t.Fatal("selected the wrong payment requirement")
	}
}

func TestSelectStellarRequirementRejectsUnsupported(t *testing.T) {
	required := &PaymentRequired{
		X402Version: Version2,
		Accepts: []PaymentRequirements{
			validRequirement(t, NetworkStellarTestnet),
		},
	}
	if _, err := SelectStellarRequirement(required, NetworkStellarPubnet); err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected network rejection, got %v", err)
	}
}

func TestValidateStellarRequirementRejectsBadInputs(t *testing.T) {
	valid := validRequirement(t, NetworkStellarTestnet)

	cases := []struct {
		name string
		mut  func(*PaymentRequirements)
	}{
		{"wrong scheme", func(r *PaymentRequirements) { r.Scheme = "upto" }},
		{"wrong network", func(r *PaymentRequirements) { r.Network = "base-sepolia" }},
		{"classic native asset", func(r *PaymentRequirements) { r.Asset = "XLM" }},
		{"classic issued asset", func(r *PaymentRequirements) { r.Asset = "USDC:" + keypair.MustRandom().Address() }},
		{"invalid payTo", func(r *PaymentRequirements) { r.PayTo = "not-an-address" }},
		{"non-positive timeout", func(r *PaymentRequirements) { r.MaxTimeoutSeconds = 0 }},
		{"excessive timeout", func(r *PaymentRequirements) { r.MaxTimeoutSeconds = maxPaymentTimeoutSeconds + 1 }},
		{"decimal amount", func(r *PaymentRequirements) { r.Amount = "1.5" }},
		{"zero amount", func(r *PaymentRequirements) { r.Amount = "0" }},
		{"i128 overflow", func(r *PaymentRequirements) { r.Amount = "170141183460469231731687303715884105728" }},
		{"fees not sponsored", func(r *PaymentRequirements) { r.Extra["areFeesSponsored"] = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			req.Extra = map[string]interface{}{"areFeesSponsored": true}
			tc.mut(&req)
			if err := ValidateStellarRequirement(&req); err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
		})
	}
}

func TestParseAtomicAmount(t *testing.T) {
	amount, err := ParseAtomicAmount("170141183460469231731687303715884105727")
	if err != nil {
		t.Fatalf("parse max i128: %v", err)
	}
	if amount.BitLen() != 127 {
		t.Fatalf("unexpected bit length %d", amount.BitLen())
	}
	if _, err := ParseAtomicAmount("170141183460469231731687303715884105728"); err != nil {
		t.Fatalf("parse overflowing integer: %v", err)
	}
	if amount.Cmp(big.NewInt(0)) <= 0 {
		t.Fatal("invalid parsed amount")
	}
	for _, invalid := range []string{"", "-1", "1.0", "0x10", "1_000"} {
		if _, err := ParseAtomicAmount(invalid); err == nil {
			t.Fatalf("expected invalid amount %q", invalid)
		}
	}
}
