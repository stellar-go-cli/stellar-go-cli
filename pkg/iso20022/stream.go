package iso20022

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// ─────────────────────────────────────────────
// Streaming batch writers
// ─────────────────────────────────────────────
//
// Disbursement batches run to tens of thousands of payments — too large to
// comfortably build as one in-memory document. The stream writers emit the
// same XML as BuildPacs008Batch/BuildPain001, one instruction at a time,
// holding only a single transaction fragment in memory.
//
// Because NbOfTxs and CtrlSum live in the GrpHdr (before the transactions),
// totals must be supplied upfront — ComputeBatchTotals derives them for
// slice-driven callers, or fill BatchTotals yourself when streaming from a
// database cursor.

// BatchTotals carries the aggregate values a batch GrpHdr needs upfront.
type BatchTotals struct {
	NbOfTxs   int    // transaction count
	CtrlSum   string // sum of instructed amounts ("" when uncomputable)
	SingleCcy string // settlement currency shared by every instruction ("" when mixed)
}

// ComputeBatchTotals derives NbOfTxs, CtrlSum, and the shared settlement
// currency for a batch. NbOfTxs is always populated; a sum error returns the
// totals so far plus the error.
func ComputeBatchTotals(instrs []*CreditTransferInstruction) (BatchTotals, error) {
	t := BatchTotals{NbOfTxs: len(instrs)}
	amounts := make([]string, 0, len(instrs))
	ccys := map[string]bool{}
	for _, instr := range instrs {
		p := instr.Payment
		amounts = append(amounts, normalizeAmount(p.Amount))
		ccy, _ := settlementCurrency(paymentAsset(p), p.AssetIssuer, p.Amount)
		ccys[ccy] = true
	}
	if len(ccys) == 1 {
		for c := range ccys {
			t.SingleCcy = c
		}
	}
	sum, err := sumAmounts(amounts)
	if err != nil {
		return t, err
	}
	t.CtrlSum = sum
	return t, nil
}

// writeElem encodes v as a single XML element with the given name.
func writeElem(enc *xml.Encoder, name string, v interface{}) error {
	return enc.EncodeElement(v, xml.StartElement{Name: xml.Name{Local: name}})
}

// ─────────────────────────────────────────────
// pacs.008 stream writer
// ─────────────────────────────────────────────

// Pacs008StreamWriter streams a pacs.008.001.14 document to an io.Writer.
type Pacs008StreamWriter struct {
	w      io.Writer
	enc    *xml.Encoder
	opts   *Pacs008BatchOptions
	want   int
	wrote  int
	closed bool
	err    error
}

// NewPacs008StreamWriter writes the XML declaration, Document opening, and
// GrpHdr to w, then returns a writer ready for WriteInstruction calls.
func NewPacs008StreamWriter(w io.Writer, opts *Pacs008BatchOptions, totals BatchTotals) (*Pacs008StreamWriter, error) {
	if opts == nil {
		opts = &Pacs008BatchOptions{}
	}
	creDtTm := opts.CreDtTm
	if creDtTm == "" {
		creDtTm = time.Now().UTC().Format(time.RFC3339)
	}
	msgID := opts.MsgID
	if msgID == "" {
		msgID = "SGC1" + safeTruncate(generateUUIDv4(), 8)
	}

	s := &Pacs008StreamWriter{
		w:    w,
		enc:  xml.NewEncoder(w),
		opts: opts,
		want: totals.NbOfTxs,
	}
	if _, err := io.WriteString(w, xml.Header+`<Document xmlns="`+NSPacs008+`"><FIToFICstmrCdtTrf>`); err != nil {
		return nil, err
	}
	if err := writeElem(s.enc, "GrpHdr", pacs008GrpHdr(opts, msgID, creDtTm, totals)); err != nil {
		return nil, err
	}
	if err := s.enc.Flush(); err != nil {
		return nil, err
	}
	return s, nil
}

// WriteInstruction appends one CdtTrfTxInf element.
func (s *Pacs008StreamWriter) WriteInstruction(instr *CreditTransferInstruction) error {
	if s.err != nil {
		return s.err
	}
	if s.closed {
		return fmt.Errorf("Pacs008StreamWriter: write after Close")
	}
	if instr == nil || instr.Payment == nil {
		return fmt.Errorf("Pacs008StreamWriter: instruction has nil payment")
	}
	p := instr.Payment
	txID := s.opts.TxID
	if txID == "" {
		txID = p.ID
	}
	uetr := s.opts.UETR
	if uetr == "" {
		uetr = generateUUIDv4()
	}
	sttlmDt := p.CreatedAt.UTC().Format("2006-01-02")
	txInf := pacs008TxInf(instr, &s.opts.Pacs008Options, s.opts.Debtor, txID, uetr, sttlmDt)
	if err := writeElem(s.enc, "CdtTrfTxInf", *txInf); err != nil {
		s.err = err
		return err
	}
	s.wrote++
	return s.enc.Flush()
}

// Written returns the number of transactions written so far.
func (s *Pacs008StreamWriter) Written() int { return s.wrote }

// Close writes the closing tags. It reports an error when the number of
// written transactions does not match the totals NbOfTxs the GrpHdr claimed —
// the emitted document is then invalid and should be discarded.
func (s *Pacs008StreamWriter) Close() error {
	if s.closed {
		return s.err
	}
	s.closed = true
	if s.err != nil {
		return s.err
	}
	if _, err := io.WriteString(s.w, "</FIToFICstmrCdtTrf></Document>\n"); err != nil {
		s.err = err
		return err
	}
	if s.wrote != s.want {
		s.err = fmt.Errorf("Pacs008StreamWriter: wrote %d transactions, GrpHdr claims NbOfTxs=%d", s.wrote, s.want)
		return s.err
	}
	return nil
}

// StreamPacs008Batch streams a full pacs.008 batch to w — the streaming
// equivalent of BuildPacs008Batch.
func StreamPacs008Batch(w io.Writer, instrs []*CreditTransferInstruction, opts *Pacs008BatchOptions) error {
	if err := requireInstructions(instrs, "StreamPacs008Batch"); err != nil {
		return err
	}
	totals, _ := ComputeBatchTotals(instrs) //nolint:errcheck // mirrors BuildPacs008Batch degradation
	sw, err := NewPacs008StreamWriter(w, opts, totals)
	if err != nil {
		return err
	}
	for _, instr := range instrs {
		if err := sw.WriteInstruction(instr); err != nil {
			return err
		}
	}
	return sw.Close()
}

// ─────────────────────────────────────────────
// pain.001 stream writer
// ─────────────────────────────────────────────

// Pain001StreamWriter streams a pain.001.001.13 document to an io.Writer —
// the format a disbursement platform emits for its bank/agent.
type Pain001StreamWriter struct {
	w      io.Writer
	enc    *xml.Encoder
	opts   *Pain001Options
	want   int
	wrote  int
	closed bool
	err    error
}

// NewPain001StreamWriter writes the XML declaration, Document opening,
// GrpHdr, and PmtInf header to w, then returns a writer ready for
// WriteInstruction calls.
//
// first seeds the header fallbacks pain.001 requires (debtor, ReqdExctnDt) —
// pass the batch's first instruction when known, or nil when opts.Debtor and
// opts.ReqdExctnDt cover it. Either way, every instruction must still be
// written via WriteInstruction, including the seed.
func NewPain001StreamWriter(w io.Writer, opts *Pain001Options, totals BatchTotals, first *CreditTransferInstruction) (*Pain001StreamWriter, error) {
	if opts == nil {
		opts = &Pain001Options{}
	}
	creDtTm := time.Now().UTC().Format(time.RFC3339)
	msgID := opts.MsgID
	if msgID == "" {
		msgID = "SGC1P" + safeTruncate(generateUUIDv4(), 7)
	}

	seed := &models.Payment{}
	var firstDebtor *Party
	if first != nil {
		if first.Payment != nil {
			seed = first.Payment
		}
		firstDebtor = first.Debtor
	}

	s := &Pain001StreamWriter{
		w:    w,
		enc:  xml.NewEncoder(w),
		opts: opts,
		want: totals.NbOfTxs,
	}
	if _, err := io.WriteString(w, xml.Header+`<Document xmlns="`+NSPain001+`"><CstmrCdtTrfInitn>`); err != nil {
		return nil, err
	}

	// Marshal the PmtInf header via EncodeElement so the element name is
	// <PmtInf>; strip the closing tag — transactions are appended before it.
	grpHdr, pmtInf := pain001Head(seed, firstDebtor, opts, msgID, creDtTm, totals)

	var headBuf bytes.Buffer
	headEnc := xml.NewEncoder(&headBuf)
	if err := writeElem(headEnc, "GrpHdr", grpHdr); err != nil {
		return nil, err
	}
	if err := writeElem(headEnc, "PmtInf", pmtInf); err != nil {
		return nil, err
	}
	if err := headEnc.Flush(); err != nil {
		return nil, err
	}
	head := bytes.TrimSuffix(headBuf.Bytes(), []byte("</PmtInf>"))
	if _, err := w.Write(head); err != nil {
		return nil, err
	}
	return s, nil
}

// WriteInstruction appends one CdtTrfTxInf element to the open PmtInf.
func (s *Pain001StreamWriter) WriteInstruction(instr *CreditTransferInstruction) error {
	if s.err != nil {
		return s.err
	}
	if s.closed {
		return fmt.Errorf("Pain001StreamWriter: write after Close")
	}
	if instr == nil || instr.Payment == nil {
		return fmt.Errorf("Pain001StreamWriter: instruction has nil payment")
	}
	txInf := pain001TxInf(instr, s.opts)
	if err := writeElem(s.enc, "CdtTrfTxInf", *txInf); err != nil {
		s.err = err
		return err
	}
	s.wrote++
	return s.enc.Flush()
}

// Written returns the number of transactions written so far.
func (s *Pain001StreamWriter) Written() int { return s.wrote }

// Close writes the PmtInf and document closing tags. It reports an error when
// the number of written transactions does not match the totals NbOfTxs the
// header claimed — the emitted document is then invalid and should be
// discarded.
func (s *Pain001StreamWriter) Close() error {
	if s.closed {
		return s.err
	}
	s.closed = true
	if s.err != nil {
		return s.err
	}
	if _, err := io.WriteString(s.w, "</PmtInf></CstmrCdtTrfInitn></Document>\n"); err != nil {
		s.err = err
		return err
	}
	if s.wrote != s.want {
		s.err = fmt.Errorf("Pain001StreamWriter: wrote %d transactions, header claims NbOfTxs=%d", s.wrote, s.want)
		return s.err
	}
	return nil
}

// StreamPain001Batch streams a full pain.001 batch to w — the streaming
// equivalent of BuildPain001.
func StreamPain001Batch(w io.Writer, instrs []*CreditTransferInstruction, opts *Pain001Options) error {
	if err := requireInstructions(instrs, "StreamPain001Batch"); err != nil {
		return err
	}
	totals, err := ComputeBatchTotals(instrs)
	if err != nil {
		return fmt.Errorf("StreamPain001Batch: %w", err)
	}
	sw, err := NewPain001StreamWriter(w, opts, totals, instrs[0])
	if err != nil {
		return err
	}
	for _, instr := range instrs {
		if err := sw.WriteInstruction(instr); err != nil {
			return err
		}
	}
	return sw.Close()
}
