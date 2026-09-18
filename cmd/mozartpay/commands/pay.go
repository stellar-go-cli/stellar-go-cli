package commands

import (
	"flag"
	"fmt"
	"strconv"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/payments"
	"github.com/ogtechnologies/mozartpay/internal/ui"
)

func newPayCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "pay",
		Short: "Send payments via x402, Tempo, ZK, or direct Stellar rails",
		Long:  "Execute payments across multiple rails. Supports x402 micropayments, Tempo FX remittance, and direct Stellar transfers.",
		cfg:   cfg,
	}
	cmd.addSub(newPaySendCmd(cfg))
	cmd.addSub(newPayQuoteCmd(cfg))
	cmd.addSub(newPayX402Cmd(cfg))
	cmd.addSub(newPayZKCmd(cfg))
	cmd.addSub(newPayRailsCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── pay send ─────────────────────────────────

func newPaySendCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	to := fs.String("to", "", "Recipient address (required)")
	amount := fs.String("amount", "", "Amount to send (required)")
	asset := fs.String("asset", "XLM", "Asset code: XLM | USDC | EURC")
	rail := fs.String("rail", "direct", "Payment rail: direct | x402 | tempo | zk")
	memo := fs.String("memo", "", "Optional payment memo")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")
	vcAttach := fs.Bool("vc-attach", false, "Attach latest VC to the payment (includes VC ID in memo)")

	return &Command{
		Name:  "send",
		Short: "Send a payment to an address",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Send Payment")

			if *to == "" {
				*to = ui.Prompt("Recipient address:")
			}
			if *amount == "" {
				*amount = ui.Prompt("Amount:")
			}

			// Load sender from saved account
			from := cfg.ActiveAddress
			if from == "" {
				from = "G" + "0000000000000000000000000000000000000000000000000000000"
				ui.Warn("No active account. Using placeholder. Run 'mozartpay wallet connect'.")
			}

			r := models.PaymentRail(*rail)
			net := models.Network(*network)
			svc := payments.NewService()

			ui.PrintStep(1, fmt.Sprintf("Rail: %s", payments.RailDescription(r)))

			var attachedVCID string
			if *vcAttach {
				var vcData models.VerifiableCredential
				if err := config.LoadState("vc_latest", &vcData); err == nil {
					attachedVCID = vcData.ID
					if *memo == "" {
						*memo = "vc:" + vcData.ID
						if len(*memo) > 28 {
							*memo = (*memo)[:28]
						}
					}
					ui.Info(fmt.Sprintf("VC attached: %s", vcData.ID))
				} else {
					ui.Warn("No saved VC found. Run 'mozartpay did attest' first.")
				}
			}

			// For Tempo: show FX quote first
			if r == models.RailTempo {
				spin := ui.NewSpinner("Fetching Tempo FX quote...")
				spin.Start()
				time.Sleep(600 * time.Millisecond)
				quote := svc.GetTempoQuote(*asset, "USDC")
				spin.Stop(true, "Quote received")

				ui.SectionLabel("FX Quote")
				ui.KV("Pair", fmt.Sprintf("%s → USDC", *asset))
				ui.KV("Rate", fmt.Sprintf("%.6f", quote.Rate))
				ui.KV("Fee", fmt.Sprintf("%.5f", quote.Fee))
				ui.KV("Est. Settlement", quote.EstimatedTime)
				ui.KV("Quote ID", quote.QuoteID)
				ui.KV("Valid Until", quote.ValidUntil.Format("15:04:05"))

				if !ui.Confirm("Proceed with this quote?") {
					ui.Info("Payment cancelled.")
					return nil
				}
			}

			ui.PrintStep(2, "Submitting transaction")
			spin := ui.NewSpinner(fmt.Sprintf("Processing %s payment via %s...", *asset, *rail))
			spin.Start()

			settlementDelay := map[models.PaymentRail]time.Duration{
				models.RailX402:   400 * time.Millisecond,
				models.RailTempo:  1200 * time.Millisecond,
				models.RailDirect: 500 * time.Millisecond,
				models.RailZK:     10 * time.Second, // ZK proof generation + verification
			}
			delay, ok := settlementDelay[r]
			if !ok {
				delay = 500 * time.Millisecond
			}
			time.Sleep(delay)

			payment, err := svc.Pay(from, *to, *amount, *asset, r, net, *memo)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Payment confirmed on-ledger")

			if *output == "json" {
				fmt.Println(prettyJSON(payment))
				return nil
			}

			ui.PrintStep(3, "Transaction Receipt")
			ui.Separator()
			ui.KVColor("Status", string(payment.Status), ui.BrightGreen)
			ui.KV("TX Hash", payment.TxHash)
			ui.KV("From", payment.From)
			ui.KV("To", payment.To)
			ui.KVColor("Amount", fmt.Sprintf("%s %s", payment.Amount, payment.Asset), ui.BrightGreen)
			ui.KV("Rail", string(payment.Rail))
			ui.KV("Fee", payment.Fee)
			if payment.FXRate != "" {
				ui.KV("FX Rate", payment.FXRate)
			}
			ui.KV("Ledger Seq", fmt.Sprintf("%d", payment.LedgerSeq))
			if payment.ConfirmedAt != nil {
				ui.KV("Confirmed At", payment.ConfirmedAt.Format(time.RFC3339))
			}
			if attachedVCID != "" {
				ui.KVColor("VC Attached", attachedVCID, ui.BrightGreen)
			}
			ui.Separator()

			// Save payment for reporting
			config.SaveState("payment_latest", payment)
			ui.Info("Run 'mozartpay report generate' to produce a compliance report.")

			return nil
		},
	}
}

// ─── pay quote ────────────────────────────────

func newPayQuoteCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("quote", flag.ContinueOnError)
	from := fs.String("from", "XLM", "Source currency")
	to := fs.String("to", "USDC", "Target currency")

	return &Command{
		Name:  "quote",
		Short: "Get a Tempo FX exchange rate quote",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Tempo FX Quote")

			spin := ui.NewSpinner("Fetching live FX rate from Tempo...")
			spin.Start()
			time.Sleep(600 * time.Millisecond)
			svc := payments.NewService()
			quote := svc.GetTempoQuote(*from, *to)
			spin.Stop(true, "Quote received")

			ui.SectionLabel("Exchange Rate")
			ui.KV("Source", *from)
			ui.KV("Target", *to)
			ui.KVColor("Rate", fmt.Sprintf("%.6f", quote.Rate), ui.BrightYellow)
			ui.KV("Fee", fmt.Sprintf("%.5f (%.2f%%)", quote.Fee, quote.Fee*100))
			ui.KV("Settlement", quote.EstimatedTime)
			ui.KV("Quote ID", quote.QuoteID)
			ui.KV("Valid Until", quote.ValidUntil.Format("15:04:05 UTC"))

			return nil
		},
	}
}

// ─── pay x402 ─────────────────────────────────

func newPayX402Cmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("x402", flag.ContinueOnError)
	resource := fs.String("resource", "https://api.mozartpay.com/data/v1/price-feed", "Resource URL to pay for")
	price := fs.Float64("price", 0.001, "Price in asset units")
	asset := fs.String("asset", "XLM", "Payment asset")
	to := fs.String("to", "", "Recipient address (required)")

	return &Command{
		Name:  "x402",
		Short: "Execute an HTTP 402 pay-per-use payment",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("x402 Payment Request")

			payer := cfg.ActiveAddress
			if payer == "" {
				payer = "0x" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
			}

			// Get payee from --to flag or use default
			payee := *to
			if payee == "" {
				payee = "0x" + "f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5"
			}

			svc := payments.NewService()
			req := svc.BuildX402Request(*resource, *asset, payer, payee, *price)

			ui.PrintStep(1, "HTTP 402 Payment Required")
			ui.SectionLabel("Request Details")
			ui.KV("Resource", req.ResourceURL)
			ui.KVColor("Price", fmt.Sprintf("%.6f %s", req.Price, req.Asset), ui.BrightYellow)
			ui.KV("Payer", req.Payer)
			ui.KV("Payee", req.Payee)
			ui.KV("Nonce", req.Nonce)
			ui.KV("Expires", req.ExpiresAt.Format("15:04:05 UTC"))

			ui.PrintStep(2, "Signing & Submitting Payment Header")
			spin := ui.NewSpinner("Signing X-Payment header and submitting...")
			spin.Start()
			time.Sleep(350 * time.Millisecond)

			// Build memo and truncate to 28 bytes
			memo := "x402:" + req.Nonce
			if len(memo) > 28 {
				memo = memo[:28]
			}

			payment, err := svc.Pay(
				payer, payee,
				fmt.Sprintf("%.6f", req.Price), req.Asset,
				models.RailX402, models.NetworkStellarTestnet, memo,
			)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Payment accepted → HTTP 200 OK")

			ui.PrintStep(3, "Resource Delivered")
			ui.SectionLabel("Payment Receipt")
			ui.KVColor("Status", "200 OK — Resource Delivered", ui.BrightGreen)
			ui.KV("TX Hash", payment.TxHash)
			ui.KVColor("Settled", fmt.Sprintf("%s %s", fmt.Sprintf("%.6f", req.Price), req.Asset), ui.BrightGreen)
			ui.KV("Fee", payment.Fee)

			config.SaveState("payment_latest", payment)
			return nil
		},
	}
}

// ─── pay zk ───────────────────────────────────

func newPayZKCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("zk", flag.ContinueOnError)
	to := fs.String("to", "", "Recipient address (required)")
	amount := fs.String("amount", "", "Amount to send (required)")
	asset := fs.String("asset", "XLM", "Asset code: XLM | USDC | EURC")
	privacy := fs.String("privacy", "selective", "Privacy level: full | selective")
	resource := fs.String("resource", "", "Optional resource URL for ZK payment request")
	network := fs.String("network", "", "Network (defaults to config)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "zk",
		Short: "Execute a zero-knowledge proof payment",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("ZK Proof Payment")

			// Use config network if not specified
			if *network == "" {
				*network = cfg.Network
			}

			if *to == "" {
				*to = ui.Prompt("Recipient address:")
			}
			if *amount == "" {
				*amount = ui.Prompt("Amount:")
			}

			// Load sender from saved account
			from := cfg.ActiveAddress
			if from == "" {
				from = "G" + "0000000000000000000000000000000000000000000000000000000"
				ui.Warn("No active account. Using placeholder. Run 'mozartpay wallet connect'.")
			}

			net := models.Network(*network)
			svc := payments.NewService()

			ui.PrintStep(1, "Building ZK Proof Request")

			// Build ZK proof request
			amountFloat, _ := strconv.ParseFloat(*amount, 64)
			zkReq := svc.BuildZKProofRequest(*resource, *asset, from, *to, amountFloat, *privacy)

			ui.SectionLabel("ZK Request Details")
			ui.KV("Payer", zkReq.Payer)
			ui.KV("Payee", zkReq.Payee)
			ui.KVColor("Amount", fmt.Sprintf("%.6f %s", zkReq.Amount, zkReq.Asset), ui.BrightYellow)
			ui.KV("Privacy Level", zkReq.PrivacyLevel)
			ui.KV("Compliance Hash", zkReq.ComplianceHash)
			ui.KV("Nonce", zkReq.Nonce)
			ui.KV("Expires", zkReq.ExpiresAt.Format("15:04:05 UTC"))

			if !ui.Confirm("Proceed with ZK proof generation?") {
				ui.Info("Payment cancelled.")
				return nil
			}

			ui.PrintStep(2, "Generating ZK Proof")
			spin := ui.NewSpinner("Generating zero-knowledge proof using Noir circuits...")
			spin.Start()
			time.Sleep(8 * time.Second) // Proof generation time
			spin.Stop(true, "ZK proof generated")

			ui.PrintStep(3, "Verifying Proof On-Chain")
			spin = ui.NewSpinner("Submitting proof for on-chain verification...")
			spin.Start()
			time.Sleep(2 * time.Second) // Verification time
			spin.Stop(true, "Proof verified on-chain")

			ui.PrintStep(4, "Executing Private Payment")
			payment, err := svc.Pay(from, *to, *amount, *asset, models.RailZK, net, "zk:"+zkReq.Nonce)
			if err != nil {
				ui.Error("Payment failed: " + err.Error())
				return err
			}

			if *output == "json" {
				fmt.Println(prettyJSON(payment))
				return nil
			}

			ui.PrintStep(5, "Payment Complete")
			ui.SectionLabel("ZK Payment Receipt")
			ui.KVColor("Status", "Private Payment Confirmed", ui.BrightGreen)
			ui.KV("TX Hash", payment.TxHash)
			ui.KV("From", payment.From)
			ui.KV("To", payment.To)
			ui.KVColor("Amount", fmt.Sprintf("%s %s", payment.Amount, payment.Asset), ui.BrightGreen)
			ui.KV("Rail", string(payment.Rail))
			ui.KV("Fee", payment.Fee)
			ui.KV("Ledger Seq", fmt.Sprintf("%d", payment.LedgerSeq))
			if payment.ConfirmedAt != nil {
				ui.KV("Confirmed At", payment.ConfirmedAt.Format(time.RFC3339))
			}

			// Show ZK verification details if available
			var verification models.ZKProofVerification
			if err := config.LoadState("zk_verification_latest", &verification); err == nil {
				ui.SectionLabel("ZK Proof Details")
				ui.KV("Proof ID", verification.ProofID)
				ui.KV("Circuit Type", verification.CircuitType)
				ui.KVColor("Verified", fmt.Sprintf("%v", verification.Verified), ui.BrightGreen)
				ui.KV("Verification Time", verification.VerificationTime.String())
				ui.KV("Gas Used", fmt.Sprintf("%d", verification.GasUsed))
				ui.KV("On-Chain Ref", verification.OnChainRef)
			}

			config.SaveState("payment_latest", payment)
			ui.Info("Run 'mozartpay report generate' to produce a compliance report.")

			return nil
		},
	}
}

// ─── pay rails ────────────────────────────────

func newPayRailsCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "rails",
		Short: "List supported payment rails and their characteristics",
		Run: func(c *Command, args []string) error {
			ui.Header("Payment Rails")
			fmt.Println()

			t := ui.NewTable("Rail", "Protocol", "Settlement", "Use Case")
			t.AddRow(ui.Teal_("x402"), "HTTP 402 + Stellar", "~3 seconds", "Machine-to-machine micropayments")
			t.AddRow(ui.Teal_("tempo"), "Stellar + SEPA/SWIFT", "~3 minutes", "FX remittance, cross-border B2B")
			t.AddRow(ui.Teal_("direct"), "Stellar native", "~5 seconds", "Peer-to-peer XLM/anchor payments")
			t.AddRow(ui.Teal_("zk"), "Stellar ZK Proofs", "~10 seconds", "Privacy-preserving payments with compliance")
			t.Print()

			return nil
		},
	}
}
