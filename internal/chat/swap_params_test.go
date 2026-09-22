package chat

import (
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
)

func TestNormalizeSwapParams(t *testing.T) {
	supported := supportedSwapAssets(models.NetworkStellarTestnet)

	tests := []struct {
		name       string
		from       string
		to         string
		amount     string
		wantFrom   string
		wantTo     string
		wantAmount string
	}{
		{
			name:       "verbatim conversational answers from failing session",
			from:       "xlm to usd",
			to:         "50 xlm to usd",
			amount:     "50",
			wantFrom:   "XLM",
			wantTo:     "USDC",
			wantAmount: "50",
		},
		{
			name:       "lowercase codes normalize",
			from:       "xlm",
			to:         "usdc",
			amount:     "100",
			wantFrom:   "XLM",
			wantTo:     "USDC",
			wantAmount: "100",
		},
		{
			name:       "pair in from fills missing to",
			from:       "xlm to usd",
			to:         "",
			amount:     "",
			wantFrom:   "XLM",
			wantTo:     "USDC",
			wantAmount: "",
		},
		{
			name:       "amount extracted from free text",
			from:       "usdc",
			to:         "50 usdc to xlm",
			amount:     "",
			wantFrom:   "USDC",
			wantTo:     "XLM",
			wantAmount: "50",
		},
		{
			name:       "usd alias maps to USDC",
			from:       "xlm",
			to:         "usd",
			amount:     "10",
			wantFrom:   "XLM",
			wantTo:     "USDC",
			wantAmount: "10",
		},
		{
			name:       "unknown asset left untouched for validation",
			from:       "doge",
			to:         "xlm",
			amount:     "5",
			wantFrom:   "doge",
			wantTo:     "XLM",
			wantAmount: "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, amount := normalizeSwapParams(tt.from, tt.to, tt.amount, supported)
			if from != tt.wantFrom || to != tt.wantTo || amount != tt.wantAmount {
				t.Errorf("normalizeSwapParams(%q, %q, %q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.from, tt.to, tt.amount, from, to, amount, tt.wantFrom, tt.wantTo, tt.wantAmount)
			}
		})
	}
}

func TestCanonicalSwapAsset(t *testing.T) {
	supported := supportedSwapAssets(models.NetworkStellarTestnet)

	if code, ok := canonicalSwapAsset("xlm", supported); !ok || code != "XLM" {
		t.Errorf("expected xlm -> XLM, got %q (ok=%v)", code, ok)
	}
	if code, ok := canonicalSwapAsset("USD", supported); !ok || code != "USDC" {
		t.Errorf("expected USD -> USDC, got %q (ok=%v)", code, ok)
	}
	if _, ok := canonicalSwapAsset("xlm to usd", supported); ok {
		t.Error("multi-word input must not resolve as a single asset")
	}
	if _, ok := canonicalSwapAsset("DOGE", supported); ok {
		t.Error("unknown asset must not resolve")
	}
}
