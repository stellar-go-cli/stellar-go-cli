package iso20022

import (
	"strings"
	"testing"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
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

func TestBuildPacs008(t *testing.T) {
	xml, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}
	if !strings.Contains(xml, "pacs.008.001.08") {
		t.Error("XML does not contain pacs.008 namespace")
	}
	if !strings.Contains(xml, "FIToFICstmrCdtTrf") {
		t.Error("XML does not contain FIToFICstmrCdtTrf root element")
	}
	if !strings.Contains(xml, "GrpHdr") {
		t.Error("XML does not contain GrpHdr")
	}
	if !strings.Contains(xml, "CdtTrfTxInf") {
		t.Error("XML does not contain CdtTrfTxInf")
	}
	if !strings.Contains(xml, "EndToEndId") {
		t.Error("XML does not contain EndToEndId")
	}
	if !strings.Contains(xml, "IntrBkSttlmAmt") {
		t.Error("XML does not contain IntrBkSttlmAmt")
	}
	if !strings.Contains(xml, "Dbtr") {
		t.Error("XML does not contain Dbtr")
	}
	if !strings.Contains(xml, "Cdtr") {
		t.Error("XML does not contain Cdtr")
	}
	if !strings.Contains(xml, "ChrgBr") {
		t.Error("XML does not contain ChrgBr")
	}
	if !strings.Contains(xml, "UETR") {
		t.Error("XML does not contain UETR")
	}
	if !strings.Contains(xml, "RmtInf") {
		t.Error("XML does not contain RmtInf (remittance info from memo)")
	}
	if !strings.Contains(xml, "USDC") {
		t.Error("XML does not contain currency USDC")
	}
	if err := ValidateXML([]byte(xml)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs008RoundTrip(t *testing.T) {
	xml, err := BuildPacs008(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs008 failed: %v", err)
	}

	var doc Pacs008Document
	if err := UnmarshalXML([]byte(xml), &doc); err != nil {
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
	if tx.IntrBkSttlmAmt == nil {
		t.Fatal("IntrBkSttlmAmt is nil")
	}
	if tx.IntrBkSttlmAmt.Ccy != "USDC" {
		t.Errorf("currency mismatch: got %s, want USDC", tx.IntrBkSttlmAmt.Ccy)
	}
	if tx.IntrBkSttlmAmt.Value != "100.0000000" {
		t.Errorf("amount mismatch: got %s, want 100.0000000", tx.IntrBkSttlmAmt.Value)
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

	xml, err := BuildPacs008(demoPayment(), opts)
	if err != nil {
		t.Fatalf("BuildPacs008 with options failed: %v", err)
	}
	if !strings.Contains(xml, "MOZTATWW") {
		t.Error("XML does not contain instg BIC")
	}
	if !strings.Contains(xml, "CHASUS33") {
		t.Error("XML does not contain instd BIC")
	}
	if !strings.Contains(xml, "Alice Corp") {
		t.Error("XML does not contain debtor name")
	}
	if !strings.Contains(xml, "Bob Inc") {
		t.Error("XML does not contain creditor name")
	}
	if !strings.Contains(xml, "AT123456789012345678") {
		t.Error("XML does not contain debtor IBAN")
	}
	if !strings.Contains(xml, "DE987654321098765432") {
		t.Error("XML does not contain creditor IBAN")
	}
	if !strings.Contains(xml, "GDS") {
		t.Error("XML does not contain purpose code")
	}
	if !strings.Contains(xml, "Invoice 12345") {
		t.Error("XML does not contain remittance info")
	}
}

func TestBuildPacs002(t *testing.T) {
	xml, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsACSC,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}
	if !strings.Contains(xml, "pacs.002.001.12") {
		t.Error("XML does not contain pacs.002 namespace")
	}
	if !strings.Contains(xml, "FIToFIPmtStsRpt") {
		t.Error("XML does not contain FIToFIPmtStsRpt root element")
	}
	if !strings.Contains(xml, "TxSts") {
		t.Error("XML does not contain TxSts")
	}
	if !strings.Contains(xml, "ACSC") {
		t.Error("XML does not contain ACSC status")
	}
	if !strings.Contains(xml, "OrgnlEndToEndId") {
		t.Error("XML does not contain OrgnlEndToEndId")
	}
	if !strings.Contains(xml, "OrgnlTxRef") {
		t.Error("XML does not contain OrgnlTxRef")
	}
	if err := ValidateXML([]byte(xml)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs002RoundTrip(t *testing.T) {
	xml, err := BuildPacs002(demoPayment(), &Pacs002Options{
		Status:     TxStsRJCT,
		ReasonCode: "AC01",
	})
	if err != nil {
		t.Fatalf("BuildPacs002 failed: %v", err)
	}

	var doc Pacs002Document
	if err := UnmarshalXML([]byte(xml), &doc); err != nil {
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
}

func TestBuildPacs004(t *testing.T) {
	xml, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}
	if !strings.Contains(xml, "pacs.004.001.12") {
		t.Error("XML does not contain pacs.004 namespace")
	}
	if !strings.Contains(xml, "PmtRvsl") {
		t.Error("XML does not contain PmtRvsl root element")
	}
	if !strings.Contains(xml, "RvslId") {
		t.Error("XML does not contain RvslId")
	}
	if !strings.Contains(xml, "OrgnlEndToEndId") {
		t.Error("XML does not contain OrgnlEndToEndId")
	}
	if !strings.Contains(xml, "FRAD") {
		t.Error("XML does not contain reversal reason code FRAD")
	}
	if err := ValidateXML([]byte(xml)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs004RoundTrip(t *testing.T) {
	xml, err := BuildPacs004(demoPayment(), &Pacs004Options{
		ReversalReasonCode: "FRAD",
	})
	if err != nil {
		t.Fatalf("BuildPacs004 failed: %v", err)
	}

	var doc Pacs004Document
	if err := UnmarshalXML([]byte(xml), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}
	if doc.Xmlns != NSPacs004 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs004)
	}
	if len(doc.PmtRvsl.TxInf) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.PmtRvsl.TxInf))
	}
}

func TestBuildPacs009(t *testing.T) {
	xml, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}
	if !strings.Contains(xml, "pacs.009.001.10") {
		t.Error("XML does not contain pacs.009 namespace")
	}
	if !strings.Contains(xml, "FIToFICdtTrf") {
		t.Error("XML does not contain FIToFICdtTrf root element")
	}
	if !strings.Contains(xml, "CdtTrfTxInf") {
		t.Error("XML does not contain CdtTrfTxInf")
	}
	if !strings.Contains(xml, "EndToEndId") {
		t.Error("XML does not contain EndToEndId")
	}
	if !strings.Contains(xml, "IntrBkSttlmAmt") {
		t.Error("XML does not contain IntrBkSttlmAmt")
	}
	if !strings.Contains(xml, "DEBT") {
		t.Error("XML does not contain default DEBT charge bearer")
	}
	if err := ValidateXML([]byte(xml)); err != nil {
		t.Errorf("XML is not well-formed: %v", err)
	}
}

func TestBuildPacs009RoundTrip(t *testing.T) {
	xml, err := BuildPacs009(demoPayment(), nil)
	if err != nil {
		t.Fatalf("BuildPacs009 failed: %v", err)
	}

	var doc Pacs009Document
	if err := UnmarshalXML([]byte(xml), &doc); err != nil {
		t.Fatalf("UnmarshalXML failed: %v", err)
	}
	if doc.Xmlns != NSPacs009 {
		t.Errorf("namespace mismatch: got %s, want %s", doc.Xmlns, NSPacs009)
	}
	if len(doc.FIToFICdtTrf.CdtTrfTxInf) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(doc.FIToFICdtTrf.CdtTrfTxInf))
	}
	tx := doc.FIToFICdtTrf.CdtTrfTxInf[0]
	if tx.PmtId == nil || tx.PmtId.EndToEndId == "" {
		t.Error("EndToEndId is empty after round-trip")
	}
}

func TestValidateXML(t *testing.T) {
	if err := ValidateXML([]byte(`<root><child>text</child></root>`)); err != nil {
		t.Errorf("valid XML failed validation: %v", err)
	}
	if err := ValidateXML([]byte(`<root><unclosed>`)); err == nil {
		t.Error("invalid XML passed validation")
	}
}

func TestBuildPacs008NilPayment(t *testing.T) {
	_, err := BuildPacs008(nil, nil)
	if err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs002NilPayment(t *testing.T) {
	_, err := BuildPacs002(nil, nil)
	if err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs004NilPayment(t *testing.T) {
	_, err := BuildPacs004(nil, nil)
	if err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestBuildPacs009NilPayment(t *testing.T) {
	_, err := BuildPacs009(nil, nil)
	if err == nil {
		t.Error("expected error for nil payment")
	}
}
