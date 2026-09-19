package iso20022

import (
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// ─────────────────────────────────────────────
// pacs.008.001.08 — FIToFICstmrCdtTrf
// ─────────────────────────────────────────────

// Pacs008Document is the root XML document for pacs.008
type Pacs008Document struct {
	XMLName           xml.Name `xml:"Document"`
	Xmlns             string   `xml:"xmlns,attr"`
	FIToFICstmrCdtTrf struct {
		XMLName     xml.Name                      `xml:"FIToFICstmrCdtTrf"`
		GrpHdr      GroupHeader93                 `xml:"GrpHdr"`
		CdtTrfTxInf []CreditTransferTransaction39 `xml:"CdtTrfTxInf"`
	} `xml:"FIToFICstmrCdtTrf"`
}

// CreditTransferTransaction39 — pacs.008.001.08 transaction info.
// XSD sequence (subset implemented):
//
//	PmtId, PmtTpInf?, IntrBkSttlmAmt, IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?,
//	SttlmTmReq?, AccptncDtTm?, PoolgAdjstmntDt?, InstdAmt?, XchgRate?, ChrgBr,
//	ChrgsInf*, PrvsInstgAgt1..3(+Acct)?, IntrmyAgt1..3(+Acct)?, UltmtDbtr?,
//	InitgPty?, InstgAgt?, InstdAgt?, Dbtr, DbtrAcct?, DbtrAgt, DbtrAgtAcct?,
//	CdtrAgt, CdtrAgtAcct?, Cdtr, CdtrAcct?, UltmtCdtr?, InstrForCdtrAgt*,
//	InstrForNxtAgt*, Purp?, RgltryRptg*, Tax?, RltdRmtInf*, RmtInf?, NclsdFile?,
//	SplmtryData*
type CreditTransferTransaction39 struct {
	PmtId          *PaymentIdentification7                       `xml:"PmtId"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	InstdAmt       *ActiveOrHistoricCurrencyAndAmount            `xml:"InstdAmt,omitempty"`
	ChrgBr         string                                        `xml:"ChrgBr"`
	ChrgsInf       []ChargesInformation1                         `xml:"ChrgsInf,omitempty"`
	UltmtDbtr      *PartyIdentification135                       `xml:"UltmtDbtr,omitempty"`
	InitgPty       *PartyIdentification135                       `xml:"InitgPty,omitempty"`
	InstgAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	Dbtr           *PartyIdentification135                       `xml:"Dbtr"`
	DbtrAcct       *CashAccount38                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt"`
	DbtrAgtAcct    *CashAccount38                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt"`
	CdtrAgtAcct    *CashAccount38                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *PartyIdentification135                       `xml:"Cdtr"`
	CdtrAcct       *CashAccount38                                `xml:"CdtrAcct,omitempty"`
	UltmtCdtr      *PartyIdentification135                       `xml:"UltmtCdtr,omitempty"`
	Purp           *Purpose1Choice                               `xml:"Purp,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
}

// Pacs008Options configures optional fields for pacs.008
type Pacs008Options struct {
	InstgBIC       string
	InstdBIC       string
	DbtrBIC        string
	CdtrBIC        string
	DbtrName       string
	DbtrAddress    *PostalAddress24
	CdtrName       string
	CdtrAddress    *PostalAddress24
	DbtrAcctIBAN   string
	CdtrAcctIBAN   string
	ChargeBearer   string
	PurposeCode    string
	RemittanceInfo []string
	UETR           string
	TxID           string
	InstrID        string
}

// BuildPacs008 builds a complete pacs.008.001.08 XML from a Payment model
func BuildPacs008(p *models.Payment, opts *Pacs008Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs008Options{}
	}

	currency := p.Asset
	if currency == "" {
		currency = "XLM"
	}

	msgID := "MZTP" + safeTruncate(p.ID, 8)
	creDtTm := p.CreatedAt.UTC().Format(time.RFC3339)
	sttlmDt := p.CreatedAt.UTC().Format("2006-01-02")

	endToEndID := safeTruncate(p.TxHash, 16)
	if endToEndID == "" {
		endToEndID = p.ID
	}

	uetr := opts.UETR
	if uetr == "" {
		uetr = generateUUIDv4()
	}

	txID := opts.TxID
	if txID == "" {
		txID = p.ID
	}

	doc := &Pacs008Document{
		Xmlns: NSPacs008,
	}

	doc.FIToFICstmrCdtTrf.GrpHdr = GroupHeader93{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction7{
			SttlmMtd: "CLRG",
		},
	}

	if opts.InstgBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	txInf := CreditTransferTransaction39{
		PmtId: &PaymentIdentification7{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
		IntrBkSttlmDt: sttlmDt,
	}

	if opts.ChargeBearer != "" {
		txInf.ChrgBr = opts.ChargeBearer
	} else {
		txInf.ChrgBr = string(ChrgBrShar)
	}

	// InitgPty lives at transaction level in pacs.008 (not in GrpHdr)
	txInf.InitgPty = &PartyIdentification135{Nm: "MozartPay"}

	dbtrName := opts.DbtrName
	if dbtrName == "" {
		dbtrName = safeTruncate(p.From, 16)
	}
	txInf.Dbtr = &PartyIdentification135{
		Nm:      dbtrName,
		PstlAdr: opts.DbtrAddress,
	}

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
	}

	// DbtrAgt is mandatory — fall back to Othr/Id when no BIC is available
	txInf.DbtrAgt = agentOrFallback(opts.DbtrBIC, safeTruncate(p.From, 35))

	// Creditor side order: CdtrAgt → Cdtr → CdtrAcct
	txInf.CdtrAgt = agentOrFallback(opts.CdtrBIC, safeTruncate(p.To, 35))

	cdtrName := opts.CdtrName
	if cdtrName == "" {
		cdtrName = safeTruncate(p.To, 16)
	}
	txInf.Cdtr = &PartyIdentification135{
		Nm:      cdtrName,
		PstlAdr: opts.CdtrAddress,
	}

	if opts.CdtrAcctIBAN != "" {
		txInf.CdtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.PurposeCode != "" {
		txInf.Purp = &Purpose1Choice{Cd: opts.PurposeCode}
	}

	if len(opts.RemittanceInfo) > 0 {
		txInf.RmtInf = &RemittanceInformation2{
			Ustrd: opts.RemittanceInfo,
		}
	} else if p.Memo != "" {
		txInf.RmtInf = &RemittanceInformation2{
			Ustrd: []string{p.Memo},
		}
	}

	doc.FIToFICstmrCdtTrf.CdtTrfTxInf = []CreditTransferTransaction39{txInf}

	return MarshalXML(doc, NSPacs008)
}

// safeTruncate truncates a string to n chars
func safeTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// generateUUIDv4 generates a RFC 4122 v4 UUID string using crypto/rand
func generateUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-derived bytes; still sets version/variant bits
		n := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(n >> uint(i*8))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
