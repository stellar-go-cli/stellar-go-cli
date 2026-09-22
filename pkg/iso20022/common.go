package iso20022

import (
	"fmt"
	"math/big"
	"strings"
)

// ─────────────────────────────────────────────
// Namespace constants for each pacs message type
// ─────────────────────────────────────────────

const (
	NSPacs008 = "urn:iso:std:iso:20022:tech:xsd:pacs.008.001.14"
	NSPacs002 = "urn:iso:std:iso:20022:tech:xsd:pacs.002.001.16"
	NSPacs004 = "urn:iso:std:iso:20022:tech:xsd:pacs.004.001.15"
	NSPacs009 = "urn:iso:std:iso:20022:tech:xsd:pacs.009.001.13"
	NSPain001 = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.13"
	NSCamt054 = "urn:iso:std:iso:20022:tech:xsd:camt.054.001.14"
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

// PostalAddress27 — structured postal address
type PostalAddress27 struct {
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

// PersonIdentification18 — person identification
type PersonIdentification18 struct {
	DtAndPlcOfBirth *DateAndPlaceOfBirth1          `xml:"DtAndPlcOfBirth,omitempty"`
	Othr            []GenericPersonIdentification2 `xml:"Othr,omitempty"`
}

// DateAndPlaceOfBirth1
type DateAndPlaceOfBirth1 struct {
	BirthDt     string `xml:"BirthDt"`
	CityOfBirth string `xml:"CityOfBirth,omitempty"`
	CtryOfBirth string `xml:"CtryOfBirth,omitempty"`
}

// GenericPersonIdentification2
type GenericPersonIdentification2 struct {
	Id      string                                 `xml:"Id"`
	SchmeNm *PersonIdentificationSchemeName1Choice `xml:"SchmeNm,omitempty"`
	Issr    string                                 `xml:"Issr,omitempty"`
}

// PersonIdentificationSchemeName1Choice
type PersonIdentificationSchemeName1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// OrganisationIdentification39 — organisation identification
type OrganisationIdentification39 struct {
	AnyBIC string                               `xml:"AnyBIC,omitempty"`
	LEI    string                               `xml:"LEI,omitempty"`
	Othr   []GenericOrganisationIdentification3 `xml:"Othr,omitempty"`
}

// GenericOrganisationIdentification3
type GenericOrganisationIdentification3 struct {
	Id      string                                       `xml:"Id"`
	SchmeNm *OrganisationIdentificationSchemeName1Choice `xml:"SchmeNm,omitempty"`
	Issr    string                                       `xml:"Issr,omitempty"`
}

// OrganisationIdentificationSchemeName1Choice
type OrganisationIdentificationSchemeName1Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// Party52Choice — party as person or organisation
type Party52Choice struct {
	PrvtId *PersonIdentification18       `xml:"PrvtId,omitempty"`
	OrgId  *OrganisationIdentification39 `xml:"OrgId,omitempty"`
}

// PartyIdentification272 — name + address + identification.
// XSD sequence: Nm?, PstlAdr?, Id?, CtryOfRes?, CtctDtls?
type PartyIdentification272 struct {
	Nm        string           `xml:"Nm,omitempty"`
	PstlAdr   *PostalAddress27 `xml:"PstlAdr,omitempty"`
	Id        *Party52Choice   `xml:"Id,omitempty"`
	CtryOfRes string           `xml:"CtryOfRes,omitempty"`
	CtctDtls  *Contact13       `xml:"CtctDtls,omitempty"`
}

// Contact13 — contact details (CtctDtls). Subset of the XSD sequence.
type Contact13 struct {
	Nm       string `xml:"Nm,omitempty"`
	PhneNb   string `xml:"PhneNb,omitempty"`
	MobNb    string `xml:"MobNb,omitempty"`
	EmailAdr string `xml:"EmailAdr,omitempty"`
}

// Party50Choice — party as a party or an agent (used in pacs.004 RtrChain)
type Party50Choice struct {
	Pty *PartyIdentification272                       `xml:"Pty,omitempty"`
	Agt *BranchAndFinancialInstitutionIdentification8 `xml:"Agt,omitempty"`
}

// ─────────────────────────────────────────────
// Financial institution identification
// ─────────────────────────────────────────────

// FinancialInstitutionIdentification23 — inner FinInstnId content
type FinancialInstitutionIdentification23 struct {
	BICFI       string                               `xml:"BICFI,omitempty"`
	ClrSysMmbId *ClearingSystemMemberIdentification2 `xml:"ClrSysMmbId,omitempty"`
	LEI         string                               `xml:"LEI,omitempty"`
	Nm          string                               `xml:"Nm,omitempty"`
	PstlAdr     *PostalAddress27                     `xml:"PstlAdr,omitempty"`
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

// BranchAndFinancialInstitutionIdentification8 — agent identification
type BranchAndFinancialInstitutionIdentification8 struct {
	FinInstnId *FinancialInstitutionIdentification23 `xml:"FinInstnId"`
	BrnchId    *BranchData5                          `xml:"BrnchId,omitempty"`
}

// BranchData5 — branch identification
type BranchData5 struct {
	Id      string           `xml:"Id,omitempty"`
	LEI     string           `xml:"LEI,omitempty"`
	Nm      string           `xml:"Nm,omitempty"`
	PstlAdr *PostalAddress27 `xml:"PstlAdr,omitempty"`
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

// CashAccount40 — account identification
type CashAccount40 struct {
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

// Purpose2Choice — payment purpose
type Purpose2Choice struct {
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

// Charges16 — charges info.
// XSD sequence: Amt, Agt, Tp?
type Charges16 struct {
	Amt *ActiveOrHistoricCurrencyAndAmount            `xml:"Amt"`
	Agt *BranchAndFinancialInstitutionIdentification8 `xml:"Agt"`
	Tp  *ChargeType3Choice                            `xml:"Tp,omitempty"`
}

// ChargeType3Choice — charge type as code or proprietary
type ChargeType3Choice struct {
	Cd    string `xml:"Cd,omitempty"`
	Prtry string `xml:"Prtry,omitempty"`
}

// ─────────────────────────────────────────────
// Settlement
// ─────────────────────────────────────────────

// SettlementInstruction15 — settlement info inside GrpHdr (pacs.008/004/009)
// XSD sequence: SttlmMtd, SttlmAcct?, ClrSys?, InstgRmbrsmntAgt?, InstgRmbrsmntAgtAcct?,
// InstdRmbrsmntAgt?, InstdRmbrsmntAgtAcct?, ThrdRmbrsmntAgt?, ThrdRmbrsmntAgtAcct?
// NOTE: no SttlmDt — settlement date lives at IntrBkSttlmDt (tx level) or GrpHdr/IntrBkSttlmDt.
type SettlementInstruction15 struct {
	SttlmMtd             string                                        `xml:"SttlmMtd"`
	SttlmAcct            *CashAccount40                                `xml:"SttlmAcct,omitempty"`
	ClrSys               *ClearingSystemIdentification3Choice          `xml:"ClrSys,omitempty"`
	InstgRmbrsmntAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"InstgRmbrsmntAgt,omitempty"`
	InstgRmbrsmntAgtAcct *CashAccount40                                `xml:"InstgRmbrsmntAgtAcct,omitempty"`
	InstdRmbrsmntAgt     *BranchAndFinancialInstitutionIdentification8 `xml:"InstdRmbrsmntAgt,omitempty"`
	InstdRmbrsmntAgtAcct *CashAccount40                                `xml:"InstdRmbrsmntAgtAcct,omitempty"`
	ThrdRmbrsmntAgt      *BranchAndFinancialInstitutionIdentification8 `xml:"ThrdRmbrsmntAgt,omitempty"`
	ThrdRmbrsmntAgtAcct  *CashAccount40                                `xml:"ThrdRmbrsmntAgtAcct,omitempty"`
}

// ─────────────────────────────────────────────
// Group headers (per-message — XSD sequences differ)
// ─────────────────────────────────────────────

// GroupHeader131 — pacs.008.001.14 / pacs.009.001.13
// XSD sequence: MsgId, CreDtTm, XpryDtTm?, BtchBookg?, NbOfTxs, CtrlSum?, TtlIntrBkSttlmAmt?,
// IntrBkSttlmDt?, SttlmInf, PmtTpInf?, InstgAgt?, InstdAgt?
type GroupHeader131 struct {
	MsgId             string                                        `xml:"MsgId"`
	CreDtTm           string                                        `xml:"CreDtTm"`
	XpryDtTm          string                                        `xml:"XpryDtTm,omitempty"`
	BtchBookg         *bool                                         `xml:"BtchBookg,omitempty"`
	NbOfTxs           string                                        `xml:"NbOfTxs"`
	CtrlSum           string                                        `xml:"CtrlSum,omitempty"`
	TtlIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"TtlIntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt     string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf          *SettlementInstruction15                      `xml:"SttlmInf"`
	PmtTpInf          *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	InstgAgt          *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt          *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
}

// GroupHeader120 — pacs.002.001.16
// XSD sequence: MsgId, CreDtTm, InstgAgt?, InstdAgt?, OrgnlBizQry?
// NOTE: no NbOfTxs, no SttlmInf, no InitgPty
type GroupHeader120 struct {
	MsgId       string                                        `xml:"MsgId"`
	CreDtTm     string                                        `xml:"CreDtTm"`
	InstgAgt    *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt    *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
	OrgnlBizQry *OriginalBusinessQuery1                       `xml:"OrgnlBizQry,omitempty"`
}

// OriginalBusinessQuery1 — reference to original query (pacs.002)
type OriginalBusinessQuery1 struct {
	MsgId   string `xml:"MsgId"`
	MsgNmId string `xml:"MsgNmId,omitempty"`
	CreDtTm string `xml:"CreDtTm,omitempty"`
}

// GroupHeader123 — pacs.004.001.15
// XSD sequence: MsgId, CreDtTm, Authstn*, BtchBookg?, NbOfTxs, CtrlSum?, GrpRtr?,
// TtlRtrdIntrBkSttlmAmt?, IntrBkSttlmDt?, SttlmInf, PmtTpInf?, InstgAgt?, InstdAgt?
type GroupHeader123 struct {
	MsgId                 string                                        `xml:"MsgId"`
	CreDtTm               string                                        `xml:"CreDtTm"`
	BtchBookg             *bool                                         `xml:"BtchBookg,omitempty"`
	NbOfTxs               string                                        `xml:"NbOfTxs"`
	CtrlSum               string                                        `xml:"CtrlSum,omitempty"`
	GrpRtr                *bool                                         `xml:"GrpRtr,omitempty"`
	TtlRtrdIntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"TtlRtrdIntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt         string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf              *SettlementInstruction15                      `xml:"SttlmInf"`
	PmtTpInf              *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	InstgAgt              *BranchAndFinancialInstitutionIdentification8 `xml:"InstgAgt,omitempty"`
	InstdAgt              *BranchAndFinancialInstitutionIdentification8 `xml:"InstdAgt,omitempty"`
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

// PaymentIdentification13 — payment identification
// XSD sequence: InstrId?, EndToEndId, TxId?, UETR?, ClrSysRef?
// NOTE: TxId/UETR are optional in the latest versions; we always emit them.
type PaymentIdentification13 struct {
	InstrId    string `xml:"InstrId,omitempty"`
	EndToEndId string `xml:"EndToEndId"`
	TxId       string `xml:"TxId"`
	UETR       string `xml:"UETR"`
	ClrSysRef  string `xml:"ClrSysRef,omitempty"`
}

// ─────────────────────────────────────────────
// Original group / transaction references
// ─────────────────────────────────────────────

// OriginalGroupHeader19 — original group info (pacs.004.001.15 document level)
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?, RtrRsnInf*
// NOTE: no OrgnlNbOfTxs/OrgnlCtrlSum in this version.
type OriginalGroupHeader19 struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
}

// OriginalGroupHeader22 — original group info and status (pacs.002.001.16
// document level, element name OrgnlGrpInfAndSts)
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?, OrgnlNbOfTxs?,
// OrgnlCtrlSum?, GrpSts?, StsRsnInf*, NbOfTxsPerSts*
type OriginalGroupHeader22 struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
}

// OriginalGroupInformation33 — original group info (transaction level,
// pacs.002 TxInfAndSts / pacs.004 TxInf)
// XSD sequence: OrgnlMsgId, OrgnlMsgNmId, OrgnlCreDtTm?
type OriginalGroupInformation33 struct {
	OrgnlMsgId   string `xml:"OrgnlMsgId"`
	OrgnlMsgNmId string `xml:"OrgnlMsgNmId"`
	OrgnlCreDtTm string `xml:"OrgnlCreDtTm,omitempty"`
}

// OriginalTransactionReference45 — reference to original transaction.
// Covers pacs.004.001.15 OrgnlTxRef; identical emitted subset to
// OriginalTransactionReference47 (pacs.002.001.16).
// XSD sequence (relevant subset): IntrBkSttlmAmt?, Amt?, IntrBkSttlmDt?, ReqdColltnDt?,
// ReqdExctnDt?, CdtrSchmeId?, SttlmInf?, PmtTpInf?, PmtMtd?, MndtRltdInf?, RmtInf?,
// UltmtDbtr?, Dbtr?, DbtrAcct?, DbtrAgt?, DbtrAgtAcct?, CdtrAgt?, CdtrAgtAcct?,
// Cdtr?, CdtrAcct?, UltmtCdtr?, Purp?
type OriginalTransactionReference45 struct {
	IntrBkSttlmAmt *ActiveOrHistoricCurrencyAndAmount            `xml:"IntrBkSttlmAmt,omitempty"`
	IntrBkSttlmDt  string                                        `xml:"IntrBkSttlmDt,omitempty"`
	SttlmInf       *SettlementInstruction15                      `xml:"SttlmInf,omitempty"`
	PmtTpInf       *PaymentTypeInformation28                     `xml:"PmtTpInf,omitempty"`
	RmtInf         *RemittanceInformation2                       `xml:"RmtInf,omitempty"`
	UltmtDbtr      *Party50Choice                                `xml:"UltmtDbtr,omitempty"`
	Dbtr           *Party50Choice                                `xml:"Dbtr,omitempty"`
	DbtrAcct       *CashAccount40                                `xml:"DbtrAcct,omitempty"`
	DbtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"DbtrAgt,omitempty"`
	DbtrAgtAcct    *CashAccount40                                `xml:"DbtrAgtAcct,omitempty"`
	CdtrAgt        *BranchAndFinancialInstitutionIdentification8 `xml:"CdtrAgt,omitempty"`
	CdtrAgtAcct    *CashAccount40                                `xml:"CdtrAgtAcct,omitempty"`
	Cdtr           *Party50Choice                                `xml:"Cdtr,omitempty"`
	CdtrAcct       *CashAccount40                                `xml:"CdtrAcct,omitempty"`
	UltmtCdtr      *Party50Choice                                `xml:"UltmtCdtr,omitempty"`
	Purp           *Purpose2Choice                               `xml:"Purp,omitempty"`
}

// ─────────────────────────────────────────────
// Supplementary data
// ─────────────────────────────────────────────

// SupplementaryData1 — SplmtryData block (last element in tx sequences)
type SupplementaryData1 struct {
	PlcAndNm string                      `xml:"PlcAndNm,omitempty"`
	Envlp    *SupplementaryDataEnvelope1 `xml:"Envlp"`
}

// SupplementaryDataEnvelope1 — Envlp content. The XSD allows any XML here;
// we emit a proprietary Asset element carrying the non-ISO asset details.
type SupplementaryDataEnvelope1 struct {
	Asset *SupplementaryAsset `xml:"Asset,omitempty"`
}

// SupplementaryAsset — proprietary asset identification inside Envlp.
// Preserves the real asset code/issuer/exact amount when Ccy is XXX.
type SupplementaryAsset struct {
	Cd   string `xml:"Cd"`
	Issr string `xml:"Issr,omitempty"`
	Amt  string `xml:"Amt,omitempty"`
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

// iso4217Codes — active ISO 4217 alphabetic codes accepted as-is in Ccy
// attributes. Anything else (crypto asset codes like XLM/USDC) maps to XXX
// with the real asset preserved in SplmtryData.
var iso4217Codes = map[string]bool{
	"AED": true, "AFN": true, "ALL": true, "AMD": true, "ANG": true, "AOA": true,
	"ARS": true, "AUD": true, "AWG": true, "AZN": true,
	"BAM": true, "BBD": true, "BDT": true, "BGN": true, "BHD": true, "BIF": true,
	"BMD": true, "BND": true, "BOB": true, "BOV": true, "BRL": true, "BSD": true,
	"BTN": true, "BWP": true, "BYN": true, "BZD": true,
	"CAD": true, "CDF": true, "CHE": true, "CHF": true, "CHW": true, "CLF": true,
	"CLP": true, "CNY": true, "COP": true, "COU": true, "CRC": true, "CUC": true,
	"CUP": true, "CVE": true, "CZK": true,
	"DJF": true, "DKK": true, "DOP": true, "DZD": true,
	"EGP": true, "ERN": true, "ETB": true, "EUR": true,
	"FJD": true, "FKP": true,
	"GBP": true, "GEL": true, "GHS": true, "GIP": true, "GMD": true, "GNF": true,
	"GTQ": true, "GYD": true,
	"HKD": true, "HNL": true, "HRK": true, "HTG": true, "HUF": true,
	"IDR": true, "ILS": true, "INR": true, "IQD": true, "IRR": true, "ISK": true,
	"JMD": true, "JOD": true, "JPY": true,
	"KES": true, "KGS": true, "KHR": true, "KMF": true, "KPW": true, "KRW": true,
	"KWD": true, "KYD": true, "KZT": true,
	"LAK": true, "LBP": true, "LKR": true, "LRD": true, "LSL": true, "LYD": true,
	"MAD": true, "MDL": true, "MGA": true, "MKD": true, "MMK": true, "MNT": true,
	"MOP": true, "MRU": true, "MUR": true, "MVR": true, "MWK": true, "MXN": true,
	"MXV": true, "MYR": true, "MZN": true,
	"NAD": true, "NGN": true, "NIO": true, "NOK": true, "NPR": true, "NZD": true,
	"OMR": true,
	"PAB": true, "PEN": true, "PGK": true, "PHP": true, "PKR": true, "PLN": true,
	"PYG": true,
	"QAR": true,
	"RON": true, "RSD": true, "RUB": true, "RWF": true,
	"SAR": true, "SBD": true, "SCR": true, "SDG": true, "SEK": true, "SGD": true,
	"SHP": true, "SLE": true, "SLL": true, "SOS": true, "SRD": true, "SSP": true,
	"STN": true, "SVC": true, "SYP": true, "SZL": true,
	"THB": true, "TJS": true, "TMT": true, "TND": true, "TOP": true, "TRY": true,
	"TTD": true, "TWD": true, "TZS": true,
	"UAH": true, "UGX": true, "USD": true, "USN": true, "UYI": true, "UYU": true,
	"UYW": true, "UZS": true,
	"VED": true, "VES": true, "VND": true, "VUV": true,
	"WST": true,
	"XAF": true, "XAG": true, "XAU": true, "XBA": true, "XBB": true, "XBC": true,
	"XBD": true, "XCD": true, "XDR": true, "XOF": true, "XPD": true, "XPF": true,
	"XPT": true, "XSU": true, "XTS": true, "XUA": true, "XXX": true,
	"YER": true,
	"ZAR": true, "ZMW": true, "ZWL": true,
}

// settlementCurrency maps a payment asset to an ISO 4217 Ccy value.
// asset may be a bare code ("USDC") or Stellar "CODE:ISSUER" form; an
// explicit issuer argument (models.Payment.AssetIssuer) is used when the
// bare-code form is given.
// Non-ISO assets return "XXX" plus a SplmtryData block preserving the real
// asset code, issuer, and exact amount.
func settlementCurrency(asset, issuer, exactAmount string) (string, *SupplementaryData1) {
	code := asset
	if i := strings.IndexByte(asset, ':'); i >= 0 {
		code, issuer = asset[:i], asset[i+1:]
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if iso4217Codes[code] {
		return code, nil
	}
	return "XXX", &SupplementaryData1{
		Envlp: &SupplementaryDataEnvelope1{
			Asset: &SupplementaryAsset{Cd: code, Issr: issuer, Amt: exactAmount},
		},
	}
}

// sumAmounts returns the decimal sum of amount strings, for CtrlSum and
// total-amount elements in batch messages.
func sumAmounts(amounts []string) (string, error) {
	total := new(big.Rat)
	for _, a := range amounts {
		v, ok := new(big.Rat).SetString(strings.TrimSpace(a))
		if !ok {
			return "", fmt.Errorf("invalid amount %q", a)
		}
		total.Add(total, v)
	}
	return ratDecimalString(total), nil
}

// ratDecimalString renders a big.Rat as a plain decimal (no exponent),
// truncated to 18 fraction digits (the DecimalNumber limit).
func ratDecimalString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	s := r.FloatString(18)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// normalizeAmount truncates a decimal string to the 5 fraction digits allowed
// by ActiveOrHistoricCurrencyAndAmount (fractionDigits=5). Stellar amounts
// carry 7 decimals; the exact value is preserved in SplmtryData.
func normalizeAmount(amt string) string {
	if i := strings.IndexByte(amt, '.'); i >= 0 && len(amt)-i-1 > 5 {
		return amt[:i+6]
	}
	return amt
}

// agentByBIC builds an agent identified by BICFI
func agentByBIC(bic string) *BranchAndFinancialInstitutionIdentification8 {
	return &BranchAndFinancialInstitutionIdentification8{
		FinInstnId: &FinancialInstitutionIdentification23{BICFI: bic},
	}
}

// agentByOtherID builds an agent identified by a generic Othr/Id (e.g. "NOTPROVIDED"
// or a Stellar address) for mandatory agent elements when no BIC is available.
func agentByOtherID(id string) *BranchAndFinancialInstitutionIdentification8 {
	return &BranchAndFinancialInstitutionIdentification8{
		FinInstnId: &FinancialInstitutionIdentification23{
			Othr: &GenericFinancialIdentification1{Id: id},
		},
	}
}

// agentOrFallback returns agentByBIC(bic) if non-empty, else agentByOtherID(fallbackID)
func agentOrFallback(bic, fallbackID string) *BranchAndFinancialInstitutionIdentification8 {
	if bic != "" {
		return agentByBIC(bic)
	}
	if fallbackID == "" {
		fallbackID = "NOTPROVIDED"
	}
	return agentByOtherID(fallbackID)
}
