package reporting

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// Service generates post-transaction reports
type Service struct{}

func NewService() *Service { return &Service{} }

// GenerateReport creates a full post-transaction report
func (s *Service) GenerateReport(
	payment *models.Payment,
	asset *models.Asset,
	did string,
	vc *models.VerifiableCredential,
) (*models.TransactionReport, error) {
	report := &models.TransactionReport{
		ReportID:    "RPT-" + mpCrypto.RandomHex(8),
		GeneratedAt: time.Now().UTC(),
		Payment:     payment,
		Asset:       asset,
		DID:         did,
		VCAttached:  vc != nil,
		VC:          vc,
	}

	// Attach integrations data if present
	if asset != nil {
		report.CarbonOffset = asset.CarbonOffset
		report.Score = asset.Score
	}

	// Build ISO 20022 message
	if payment != nil {
		iso, err := s.buildISO20022(payment)
		if err == nil {
			report.ISO20022 = iso
		}
	}

	// Generate audit hash
	report.AuditHash = s.generateAuditHash(report)

	return report, nil
}

// FormatJSON renders the report as formatted JSON
func (s *Service) FormatJSON(report *models.TransactionReport) (string, error) {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FormatISO20022XML renders the ISO 20022 message as XML
func (s *Service) FormatISO20022XML(report *models.TransactionReport) (string, error) {
	if report.ISO20022 == nil {
		return "", fmt.Errorf("no ISO 20022 data in report")
	}
	if report.ISO20022.XML != "" {
		return report.ISO20022.XML, nil
	}
	if report.Payment == nil {
		return "", fmt.Errorf("no payment data to generate XML")
	}
	return iso20022.BuildPacs008(report.Payment, nil)
}

// FormatPacs002 generates a pacs.002 payment status report
func (s *Service) FormatPacs002(report *models.TransactionReport, status iso20022.TransactionStatus, reasonCode string) (string, error) {
	if report.Payment == nil {
		return "", fmt.Errorf("no payment data")
	}
	return iso20022.BuildPacs002(report.Payment, &iso20022.Pacs002Options{
		Status:     status,
		ReasonCode: reasonCode,
	})
}

// FormatPacs004 generates a pacs.004 payment reversal
func (s *Service) FormatPacs004(report *models.TransactionReport, reasonCode string) (string, error) {
	if report.Payment == nil {
		return "", fmt.Errorf("no payment data")
	}
	return iso20022.BuildPacs004(report.Payment, &iso20022.Pacs004Options{
		ReversalReasonCode: reasonCode,
	})
}

// FormatPacs009 generates a pacs.009 FI-to-FI direct debit
func (s *Service) FormatPacs009(report *models.TransactionReport) (string, error) {
	if report.Payment == nil {
		return "", fmt.Errorf("no payment data")
	}
	return iso20022.BuildPacs009(report.Payment, nil)
}

// FormatSummary renders a human-readable text summary
func (s *Service) FormatSummary(report *models.TransactionReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Report ID   : %s\n", report.ReportID))
	sb.WriteString(fmt.Sprintf("Generated   : %s\n", report.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("DID         : %s\n", report.DID))
	sb.WriteString(fmt.Sprintf("VC Attached : %v\n", report.VCAttached))
	sb.WriteString(fmt.Sprintf("Audit Hash  : %s\n", report.AuditHash))

	if report.Payment != nil {
		p := report.Payment
		sb.WriteString("\n── Payment ──────────────────────────\n")
		sb.WriteString(fmt.Sprintf("  TX Hash    : %s\n", p.TxHash))
		sb.WriteString(fmt.Sprintf("  From       : %s\n", p.From))
		if p.Network == models.NetworkStellarTestnet || p.Network == models.NetworkStellarMainnet {
			sb.WriteString(fmt.Sprintf("  From URL   : %s\n", stellarExplorerURL(p.From, p.Network)))
		}
		sb.WriteString(fmt.Sprintf("  To         : %s\n", p.To))
		if p.Network == models.NetworkStellarTestnet || p.Network == models.NetworkStellarMainnet {
			sb.WriteString(fmt.Sprintf("  To URL     : %s\n", stellarExplorerURL(p.To, p.Network)))
		}
		sb.WriteString(fmt.Sprintf("  Amount     : %s %s\n", p.Amount, p.Asset))
		sb.WriteString(fmt.Sprintf("  Rail       : %s\n", p.Rail))
		sb.WriteString(fmt.Sprintf("  Status     : %s\n", p.Status))
		if p.FXRate != "" {
			sb.WriteString(fmt.Sprintf("  FX Rate    : %s\n", p.FXRate))
		}
		sb.WriteString(fmt.Sprintf("  Fee        : %s\n", p.Fee))
		if p.ConfirmedAt != nil {
			sb.WriteString(fmt.Sprintf("  Confirmed  : %s\n", p.ConfirmedAt.Format(time.RFC3339)))
		}
	}

	if report.Asset != nil {
		a := report.Asset
		sb.WriteString("\n── Asset ────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("  Contract   : %s\n", a.ContractID))
		sb.WriteString(fmt.Sprintf("  Name       : %s (%s)\n", a.Name, a.Symbol))
		sb.WriteString(fmt.Sprintf("  Type       : %s\n", a.Type))
		sb.WriteString(fmt.Sprintf("  Standard   : %s\n", a.Standard))
		sb.WriteString(fmt.Sprintf("  Supply     : %s\n", a.TotalSupply))
	}

	if report.CarbonOffset != nil {
		c := report.CarbonOffset
		sb.WriteString("\n── Carbon Credit ────────────────────\n")
		sb.WriteString(fmt.Sprintf("  Token ID   : %s\n", c.TokenID))
		sb.WriteString(fmt.Sprintf("  Amount     : %.4f tCO2e\n", c.Amount))
		sb.WriteString(fmt.Sprintf("  Vintage    : %d\n", c.Vintage))
		sb.WriteString(fmt.Sprintf("  Standard   : %s\n", c.Standard))
		sb.WriteString(fmt.Sprintf("  Retired    : %v\n", c.Retired))
	}

	if report.Score != nil {
		sc := report.Score
		sb.WriteString("\n── Stablecoin Score ─────────────────\n")
		sb.WriteString(fmt.Sprintf("  Provider   : %s\n", sc.Provider))
		sb.WriteString(fmt.Sprintf("  Score      : %d / 1000\n", sc.Score))
		sb.WriteString(fmt.Sprintf("  Grade      : %s\n", sc.Grade))
		sb.WriteString(fmt.Sprintf("  Risk       : %s\n", sc.RiskLevel))
	}

	return sb.String()
}

// ─────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────

func (s *Service) buildISO20022(p *models.Payment) (*models.ISO20022Message, error) {
	msgID := "MZTP" + safeTruncateID(p.ID, 8)
	currency := p.Asset
	if currency == "" {
		currency = "XLM"
	}

	xmlStr, err := iso20022.BuildPacs008(p, nil)
	if err != nil {
		return nil, fmt.Errorf("build pacs.008: %w", err)
	}

	return &models.ISO20022Message{
		MessageType:     iso20022.MsgPacs008,
		MessageID:       msgID,
		CreatedAt:       p.CreatedAt,
		InitiatingParty: "MozartPay",
		PaymentInfo: models.ISO20022PaymentInfo{
			PaymentID:    p.ID,
			Method:       "TRF",
			Amount:       p.Amount,
			Currency:     currency,
			CreditorName: truncate(p.To, 16),
			DebtorName:   truncate(p.From, 16),
			EndToEndID:   safeTruncateID(p.TxHash, 16),
		},
		XML: xmlStr,
	}, nil
}

func (s *Service) generateAuditHash(report *models.TransactionReport) string {
	parts := []string{report.ReportID, report.DID, report.GeneratedAt.String()}
	if report.Payment != nil {
		parts = append(parts, report.Payment.TxHash)
	}
	return mpCrypto.Hash256([]byte(strings.Join(parts, ":")))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func safeTruncateID(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func stellarExplorerURL(address string, network models.Network) string {
	switch network {
	case models.NetworkStellarTestnet:
		return fmt.Sprintf("https://stellar.expert/explorer/testnet/account/%s", address)
	case models.NetworkStellarMainnet:
		return fmt.Sprintf("https://stellar.expert/explorer/public/account/%s", address)
	default:
		return fmt.Sprintf("https://stellar.expert/explorer/testnet/account/%s", address)
	}
}
