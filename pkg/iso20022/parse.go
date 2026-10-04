package iso20022

import (
	"encoding/xml"
	"fmt"
)

// ─────────────────────────────────────────────
// Parsing: ingest ISO 20022 documents back
// ─────────────────────────────────────────────
//
// The builders produce XML; these parsers read it back — the direction a
// disbursement platform needs to reconcile bank-side status reports
// (pacs.002 / pain.002) and statements (camt.053) against sent batches.

// nsByMessage maps each supported message type to its XSD namespace.
var nsByMessage = map[string]string{
	MsgPacs008: NSPacs008,
	MsgPacs002: NSPacs002,
	MsgPacs004: NSPacs004,
	MsgPacs009: NSPacs009,
	MsgPain001: NSPain001,
	MsgPain002: NSPain002,
	MsgCamt053: NSCamt053,
	MsgCamt054: NSCamt054,
}

// DetectMessageType reads the Document's xmlns and returns the Msg* constant
// for the message, or an error for unrecognized/non-ISO-20022 documents.
func DetectMessageType(data []byte) (string, error) {
	var probe struct {
		XMLName xml.Name
		Xmlns   string `xml:"xmlns,attr"`
	}
	if err := xml.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("XML parse: %w", err)
	}
	if probe.XMLName.Local != "Document" {
		return "", fmt.Errorf("not an ISO 20022 document (root element %q)", probe.XMLName.Local)
	}
	for msg, ns := range nsByMessage {
		if probe.Xmlns == ns {
			return msg, nil
		}
	}
	return "", fmt.Errorf("unrecognized ISO 20022 namespace %q", probe.Xmlns)
}

// checkNS verifies a parsed document's namespace matches the expected one.
// A mismatch usually means a different message version — the struct tags only
// bind by local name, so a version check is the only way to catch it.
func checkNS(got, want string) error {
	if got != "" && got != want {
		return fmt.Errorf("unexpected namespace %q (want %q)", got, want)
	}
	return nil
}

// ParsePacs008 parses a pacs.008.001.14 document.
func ParsePacs008(data []byte) (*Pacs008Document, error) {
	var doc Pacs008Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pacs.008: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPacs008); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParsePacs002 parses a pacs.002.001.16 document.
func ParsePacs002(data []byte) (*Pacs002Document, error) {
	var doc Pacs002Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pacs.002: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPacs002); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParsePacs004 parses a pacs.004.001.15 document.
func ParsePacs004(data []byte) (*Pacs004Document, error) {
	var doc Pacs004Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pacs.004: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPacs004); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParsePacs009 parses a pacs.009.001.13 document.
func ParsePacs009(data []byte) (*Pacs009Document, error) {
	var doc Pacs009Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pacs.009: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPacs009); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParsePain001 parses a pain.001.001.13 document.
func ParsePain001(data []byte) (*Pain001Document, error) {
	var doc Pain001Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pain.001: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPain001); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParsePain002 parses a pain.002.001.10 document.
func ParsePain002(data []byte) (*Pain002Document, error) {
	var doc Pain002Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pain.002: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSPain002); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParseCamt053 parses a camt.053.001.13 document.
func ParseCamt053(data []byte) (*Camt053Document, error) {
	var doc Camt053Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse camt.053: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSCamt053); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ParseCamt054 parses a camt.054.001.14 document.
func ParseCamt054(data []byte) (*Camt054Document, error) {
	var doc Camt054Document
	if err := UnmarshalXML(data, &doc); err != nil {
		return nil, fmt.Errorf("parse camt.054: %w", err)
	}
	if err := checkNS(doc.Xmlns, NSCamt054); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ─────────────────────────────────────────────
// StatusReport — flattened reconciliation view
// ─────────────────────────────────────────────

// StatusReport is a flattened per-transaction status extracted from a status
// report message (pacs.002 or pain.002). It carries the identifiers a
// disbursement system uses to match the report back to sent payments.
type StatusReport struct {
	StsID         string // StsId — servicer-assigned status ID
	EndToEndID    string // OrgnlEndToEndId — matches the sent payment's EndToEndId
	InstrID       string // OrgnlInstrId — original instruction ID
	TxID          string // OrgnlTxId — original transaction ID
	Status        string // TxSts — e.g. "ACSC", "RJCT", "PDNG"
	ReasonCode    string // StsRsnInf>Rsn>Cd
	ReasonInfo    string // StsRsnInf>AddtlInf (first line)
	OriginalMsgID string // OrgnlGrpInfAndSts>OrgnlMsgId — the sent message ID
}

// statusReport converts a pacs.002 TxInfAndSts to a StatusReport.
func (t *PaymentTransaction177) statusReport(origMsgID string) StatusReport {
	r := StatusReport{
		StsID:         t.StsId,
		EndToEndID:    t.OrgnlEndToEndId,
		InstrID:       t.OrgnlInstrId,
		TxID:          t.OrgnlTxId,
		Status:        t.TxSts,
		OriginalMsgID: origMsgID,
	}
	if len(t.StsRsnInf) > 0 && t.StsRsnInf[0].Rsn != nil {
		r.ReasonCode = t.StsRsnInf[0].Rsn.Cd
		if r.ReasonCode == "" {
			r.ReasonCode = t.StsRsnInf[0].Rsn.Prtry
		}
		if len(t.StsRsnInf[0].AddtlInf) > 0 {
			r.ReasonInfo = t.StsRsnInf[0].AddtlInf[0]
		}
	}
	return r
}

// StatusReports returns one StatusReport per transaction in the document —
// the reconciliation entry point for a pacs.002 status report.
func (d *Pacs002Document) StatusReports() []StatusReport {
	var origMsgID string
	if d.FIToFIPmtStsRpt.OrgnlGrpInfAndSts != nil {
		origMsgID = d.FIToFIPmtStsRpt.OrgnlGrpInfAndSts.OrgnlMsgId
	}
	reports := make([]StatusReport, 0, len(d.FIToFIPmtStsRpt.TxInfAndSts))
	for i := range d.FIToFIPmtStsRpt.TxInfAndSts {
		reports = append(reports, d.FIToFIPmtStsRpt.TxInfAndSts[i].statusReport(origMsgID))
	}
	return reports
}

// ─────────────────────────────────────────────
// Reconciliation — match reports back to sent payments
// ─────────────────────────────────────────────
//
// The disbursement loop: build a batch (pain.001/pacs.008), send it, then
// ingest the bank's status report (pain.002/pacs.002) and update each
// payment's state. ReconcileInstructions does the matching half of that —
// reports are keyed by the same identifiers the builders emit:
//
//   - OrgnlEndToEndId ↔ endToEndID(payment) — tx hash, else payment ID
//   - OrgnlInstrId    ↔ payment ID (truncated to 35 chars)
//   - OrgnlTxId       ↔ payment ID
//
// When several reports reference one payment (PDNG then ACSC), the last wins.
// Status codes map to a consumer's state machine — e.g. a disbursement
// platform maps ACSC→success, RJCT→failed, PDNG/ACSP→pending.

// ReconciliationResult pairs one sent instruction with its latest status.
type ReconciliationResult struct {
	Instruction *CreditTransferInstruction
	PaymentID   string        // instr.Payment.ID
	Report      *StatusReport // nil when unmatched
	Status      string        // ISO status code ("" when unmatched)
	ReasonCode  string
	Matched     bool
}

// ReconciliationSummary is the outcome of matching a status report against
// the sent batch: Results in instruction order, plus Unmatched reports that
// reference payments outside this batch.
type ReconciliationSummary struct {
	Results   []ReconciliationResult
	Unmatched []StatusReport
}

// ReconcileInstructions matches status reports to the instructions that were
// sent, using the identification chain above.
func ReconcileInstructions(instrs []*CreditTransferInstruction, reports []StatusReport) *ReconciliationSummary {
	// Index every identifier the builders emit for each instruction.
	byID := make(map[string]int, len(instrs)*3)
	for i, instr := range instrs {
		p := instr.Payment
		if p == nil {
			continue
		}
		byID[endToEndID(p)] = i
		byID[safeTruncate(p.ID, 35)] = i
		byID[p.ID] = i
	}

	sum := &ReconciliationSummary{
		Results: make([]ReconciliationResult, len(instrs)),
	}
	for i, instr := range instrs {
		sum.Results[i].Instruction = instr
		if instr.Payment != nil {
			sum.Results[i].PaymentID = instr.Payment.ID
		}
	}

	for i := range reports {
		r := &reports[i]
		idx, ok := byID[r.EndToEndID]
		if !ok {
			idx, ok = byID[r.InstrID]
		}
		if !ok {
			idx, ok = byID[r.TxID]
		}
		if !ok {
			sum.Unmatched = append(sum.Unmatched, *r)
			continue
		}
		res := &sum.Results[idx]
		res.Report = r
		res.Status = r.Status
		res.ReasonCode = r.ReasonCode
		res.Matched = true // last report for this instruction wins
	}
	return sum
}
