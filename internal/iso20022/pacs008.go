package iso20022

import (
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
	XMLName          xml.Name `xml:"Document"`
	Xmlns            string   `xml:"xmlns,attr"`
	FIToFICstmrCdtTrf struct {
		XMLName      xml.Name                     `xml:"FIToFICstmrCdtTrf"`
		GrpHdr       GroupHeader                   `xml:"GrpHdr"`
		CdtTrfTxInf  []CreditTransferTransaction30 `xml:"CdtTrfTxInf"`
	} `xml:"FIToFICstmrCdtTrf"`
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

	doc.FIToFICstmrCdtTrf.GrpHdr = GroupHeader{
		MsgId:   msgID,
		CreDtTm: creDtTm,
		NbOfTxs: "1",
		SttlmInf: &SettlementInformation1{
			SttlmMtd: "CLRG",
			SttlmDt:  sttlmDt,
		},
	}

	if opts.InstgBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstgAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstgBIC,
		}
	}
	if opts.InstdBIC != "" {
		doc.FIToFICstmrCdtTrf.GrpHdr.InstdAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.InstdBIC,
		}
	}

	doc.FIToFICstmrCdtTrf.GrpHdr.InitgPty = &PartyIdentification135{
		Nm: "MozartPay",
	}

	txInf := CreditTransferTransaction30{
		PmtId: &PaymentIdentification4{
			InstrId:    opts.InstrID,
			EndToEndId: endToEndID,
			TxId:       txID,
			UETR:       uetr,
		},
		IntrBkSttlmDt:  sttlmDt,
		IntrBkSttlmAmt: &ActiveOrHistoricCurrencyAndAmount{
			Ccy:   currency,
			Value: p.Amount,
		},
	}

	if opts.ChargeBearer != "" {
		txInf.ChrgBr = opts.ChargeBearer
	} else {
		txInf.ChrgBr = string(ChrgBrShar)
	}

	if opts.DbtrBIC != "" {
		txInf.DbtrAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.DbtrBIC,
		}
	}
	if opts.CdtrBIC != "" {
		txInf.CdtrAgt = &BranchAndFinancialInstitutionIdentification6{
			BICFI: opts.CdtrBIC,
		}
	}

	dbtrName := opts.DbtrName
	if dbtrName == "" {
		dbtrName = safeTruncate(p.From, 16)
	}
	txInf.Dbtr = &PartyIdentification135{
		Nm:      dbtrName,
		PstlAdr: opts.DbtrAddress,
	}

	cdtrName := opts.CdtrName
	if cdtrName == "" {
		cdtrName = safeTruncate(p.To, 16)
	}
	txInf.Cdtr = &PartyIdentification135{
		Nm:      cdtrName,
		PstlAdr: opts.CdtrAddress,
	}

	if opts.DbtrAcctIBAN != "" {
		txInf.DbtrAcct = &CashAccount38{
			Id: &AccountIdentification4Choice{IBAN: opts.DbtrAcctIBAN},
		}
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

	doc.FIToFICstmrCdtTrf.CdtTrfTxInf = []CreditTransferTransaction30{txInf}

	return MarshalXML(doc, NSPacs008)
}

// safeTruncate truncates a string to n chars, returning "" if input is shorter than n
func safeTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// generateUUIDv4 generates a RFC 4122 v4 UUID string
func generateUUIDv4() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> uint(i*8))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
