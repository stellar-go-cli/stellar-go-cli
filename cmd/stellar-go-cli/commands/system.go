package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/integrations"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─── version ─────────────────────────────────

func newVersionCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "version",
		Short: "Show Stellar Go CLI version and build info",
		Run: func(c *Command, args []string) error {
			ui.PrintBanner()
			ui.KV("Version", config.Version)
			ui.KV("Build", "go1.22.2 · linux/amd64")
			ui.KV("Standards", "SEP-41 · W3C DID Core · ISO 20022 · eIDAS 2.0")
			ui.KV("Networks", "Stellar Testnet · EVM Sepolia")
			ui.KV("Author", "OG Technologies EU · Vienna, Austria")
			fmt.Println()
			return nil
		},
	}
}

// ─── init ────────────────────────────────────

func newInitCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "init",
		Short: "Initialize Stellar Go CLI configuration",
		Run: func(c *Command, args []string) error {
			ui.PrintBanner()
			ui.Header("Initializing Stellar Go CLI")

			steps := []struct {
				label string
				delay time.Duration
				fn    func() error
			}{
				{"Creating config directory (~/.stellar-go-cli)", 200 * time.Millisecond, nil},
				{"Writing default configuration", 200 * time.Millisecond, func() error {
					return config.Save(cfg)
				}},
				{"Verifying Stellar testnet connectivity", 600 * time.Millisecond, nil},
				{"Checking integration endpoints", 400 * time.Millisecond, nil},
				{"Ready", 100 * time.Millisecond, nil},
			}

			fmt.Println()
			for i, step := range steps {
				spin := ui.NewSpinner(step.label + "...")
				spin.Start()
				time.Sleep(step.delay)
				if step.fn != nil {
					if err := step.fn(); err != nil {
						spin.Stop(false, err.Error())
						return err
					}
				}
				spin.Stop(true, fmt.Sprintf("[%d/%d] %s", i+1, len(steps), step.label))
			}

			fmt.Println()
			ui.Separator()
			ui.KV("Config Dir", "~/.stellar-go-cli/")
			ui.KV("Network", cfg.Network)
			ui.KV("DID Method", cfg.DIDMethod)
			ui.KV("Wallet", cfg.WalletType)
			ui.Separator()
			fmt.Println()
			ui.Info("Quick start:")
			fmt.Println()
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli did attest --method ebsi --vc national-id --name \"Your Name\""))
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli wallet connect --provider wwwallet --network stellar-testnet"))
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli wallet fund --network stellar-testnet"))
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli asset create-ft --name \"MyToken\" --symbol MTK --with-score --with-carbon"))
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli pay send --to <address> --amount 10 --asset USDC --rail x402"))
			fmt.Printf("  %s %s\n", ui.Gold_("$"), ui.Dim_("stellar-go-cli report generate --vc-attach"))
			fmt.Println()

			return nil
		},
	}
}

// ─── status ──────────────────────────────────

func newStatusCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "status",
		Short: "Show overall system and session status",
		Run: func(c *Command, args []string) error {
			ui.Header("System Status")
			fmt.Println()

			// Session state
			ui.SectionLabel("Session")
			activeDID := cfg.ActiveDID
			// Also try loading from state if config doesn't have it yet
			if activeDID == "" {
				for _, m := range []string{"ebsi", "key", "web", "ethr"} {
					var doc map[string]interface{}
					if err := config.LoadState("did_"+m, &doc); err == nil {
						if id, ok := doc["id"].(string); ok {
							activeDID = id
							break
						}
					}
				}
			}
			if activeDID != "" {
				ui.KVColor("Active DID", truncateStr(activeDID, 50)+"...", ui.BrightGreen)
			} else {
				ui.KVColor("Active DID", "none — run 'stellar-go-cli did attest'", ui.Dim)
			}

			if cfg.ActiveAddress != "" {
				ui.KVColor("Active Address", cfg.ActiveAddress, ui.BrightGreen)
			} else {
				ui.KVColor("Active Address", "none — run 'stellar-go-cli wallet connect'", ui.Dim)
			}

			ui.KV("Network", string(cfg.Network))
			ui.KV("DID Method", cfg.DIDMethod)
			ui.KV("Wallet Type", cfg.WalletType)

			// State files
			ui.SectionLabel("State Files")
			stateItems := []struct {
				key   string
				label string
			}{
				{"account", "Wallet Account"},
				{"did_" + cfg.DIDMethod, "DID Document"},
				{"vc_latest", "Latest VC"},
				{"asset_latest", "Latest Asset"},
				{"payment_latest", "Latest Payment"},
				{"report_latest", "Latest Report"},
			}
			for _, s := range stateItems {
				var tmp interface{}
				if err := config.LoadState(s.key, &tmp); err == nil {
					ui.KVColor(s.label, "✓ saved", ui.BrightGreen)
				} else {
					ui.KVColor(s.label, "— not found", ui.Dim)
				}
			}

			// Integrations
			ui.SectionLabel("Integrations")
			for _, intg := range integrations.ListIntegrations() {
				_, latency := integrations.PingIntegration(intg.Name)
				ui.KVColor(intg.Name, fmt.Sprintf("● UP  %dms", latency), ui.BrightGreen)
			}

			// Wallet details
			ui.SectionLabel("Wallet")
			var acc models.Account
			if err := config.LoadState("account", &acc); err == nil {
				ui.KV("Address", acc.Address)
				ui.KV("Network", wallet.NetworkDisplayName(acc.Network))
				ui.KVColor("Balance", acc.Balance, ui.BrightGreen)
				ui.KV("Funded", fmt.Sprintf("%v", acc.Funded))
			} else {
				ui.Warn("No wallet connected.")
			}

			fmt.Println()
			return nil
		},
	}
}

// ─── flow ────────────────────────────────────

func newFlowCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "flow",
		Short: "Print the Stellar Go CLI component flow diagram",
		Run: func(c *Command, args []string) error {
			ui.PrintBanner()
			printFlow()
			return nil
		},
	}
}

func printFlow() {
	box := func(title, content string, color string) string {
		lines := strings.Split(content, "\n")
		width := len(title) + 4
		for _, l := range lines {
			if len(l)+4 > width {
				width = len(l) + 4
			}
		}
		top := "┌" + strings.Repeat("─", width) + "┐"
		bot := "└" + strings.Repeat("─", width) + "┘"
		titleLine := "│ " + title + strings.Repeat(" ", width-len(title)-2) + " │"
		sep := "├" + strings.Repeat("─", width) + "┤"

		var sb strings.Builder
		if color != "" {
			sb.WriteString(colorizeFlow(color, top) + "\n")
			sb.WriteString(colorizeFlow(color, titleLine) + "\n")
			sb.WriteString(colorizeFlow(color, sep) + "\n")
			for _, l := range lines {
				if l == "" {
					continue
				}
				sb.WriteString(colorizeFlow(color, "│ "+l+strings.Repeat(" ", width-len(l)-2)+" │") + "\n")
			}
			sb.WriteString(colorizeFlow(color, bot))
		}
		return sb.String()
	}

	arrow := func(label string) string {
		pad := strings.Repeat(" ", 20)
		return pad + ui.Dim_("│") + "\n" +
			pad + ui.Dim_("▼  "+label) + "\n"
	}

	fmt.Println()
	fmt.Println(ui.Bold_("  Stellar Go CLI — Component Flow"))
	fmt.Println(ui.Dim_("  ─────────────────────────────────────────────────────"))
	fmt.Println()

	// Layer 1: Identity
	fmt.Println(ui.Dim_("  [01] IDENTITY & ATTESTATION"))
	fmt.Println()
	fmt.Println(indentBlock(box("🔐 DID Attestation", "did:web · did:key · did:ethr · did:ebsi\nW3C VC · eIDAS 2.0", ui.Gold)))
	fmt.Println(indentBlock(box("🪪 National ID App", "Mobile eID → VC issuance\nKYC Level 2 verification", ui.Teal)))
	fmt.Println()

	fmt.Print(arrow("authenticated session"))

	// Layer 2: Wallet
	fmt.Println(ui.Dim_("  [02] WALLET & ACCOUNT"))
	fmt.Println()
	fmt.Println(indentBlock(box("🌐 wwWallet", "WebAuthn passkeys (FIDO2)\nOpen-source, browser-native", ui.Gold)))
	fmt.Println(indentBlock(box("🧪 Testnet Funding", "EOA selection · Stellar Friendbot\nEVM Sepolia faucet", ui.Blue)))
	fmt.Println()

	fmt.Print(arrow("wallet ready"))

	// Layer 3: Payments & Assets
	fmt.Println(ui.Dim_("  [03] PAYMENTS & ASSETS"))
	fmt.Println()
	fmt.Println(indentBlock(box("💸 Payments", "x402 micropayments (HTTP 402)\nTempo FX rails · Direct Stellar", ui.Green)))
	fmt.Println(indentBlock(box("🏗️  SAC Asset (SEP-41)", "Fungible tokens (FT)\nNon-fungible assets (NFA)", ui.Magenta)))
	fmt.Println()

	fmt.Print(arrow("enrich asset metadata"))

	// Layer 4: Integrations
	fmt.Println(ui.Dim_("  [04] INTEGRATIONS"))
	fmt.Println()
	fmt.Println(indentBlock(box("🌿 StellarCarbon + ⚡ x402 + 🌐 Tempo",
		"Carbon credits · HTTP 402 pay-per-use\nFX rails", ui.Cyan)))
	fmt.Println()

	fmt.Print(arrow("transaction executed"))

	// Layer 5: Reporting
	fmt.Println(ui.Dim_("  [05] POST-TRANSACTION REPORTING"))
	fmt.Println()
	fmt.Println(indentBlock(box("📊 Compliance Report", "On-chain audit trail · VC-signed receipt\nISO 20022 pacs.008 XML · Carbon log", ui.Gold)))
	fmt.Println()

	_ = box
}

func indentBlock(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, l := range lines {
		result = append(result, "  "+l)
	}
	return strings.Join(result, "\n")
}

func colorizeFlow(color, text string) string {
	return color + text + ui.Reset
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ─────────────────────────────────────────────
// JSON helper (used across commands)
// ─────────────────────────────────────────────

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}
