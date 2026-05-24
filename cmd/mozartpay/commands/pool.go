package commands

import (
	"flag"
	"fmt"
	"strings"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/pool"
	"github.com/ogtechnologies/mozartpay/internal/ui"
)

func newPoolCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "pool",
		Short: "Query Stellar liquidity pools (CAP-38 AMM)",
		Long:  "Fetch and display liquidity pool information from Stellar AMMs including reserves, fees, and prices.",
	}
	cmd.addSub(newPoolListCmd(cfg))
	cmd.addSub(newPoolInfoCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── pool list ────────────────────────────────

func newPoolListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	limit := fs.Uint("limit", 20, "Maximum number of pools to fetch (max 200)")
	asset := fs.String("asset", "", "Filter pools containing specific asset (e.g., XLM, USDC)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "list",
		Short: "List liquidity pools",
		Long:  "Fetch and display liquidity pools from Stellar AMM with optional filtering.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Liquidity Pools")

			net := models.Network(*network)
			if net == "" {
				net = models.NetworkStellarTestnet
			}

			svc := pool.NewService(net)

			ui.PrintStep(1, "Fetching Pools from Horizon")
			spin := ui.NewSpinner(fmt.Sprintf("Querying %s for liquidity pools...", net))
			spin.Start()

			var pools []models.LiquidityPool
			var err error

			if *asset != "" {
				// Normalize asset code
				assetCode := strings.ToUpper(*asset)
				pools, err = svc.GetPoolsForAsset(assetCode, *limit)
			} else {
				pools, err = svc.ListPools(*limit)
			}

			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, fmt.Sprintf("Found %d pools", len(pools)))

			if *output == "json" {
				fmt.Println(prettyJSON(pools))
				return nil
			}

			// Pretty output
			ui.SectionLabel("Results")
			if len(pools) == 0 {
				ui.Warn("No liquidity pools found")
				if *asset != "" {
					ui.Info(fmt.Sprintf("No pools found containing asset: %s", *asset))
				}
				return nil
			}

			// Display pool summary
			for i, p := range pools {
				// Format asset pair
				pair := formatPoolPair(p)
				feePercent := float64(p.FeeBP) / 100.0

				fmt.Printf("\nPool %d: %s\n", i+1, pair)
				ui.KV("Pool ID", p.ID)
				ui.KV("Fee", fmt.Sprintf("%.2f%%", feePercent))
				ui.KV("Total Shares", truncateString(p.TotalShares, 20))

				if len(p.Reserves) == 2 {
					ui.KV("Reserve A", fmt.Sprintf("%s %s", p.Reserves[0].Amount, formatAssetName(p.Reserves[0].Asset)))
					ui.KV("Reserve B", fmt.Sprintf("%s %s", p.Reserves[1].Amount, formatAssetName(p.Reserves[1].Asset)))

					// Calculate price
					reserveAAmt, _ := parseFloat(p.Reserves[0].Amount)
					reserveBAmt, _ := parseFloat(p.Reserves[1].Amount)
					if reserveAAmt > 0 {
						price := reserveBAmt / reserveAAmt
						ui.KV("Price", fmt.Sprintf("1 %s = %.7f %s",
							formatAssetName(p.Reserves[0].Asset),
							price,
							formatAssetName(p.Reserves[1].Asset)))
					}
				}

				// Explorer link
				explorerURL := svc.ExplorerURL(p.ID)
				ui.KV("Explorer", explorerURL)
				fmt.Println()
			}

			ui.Info(fmt.Sprintf("Showing %d of %d pools", len(pools), len(pools)))
			ui.Info("Use 'pool info <id>' for detailed pool information")

			return nil
		},
	}
}

// ─── pool info ────────────────────────────────

func newPoolInfoCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	poolID := fs.String("id", "", "Pool ID (hex) - can also be passed as first argument")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "info",
		Short: "Get detailed information about a specific pool",
		Long:  "Fetch and display detailed information for a liquidity pool by its ID.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			// Get pool ID from flag or argument
			id := *poolID
			if id == "" && len(args) > 0 {
				id = args[0]
			}

			if id == "" {
				return fmt.Errorf("pool ID required (use --id or pass as argument)")
			}

			ui.Header("Pool Details")

			net := models.Network(*network)
			if net == "" {
				net = models.NetworkStellarTestnet
			}

			svc := pool.NewService(net)

			ui.PrintStep(1, "Fetching Pool Information")
			spin := ui.NewSpinner(fmt.Sprintf("Querying pool %s...", truncateString(id, 16)))
			spin.Start()

			pool, err := svc.GetPool(id)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Pool found")

			if *output == "json" {
				fmt.Println(prettyJSON(pool))
				return nil
			}

			// Pretty output
			pair := formatPoolPair(*pool)
			feePercent := float64(pool.FeeBP) / 100.0

			ui.SectionLabel("Overview")
			ui.KV("Pool Pair", pair)
			ui.KV("Pool ID", pool.ID)
			ui.KV("Type", string(pool.Type))
			ui.KV("Fee", fmt.Sprintf("%.2f%% (%d basis points)", feePercent, pool.FeeBP))
			ui.KV("Total Shares", pool.TotalShares)
			ui.KV("Last Modified", pool.LastModifiedTime)

			ui.SectionLabel("Reserves")
			for _, reserve := range pool.Reserves {
				asset := formatAssetName(reserve.Asset)
				ui.KV(asset, reserve.Amount)
			}

			if len(pool.Reserves) == 2 {
				ui.SectionLabel("Prices")
				reserveAAmt, _ := parseFloat(pool.Reserves[0].Amount)
				reserveBAmt, _ := parseFloat(pool.Reserves[1].Amount)
				assetA := formatAssetName(pool.Reserves[0].Asset)
				assetB := formatAssetName(pool.Reserves[1].Asset)

				if reserveAAmt > 0 {
					priceAtoB := reserveBAmt / reserveAAmt
					ui.KV(fmt.Sprintf("1 %s", assetA), fmt.Sprintf("%.7f %s", priceAtoB, assetB))
				}
				if reserveBAmt > 0 {
					priceBtoA := reserveAAmt / reserveBAmt
					ui.KV(fmt.Sprintf("1 %s", assetB), fmt.Sprintf("%.7f %s", priceBtoA, assetA))
				}
			}

			ui.SectionLabel("Links")
			ui.KV("Stellar Expert", svc.ExplorerURL(pool.ID))

			return nil
		},
	}
}

// Helper functions

func formatPoolPair(p models.LiquidityPool) string {
	if len(p.Reserves) < 2 {
		return "Unknown"
	}
	a := formatAssetName(p.Reserves[0].Asset)
	b := formatAssetName(p.Reserves[1].Asset)
	return fmt.Sprintf("%s/%s", a, b)
}

func formatAssetName(asset string) string {
	if asset == "native" {
		return "XLM"
	}
	if strings.Contains(asset, ":") {
		parts := strings.Split(asset, ":")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return asset
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
