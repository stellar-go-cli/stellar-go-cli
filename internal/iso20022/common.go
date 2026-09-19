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

// ActiveOrHistoricCurrencyAndAmountGeneric — for non-IntrBk amount fields
type ActiveOrHistoricCurrencyAndAmountGeneric struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
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
	LEI    string                               `xml:"LEI,omitempty"`
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
	Nm        string           `xml:"Nm,omitempty"`
	PstlAdr   *PostalAddress24 `xml:"PstlAdr,omitempty"`
	Id        *Party38Choice   `xml:"Id,omitempty"`
	CtryOfRes string           `xml:"CtryOfRes,omitempty"`
}

// Party40Choice — party as a party or an agent (used in pacs.004 RtrChain)
type Party40Choice struct {
	Pty *PartyIdentification135                       `xml:"Pty,omitempty"`
	Agt *BranchAndFinancialInstitutionIdentification6 `xml:"Agt,omitempty"`
}

// ─────────────────────────────────────────────
// Financial institution identification
// ─────────────────────────────────────────────

// FinancialInstitutionIdentification18 — inner FinInstnId content
type FinancialInstitutionIdentification18 struct {
	BICFI       string                               `xml:"BICFI,omitempty"`
	ClrSysMmbId *ClearingSystemMemberIdentification2 `xml:"ClrSysMmbId,omitempty"`
	LEI         string                               `xml:"LEI,omitempty"`
	Nm          string                               `xml:"Nm,omitempty"`
	PstlAdr     *PostalAddress24                     `xml:"PstlAdr,omitempty"`
	Othr        *GenericFinancialIdentification1     `xml:"Othr,omitempty"`
}

// GenericFinancialIdentification1 — Othr identification for an FI
type GenericFinancialIdentification1 struct {
	Id      string                                    `xml:"Id"`
	SchmeNm *FinancialIdentificationSchemeName1Choice `xml:"SchmeNm,omitempty"`
	Issr    string                                    `xml:"Issr,omitempty"`
}

// FinancialIdentificationSchemeName1Choice
type FinancialIdentificationSchemeName1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// BranchAndFinancialInstitutionIdentification6 — agent identification
type BranchAndFinancialInstitutionIdentification6 struct {
	FinInstnId *FinancialInstitutionIdentification18 `xml:"FinInstnId"`
	BrnchId    *BranchData3                          `xml:"BrnchId,omitempty"`
}

// BranchData3 — branch identification
type BranchData3 struct {
	Id      string           `xml:"Id,omitempty"`
	LEI     string           `xml:"LEI,omitempty"`
	Nm      string           `xml:"Nm,omitempty"`
	PstlAdr *PostalAddress24 `xml:"PstlAdr,omitempty"`
}

// ClearingSystemMemberIdentification2 — clearing system member ID
type ClearingSystemMemberIdentification2 struct {
	ClrSysId *ClearingSystemIdentification2Choice `xml:"ClrSysId,omitempty"`
	MmbId    string                               `xml:"MmbId"`
}

// ClearingSystemIdentification2Choice
type ClearingSystemIdentification2Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// ClearingSystemIdentification3Choice — for SttlmInf/ClrSys
type ClearingSystemIdentification3Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// ─────────────────────────────────────────────
// Accounts
// ─────────────────────────────────────────────

// CashAccount38 — account identification
type CashAccount38 struct {
	Id   *AccountIdentification4Choice `xml:"Id"`
	Tp   *CashAccountType2Choice       `xml:"Tp,omitempty"`
	Ccy  string                        `xml:"Ccy,omitempty"`
	Nm   string                        `xml:"Nm,omitempty"`
	Prxy *ProxyAccountIdentification1  `xml:"Prxy,omitempty"`
}

// CashAccountType2Choice
type CashAccountType2Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// ProxyAccountIdentification1
type ProxyAccountIdentification1 struct {
	Tp *ProxyAccountType1Choice `xml:"Tp,omitempty"`
	Id string                   `xml:"Id"`
}

// ProxyAccountType1Choice
type ProxyAccountType1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// AccountIdentification4Choice — IBAN or other
type AccountIdentification4Choice struct {
	IBAN string                         `xml:"IBAN,omitempty"`
	Othr *GenericAccountIdentification1 `xml:"Othr,omitempty"`
}

// GenericAccountIdentification1
type GenericAccountIdentification1 struct {
	Id      string                    `xml:"Id"`
	SchmeNm *AccountSchemeName1Choice `xml:"SchmeNm,omitempty"`
	Issr    string                    `xml:"Issr,omitempty"`
}

// AccountSchemeName1Choice
type AccountSchemeName1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// ─────────────────────────────────────────────
// Purpose / remittance / charges
// ─────────────────────────────────────────────

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

// ─────────────────────────────────────────────
// Settlement
// ─────────────────────────────────────────────

// SettlementInstruction7 — settlement info inside GrpHdr (pacs.008/004/009)
// XSD sequence: SttlmMtd, SttlmAcct?, ClrSys?, InstgRmbrsmntAgt?, InstgRmbrsmntAgtAcct?,
// InstdRmbrsmntAgt?, InstdRmbrsmntAgtAcct?, ThrdRmbrsmntAgt?, ThrdRmbrsmntAgtAcct?
// NOTE: no SttlmDt — settlement date lives at IntrBkSttlmDt (tx level) or GrpHdr/IntrBkSttlmDt.
type SettlementInstruction7 struct {
	SttlmMtd             string                                        `xml:"SttlmMtd"`
	SttlmAcct            *CashAccount38                                `xml:"SttlmAcct,omitempty"`
	ClrSys               *ClearingSystemIdentification3Choice          `xml:"ClrSys,omitempty"`
	InstgRmbrsmntAgt     *BranchAndFinancialInstitutionIdentification6 `xml:"InstgRmbrsmntAgt,omitempty"`
	InstgRmbrsmntAgtAcct *CashAccount38                                `xml:"InstgRmbrsmntAgtAcct,omitempty"`
	InstdRmbrsmntAgt     *BranchAndFinancialInstitutionIdentification6 `xml:"InstdRmbrsmntAgt,omitempty"`
	InstdRmbrsmntAgtAcct *CashAccount38                                `xml:"InstdRmbrsmntAgtAcct,omitempty"`
	ThrdRmbrsmntAgt      *BranchAndFinancialInstitutionIdentification6 `xml:"ThrdRmbrsmntAgt,omitempty"`
	ThrdRmbrsmntAgtAcct  *CashAccount38                                `xml:"ThrdRmbrsmntAgtAcct,omitempty"`
}

// ─────────────────────────────────────────────
// Group headers (per-message — XSD sequences differ)
// ─────────────────────────────────────────────

// GroupHeader93 — pacs.008.001.08 / pacs.009.001.10
// XSD sequence: MsgId, CreDtTm, BtchBookg?, NbOfTxs, CtrlSum?, TtlIntrBkSttlmAmt?,
// IntrBkSttlmDt?, SttlmInf, PmtTpInf?, InstgAgt?, InstdAgt?
type GroupHeader93 struct {
	MsgId             string                                        `xml:"MsgId"`
	CreDtTm           string                                        `xml:"CreDtTm"`
	BtchBookg         *bool                                         `xml:"BtchBookg,omitempty"`
	NbOfTxs           string                                        `xml:"NbOfTxs"`
	CtrlSum           string                                        `xml:"CtrlSum,omitempty"`
	TtlIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"TtlIntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt     string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf          *SettlementInstruction7                       `xml:"SttlmInf"`
	PmtTpInf          *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	InstgAgt          *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt          *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
}

// GroupHeader91 — pacs.002.001.12
// XSD sequence: MsgId, CreDtTm, InstgAgt?, InstdAgt?, OrgnlBizQry?
// NOTE: no NbOfTxs, no SttlmInf, no InitgPty
type GroupHeader91 struct {
	MsgId       string                                        `xml:"MsgId"`
	CreDtTm     string                                        `xml:"CreDtTm"`
	InstgAgt    *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt    *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
	OrgnlBizQry *OriginalBusinessQuery1                       `xml:"OrgnlBizQry,omitempty"`
}

// OriginalBusinessQuery1 — reference to original query (pacs.002)
type OriginalBusinessQuery1 struct {
	MsgId   string `xml:"MsgId"`
	MsgNmId string `xml:"MsgNmId,omitempty"`
	CreDtTm string `xml:"CreDtTm,omitempty"`
}

// GroupHeader90 — pacs.004.001.12
// XSD sequence: MsgId, CreDtTm, BtchBookg?, NbOfTxs, CtrlSum?, TtlRtrdIntrBkSttlmAmt?,
// IntrBkSttlmDt?, SttlmInf, InstgAgt?, InstdAgt?
type GroupHeader90 struct {
	MsgId                 string                                        `xml:"MsgId"`
	CreDtTm               string                                        `xml:"CreDtTm"`
	BtchBookg             *bool                                         `xml:"BtchBookg,omitempty"`
	NbOfTxs               string                                        `xml:"NbOfTxs"`
	CtrlSum               string                                        `xml:"CtrlSum,omitempty"`
	TtlRtrdIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"TtlRtrdIntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt         string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf              *SettlementInstruction7                       `xml:"SttlmInf"`
	InstgAgt              *BranchAndFinancialInstitutionIdentification6 `xml:"InstgAgt,omitempty"`
	InstdAgt              *BranchAndFinancialInstitutionIdentification6 `xml:"InstdAgt,omitempty"`
}

// PaymentTypeInformation28 — payment type info (GrpHdr level)
type PaymentTypeInformation28 struct {
	InstrPrty string                  `xml:"InstrPrty,omitempty"`
	ClrChanl  string                  `xml:"ClrChanl,omitempty"`
	SvcLvl    []ServiceLevel8Choice   `xml:"SvcLvl,omitempty"`
	LclInstrm *LocalInstrument2Choice `xml:"LclInstrm,omitempty"`
	CtgyPurp  *CategoryPurpose1Choice `xml:"CtgyPurp,omitempty"`
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

// ─────────────────────────────────────────────
// Payment identification
// ─────────────────────────────────────────────

// PaymentIdentification7 — payment identification (pacs.008.001.08 / pacs.009.001.10)
// XSD sequence: InstrId?, EndToEndId, TxId, UETR, ClrSysRef?
// NOTE: TxId and UETR are mandatory in this version.
type PaymentIdentification7 struct {
	InstrId    string `xml:"InstrId,omitempty"`
	EndToEndId string `xml:"EndToEndId"`
	TxId       string `xml:"TxId"`
	UETR       string `xml:"UETR"`
	ClrSysRef  string `xml:"ClrSysRef,omitempty"`
}

// ─────────────────────────────────────────────
// Original group / transaction references
// ─────────────────────────────────────────────

// OriginalGroupHeader21 — original group info (pacs.004)
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?, OrgnlNbOfTxs?, OrgnlCtrlSum?, RtrRsnInf*
type OriginalGroupHeader21 struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
	OrgnlNbOfTxs string `xml:"OrgnlNbOfTxs,omitempty"`
	OrgnlCtrlSum string `xml:"OrgnlCtrlSum,omitempty"`
}

// OriginalGroupHeader17 — original group info (pacs.002 TxInfAndSts level)
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?
type OriginalGroupHeader17 struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
}

// OriginalTransactionReference28 — reference to original transaction (pacs.002/pacs.004)
// XSD sequence (relevant subset): IntrBkSttlmAmt?, Amt?, IntrBkSttlmDt?, ReqrdColltnDt?,
// ReqrdExctnDt?, CdtrSchmeId?, SttlmInf?, PmtTpInf?, PmtMtd?, MndtRltdInf?, RmtInf?,
// UltmtDbtr?, Dbtr?, DbtrAcct?, DbtrAgt?, DbtrAgtAcct?, CdtrAgt?, CdtrAgtAcct?,
// Cdtr?, CdtrAcct?, UltmtCdtr?, Purp?
type OriginalTransactionReference28 struct {
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf       *SettlementInstruction7                       `xml:"SttlmInf,omitempty"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
	UltmtDbtr      *Party40Choice                                `xml:"UltmtDbtr,omitempty"`
	Dbtr           *Party40Choice                                `xml:"Dbtr,omitempty"`
	DbtrAcct       *CashAccount38                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"DbtrAgt,omitempty"`
	DbtrAgtAcct    *CashAccount38                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification6 `xml:"CdtrAgt,omitempty"`
	CdtrAgtAcct    *CashAccount38                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *Party40Choice                                `xml:"Cdtr,omitempty"`
	CdtrAcct       *CashAccount38                                `xml:"CdtrAcct,omitempty"`
	UltmtCdtr      *Party40Choice                                `xml:"UltmtCdtr,omitempty"`
	Purp           *Purpose1Choice                               `xml:"Purp,omitempty"`
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// agentByBIC builds an agent identified by BICFI
func agentByBIC(bic string) *BranchAndFinancialInstitutionIdentification6 {
	return &BranchAndFinancialInstitutionIdentification6{
		FinInstnId: &FinancialInstitutionIdentification18{BICFI: bic},
	}
}

// agentByOtherID builds an agent identified by a generic Othr/Id (e.g. "NOTPROVIDED"
// or a Stellar address) for mandatory agent elements when no BIC is available.
func agentByOtherID(id string) *BranchAndFinancialInstitutionIdentification6 {
	return &BranchAndFinancialInstitutionIdentification6{
		FinInstnId: &FinancialInstitutionIdentification18{
			Othr: &GenericFinancialIdentification1{Id: id},
		},
	}
}

// agentOrFallback returns agentByBIC(bic) if non-empty, else agentByOtherID(fallbackID)
func agentOrFallback(bic, fallbackID string) *BranchAndFinancialInstitutionIdentification6 {
	if bic != "" {
		return agentByBIC(bic)
	}
	if fallbackID == "" {
		fallbackID = "NOTPROVIDED"
	}
	return agentByOtherID(fallbackID)
}
