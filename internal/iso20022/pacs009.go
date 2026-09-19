package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.009.001.10 — FICdtTrf (FI Credit Transfer)
// ─────────────────────────────────────────────

// Pacs009Document is the root XML document for pacs.009
type Pacs009Document struct {
	XMLName  xml.Name `xml:"Document"`
	Xmlns    string   `xml:"xmlns,attr"`
	FICdtTrf struct {
		XMLName     xml.Name                      `xml:"FICdtTrf"`
		GrpHdr      GroupHeader93                 `xml:"GrpHdr"`
		CdtTrfTxInf []CreditTransferTransaction45 `xml:"CdtTrfTxInf"`
	} `xml:"FICdtTrf"`
}

// CreditTransferTransaction45 — pacs.009.001.10 transaction info.
// XSD sequence (subset implemented):
//
//	PmtId, PmtTpInf?, IntrBkSttlmAmt, IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?,
//	SttlmTmReq?, AccptncDtTm?, PoolgAdjstmntDt?, InstdAmt?, XchgRate?, XchgRateInf?,
//	ChrgBr, ChrgsInf*, MndtRltdInf?, PrvsInstgAgt1..3(+Acct)?, IntrmyAgt1..3(+Acct)?,
//	InstgAgt?, InstdAgt?, Dbtr, DbtrAcct?, DbtrAgt?, DbtrAgtAcct?, CdtrAgt?,
//	CdtrAgtAcct?, Cdtr, CdtrAcct?, UltmtDbtr?, UltmtCdtr?, InstrForCdtrAgt*,
//	InstrForNxtAgt*, Purp?, RgltryRptg*, RltdRmtInf*, RmtInf?, NclsdFile?, SplmtryData*
//
// NOTE: Dbtr/Cdtr are BranchAndFinancialInstitutionIdentification6 (financial
// institutions), not parties — and both are mandatory.
type CreditTransferTransaction45 struct {
	PmtId          *PaymentIdentification7                       `xml:"PmtId"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	InstdAmt       *ActiveOrHistoricCurrencyAndAmount            `xml:"InstdAmt,omitempty"`
	ChrgBr         string                                        `xml:"ChrgBr"`
	ChrgsInf       []ChargesInformation1                         `xml:"ChrgsInf,omitempty"`
	InstgAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	Dbtr           *BranchAndFinancialInstitutionIdentification6 `xml:"Dbtr"`
	DbtrAcct       *CashAccount38                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt,omitempty"`
	DbtrAgtAcct    *CashAccount38                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt,omitempty"`
	CdtrAgtAcct    *CashAccount38                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *BranchAndFinancialInstitutionIdentification6 `xml:"Cdtr"`
	CdtrAcct       *CashAccount38                                `xml:"CdtrAcct,omitempty"`
	Purp           *Purpose1Choice                               `xml:"Purp,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
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
	ChargeBearer   string
	PurposeCode    string
	RemittanceInfo []string
	UETR           string
	TxID           string
	InstrID        string
}

// BuildPacs009 builds a pacs.009.001.10 FI credit transfer
func BuildPacs009(p *models.Payment, opts *Pacs009Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs009Options{}
	}

	currency := p.Asset
	if currency == "" {
		currency = "XLM"
	}

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

	doc.FICdtTrf.GrpHdr = GroupHeader93{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction7{
			SttlmMtd: "CLRG",
		},
	}

	if opts.InstgBIC != "" {
		doc.FICdtTrf.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.FICdtTrf.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	txInf := CreditTransferTransaction45{
		PmtId: &PaymentIdentification7{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		IntrBkSttlmDt: sttlmDt,
	}

	if opts.ChargeBearer != "" {
		txInf.ChrgBr = opts.ChargeBearer
	} else {
		txInf.ChrgBr = string(ChrgBrDebt)
	}

	// Dbtr/Cdtr are mandatory financial institutions — BIC if provided,
	// otherwise Othr/Id carrying the Stellar address (Max35Text).
	txInf.Dbtr = agentOrFallback(opts.DbtrBIC, safeTruncate(p.From, 35))
	txInf.Cdtr = agentOrFallback(opts.CdtrBIC, safeTruncate(p.To, 35))

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount38{
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
		txInf.CdtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.PurposeCode != "" {
		txInf.Purp = &Purpose1Choice{Cd: opts.PurposeCode}
	}

	if len(opts.RemittanceInfo) > 0 {
		txInf.RmtInf = &RemittanceInformation2{Ustrd: opts.RemittanceInfo}
	} else if p.Memo != "" {
		txInf.RmtInf = &RemittanceInformation2{Ustrd: []string{p.Memo}}
	}

	doc.FICdtTrf.CdtTrfTxInf = []CreditTransferTransaction45{txInf}

	return MarshalXML(doc, NSPacs009)
}
