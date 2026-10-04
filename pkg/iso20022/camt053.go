package iso20022

import (
	"encoding/xml"
	"time"
)

// ─────────────────────────────────────────────
// camt.053.001.13 — BkToCstmrStmt
// ─────────────────────────────────────────────
//
// Bank-to-customer statement: the periodic account statement a disbursement
// platform reconciles against — opening/closing balances plus one entry per
// settled payment.

// Camt053Document is the root XML document for camt.053
type Camt053Document struct {
	XMLName       xml.Name `xml:"Document"`
	Xmlns         string   `xml:"xmlns,attr"`
	BkToCstmrStmt struct {
		XMLName xml.Name           `xml:"BkToCstmrStmt"`
		GrpHdr  GroupHeader116     `xml:"GrpHdr"`
		Stmt    []AccountStatement `xml:"Stmt"`
	} `xml:"BkToCstmrStmt"`
}

// AccountStatement — camt.053.001.13 statement block.
// XSD sequence: Id, StmtPgntn?, ElctrncSeqNb?, RptgSeq?, LglSeqNb?, CreDtTm?,
// FrToDt?, CpyDplctInd?, Dplct?, RptgSrc?, Acct, RltdAcct?, Intrst?, Bal*,
// TxsSummry?, Ntry*, AddtlStmtInf?
type AccountStatement struct {
	Id           string           `xml:"Id"`
	ElctrncSeqNb string           `xml:"ElctrncSeqNb,omitempty"`
	CreDtTm      string           `xml:"CreDtTm,omitempty"`
	FrToDt       *DatePeriod      `xml:"FrToDt,omitempty"`
	Acct         *CashAccount43   `xml:"Acct"`
	Bal          []AccountBalance `xml:"Bal,omitempty"`
	Ntry         []ReportEntry16  `xml:"Ntry,omitempty"`
	AddtlStmtInf string           `xml:"AddtlStmtInf,omitempty"`
}

// DatePeriod — statement period (FrToDt).
type DatePeriod struct {
	FrDtTm string `xml:"FrDtTm"`
	ToDtTm string `xml:"ToDtTm"`
}

// AccountBalance — camt.053 Bal element.
// XSD sequence: Tp, CdtLine?, Amt, CdtDbtInd, Dt, Avlbty*
type AccountBalance struct {
	Tp        *BalanceType                       `xml:"Tp"`
	Amt       *ActiveOrHistoricCurrencyAndAmount `xml:"Amt"`
	CdtDbtInd string                             `xml:"CdtDbtInd"`
	Dt        *DateAndDateTime2Choice            `xml:"Dt"`
}

// BalanceType — balance type (Tp>CdOrPrtry>Cd).
// XSD sequence: CdOrPrtry, SubTp?
type BalanceType struct {
	CdOrPrtry *BalanceTypeCode `xml:"CdOrPrtry"`
}

// BalanceTypeCode — balance type code or proprietary value.
type BalanceTypeCode struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// StatementBalance describes one Bal element (e.g. OPBD/CLBD) as input.
type StatementBalance struct {
	TypeCode  string // "OPBD" (opening) or "CLBD" (closing)
	Amount    string
	Ccy       string
	CdtDbtInd string // "CRDT" or "DBIT"
	Date      string // ISO date (2006-01-02)
}

// Camt053Options configures a camt.053 statement.
type Camt053Options struct {
	MsgID                string
	StatementID          string // Stmt>Id — defaults to MsgID + "-1"
	Account              *Party // Stmt-level account (org's account)
	ServicerBIC          string // Svcr — servicing agent BIC
	ElectronicSeq        string // ElctrncSeqNb
	PeriodFrom           string // FrToDt>FrDtTm (RFC3339) — optional
	PeriodTo             string // FrToDt>ToDtTm (RFC3339) — optional
	Balances             []StatementBalance
	CreditDebitIndicator string // DBIT (default, disbursement payer view) or CRDT
	EntryStatusCode      string // default "BOOK"
	PmtInfID             string // links Refs back to the pain.001 PmtInf
	OriginalMsgID        string // links Refs back to the pain.001 MsgId
}

// BuildCamt053 builds a camt.053.001.13 bank statement with one Ntry per
// instruction and the balances given in opts.
func BuildCamt053(instrs []*CreditTransferInstruction, opts *Camt053Options) (string, error) {
	if err := requireInstructions(instrs, "BuildCamt053"); err != nil {
		return "", err
	}
	if opts == nil {
		opts = &Camt053Options{}
	}
	cdtDbt := opts.CreditDebitIndicator
	if cdtDbt == "" {
		cdtDbt = "DBIT"
	}
	stsCd := opts.EntryStatusCode
	if stsCd == "" {
		stsCd = "BOOK"
	}

	first := instrs[0].Payment
	msgID := opts.MsgID
	if msgID == "" {
		msgID = "SGC1T" + safeTruncate(first.ID, 7)
	}
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	stmtID := opts.StatementID
	if stmtID == "" {
		stmtID = msgID + "-1"
	}

	doc := &Camt053Document{Xmlns: NSCamt053}
	doc.BkToCstmrStmt.GrpHdr = GroupHeader116{
		MsgId:   msgID,
		CreDtTm: creDtTm,
	}

	stmt := AccountStatement{
		Id:      stmtID,
		CreDtTm: creDtTm,
		Acct:    notificationAccount(opts.Account, first),
	}
	if opts.ElectronicSeq != "" {
		stmt.ElctrncSeqNb = opts.ElectronicSeq
	}
	if opts.PeriodFrom != "" || opts.PeriodTo != "" {
		stmt.FrToDt = &DatePeriod{FrDtTm: opts.PeriodFrom, ToDtTm: opts.PeriodTo}
	}
	if opts.ServicerBIC != "" {
		// The servicer is nested inside Acct (CashAccount43>Svcr), not a
		// statement-level element.
		stmt.Acct.Svcr = agentByBIC(opts.ServicerBIC)
	}

	for _, b := range opts.Balances {
		stmt.Bal = append(stmt.Bal, AccountBalance{
			Tp:        &BalanceType{CdOrPrtry: &BalanceTypeCode{Cd: b.TypeCode}},
			Amt:       &ActiveOrHistoricCurrencyAndAmount{Ccy: b.Ccy, Value: normalizeAmount(b.Amount)},
			CdtDbtInd: b.CdtDbtInd,
			Dt:        &DateAndDateTime2Choice{Dt: b.Date},
		})
	}

	entries := make([]ReportEntry16, 0, len(instrs))
	for _, instr := range instrs {
		entries = append(entries, *camtEntry(instr, opts.OriginalMsgID, opts.PmtInfID, cdtDbt, stsCd))
	}
	stmt.Ntry = entries
	doc.BkToCstmrStmt.Stmt = []AccountStatement{stmt}

	return MarshalXML(doc, NSCamt053)
}
