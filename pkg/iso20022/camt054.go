package iso20022

import (
	"encoding/xml"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// camt.054.001.14 — BkToCstmrDbtCdtNtfctn
// ─────────────────────────────────────────────
//
// Bank-to-customer debit/credit notification: settlement evidence for the
// account holder (e.g. proof that a disbursement batch left the org account).

// Camt054Document is the root XML document for camt.054
type Camt054Document struct {
	XMLName               xml.Name `xml:"Document"`
	Xmlns                 string   `xml:"xmlns,attr"`
	BkToCstmrDbtCdtNtfctn struct {
		XMLName xml.Name                `xml:"BkToCstmrDbtCdtNtfctn"`
		GrpHdr  GroupHeader116          `xml:"GrpHdr"`
		Ntfctn  []AccountNotification25 `xml:"Ntfctn"`
	} `xml:"BkToCstmrDbtCdtNtfctn"`
}

// GroupHeader116 — camt.054.001.14 group header.
// XSD sequence: MsgId, CreDtTm, MsgRcpt?, MsgPgntn?, OrgnlBizQry?, AddtlInf?
type GroupHeader116 struct {
	MsgId    string `xml:"MsgId"`
	CreDtTm  string `xml:"CreDtTm"`
	AddtlInf string `xml:"AddtlInf,omitempty"`
}

// AccountNotification25 — camt.054.001.14 notification block.
// XSD sequence: Id, NtfctnPgntn?, ElctrncSeqNb?, RptgSeq?, LglSeqNb?, CreDtTm?,
// FrToDt?, CpyDplctInd?, RptgSrc?, Acct, RltdAcct?, Intrst?, TxsSummry?,
// Ntry*, AddtlNtfctnInf?
type AccountNotification25 struct {
	Id             string          `xml:"Id"`
	CreDtTm        string          `xml:"CreDtTm,omitempty"`
	Acct           *CashAccount43  `xml:"Acct"`
	Ntry           []ReportEntry16 `xml:"Ntry,omitempty"`
	AddtlNtfctnInf string          `xml:"AddtlNtfctnInf,omitempty"`
}

// CashAccount43 — notification account (Id optional at type level but
// practically required for a usable notification).
// XSD sequence: Id?, Tp?, Ccy?, Nm?, Prxy?, Ownr?, Svcr?
type CashAccount43 struct {
	Id   *AccountIdentification4Choice                 `xml:"Id,omitempty"`
	Tp   *CashAccountType2Choice                       `xml:"Tp,omitempty"`
	Ccy  string                                        `xml:"Ccy,omitempty"`
	Nm   string                                        `xml:"Nm,omitempty"`
	Prxy *ProxyAccountIdentification1                  `xml:"Prxy,omitempty"`
	Ownr *PartyIdentification272                       `xml:"Ownr,omitempty"`
	Svcr *BranchAndFinancialInstitutionIdentification8 `xml:"Svcr,omitempty"`
}

// ReportEntry16 — camt.054.001.14 entry.
// XSD sequence (subset): NtryRef?, Amt, CdtDbtInd, RvslInd?, Sts, BookgDt?,
// ValDt?, AcctSvcrRef?, Avlbty?, BkTxCd, ..., NtryDtls?, AddtlNtryInf?
type ReportEntry16 struct {
	NtryRef      string                             `xml:"NtryRef,omitempty"`
	Amt          *ActiveOrHistoricCurrencyAndAmount `xml:"Amt"`
	CdtDbtInd    string                             `xml:"CdtDbtInd"`
	Sts          *EntryStatus1Choice                `xml:"Sts"`
	BookgDt      *DateAndDateTime2Choice            `xml:"BookgDt,omitempty"`
	ValDt        *DateAndDateTime2Choice            `xml:"ValDt,omitempty"`
	AcctSvcrRef  string                             `xml:"AcctSvcrRef,omitempty"`
	BkTxCd       *BankTransactionCodeStructure4     `xml:"BkTxCd"`
	NtryDtls     *EntryDetails16                    `xml:"NtryDtls,omitempty"`
	AddtlNtryInf string                             `xml:"AddtlNtryInf,omitempty"`
}

// EntryStatus1Choice — entry status as external code or proprietary.
// XSD choice: emit exactly one of Cd/Prtry.
type EntryStatus1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// BankTransactionCodeStructure4 — BkTxCd at Ntry level.
// XSD sequence: Domn?, Prtry?
type BankTransactionCodeStructure4 struct {
	Domn  *BankTransactionCodeStructure5            `xml:"Domn,omitempty"`
	Prtry *ProprietaryBankTransactionCodeStructure1 `xml:"Prtry,omitempty"`
}

// BankTransactionCodeStructure5 — domain
type BankTransactionCodeStructure5 struct {
	Cd   string                         `xml:"Cd"`
	Fmly *BankTransactionCodeStructure6 `xml:"Fmly"`
}

// BankTransactionCodeStructure6 — family
type BankTransactionCodeStructure6 struct {
	Cd        string `xml:"Cd"`
	SubFmlyCd string `xml:"SubFmlyCd"`
}

// ProprietaryBankTransactionCodeStructure1 — proprietary bank tx code
type ProprietaryBankTransactionCodeStructure1 struct {
	Cd   string `xml:"Cd"`
	Issr string `xml:"Issr,omitempty"`
}

// EntryDetails16 — NtryDtls block
type EntryDetails16 struct {
	TxDtls []EntryTransaction16 `xml:"TxDtls"`
}

// EntryTransaction16 — transaction details.
// XSD sequence (subset): Refs?, Amt?, CdtDbtInd?, ..., RltdPties?, ...,
// RmtInf?, ..., SplmtryData*
type EntryTransaction16 struct {
	Refs        *TransactionReferences10           `xml:"Refs,omitempty"`
	Amt         *ActiveOrHistoricCurrencyAndAmount `xml:"Amt,omitempty"`
	CdtDbtInd   string                             `xml:"CdtDbtInd,omitempty"`
	RltdPties   *TransactionParties12              `xml:"RltdPties,omitempty"`
	RmtInf      *RemittanceInformation26           `xml:"RmtInf,omitempty"`
	SplmtryData []SupplementaryData1               `xml:"SplmtryData,omitempty"`
}

// TransactionReferences10 — references linking the entry back to the
// originating message and transaction.
type TransactionReferences10 struct {
	MsgId       string `xml:"MsgId,omitempty"`
	AcctSvcrRef string `xml:"AcctSvcrRef,omitempty"`
	PmtInfId    string `xml:"PmtInfId,omitempty"`
	InstrId     string `xml:"InstrId,omitempty"`
	EndToEndId  string `xml:"EndToEndId,omitempty"`
	UETR        string `xml:"UETR,omitempty"`
	TxId        string `xml:"TxId,omitempty"`
}

// TransactionParties12 — related parties (RltdPties).
// XSD sequence: InitgPty?, Dbtr?, DbtrAcct?, UltmtDbtr?, Cdtr?, CdtrAcct?,
// UltmtCdtr?, TradgPty?, Prtry?
type TransactionParties12 struct {
	InitgPty *Party50Choice `xml:"InitgPty,omitempty"`
	Dbtr     *Party50Choice `xml:"Dbtr,omitempty"`
	DbtrAcct *CashAccount40 `xml:"DbtrAcct,omitempty"`
	Cdtr     *Party50Choice `xml:"Cdtr,omitempty"`
	CdtrAcct *CashAccount40 `xml:"CdtrAcct,omitempty"`
}

// Camt054Options configures a camt.054 debit/credit notification.
type Camt054Options struct {
	MsgID                string
	Account              *Party // Ntfctn-level account (org's account)
	CreditDebitIndicator string // DBIT (default, disbursement payer view) or CRDT
	EntryStatusCode      string // default "BOOK"
	PmtInfID             string // links Refs back to the pain.001 PmtInf
	OriginalMsgID        string // links Refs back to the pain.001 MsgId
}

// BuildCamt054 builds a camt.054.001.14 notification with one Ntry per
// instruction — settlement evidence for a disbursement batch.
func BuildCamt054(instrs []*CreditTransferInstruction, opts *Camt054Options) (string, error) {
	if err := requireInstructions(instrs, "BuildCamt054"); err != nil {
		return "", err
	}
	if opts == nil {
		opts = &Camt054Options{}
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
		msgID = "SGC1N" + safeTruncate(first.ID, 7)
	}
	creDtTm := time.Now().UTC().Format(time.RFC3339)

	doc := &Camt054Document{Xmlns: NSCamt054}
	doc.BkToCstmrDbtCdtNtfctn.GrpHdr = GroupHeader116{
		MsgId:   msgID,
		CreDtTm: creDtTm,
	}

	ntfctn := AccountNotification25{
		Id:      msgID + "-1",
		CreDtTm: creDtTm,
		Acct:    notificationAccount(opts.Account, first),
	}

	entries := make([]ReportEntry16, 0, len(instrs))
	for _, instr := range instrs {
		entries = append(entries, *camt054Entry(instr, opts, cdtDbt, stsCd))
	}
	ntfctn.Ntry = entries
	doc.BkToCstmrDbtCdtNtfctn.Ntfctn = []AccountNotification25{ntfctn}

	return MarshalXML(doc, NSCamt054)
}

// notificationAccount builds the Ntfctn-level account: the explicit party's
// account, else the first payment's funding address. Stellar addresses exceed
// Othr/Id's Max34Text, so partyAccount places them in Prxy (Max2048Text).
func notificationAccount(acct *Party, p *models.Payment) *CashAccount43 {
	ca := partyAccount(acct)
	if ca == nil {
		from := p.From
		if from == "" {
			from = "NOTPROVIDED"
		}
		ca = partyAccount(&Party{AcctID: from})
	}
	a := &CashAccount43{Id: ca.Id, Prxy: ca.Prxy}
	if acct != nil && acct.Name != "" {
		a.Nm = acct.Name
	}
	if acct != nil && acct.AgentBIC != "" {
		a.Svcr = agentByBIC(acct.AgentBIC)
	}
	return a
}

// camt054Entry builds one Ntry for an instruction.
func camt054Entry(instr *CreditTransferInstruction, opts *Camt054Options, cdtDbt, stsCd string) *ReportEntry16 {
	p := instr.Payment
	asset := paymentAsset(p)
	ccy, assetSuppl := settlementCurrency(asset, p.AssetIssuer, p.Amount)

	entry := &ReportEntry16{
		NtryRef:   safeTruncate(p.ID, 35),
		Amt:       &ActiveOrHistoricCurrencyAndAmount{Ccy: ccy, Value: normalizeAmount(p.Amount)},
		CdtDbtInd: cdtDbt,
		Sts:       &EntryStatus1Choice{Cd: stsCd},
		// Bank transaction code: payment domain, issued credit transfer.
		BkTxCd: &BankTransactionCodeStructure4{
			Domn: &BankTransactionCodeStructure5{
				Cd: "PMNT",
				Fmly: &BankTransactionCodeStructure6{
					Cd:        "ICDT",
					SubFmlyCd: "DMCT",
				},
			},
		},
	}

	if p.ConfirmedAt != nil {
		entry.BookgDt = &DateAndDateTime2Choice{DtTm: p.ConfirmedAt.UTC().Format(time.RFC3339)}
	}
	entry.ValDt = &DateAndDateTime2Choice{Dt: p.CreatedAt.UTC().Format("2006-01-02")}
	if p.TxHash != "" {
		entry.AcctSvcrRef = safeTruncate(p.TxHash, 35)
	}

	txDtls := EntryTransaction16{
		Refs: &TransactionReferences10{
			MsgId:      opts.OriginalMsgID,
			PmtInfId:   opts.PmtInfID,
			InstrId:    safeTruncate(p.ID, 35),
			EndToEndId: endToEndID(p),
			TxId:       safeTruncate(p.TxHash, 35),
		},
		Amt:       &ActiveOrHistoricCurrencyAndAmount{Ccy: ccy, Value: normalizeAmount(p.Amount)},
		CdtDbtInd: cdtDbt,
		RltdPties: &TransactionParties12{
			Dbtr: &Party50Choice{Pty: partyIdentification(instr.Debtor, safeTruncate(p.From, 16))},
			Cdtr: &Party50Choice{Pty: partyIdentification(instr.Creditor, safeTruncate(p.To, 16))},
		},
		RmtInf: remittanceInfo26(instr.RemittanceInfo, p),
	}
	if assetSuppl != nil {
		txDtls.SplmtryData = []SupplementaryData1{*assetSuppl}
	}
	entry.NtryDtls = &EntryDetails16{TxDtls: []EntryTransaction16{txDtls}}

	return entry
}
