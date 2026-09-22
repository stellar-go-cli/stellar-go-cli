package iso20022

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
)

func demoPayment() *models.Payment {
	now := time.Now().UTC()
	confirmed := now.Add(5 * time.Second)
	return &models.Payment{
		ID:          "direct-demo0001",
		From:        "GDEMO_FROM0000000000000000000000000000000000000000000001",
		To:          "GDEMO_TO00000000000000000000000000000000000000000000000001",
		Amount:      "100.0000000",
		Asset:       "USDC",
		Rail:        models.RailDirect,
		Status:      models.PaymentConfirmed,
		Network:     models.NetworkStellarTestnet,
		Memo:        "Demo payment",
		Fee:         "0.00001 XLM",
		TxHash:      "DEMOA1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2",
		LedgerSeq:   50123456,
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}
}

// ─────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────

// childElements returns the local names of direct child elements of the element
// at the given path (e.g. "Document/FIToFICstmrCdtTrf/GrpHdr").
func childElements(t *testing.T, xmlStr, path string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(xmlStr))
	var stack []string
	var result []string
	want := strings.Split(path, "/")

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if len(stack) == len(want) {
				match := true
				for i, seg := range want {
					if stack[i] != seg {
						match = false
						break
					}
				}
				if match {
					result = append(result, el.Name.Local)
				}
			}
			stack = append(stack, el.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return result
}

// assertChildOrder verifies that the direct children of the element at path
// appear in an order consistent with the XSD sequence: every emitted child
// must exist in allowedOrder and their positions must be non-decreasing.
func assertChildOrder(t *testing.T, xmlStr, path string, allowedOrder []string) {
	t.Helper()
	children := childElements(t, xmlStr, path)
	if len(children) == 0 {
		t.Fatalf("no children found at %s", path)
	}
	pos := make(map[string]int, len(allowedOrder))
	for i, name := range allowedOrder {
		if _, dup := pos[name]; !dup {
			pos[name] = i
		}
	}
	last := -1
	for _, c := range children {
		idx, ok := pos[c]
		if !ok {
			t.Errorf("%s: child <%s> not in XSD sequence %v", path, c, allowedOrder)
			continue
		}
		if idx < last {
			t.Errorf("%s: child <%s> out of order (sequence %v, got children %v)", path, c, allowedOrder, children)
		}
		if idx > last {
			last = idx
		}
	}
}

// assertAbsent verifies the XML does not contain the given element name.
func assertAbsent(t *testing.T, xmlStr, elem string) {
	t.Helper()
	if strings.Contains(xmlStr, "<"+elem+">") || strings.Contains(xmlStr, "<"+elem+" ") {
		t.Errorf("XML must not contain <%s>", elem)
	}
}

// validateWithXSD validates xmlStr against an XSD in testdata/xsd using
// xmllint. Skips when xmllint or the XSD file is unavailable.
func validateWithXSD(t *testing.T, xmlStr, xsdFile string) {
	t.Helper()
	xsdPath := filepath.Join("testdata", "xsd", xsdFile)
	if _, err := os.Stat(xsdPath); err != nil {
		t.Skipf("XSD %s not present — download from iso20022.org (see testdata/xsd/README.md)", xsdFile)
	}
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not found in PATH")
	}
	f, err := os.CreateTemp(t.TempDir(), "msg-*.xml")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.WriteString(xmlStr); err != nil {
		t.Fatalf("write temp xml: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp xml: %v", err)
	}
	out, err := exec.Command("xmllint", "--noout", "--schema", xsdPath, f.Name()).CombinedOutput()
	if err != nil {
		t.Fatalf("XSD validation failed: %v\n%s", err, out)
	}
}

// ─────────────────────────────────────────────
// pacs.008
// ─────────────────────────────────────────────

func TestBuildPacs008(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	for _, want := range []string{
		"pacs.008.001.14", "FIToFICstmrCdtTrf", "GrpHdr", "CdtTrfTxInf",
		"EndToEndId", "IntrBkSttlmAmt", "Dbtr", "Cdtr", "DbtrAgt", "CdtrAgt",
		"ChrgBr", "UETR", "RmtInf",
		`Ccy="XXX"`, "SplmtryData", "<Cd>USDC</Cd>",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs008GrpHdrOrder(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), &Pacs008Options{
		InstgBIC: "MOZTATWW",
		InstdBIC: "CHASUS33",
	})
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FIToFICstmrCdtTrf/GrpHdr", []string{
		"MsgId", "CreDtTm", "XpryDtTm", "BtchBookg", "NbOfTxs", "CtrlSum",
		"TtlIntrBkSttlmAmt", "IntrBkSttlmDt", "SttlmInf", "PmtTpInf",
		"InstgAgt", "InstdAgt",
	})
}

func TestBuildPacs008TxOrder(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), &Pacs008Options{
		DbtrBIC:      "DEUTDEFF",
		CdtrBIC:      "BNPAFRPP",
		DbtrAcctIBAN: "AT123456789012345678",
		CdtrAcctIBAN: "DE987654321098765432",
		PurposeCode:  "GDS",
	})
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FIToFICstmrCdtTrf/CdtTrfTxInf", []string{
		"PmtId", "PmtTpInf", "IntrBkSttlmAmt", "IntrBkSttlmDt", "SttlmPrty",
		"SttlmTmIndctn", "SttlmTmReq", "AddtlDtTm",
		"InstdAmt", "XchgRate", "AgrdRate", "ChrgBr", "ChrgsInf",
		"MndtRltdInf", "PmtSgntr",
		"PrvsInstgAgt1", "PrvsInstgAgt1Acct", "PrvsInstgAgt2", "PrvsInstgAgt2Acct",
		"PrvsInstgAgt3", "PrvsInstgAgt3Acct",
		"InstgAgt", "InstdAgt",
		"IntrmyAgt1", "IntrmyAgt1Acct", "IntrmyAgt2", "IntrmyAgt2Acct",
		"IntrmyAgt3", "IntrmyAgt3Acct",
		"UltmtDbtr", "InitgPty",
		"Dbtr", "DbtrAcct", "DbtrAgt", "DbtrAgtAcct",
		"CdtrAgt", "CdtrAgtAcct", "Cdtr", "CdtrAcct", "UltmtCdtr",
		"InstrForCdtrAgt", "InstrForNxtAgt", "Purp", "RgltryRptg", "Tax",
		"RltdRmtInf", "RmtInf", "SplmtryData",
	})
}

func TestBuildPacs008ForbiddenElements(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	// SttlmDt is not part of SettlementInstruction15
	assertAbsent(t, xmlStr, "SttlmDt")
	// InitgPty must not appear inside GrpHdr
	grpHdrChildren := childElements(t, xmlStr, "Document/FIToFICstmrCdtTrf/GrpHdr")
	for _, c := range grpHdrChildren {
		if c == "InitgPty" {
			t.Error("GrpHdr must not contain InitgPty")
		}
	}
}

func TestBuildPacs008MandatoryAgents(t *testing.T) {
	// Without BICs, DbtrAgt/CdtrAgt must still be emitted (mandatory in XSD)
	xmlStr, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<DbtrAgt>") {
		t.Error("DbtrAgt missing (mandatory)")
	}
	if !strings.Contains(xmlStr, "<CdtrAgt>") {
		t.Error("CdtrAgt missing (mandatory)")
	}
	if !strings.Contains(xmlStr, "<Othr>") {
		t.Error("expected Othr/Id fallback for agents without BIC")
	}
}

func TestBuildPacs008RoundTrip(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}

	var doc Pacs008Document
	if err := UnmarshalXML([]byte(xmlStr), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}

	if doc.Xmlns != NSPacs008 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs008)
	}
	if len(doc.FIToFICstmrCdtTrf.CdtTrfTxInf) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.FIToFICstmrCdtTrf.CdtTrfTxInf))
	}
	tx := doc.FIToFICstmrCdtTrf.CdtTrfTxInf[0]
	if tx.PmtId == nil {
		t.Fatal("PmtId is nil")
	}
	if tx.PmtId.EndToEndId == "" {
		t.Error("EndToEndId is empty after round-trip")
	}
	if tx.PmtId.TxId == "" {
		t.Error("TxId is empty after round-trip (mandatory)")
	}
	if tx.PmtId.UETR == "" {
		t.Error("UETR is empty after round-trip (mandatory)")
	}
	if tx.IntrBkSttlmAmt == nil {
		t.Fatal("IntrBkSttlmAmt is nil")
	}
	// USDC is not ISO 4217 — Ccy must be XXX with the real asset in SplmtryData
	if tx.IntrBkSttlmAmt.Ccy != "XXX" {
		t.Errorf("currency mismatch: got %s, want XXX", tx.IntrBkSttlmAmt.Ccy)
	}
	// ActiveOrHistoricCurrencyAndAmount allows max 5 fraction digits
	if tx.IntrBkSttlmAmt.Value != "100.00000" {
		t.Errorf("amount mismatch: got %s, want 100.00000", tx.IntrBkSttlmAmt.Value)
	}
	if len(tx.SplmtryData) != 1 || tx.SplmtryData[0].Envlp == nil ||
		tx.SplmtryData[0].Envlp.Asset == nil || tx.SplmtryData[0].Envlp.Asset.Cd != "USDC" {
		t.Error("SplmtryData must preserve the original asset code USDC")
	}
	if tx.DbtrAgt == nil || tx.DbtrAgt.FinInstnId == nil {
		t.Error("DbtrAgt/FinInstnId missing after round-trip")
	}
	if tx.CdtrAgt == nil || tx.CdtrAgt.FinInstnId == nil {
		t.Error("CdtrAgt/FinInstnId missing after round-trip")
	}
}

func TestBuildPacs008WithOptions(t *testing.T) {
	opts := &Pacs008Options{
		InstgBIC:       "MOZTATWW",
		InstdBIC:       "CHASUS33",
		DbtrBIC:        "DEUTDEFF",
		CdtrBIC:        "BNPAFRPP",
		DbtrName:       "Alice Corp",
		CdtrName:       "Bob Inc",
		DbtrAcctIBAN:   "AT123456789012345678",
		CdtrAcctIBAN:   "DE987654321098765432",
		ChargeBearer:   "SHAR",
		PurposeCode:    "GDS",
		RemittanceInfo: []string{"Invoice 12345"},
	}

	xmlStr, err := BuildPacs008(demoPayment(), opts)
	if err != nil {
		t.Fatalf("BuildPacs008 with options failed: %v", err)
	}
	for _, want := range []string{
		"MOZTATWW", "CHASUS33", "DEUTDEFF", "BNPAFRPP",
		"Alice Corp", "Bob Inc",
		"AT123456789012345678", "DE987654321098765432",
		"GDS", "Invoice 12345",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
}

func TestXSDValidationPacs008(t *testing.T) {
	xmlStr, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.008.001.14.xsd")
}

// ─────────────────────────────────────────────
// pacs.002
// ─────────────────────────────────────────────

func TestBuildPacs002(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsACSC,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	for _, want := range []string{
		"pacs.002.001.16", "FIToFIPmtStsRpt", "TxSts", "ACSC",
		"OrgnlEndToEndId", "OrgnlTxRef", "OrgnlGrpInfAndSts",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs002GrpHdrOrder(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:   TxStsACSC,
		InstgBIC: "MOZTATWW",
		InstdBIC: "CHASUS33",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FIToFIPmtStsRpt/GrpHdr", []string{
		"MsgId", "CreDtTm", "InstgAgt", "InstdAgt", "OrgnlBizQry",
	})
}

func TestBuildPacs002TxOrder(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsRJCT,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FIToFIPmtStsRpt/TxInfAndSts", []string{
		"StsId", "OrgnlGrpInf", "OrgnlInstrId", "OrgnlEndToEndId", "OrgnlTxId",
		"OrgnlUETR", "TxSts", "StsRsnInf", "ChrgsInf", "AccptncDtTm",
		"PrcgDt", "FctvIntrBkSttlmDt", "AcctSvcrRef", "ClrSysRef",
		"CdtSttlmKey", "InstgAgt", "InstdAgt", "OrgnlTxRef", "SplmtryData",
	})
}

func TestBuildPacs002ForbiddenElements(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsRJCT,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	// GroupHeader120 has no NbOfTxs / SttlmInf / InitgPty
	grpHdrChildren := childElements(t, xmlStr, "Document/FIToFIPmtStsRpt/GrpHdr")
	for _, c := range grpHdrChildren {
		switch c {
		case "NbOfTxs", "SttlmInf", "InitgPty":
			t.Errorf("GrpHdr must not contain %s", c)
		}
	}
	// No RsnnInf wrapper inside StsRsnInf
	assertAbsent(t, xmlStr, "RsnnInf")
	// StsRsnInf children must be Orgtr?/Rsn?/AddtlInf* directly
	stsChildren := childElements(t, xmlStr, "Document/FIToFIPmtStsRpt/TxInfAndSts/StsRsnInf")
	for _, c := range stsChildren {
		switch c {
		case "Orgtr", "Rsn", "AddtlInf":
		default:
			t.Errorf("unexpected StsRsnInf child <%s>", c)
		}
	}
}

func TestBuildPacs002OrgnlTxRefOrder(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:       TxStsACSC,
		DbtrBIC:      "DEUTDEFF",
		CdtrBIC:      "BNPAFRPP",
		DbtrAcctIBAN: "AT123456789012345678",
		CdtrAcctIBAN: "DE987654321098765432",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FIToFIPmtStsRpt/TxInfAndSts/OrgnlTxRef", []string{
		"IntrBkSttlmAmt", "Amt", "IntrBkSttlmDt", "ReqdColltnDt", "ReqdExctnDt",
		"CdtrSchmeId", "SttlmInf", "PmtTpInf", "PmtMtd", "MndtRltdInf", "RmtInf",
		"UltmtDbtr", "Dbtr", "DbtrAcct", "DbtrAgt", "DbtrAgtAcct",
		"CdtrAgt", "CdtrAgtAcct", "Cdtr", "CdtrAcct", "UltmtCdtr", "Purp",
	})
}

func TestBuildPacs002RoundTrip(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsRJCT,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}

	var doc Pacs002Document
	if err := UnmarshalXML([]byte(xmlStr), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}
	if doc.Xmlns != NSPacs002 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs002)
	}
	if len(doc.FIToFIPmtStsRpt.TxInfAndSts) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.FIToFIPmtStsRpt.TxInfAndSts))
	}
	tx := doc.FIToFIPmtStsRpt.TxInfAndSts[0]
	if tx.TxSts != "RJCT" {
		t.Errorf("status mismatch: got %s, want RJCT", tx.TxSts)
	}
	if len(tx.StsRsnInf) != 1 || tx.StsRsnInf[0].Rsn == nil || tx.StsRsnInf[0].Rsn.Cd != "AC01" {
		t.Error("StsRsnInf/Rsn/Cd mismatch after round-trip")
	}
}

func TestXSDValidationPacs002(t *testing.T) {
	xmlStr, err := BuildPacs002(demoPayment(), &Pacs002Options{Status: TxStsACSC})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.002.001.16.xsd")
}

// ─────────────────────────────────────────────
// pacs.004
// ─────────────────────────────────────────────

func TestBuildPacs004(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	for _, want := range []string{
		"pacs.004.001.15", "PmtRtr", "RtrId", "OrgnlEndToEndId",
		"OrgnlGrpInf", "RtrChain", "FRAD",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs004RootElement(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<PmtRtr>") {
		t.Error("root element must be PmtRtr")
	}
	assertAbsent(t, xmlStr, "PmtRvsl")
}

func TestBuildPacs004GrpHdrOrder(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{
		InstgBIC: "MOZTATWW",
		InstdBIC: "CHASUS33",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/PmtRtr/GrpHdr", []string{
		"MsgId", "CreDtTm", "Authstn", "BtchBookg", "NbOfTxs", "CtrlSum",
		"GrpRtr", "TtlRtrdIntrBkSttlmAmt", "IntrBkSttlmDt", "SttlmInf",
		"PmtTpInf", "InstgAgt", "InstdAgt",
	})
}

func TestBuildPacs004TxOrder(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/PmtRtr/TxInf", []string{
		"RtrId", "OrgnlGrpInf", "OrgnlInstrId", "OrgnlEndToEndId", "OrgnlTxId",
		"OrgnlUETR", "OrgnlClrSysRef", "OrgnlIntrBkSttlmAmt", "OrgnlIntrBkSttlmDt",
		"PmtTpInf", "RtrdIntrBkSttlmAmt", "IntrBkSttlmDt", "SttlmPrty",
		"SttlmTmIndctn", "SttlmTmReq", "RtrdInstdAmt", "XchgRate", "AgrdRate",
		"CompstnAmt", "ChrgBr", "ChrgsInf", "ClrSysRef", "InstgAgt", "InstdAgt",
		"RtrChain", "RtrRsnInf", "OrgnlTxRef", "SplmtryData",
	})
}

func TestBuildPacs004ForbiddenElements(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	assertAbsent(t, xmlStr, "SttlmDt")
	assertAbsent(t, xmlStr, "RvslId")
	assertAbsent(t, xmlStr, "RvslRsnInf")
	assertAbsent(t, xmlStr, "RvslIntrBkSttlmAmt")
	grpHdrChildren := childElements(t, xmlStr, "Document/PmtRtr/GrpHdr")
	for _, c := range grpHdrChildren {
		if c == "InitgPty" {
			t.Error("GrpHdr must not contain InitgPty")
		}
	}
}

func TestBuildPacs004RoundTrip(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}

	var doc Pacs004Document
	if err := UnmarshalXML([]byte(xmlStr), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}
	if doc.Xmlns != NSPacs004 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs004)
	}
	if len(doc.PmtRtr.TxInf) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.PmtRtr.TxInf))
	}
	tx := doc.PmtRtr.TxInf[0]
	if tx.RtrId == "" {
		t.Error("RtrId empty after round-trip")
	}
	if tx.RtrChain == nil || tx.RtrChain.Dbtr == nil || tx.RtrChain.Cdtr == nil {
		t.Error("RtrChain Dbtr/Cdtr missing after round-trip")
	}
	if len(tx.RtrRsnInf) != 1 || tx.RtrRsnInf[0].Rsn == nil || tx.RtrRsnInf[0].Rsn.Cd != "FRAD" {
		t.Error("RtrRsnInf/Rsn/Cd mismatch after round-trip")
	}
}

func TestXSDValidationPacs004(t *testing.T) {
	xmlStr, err := BuildPacs004(demoPayment(), &Pacs004Options{ReversalReasonCode: "FRAD"})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.004.001.15.xsd")
}

// ─────────────────────────────────────────────
// pacs.009
// ─────────────────────────────────────────────

func TestBuildPacs009(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	for _, want := range []string{
		"pacs.009.001.13", "FICdtTrf", "CdtTrfTxInf", "EndToEndId",
		"IntrBkSttlmAmt", "Dbtr", "Cdtr",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("XML does not contain %s", want)
		}
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs009RootElement(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	if !strings.Contains(xmlStr, "<FICdtTrf>") {
		t.Error("root element must be FICdtTrf")
	}
	assertAbsent(t, xmlStr, "FIToFICdtTrf")
}

func TestBuildPacs009TxOrder(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), &Pacs009Options{
		DbtrBIC:      "DEUTDEFF",
		CdtrBIC:      "BNPAFRPP",
		DbtrAgtBIC:   "MOZTATWW",
		CdtrAgtBIC:   "CHASUS33",
		DbtrAcctIBAN: "AT123456789012345678",
		CdtrAcctIBAN: "DE987654321098765432",
		PurposeCode:  "GDS",
	})
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/FICdtTrf/CdtTrfTxInf", []string{
		"PmtId", "PmtTpInf", "IntrBkSttlmAmt", "IntrBkSttlmDt", "SttlmPrty",
		"SttlmTmIndctn", "SttlmTmReq", "XpryDtTm", "PmtSgntr",
		"PrvsInstgAgt1", "PrvsInstgAgt1Acct", "PrvsInstgAgt2", "PrvsInstgAgt2Acct",
		"PrvsInstgAgt3", "PrvsInstgAgt3Acct",
		"InstgAgt", "InstdAgt",
		"IntrmyAgt1", "IntrmyAgt1Acct", "IntrmyAgt2", "IntrmyAgt2Acct",
		"IntrmyAgt3", "IntrmyAgt3Acct",
		"UltmtDbtr", "Dbtr", "DbtrAcct", "DbtrAgt", "DbtrAgtAcct",
		"CdtrAgt", "CdtrAgtAcct", "Cdtr", "CdtrAcct", "UltmtCdtr",
		"InstrForCdtrAgt", "InstrForNxtAgt",
		"Purp", "RgltryRptg", "RmtInf",
		"UndrlygAllcn", "UndrlygCstmrCdtTrf", "UndrlygFICdtTrf", "SplmtryData",
	})
}

func TestBuildPacs009FIParties(t *testing.T) {
	// Dbtr/Cdtr must be financial institutions (FinInstnId), not parties
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	dbtrChildren := childElements(t, xmlStr, "Document/FICdtTrf/CdtTrfTxInf/Dbtr")
	if len(dbtrChildren) != 1 || dbtrChildren[0] != "FinInstnId" {
		t.Errorf("Dbtr must contain FinInstnId, got %v", dbtrChildren)
	}
	cdtrChildren := childElements(t, xmlStr, "Document/FICdtTrf/CdtTrfTxInf/Cdtr")
	if len(cdtrChildren) != 1 || cdtrChildren[0] != "FinInstnId" {
		t.Errorf("Cdtr must contain FinInstnId, got %v", cdtrChildren)
	}
}

func TestBuildPacs009ForbiddenElements(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	assertAbsent(t, xmlStr, "SttlmDt")
	// ChrgBr is not part of pacs.009.001.13 CdtTrfTxInf
	assertAbsent(t, xmlStr, "ChrgBr")
	grpHdrChildren := childElements(t, xmlStr, "Document/FICdtTrf/GrpHdr")
	for _, c := range grpHdrChildren {
		if c == "InitgPty" {
			t.Error("GrpHdr must not contain InitgPty")
		}
	}
}

func TestBuildPacs009RoundTrip(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}

	var doc Pacs009Document
	if err := UnmarshalXML([]byte(xmlStr), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}
	if doc.Xmlns != NSPacs009 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs009)
	}
	if len(doc.FICdtTrf.CdtTrfTxInf) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.FICdtTrf.CdtTrfTxInf))
	}
	tx := doc.FICdtTrf.CdtTrfTxInf[0]
	if tx.PmtId == nil || tx.PmtId.EndToEndId == "" {
		t.Error("EndToEndId is empty after round-trip")
	}
	if tx.Dbtr == nil || tx.Dbtr.FinInstnId == nil {
		t.Error("Dbtr/FinInstnId missing after round-trip")
	}
	if tx.Cdtr == nil || tx.Cdtr.FinInstnId == nil {
		t.Error("Cdtr/FinInstnId missing after round-trip")
	}
}

func TestXSDValidationPacs009(t *testing.T) {
	xmlStr, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	validateWithXSD(t, xmlStr, "pacs.009.001.13.xsd")
}

// ─────────────────────────────────────────────
// Shared
// ─────────────────────────────────────────────

func TestValidateXML(t *testing.T) {
	if err := ValidateXML([]byte(`<root><child>text</child></root>`)); err != nil {
		t.Errorf("valid XML failed validation: %v", err)
	}
	if err := ValidateXML([]byte(`<root><unclosed>`)); err == nil {
		t.Error("invalid XML passed validation")
	}
}

func TestGenerateUUIDv4Format(t *testing.T) {
	u := generateUUIDv4()
	if len(u) != 36 {
		t.Fatalf("UUID length %d, want 36", len(u))
	}
	if u[14] != '4' {
		t.Errorf("UUID version nibble = %c, want 4", u[14])
	}
	if u[19] != '8' && u[19] != '9' && u[19] != 'a' && u[19] != 'b' {
		t.Errorf("UUID variant nibble = %c, want 8/9/a/b", u[19])
	}
	if generateUUIDv4() == u {
		t.Error("consecutive UUIDs identical — entropy failure")
	}
}

func TestBuildPacs008NilPayment(t *testing.T) {
	if _, err := BuildPacs008(nil, nil); err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs002NilPayment(t *testing.T) {
	if _, err := BuildPacs002(nil, nil); err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs004NilPayment(t *testing.T) {
	if _, err := BuildPacs004(nil, nil); err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs009NilPayment(t *testing.T) {
	if _, err := BuildPacs009(nil, nil); err == nil {
		t.Error("expected error for nil payment")
	}
}
