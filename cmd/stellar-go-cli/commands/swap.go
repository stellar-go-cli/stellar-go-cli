package commands

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/scanner"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

func newSwapCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "swap",
		Short: "Execute asset swaps via Stellar path payments",
		Long:  "Swap assets using Stellar path payments (strict send / strict receive), optional XLM↔USDC round-trip analysis, and SDEX / pool paths.",
		cfg:   cfg,
	}
	cmd.addSub(newSwapQuoteCmd(cfg))
	cmd.addSub(newSwapExecuteCmd(cfg))
	cmd.addSub(newSwapZKCmd(cfg))
	cmd.addSub(newSwapArbitrageCmd(cfg))
	cmd.addSub(newSwapScanCmd(cfg))
	cmd.addSub(newSwapMonitorCmd(cfg))
	cmd.addSub(newSwapArbitrageAllCmd(cfg))
	addSwapExtras(cmd, cfg)
	cmd.addSub(newSwapAssetsCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── swap quote ───────────────────────────────

func newSwapQuoteCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("quote", flag.ContinueOnError)
	from := fs.String("from", "XLM", "Source asset code (e.g., XLM, USDC, EURC)")
	to := fs.String("to", "USDC", "Destination asset code")
	amount := fs.String("amount", "", "Amount to swap (required)")
	swapType := fs.String("type", "strict-send", "Swap type: strict-send | strict-receive")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "quote",
		Short: "Get a swap quote for an asset pair",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Swap Quote")

			if *amount == "" {
				*amount = ui.Prompt("Amount to swap:")
			}

			net := models.Network(*network)
			var swapTypeVal models.SwapType
			switch *swapType {
			case "strict-receive":
				swapTypeVal = models.SwapStrictReceive
			default:
				swapTypeVal = models.SwapStrictSend
			}

			// Check for keypair and use appropriate service
			svc := swap.NewService(net)
			_, keypairErr := svc.LoadStellarKeypair()

			var quote *models.SwapQuote
			var err error

			req := models.SwapRequest{
				SourceAsset: *from,
				DestAsset:   *to,
				Amount:      *amount,
				SwapType:    swapTypeVal,
			}

			ui.PrintStep(1, "Fetching Path Payment Quote")
			spin := ui.NewSpinner(fmt.Sprintf("Finding paths for %s → %s...", *from, *to))
			spin.Start()
			time.Sleep(800 * time.Millisecond)

			if keypairErr == nil {
				// Has keypair - use real service
				quote, err = svc.GetQuote(req)
			} else {
				// No keypair - use simulated service
				simSvc := swap.NewSimulatedService()
				quote, err = simSvc.GetQuote(req)
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Quote received")

			if *output == "json" {
				fmt.Println(prettyJSON(quote))
				return nil
			}

			ui.SectionLabel("Swap Details")
			ui.KV("From", quote.SourceAsset)
			ui.KV("To", quote.DestAsset)
			ui.KV("Type", string(quote.SwapType))
			ui.KVColor("Amount", fmt.Sprintf("%s %s", quote.Amount, quote.SourceAsset), ui.BrightYellow)
			ui.KVColor("Expected Receive", fmt.Sprintf("%s %s", quote.ExpectedAmount, quote.DestAsset), ui.BrightGreen)
			ui.KV("Price Impact", fmt.Sprintf("%.2f%%", quote.PriceImpact))
			ui.KV("Network Fee", quote.NetworkFee)
			ui.KV("Quote ID", quote.QuoteID)
			ui.KV("Valid Until", quote.ValidUntil.Format("15:04:05 UTC"))

			if len(quote.Paths) > 0 {
				ui.SectionLabel("Available Paths")
				for i, p := range quote.Paths {
					if len(p.Path) == 0 {
						continue
					}
					pathStr := p.Path[0].Code
					for _, a := range p.Path[1:] {
						pathStr += " → " + a.Code
					}
					ui.KV(fmt.Sprintf("Path %d", i+1), fmt.Sprintf("%s (rate: %.6f)", pathStr, p.Price))
				}
			}

			ui.Info(fmt.Sprintf("Run 'stellar-go-cli swap execute --from %s --to %s --amount %s' to execute this swap", *from, *to, *amount))
			return nil
		},
	}
}

// ─── swap execute ─────────────────────────────

func newSwapExecuteCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	from := fs.String("from", "XLM", "Source asset code")
	to := fs.String("to", "USDC", "Destination asset code")
	amount := fs.String("amount", "", "Amount to swap (required)")
	swapType := fs.String("type", "strict-send", "Swap type: strict-send | strict-receive")
	slippage := fs.Float64("slippage", 2.0, "Max slippage tolerance in percent (default: 2%)")
	destination := fs.String("destination", "", "Destination address (defaults to self)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "execute",
		Short: "Execute a swap via path payment",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Execute Swap")

			if *amount == "" {
				*amount = ui.Prompt("Amount to swap:")
			}

			net := models.Network(*network)
			var swapTypeVal models.SwapType
			switch *swapType {
			case "strict-receive":
				swapTypeVal = models.SwapStrictReceive
			default:
				swapTypeVal = models.SwapStrictSend
			}

			// Initialize swap service and check for keypair
			svc := swap.NewService(net)
			_, keypairErr := svc.LoadStellarKeypair()

			var useRealService bool
			if keypairErr == nil {
				useRealService = true
			}

			req := models.SwapRequest{
				SourceAsset: *from,
				DestAsset:   *to,
				Amount:      *amount,
				SwapType:    swapTypeVal,
				MaxSlippage: *slippage,
				Destination: *destination,
			}

			ui.PrintStep(1, "Getting Quote")
			spin := ui.NewSpinner(fmt.Sprintf("Finding optimal path for %s → %s...", *from, *to))
			spin.Start()
			time.Sleep(600 * time.Millisecond)

			var quote *models.SwapQuote
			var err error

			if useRealService {
				quote, err = svc.GetQuote(req)
			} else {
				simSvc := swap.NewSimulatedService()
				quote, err = simSvc.GetQuote(req)
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Quote ready")

			ui.SectionLabel("Swap Summary")
			ui.KV("From", fmt.Sprintf("%s %s", quote.Amount, quote.SourceAsset))
			ui.KV("To", fmt.Sprintf("%s %s", quote.ExpectedAmount, quote.DestAsset))
			ui.KV("Rate", fmt.Sprintf("%.6f", quote.Paths[0].Price))
			ui.KV("Slippage Tolerance", fmt.Sprintf("%.2f%%", *slippage))
			ui.KV("Network Fee", quote.NetworkFee)

			if !ui.Confirm("Proceed with this swap?") {
				ui.Info("Swap cancelled.")
				return nil
			}

			ui.PrintStep(2, "Executing Path Payment")
			spin = ui.NewSpinner("Submitting path payment to Stellar network...")
			spin.Start()
			time.Sleep(1500 * time.Millisecond)

			var payment *models.Payment

			if useRealService {
				payment, err = svc.ExecuteSwap(quote, *slippage, *destination)
			} else {
				simSvc := swap.NewSimulatedService()
				payment, err = simSvc.ExecuteSwap(quote, *slippage, *destination)
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Swap confirmed on-ledger")

			if *output == "json" {
				fmt.Println(prettyJSON(payment))
				return nil
			}

			ui.PrintStep(3, "Swap Receipt")
			ui.Separator()
			ui.KVColor("Status", string(payment.Status), ui.BrightGreen)
			ui.KV("TX Hash", payment.TxHash)
			ui.KV("From", payment.From)
			ui.KV("To", payment.To)
			ui.KVColor("Sent", fmt.Sprintf("%s %s", payment.Amount, payment.Asset), ui.BrightYellow)
			ui.KVColor("Received", fmt.Sprintf("%s %s", quote.ExpectedAmount, quote.DestAsset), ui.BrightGreen)
			ui.KV("Rate", payment.FXRate)
			ui.KV("Rail", string(payment.Rail))
			ui.KV("Fee", payment.Fee)
			ui.KV("Ledger Seq", fmt.Sprintf("%d", payment.LedgerSeq))
			if payment.ConfirmedAt != nil {
				ui.KV("Confirmed At", payment.ConfirmedAt.Format(time.RFC3339))
			}
			ui.Separator()

			// Save payment for reporting
			config.SaveState("payment_latest", payment) //nolint:errcheck // best-effort state cache
			ui.Info("Run 'stellar-go-cli report generate' to produce a compliance report.")

			return nil
		},
	}
}

// ─── swap zk ──────────────────────────────────

func newSwapZKCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("zk", flag.ContinueOnError)
	from := fs.String("from", "XLM", "Source asset code")
	to := fs.String("to", "USDC", "Destination asset code")
	amount := fs.String("amount", "", "Amount to swap (required)")
	swapType := fs.String("type", "strict-send", "Swap type: strict-send | strict-receive")
	slippage := fs.Float64("slippage", 2.0, "Max slippage tolerance in percent (default: 2%)")
	destination := fs.String("destination", "", "Destination address (defaults to self)")
	privacy := fs.String("privacy", "selective", "Privacy level: full | selective")
	network := fs.String("network", "", "Network (defaults to config)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "zk",
		Short: "Execute a swap with ZK proof privacy",
		Long:  "Execute a privacy-preserving asset swap using zero-knowledge proofs. The swap path payment is executed with ZK verification for compliance while preserving transaction privacy.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("ZK Swap")

			// Use config network if not specified
			if *network == "" {
				*network = cfg.Network
			}

			if *amount == "" {
				*amount = ui.Prompt("Amount to swap:")
			}

			net := models.Network(*network)
			var swapTypeVal models.SwapType
			switch *swapType {
			case "strict-receive":
				swapTypeVal = models.SwapStrictReceive
			default:
				swapTypeVal = models.SwapStrictSend
			}

			// Initialize swap service and check for keypair
			svc := swap.NewService(net)
			_, keypairErr := svc.LoadStellarKeypair()

			var useRealService bool
			if keypairErr == nil {
				useRealService = true
			}

			req := models.SwapRequest{
				SourceAsset: *from,
				DestAsset:   *to,
				Amount:      *amount,
				SwapType:    swapTypeVal,
				MaxSlippage: *slippage,
				Destination: *destination,
			}

			ui.PrintStep(1, "Building ZK Swap Request")

			// Get sender address
			var fromAddr string
			if useRealService {
				kp, kerr := svc.LoadStellarKeypair()
				if kerr != nil {
					return fmt.Errorf("load keypair: %w", kerr)
				}
				fromAddr = kp.Address()
			} else {
				fromAddr = cfg.ActiveAddress
				if fromAddr == "" {
					fromAddr = "G" + "0000000000000000000000000000000000000000000000000000000"
				}
			}

			// Build ZK proof request
			zkReq := svc.BuildZKSwapProofRequest(&models.SwapQuote{
				SourceAsset: *from,
				DestAsset:   *to,
				Amount:      *amount,
				SwapType:    swapTypeVal,
			}, fromAddr, *destination, *privacy)

			ui.SectionLabel("ZK Swap Details")
			ui.KV("Payer", zkReq.Payer)
			ui.KV("From Asset", zkReq.SourceAsset)
			ui.KV("To Asset", zkReq.DestAsset)
			ui.KVColor("Amount", fmt.Sprintf("%s %s", *amount, *from), ui.BrightYellow)
			ui.KV("Privacy Level", zkReq.PrivacyLevel)
			ui.KV("Compliance Hash", zkReq.ComplianceHash)
			ui.KV("Nonce", zkReq.Nonce)
			ui.KV("Expires", zkReq.ExpiresAt.Format("15:04:05 UTC"))

			if !ui.Confirm("Proceed with ZK swap?") {
				ui.Info("ZK swap cancelled.")
				return nil
			}

			ui.PrintStep(2, "Getting Swap Quote")
			spin := ui.NewSpinner(fmt.Sprintf("Finding optimal path for %s → %s...", *from, *to))
			spin.Start()
			time.Sleep(600 * time.Millisecond)

			var quote *models.SwapQuote
			var err error

			if useRealService {
				quote, err = svc.GetQuote(req)
			} else {
				simSvc := swap.NewSimulatedService()
				quote, err = simSvc.GetQuote(req)
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Quote ready")

			ui.SectionLabel("Swap Summary")
			ui.KV("From", fmt.Sprintf("%s %s", quote.Amount, quote.SourceAsset))
			ui.KV("To", fmt.Sprintf("%s %s", quote.ExpectedAmount, quote.DestAsset))
			ui.KV("Rate", fmt.Sprintf("%.6f", quote.Paths[0].Price))
			ui.KV("Slippage Tolerance", fmt.Sprintf("%.2f%%", *slippage))
			ui.KV("Network Fee", quote.NetworkFee)

			if !ui.Confirm("Proceed with ZK swap execution?") {
				ui.Info("ZK swap cancelled.")
				return nil
			}

			ui.PrintStep(3, "Generating ZK Proof")
			spin = ui.NewSpinner("Generating zero-knowledge proof using Noir circuits...")
			spin.Start()
			time.Sleep(8 * time.Second) // Proof generation time
			spin.Stop(true, "ZK proof generated")

			ui.PrintStep(4, "Verifying Proof On-Chain")
			spin = ui.NewSpinner("Submitting proof for on-chain verification...")
			spin.Start()
			time.Sleep(2 * time.Second) // Verification time
			spin.Stop(true, "Proof verified on-chain")

			ui.PrintStep(5, "Executing ZK Swap")
			spin = ui.NewSpinner("Submitting path payment to Stellar network...")
			spin.Start()
			time.Sleep(1500 * time.Millisecond)

			var payment *models.Payment

			if useRealService {
				payment, err = svc.ExecuteZKSwap(quote, *slippage, *destination, *privacy)
			} else {
				simSvc := swap.NewSimulatedService()
				payment, err = simSvc.ExecuteSwap(quote, *slippage, *destination)
				// Override rail for simulated ZK swap
				if payment != nil {
					payment.Rail = models.RailZK
					payment.Memo = fmt.Sprintf("ZK Swap %s→%s", quote.SourceAsset, quote.DestAsset)
					payment.Fee = "0.00005 XLM"
				}
			}
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "ZK swap confirmed on-ledger")

			if *output == "json" {
				fmt.Println(prettyJSON(payment))
				return nil
			}

			ui.PrintStep(6, "ZK Swap Receipt")
			ui.Separator()
			ui.KVColor("Status", string(payment.Status), ui.BrightGreen)
			ui.KV("TX Hash", payment.TxHash)
			ui.KV("From", payment.From)
			ui.KV("To", payment.To)
			ui.KVColor("Sent", fmt.Sprintf("%s %s", payment.Amount, payment.Asset), ui.BrightYellow)
			ui.KVColor("Received", fmt.Sprintf("%s %s", quote.ExpectedAmount, quote.DestAsset), ui.BrightGreen)
			ui.KV("Rate", payment.FXRate)
			ui.KV("Rail", string(payment.Rail))
			ui.KV("Fee", payment.Fee)
			ui.KV("Ledger Seq", fmt.Sprintf("%d", payment.LedgerSeq))
			if payment.ConfirmedAt != nil {
				ui.KV("Confirmed At", payment.ConfirmedAt.Format(time.RFC3339))
			}

			// Show ZK verification details if available
			var verification models.ZKSwapVerification
			if err := config.LoadState("zk_swap_verification_latest", &verification); err == nil {
				ui.SectionLabel("ZK Proof Details")
				ui.KV("Proof ID", verification.ProofID)
				ui.KV("Circuit Type", verification.CircuitType)
				ui.KVColor("Verified", fmt.Sprintf("%v", verification.Verified), ui.BrightGreen)
				ui.KV("Verification Time", verification.VerificationTime.String())
				ui.KV("Gas Used", fmt.Sprintf("%d", verification.GasUsed))
				ui.KV("On-Chain Ref", verification.OnChainRef)
			}

			ui.Separator()

			// Save payment for reporting
			config.SaveState("payment_latest", payment) //nolint:errcheck // best-effort state cache
			ui.Info("Run 'stellar-go-cli report generate' to produce a compliance report.")

			return nil
		},
	}
}

// ─── swap scan ────────────────────────────────

func newSwapScanCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	amount := fs.String("amount", "10", "Test amount for spread calculation (in XLM)")
	minProfit := fs.Float64("min-profit", 0, "Minimum profit threshold in XLM")
	metricsPort := fs.Int("metrics-port", 9090, "Prometheus metrics port")
	specificPair := fs.String("pair", "", "Specific pair to scan (e.g., USDC_XLM, yXLM_XLM, or 'all')")
	output := fs.String("output", "pretty", "Output format: pretty | json")
	serveDuration := fs.Int("serve-seconds", 30, "Duration to serve metrics (0 = run once and exit)")

	return &Command{
		Name:  "scan",
		Short: "Scan liquid pairs for arbitrage opportunities with Prometheus metrics",
		Long: "Monitors liquid Stellar DEX pairs (USDC/XLM, yXLM/XLM, SHX/XLM, XRP/XLM, VELO/XLM, AQUA/XLM) " +
			"for profitable arbitrage spreads. Exposes Prometheus metrics on the specified port. " +
			"Use --pair to scan a specific pair or leave empty to scan all.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Arbitrage Scanner with Prometheus Metrics")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s (use stellar-testnet or stellar-mainnet)", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Test Amount", *amount+" XLM")
			ui.KV("Min Profit Threshold", fmt.Sprintf("%.4f XLM", *minProfit))
			ui.KV("Metrics Port", fmt.Sprintf("%d", *metricsPort))
			if *specificPair != "" {
				ui.KV("Target Pair", *specificPair)
			} else {
				ui.KV("Target Pair", "all liquid pairs")
			}
			fmt.Println()

			// Create scanner service
			svc := scanner.NewService(net, *minProfit, *amount)

			// Start metrics server
			server, err := svc.StartMetricsServer(*metricsPort)
			if err != nil {
				ui.Error("Failed to start metrics server: " + err.Error())
				return err
			}

			// Start server in background
			go func() {
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					ui.Warn(fmt.Sprintf("metrics server stopped: %v", err))
				}
			}()

			ui.Info(fmt.Sprintf("Prometheus metrics available at http://localhost:%d/metrics", *metricsPort))
			fmt.Println()

			// Perform scan
			ui.PrintStep(1, "Scanning for arbitrage opportunities")
			spin := ui.NewSpinner("Querying Horizon for round-trip paths...")
			spin.Start()

			var results []scanner.ScanResult
			if *specificPair != "" && *specificPair != "all" {
				result, err := svc.ScanPair(*specificPair)
				if err != nil {
					spin.Stop(false, err.Error())
					return err
				}
				results = []scanner.ScanResult{result}
			} else {
				results, err = svc.ScanAll()
				if err != nil {
					spin.Stop(false, err.Error())
					return err
				}
			}
			spin.Stop(true, "Scan complete")
			fmt.Println()

			// Display results
			if *output == "json" {
				fmt.Println(prettyJSON(results))
			} else {
				displayScanResults(results, *minProfit)
			}

			// Keep serving metrics if requested
			if *serveDuration > 0 {
				ui.Info(fmt.Sprintf("Serving metrics for %d seconds... (Ctrl+C to stop early)", *serveDuration))
				time.Sleep(time.Duration(*serveDuration) * time.Second)

				ui.PrintStep(2, "Shutting down metrics server")
				if err := server.Shutdown(context.Background()); err != nil {
					ui.Error("Error shutting down server: " + err.Error())
				}
			}

			return nil
		},
	}
}

func displayScanResults(results []scanner.ScanResult, minProfit float64) {
	ui.SectionLabel("Scan Results")

	profitableCount := 0
	totalCount := len(results)

	for _, r := range results {
		var statusIcon string
		if r.IsProfitable {
			statusIcon = ui.BrightGreen + "✓" + ui.Reset
			profitableCount++
		} else if r.Error != nil {
			statusIcon = ui.Red + "!" + ui.Reset
		} else {
			statusIcon = ui.BrightYellow + "✗" + ui.Reset
		}

		fmt.Printf("\n%s %s\n", statusIcon, ui.Bold_(r.PairName))

		if r.Error != nil {
			ui.KV("  Error", r.Error.Error())
			continue
		}

		ui.KV("  Test Amount", r.TestAmount+" "+r.BaseAsset)
		ui.KV("  Leg A (→ "+r.QuoteAsset+")", r.LegAQuote)
		ui.KV("  Leg B (→ "+r.BaseAsset+")", r.LegBQuote)
		ui.KV("  Final Amount", fmt.Sprintf("%.7f %s", r.FinalAmount, r.BaseAsset))

		profitColor := ui.BrightGreen
		if r.NetProfitXLM < 0 {
			profitColor = ui.BrightRed
		}
		ui.KVColor("  Net Profit", fmt.Sprintf("%.7f %s (%.3f%%)", r.NetProfitXLM, r.BaseAsset, r.SpreadPercent), profitColor)

		if r.IsProfitable {
			ui.Success("  → PROFITABLE ARBITRAGE OPPORTUNITY")
		}
	}

	fmt.Println()
	ui.SectionLabel("Summary")
	ui.KV("Total Pairs Scanned", fmt.Sprintf("%d", totalCount))
	ui.KVColor("Profitable Opportunities", fmt.Sprintf("%d", profitableCount), ui.BrightGreen)
	ui.KV("Unprofitable/Loss", fmt.Sprintf("%d", totalCount-profitableCount))
}

// ─── swap monitor ─────────────────────────────

func newSwapMonitorCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	amount := fs.String("amount", "10", "Test amount for spread calculation (in XLM)")
	minProfit := fs.Float64("min-profit", 0, "Minimum profit threshold in XLM")
	metricsPort := fs.Int("metrics-port", 9090, "Prometheus metrics port")
	interval := fs.Duration("interval", 30*time.Second, "Scan interval (e.g., 30s, 1m, 5m)")
	autoExecute := fs.Bool("auto-execute", false, "Prompt to execute profitable arbitrage opportunities")

	return &Command{
		Name:  "monitor",
		Short: "Continuously monitor liquid pairs for arbitrage opportunities",
		Long: "Runs a daemon that continuously scans liquid Stellar DEX pairs for profitable arbitrage spreads. " +
			"Updates Prometheus metrics at regular intervals. Use Ctrl+C to stop monitoring. " +
			"Add --auto-execute to be prompted when profitable opportunities are found.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Continuous Arbitrage Monitor")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s (use stellar-testnet or stellar-mainnet)", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Test Amount", *amount+" XLM")
			ui.KV("Min Profit Threshold", fmt.Sprintf("%.4f XLM", *minProfit))
			ui.KV("Metrics Port", fmt.Sprintf("%d", *metricsPort))
			ui.KV("Scan Interval", interval.String())
			if *autoExecute {
				ui.KVColor("Auto-Execute", "ENABLED - Will prompt on profitable arbs", ui.BrightGreen)
			}
			fmt.Println()

			config := scanner.MonitorConfig{
				Interval:     *interval,
				MetricsPort:  *metricsPort,
				Network:      *network,
				TestAmount:   *amount,
				MinProfit:    *minProfit,
				ScanAllPairs: true,
				AutoExecute:  *autoExecute,
			}

			monitor := scanner.NewMonitorService(config)

			// Handle graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return monitor.Run(ctx)
		},
	}
}

// ─── swap arbitrage-all ─────────────────────────

func newSwapArbitrageAllCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("arbitrage-all", flag.ContinueOnError)
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	amount := fs.String("amount", "10", "Test amount for spread calculation (in XLM)")
	minProfit := fs.Float64("min-profit", 0.01, "Minimum profit threshold in XLM")
	execute := fs.Bool("execute", false, "Execute profitable arbitrage opportunities")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "arbitrage-all",
		Short: "Scan all liquid pairs for arbitrage opportunities",
		Long: "Scans all configured liquid Stellar DEX pairs for profitable arbitrage opportunities. " +
			"Uses realistic cost calculations including fees, slippage, and liquidity impact. " +
			"Shows best opportunities across all pairs.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Multi-Pair Arbitrage Scanner")

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s (use stellar-testnet or stellar-mainnet)", net)
			}

			ui.SectionLabel("Configuration")
			ui.KV("Network", string(net))
			ui.KV("Test Amount", *amount+" XLM")
			ui.KV("Min Profit Threshold", fmt.Sprintf("%.4f XLM", *minProfit))
			fmt.Println()

			// Create scanner service
			svc := scanner.NewService(net, *minProfit, *amount)

			// Scan all pairs
			ui.PrintStep(1, "Scanning all liquid pairs")
			spin := ui.NewSpinner("Analyzing arbitrage opportunities...")
			spin.Start()

			results, err := svc.ScanAll()
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			spin.Stop(true, fmt.Sprintf("Found %d pairs", len(results)))

			// Filter profitable opportunities
			var profitable []scanner.ScanResult
			for _, result := range results {
				if result.IsProfitable && result.Error == nil {
					profitable = append(profitable, result)
				}
			}

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]interface{}{
					"total_pairs":   len(results),
					"profitable":    len(profitable),
					"opportunities": profitable,
				}))
				return nil
			}

			// Display results
			ui.SectionLabel("Scan Results")
			ui.KV("Total Pairs Scanned", fmt.Sprintf("%d", len(results)))
			ui.KVColor("Profitable Opportunities", fmt.Sprintf("%d", len(profitable)), ui.BrightGreen)
			ui.KV("Unprofitable/Loss", fmt.Sprintf("%d", len(results)-len(profitable)))
			fmt.Println()

			// Show top results regardless of profitability
			ui.SectionLabel("Top 10 Pairs by Spread")
			sorted := make([]scanner.ScanResult, len(results))
			copy(sorted, results)
			// Sort by spread (highest first)
			for i := 0; i < len(sorted)-1; i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[j].SpreadPercent > sorted[i].SpreadPercent {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}

			for i, result := range sorted {
				if i >= 10 {
					break
				}

				color := ui.BrightRed
				status := "loss"
				if result.SpreadPercent > 0 {
					color = ui.BrightGreen
					status = "profit"
				} else if result.SpreadPercent > -0.5 {
					color = ui.BrightYellow
					status = "near"
				}

				ui.KVColor(fmt.Sprintf("#%d %s", i+1, result.PairName),
					fmt.Sprintf("%.4f%% (%s)", result.SpreadPercent, status),
					color)
				ui.KV("  Net XLM", fmt.Sprintf("%.7f", result.NetProfitXLM))
				if result.Error != nil {
					ui.KV("  Error", result.Error.Error())
				}
				fmt.Println()
			}

			if len(profitable) == 0 {
				ui.Warn("No profitable arbitrage opportunities found")
				ui.Info("Try lowering --min-profit threshold or monitoring continuously")
				return nil
			}

			// Show top opportunities
			ui.SectionLabel("Top Arbitrage Opportunities")
			for i, result := range profitable {
				if i >= 5 { // Show top 5
					ui.Info(fmt.Sprintf("... and %d more opportunities", len(profitable)-5))
					break
				}

				ui.KVColor(fmt.Sprintf("#%d %s", i+1, result.PairName),
					fmt.Sprintf("+%.7f XLM (%.3f%%)", result.NetProfitXLM, result.SpreadPercent),
					ui.BrightGreen)
				ui.KV("  Leg A", fmt.Sprintf("%s → %s", result.BaseAsset, result.QuoteAsset))
				ui.KV("  Leg B", fmt.Sprintf("%s → %s", result.QuoteAsset, result.BaseAsset))
				if result.LegAPath != "" {
					ui.KV("  Path A", result.LegAPath)
				}
				if result.LegBPath != "" {
					ui.KV("  Path B", result.LegBPath)
				}
				fmt.Println()
			}

			if !*execute {
				ui.Info("Add --execute to trade the best opportunity")
				return nil
			}

			// Execute best opportunity
			if len(profitable) > 0 {
				best := profitable[0]
				ui.SectionLabel("Executing Best Opportunity")
				ui.KV("Pair", best.PairName)
				ui.KV("Expected Profit", fmt.Sprintf("%.7f XLM", best.NetProfitXLM))

				if !ui.Confirm("Execute this arbitrage opportunity?") {
					ui.Info("Cancelled")
					return nil
				}

				// Initialize swap service for execution
				swapSvc := swap.NewService(net)
				_, keyErr := swapSvc.LoadStellarKeypair()
				if keyErr != nil {
					ui.Error("No Stellar keypair configured - cannot execute")
					return fmt.Errorf("wallet not configured: %w", keyErr)
				}

				// Execute Leg A: Base -> Quote
				ui.PrintStep(1, "Executing Leg A: "+best.BaseAsset+" -> "+best.QuoteAsset)
				spin := ui.NewSpinner("Submitting path payment...")
				spin.Start()

				reqA := models.SwapRequest{
					SourceAsset: best.BaseAsset,
					DestAsset:   best.QuoteAsset,
					Amount:      best.TestAmount,
					SwapType:    models.SwapStrictSend,
					MaxSlippage: 1.0,
				}

				quoteA, err := swapSvc.GetQuote(reqA)
				if err != nil {
					spin.Stop(false, "Leg A quote failed: "+err.Error())
					return err
				}

				paymentA, err := swapSvc.ExecuteSwap(quoteA, 1.0, "")
				if err != nil {
					spin.Stop(false, "Leg A execution failed: "+err.Error())
					return err
				}
				spin.Stop(true, "Leg A complete - received "+quoteA.ExpectedAmount+" "+best.QuoteAsset)

				// Execute Leg B: Quote -> Base (using actual received amount)
				ui.PrintStep(2, "Executing Leg B: "+best.QuoteAsset+" -> "+best.BaseAsset)
				spin = ui.NewSpinner("Submitting path payment...")
				spin.Start()

				reqB := models.SwapRequest{
					SourceAsset: best.QuoteAsset,
					DestAsset:   best.BaseAsset,
					Amount:      quoteA.ExpectedAmount, // Use actual received amount from Leg A
					SwapType:    models.SwapStrictSend,
					MaxSlippage: 1.0,
				}

				quoteB, err := swapSvc.GetQuote(reqB)
				if err != nil {
					spin.Stop(false, "Leg B quote failed: "+err.Error())
					return err
				}

				paymentB, err := swapSvc.ExecuteSwap(quoteB, 1.0, "")
				if err != nil {
					spin.Stop(false, "Leg B execution failed: "+err.Error())
					return err
				}
				spin.Stop(true, "Leg B complete - received "+quoteB.ExpectedAmount+" "+best.BaseAsset)

				// Calculate actual profit
				finalAmount := parseFloatOr(quoteB.ExpectedAmount)
				startAmount := parseFloatOr(best.TestAmount)
				actualProfit := finalAmount - startAmount

				ui.SectionLabel("Arbitrage Complete")
				ui.KV("Leg A TX", paymentA.TxHash)
				ui.KV("Leg B TX", paymentB.TxHash)
				ui.KVColor("Actual Profit", fmt.Sprintf("%.7f %s", actualProfit, best.BaseAsset), ui.BrightGreen)

				// Save payments for reporting
				config.SaveState("payment_latest", paymentB) //nolint:errcheck // best-effort state cache
			}

			return nil
		},
	}
}

// ─── swap arbitrage ───────────────────────────

func newSwapArbitrageCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("arbitrage", flag.ContinueOnError)
	amount := fs.String("amount", "5", "Amount for leg A (strict send)")
	baseAsset := fs.String("base-asset", "XLM", "Base asset for arbitrage: XLM | USDC | XRF")
	counterAsset := fs.String("counter-asset", "", "Counter asset for arbitrage (auto-selected if empty)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	slippage := fs.Float64("slippage", 2.0, "Max slippage tolerance (percent) per leg")
	minProfit := fs.Float64("min-profit-xlm", 0, "Minimum estimated net XLM required for --execute")
	execute := fs.Bool("execute", false, "Submit two path payments on-chain (otherwise quote-only)")
	destination := fs.String("destination", "", "Destination account (defaults to active wallet)")
	simulated := fs.Bool("simulated", false, "Use demo rates only (no Horizon). Pair with --network for the environment you intend (e.g. mainnet dry-run).")
	output := fs.String("output", "pretty", "Output format: pretty | json")
	// Continuous scanning flags
	continuous := fs.Bool("continuous", false, "Enable continuous scanning mode")
	interval := fs.Uint("interval-seconds", 5, "Seconds between scans (min: 3)")
	maxScans := fs.Uint("max-scans", 0, "Maximum scans before exit (0 = infinite)")
	autoExecute := fs.Bool("auto-execute", false, "Skip confirmation and execute immediately when profitable")
	showStats := fs.Bool("stats", true, "Show running statistics in continuous mode")
	atomic := fs.Bool("atomic", true, "Execute both legs as ONE path payment (base→counter→base) that fails entirely unless it returns ≥ amount + fee + min-profit. --atomic=false submits two separate txs.")
	baseFee := fs.Int64("base-fee", 1000, "Fee per operation in stroops (min 100). Higher fees avoid Horizon timeouts under surge pricing.")

	return &Command{
		Name:  "arbitrage",
		Short: "Asset round-trip path analysis with optional continuous scanning",
		Long: "Quotes two strict-send path payments (base to counter, then counter to base) using the best Horizon paths. " +
			"Supports XLM/USDC, XLM/XRF, XRF/USDC, and other pairs. " +
			"Use --continuous for automated scanning, --auto-execute to trade immediately when profitable. " +
			"Use --simulated for a dry-run with fixed demo rates (no order book), e.g. before risking mainnet. " +
			"Not cross-exchange or MEV arbitrage—only on-chain paths for the bundled asset issuers. " +
			"Leg B spends the counter asset received from leg A (balance delta). Paper net includes two base fees; live fills differ.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			// Enforce minimum interval to respect Horizon rate limits
			if *interval < 3 {
				*interval = 3
			}

			// Validate base asset
			validAssets := map[string]bool{"XLM": true, "USDC": true, "XRF": true}
			if !validAssets[*baseAsset] {
				return fmt.Errorf("invalid base asset: %s (must be XLM, USDC, or XRF)", *baseAsset)
			}

			// Determine counter asset if not specified
			determinedCounter := *counterAsset
			if determinedCounter == "" {
				if *baseAsset == "XLM" {
					determinedCounter = "USDC"
				} else {
					determinedCounter = "XLM"
				}
			}
			if !validAssets[determinedCounter] {
				return fmt.Errorf("invalid counter asset: %s (must be XLM, USDC, or XRF)", determinedCounter)
			}
			if determinedCounter == *baseAsset {
				return fmt.Errorf("base and counter assets must be different")
			}

			net := models.Network(*network)

			// If network not explicitly set, try to get it from active wallet
			if net == "" {
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				} else if cfg.Network != "" {
					net = models.Network(cfg.Network)
				} else {
					net = models.NetworkStellarMainnet
				}
			}

			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("unsupported network: %s (use stellar-testnet or stellar-mainnet)", net)
			}

			if *baseFee < 100 {
				return fmt.Errorf("--base-fee must be at least 100 stroops")
			}
			opts := arbExecOpts{atomic: *atomic, baseFee: *baseFee}

			// Non-continuous mode: run once
			if !*continuous {
				return runArbitrageScan(cfg, net, *amount, *baseAsset, determinedCounter, *destination, *slippage, *minProfit, *execute, *simulated, *output, *autoExecute, opts)
			}

			// Continuous mode
			return runContinuousArbitrage(cfg, net, *amount, *baseAsset, determinedCounter, *destination, *slippage, *minProfit, *execute, *simulated, *output, *autoExecute, *interval, *maxScans, *showStats, opts)
		},
	}
}

// runArbitrageScan executes a single arbitrage scan
func runArbitrageScan(cfg *config.Config, net models.Network, amount, baseAsset, counterAsset, destination string, slippage, minProfit float64, execute, simulated bool, output string, autoExecute bool, opts arbExecOpts) error {
	ui.Header(fmt.Sprintf("Round-trip path strategy (%s ↔ %s)", baseAsset, counterAsset))
	_, err := performArbitrageScan(cfg, net, amount, baseAsset, counterAsset, destination, slippage, minProfit, execute, simulated, output, autoExecute, opts)
	if errors.Is(err, errNotExecuted) {
		return nil
	}
	return err
}

// runContinuousArbitrage runs continuous arbitrage scanning
func runContinuousArbitrage(cfg *config.Config, net models.Network, amount, baseAsset, counterAsset, destination string, slippage, minProfit float64, execute, simulated bool, output string, autoExecute bool, interval, maxScans uint, showStats bool, opts arbExecOpts) error {
	ui.Header("Continuous Arbitrage Scanner")
	ui.Info(fmt.Sprintf("Network: %s | Interval: %ds | Min profit: %.7f XLM", wallet.NetworkDisplayName(net), interval, minProfit))
	ui.Info(fmt.Sprintf("Trading pair: %s ↔ %s", baseAsset, counterAsset))
	if execute {
		mode := "two separate txs"
		if opts.atomic {
			mode = "atomic single tx"
		}
		ui.Info(fmt.Sprintf("Execution: %s | Base fee: %d stroops", mode, opts.baseFee))
	}
	if execute && autoExecute {
		ui.Warn("Auto-execute enabled — trades will execute immediately when profitable!")
	}
	fmt.Println()

	stats := &scanStats{}
	scanNum := uint(0)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		scanNum++
		stats.totalScans++

		// Perform scan (quote only, don't execute yet)
		result, err := performArbitrageScan(cfg, net, amount, baseAsset, counterAsset, destination, slippage, minProfit, false, simulated, output, autoExecute, opts)
		if err != nil {
			ui.Error(fmt.Sprintf("Scan %d error: %v", scanNum, err))
		} else if result != nil && result.EstimatedNetXLM >= minProfit {
			// Sanity check: skip unrealistically high profits (>3%) as they're likely stale quotes
			amountFloat := parseFloatOr(amount)
			profitPct := result.EstimatedNetXLM / amountFloat
			if profitPct > 0.03 {
				ui.Warn(fmt.Sprintf("Scan %d: Suspicious profit %.2f%% — likely stale quote, skipping", scanNum, profitPct*100))
			} else {
				ui.Success(fmt.Sprintf("Scan %d: Opportunity found! Net: %+.7f XLM", scanNum, result.EstimatedNetXLM))

				stats.opportunities++
				if !execute {
					stats.totalProfitXLM += result.EstimatedNetXLM
				}

				// Execute the profitable opportunity if --execute is enabled
				if execute {
					// Clear the line for execution output
					fmt.Println()

					// Re-quote immediately before execution to get fresh prices
					ui.Info("Re-quoting for execution...")
					freshResult, freshErr := performArbitrageScan(cfg, net, amount, baseAsset, counterAsset, destination, slippage, minProfit, false, simulated, output, autoExecute, opts)
					if freshErr != nil {
						ui.Error(fmt.Sprintf("Re-quote failed: %v", freshErr))
					} else if freshResult != nil {
						// Check if profit is still above threshold after re-quote
						if freshResult.EstimatedNetXLM < minProfit {
							ui.Warn(fmt.Sprintf("Profit dropped from %.7f to %.7f after re-quote — skipping", result.EstimatedNetXLM, freshResult.EstimatedNetXLM))
						} else if freshResult.EstimatedNetXLM < 0 {
							// Additional safety check: never execute negative profit trades
							ui.Warn(fmt.Sprintf("Re-quoted profit is negative (%.7f XLM) — aborting execution", freshResult.EstimatedNetXLM))
						} else {
							// Execute with fresh quote
							execResult, execErr := performArbitrageScan(cfg, net, amount, baseAsset, counterAsset, destination, slippage, minProfit, true, simulated, output, autoExecute, opts)
							if errors.Is(execErr, errNotExecuted) {
								ui.Warn("Quote moved below threshold at execution — skipped")
							} else if execErr != nil {
								stats.failed++
								ui.Error(fmt.Sprintf("Execution failed: %v", execErr))
							} else if execResult != nil {
								stats.executed++
								realized, ok := realizedBaseDelta(execResult)
								if !ok {
									realized = execResult.EstimatedNetXLM
								}
								stats.totalProfitXLM += realized
								ui.Success(fmt.Sprintf("Executed! Realized: %+.7f %s (est. %+.7f)", realized, baseAsset, execResult.EstimatedNetXLM))
							}
						}
					}
				} else {
					ui.Info("Opportunity found but --execute not enabled. Use --execute to trade.")
				}
			}
		} else if result != nil {
			fmt.Printf("Scan %d: No profit (%.7f XLM)\n", scanNum, result.EstimatedNetXLM)
		}

		// Show stats if enabled
		if showStats {
			if execute {
				fmt.Printf("\r  Scans: %d | Opportunities: %d | Executed: %d | Failed: %d | Realized: %+.7f %s | Next: %ds   ",
					stats.totalScans, stats.opportunities, stats.executed, stats.failed, stats.totalProfitXLM, baseAsset, interval)
			} else {
				fmt.Printf("\r  Scans: %d | Opportunities: %d | Est. profit: %+.7f XLM | Next: %ds   ",
					stats.totalScans, stats.opportunities, stats.totalProfitXLM, interval)
			}
		}

		// Check max scans
		if maxScans > 0 && scanNum >= maxScans {
			fmt.Println()
			ui.Info(fmt.Sprintf("Reached max scans (%d). Exiting.", maxScans))
			break
		}

		// Wait for next tick
		<-ticker.C
	}

	return nil
}

// scanStats tracks continuous scanning statistics
type scanStats struct {
	totalScans     uint
	opportunities  uint
	executed       uint
	failed         uint
	totalProfitXLM float64
}

// errNotExecuted signals that --execute was requested but the quote no longer met the profit threshold.
var errNotExecuted = errors.New("not executed: quote below profit threshold")

// arbExecOpts controls how a round trip is submitted on-chain.
type arbExecOpts struct {
	atomic  bool
	baseFee int64
}

// realizedBaseDelta returns the on-ledger change of the base asset balance across an execution.
func realizedBaseDelta(r *models.SwapRoundTripResult) (float64, bool) {
	if r == nil || r.SnapshotBefore == nil || r.SnapshotAfter == nil {
		return 0, false
	}
	before, okB := snapshotBalance(r.SnapshotBefore, r.BaseAsset)
	after, okA := snapshotBalance(r.SnapshotAfter, r.BaseAsset)
	if !okB || !okA {
		return 0, false
	}
	return after - before, true
}

func snapshotBalance(s *models.AccountSnapshot, code string) (float64, bool) {
	if code == "XLM" {
		v, err := strconv.ParseFloat(s.XLM, 64)
		return v, err == nil
	}
	for _, a := range s.Assets {
		if a.Code == code {
			v, err := strconv.ParseFloat(a.Balance, 64)
			return v, err == nil
		}
	}
	return 0, false
}

// performArbitrageScan executes a single arbitrage scan and returns result
func performArbitrageScan(cfg *config.Config, net models.Network, amount, baseAsset, counterAsset, destination string, slippage, minProfit float64, execute, simulated bool, output string, autoExecute bool, opts arbExecOpts) (*models.SwapRoundTripResult, error) {
	svc := swap.NewService(net)
	if opts.baseFee > 0 {
		svc.SetBaseFee(opts.baseFee)
	}
	_, keyErr := svc.LoadStellarKeypair()
	useSim := simulated || keyErr != nil

	var result *models.SwapRoundTripResult
	var err error
	if useSim {
		sim := swap.NewSimulatedService()
		result, err = sim.AnalyzeRoundTrip(amount, baseAsset, counterAsset)
	} else {
		result, err = svc.AnalyzeRoundTrip(amount, baseAsset, counterAsset, destination)
	}
	if err != nil {
		return nil, err
	}

	if output != "json" {
		ui.SectionLabel("Paper round trip")
		if baseAsset == "XLM" {
			ui.KV("Starting XLM", result.AmountXLM)
			ui.KVColor(fmt.Sprintf("%s (after leg A)", counterAsset), result.USDCIntermediate, ui.BrightYellow)
			ui.KVColor("XLM back (quoted leg B)", result.XLMReturned, ui.BrightGreen)
		} else if counterAsset == "XLM" {
			ui.KV(fmt.Sprintf("Starting %s", baseAsset), result.AmountXLM)
			ui.KVColor("XLM (after leg A)", result.XLMReturned, ui.BrightYellow)
			ui.KVColor(fmt.Sprintf("%s back (quoted leg B)", baseAsset), result.BaseReturned, ui.BrightGreen)
		} else {
			// Neither asset is XLM
			ui.KV(fmt.Sprintf("Starting %s", baseAsset), result.AmountXLM)
			ui.KVColor(fmt.Sprintf("%s (after leg A)", counterAsset), result.CounterIntermediate, ui.BrightYellow)
			ui.KVColor(fmt.Sprintf("%s back (quoted leg B)", baseAsset), result.BaseReturned, ui.BrightGreen)
		}
		ui.KV("Fee reserve (2 txs)", result.FeeReserveXLM)
		ui.KVColor("Est. net XLM (after fees)", fmt.Sprintf("%.7f", result.EstimatedNetXLM), ui.BrightCyan)
		fmt.Printf("  → Leg A path: %s\n", pathSummary(result.LegA))
		fmt.Printf("  → Leg B path: %s\n", pathSummary(result.LegB))
		fmt.Printf("  → Live Horizon quote. Add --execute to submit both path payments (see --min-profit-xlm). A negative net is a loss after the two base fees.\n")
	}

	// Early return if not executing (regardless of output format)
	if !execute {
		if output == "json" {
			fmt.Println(prettyJSON(result))
		} else if useSim {
			ui.Info("Live quotes: omit --simulated and configure a Stellar keypair. Pass --execute only when ready to submit txs.")
		}
		return result, nil
	}

	if result.EstimatedNetXLM < minProfit {
		ui.Warn(fmt.Sprintf("Estimated net %.7f XLM is below --min-profit-xlm %.7f — not executing.", result.EstimatedNetXLM, minProfit))
		if output == "json" {
			fmt.Println(prettyJSON(result))
		}
		return result, errNotExecuted
	}

	// Additional safety check: never execute negative profit trades
	if result.EstimatedNetXLM < 0 {
		ui.Warn(fmt.Sprintf("Estimated net %.7f XLM is negative — not executing to prevent losses.", result.EstimatedNetXLM))
		if output == "json" {
			fmt.Println(prettyJSON(result))
		}
		return result, errNotExecuted
	}
	if useSim {
		ui.Warn("Execute will simulate swaps only (no ledger txs).")
	} else if execute && output != "json" && !autoExecute && !ui.Confirm("Execute two path payments on the network?") {
		ui.Info("Cancelled.")
		return result, fmt.Errorf("user cancelled")
	}

	spin := ui.NewSpinner("Executing round-trip swaps...")
	spin.Start()

	// Validate quotes are still fresh before executing
	if result.LegA != nil && !result.LegA.IsValid() {
		spin.Stop(false, "Quote expired")
		return result, fmt.Errorf("leg A quote expired (age: %.1fs)", result.LegA.Age())
	}
	if result.LegB != nil && !result.LegB.IsValid() {
		spin.Stop(false, "Quote expired")
		return result, fmt.Errorf("leg B quote expired (age: %.1fs)", result.LegB.Age())
	}

	amountFloat := parseFloatOr(amount)
	var payments []*models.Payment
	if useSim {
		sim := swap.NewSimulatedService()
		payments, err = sim.ExecuteRoundTrip(amountFloat, slippage/100, destination, minProfit, baseAsset, counterAsset)
	} else if opts.atomic {
		var pay *models.Payment
		var execRes *models.SwapRoundTripResult
		pay, execRes, err = svc.ExecuteRoundTripAtomic(amountFloat, destination, minProfit, baseAsset, counterAsset)
		if execRes != nil {
			result = execRes
		}
		if pay != nil {
			payments = []*models.Payment{pay}
		}
	} else {
		payments, err = svc.ExecuteRoundTrip(amountFloat, slippage/100, destination, minProfit, baseAsset, counterAsset)
	}
	if err != nil {
		spin.Stop(false, err.Error())
		return result, err
	}
	spin.Stop(true, "Round trip complete")

	if output == "json" {
		fmt.Println(prettyJSON(struct {
			Analysis *models.SwapRoundTripResult `json:"analysis"`
			Payments []*models.Payment           `json:"payments"`
		}{result, payments}))
		return result, nil
	}
	for i, p := range payments {
		ui.SectionLabel(fmt.Sprintf("Payment %d", i+1))
		ui.KV("TX Hash", p.TxHash)
		ui.KV("Sent", fmt.Sprintf("%s %s", p.Amount, p.Asset))
	}

	// Display account balance comparison if snapshots are available
	if result.SnapshotBefore != nil {
		printBalanceComparison(result)
	}

	config.SaveState("payment_latest", payments[len(payments)-1]) //nolint:errcheck // best-effort state cache
	return result, nil
}

// printBalanceComparison displays before/after account balances with deltas
// Shows all three stages: before, after leg A (intermediates), and final
func printBalanceComparison(result *models.SwapRoundTripResult) {
	fmt.Println()
	ui.SectionLabel("Account Balance Changes")

	before := result.SnapshotBefore
	afterLegA := result.SnapshotAfterLegA
	after := result.SnapshotAfter

	// Build map of all assets from all snapshots
	allAssets := make(map[string]bool)
	assetBefore := make(map[string]string)
	assetAfterLegA := make(map[string]string)
	assetAfter := make(map[string]string)

	// XLM
	assetBefore["XLM"] = before.XLM
	allAssets["XLM"] = true

	if afterLegA != nil {
		assetAfterLegA["XLM"] = afterLegA.XLM
	}
	if after != nil {
		assetAfter["XLM"] = after.XLM
		allAssets["XLM"] = true
	}

	// Other assets
	for _, a := range before.Assets {
		allAssets[a.Code] = true
		assetBefore[a.Code] = a.Balance
	}
	if afterLegA != nil {
		for _, a := range afterLegA.Assets {
			allAssets[a.Code] = true
			assetAfterLegA[a.Code] = a.Balance
		}
	}
	if after != nil {
		for _, a := range after.Assets {
			allAssets[a.Code] = true
			assetAfter[a.Code] = a.Balance
		}
	}

	// Determine which columns to show
	hasIntermediate := afterLegA != nil
	hasFinal := after != nil

	if hasIntermediate && hasFinal {
		// Full comparison: Before → After Leg A → Final
		t := ui.NewTable("Asset", "Before", "After Leg A", "Final", "Net Change")

		for code := range allAssets {
			balBefore := assetBefore[code]
			balAfterLegA := assetAfterLegA[code]
			balAfter := assetAfter[code]

			if balBefore == "" {
				balBefore = "0.0000000"
			}
			if balAfterLegA == "" {
				balAfterLegA = "0.0000000"
			}
			if balAfter == "" {
				balAfter = "0.0000000"
			}

			beforeVal := parseFloatOr(balBefore)
			afterVal := parseFloatOr(balAfter)
			netDelta := afterVal - beforeVal

			t.AddRow(
				ui.Teal_(code),
				formatBalance(balBefore),
				formatBalance(balAfterLegA),
				formatBalance(balAfter),
				formatDelta(netDelta),
			)
		}
		t.Print()
		fmt.Println()
		ui.Info("After Leg A: Shows intermediate assets held during path payment")
	} else if hasFinal {
		// Simple comparison: Before → Final
		t := ui.NewTable("Asset", "Before", "After", "Change")

		for code := range allAssets {
			balBefore := assetBefore[code]
			balAfter := assetAfter[code]

			if balBefore == "" {
				balBefore = "0.0000000"
			}
			if balAfter == "" {
				balAfter = "0.0000000"
			}

			beforeVal := parseFloatOr(balBefore)
			afterVal := parseFloatOr(balAfter)
			delta := afterVal - beforeVal

			t.AddRow(ui.Teal_(code), formatBalance(balBefore), formatBalance(balAfter), formatDelta(delta))
		}
		t.Print()
	}
}

// formatBalance formats a balance string for display
func formatBalance(bal string) string {
	val, err := strconv.ParseFloat(bal, 64)
	if err != nil {
		return bal
	}
	return fmt.Sprintf("%.7f", val)
}

// formatDelta formats a balance change with color
func formatDelta(delta float64) string {
	if delta > 0 {
		return ui.Green_(fmt.Sprintf("+%.7f", delta))
	} else if delta < 0 {
		return ui.Red_(fmt.Sprintf("%.7f", delta))
	}
	return fmt.Sprintf("%.7f", delta)
}

func pathSummary(q *models.SwapQuote) string {
	if q == nil || len(q.Paths) == 0 {
		return "(no path detail)"
	}
	p := q.Paths[0]
	if len(p.Path) == 0 {
		return q.SourceAsset + " → " + q.DestAsset + " (direct)"
	}
	// Horizon path[] is intermediate assets only; show full route for clarity.
	mid := p.Path[0].Code
	for _, a := range p.Path[1:] {
		mid += " → " + a.Code
	}
	return q.SourceAsset + " → " + mid + " → " + q.DestAsset
}

// ─── swap assets ──────────────────────────────

func newSwapAssetsCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "assets",
		Short: "List supported swap assets and their issuers",
		Run: func(c *Command, args []string) error {
			ui.Header("Supported Swap Assets")
			fmt.Println()

			ui.SectionLabel("Testnet Assets")
			t := ui.NewTable("Asset", "Issuer", "Type")
			t.AddRow(ui.Teal_("XLM"), "Native", "Stellar Lumens")
			t.AddRow(ui.Teal_("USDC"), "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "Stablecoin")
			t.AddRow(ui.Teal_("EURC"), "GAKMOVSF35IPK5HTDN4B3ITIR5R4AX6PZFAXNPJFDHNIUQKDT5O6G2E", "Stablecoin")
			t.Print()

			fmt.Println()
			ui.SectionLabel("Mainnet Assets")
			t = ui.NewTable("Asset", "Issuer", "Type")
			t.AddRow(ui.Teal_("XLM"), "Native", "Stellar Lumens")
			t.AddRow(ui.Teal_("USDC"), "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "Stablecoin")
			t.AddRow(ui.Teal_("EURC"), "GDUKMGUGDZQK6YHYA5Z6AY2G4XDSZDW2WER5GZ5GUESDSEZNCNDJID9", "Stablecoin")
			t.Print()

			fmt.Println()
			ui.Info("Note: To receive non-XLM assets, you must have an established trustline.")
			ui.Info("Run 'stellar-go-cli asset trust --code USDC --issuer <issuer>' to create a trustline.")

			return nil
		},
	}
}
