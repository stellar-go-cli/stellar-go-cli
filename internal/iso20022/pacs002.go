package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.002.001.12 — FIToFIPmtStsRpt
// ─────────────────────────────────────────────

// Pacs002Document is the root XML document for pacs.002
type Pacs002Document struct {
	XMLName         xml.Name `xml:"Document"`
	Xmlns           string   `xml:"xmlns,attr"`
	FIToFIPmtStsRpt struct {
		XMLName     xml.Name                `xml:"FIToFIPmtStsRpt"`
		GrpHdr      GroupHeader91           `xml:"GrpHdr"`
		OrgnlGrpInf *OriginalGroupHeader17  `xml:"OrgnlGrpInf,omitempty"`
		TxInfAndSts []PaymentTransaction110 `xml:"TxInfAndSts"`
	} `xml:"FIToFIPmtStsRpt"`
}

// PaymentTransaction110 — pacs.002.001.12 transaction status info.
// XSD sequence (subset implemented):
//
//	StsId?, OrgnlGrpInf?, OrgnlInstrId?, OrgnlEndToEndId?, OrgnlTxId?, OrgnlUETR?,
//	TxSts?, StsRsnInf*, ChrgsInf*, AccptncDtTm?, FctvIntrBkSttlmDt?, AcctSvcrRef?,
//	SvrlgAcctSvcr?, SvrlgIntrmyAgt1..3?, InstgAgt?, InstdAgt?, OrgnlTxRef?,
//	SplmtryData*, NclsdFile?
type PaymentTransaction110 struct {
	XMLName         xml.Name                                      `xml:"TxInfAndSts"`
	StsId           string                                        `xml:"StsId,omitempty"`
	OrgnlGrpInf     *OriginalGroupHeader17                        `xml:"OrgnlGrpInf,omitempty"`
	OrgnlInstrId    string                                        `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId string                                        `xml:"OrgnlEndToEndId,omitempty"`
	OrgnlTxId       string                                        `xml:"OrgnlTxId,omitempty"`
	OrgnlUETR       string                                        `xml:"OrgnlUETR,omitempty"`
	TxSts           string                                        `xml:"TxSts,omitempty"`
	StsRsnInf       []StatusReasonInformation12                   `xml:"StsRsnInf,omitempty"`
	ChrgsInf        []ChargesInformation1                         `xml:"ChrgsInf,omitempty"`
	AccptncDtTm     string                                        `xml:"AccptncDtTm,omitempty"`
	InstgAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	OrgnlTxRef      *OriginalTransactionReference28               `xml:"OrgnlTxRef,omitempty"`
}

// StatusReasonInformation12 — reason for status.
// XSD sequence: Orgtr?, Rsn?{Cd|Prtry}, AddtlInf*
type StatusReasonInformation12 struct {
	XMLName  xml.Name                `xml:"StsRsnInf"`
	Orgtr    *PartyIdentification135 `xml:"Orgtr,omitempty"`
	Rsn      *StatusReason6Choice    `xml:"Rsn,omitempty"`
	AddtlInf []string                `xml:"AddtlInf,omitempty"`
}

// StatusReason6Choice — coded or proprietary status reason
type StatusReason6Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// Pacs002Options configures optional fields for pacs.002
type Pacs002Options struct {
	Status         TransactionStatus
	ReasonCode     string
	ReasonPrtry    string
	AdditionalInfo string
	InstgBIC       string
	InstdBIC       string
	OrgnlInstrId   string
	OrgnlTxId      string
	OrgnlUETR      string
	OrgnlMsgId     string // original pacs.008 MsgId for OrgnlGrpInf
	DbtrBIC        string
	CdtrBIC        string
	DbtrAcctIBAN   string
	CdtrAcctIBAN   string
}

// BuildPacs002 builds a pacs.002.001.12 payment status report
func BuildPacs002(p *models.Payment, opts *Pacs002Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs002Options{Status: TxStsACSC}
	}

	msgID := "MZTP" + safeTruncate(p.ID, 8) + "S"
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	endToEndID := safeTruncate(p.TxHash, 16)
	if endToEndID == "" {
		endToEndID = p.ID
	}

	currency := p.Asset
	if currency == "" {
		currency = "XLM"
	}

	doc := &Pacs002Document{
		Xmlns: NSPacs002,
	}

	doc.FIToFIPmtStsRpt.GrpHdr = GroupHeader91{
		MsgId:   msgID,
		CreDtTm: creDtTm,
	}

	if opts.InstgBIC != "" {
		doc.FIToFIPmtStsRpt.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.FIToFIPmtStsRpt.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	// OrgnlGrpInf references the original pacs.008 message
	orgnlMsgID := opts.OrgnlMsgId
	if orgnlMsgID == "" {
		orgnlMsgID = "MZTP" + safeTruncate(p.ID, 8)
	}
	doc.FIToFIPmtStsRpt.OrgnlGrpInf = &OriginalGroupHeader17{
		OrgnlMsgId:   orgnlMsgID,
		OrgnlMsgNmId: "pacs.008.001.08",
		OrgnlCreDtTm: p.CreatedAt.UTC().Format(time.RFC3339),
	}

	txInf := PaymentTransaction110{
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID,
		OrgnlTxId:       opts.OrgnlTxId,
		OrgnlUETR:       opts.OrgnlUETR,
		TxSts:           string(opts.Status),
		AccptncDtTm:     creDtTm,
	}

	if opts.ReasonCode != "" || opts.ReasonPrtry != "" {
		rsn := &StatusReason6Choice{Cd: opts.ReasonCode, Prtry: opts.ReasonPrtry}
		stsRsn := StatusReasonInformation12{Rsn: rsn}
		if opts.AdditionalInfo != "" {
			stsRsn.AddtlInf = []string{opts.AdditionalInfo}
		}
		txInf.StsRsnInf = []StatusReasonInformation12{stsRsn}
	}

	txInf.OrgnlTxRef = &OriginalTransactionReference28{
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		IntrBkSttlmDt: p.CreatedAt.UTC().Format("2006-01-02"),
		Dbtr:          &Party40Choice{Pty: &PartyIdentification135{Nm: safeTruncate(p.From, 16)}},
		Cdtr:          &Party40Choice{Pty: &PartyIdentification135{Nm: safeTruncate(p.To, 16)}},
	}

	if opts.DbtrAcctIBAN != "" {
		txInf.OrgnlTxRef.DbtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
	}
	if opts.DbtrBIC != "" {
		txInf.OrgnlTxRef.DbtrAgt = agentByBIC(opts.DbtrBIC)
	}
	if opts.CdtrBIC != "" {
		txInf.OrgnlTxRef.CdtrAgt = agentByBIC(opts.CdtrBIC)
	}
	if opts.CdtrAcctIBAN != "" {
		txInf.OrgnlTxRef.CdtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	doc.FIToFIPmtStsRpt.TxInfAndSts = []PaymentTransaction110{txInf}

	return MarshalXML(doc, NSPacs002)
}
