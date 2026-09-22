package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// pacs.004.001.15 — PmtRtr (Payment Return)
// ─────────────────────────────────────────────

// Pacs004Document is the root XML document for pacs.004
type Pacs004Document struct {
	XMLName xml.Name `xml:"Document"`
	Xmlns   string   `xml:"xmlns,attr"`
	PmtRtr  struct {
		XMLName     xml.Name                `xml:"PmtRtr"`
		GrpHdr      GroupHeader123          `xml:"GrpHdr"`
		OrgnlGrpInf *OriginalGroupHeader19  `xml:"OrgnlGrpInf,omitempty"`
		TxInf       []PaymentTransaction168 `xml:"TxInf"`
	} `xml:"PmtRtr"`
}

// PaymentTransaction168 — pacs.004.001.15 return transaction info.
// XSD sequence (subset implemented):
//
//	RtrId?, OrgnlGrpInf?, OrgnlInstrId?, OrgnlEndToEndId?, OrgnlTxId?, OrgnlUETR?,
//	OrgnlClrSysRef?, OrgnlIntrBkSttlmAmt?, OrgnlIntrBkSttlmDt?, PmtTpInf?,
//	RtrdIntrBkSttlmAmt, IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?, SttlmTmReq?,
//	RtrdInstdAmt?, XchgRate?, AgrdRate?, CompstnAmt?, ChrgBr?, ChrgsInf*,
//	ClrSysRef?, InstgAgt?, InstdAgt?, RtrChain?, RtrRsnInf*, OrgnlTxRef?,
//	SplmtryData*
//
// NOTE: RtrdIntrBkSttlmAmt is mandatory in .15.
type PaymentTransaction168 struct {
	XMLName             xml.Name                                      `xml:"TxInf"`
	RtrId               string                                        `xml:"RtrId,omitempty"`
	OrgnlGrpInf         *OriginalGroupInformation33                   `xml:"OrgnlGrpInf,omitempty"`
	OrgnlInstrId        string                                        `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId     string                                        `xml:"OrgnlEndToEndId,omitempty"`
	OrgnlTxId           string                                        `xml:"OrgnlTxId,omitempty"`
	OrgnlUETR           string                                        `xml:"OrgnlUETR,omitempty"`
	OrgnlIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"OrgnlIntrBkSttlmAmt,omitempty"`
	OrgnlIntrBkSttlmDt  string                                        `xml:"OrgnlIntrBkSttlmDt,omitempty"`
	RtrdIntrBkSttlmAmt  *ActiveOrHistoricCurrencyAndAmount            `xml:"RtrdIntrBkSttlmAmt"`
	IntrBkSttlmDt       string                                        `xml:"IntrBkSttlmDt,omitempty"`
	ChrgBr              string                                        `xml:"ChrgBr,omitempty"`
	ChrgsInf            []Charges16                                   `xml:"ChrgsInf,omitempty"`
	InstgAgt            *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt            *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
	RtrChain            *TransactionParties11                         `xml:"RtrChain,omitempty"`
	RtrRsnInf           []PaymentReturnReason7                        `xml:"RtrRsnInf,omitempty"`
	OrgnlTxRef          *OriginalTransactionReference45               `xml:"OrgnlTxRef,omitempty"`
	SplmtryData         []SupplementaryData1                          `xml:"SplmtryData,omitempty"`
}

// TransactionParties11 — return chain parties (pacs.004.001.15).
// XSD sequence: UltmtDbtr?, Dbtr, DbtrAcct?, InitgPty?, DbtrAgt?, DbtrAgtAcct?,
// PrvsInstgAgt1..3(+Acct)?, IntrmyAgt1..3(+Acct)?, CdtrAgt?, CdtrAgtAcct?,
// Cdtr, CdtrAcct?, UltmtCdtr?
// Dbtr/Cdtr are Party50Choice (Pty | Agt); mandatory.
type TransactionParties11 struct {
	XMLName     xml.Name                                      `xml:"RtrChain"`
	UltmtDbtr   *Party50Choice                                `xml:"UltmtDbtr,omitempty"`
	Dbtr        *Party50Choice                                `xml:"Dbtr"`
	DbtrAcct    *CashAccount40                                `xml:"DbtrAcct,omitempty"`
	InitgPty    *Party50Choice                                `xml:"InitgPty,omitempty"`
	DbtrAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt,omitempty"`
	DbtrAgtAcct *CashAccount40                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt,omitempty"`
	CdtrAgtAcct *CashAccount40                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr        *Party50Choice                                `xml:"Cdtr"`
	CdtrAcct    *CashAccount40                                `xml:"CdtrAcct,omitempty"`
	UltmtCdtr   *Party50Choice                                `xml:"UltmtCdtr,omitempty"`
}

// PaymentReturnReason7 — reason for return.
// XSD sequence: Orgtr?, Rsn?{Cd|Prtry}, AddtlInf*
type PaymentReturnReason7 struct {
	XMLName  xml.Name                `xml:"RtrRsnInf"`
	Orgtr    *PartyIdentification272 `xml:"Orgtr,omitempty"`
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

// BuildPacs004 builds a pacs.004.001.15 payment return
func BuildPacs004(p *models.Payment, opts *Pacs004Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs004Options{}
	}

	msgID := "SGC1" + safeTruncate(p.ID, 8) + "R"
	creDtTm := time.Now().UTC().Format(time.RFC3339)
	sttlmDt := p.CreatedAt.UTC().Format("2006-01-02")

	endToEndID := safeTruncate(p.TxHash, 16)
	if endToEndID == "" {
		endToEndID = p.ID
	}

	asset := p.Asset
	if asset == "" {
		asset = "XLM"
	}
	ccy, assetSuppl := settlementCurrency(asset, p.AssetIssuer, p.Amount)

	doc := &Pacs004Document{
		Xmlns: NSPacs004,
	}

	doc.PmtRtr.GrpHdr = GroupHeader123{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction15{
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
		orgnlMsgID = "SGC1" + safeTruncate(p.ID, 8)
	}
	doc.PmtRtr.OrgnlGrpInf = &OriginalGroupHeader19{
		OrgnlMsgId:   orgnlMsgID,
		OrgnlMsgNmId: "pacs.008.001.14",
		OrgnlCreDtTm: p.CreatedAt.UTC().Format(time.RFC3339),
	}

	txInf := PaymentTransaction168{
		RtrId:           "RTR" + safeTruncate(p.ID, 8),
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID,
		OrgnlTxId:       opts.OrgnlTxId,
		OrgnlUETR:       opts.OrgnlUETR,
		OrgnlIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		OrgnlIntrBkSttlmDt: sttlmDt,
		RtrdIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		IntrBkSttlmDt: sttlmDt,
	}
	if assetSuppl != nil {
		txInf.SplmtryData = []SupplementaryData1{*assetSuppl}
	}

	// RtrChain carries the original debtor/creditor parties
	txInf.RtrChain = &TransactionParties11{
		Dbtr: &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.From, 16)}},
		Cdtr: &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.To, 16)}},
	}
	if opts.DbtrAcctIBAN != "" {
		txInf.RtrChain.DbtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
	}
	if opts.CdtrAcctIBAN != "" {
		txInf.RtrChain.CdtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.ReversalReasonCode != "" || opts.ReversalReasonPrtry != "" {
		rsn := &ReturnReason5Choice{Cd: opts.ReversalReasonCode, Prtry: opts.ReversalReasonPrtry}
		rtrRsn := PaymentReturnReason7{Rsn: rsn}
		if opts.AdditionalInfo != "" {
			rtrRsn.AddtlInf = []string{opts.AdditionalInfo}
		}
		txInf.RtrRsnInf = []PaymentReturnReason7{rtrRsn}
	}

	doc.PmtRtr.TxInf = []PaymentTransaction168{txInf}

	return MarshalXML(doc, NSPacs004)
}
