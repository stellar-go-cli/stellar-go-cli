package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.009.001.10 — FIToFICdtTrf (FI-to-FI Direct Debit)
// ─────────────────────────────────────────────

// Pacs009Document is the root XML document for pacs.009
type Pacs009Document struct {
	XMLName       xml.Name `xml:"Document"`
	Xmlns         string   `xml:"xmlns,attr"`
	FIToFICdtTrf  struct {
		XMLName     xml.Name                      `xml:"FIToFICdtTrf"`
		GrpHdr      GroupHeader                    `xml:"GrpHdr"`
		CdtTrfTxInf []Pacs009CreditTransferTransaction `xml:"CdtTrfTxInf"`
	} `xml:"FIToFICdtTrf"`
}

// Pacs009CreditTransferTransaction — direct debit transaction info
type Pacs009CreditTransferTransaction struct {
	XMLName         xml.Name                          `xml:"CdtTrfTxInf"`
	PmtId           *PaymentIdentification4            `xml:"PmtId"`
	IntrBkSttlmDt   string                            `xml:"IntrBkSttlmDt,omitempty"`
	IntrBkSttlmAmt  *ActiveOrHistoricCurrencyAndAmount `xml:"IntrBkSttlmAmt,omitempty"`
	ChrgBr          string                            `xml:"ChrgBr,omitempty"`
	InstgAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt>FinInstnId,omitempty"`
	InstdAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt>FinInstnId,omitempty"`
	DbtrAgt         *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt>FinInstnId,omitempty"`
	CdtrAgt         *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt>FinInstnId,omitempty"`
	Dbtr            *PartyIdentification135           `xml:"Dbtr,omitempty"`
	DbtrAcct        *CashAccount38                   `xml:"DbtrAcct,omitempty"`
	Cdtr            *PartyIdentification135           `xml:"Cdtr,omitempty"`
	CdtrAcct        *CashAccount38                   `xml:"CdtrAcct,omitempty"`
	Purp            *Purpose1Choice                  `xml:"Purp,omitempty"`
	RmtInf          *RemittanceInformation2          `xml:"RmtInf,omitempty"`
}

// Pacs009Options configures optional fields for pacs.009
type Pacs009Options struct {
	InstgBIC       string
	InstdBIC       string
	DbtrBIC        string
	CdtrBIC        string
	DbtrName       string
	CdtrName       string
	DbtrAcctIBAN   string
	CdtrAcctIBAN   string
	ChargeBearer   string
	PurposeCode    string
	RemittanceInfo []string
	UETR           string
	TxID           string
	InstrID        string
}

// BuildPacs009 builds a pacs.009.001.10 FI-to-FI direct debit
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

	doc.FIToFICdtTrf.GrpHdr = GroupHeader{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInformation1{
			SttlmMtd: "CLRG",
			SttlmDt:  sttlmDt,
		},
	}

	if opts.InstgBIC != "" {
		doc.FIToFICdtTrf.GrpHdr.InstgAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstgBIC,
		}
	}
	if opts.InstdBIC != "" {
		doc.FIToFICdtTrf.GrpHdr.InstdAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstdBIC,
		}
	}

	doc.FIToFICdtTrf.GrpHdr.InitgPty = &PartyIdentification135{
		Nm: "MozartPay",
	}

	txInf := Pacs009CreditTransferTransaction{
		PmtId: &PaymentIdentification4{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmDt: sttlmDt,
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
	}

	if opts.ChargeBearer != "" {
		txInf.ChrgBr = opts.ChargeBearer
	} else {
		txInf.ChrgBr = string(ChrgBrDebt)
	}

	if opts.DbtrBIC != "" {
		txInf.DbtrAgt = &BranchAndFinancialInstitutionIdentification6{BICFI: opts.DbtrBIC}
	}
	if opts.CdtrBIC != "" {
		txInf.CdtrAgt = &BranchAndFinancialInstitutionIdentification6{BICFI: opts.CdtrBIC}
	}

	dbtrName := opts.DbtrName
	if dbtrName == "" {
		dbtrName = safeTruncate(p.From, 16)
	}
	txInf.Dbtr = &PartyIdentification135{Nm: dbtrName}

	cdtrName := opts.CdtrName
	if cdtrName == "" {
		cdtrName = safeTruncate(p.To, 16)
	}
	txInf.Cdtr = &PartyIdentification135{Nm: cdtrName}

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
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

	doc.FIToFICdtTrf.CdtTrfTxInf = []Pacs009CreditTransferTransaction{txInf}

	return MarshalXML(doc, NSPacs009)
}
