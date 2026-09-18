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
	XMLName          xml.Name `xml:"Document"`
	Xmlns            string   `xml:"xmlns,attr"`
	FIToFIPmtStsRpt  struct {
		XMLName       xml.Name                     `xml:"FIToFIPmtStsRpt"`
		GrpHdr        GroupHeader                   `xml:"GrpHdr"`
		TxInfAndSts   []PaymentTransactionStatusReport `xml:"TxInfAndSts"`
	} `xml:"FIToFIPmtStsRpt"`
}

// PaymentTransactionStatusReport — individual transaction status
type PaymentTransactionStatusReport struct {
	XMLName        xml.Name `xml:"TxInfAndSts"`
	OrgnlInstrId   string   `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId string `xml:"OrgnlEndToEndId"`
	OrgnlTxId      string   `xml:"OrgnlTxId,omitempty"`
	TxSts          string   `xml:"TxSts,omitempty"`
	StsRsnInf      []StatusReasonInformation1 `xml:"StsRsnInf,omitempty"`
	AccptncDtTm   string   `xml:"AccptncDtTm,omitempty"`
	OrgnlTxRef     *OriginalTransactionReference1 `xml:"OrgnlTxRef,omitempty"`
}

// StatusReasonInformation1 — reason for status
type StatusReasonInformation1 struct {
	XMLName  xml.Name `xml:"StsRsnInf"`
	RsnnInf  *ReasonInformation1 `xml:"RsnnInf,omitempty"`
}

// ReasonInformation1 — reason code + additional info
type ReasonInformation1 struct {
	XMLName xml.Name `xml:"RsnnInf"`
	Rsn     string   `xml:"Rsn>Cd,omitempty"`
	AddtlInf string `xml:"AddtlInf,omitempty"`
}

// OriginalTransactionReference1 — reference to original transaction
type OriginalTransactionReference1 struct {
	XMLName         xml.Name `xml:"OrgnlTxRef"`
	IntrBkSttlmAmt  *ActiveOrHistoricCurrencyAndAmount `xml:"IntrBkSttlmAmt,omitempty"`
	Dbtr            *PartyIdentification135 `xml:"Dbtr,omitempty"`
	Cdtr            *PartyIdentification135 `xml:"Cdtr,omitempty"`
	DbtrAgt         *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt>FinInstnId,omitempty"`
	CdtrAgt         *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt>FinInstnId,omitempty"`
}

// Pacs002Options configures optional fields for pacs.002
type Pacs002Options struct {
	Status          TransactionStatus
	ReasonCode     string
	AdditionalInfo string
	InstgBIC       string
	InstdBIC       string
	OrgnlInstrId   string
	OrgnlTxId      string
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

	doc.FIToFIPmtStsRpt.GrpHdr = GroupHeader{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
	}

	if opts.InstgBIC != "" {
		doc.FIToFIPmtStsRpt.GrpHdr.InstgAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstgBIC,
		}
	}
	if opts.InstdBIC != "" {
		doc.FIToFIPmtStsRpt.GrpHdr.InstdAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstdBIC,
		}
	}

	doc.FIToFIPmtStsRpt.GrpHdr.InitgPty = &PartyIdentification135{
		Nm: "MozartPay",
	}

	txInf := PaymentTransactionStatusReport{
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID,
		OrgnlTxId:       opts.OrgnlTxId,
		TxSts:           string(opts.Status),
		AccptncDtTm:     creDtTm,
	}

	if opts.ReasonCode != "" {
		txInf.StsRsnInf = []StatusReasonInformation1{
			{
				RsnnInf: &ReasonInformation1{
					Rsn:      opts.ReasonCode,
					AddtlInf: opts.AdditionalInfo,
				},
			},
		}
	}

	txInf.OrgnlTxRef = &OriginalTransactionReference1{
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		Dbtr: &PartyIdentification135{Nm: safeTruncate(p.From, 16)},
		Cdtr: &PartyIdentification135{Nm: safeTruncate(p.To, 16)},
	}

	doc.FIToFIPmtStsRpt.TxInfAndSts = []PaymentTransactionStatusReport{txInf}

	return MarshalXML(doc, NSPacs002)
}
