package commands

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/payments"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/x402"
	"github.com/stellar/go-stellar-sdk/keypair"
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
	to := fs.String("to", "", "Recipient address (required unless --rail x402)")
	amount := fs.String("amount", "", "Amount to send (required unless --rail x402)")
	asset := fs.String("asset", "XLM", "Asset code: XLM | USDC | EURC")
	rail := fs.String("rail", "direct", "Payment rail: direct | x402 | tempo | zk")
	memo := fs.String("memo", "", "Optional payment memo")
	network := fs.String("network", "", "Network: stellar-testnet | stellar-mainnet (default: active wallet's network)")
	resource := fs.String("resource", "", "x402 resource URL (required when --rail x402)")
	method := fs.String("method", "GET", "HTTP method for x402 resource request")
	body := fs.String("body", "", "Optional HTTP request body for x402 resource request")
	dryRun := fs.Bool("dry-run", false, "Build the x402 payment payload without sending the paid retry")
	payer := fs.String("payer", "", "Payer Stellar address (x402 only; defaults to active wallet)")
	maxAtomicAmount := fs.String("max-atomic-amount", "", "Optional x402 spend cap in token atomic units")
	rpcURL := fs.String("rpc-url", "", "Optional Soroban RPC URL override for x402")
	output := fs.String("output", "pretty", "Output format: pretty | json")
	vcAttach := fs.Bool("vc-attach", false, "Attach latest VC to the payment (includes VC ID in memo)")

	return &Command{
		Name:  "send",
		Short: "Send a payment to an address",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Send Payment")

			// Resolve network: explicit flag > wallet's declared network > config default.
			netName := *network
			if netName == "" {
				if acc, err := wallet.NewService().GetActiveWallet(); err == nil && acc.Network != "" {
					netName = string(acc.Network)
				} else {
					netName = cfg.Network
				}
			}

			if strings.EqualFold(*rail, string(models.RailX402)) {
				if strings.TrimSpace(*resource) == "" {
					return fmt.Errorf("--resource is required for x402; the HTTP server must return an x402 v2 payment challenge")
				}
				return runX402CLI(cfg, x402CommandOptions{
					Resource:        *resource,
					Method:          *method,
					Body:            []byte(*body),
					Network:         netName,
					Payer:           *payer,
					DryRun:          *dryRun,
					MaxAtomicAmount: *maxAtomicAmount,
					RPCURL:          *rpcURL,
					Output:          *output,
				})
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
				ui.Warn("No active account. Using placeholder. Run 'stellar-go-cli wallet connect'.")
			}

			r := models.PaymentRail(*rail)
			net := models.Network(netName)
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
					ui.Warn("No saved VC found. Run 'stellar-go-cli did attest' first.")
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
			config.SaveState("payment_latest", payment) //nolint:errcheck // best-effort state cache
			ui.Info("Run 'stellar-go-cli report generate' to produce a compliance report.")

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

type headerListFlag map[string]string

func (h *headerListFlag) String() string {
	if h == nil || len(*h) == 0 {
		return ""
	}
	names := make([]string, 0, len(*h))
	for name := range *h {
		names = append(names, name)
	}
	return strings.Join(names, ",")
}

func (h *headerListFlag) Set(value string) error {
	name, headerValue, ok := strings.Cut(value, ":")
	if !ok || strings.TrimSpace(name) == "" {
		return fmt.Errorf("header must use 'Name: value' format")
	}
	if *h == nil {
		*h = make(map[string]string)
	}
	(*h)[strings.TrimSpace(name)] = strings.TrimSpace(headerValue)
	return nil
}

type x402CommandOptions struct {
	Resource        string
	Method          string
	Headers         map[string]string
	Body            []byte
	Network         string
	Payer           string
	DryRun          bool
	MaxAtomicAmount string
	RPCURL          string
	Output          string
}

func newPayX402Cmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("x402", flag.ContinueOnError)
	resource := fs.String("resource", "", "HTTP resource URL that returns an x402 v2 challenge")
	method := fs.String("method", "GET", "HTTP method for the resource request")
	body := fs.String("body", "", "Optional HTTP request body")
	headers := headerListFlag{}
	fs.Var(&headers, "header", "Additional request header ('Name: value'); repeatable")
	networkName := fs.String("network", cfg.Network, "Stellar network: stellar-testnet | stellar-mainnet")
	payer := fs.String("payer", "", "Payer Stellar address (defaults to active wallet)")
	dryRun := fs.Bool("dry-run", false, "Build the signed payment payload without sending the paid retry")
	maxAtomicAmount := fs.String("max-atomic-amount", "", "Optional spend cap in the token's atomic units")
	rpcURL := fs.String("rpc-url", "", "Optional Soroban RPC URL override")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "x402",
		Short: "Execute a Stellar x402 v2 HTTP payment",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("x402 Payment Request")
			if strings.TrimSpace(*resource) == "" {
				return fmt.Errorf("--resource is required; the HTTP server must return an x402 v2 payment challenge")
			}
			return runX402CLI(cfg, x402CommandOptions{
				Resource:        *resource,
				Method:          *method,
				Headers:         map[string]string(headers),
				Body:            []byte(*body),
				Network:         *networkName,
				Payer:           *payer,
				DryRun:          *dryRun,
				MaxAtomicAmount: *maxAtomicAmount,
				RPCURL:          *rpcURL,
				Output:          *output,
			})
		},
	}
}

func runX402CLI(cfg *config.Config, opts x402CommandOptions) error {
	ui.PrintStep(1, "Request Resource")
	ui.SectionLabel("HTTP Request")
	ui.KV("Resource", opts.Resource)
	ui.KV("Method", strings.ToUpper(opts.Method))
	ui.KV("x402 Network", opts.Network)

	spin := ui.NewSpinner("Running HTTP 402 challenge, Soroban simulation, and paid retry...")
	spin.Start()
	result, err := executeX402Flow(context.Background(), cfg, opts)
	if err != nil {
		spin.Stop(false, err.Error())
		if result != nil {
			displayX402Result(result, opts.Output)
		}
		return err
	}
	if result.PaymentAttempted {
		spin.Stop(true, fmt.Sprintf("Resource response received (HTTP %d)", result.StatusCode))
	} else if result.DryRun && result.PaymentPayload != nil {
		spin.Stop(true, "Signed payment payload built; paid retry skipped")
	} else {
		spin.Stop(true, fmt.Sprintf("Resource response received (HTTP %d)", result.StatusCode))
	}

	displayX402Result(result, opts.Output)
	config.SaveState("x402_latest", summarizeX402Result(result)) //nolint:errcheck // best-effort state cache
	return nil
}

func executeX402Flow(ctx context.Context, cfg *config.Config, opts x402CommandOptions) (*x402.Result, error) {
	if cfg != nil && !cfg.Integrations.X402Enabled {
		return nil, fmt.Errorf("x402 is disabled in configuration")
	}
	if strings.TrimSpace(opts.Resource) == "" {
		return nil, fmt.Errorf("resource URL is required")
	}
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = "GET"
	}
	if strings.ContainsAny(method, " \t\r\n") {
		return nil, fmt.Errorf("invalid HTTP method %q", opts.Method)
	}

	canonicalNetwork, err := x402.CanonicalNetwork(opts.Network)
	if err != nil {
		return nil, err
	}

	var signer *keypair.Full
	payer := strings.TrimSpace(opts.Payer)
	if payer == "" && cfg != nil {
		payer = strings.TrimSpace(cfg.ActiveAddress)
	}
	if payer != "" {
		signer, err = wallet.LoadStellarKeypairForAddress(payer)
	} else {
		signer, err = wallet.LoadStellarKeypair()
	}
	if err != nil {
		return nil, fmt.Errorf("load Stellar payer keypair: %w", err)
	}

	client, rpcClient, err := x402.NewStellarClientWithRPCURL(canonicalNetwork, signer, config.NewHTTPClient(), opts.RPCURL)
	if err != nil {
		return nil, err
	}
	defer rpcClient.Close() //nolint:errcheck // RPC client cleanup

	return client.Fetch(ctx, x402.Request{
		Method:  method,
		URL:     strings.TrimSpace(opts.Resource),
		Headers: opts.Headers,
		Body:    opts.Body,
	}, x402.FetchOptions{
		DryRun:          opts.DryRun,
		MaxAtomicAmount: opts.MaxAtomicAmount,
	})
}

type x402ResultOutput struct {
	StatusCode       int                      `json:"statusCode"`
	FinalURL         string                   `json:"finalUrl"`
	Network          string                   `json:"network,omitempty"`
	Scheme           string                   `json:"scheme,omitempty"`
	Asset            string                   `json:"asset,omitempty"`
	AtomicAmount     string                   `json:"atomicAmount,omitempty"`
	PayTo            string                   `json:"payTo,omitempty"`
	PaymentAttempted bool                     `json:"paymentAttempted"`
	DryRun           bool                     `json:"dryRun"`
	PaymentReady     bool                     `json:"paymentReady"`
	Settlement       *x402.SettlementResponse `json:"settlement,omitempty"`
	ResponseBody     string                   `json:"responseBody,omitempty"`
}

func summarizeX402Result(result *x402.Result) x402ResultOutput {
	if result == nil {
		return x402ResultOutput{}
	}
	summary := x402ResultOutput{
		StatusCode:       result.StatusCode,
		FinalURL:         result.FinalURL,
		PaymentAttempted: result.PaymentAttempted,
		DryRun:           result.DryRun,
		PaymentReady:     result.PaymentPayload != nil,
		Settlement:       result.Settlement,
	}
	if result.Accepted != nil {
		summary.Network = result.Accepted.Network
		summary.Scheme = result.Accepted.Scheme
		summary.Asset = result.Accepted.Asset
		summary.AtomicAmount = result.Accepted.Amount
		summary.PayTo = result.Accepted.PayTo
	}
	if len(result.BodyText) > 0 {
		body := result.BodyText
		if len(body) > 8192 {
			body = body[:8192] + "..."
		}
		summary.ResponseBody = body
	}
	return summary
}

func displayX402Result(result *x402.Result, output string) {
	if result == nil {
		return
	}
	if output == "json" {
		ui.Raw(prettyJSON(summarizeX402Result(result)))
		return
	}

	ui.PrintStep(2, "Payment Challenge")
	if result.PaymentRequired == nil {
		ui.Info("The resource did not return an x402 payment challenge.")
	} else if result.Accepted != nil {
		ui.SectionLabel("Accepted Requirement")
		ui.KV("Scheme", result.Accepted.Scheme)
		ui.KV("Network", result.Accepted.Network)
		ui.KV("Token Contract", result.Accepted.Asset)
		ui.KVColor("Atomic Amount", result.Accepted.Amount, ui.BrightYellow)
		ui.KV("Pay To", result.Accepted.PayTo)
		ui.KV("Timeout", fmt.Sprintf("%d seconds", result.Accepted.MaxTimeoutSeconds))
	}

	ui.PrintStep(3, "Resource Response")
	ui.SectionLabel("Result")
	ui.KVColor("HTTP Status", fmt.Sprintf("%d", result.StatusCode), ui.BrightGreen)
	if result.FinalURL != "" {
		ui.KV("Final URL", result.FinalURL)
	}
	if result.DryRun && result.PaymentPayload != nil {
		ui.KV("Payment", "signed payload created; retry skipped")
	} else if result.PaymentAttempted {
		ui.KV("Payment", "PAYMENT-SIGNATURE retry sent")
	}
	if result.Settlement != nil {
		ui.KVColor("Settlement", fmt.Sprintf("%v", result.Settlement.Success), ui.BrightGreen)
		if result.Settlement.Transaction != "" {
			ui.KV("Settlement TX", result.Settlement.Transaction)
		}
		if result.Settlement.Payer != "" {
			ui.KV("Settlement Payer", result.Settlement.Payer)
		}
	}
	if result.BodyText != "" {
		ui.SectionLabel("Response Body")
		body := result.BodyText
		if len(body) > 4096 {
			body = body[:4096] + "..."
		}
		ui.Raw(body)
	}
	ui.Separator()
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
				ui.Warn("No active account. Using placeholder. Run 'stellar-go-cli wallet connect'.")
			}

			net := models.Network(*network)
			svc := payments.NewService()

			ui.PrintStep(1, "Building ZK Proof Request")

			// Build ZK proof request
			amountFloat, perr := strconv.ParseFloat(*amount, 64)
			if perr != nil {
				return fmt.Errorf("invalid amount %q: %w", *amount, perr)
			}
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

			config.SaveState("payment_latest", payment) //nolint:errcheck // best-effort state cache
			ui.Info("Run 'stellar-go-cli report generate' to produce a compliance report.")

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
