package iso20022

// ─────────────────────────────────────────────
// Namespace constants for each pacs message type
// ─────────────────────────────────────────────

const (
	NSPacs008 = "urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"
	NSPacs002 = "urn:iso:std:iso:20022:tech:xsd:pacs.002.001.12"
	NSPacs004 = "urn:iso:std:iso:20022:tech:xsd:pacs.004.001.12"
	NSPacs009 = "urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10"
)

// ─────────────────────────────────────────────
// Code enums
// ─────────────────────────────────────────────

// ChargeBearerType1Code — who bears the charges
type ChargeBearerType1Code string

const (
	ChrgBrDebt ChargeBearerType1Code = "DEBT" // Borne by debtor
	ChrgBrCred ChargeBearerType1Code = "CRED" // Borne by creditor
	ChrgBrShar ChargeBearerType1Code = "SHAR" // Shared
	ChrgBrSlev ChargeBearerType1Code = "SLEV" // Service level
)

// ClearingChannel2Code — clearing channel identifier
type ClearingChannel2Code string

const (
	ClrChnRTGS ClearingChannel2Code = "RTGS"
	ClrChnACH  ClearingChannel2Code = "ACH"
	ClrChnCBT  ClearingChannel2Code = "CBT"
)

// Priority3Code — payment priority
type Priority3Code string

const (
	PriHigh Priority3Code = "HIGH"
	PriNorm Priority3Code = "NORM"
)

// ServiceLevel8Code — service level
type ServiceLevel8Code string

const (
	SvcURGP ServiceLevel8Code = "URGP" // Urgent
	SvcSDVA ServiceLevel8Code = "SDVA" // Same Day Value
	SvcPRPT ServiceLevel8Code = "PRPT" // Priority
	SvcSVPG ServiceLevel8Code = "SVPG" // SEPA
)

// TransactionStatus — for pacs.002
type TransactionStatus string

const (
	TxStsACSC TransactionStatus = "ACSC" // Accepted - Settlement Completed
	TxStsRJCT TransactionStatus = "RJCT" // Rejected
	TxStsPDNG TransactionStatus = "PDNG" // Pending
	TxStsACCP TransactionStatus = "ACCP" // Accepted
	TxStsACSP TransactionStatus = "ACSP" // Accepted - In Progress
)

// ─────────────────────────────────────────────
// Shared structural types
// ─────────────────────────────────────────────

// ActiveOrHistoricCurrencyAndAmount — amount with currency attribute
type ActiveOrHistoricCurrencyAndAmount struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
}

// ActiveOrHistoricCurrencyAndAmountGeneric — for non-IntrBk fields
type ActiveOrHistoricCurrencyAndAmountGeneric struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
}

// BICFIIdentifier — financial institution identification by BIC
type BICFIIdentifier struct {
	BICFI string `xml:"BICFI,omitempty"`
}

// BranchAndFinancialInstitutionIdentification6 — full agent identification
type BranchAndFinancialInstitutionIdentification6 struct {
	BICFI       string                               `xml:"BICFI,omitempty"`
	ClrSysMmbId *ClearingSystemMemberIdentification2 `xml:"ClrSysMmbId,omitempty"`
	Nm          string                               `xml:"Nm,omitempty"`
}

// ClearingSystemMemberIdentification2 — clearing system member ID
type ClearingSystemMemberIdentification2 struct {
	ClrSysId string `xml:"ClrSysId>Cd,omitempty"`
	MmbId    string `xml:"MmbId,omitempty"`
}

// PostalAddress24 — structured postal address
type PostalAddress24 struct {
	StreetNm    string   `xml:"StrtNm,omitempty"`
	BldgNb      string   `xml:"BldgNb,omitempty"`
	BldgNm      string   `xml:"BldgNm,omitempty"`
	Floor       string   `xml:"Flr,omitempty"`
	PstCd       string   `xml:"PstCd,omitempty"`
	TwnNm       string   `xml:"TwnNm,omitempty"`
	TwnLctnNm   string   `xml:"TwnLctnNm,omitempty"`
	DstrctNm    string   `xml:"DstrctNm,omitempty"`
	CtrySubDvsn string   `xml:"CtrySubDvsn,omitempty"`
	Ctry        string   `xml:"Ctry,omitempty"`
	AdrLine     []string `xml:"AdrLine,omitempty"`
}

// NamePrefix1Code — name prefix
type NamePrefix1Code string

const (
	NmPrfxMister NamePrefix1Code = "MIST"
	NmPrfxMiss   NamePrefix1Code = "MISS"
	NmPrfxMadam  NamePrefix1Code = "MADM"
	NmPrfxMs     NamePrefix1Code = "MIMS"
)

// PersonIdentification13 — person identification
type PersonIdentification13 struct {
	DtAndPlcOfBirth *DateAndPlaceOfBirth1          `xml:"DtAndPlcOfBirth,omitempty"`
	Othr            []GenericPersonIdentification1 `xml:"Othr,omitempty"`
}

// DateAndPlaceOfBirth1
type DateAndPlaceOfBirth1 struct {
	BirthDt     string `xml:"BirthDt"`
	CityOfBirth string `xml:"CityOfBirth,omitempty"`
	CtryOfBirth string `xml:"CtryOfBirth,omitempty"`
}

// GenericPersonIdentification1
type GenericPersonIdentification1 struct {
	Id      string `xml:"Id"`
	SchmeNm string `xml:"SchmeNm>Cd,omitempty"`
	Issr    string `xml:"Issr,omitempty"`
}

// OrganisationIdentification29 — organisation identification
type OrganisationIdentification29 struct {
	AnyBIC string                               `xml:"AnyBIC,omitempty"`
	Othr   []GenericOrganisationIdentification1 `xml:"Othr,omitempty"`
}

// GenericOrganisationIdentification1
type GenericOrganisationIdentification1 struct {
	Id      string `xml:"Id"`
	SchmeNm string `xml:"SchmeNm>Cd,omitempty"`
	Issr    string `xml:"Issr,omitempty"`
}

// Party38Choice — party as person or organisation
type Party38Choice struct {
	PrvtId *PersonIdentification13       `xml:"PrvtId,omitempty"`
	OrgId  *OrganisationIdentification29 `xml:"OrgId,omitempty"`
}

// PartyIdentification135 — name + address + identification
type PartyIdentification135 struct {
	Nm      string           `xml:"Nm,omitempty"`
	PstlAdr *PostalAddress24 `xml:"PstlAdr,omitempty"`
	Id      *Party38Choice   `xml:"Id,omitempty"`
}

// BranchAndFinancialInstitutionIdentification6 wrapper for agent
type Agent struct {
	FinInstnId *BranchAndFinancialInstitutionIdentification6 `xml:"FinInstnId"`
}

// CashAccount38 — account identification
type CashAccount38 struct {
	Id *AccountIdentification4Choice `xml:"Id"`
	Nm string                        `xml:"Nm,omitempty"`
}

// AccountIdentification4Choice — IBAN or other
type AccountIdentification4Choice struct {
	IBAN string                         `xml:"IBAN,omitempty"`
	Othr *GenericAccountIdentification1 `xml:"Othr,omitempty"`
}

// GenericAccountIdentification1
type GenericAccountIdentification1 struct {
	Id      string `xml:"Id"`
	SchmeNm string `xml:"SchmeNm>Cd,omitempty"`
	Issr    string `xml:"Issr,omitempty"`
}

// Purpose1Choice — payment purpose
type Purpose1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// CategoryPurpose1Choice — high-level purpose category
type CategoryPurpose1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// RemittanceInformation2 — remittance info (structured or unstructured)
type RemittanceInformation2 struct {
	Ustrd []string                           `xml:"Ustrd,omitempty"`
	Strd  []StructuredRemittanceInformation1 `xml:"Strd,omitempty"`
}

// StructuredRemittanceInformation1
type StructuredRemittanceInformation1 struct {
	RfrdDocInf  *ReferredDocumentInformation1 `xml:"RfrdDocInf,omitempty"`
	RmtLctn     string                        `xml:"RmtLctn,omitempty"`
	AddtlRmtInf string                        `xml:"AddtlRmtInf,omitempty"`
}

// ReferredDocumentInformation1
type ReferredDocumentInformation1 struct {
	Tp     *ReferredDocumentType1 `xml:"Tp,omitempty"`
	Nb     string                 `xml:"Nb,omitempty"`
	RltdDt string                 `xml:"RltdDt,omitempty"`
}

// ReferredDocumentType1
type ReferredDocumentType1 struct {
	CdOrPrtry string `xml:"CdOrPrtry>Cd,omitempty"`
	Prtry     string `xml:"CdOrPrtry>Prtry,omitempty"`
	Issr      string `xml:"Issr,omitempty"`
}

// ChargesInformation1 — charges info
type ChargesInformation1 struct {
	Amt    *ActiveOrHistoricCurrencyAndAmountGeneric `xml:"Amt,omitempty"`
	CdtPty string                                    `xml:"CdtPty,omitempty"`
}

// SettlementInformation1 — settlement method
type SettlementInformation1 struct {
	SttlmMtd string `xml:"SttlmMtd"`
	SttlmDt  string `xml:"SttlmDt,omitempty"`
}

// GroupHeader — shared group header (varies by message, use as base)
type GroupHeader struct {
	MsgId    string                                        `xml:"MsgId"`
	CreDtTm  string                                        `xml:"CreDtTm"`
	NbOfTxs  string                                        `xml:"NbOfTxs"`
	SttlmInf *SettlementInformation1                       `xml:"SttlmInf,omitempty"`
	InstgAgt *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt>FinInstnId,omitempty"`
	InstdAgt *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt>FinInstnId,omitempty"`
	InitgPty *PartyIdentification135                       `xml:"InitgPty,omitempty"`
}

// PaymentIdentification4 — payment identification
type PaymentIdentification4 struct {
	InstrId    string `xml:"InstrId,omitempty"`
	EndToEndId string `xml:"EndToEndId"`
	TxId       string `xml:"TxId,omitempty"`
	UETR       string `xml:"UETR,omitempty"`
}

// CreditTransferTransaction30 — single credit transfer transaction
type CreditTransferTransaction30 struct {
	PmtId          *PaymentIdentification4                       `xml:"PmtId"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt,omitempty"`
	ChrgBr         string                                        `xml:"ChrgBr,omitempty"`
	ChrgsInf       []ChargesInformation1                         `xml:"ChrgsInf,omitempty"`
	InstgAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt>FinInstnId,omitempty"`
	InstdAgt       *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt>FinInstnId,omitempty"`
	Dbtr           *PartyIdentification135                       `xml:"Dbtr,omitempty"`
	DbtrAcct       *CashAccount38                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt>FinInstnId,omitempty"`
	Cdtr           *PartyIdentification135                       `xml:"Cdtr,omitempty"`
	CdtrAcct       *CashAccount38                                `xml:"CdtrAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt>FinInstnId,omitempty"`
	Purp           *Purpose1Choice                               `xml:"Purp,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
}

// ServiceLevel8Choice — service level wrapper
type ServiceLevel8Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// LocalInstrument2Choice — local instrument
type LocalInstrument2Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}
