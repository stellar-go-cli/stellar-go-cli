package commands

import (
	"flag"
	"fmt"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/triangular"
	"github.com/ogtechnologies/mozartpay/internal/ui"
)

func newTriangularCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "triangular",
		Short: "Triangular arbitrage with 3-leg cycles",
		Long:  "Scan for triangular arbitrage opportunities (XLM→USDC→yXLM→XLM) with statistical analysis",
		cfg:   cfg,
	}

	cmd.addSub(newTriangularScanCmd(cfg))
	cmd.addSub(newTriangularMonitorCmd(cfg))
	cmd.addSub(newTriangularBacktestCmd(cfg))

	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── triangular scan ───────────────────────────

func newTriangularScanCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	amount := fs.String("amount", "10", "Starting XLM amount")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "scan",
		Short: "Scan for triangular arbitrage opportunities",
		Long:  "Performs one-time scan of all triangular arbitrage cycles (3-leg paths)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Triangular Arbitrage Scanner")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Test Amount", *amount+" XLM")
			fmt.Println()

			svc := triangular.NewService(net)

			ui.PrintStep(1, "Scanning triangular cycles")
			results, err := svc.FindTriangularPaths([]string{"USDC", "yXLM"}, *amount)
			if err != nil {
				ui.Error(fmt.Sprintf("Scan failed: %v", err))
				return err
			}

			// Record and analyze each result
			for i := range results {
				if err := svc.RecordAndAnalyze(&results[i]); err != nil {
					// Log but don't fail
					fmt.Printf("Warning: Failed to record result %d: %v\n", i+1, err)
				}
			}

			if *output == "json" {
				fmt.Println(prettyJSON(results))
				return nil
			}

			// Display results
			fmt.Println(triangular.DisplayResults(results))

			// Summary
			opportunities := 0
			for _, r := range results {
				if r.Path.IsOpportunity || r.IsMeanReversion {
					opportunities++
				}
			}

			// Show historical analysis if available
			if len(results) > 0 {
				ui.SectionLabel("Historical Analysis")
				for _, r := range results {
					if r.ZScore != 0 {
						ui.KV(r.Path.Name+" Z-Score", fmt.Sprintf("%.2f", r.ZScore))
						ui.KV(r.Path.Name+" Mean", fmt.Sprintf("%.6f", r.HistoricalMean))
						ui.KV(r.Path.Name+" Volatility", fmt.Sprintf("%.4f%%", r.Volatility*100))
					}
				}
				fmt.Println()
			}

			ui.SectionLabel("Summary")
			ui.KV("Cycles Scanned", fmt.Sprintf("%d", len(results)))
			ui.KVColor("Opportunities Found", fmt.Sprintf("%d", opportunities), ui.BrightGreen)

			if opportunities == 0 {
				ui.Info("No profitable triangular arbitrage detected")
				ui.Info("Try monitoring continuously to catch fleeting opportunities")
			}

			return nil
		},
	}
}

// ─── triangular monitor ───────────────────────

func newTriangularMonitorCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	interval := fs.Duration("interval", 30, "Scan interval in seconds")
	minProfit := fs.Float64("min-profit", 0.01, "Minimum profit threshold in percent")

	return &Command{
		Name:  "monitor",
		Short: "Continuously monitor triangular arbitrage",
		Long:  "Runs continuous triangular arbitrage monitoring with configurable interval",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Triangular Arbitrage Monitor")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Interval", fmt.Sprintf("%v", *interval))
			ui.KV("Min Profit", fmt.Sprintf("%.2f%%", *minProfit))
			fmt.Println()

			svc := triangular.NewService(net)

			ui.Info("Starting continuous monitoring...")
			ui.Info("Press Ctrl+C to stop\n")

			// Simple loop (will add proper monitor service in Phase 3)
			for {
				results, err := svc.FindTriangularPaths([]string{"USDC", "yXLM"}, "")
				if err != nil {
					ui.Error(fmt.Sprintf("Scan error: %v", err))
					continue
				}

				for _, r := range results {
					if r.ProfitPercent > *minProfit {
						ui.KVColor("OPPORTUNITY", fmt.Sprintf("%s: %.4f%%", r.Path.Name, r.ProfitPercent), ui.BrightGreen)
					}
				}

				// Sleep for interval
				// time.Sleep(*interval) // Simplified for now
				break // Single iteration for testing
			}

			return nil
		},
	}
}

// ─── triangular backtest ──────────────────────

func newTriangularBacktestCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("backtest", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	hours := fs.Int("hours", 168, "Hours of historical data to fetch (default: 168 = 7 days)")
	threshold := fs.Float64("threshold", 2.0, "Z-score threshold for trading signals")
	useRealData := fs.Bool("real", true, "Use real Horizon trade data (not simulated)")

	return &Command{
		Name:  "backtest",
		Short: "Backtest triangular arbitrage strategy",
		Long:  "Runs historical backtest using real Stellar DEX trade data from Horizon API",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Triangular Arbitrage Backtest")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Data Source", map[bool]string{true: "Horizon (Real)", false: "Simulated"}[*useRealData])
			ui.KV("Hours of Data", fmt.Sprintf("%d", *hours))
			ui.KV("Z-Score Threshold", fmt.Sprintf("%.1f", *threshold))
			fmt.Println()

			paths := []string{
				"XLM→USDC→yXLM→XLM",
				"XLM→yXLM→USDC→XLM",
			}

			if *useRealData {
				// Use real Horizon data
				engine := triangular.NewRealDataBacktestEngine(net)

				ui.PrintStep(1, "Fetching real trade data from Stellar Horizon")
				fmt.Println()

				for _, path := range paths {
					result, err := engine.RunBacktestWithRealData(path, *hours, *threshold)
					if err != nil {
						ui.Warn(fmt.Sprintf("Skipped %s: %v", path, err))
						continue
					}
					fmt.Println(triangular.DisplayRealBacktest(result))
				}
			} else {
				// Fallback to simulated data
				history, err := triangular.NewHistoryStore()
				if err != nil {
					ui.Error(fmt.Sprintf("Failed to open history: %v", err))
					return err
				}
				defer history.Close()

				engine := triangular.NewBacktestEngine(history)

				ui.PrintStep(1, "Running backtest simulation (legacy)")

				for _, path := range paths {
					result, err := engine.RunBacktest(path, *hours/24, *threshold)
					if err != nil {
						ui.Warn(fmt.Sprintf("Skipped %s: %v", path, err))
						continue
					}
					fmt.Println(triangular.DisplayBacktest(result))
				}
			}

			return nil
		},
	}
}
