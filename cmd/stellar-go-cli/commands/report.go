package commands

import (
	"flag"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/reporting"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

func newReportCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "report",
		Short: "Generate post-transaction compliance reports",
		Long:  "Produce VC-linked audit reports, ISO 20022 XML exports, and carbon offset logs.",
		cfg:   cfg,
	}
	cmd.addSub(newReportGenerateCmd(cfg))
	cmd.addSub(newReportShowCmd(cfg))
	cmd.addSub(newReportISO20022Cmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── report generate ──────────────────────────

func newReportGenerateCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	vcAttach := fs.Bool("vc-attach", true, "Attach latest VC to the report")
	output := fs.String("output", "pretty", "Output: pretty | json | iso20022")
	txHash := fs.String("tx", "", "Filter by transaction hash (informational)")

	return &Command{
		Name:  "generate",
		Short: "Generate a post-transaction report",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Post-Transaction Report")

			// Load saved state
			var payment *models.Payment
			var paymentData models.Payment
			if err := config.LoadState("payment_latest", &paymentData); err == nil {
				payment = &paymentData
			}

			var asset *models.Asset
			var assetData models.Asset
			if err := config.LoadState("asset_latest", &assetData); err == nil {
				asset = &assetData
			}

			activeDID := cfg.ActiveDID
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
			if activeDID == "" {
				activeDID = "did:key:no-active-did"
			}
			did := activeDID

			var vc *models.VerifiableCredential
			if *vcAttach {
				var vcData models.VerifiableCredential
				if err := config.LoadState("vc_latest", &vcData); err == nil {
					vc = &vcData
				}
			}

			if payment == nil && asset == nil {
				ui.Warn("No payment or asset found in state.")
				ui.Info("Run 'mozartpay pay send' or 'mozartpay asset create-ft' first.")
				ui.Info("Generating demo report...")
				payment = demoPayment()
				asset = demoAsset()
			}

			if *txHash != "" {
				ui.Info(fmt.Sprintf("Filtering report for TX: %s", *txHash))
			}

			spin := ui.NewSpinner("Generating compliance report...")
			spin.Start()
			time.Sleep(700 * time.Millisecond)

			svc := reporting.NewService()
			report, err := svc.GenerateReport(payment, asset, did, vc)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, fmt.Sprintf("Report %s generated", report.ReportID))

			switch *output {
			case "json":
				out, _ := svc.FormatJSON(report)
				fmt.Println(out)
			case "iso20022":
				out, err := svc.FormatISO20022XML(report)
				if err != nil {
					ui.Error("ISO 20022 generation failed: " + err.Error())
					return err
				}
				fmt.Println(out)
			default:
				fmt.Println()
				fmt.Println(svc.FormatSummary(report))
			}

			config.SaveState("report_latest", report)
			if *output == "pretty" {
				ui.Info("Full report saved to ~/.mozartpay/state/report_latest.json")
				ui.Info("Export as XML: mozartpay report iso20022")
			}

			return nil
		},
	}
}

// ─── report show ─────────────────────────────

func newReportShowCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	output := fs.String("output", "pretty", "Output: pretty | json")

	return &Command{
		Name:  "show",
		Short: "Show the latest saved report",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Latest Report")

			var report models.TransactionReport
			if err := config.LoadState("report_latest", &report); err != nil {
				ui.Warn("No report found. Run 'mozartpay report generate' first.")
				return nil
			}

			svc := reporting.NewService()
			if *output == "json" {
				out, _ := svc.FormatJSON(&report)
				fmt.Println(out)
			} else {
				fmt.Println(svc.FormatSummary(&report))
			}

			return nil
		},
	}
}

// ─── report iso20022 ─────────────────────────

func newReportISO20022Cmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("iso20022", flag.ContinueOnError)
	msgType := fs.String("type", "pacs.008", "Message type: pacs.008 | pacs.002 | pacs.004 | pacs.009")
	status := fs.String("status", "ACSC", "Transaction status for pacs.002 (ACSC|RJCT|PDNG)")
	reasonCode := fs.String("reason", "", "Reason code for pacs.002/pacs.004")

	return &Command{
		Name:  "iso20022",
		Short: "Export the latest report as ISO 20022 XML",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("ISO 20022 Export")

			var report models.TransactionReport
			if err := config.LoadState("report_latest", &report); err != nil {
				ui.Info("No report found. Generating from latest payment...")
				var payment models.Payment
				if err2 := config.LoadState("payment_latest", &payment); err2 != nil {
					payment = *demoPayment()
				}
				svc := reporting.NewService()
				r, _ := svc.GenerateReport(&payment, nil, cfg.ActiveDID, nil)
				report = *r
			}

			svc := reporting.NewService()
			var (
				xmlStr string
				err    error
			)

			switch *msgType {
			case "pacs.008":
				xmlStr, err = svc.FormatISO20022XML(&report)
				ui.SectionLabel("pacs.008.001.08")
			case "pacs.002":
				xmlStr, err = svc.FormatPacs002(&report, iso20022.TransactionStatus(*status), *reasonCode)
				ui.SectionLabel("pacs.002.001.12")
			case "pacs.004":
				xmlStr, err = svc.FormatPacs004(&report, *reasonCode)
				ui.SectionLabel("pacs.004.001.12")
			case "pacs.009":
				xmlStr, err = svc.FormatPacs009(&report)
				ui.SectionLabel("pacs.009.001.10")
			default:
				ui.Error(fmt.Sprintf("unsupported message type: %s", *msgType))
				return fmt.Errorf("unsupported message type: %s", *msgType)
			}

			if err != nil {
				ui.Error("Export failed: " + err.Error())
				return err
			}

			fmt.Println(xmlStr)

			return nil
		},
	}
}

// ─────────────────────────────────────────────
// Demo data helpers (used when no state exists)
// ─────────────────────────────────────────────

func demoPayment() *models.Payment {
	now := time.Now().UTC()
	confirmed := now.Add(5 * time.Second)
	return &models.Payment{
		ID:          "direct-demo0001",
		From:        "GDEMO_FROM0000000000000000000000000000000000000000000001",
		To:          "GDEMO_TO00000000000000000000000000000000000000000000000001",
		Amount:      "100.0000000",
		Asset:       "USDC",
		Rail:        models.RailDirect,
		Status:      models.PaymentConfirmed,
		Network:     models.NetworkStellarTestnet,
		Memo:        "Demo payment",
		Fee:         "0.00001 XLM",
		TxHash:      "DEMO" + "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2",
		LedgerSeq:   50123456,
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}
}

func demoAsset() *models.Asset {
	return &models.Asset{
		ID:          "demo-asset-0001",
		ContractID:  "CDEMO0000000000000000000000000000000000000000000000000001",
		Type:        models.AssetFungible,
		Name:        "Demo Token",
		Symbol:      "DEMO",
		Decimals:    7,
		TotalSupply: "1000000",
		Issuer:      "GDEMO_ISSUER000000000000000000000000000000000000000000001",
		Network:     models.NetworkStellarTestnet,
		Standard:    "SEP-41",
		CreatedAt:   time.Now().UTC(),
		TxHash:      "DEMO" + "B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3",
	}
}
