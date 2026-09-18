package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.004.001.12 — PmtRvsl (Payment Reversal)
// ─────────────────────────────────────────────

// Pacs004Document is the root XML document for pacs.004
type Pacs004Document struct {
	XMLName xml.Name `xml:"Document"`
	Xmlns   string   `xml:"xmlns,attr"`
	PmtRvsl struct {
		XMLName xml.Name             `xml:"PmtRvsl"`
		GrpHdr  GroupHeader          `xml:"GrpHdr"`
		TxInf   []ReversalTransaction `xml:"TxInf"`
	} `xml:"PmtRvsl"`
}

// ReversalTransaction — single reversal transaction info
type ReversalTransaction struct {
	XMLName              xml.Name                          `xml:"TxInf"`
	RvslId               string                            `xml:"RvslId,omitempty"`
	OrgnlInstrId         string                            `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId      string                            `xml:"OrgnlEndToEndId"`
	OrgnlTxId            string                            `xml:"OrgnlTxId,omitempty"`
	RvslRsnInf           []ReversalReasonInformation1      `xml:"RvslRsnInf,omitempty"`
	OrgnlIntrBkSttlmDt   string                            `xml:"OrgnlIntrBkSttlmDt,omitempty"`
	OrgnlIntrBkSttlmAmt  *ActiveOrHistoricCurrencyAndAmount `xml:"OrgnlIntrBkSttlmAmt,omitempty"`
	RvslIntrBkSttlmAmt   *ActiveOrHistoricCurrencyAndAmount `xml:"RvslIntrBkSttlmAmt,omitempty"`
	Dbtr                 *PartyIdentification135           `xml:"Dbtr,omitempty"`
	Cdtr                 *PartyIdentification135           `xml:"Cdtr,omitempty"`
	DbtrAgt              *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt>FinInstnId,omitempty"`
	CdtrAgt              *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt>FinInstnId,omitempty"`
	RmtInf               *RemittanceInformation2          `xml:"RmtInf,omitempty"`
}

// ReversalReasonInformation1 — reason for reversal
type ReversalReasonInformation1 struct {
	XMLName xml.Name `xml:"RvslRsnInf"`
	Rsn     string   `xml:"Rsn>Cd,omitempty"`
	Prtry   string   `xml:"Rsn>Prtry,omitempty"`
	AddtlInf string  `xml:"AddtlInf,omitempty"`
}

// Pacs004Options configures optional fields for pacs.004
type Pacs004Options struct {
	ReversalReasonCode string
	ReversalReasonPrtry string
	AdditionalInfo      string
	InstgBIC           string
	InstdBIC           string
	OrgnlInstrId       string
	OrgnlTxId          string
}

// BuildPacs004 builds a pacs.004.001.12 payment reversal
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

	doc.PmtRvsl.GrpHdr = GroupHeader{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInformation1{
			SttlmMtd: "CLRG",
			SttlmDt:  sttlmDt,
		},
	}

	if opts.InstgBIC != "" {
		doc.PmtRvsl.GrpHdr.InstgAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstgBIC,
		}
	}
	if opts.InstdBIC != "" {
		doc.PmtRvsl.GrpHdr.InstdAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstdBIC,
		}
	}

	doc.PmtRvsl.GrpHdr.InitgPty = &PartyIdentification135{
		Nm: "MozartPay",
	}

	txInf := ReversalTransaction{
		RvslId:             "RVSL" + safeTruncate(p.ID, 8),
		OrgnlInstrId:       opts.OrgnlInstrId,
		OrgnlEndToEndId:    endToEndID,
		OrgnlTxId:          opts.OrgnlTxId,
		OrgnlIntrBkSttlmDt: sttlmDt,
		OrgnlIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		RvslIntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		Dbtr: &PartyIdentification135{Nm: safeTruncate(p.From, 16)},
		Cdtr: &PartyIdentification135{Nm: safeTruncate(p.To, 16)},
	}

	if opts.ReversalReasonCode != "" || opts.ReversalReasonPrtry != "" {
		txInf.RvslRsnInf = []ReversalReasonInformation1{
			{
				Rsn:      opts.ReversalReasonCode,
				Prtry:    opts.ReversalReasonPrtry,
				AddtlInf: opts.AdditionalInfo,
			},
		}
	}

	if p.Memo != "" {
		txInf.RmtInf = &RemittanceInformation2{
			Ustrd: []string{p.Memo},
		}
	}

	doc.PmtRvsl.TxInf = []ReversalTransaction{txInf}

	return MarshalXML(doc, NSPacs004)
}
