package iso20022

import (
	"fmt"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────
// DetectMessageType
// ─────────────────────────────────────────────

func TestDetectMessageType(t *testing.T) {
	p := demoPayment()
	instrs := []*CreditTransferInstruction{demoInstruction(0)}

	pacs008, err := BuildPacs008Batch(instrs, nil)
	if err != nil {
		t.Fatalf("BuildPacs008Batch: %v", err)
	}
	pacs002, err := BuildPacs002Batch(instrs, nil)
	if err != nil {
		t.Fatalf("BuildPacs002Batch: %v", err)
	}
	pain001, err := BuildPain001(instrs, &Pain001Options{
		Debtor: &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
	})
	if err != nil {
		t.Fatalf("BuildPain001: %v", err)
	}
	pain002, err := BuildPain002Batch(instrs, nil)
	if err != nil {
		t.Fatalf("BuildPain002Batch: %v", err)
	}
	camt054, err := BuildCamt054(instrs, nil)
	if err != nil {
		t.Fatalf("BuildCamt054: %v", err)
	}
	camt053, err := BuildCamt053(instrs, nil)
	if err != nil {
		t.Fatalf("BuildCamt053: %v", err)
	}
	pacs004, err := BuildPacs004(p, nil)
	if err != nil {
		t.Fatalf("BuildPacs004: %v", err)
	}
	pacs009, err := BuildPacs009(p, nil)
	if err != nil {
		t.Fatalf("BuildPacs009: %v", err)
	}

	cases := []struct {
		xml  string
		want string
	}{
		{pacs008, MsgPacs008},
		{pacs002, MsgPacs002},
		{pain001, MsgPain001},
		{pain002, MsgPain002},
		{camt053, MsgCamt053},
		{camt054, MsgCamt054},
		{pacs004, MsgPacs004},
		{pacs009, MsgPacs009},
	}
	for i, c := range cases {
		got, err := DetectMessageType([]byte(c.xml))
		if err != nil {
			t.Fatalf("case %d: DetectMessageType: %v", i, err)
		}
		if got != c.want {
			t.Errorf("case %d: got %q, want %q", i, got, c.want)
		}
	}
}

func TestDetectMessageTypeErrors(t *testing.T) {
	if _, err := DetectMessageType([]byte("not xml")); err == nil {
		t.Error("expected error for non-XML")
	}
	if _, err := DetectMessageType([]byte(`<Foo xmlns="x"/>`)); err == nil {
		t.Error("expected error for non-Document root")
	}
	if _, err := DetectMessageType([]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:made.up.001.99"/>`)); err == nil {
		t.Error("expected error for unknown namespace")
	}
}

// ─────────────────────────────────────────────
// Typed parsers + StatusReports
// ─────────────────────────────────────────────

func TestParsePacs002StatusReports(t *testing.T) {
	i1 := demoInstruction(0)
	i2 := demoInstruction(1)
	i2.TxStatus = TxStsRJCT
	i2.TxReason = "AC01"

	xmlStr, err := BuildPacs002Batch([]*CreditTransferInstruction{i1, i2}, &Pacs002Options{
		OrgnlMsgId: "ORIG-MSG-42",
	})
	if err != nil {
		t.Fatalf("BuildPacs002Batch: %v", err)
	}

	doc, err := ParsePacs002([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParsePacs002: %v", err)
	}

	reports := doc.StatusReports()
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if reports[0].Status != "ACSC" {
		t.Errorf("report 0 status: got %q, want ACSC", reports[0].Status)
	}
	if reports[1].Status != "RJCT" || reports[1].ReasonCode != "AC01" {
		t.Errorf("report 1: got %s/%s, want RJCT/AC01", reports[1].Status, reports[1].ReasonCode)
	}
	if reports[0].OriginalMsgID != "ORIG-MSG-42" {
		t.Errorf("OriginalMsgID: got %q, want ORIG-MSG-42", reports[0].OriginalMsgID)
	}
	if reports[0].EndToEndID == "" {
		t.Error("EndToEndID empty — reconciliation needs it")
	}
}

func TestParsePain002RoundTrip(t *testing.T) {
	i1 := demoInstruction(0)
	i2 := demoInstruction(1)
	i2.TxStatus = TxStsRJCT
	i2.TxReason = "AC01"

	xmlStr, err := BuildPain002Batch([]*CreditTransferInstruction{i1, i2}, &Pain002Options{
		OrgnlMsgId:    "SGC1P-AB12",
		OrgnlPmtInfId: "SGC1P-AB12-1",
		OrgnlInstrId:  "INSTR-77",
	})
	if err != nil {
		t.Fatalf("BuildPain002Batch: %v", err)
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Fatalf("pain.002 not well-formed: %v", err)
	}
	for _, want := range []string{
		"pain.002.001.10", "CstmrPmtStsRpt", "OrgnlGrpInfAndSts",
		"OrgnlPmtInfAndSts", "TxInfAndSts", "<TxSts>ACSC</TxSts>",
		"<TxSts>RJCT</TxSts>", "<Cd>AC01</Cd>", "SGC1P-AB12",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("pain.002 does not contain %s", want)
		}
	}

	doc, err := ParsePain002([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParsePain002: %v", err)
	}
	reports := doc.StatusReports()
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if reports[1].Status != "RJCT" || reports[1].ReasonCode != "AC01" {
		t.Errorf("report 1: got %s/%s, want RJCT/AC01", reports[1].Status, reports[1].ReasonCode)
	}
	if reports[0].OriginalMsgID != "SGC1P-AB12" {
		t.Errorf("OriginalMsgID: got %q", reports[0].OriginalMsgID)
	}
	if reports[0].InstrID != "INSTR-77" {
		t.Errorf("InstrID: got %q, want INSTR-77", reports[0].InstrID)
	}
}

func TestBuildPain002GroupStatus(t *testing.T) {
	// Uniform status → GrpSts = that status; mixed → PART.
	xmlStr, err := BuildPain002Batch([]*CreditTransferInstruction{demoInstruction(0)}, nil)
	if err != nil {
		t.Fatalf("BuildPain002Batch: %v", err)
	}
	if !strings.Contains(xmlStr, "<GrpSts>ACSC</GrpSts>") {
		t.Error("expected GrpSts ACSC for uniform batch")
	}

	i2 := demoInstruction(1)
	i2.TxStatus = TxStsPDNG
	xmlStr, err = BuildPain002Batch([]*CreditTransferInstruction{demoInstruction(0), i2}, nil)
	if err != nil {
		t.Fatalf("BuildPain002Batch: %v", err)
	}
	if !strings.Contains(xmlStr, "<GrpSts>PART</GrpSts>") {
		t.Error("expected GrpSts PART for mixed batch")
	}
}

func TestParseWrongMessageType(t *testing.T) {
	xmlStr, err := BuildPacs008Batch([]*CreditTransferInstruction{demoInstruction(0)}, nil)
	if err != nil {
		t.Fatalf("BuildPacs008Batch: %v", err)
	}
	// Struct tags bind by local name, so unmarshaling succeeds — the
	// namespace check is what must reject it.
	if _, err := ParsePacs002([]byte(xmlStr)); err == nil {
		t.Error("expected namespace mismatch error")
	}
}

// ─────────────────────────────────────────────
// camt.053
// ─────────────────────────────────────────────

func TestBuildCamt053(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1)}
	xmlStr, err := BuildCamt053(instrs, &Camt053Options{
		Account:       &Party{Name: "Relief Org", AcctID: "GDEBTOR_ACCT0000000000000000000000000000000000000000"},
		ServicerBIC:   "DEUTDEFF",
		ElectronicSeq: "42",
		PeriodFrom:    "2026-03-01T00:00:00Z",
		PeriodTo:      "2026-03-31T23:59:59Z",
		OriginalMsgID: "SGC1P-ORIG",
		PmtInfID:      "SGC1P-ORIG-1",
		Balances: []StatementBalance{
			{TypeCode: "OPBD", Amount: "1000.00", Ccy: "USD", CdtDbtInd: "CRDT", Date: "2026-03-01"},
			{TypeCode: "CLBD", Amount: "900.00", Ccy: "USD", CdtDbtInd: "CRDT", Date: "2026-03-31"},
		},
	})
	if err != nil {
		t.Fatalf("BuildCamt053: %v", err)
	}
	for _, want := range []string{
		"camt.053.001.13", "BkToCstmrStmt", "<Stmt>", "<Ntry>",
		"<ElctrncSeqNb>42</ElctrncSeqNb>", "<FrToDt>", "<Bal>",
		"<Cd>OPBD</Cd>", "<Cd>CLBD</Cd>", "SGC1P-ORIG",
		"<CdtDbtInd>DBIT</CdtDbtInd>",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("camt.053 does not contain %s", want)
		}
	}
	if got := strings.Count(xmlStr, "<Ntry>"); got != 2 {
		t.Errorf("expected 2 Ntry, got %d", got)
	}
	if err := ValidateXML([]byte(xmlStr)); err != nil {
		t.Errorf("camt.053 not well-formed: %v", err)
	}
}

func TestBuildCamt053Order(t *testing.T) {
	xmlStr, err := BuildCamt053([]*CreditTransferInstruction{demoInstruction(0)}, &Camt053Options{
		Balances: []StatementBalance{
			{TypeCode: "CLBD", Amount: "1", Ccy: "USD", CdtDbtInd: "CRDT", Date: "2026-03-31"},
		},
	})
	if err != nil {
		t.Fatalf("BuildCamt053: %v", err)
	}
	assertChildOrder(t, xmlStr, "Document/BkToCstmrStmt/Stmt", []string{
		"Id", "StmtPgntn", "ElctrncSeqNb", "RptgSeq", "LglSeqNb", "CreDtTm",
		"FrToDt", "CpyDplctInd", "Dplct", "RptgSrc", "Acct", "RltdAcct",
		"Intrst", "Bal", "TxsSummry", "Ntry", "AddtlStmtInf",
	})
	assertChildOrder(t, xmlStr, "Document/BkToCstmrStmt/Stmt/Bal", []string{
		"Tp", "CdtLine", "Amt", "CdtDbtInd", "Dt", "Avlbty",
	})
}

func TestParseCamt053RoundTrip(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1)}
	xmlStr, err := BuildCamt053(instrs, &Camt053Options{
		Account:       &Party{AcctID: "GDEBTOR_ACCT0000000000000000000000000000000000000000"},
		ElectronicSeq: "7",
		Balances: []StatementBalance{
			{TypeCode: "OPBD", Amount: "500.00", Ccy: "USD", CdtDbtInd: "CRDT", Date: "2026-03-01"},
		},
	})
	if err != nil {
		t.Fatalf("BuildCamt053: %v", err)
	}
	doc, err := ParseCamt053([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(doc.BkToCstmrStmt.Stmt) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(doc.BkToCstmrStmt.Stmt))
	}
	stmt := doc.BkToCstmrStmt.Stmt[0]
	if stmt.ElctrncSeqNb != "7" {
		t.Errorf("ElctrncSeqNb: got %q", stmt.ElctrncSeqNb)
	}
	if len(stmt.Bal) != 1 || stmt.Bal[0].Tp.CdOrPrtry.Cd != "OPBD" {
		t.Errorf("balances not parsed: %+v", stmt.Bal)
	}
	if len(stmt.Ntry) != 2 {
		t.Errorf("expected 2 entries, got %d", len(stmt.Ntry))
	}
	if stmt.Acct == nil {
		t.Error("Acct not parsed")
	}
}

func TestParseCamt054RoundTrip(t *testing.T) {
	xmlStr, err := BuildCamt054([]*CreditTransferInstruction{demoInstruction(0)}, &Camt054Options{
		Account: &Party{AcctID: "GDEBTOR_ACCT0000000000000000000000000000000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildCamt054: %v", err)
	}
	doc, err := ParseCamt054([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParseCamt054: %v", err)
	}
	if len(doc.BkToCstmrDbtCdtNtfctn.Ntfctn) == 0 {
		t.Error("expected notifications")
	}
}

// ─────────────────────────────────────────────
// Other parser smoke tests
// ─────────────────────────────────────────────

func TestParseRoundTrips(t *testing.T) {
	p := demoPayment()
	instrs := []*CreditTransferInstruction{demoInstruction(0)}

	pacs008, _ := BuildPacs008Batch(instrs, nil)
	if _, err := ParsePacs008([]byte(pacs008)); err != nil {
		t.Errorf("ParsePacs008: %v", err)
	}
	pacs004, _ := BuildPacs004(p, nil)
	if _, err := ParsePacs004([]byte(pacs004)); err != nil {
		t.Errorf("ParsePacs004: %v", err)
	}
	pacs009, _ := BuildPacs009(p, nil)
	if _, err := ParsePacs009([]byte(pacs009)); err != nil {
		t.Errorf("ParsePacs009: %v", err)
	}
	pain001, _ := BuildPain001(instrs, &Pain001Options{
		Debtor: &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
	})
	if _, err := ParsePain001([]byte(pain001)); err != nil {
		t.Errorf("ParsePain001: %v", err)
	}
}

// ─────────────────────────────────────────────
// Reconciliation — the full disbursement loop
// ─────────────────────────────────────────────

func TestReconcileInstructions(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1), demoInstruction(2)}
	// Distinct tx hashes → distinct EndToEndIds. (Payments sharing one
	// Stellar tx share the hash and can't be told apart by a status report.)
	for i, instr := range instrs {
		instr.Payment.TxHash = fmt.Sprintf("TXHASH%04d", i)
	}

	// Bank replies: instr 0 ACSC, instr 1 RJCT, instr 2 has no report; plus a
	// report for a payment outside this batch.
	instrs[1].TxStatus = TxStsRJCT
	instrs[1].TxReason = "AC01"
	xmlStr, err := BuildPain002Batch(instrs[:2], &Pain002Options{
		OrgnlMsgId: "SGC1P-BATCH",
	})
	if err != nil {
		t.Fatalf("BuildPain002Batch: %v", err)
	}
	doc, err := ParsePain002([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParsePain002: %v", err)
	}
	reports := doc.StatusReports()
	reports = append(reports, StatusReport{EndToEndID: "OTHER-PAYMENT-9", Status: "ACSC"})

	sum := ReconcileInstructions(instrs, reports)

	if len(sum.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(sum.Results))
	}
	if !sum.Results[0].Matched || sum.Results[0].Status != "ACSC" {
		t.Errorf("result 0: %+v — want matched ACSC", sum.Results[0])
	}
	if !sum.Results[1].Matched || sum.Results[1].Status != "RJCT" || sum.Results[1].ReasonCode != "AC01" {
		t.Errorf("result 1: %+v — want matched RJCT/AC01", sum.Results[1])
	}
	if sum.Results[2].Matched {
		t.Errorf("result 2 should be unmatched (no report)")
	}
	if len(sum.Unmatched) != 1 || sum.Unmatched[0].EndToEndID != "OTHER-PAYMENT-9" {
		t.Errorf("Unmatched: %+v — want the out-of-batch report", sum.Unmatched)
	}
}

func TestReconcileLastReportWins(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0)}
	p := instrs[0].Payment
	e2e := endToEndID(p)

	// PDNG then ACSC for the same payment — the later status wins.
	sum := ReconcileInstructions(instrs, []StatusReport{
		{EndToEndID: e2e, Status: "PDNG"},
		{EndToEndID: e2e, Status: "ACSC"},
	})
	if sum.Results[0].Status != "ACSC" {
		t.Errorf("Status: got %q, want ACSC (last wins)", sum.Results[0].Status)
	}
	if len(sum.Unmatched) != 0 {
		t.Errorf("Unmatched: got %d, want 0", len(sum.Unmatched))
	}
}

func TestReconcileByInstrAndTxID(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0)}
	p := instrs[0].Payment

	// Reports keyed by InstrId or TxId (both = payment ID) must also match.
	sum := ReconcileInstructions(instrs, []StatusReport{
		{InstrID: safeTruncate(p.ID, 35), Status: "PDNG"},
	})
	if !sum.Results[0].Matched {
		t.Error("InstrID match failed")
	}
	sum = ReconcileInstructions(instrs, []StatusReport{
		{TxID: p.ID, Status: "ACSC"},
	})
	if !sum.Results[0].Matched {
		t.Error("TxID match failed")
	}
}

// Sanity check that Stellar refs survive a pacs.002 round-trip: the tx hash
// maps to OrgnlEndToEndId (truncated to 16 chars by endToEndID).
func TestPacs002PreservesStellarRefs(t *testing.T) {
	p := demoPayment()
	p.TxHash = "STELLARTXHASH0000000000000000000000000000000000000000000000AB"
	instr := &CreditTransferInstruction{Payment: p}
	xmlStr, err := BuildPacs002Batch([]*CreditTransferInstruction{instr}, nil)
	if err != nil {
		t.Fatalf("BuildPacs002Batch: %v", err)
	}
	doc, err := ParsePacs002([]byte(xmlStr))
	if err != nil {
		t.Fatalf("ParsePacs002: %v", err)
	}
	reports := doc.StatusReports()
	if len(reports) == 0 {
		t.Fatal("no StatusReports parsed")
	}
	if reports[0].EndToEndID != "STELLARTXHASH000" {
		t.Errorf("EndToEndID: got %q, want truncated tx hash", reports[0].EndToEndID)
	}
}
