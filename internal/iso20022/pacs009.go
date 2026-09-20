package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.009.001.13 — FICdtTrf (FI Credit Transfer)
// ─────────────────────────────────────────────

// Pacs009Document is the root XML document for pacs.009
type Pacs009Document struct {
	XMLName  xml.Name `xml:"Document"`
	Xmlns    string   `xml:"xmlns,attr"`
	FICdtTrf struct {
		XMLName     xml.Name                      `xml:"FICdtTrf"`
		GrpHdr      GroupHeader131                `xml:"GrpHdr"`
		CdtTrfTxInf []CreditTransferTransaction79 `xml:"CdtTrfTxInf"`
	} `xml:"FICdtTrf"`
}

// CreditTransferTransaction79 — pacs.009.001.13 transaction info.
// XSD sequence (subset implemented):
//
//	PmtId, PmtTpInf?, IntrBkSttlmAmt, IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?,
//	SttlmTmReq?, XpryDtTm?, PmtSgntr?, PrvsInstgAgt1..3(+Acct)?, InstgAgt?,
//	InstdAgt?, IntrmyAgt1..3(+Acct)?, UltmtDbtr?, Dbtr, DbtrAcct?, DbtrAgt?,
//	DbtrAgtAcct?, CdtrAgt?, CdtrAgtAcct?, Cdtr, CdtrAcct?, UltmtCdtr?,
//	InstrForCdtrAgt*, InstrForNxtAgt*, Purp?, RgltryRptg*, RmtInf?,
//	UndrlygAllcn*, UndrlygCstmrCdtTrf?, UndrlygFICdtTrf?, SplmtryData*
//
// NOTE: Dbtr/Cdtr are BranchAndFinancialInstitutionIdentification8 (financial
// institutions), not parties — and both are mandatory. No ChrgBr, ChrgsInf,
// InstdAmt or XchgRate in .13.
type CreditTransferTransaction79 struct {
	PmtId          *PaymentIdentification13                      `xml:"PmtId"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	InstgAgt       *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt       *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
	Dbtr           *BranchAndFinancialInstitutionIdentification8 `xml:"Dbtr"`
	DbtrAcct       *CashAccount40                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt,omitempty"`
	DbtrAgtAcct    *CashAccount40                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt,omitempty"`
	CdtrAgtAcct    *CashAccount40                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *BranchAndFinancialInstitutionIdentification8 `xml:"Cdtr"`
	CdtrAcct       *CashAccount40                                `xml:"CdtrAcct,omitempty"`
	Purp           *Purpose2Choice                               `xml:"Purp,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
	SplmtryData    []SupplementaryData1                          `xml:"SplmtryData,omitempty"`
}

// Pacs009Options configures optional fields for pacs.009
type Pacs009Options struct {
	InstgBIC       string
	InstdBIC       string
	DbtrBIC        string // debtor FI BIC
	CdtrBIC        string // creditor FI BIC
	DbtrAgtBIC     string // debtor's agent BIC
	CdtrAgtBIC     string // creditor's agent BIC
	DbtrAcctIBAN   string
	CdtrAcctIBAN   string
	PurposeCode    string
	RemittanceInfo []string
	UETR           string
	TxID           string
	InstrID        string
}

// BuildPacs009 builds a pacs.009.001.13 FI credit transfer
func BuildPacs009(p *models.Payment, opts *Pacs009Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs009Options{}
	}

	asset := p.Asset
	if asset == "" {
		asset = "XLM"
	}
	ccy, assetSuppl := settlementCurrency(asset, p.Amount)

	msgID := "MZTP" + safeTruncate(p.ID, 8) + "D"
	creDtTm := p.CreatedAt.UTC().Format(time.RFC3339)
	sttlmDt := p.CreatedAt.UTC().Format("2006-01-02")

	endToEndID := safeTruncate(p.TxHash, 16)
	if endToEndID == "" {
		endToEndID = p.ID
	}

	uetr := opts.UETR
	if uetr == "" {
		uetr = generateUUIDv4()
	}

	txID := opts.TxID
	if txID == "" {
		txID = p.ID
	}

	doc := &Pacs009Document{
		Xmlns: NSPacs009,
	}

	doc.FICdtTrf.GrpHdr = GroupHeader131{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction15{
			SttlmMtd: "CLRG",
		},
	}

	if opts.InstgBIC != "" {
		doc.FICdtTrf.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.FICdtTrf.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	txInf := CreditTransferTransaction79{
		PmtId: &PaymentIdentification13{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		IntrBkSttlmDt: sttlmDt,
	}
	if assetSuppl != nil {
		txInf.SplmtryData = []SupplementaryData1{*assetSuppl}
	}

	// Dbtr/Cdtr are mandatory financial institutions — BIC if provided,
	// otherwise Othr/Id carrying the Stellar address (Max35Text).
	txInf.Dbtr = agentOrFallback(opts.DbtrBIC, safeTruncate(p.From, 35))
	txInf.Cdtr = agentOrFallback(opts.CdtrBIC, safeTruncate(p.To, 35))

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
	}
	if opts.DbtrAgtBIC != "" {
		txInf.DbtrAgt = agentByBIC(opts.DbtrAgtBIC)
	}
	if opts.CdtrAgtBIC != "" {
		txInf.CdtrAgt = agentByBIC(opts.CdtrAgtBIC)
	}
	if opts.CdtrAcctIBAN != "" {
		txInf.CdtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.PurposeCode != "" {
		txInf.Purp = &Purpose2Choice{Cd: opts.PurposeCode}
	}

	if len(opts.RemittanceInfo) > 0 {
		txInf.RmtInf = &RemittanceInformation2{Ustrd: opts.RemittanceInfo}
	} else if p.Memo != "" {
		txInf.RmtInf = &RemittanceInformation2{Ustrd: []string{p.Memo}}
	}

	doc.FICdtTrf.CdtTrfTxInf = []CreditTransferTransaction79{txInf}

	return MarshalXML(doc, NSPacs009)
}
