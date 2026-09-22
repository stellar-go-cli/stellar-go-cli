package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// pacs.002.001.16 — FIToFIPmtStsRpt
// ─────────────────────────────────────────────

// Pacs002Document is the root XML document for pacs.002
type Pacs002Document struct {
	XMLName         xml.Name `xml:"Document"`
	Xmlns           string   `xml:"xmlns,attr"`
	FIToFIPmtStsRpt struct {
		XMLName           xml.Name                `xml:"FIToFIPmtStsRpt"`
		GrpHdr            GroupHeader120          `xml:"GrpHdr"`
		OrgnlGrpInfAndSts *OriginalGroupHeader22  `xml:"OrgnlGrpInfAndSts,omitempty"`
		TxInfAndSts       []PaymentTransaction177 `xml:"TxInfAndSts"`
	} `xml:"FIToFIPmtStsRpt"`
}

// PaymentTransaction177 — pacs.002.001.16 transaction status info.
// XSD sequence (subset implemented):
//
//	StsId?, OrgnlGrpInf?, OrgnlInstrId?, OrgnlEndToEndId?, OrgnlTxId?, OrgnlUETR?,
//	TxSts?, StsRsnInf*, ChrgsInf*, AccptncDtTm?, PrcgDt?, FctvIntrBkSttlmDt?,
//	AcctSvcrRef?, ClrSysRef?, CdtSttlmKey?, InstgAgt?, InstdAgt?, OrgnlTxRef?,
//	SplmtryData*
type PaymentTransaction177 struct {
	XMLName         xml.Name                                      `xml:"TxInfAndSts"`
	StsId           string                                        `xml:"StsId,omitempty"`
	OrgnlGrpInf     *OriginalGroupInformation33                   `xml:"OrgnlGrpInf,omitempty"`
	OrgnlInstrId    string                                        `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId string                                        `xml:"OrgnlEndToEndId,omitempty"`
	OrgnlTxId       string                                        `xml:"OrgnlTxId,omitempty"`
	OrgnlUETR       string                                        `xml:"OrgnlUETR,omitempty"`
	TxSts           string                                        `xml:"TxSts,omitempty"`
	StsRsnInf       []StatusReasonInformation14                   `xml:"StsRsnInf,omitempty"`
	ChrgsInf        []Charges16                                   `xml:"ChrgsInf,omitempty"`
	AccptncDtTm     string                                        `xml:"AccptncDtTm,omitempty"`
	InstgAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
	OrgnlTxRef      *OriginalTransactionReference45               `xml:"OrgnlTxRef,omitempty"`
	SplmtryData     []SupplementaryData1                          `xml:"SplmtryData,omitempty"`
}

// StatusReasonInformation14 — reason for status.
// XSD sequence: Orgtr?, Rsn?{Cd|Prtry}, AddtlInf*
type StatusReasonInformation14 struct {
	XMLName  xml.Name                `xml:"StsRsnInf"`
	Orgtr    *PartyIdentification272 `xml:"Orgtr,omitempty"`
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

// BuildPacs002 builds a pacs.002.001.16 payment status report
func BuildPacs002(p *models.Payment, opts *Pacs002Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	return buildPacs002([]*CreditTransferInstruction{{Payment: p}}, opts)
}

// BuildPacs002Batch builds a pacs.002.001.16 with one TxInfAndSts per
// instruction. Per-transaction status comes from each instruction's
// TxStatus/TxReason, falling back to opts.Status/opts.ReasonCode.
func BuildPacs002Batch(instrs []*CreditTransferInstruction, opts *Pacs002Options) (string, error) {
	if err := requireInstructions(instrs, "BuildPacs002Batch"); err != nil {
		return "", err
	}
	return buildPacs002(instrs, opts)
}

func buildPacs002(instrs []*CreditTransferInstruction, opts *Pacs002Options) (string, error) {
	if opts == nil {
		opts = &Pacs002Options{Status: TxStsACSC}
	}
	if opts.Status == "" {
		opts.Status = TxStsACSC
	}

	first := instrs[0].Payment
	msgID := "SGC1" + safeTruncate(first.ID, 8) + "S"
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	doc := &Pacs002Document{Xmlns: NSPacs002}
	doc.FIToFIPmtStsRpt.GrpHdr = GroupHeader120{
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
		orgnlMsgID = "SGC1" + safeTruncate(first.ID, 8)
	}
	doc.FIToFIPmtStsRpt.OrgnlGrpInfAndSts = &OriginalGroupHeader22{
		OrgnlMsgId:   orgnlMsgID,
		OrgnlMsgNmId: "pacs.008.001.14",
		OrgnlCreDtTm: first.CreatedAt.UTC().Format(time.RFC3339),
	}

	txns := make([]PaymentTransaction177, 0, len(instrs))
	for _, instr := range instrs {
		txns = append(txns, *pacs002TxInf(instr, opts, creDtTm))
	}
	doc.FIToFIPmtStsRpt.TxInfAndSts = txns

	return MarshalXML(doc, NSPacs002)
}

func pacs002TxInf(instr *CreditTransferInstruction, opts *Pacs002Options, creDtTm string) *PaymentTransaction177 {
	p := instr.Payment
	asset := paymentAsset(p)
	ccy, assetSuppl := settlementCurrency(asset, p.AssetIssuer, p.Amount)

	status := opts.Status
	if instr.TxStatus != "" {
		status = instr.TxStatus
	}
	reason := opts.ReasonCode
	if instr.TxReason != "" {
		reason = instr.TxReason
	}

	txInf := &PaymentTransaction177{
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID(p),
		OrgnlTxId:       opts.OrgnlTxId,
		OrgnlUETR:       opts.OrgnlUETR,
		TxSts:           string(status),
		AccptncDtTm:     creDtTm,
	}

	if reason != "" || opts.ReasonPrtry != "" {
		rsn := &StatusReason6Choice{Cd: reason, Prtry: opts.ReasonPrtry}
		stsRsn := StatusReasonInformation14{Rsn: rsn}
		if opts.AdditionalInfo != "" {
			stsRsn.AddtlInf = []string{opts.AdditionalInfo}
		}
		txInf.StsRsnInf = []StatusReasonInformation14{stsRsn}
	}

	if assetSuppl != nil {
		txInf.SplmtryData = []SupplementaryData1{*assetSuppl}
	}

	txInf.OrgnlTxRef = &OriginalTransactionReference45{
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		IntrBkSttlmDt: p.CreatedAt.UTC().Format("2006-01-02"),
		Dbtr:          &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.From, 16)}},
		Cdtr:          &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.To, 16)}},
	}

	if opts.DbtrAcctIBAN != "" {
		txInf.OrgnlTxRef.DbtrAcct = &CashAccount40{
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
		txInf.OrgnlTxRef.CdtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	return txInf
}
