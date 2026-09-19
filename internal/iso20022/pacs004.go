package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.004.001.12 — PmtRtr (Payment Return)
// ─────────────────────────────────────────────

// Pacs004Document is the root XML document for pacs.004
type Pacs004Document struct {
	XMLName xml.Name `xml:"Document"`
	Xmlns   string   `xml:"xmlns,attr"`
	PmtRtr  struct {
		XMLName     xml.Name                `xml:"PmtRtr"`
		GrpHdr      GroupHeader90           `xml:"GrpHdr"`
		OrgnlGrpInf *OriginalGroupHeader21  `xml:"OrgnlGrpInf,omitempty"`
		TxInf       []PaymentTransaction118 `xml:"TxInf"`
	} `xml:"PmtRtr"`
}

// PaymentTransaction118 — pacs.004.001.12 return transaction info.
// XSD sequence (subset implemented):
//
//	RtrId?, OrgnlGrpInf?, OrgnlInstrId?, OrgnlEndToEndId?, OrgnlTxId?, OrgnlUETR?,
//	OrgnlClrSysRef?, OrgnlIntrBkSttlmAmt?, OrgnlIntrBkSttlmDt?, RtrdIntrBkSttlmAmt?,
//	IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?, SttlmTmReq?, RtrdInstdAmt?,
//	XchgRate?, CmpstnAmt?, ChrgBr?, ChrgsInf*, InstgAgt?, InstdAgt?, RtrChain?,
//	RtrRsnInf*, OrgnlTxRef?, SplmtryData*
type PaymentTransaction118 struct {
	XMLName             xml.Name                                      `xml:"TxInf"`
	RtrId               string                                        `xml:"RtrId,omitempty"`
	OrgnlGrpInf         *OriginalGroupHeader17                        `xml:"OrgnlGrpInf,omitempty"`
	OrgnlInstrId        string                                        `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId     string                                        `xml:"OrgnlEndToEndId,omitempty"`
	OrgnlTxId           string                                        `xml:"OrgnlTxId,omitempty"`
	OrgnlUETR           string                                        `xml:"OrgnlUETR,omitempty"`
	OrgnlIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"OrgnlIntrBkSttlmAmt,omitempty"`
	OrgnlIntrBkSttlmDt  string                                        `xml:"OrgnlIntrBkSttlmDt,omitempty"`
	RtrdIntrBkSttlmAmt  *ActiveOrHistoricCurrencyAndAmount            `xml:"RtrdIntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt       string                                        `xml:"IntrBkSttlmDt,omitempty"`
	ChrgBr              string                                        `xml:"ChrgBr,omitempty"`
	ChrgsInf            []ChargesInformation1                         `xml:"ChrgsInf,omitempty"`
	InstgAgt            *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt            *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	RtrChain            *TransactionParties7                          `xml:"RtrChain,omitempty"`
	RtrRsnInf           []ReturnReasonInformation9                    `xml:"RtrRsnInf,omitempty"`
	OrgnlTxRef          *OriginalTransactionReference28               `xml:"OrgnlTxRef,omitempty"`
}

// TransactionParties7 — return chain parties (pacs.004).
// XSD sequence: UltmtDbtr?, Dbtr, DbtrAcct?, InitgPty?, InstgAgt?, InstdAgt?,
// IntrmyAgt1..3(+Acct)?, UltmtCdtr?, Cdtr, CdtrAcct?
// Dbtr/Cdtr are Party40Choice (Pty | Agt); mandatory.
type TransactionParties7 struct {
	XMLName   xml.Name                                      `xml:"RtrChain"`
	UltmtDbtr *Party40Choice                                `xml:"UltmtDbtr,omitempty"`
	Dbtr      *Party40Choice                                `xml:"Dbtr"`
	DbtrAcct  *CashAccount38                                `xml:"DbtrAcct,omitempty"`
	InitgPty  *Party40Choice                                `xml:"InitgPty,omitempty"`
	InstgAgt  *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt  *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	UltmtCdtr *Party40Choice                                `xml:"UltmtCdtr,omitempty"`
	Cdtr      *Party40Choice                                `xml:"Cdtr"`
	CdtrAcct  *CashAccount38                                `xml:"CdtrAcct,omitempty"`
}

// ReturnReasonInformation9 — reason for return.
// XSD sequence: Orgtr?, Rsn?{Cd|Prtry}, AddtlInf*
type ReturnReasonInformation9 struct {
	XMLName  xml.Name                `xml:"RtrRsnInf"`
	Orgtr    *PartyIdentification135 `xml:"Orgtr,omitempty"`
	Rsn      *ReturnReason5Choice    `xml:"Rsn,omitempty"`
	AddtlInf []string                `xml:"AddtlInf,omitempty"`
}

// ReturnReason5Choice — coded or proprietary return reason
type ReturnReason5Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// Pacs004Options configures optional fields for pacs.004
type Pacs004Options struct {
	ReversalReasonCode  string
	ReversalReasonPrtry string
	AdditionalInfo      string
	InstgBIC            string
	InstdBIC            string
	OrgnlInstrId        string
	OrgnlTxId           string
	OrgnlUETR           string
	OrgnlMsgId          string // original pacs.008 MsgId for OrgnlGrpInf
	DbtrBIC             string
	CdtrBIC             string
	DbtrAcctIBAN        string
	CdtrAcctIBAN        string
}

// BuildPacs004 builds a pacs.004.001.12 payment return
func BuildPacs004(p *models.Payment, opts *Pacs004Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs004Options{}
	}

	msgID := "MZTP" + safeTruncate(p.ID, 8) + "R"
	creDtTm := time.Now().UTC().Format(time.RFC3339)
	sttlmDt := p.CreatedAt.UTC().Format("2006-01-02")

	endToEndID := safeTruncate(p.TxHash, 16)
	if endToEndID == "" {
		endToEndID = p.ID
	}

	currency := p.Asset
	if currency == "" {
		currency = "XLM"
	}

	doc := &Pacs004Document{
		Xmlns: NSPacs004,
	}

	doc.PmtRtr.GrpHdr = GroupHeader90{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction7{
			SttlmMtd: "CLRG",
		},
	}

	if opts.InstgBIC != "" {
		doc.PmtRtr.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.PmtRtr.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	// OrgnlGrpInf references the original pacs.008 message
	orgnlMsgID := opts.OrgnlMsgId
	if orgnlMsgID == "" {
		orgnlMsgID = "MZTP" + safeTruncate(p.ID, 8)
	}
	doc.PmtRtr.OrgnlGrpInf = &OriginalGroupHeader21{
		OrgnlMsgId:   orgnlMsgID,
		OrgnlMsgNmId: "pacs.008.001.08",
		OrgnlCreDtTm: p.CreatedAt.UTC().Format(time.RFC3339),
		OrgnlNbOfTxs: "1",
	}

	txInf := PaymentTransaction118{
		RtrId:           "RTR" + safeTruncate(p.ID, 8),
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID,
		OrgnlTxId:       opts.OrgnlTxId,
		OrgnlUETR:       opts.OrgnlUETR,
		OrgnlIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		OrgnlIntrBkSttlmDt: sttlmDt,
		RtrdIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		IntrBkSttlmDt: sttlmDt,
	}

	// RtrChain carries the original debtor/creditor parties
	txInf.RtrChain = &TransactionParties7{
		Dbtr: &Party40Choice{Pty: &PartyIdentification135{Nm: safeTruncate(p.From, 16)}},
		Cdtr: &Party40Choice{Pty: &PartyIdentification135{Nm: safeTruncate(p.To, 16)}},
	}
	if opts.DbtrAcctIBAN != "" {
		txInf.RtrChain.DbtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
	}
	if opts.CdtrAcctIBAN != "" {
		txInf.RtrChain.CdtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.ReversalReasonCode != "" || opts.ReversalReasonPrtry != "" {
		rsn := &ReturnReason5Choice{Cd: opts.ReversalReasonCode, Prtry: opts.ReversalReasonPrtry}
		rtrRsn := ReturnReasonInformation9{Rsn: rsn}
		if opts.AdditionalInfo != "" {
			rtrRsn.AddtlInf = []string{opts.AdditionalInfo}
		}
		txInf.RtrRsnInf = []ReturnReasonInformation9{rtrRsn}
	}

	doc.PmtRtr.TxInf = []PaymentTransaction118{txInf}

	return MarshalXML(doc, NSPacs004)
}
