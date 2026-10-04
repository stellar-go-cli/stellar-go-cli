package iso20022

import (
	"bytes"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────
// ComputeBatchTotals
// ─────────────────────────────────────────────

func TestComputeBatchTotals(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1), demoInstruction(2)}
	totals, err := ComputeBatchTotals(instrs)
	if err != nil {
		t.Fatalf("ComputeBatchTotals: %v", err)
	}
	if totals.NbOfTxs != 3 {
		t.Errorf("NbOfTxs: got %d, want 3", totals.NbOfTxs)
	}
	if totals.CtrlSum != "35.75" {
		t.Errorf("CtrlSum: got %q, want 35.75", totals.CtrlSum)
	}
	if totals.SingleCcy != "XXX" {
		t.Errorf("SingleCcy: got %q, want XXX", totals.SingleCcy)
	}
}

func TestComputeBatchTotalsMixedCcy(t *testing.T) {
	i2 := demoInstruction(1)
	i2.Payment.Asset = "USD"
	i2.Payment.AssetIssuer = "GISSUER000000000000000000000000000000000000000000000001"
	totals, err := ComputeBatchTotals([]*CreditTransferInstruction{demoInstruction(0), i2})
	if err != nil {
		t.Fatalf("ComputeBatchTotals: %v", err)
	}
	if totals.SingleCcy != "" {
		t.Errorf("SingleCcy should be empty for mixed currencies, got %q", totals.SingleCcy)
	}
	if totals.CtrlSum == "" {
		t.Error("CtrlSum should still be computed")
	}
}

// ─────────────────────────────────────────────
// StreamPacs008Batch
// ─────────────────────────────────────────────

func TestStreamPacs008Batch(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1), demoInstruction(2)}
	var buf bytes.Buffer
	if err := StreamPacs008Batch(&buf, instrs, &Pacs008BatchOptions{MsgID: "SGC1STREAM"}); err != nil {
		t.Fatalf("StreamPacs008Batch: %v", err)
	}
	xmlStr := buf.String()

	for _, want := range []string{
		"pacs.008.001.14", "FIToFICstmrCdtTrf", "SGC1STREAM",
		"<NbOfTxs>3</NbOfTxs>", "<CtrlSum>35.75</CtrlSum>",
		"TtlIntrBkSttlmAmt", "</Document>",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("streamed pacs.008 does not contain %s", want)
		}
	}
	if got := strings.Count(xmlStr, "<CdtTrfTxInf>"); got != 3 {
		t.Errorf("expected 3 CdtTrfTxInf, got %d", got)
	}
	if err := ValidateXML(buf.Bytes()); err != nil {
		t.Errorf("streamed pacs.008 not well-formed: %v", err)
	}
	// Must parse back through the same typed parser as the batch builder.
	doc, err := ParsePacs008(buf.Bytes())
	if err != nil {
		t.Fatalf("ParsePacs008(streamed): %v", err)
	}
	if len(doc.FIToFICstmrCdtTrf.CdtTrfTxInf) != 3 {
		t.Errorf("parsed %d transactions, want 3", len(doc.FIToFICstmrCdtTrf.CdtTrfTxInf))
	}
}

// Equivalence: streamed and built documents carry the same transaction IDs.
func TestStreamPacs008MatchesBatch(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1)}
	built, err := BuildPacs008Batch(instrs, nil)
	if err != nil {
		t.Fatalf("BuildPacs008Batch: %v", err)
	}
	var buf bytes.Buffer
	if err := StreamPacs008Batch(&buf, instrs, nil); err != nil {
		t.Fatalf("StreamPacs008Batch: %v", err)
	}
	streamed := buf.String()

	builtDoc, err := ParsePacs008([]byte(built))
	if err != nil {
		t.Fatalf("parse built: %v", err)
	}
	streamedDoc, err := ParsePacs008([]byte(streamed))
	if err != nil {
		t.Fatalf("parse streamed: %v", err)
	}
	btx := builtDoc.FIToFICstmrCdtTrf.CdtTrfTxInf
	stx := streamedDoc.FIToFICstmrCdtTrf.CdtTrfTxInf
	if len(btx) != len(stx) {
		t.Fatalf("tx count differs: built %d, streamed %d", len(btx), len(stx))
	}
	for i := range btx {
		if btx[i].PmtId.EndToEndId != stx[i].PmtId.EndToEndId {
			t.Errorf("tx %d EndToEndId differs: %q vs %q", i, btx[i].PmtId.EndToEndId, stx[i].PmtId.EndToEndId)
		}
	}
}

// ─────────────────────────────────────────────
// StreamPain001Batch
// ─────────────────────────────────────────────

func TestStreamPain001Batch(t *testing.T) {
	instrs := []*CreditTransferInstruction{demoInstruction(0), demoInstruction(1), demoInstruction(2)}
	opts := &Pain001Options{
		Debtor:   &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
		MsgID:    "SGC1P-STREAM",
		PmtInfID: "SGC1P-STREAM-1",
	}
	var buf bytes.Buffer
	if err := StreamPain001Batch(&buf, instrs, opts); err != nil {
		t.Fatalf("StreamPain001Batch: %v", err)
	}
	xmlStr := buf.String()

	for _, want := range []string{
		"pain.001.001.13", "CstmrCdtTrfInitn", "SGC1P-STREAM",
		"<NbOfTxs>3</NbOfTxs>", "<CtrlSum>35.75</CtrlSum>", "</PmtInf>",
		"</Document>",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("streamed pain.001 does not contain %s", want)
		}
	}
	if got := strings.Count(xmlStr, "<CdtTrfTxInf>"); got != 3 {
		t.Errorf("expected 3 CdtTrfTxInf, got %d", got)
	}
	if err := ValidateXML(buf.Bytes()); err != nil {
		t.Errorf("streamed pain.001 not well-formed: %v", err)
	}
	doc, err := ParsePain001(buf.Bytes())
	if err != nil {
		t.Fatalf("ParsePain001(streamed): %v", err)
	}
	if len(doc.CstmrCdtTrfInitn.PmtInf) != 1 {
		t.Fatalf("expected 1 PmtInf, got %d", len(doc.CstmrCdtTrfInitn.PmtInf))
	}
	if len(doc.CstmrCdtTrfInitn.PmtInf[0].CdtTrfTxInf) != 3 {
		t.Errorf("parsed %d CdtTrfTxInf, want 3", len(doc.CstmrCdtTrfInitn.PmtInf[0].CdtTrfTxInf))
	}
}

// ─────────────────────────────────────────────
// Writer error cases
// ─────────────────────────────────────────────

func TestStreamWriterCountMismatch(t *testing.T) {
	var buf bytes.Buffer
	sw, err := NewPacs008StreamWriter(&buf, nil, BatchTotals{NbOfTxs: 2, CtrlSum: "10"})
	if err != nil {
		t.Fatalf("NewPacs008StreamWriter: %v", err)
	}
	if err := sw.WriteInstruction(demoInstruction(0)); err != nil {
		t.Fatalf("WriteInstruction: %v", err)
	}
	if err := sw.Close(); err == nil {
		t.Error("expected NbOfTxs mismatch error on Close")
	}
}

func TestStreamWriterNilInstruction(t *testing.T) {
	var buf bytes.Buffer
	sw, err := NewPain001StreamWriter(&buf, &Pain001Options{
		Debtor: &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
	}, BatchTotals{NbOfTxs: 1}, nil)
	if err != nil {
		t.Fatalf("NewPain001StreamWriter: %v", err)
	}
	if err := sw.WriteInstruction(nil); err == nil {
		t.Error("expected error for nil instruction")
	}
	if err := sw.WriteInstruction(&CreditTransferInstruction{}); err == nil {
		t.Error("expected error for nil payment")
	}
}

func TestStreamWriterWritten(t *testing.T) {
	var buf bytes.Buffer
	sw, err := NewPacs008StreamWriter(&buf, nil, BatchTotals{NbOfTxs: 2, CtrlSum: "30.5"})
	if err != nil {
		t.Fatalf("NewPacs008StreamWriter: %v", err)
	}
	_ = sw.WriteInstruction(demoInstruction(0))
	_ = sw.WriteInstruction(demoInstruction(1))
	if sw.Written() != 2 {
		t.Errorf("Written: got %d, want 2", sw.Written())
	}
	if err := sw.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestStreamWriterNilFirstSeed(t *testing.T) {
	// nil seed + fully opted writer must not panic — fallbacks kick in.
	var buf bytes.Buffer
	sw, err := NewPain001StreamWriter(&buf, &Pain001Options{
		Debtor:      &Party{AcctID: "GABC", AgentBIC: "DEUTDEFF"},
		ReqdExctnDt: "2026-04-01",
	}, BatchTotals{NbOfTxs: 1, CtrlSum: "10"}, nil)
	if err != nil {
		t.Fatalf("NewPain001StreamWriter: %v", err)
	}
	if err := sw.WriteInstruction(demoInstruction(0)); err != nil {
		t.Fatalf("WriteInstruction: %v", err)
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := ValidateXML(buf.Bytes()); err != nil {
		t.Errorf("document not well-formed: %v", err)
	}
}

// Large batch: streaming must not hold the document in memory — a quick
// sanity run at modest scale proves the incremental path end to end.
func TestStreamPacs008LargeBatch(t *testing.T) {
	const n = 500
	totals := BatchTotals{NbOfTxs: n}
	var buf bytes.Buffer
	sw, err := NewPacs008StreamWriter(&buf, nil, totals)
	if err != nil {
		t.Fatalf("NewPacs008StreamWriter: %v", err)
	}
	for i := 0; i < n; i++ {
		if err := sw.WriteInstruction(demoInstruction(i)); err != nil {
			t.Fatalf("WriteInstruction %d: %v", i, err)
		}
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := strings.Count(buf.String(), "<CdtTrfTxInf>"); got != n {
		t.Errorf("expected %d CdtTrfTxInf, got %d", n, got)
	}
}
