package iso20022

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// pain.001.001.13 — CstmrCdtTrfInitn
// ─────────────────────────────────────────────
//
// Customer credit transfer initiation: the message a corporate (e.g. an NGO
// running a disbursement) sends its bank to pay N receivers in one batch.

// Pain001Document is the root XML document for pain.001
type Pain001Document struct {
	XMLName          xml.Name `xml:"Document"`
	Xmlns            string   `xml:"xmlns,attr"`
	CstmrCdtTrfInitn struct {
		XMLName xml.Name               `xml:"CstmrCdtTrfInitn"`
		GrpHdr  GroupHeader114         `xml:"GrpHdr"`
		PmtInf  []PaymentInstruction51 `xml:"PmtInf"`
	} `xml:"CstmrCdtTrfInitn"`
}

// GroupHeader114 — pain.001.001.13 group header.
// XSD sequence: MsgId, CreDtTm, Authstn*, NbOfTxs, CtrlSum?, InitgPty,
// FwdgAgt?, InitnSrc?
type GroupHeader114 struct {
	MsgId    string                  `xml:"MsgId"`
	CreDtTm  string                  `xml:"CreDtTm"`
	NbOfTxs  string                  `xml:"NbOfTxs"`
	CtrlSum  string                  `xml:"CtrlSum,omitempty"`
	InitgPty *PartyIdentification272 `xml:"InitgPty"`
}

// PaymentInstruction51 — pain.001.001.13 payment information block.
// XSD sequence: PmtInfId, PmtMtd, ReqdAdvcTp?, BtchBookg?, NbOfTxs?, CtrlSum?,
// PmtTpInf?, ReqdExctnDt, PoolgAdjstmntDt?, Dbtr, DbtrAcct, DbtrAgt,
// DbtrAgtAcct?, InstrForDbtrAgt?, UltmtDbtr?, ChrgBr?, ChrgsAcct?,
// ChrgsAcctAgt?, CdtTrfTxInf*
type PaymentInstruction51 struct {
	PmtInfId    string                                        `xml:"PmtInfId"`
	PmtMtd      string                                        `xml:"PmtMtd"`
	BtchBookg   *bool                                         `xml:"BtchBookg,omitempty"`
	NbOfTxs     string                                        `xml:"NbOfTxs,omitempty"`
	CtrlSum     string                                        `xml:"CtrlSum,omitempty"`
	PmtTpInf    *PaymentTypeInformation26                     `xml:"PmtTpInf,omitempty"`
	ReqdExctnDt *DateAndDateTime2Choice                       `xml:"ReqdExctnDt"`
	Dbtr        *PartyIdentification272                       `xml:"Dbtr"`
	DbtrAcct    *CashAccount40                                `xml:"DbtrAcct"`
	DbtrAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt"`
	UltmtDbtr   *PartyIdentification272                       `xml:"UltmtDbtr,omitempty"`
	ChrgBr      string                                        `xml:"ChrgBr,omitempty"`
	CdtTrfTxInf []CreditTransferTransaction76                 `xml:"CdtTrfTxInf"`
}

// PaymentTypeInformation26 — pain.001.001.13 payment type info
type PaymentTypeInformation26 struct {
	InstrPrty string                  `xml:"InstrPrty,omitempty"`
	SvcLvl    []ServiceLevel8Choice   `xml:"SvcLvl,omitempty"`
	LclInstrm *LocalInstrument2Choice `xml:"LclInstrm,omitempty"`
	CtgyPurp  *CategoryPurpose1Choice `xml:"CtgyPurp,omitempty"`
}

// DateAndDateTime2Choice — date or date-time choice
type DateAndDateTime2Choice struct {
	Dt   string `xml:"Dt,omitempty"`
	DtTm string `xml:"DtTm,omitempty"`
}

// CreditTransferTransaction76 — pain.001.001.13 transaction info.
// XSD sequence (subset): PmtId, PmtTpInf?, Amt, XchgRateInf?, ChrgBr?,
// MndtRltdInf?, ChqInstr?, UltmtDbtr?, IntrmyAgt1..3(+Acct)?, CdtrAgt?,
// CdtrAgtAcct?, Cdtr?, CdtrAcct?, UltmtCdtr?, InstrForCdtrAgt*,
// InstrForDbtrAgt?, Purp?, RgltryRptg*, Tax?, RltdRmtInf*, RmtInf?,
// SplmtryData*
type CreditTransferTransaction76 struct {
	PmtId       *PaymentIdentification6                       `xml:"PmtId"`
	PmtTpInf    *PaymentTypeInformation26                     `xml:"PmtTpInf,omitempty"`
	Amt         *AmountType4Choice                            `xml:"Amt"`
	ChrgBr      string                                        `xml:"ChrgBr,omitempty"`
	CdtrAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt,omitempty"`
	Cdtr        *PartyIdentification272                       `xml:"Cdtr,omitempty"`
	CdtrAcct    *CashAccount40                                `xml:"CdtrAcct,omitempty"`
	Purp        *Purpose2Choice                               `xml:"Purp,omitempty"`
	RmtInf      *RemittanceInformation26                      `xml:"RmtInf,omitempty"`
	SplmtryData []SupplementaryData1                          `xml:"SplmtryData,omitempty"`
}

// PaymentIdentification6 — pain.001 payment identification.
// XSD sequence: InstrId?, EndToEndId, UETR?
type PaymentIdentification6 struct {
	InstrId    string `xml:"InstrId,omitempty"`
	EndToEndId string `xml:"EndToEndId"`
	UETR       string `xml:"UETR,omitempty"`
}

// AmountType4Choice — instructed or equivalent amount
type AmountType4Choice struct {
	InstdAmt *ActiveOrHistoricCurrencyAndAmount `xml:"InstdAmt,omitempty"`
	EqvtAmt  *ActiveOrHistoricCurrencyAndAmount `xml:"EqvtAmt,omitempty"`
}

// RemittanceInformation26 — pain.001 remittance info
type RemittanceInformation26 struct {
	Ustrd []string                            `xml:"Ustrd,omitempty"`
	Strd  []StructuredRemittanceInformation22 `xml:"Strd,omitempty"`
}

// StructuredRemittanceInformation22 — structured remittance (subset)
type StructuredRemittanceInformation22 struct {
	CdtrRefInf  *CreditorReferenceInformation3 `xml:"CdtrRefInf,omitempty"`
	AddtlRmtInf string                         `xml:"AddtlRmtInf,omitempty"`
}

// CreditorReferenceInformation3 — creditor reference (e.g. memo id/hash)
type CreditorReferenceInformation3 struct {
	Tp  *CreditorReferenceType3 `xml:"Tp,omitempty"`
	Ref string                  `xml:"Ref,omitempty"`
}

// CreditorReferenceType3 — reference type as code or proprietary
type CreditorReferenceType3 struct {
	CdOrPrtry *CreditorReferenceType2Choice `xml:"CdOrPrtry,omitempty"`
	Issr      string                        `xml:"Issr,omitempty"`
}

// CreditorReferenceType2Choice
type CreditorReferenceType2Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// Pain001Options configures a pain.001 batch message.
type Pain001Options struct {
	MsgID           string
	InitiatingParty *Party // InitgPty — required; org name or ID
	Debtor          *Party // PmtInf-level debtor (org account/agent)
	PmtInfID        string
	ReqdExctnDt     string // ISO date; defaults to the first payment's date
	ChargeBearer    string // DEBT|CRED|SHAR|SLEV — applies at PmtInf level
	BatchBooking    *bool
	PurposeCode     string
}

// BuildPain001 builds a pain.001.001.13 customer credit transfer initiation:
// one PmtInf (single debtor) containing one CdtTrfTxInf per instruction.
// NbOfTxs and CtrlSum are computed from the batch.
func BuildPain001(instrs []*CreditTransferInstruction, opts *Pain001Options) (string, error) {
	if err := requireInstructions(instrs, "BuildPain001"); err != nil {
		return "", err
	}
	if opts == nil {
		opts = &Pain001Options{}
	}

	first := instrs[0].Payment
	msgID := opts.MsgID
	if msgID == "" {
		msgID = "SGC1P" + safeTruncate(first.ID, 7)
	}
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	amounts := make([]string, 0, len(instrs))
	for _, instr := range instrs {
		amounts = append(amounts, normalizeAmount(instr.Payment.Amount))
	}
	ctrlSum, err := sumAmounts(amounts)
	if err != nil {
		return "", fmt.Errorf("BuildPain001: %w", err)
	}
	nbOfTxs := fmt.Sprintf("%d", len(instrs))

	doc := &Pain001Document{Xmlns: NSPain001}
	doc.CstmrCdtTrfInitn.GrpHdr = GroupHeader114{
		MsgId:    msgID,
		CreDtTm:  creDtTm,
		NbOfTxs:  nbOfTxs,
		CtrlSum:  ctrlSum,
		InitgPty: partyIdentification(opts.InitiatingParty, "NOTPROVIDED"),
	}

	pmtInfID := opts.PmtInfID
	if pmtInfID == "" {
		pmtInfID = msgID + "-1"
	}
	reqdDt := opts.ReqdExctnDt
	if reqdDt == "" {
		reqdDt = first.CreatedAt.UTC().Format("2006-01-02")
	}

	// Debtor: explicit opts party, else the first instruction's, else
	// NOTPROVIDED fallbacks — pain.001 requires Dbtr/DbtrAcct/DbtrAgt.
	dbtr := opts.Debtor
	if dbtr == nil {
		dbtr = instrs[0].Debtor
	}
	dbtrNm := safeTruncate(first.From, 16)

	pmtInf := PaymentInstruction51{
		PmtInfId:    pmtInfID,
		PmtMtd:      "TRF",
		BtchBookg:   opts.BatchBooking,
		NbOfTxs:     nbOfTxs,
		CtrlSum:     ctrlSum,
		ReqdExctnDt: &DateAndDateTime2Choice{Dt: reqdDt},
		Dbtr:        partyIdentification(dbtr, dbtrNm),
		DbtrAgt:     debtorAgentPain(dbtr, first),
		ChrgBr:      opts.ChargeBearer,
	}
	pmtInf.DbtrAcct = partyAccount(dbtr)
	if pmtInf.DbtrAcct == nil {
		// DbtrAcct is mandatory — fall back to the funding address, which
		// partyAccount routes to Prxy since it exceeds Othr/Id's Max34Text.
		from := first.From
		if from == "" {
			from = "NOTPROVIDED"
		}
		pmtInf.DbtrAcct = partyAccount(&Party{AcctID: from})
	}

	txns := make([]CreditTransferTransaction76, 0, len(instrs))
	for _, instr := range instrs {
		txns = append(txns, *pain001TxInf(instr, opts))
	}
	pmtInf.CdtTrfTxInf = txns
	doc.CstmrCdtTrfInitn.PmtInf = []PaymentInstruction51{pmtInf}

	return MarshalXML(doc, NSPain001)
}

// debtorAgentPain resolves the PmtInf-level DbtrAgt.
func debtorAgentPain(dbtr *Party, p *models.Payment) *BranchAndFinancialInstitutionIdentification8 {
	if dbtr != nil && dbtr.AgentBIC != "" {
		return agentByBIC(dbtr.AgentBIC)
	}
	return agentOrFallback("", safeTruncate(p.From, 35))
}

// pain001TxInf builds one CdtTrfTxInf for an instruction.
func pain001TxInf(instr *CreditTransferInstruction, opts *Pain001Options) *CreditTransferTransaction76 {
	p := instr.Payment
	asset := paymentAsset(p)
	ccy, assetSuppl := settlementCurrency(asset, p.AssetIssuer, p.Amount)

	txInf := &CreditTransferTransaction76{
		PmtId: &PaymentIdentification6{
			EndToEndId: endToEndID(p),
		},
		Amt: &AmountType4Choice{
			InstdAmt: &ActiveOrHistoricCurrencyAndAmount{
				Ccy:   ccy,
				Value: normalizeAmount(p.Amount),
			},
		},
	}
	txInf.PmtId.InstrId = safeTruncate(p.ID, 35)
	if assetSuppl != nil {
		txInf.SplmtryData = []SupplementaryData1{*assetSuppl}
	}

	cdtr := instr.Creditor
	if cdtr != nil && cdtr.AgentBIC != "" {
		txInf.CdtrAgt = agentByBIC(cdtr.AgentBIC)
	}
	if cdtr != nil || p.To != "" {
		txInf.Cdtr = partyIdentification(cdtr, safeTruncate(p.To, 16))
	}
	if acct := partyAccount(cdtr); acct != nil {
		txInf.CdtrAcct = acct
	}

	if opts.PurposeCode != "" {
		txInf.Purp = &Purpose2Choice{Cd: opts.PurposeCode}
	}

	txInf.RmtInf = remittanceInfo26(instr.RemittanceInfo, p)

	return txInf
}

// remittanceInfo26 builds pain.001 RmtInf: explicit lines or memo text go to
// Ustrd; memo_type "id"/"hash"/"return" maps to Strd>CdtrRefInf.
func remittanceInfo26(lines []string, p *models.Payment) *RemittanceInformation26 {
	if len(lines) > 0 {
		return &RemittanceInformation26{Ustrd: lines}
	}
	if p.Memo == "" {
		return nil
	}
	switch p.MemoType {
	case "id", "hash", "return":
		return &RemittanceInformation26{
			Strd: []StructuredRemittanceInformation22{{
				CdtrRefInf: &CreditorReferenceInformation3{
					Tp: &CreditorReferenceType3{
						CdOrPrtry: &CreditorReferenceType2Choice{Prtry: "MEMO"},
					},
					Ref: p.Memo,
				},
			}},
		}
	default:
		return &RemittanceInformation26{Ustrd: []string{p.Memo}}
	}
}
