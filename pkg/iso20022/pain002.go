package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// pain.002.001.10 — CstmrPmtStsRpt
// ─────────────────────────────────────────────
//
// Customer payment status report: the message a bank sends back in reply to a
// pain.001 initiation file — per-payment acceptance/rejection status. This is
// the reconciliation counterpart of BuildPain001 for disbursement flows.

// Pain002Document is the root XML document for pain.002
type Pain002Document struct {
	XMLName        xml.Name `xml:"Document"`
	Xmlns          string   `xml:"xmlns,attr"`
	CstmrPmtStsRpt struct {
		XMLName           xml.Name                     `xml:"CstmrPmtStsRpt"`
		GrpHdr            Pain002GroupHeader           `xml:"GrpHdr"`
		OrgnlGrpInfAndSts *Pain002OriginalGroupInfo    `xml:"OrgnlGrpInfAndSts,omitempty"`
		OrgnlPmtInfAndSts []Pain002OriginalPaymentInfo `xml:"OrgnlPmtInfAndSts"`
	} `xml:"CstmrPmtStsRpt"`
}

// Pain002GroupHeader — pain.002.001.10 group header.
// XSD sequence: MsgId, CreDtTm, Authstn*, InitgPty, FwdgAgt?, DbtrAgt?, CdtrAgt?
type Pain002GroupHeader struct {
	MsgId    string                                        `xml:"MsgId"`
	CreDtTm  string                                        `xml:"CreDtTm"`
	InitgPty *PartyIdentification272                       `xml:"InitgPty,omitempty"`
	DbtrAgt  *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt,omitempty"`
	CdtrAgt  *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt,omitempty"`
}

// Pain002OriginalGroupInfo — pain.002.001.10 OrgnlGrpInfAndSts.
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?, OrgnlNbOfTxs?,
// OrgnlCtrlSum?, GrpSts?, StsRsnInf*, NbOfTxsPerSts*
type Pain002OriginalGroupInfo struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
	OrgnlNbOfTxs string `xml:"OrgnlNbOfTxs,omitempty"`
	OrgnlCtrlSum string `xml:"OrgnlCtrlSum,omitempty"`
	GrpSts       string `xml:"GrpSts,omitempty"`
}

// Pain002OriginalPaymentInfo — pain.002.001.10 OrgnlPmtInfAndSts: the status
// for one original pain.001 PmtInf and its transactions.
// XSD sequence: OrgnlPmtInfId, OrgnlNbOfTxs?, OrgnlCtrlSum?, PmtInfSts?,
// StsRsnInf*, NbOfTxsPerSts*, TxInfAndSts*
type Pain002OriginalPaymentInfo struct {
	OrgnlPmtInfId string               `xml:"OrgnlPmtInfId"`
	OrgnlNbOfTxs  string               `xml:"OrgnlNbOfTxs,omitempty"`
	OrgnlCtrlSum  string               `xml:"OrgnlCtrlSum,omitempty"`
	PmtInfSts     string               `xml:"PmtInfSts,omitempty"`
	TxInfAndSts   []Pain002Transaction `xml:"TxInfAndSts,omitempty"`
}

// Pain002Transaction — pain.002.001.10 TxInfAndSts.
// XSD sequence (subset): StsId?, OrgnlInstrId?, OrgnlEndToEndId?, OrgnlUETR?,
// TxSts?, StsRsnInf*, ChrgsInf*, AccptncDtTm?, ..., OrgnlTxRef?, SplmtryData*
type Pain002Transaction struct {
	StsId           string                          `xml:"StsId,omitempty"`
	OrgnlInstrId    string                          `xml:"OrgnlInstrId,omitempty"`
	OrgnlEndToEndId string                          `xml:"OrgnlEndToEndId,omitempty"`
	OrgnlUETR       string                          `xml:"OrgnlUETR,omitempty"`
	TxSts           string                          `xml:"TxSts,omitempty"`
	StsRsnInf       []StatusReasonInformation14     `xml:"StsRsnInf,omitempty"`
	AccptncDtTm     string                          `xml:"AccptncDtTm,omitempty"`
	OrgnlTxRef      *OriginalTransactionReference45 `xml:"OrgnlTxRef,omitempty"`
}

// Pain002Options configures a pain.002 status report.
type Pain002Options struct {
	MsgID           string
	Status          TransactionStatus // default TxStsACSC; per-tx override via instr.TxStatus
	GroupStatus     string            // GrpSts — derived from tx statuses when empty
	ReasonCode      string
	ReasonPrtry     string
	AdditionalInfo  string
	OrgnlMsgId      string // original pain.001 MsgId
	OrgnlPmtInfId   string // original pain.001 PmtInfId
	OrgnlCreDtTm    string // original pain.001 CreDtTm
	OrgnlInstrId    string
	OrgnlUETR       string
	InitiatingParty *Party // InitgPty (the agent reporting status)
	DbtrBIC         string
	CdtrBIC         string
}

// BuildPain002 builds a pain.002.001.10 customer payment status report for a
// single payment — typically in reply to a pain.001.
func BuildPain002(p *models.Payment, opts *Pain002Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	return buildPain002([]*CreditTransferInstruction{{Payment: p}}, opts)
}

// BuildPain002Batch builds a pain.002.001.10 with one TxInfAndSts per
// instruction, inside a single OrgnlPmtInfAndSts block. Per-transaction
// status comes from each instruction's TxStatus/TxReason, falling back to
// opts.Status/opts.ReasonCode.
func BuildPain002Batch(instrs []*CreditTransferInstruction, opts *Pain002Options) (string, error) {
	if err := requireInstructions(instrs, "BuildPain002Batch"); err != nil {
		return "", err
	}
	return buildPain002(instrs, opts)
}

// groupStatus derives GrpSts: the common status when all transactions share
// it, "PART" for mixed outcomes.
func groupStatus(statuses []string) string {
	if len(statuses) == 0 {
		return ""
	}
	first := statuses[0]
	for _, s := range statuses[1:] {
		if s != first {
			return "PART"
		}
	}
	return first
}

func buildPain002(instrs []*CreditTransferInstruction, opts *Pain002Options) (string, error) {
	if opts == nil {
		opts = &Pain002Options{Status: TxStsACSC}
	}
	if opts.Status == "" {
		opts.Status = TxStsACSC
	}

	first := instrs[0].Payment
	msgID := opts.MsgID
	if msgID == "" {
		msgID = "SGC1S" + safeTruncate(first.ID, 7)
	}
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	doc := &Pain002Document{Xmlns: NSPain002}
	doc.CstmrPmtStsRpt.GrpHdr = Pain002GroupHeader{
		MsgId:   msgID,
		CreDtTm: creDtTm,
	}
	if opts.InitiatingParty != nil {
		doc.CstmrPmtStsRpt.GrpHdr.InitgPty = partyIdentification(opts.InitiatingParty, "NOTPROVIDED")
	}
	if opts.DbtrBIC != "" {
		doc.CstmrPmtStsRpt.GrpHdr.DbtrAgt = agentByBIC(opts.DbtrBIC)
	}
	if opts.CdtrBIC != "" {
		doc.CstmrPmtStsRpt.GrpHdr.CdtrAgt = agentByBIC(opts.CdtrBIC)
	}

	// Build per-transaction statuses
	txns := make([]Pain002Transaction, 0, len(instrs))
	statuses := make([]string, 0, len(instrs))
	for _, instr := range instrs {
		tx := pain002TxInf(instr, opts, creDtTm)
		txns = append(txns, *tx)
		statuses = append(statuses, tx.TxSts)
	}

	// OrgnlGrpInfAndSts references the original pain.001 message
	orgnlMsgID := opts.OrgnlMsgId
	if orgnlMsgID == "" {
		orgnlMsgID = "SGC1P" + safeTruncate(first.ID, 7)
	}
	grpSts := opts.GroupStatus
	if grpSts == "" {
		grpSts = groupStatus(statuses)
	}
	doc.CstmrPmtStsRpt.OrgnlGrpInfAndSts = &Pain002OriginalGroupInfo{
		OrgnlMsgId:   orgnlMsgID,
		OrgnlMsgNmId: "pain.001.001.13",
		OrgnlCreDtTm: opts.OrgnlCreDtTm,
		OrgnlNbOfTxs: fmt.Sprintf("%d", len(instrs)),
		GrpSts:       grpSts,
	}

	orgnlPmtInfID := opts.OrgnlPmtInfId
	if orgnlPmtInfID == "" {
		orgnlPmtInfID = orgnlMsgID + "-1"
	}
	doc.CstmrPmtStsRpt.OrgnlPmtInfAndSts = []Pain002OriginalPaymentInfo{{
		OrgnlPmtInfId: orgnlPmtInfID,
		OrgnlNbOfTxs:  fmt.Sprintf("%d", len(instrs)),
		TxInfAndSts:   txns,
	}}

	return MarshalXML(doc, NSPain002)
}

// pain002TxInf builds one TxInfAndSts for an instruction.
func pain002TxInf(instr *CreditTransferInstruction, opts *Pain002Options, creDtTm string) *Pain002Transaction {
	p := instr.Payment
	asset := paymentAsset(p)
	ccy, _ := settlementCurrency(asset, p.AssetIssuer, p.Amount)

	status := opts.Status
	if instr.TxStatus != "" {
		status = instr.TxStatus
	}
	reason := opts.ReasonCode
	if instr.TxReason != "" {
		reason = instr.TxReason
	}

	txInf := &Pain002Transaction{
		OrgnlInstrId:    opts.OrgnlInstrId,
		OrgnlEndToEndId: endToEndID(p),
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

	txInf.OrgnlTxRef = &OriginalTransactionReference45{
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		IntrBkSttlmDt: p.CreatedAt.UTC().Format("2006-01-02"),
		Dbtr:          &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.From, 16)}},
		Cdtr:          &Party50Choice{Pty: &PartyIdentification272{Nm: safeTruncate(p.To, 16)}},
	}

	return txInf
}

// StatusReports returns one StatusReport per transaction across all
// OrgnlPmtInfAndSts blocks — the reconciliation entry point for a pain.002.
func (d *Pain002Document) StatusReports() []StatusReport {
	var origMsgID string
	if d.CstmrPmtStsRpt.OrgnlGrpInfAndSts != nil {
		origMsgID = d.CstmrPmtStsRpt.OrgnlGrpInfAndSts.OrgnlMsgId
	}
	var reports []StatusReport
	for _, pmtInf := range d.CstmrPmtStsRpt.OrgnlPmtInfAndSts {
		for i := range pmtInf.TxInfAndSts {
			t := &pmtInf.TxInfAndSts[i]
			r := StatusReport{
				StsID:         t.StsId,
				EndToEndID:    t.OrgnlEndToEndId,
				InstrID:       t.OrgnlInstrId,
				Status:        t.TxSts,
				OriginalMsgID: origMsgID,
			}
			if len(t.StsRsnInf) > 0 && t.StsRsnInf[0].Rsn != nil {
				r.ReasonCode = t.StsRsnInf[0].Rsn.Cd
				if r.ReasonCode == "" {
					r.ReasonCode = t.StsRsnInf[0].Rsn.Prtry
				}
				if len(t.StsRsnInf[0].AddtlInf) > 0 {
					r.ReasonInfo = t.StsRsnInf[0].AddtlInf[0]
				}
			}
			reports = append(reports, r)
		}
	}
	return reports
}
