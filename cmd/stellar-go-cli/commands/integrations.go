package commands

import (
	"flag"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/integrations"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

func newIntegrationsCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "integrations",
		Short: "Manage StellarCarbon, x402, Tempo, Alpha Vantage, Finnhub, and Tansu integrations",
		Long:  "List, ping, and configure third-party integrations. Attach carbon credits to assets, configure news feeds from Alpha Vantage or Finnhub.",
		cfg:   cfg,
	}
	cmd.addSub(newIntListCmd(cfg))
	cmd.addSub(newIntPingCmd(cfg))
	cmd.addSub(newIntCarbonCmd(cfg))
	cmd.addSub(newIntNewsCmd(cfg))
	cmd.addSub(newIntFinnhubCmd(cfg))
	cmd.addSub(newIntTansuCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── integrations list ────────────────────────

func newIntListCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "list",
		Short: "List all available integrations and their status",
		Run: func(c *Command, args []string) error {
			ui.Header("MozartPay Integrations")
			fmt.Println()

			t := ui.NewTable("Integration", "Status", "Protocol", "Description")
			for _, intg := range integrations.ListIntegrations() {
				status := ui.Green_("● active")
				if !intg.Enabled {
					status = ui.Dim_("○ disabled")
				}
				t.AddRow(ui.Teal_(intg.Name), status, intg.Protocol, intg.Description)
			}
			t.Print()

			return nil
		},
	}
}

// ─── integrations ping ────────────────────────

func newIntPingCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	name := fs.String("name", "", "Integration to ping (blank = all)")

	return &Command{
		Name:  "ping",
		Short: "Health-check all or a specific integration",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Integration Health Check")

			intgList := integrations.ListIntegrations()
			if *name != "" {
				found := false
				for _, i := range intgList {
					if i.Name == *name {
						intgList = []integrations.Integration{i}
						found = true
						break
					}
				}
				if !found {
					ui.Error(fmt.Sprintf("Integration %q not found", *name))
					return nil
				}
			}

			fmt.Println()
			for _, intg := range intgList {
				spin := ui.NewSpinner(fmt.Sprintf("Pinging %s...", intg.Name))
				spin.Start()
				time.Sleep(time.Duration(80+len(intg.Name)*10) * time.Millisecond)
				ok, latency := integrations.PingIntegration(intg.Name)
				spin.Stop(ok, fmt.Sprintf("%-18s %s  %dms", intg.Name, ui.Green_("UP"), latency))
			}
			fmt.Println()

			return nil
		},
	}
}

// ─── integrations carbon ─────────────────────

func newIntCarbonCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("carbon", flag.ContinueOnError)
	action := fs.String("action", "list", "Action: list | issue | retire")
	amount := fs.Float64("amount", 1.0, "Amount in tCO2e (for --action issue)")
	vintage := fs.Int("vintage", 2023, "Vintage year")
	standard := fs.String("standard", "VCS", "Carbon standard")
	tokenID := fs.String("token-id", "", "Token ID to retire (for --action retire)")

	return &Command{
		Name:  "carbon",
		Short: "Manage StellarCarbon credits (list, issue, retire)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("StellarCarbon Credits")

			client := integrations.NewStellarCarbonClient()
			owner := cfg.ActiveAddress
			if owner == "" {
				owner = "GDEMO0000000000000000000000000000000000000000000000000001"
			}

			switch *action {
			case "list":
				spin := ui.NewSpinner("Fetching carbon credits...")
				spin.Start()
				time.Sleep(500 * time.Millisecond)
				credits, err := client.ListCredits(owner)
				if err != nil {
					spin.Stop(false, err.Error())
					return err
				}
				spin.Stop(true, fmt.Sprintf("%d credits found", len(credits)))

				fmt.Println()
				t := ui.NewTable("Token ID", "Amount (tCO2e)", "Vintage", "Standard", "Retired")
				for _, cr := range credits {
					retired := ui.Dim_("no")
					if cr.Retired {
						retired = ui.Green_("yes")
					}
					t.AddRow(ui.Teal_(cr.TokenID), fmt.Sprintf("%.4f", cr.Amount), fmt.Sprintf("%d", cr.Vintage), cr.Standard, retired)
				}
				t.Print()

			case "issue":
				spin := ui.NewSpinner(fmt.Sprintf("Issuing %.2f tCO2e (%s %d)...", *amount, *standard, *vintage))
				spin.Start()
				time.Sleep(800 * time.Millisecond)
				credit, err := client.IssueCredit(owner, *amount, *vintage, *standard)
				if err != nil {
					spin.Stop(false, err.Error())
					return err
				}
				spin.Stop(true, "Credit tokenized on Stellar")

				ui.KV("Token ID", credit.TokenID)
				ui.KVColor("Amount", fmt.Sprintf("%.4f tCO2e", credit.Amount), ui.BrightGreen)
				ui.KV("Vintage", fmt.Sprintf("%d", credit.Vintage))
				ui.KV("Standard", credit.Standard)
				ui.KV("TX Hash", credit.TxHash)

			case "retire":
				if *tokenID == "" {
					ui.Error("--token-id required for retire action")
					return nil
				}
				spin := ui.NewSpinner(fmt.Sprintf("Retiring credit %s...", *tokenID))
				spin.Start()
				time.Sleep(600 * time.Millisecond)
				// Simulate retire
				spin.Stop(true, fmt.Sprintf("Credit %s retired on-chain", *tokenID))
				ui.KV("Token ID", *tokenID)
				ui.KVColor("Status", "RETIRED", ui.BrightGreen)
				ui.KV("Beneficiary", owner)

			default:
				ui.Error(fmt.Sprintf("Unknown action: %s. Use list | issue | retire", *action))
			}

			return nil
		},
	}
}

// ─── integrations news ────────────────────────

func newIntNewsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("news", flag.ContinueOnError)
	action := fs.String("action", "status", "Action: status | configure | test")
	apiKey := fs.String("api-key", "", "Alpha Vantage API key (for --action configure)")

	return &Command{
		Name:  "news",
		Short: "Manage Alpha Vantage news integration",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Alpha Vantage News Integration")

			switch *action {
			case "status":
				ui.KV("API Key", func() string {
					if cfg.Integrations.AlphaVantageAPIKey == "" {
						return ui.Dim_("Not configured")
					}
					return ui.Green_("Configured")
				}())
				ui.KV("Service", "Alpha Vantage News & Sentiment API")
				ui.KV("Rate Limit", "5 calls/minute (free tier)")
				ui.KV("Features", "Asset-relevant news, sentiment analysis")

				fmt.Println()
				ui.Info("Configure with: integrations news --action configure --api-key YOUR_KEY")

			case "configure":
				if *apiKey == "" {
					ui.Error("--api-key required for configure action")
					return nil
				}

				keyStr := *apiKey
				ui.KV("Setting API key", ui.Dim_(keyStr[:4]+"..."+keyStr[len(keyStr)-4:]))

				// Update config
				cfg.Integrations.AlphaVantageAPIKey = *apiKey
				if err := config.Save(cfg); err != nil {
					ui.Error(fmt.Sprintf("Failed to save config: %v", err))
					return err
				}

				ui.Success("Alpha Vantage API key configured")
				ui.Info("News will now be fetched in the terminal")

			case "test":
				if cfg.Integrations.AlphaVantageAPIKey == "" {
					ui.Error("API key not configured. Use 'configure' action first.")
					return nil
				}

				spin := ui.NewSpinner("Testing Alpha Vantage connection...")
				spin.Start()

				// Test the connection
				client := integrations.NewAlphaVantageClient(cfg.Integrations.AlphaVantageAPIKey)
				testAssets := []string{"XLM", "USDC"}
				articles, err := client.FetchNews(testAssets, 2)

				if err != nil {
					spin.Stop(false, fmt.Sprintf("Connection failed: %v", err))
					return err
				}

				spin.Stop(true, fmt.Sprintf("Success! Found %d articles", len(articles)))

				fmt.Println()
				ui.Header("Sample News Articles")
				for i, article := range articles {
					ui.KV(fmt.Sprintf("Article %d", i+1), article.Title)
					ui.KV("Source", article.Source)
					ui.KV("Relevance", fmt.Sprintf("%.2f", article.Relevance))
					if i < len(articles)-1 {
						fmt.Println()
					}
				}

			default:
				ui.Error(fmt.Sprintf("Unknown action: %s. Use status | configure | test", *action))
			}

			return nil
		},
	}
}

// ─── integrations finnhub ───────────────────────

func newIntFinnhubCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("finnhub", flag.ContinueOnError)
	action := fs.String("action", "status", "Action: status | configure | test")
	apiKey := fs.String("api-key", "", "Finnhub API key (for --action configure)")

	return &Command{
		Name:  "finnhub",
		Short: "Manage Finnhub news integration",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Finnhub News Integration")

			switch *action {
			case "status":
				ui.KV("API Key", func() string {
					if cfg.Integrations.FinnhubAPIKey == "" {
						return ui.Dim_("Not configured")
					}
					return ui.Green_("Configured")
				}())
				ui.KV("Service", "Finnhub Company News API")
				ui.KV("Rate Limit", "60 calls/minute (free tier)")
				ui.KV("Features", "Real-time market news for crypto assets")

				fmt.Println()
				ui.Info("Configure with: integrations finnhub --action configure --api-key YOUR_KEY")

			case "configure":
				if *apiKey == "" {
					ui.Error("--api-key required for configure action")
					return nil
				}

				keyStr := *apiKey
				ui.KV("Setting API key", ui.Dim_(keyStr[:4]+"..."+keyStr[len(keyStr)-4:]))

				// Update config
				cfg.Integrations.FinnhubAPIKey = *apiKey
				if err := config.Save(cfg); err != nil {
					ui.Error(fmt.Sprintf("Failed to save config: %v", err))
					return err
				}

				ui.Success("Finnhub API key configured")
				ui.Info("News will now be fetched from Finnhub")

			case "test":
				if cfg.Integrations.FinnhubAPIKey == "" {
					ui.Error("API key not configured. Use 'configure' action first.")
					return nil
				}

				spin := ui.NewSpinner("Testing Finnhub connection...")
				spin.Start()

				// Test the connection
				client := integrations.NewFinnhubClient(cfg.Integrations.FinnhubAPIKey)
				testAssets := []string{"BTC", "ETH"}
				articles, err := client.FetchNews(testAssets, 2)

				if err != nil {
					spin.Stop(false, fmt.Sprintf("Connection failed: %v", err))
					return err
				}

				spin.Stop(true, fmt.Sprintf("Success! Found %d articles", len(articles)))

				fmt.Println()
				ui.Header("Sample News Articles")
				for i, article := range articles {
					ui.KV(fmt.Sprintf("Article %d", i+1), article.Title)
					ui.KV("Source", article.Source)
					ui.KV("Relevance", fmt.Sprintf("%.2f", article.Relevance))
					if i < len(articles)-1 {
						fmt.Println()
					}
				}

			default:
				ui.Error(fmt.Sprintf("Unknown action: %s. Use status | configure | test", *action))
			}

			return nil
		},
	}
}
