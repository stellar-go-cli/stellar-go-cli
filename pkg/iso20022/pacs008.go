package iso20022

import (
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// pacs.008.001.14 — FIToFICstmrCdtTrf
// ─────────────────────────────────────────────

// Pacs008Document is the root XML document for pacs.008
type Pacs008Document struct {
	XMLName           xml.Name `xml:"Document"`
	Xmlns             string   `xml:"xmlns,attr"`
	FIToFICstmrCdtTrf struct {
		XMLName     xml.Name                      `xml:"FIToFICstmrCdtTrf"`
		GrpHdr      GroupHeader131                `xml:"GrpHdr"`
		CdtTrfTxInf []CreditTransferTransaction73 `xml:"CdtTrfTxInf"`
	} `xml:"FIToFICstmrCdtTrf"`
}

// CreditTransferTransaction73 — pacs.008.001.14 transaction info.
// XSD sequence (subset implemented):
//
//	PmtId, PmtTpInf?, IntrBkSttlmAmt, IntrBkSttlmDt?, SttlmPrty?, SttlmTmIndctn?,
//	SttlmTmReq?, AddtlDtTm?, InstdAmt?, XchgRate?, AgrdRate?, ChrgBr,
//	ChrgsInf*, MndtRltdInf?, PmtSgntr?, PrvsInstgAgt1..3(+Acct)?, InstgAgt?,
//	InstdAgt?, IntrmyAgt1..3(+Acct)?, UltmtDbtr?, InitgPty?, Dbtr, DbtrAcct?,
//	DbtrAgt, DbtrAgtAcct?, CdtrAgt, CdtrAgtAcct?, Cdtr, CdtrAcct?, UltmtCdtr?,
//	InstrForCdtrAgt*, InstrForNxtAgt*, Purp?, RgltryRptg*, Tax?, RltdRmtInf*,
//	RmtInf?, SplmtryData*
type CreditTransferTransaction73 struct {
	PmtId          *PaymentIdentification13                      `xml:"PmtId"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	InstdAmt       *ActiveOrHistoricCurrencyAndAmount            `xml:"InstdAmt,omitempty"`
	ChrgBr         string                                        `xml:"ChrgBr"`
	ChrgsInf       []Charges16                                   `xml:"ChrgsInf,omitempty"`
	InstgAgt       *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt       *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
	UltmtDbtr      *PartyIdentification272                       `xml:"UltmtDbtr,omitempty"`
	InitgPty       *PartyIdentification272                       `xml:"InitgPty,omitempty"`
	Dbtr           *PartyIdentification272                       `xml:"Dbtr"`
	DbtrAcct       *CashAccount40                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt"`
	DbtrAgtAcct    *CashAccount40                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt"`
	CdtrAgtAcct    *CashAccount40                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *PartyIdentification272                       `xml:"Cdtr"`
	CdtrAcct       *CashAccount40                                `xml:"CdtrAcct,omitempty"`
	UltmtCdtr      *PartyIdentification272                       `xml:"UltmtCdtr,omitempty"`
	Purp           *Purpose2Choice                               `xml:"Purp,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
	SplmtryData    []SupplementaryData1                          `xml:"SplmtryData,omitempty"`
}

// Pacs008Options configures optional fields for pacs.008
type Pacs008Options struct {
	InstgBIC       string
	InstdBIC       string
	DbtrBIC        string
	CdtrBIC        string
	DbtrName       string
	DbtrAddress    *PostalAddress27
	CdtrName       string
	CdtrAddress    *PostalAddress27
	DbtrAcctIBAN   string
	CdtrAcctIBAN   string
	ChargeBearer   string
	PurposeCode    string
	RemittanceInfo []string
	UETR           string
	TxID           string
	InstrID        string
}

// BuildPacs008 builds a complete pacs.008.001.14 XML from a Payment model
func BuildPacs008(p *models.Payment, opts *Pacs008Options) (string, error) {
	if p == nil {
		return "", fmt.Errorf("payment is nil")
	}
	if opts == nil {
		opts = &Pacs008Options{}
	}

	asset := p.Asset
	if asset == "" {
		asset = "XLM"
	}
	ccy, assetSuppl := settlementCurrency(asset, p.Amount)

	msgID := "SGC1" + safeTruncate(p.ID, 8)
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

	doc.FIToFICstmrCdtTrf.GrpHdr = GroupHeader131{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInstruction15{
			SttlmMtd: "CLRG",
		},
	}

	if opts.InstgBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstgAgt = agentByBIC(opts.InstgBIC)
	}
	if opts.InstdBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstdAgt = agentByBIC(opts.InstdBIC)
	}

	txInf := CreditTransferTransaction73{
		PmtId: &PaymentIdentification13{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   ccy,
			Value: normalizeAmount(p.Amount),
		},
		IntrBkSttlmDt: sttlmDt,
	}
	if assetSuppl != nil {
		txInf.SplmtryData = []SupplementaryData1{*assetSuppl}
	}

	if opts.ChargeBearer != "" {
		txInf.ChrgBr = opts.ChargeBearer
	} else {
		txInf.ChrgBr = string(ChrgBrShar)
	}

	// InitgPty lives at transaction level in pacs.008 (not in GrpHdr)
	txInf.InitgPty = &PartyIdentification272{Nm: "Stellar Go CLI"}

	dbtrName := opts.DbtrName
	if dbtrName == "" {
		dbtrName = safeTruncate(p.From, 16)
	}
	txInf.Dbtr = &PartyIdentification272{
		Nm:      dbtrName,
		PstlAdr: opts.DbtrAddress,
	}

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount40{
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
	txInf.Cdtr = &PartyIdentification272{
		Nm:      cdtrName,
		PstlAdr: opts.CdtrAddress,
	}

	if opts.CdtrAcctIBAN != "" {
		txInf.CdtrAcct = &CashAccount40{
			Id: &AccountIdentification4Choice{IBAN: opts.CdtrAcctIBAN},
		}
	}

	if opts.PurposeCode != "" {
		txInf.Purp = &Purpose2Choice{Cd: opts.PurposeCode}
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

	doc.FIToFICstmrCdtTrf.CdtTrfTxInf = []CreditTransferTransaction73{txInf}

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
