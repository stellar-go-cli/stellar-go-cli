package chat

import (
	"strings"
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func TestFormatWalletBalance(t *testing.T) {
	entry := &models.WalletEntry{
		Address: "GB2JSQBXCFHRPQEHMT6NZW4MFJH3ODZJW3H2IVP763DQYBJMCJVHL3XE",
		Network: models.NetworkStellarTestnet,
		Balance: "9711.2517393",
		Funded:  true,
	}

	f := NewFormatter()

	t.Run("nil wallet", func(t *testing.T) {
		if got := f.FormatWalletBalance(nil, nil); !strings.Contains(got, "No active wallet") {
			t.Errorf("expected no-wallet message, got %q", got)
		}
	})

	t.Run("xlm only when no assets", func(t *testing.T) {
		got := f.FormatWalletBalance(entry, nil)
		if !strings.Contains(got, "9711.2517393 XLM") {
			t.Errorf("expected XLM balance, got %q", got)
		}
		if strings.Contains(got, "**Assets:**") {
			t.Errorf("expected no assets section, got %q", got)
		}
	})

	t.Run("assets listed when provided", func(t *testing.T) {
		assets := []wallet.AssetInfo{
			{Code: "USDC", Balance: "12.5000000", Type: "credit_alphanum4"},
			{Code: "EURC", Balance: "3.0000000", Type: "credit_alphanum4"},
		}
		got := f.FormatWalletBalance(entry, assets)
		for _, want := range []string{"**Assets:**", "• USDC: 12.5000000", "• EURC: 3.0000000"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in output, got %q", want, got)
			}
		}
	})
}
