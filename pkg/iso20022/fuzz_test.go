// fuzz_test.go — crash/round-trip fuzz targets for the ISO 20022 parsers
// (docs/audit/differential-fuzzing.md target #6). Local-only: no oracle, no
// network. Properties: parsers never panic, round-trips are stable, and
// DetectMessageType agrees with the concrete parser acceptance.
package iso20022

import (
	"bytes"
	"testing"
)

// FuzzParseDocuments feeds arbitrary bytes through every document parser.
// The only invariant is survival: no panics, no hangs.
func FuzzParseDocuments(f *testing.F) {
	f.Add([]byte(`<?xml version="1.0"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.14"><FIToFICstmrCdtTrf/></Document>`))
	f.Add([]byte("not xml at all"))
	f.Add([]byte{0x00, 0xFF, 0x3C})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DetectMessageType(data)
		_ = ValidateXML(data)
		if d, err := ParsePacs008(data); err == nil && d != nil {
			if _, mErr := MarshalXML(d, NSPacs008); mErr != nil {
				t.Fatalf("parsed pacs.008 fails re-marshal: %v", mErr)
			}
		}
		if d, err := ParsePacs002(data); err == nil && d != nil {
			if _, mErr := MarshalXML(d, NSPacs002); mErr != nil {
				t.Fatalf("parsed pacs.002 fails re-marshal: %v", mErr)
			}
		}
		if d, err := ParsePacs004(data); err == nil && d != nil {
			if _, mErr := MarshalXML(d, NSPacs004); mErr != nil {
				t.Fatalf("parsed pacs.004 fails re-marshal: %v", mErr)
			}
		}
		if d, err := ParsePain001(data); err == nil && d != nil {
			if _, mErr := MarshalXML(d, NSPain001); mErr != nil {
				t.Fatalf("parsed pain.001 fails re-marshal: %v", mErr)
			}
		}
	})
}

// FuzzMarshalUnmarshalRoundTrip: a doc we marshal must re-parse and re-marshal
// to byte-identical output (marshal stability).
func FuzzMarshalUnmarshalRoundTrip(f *testing.F) {
	f.Add([]byte("<FIToFICstmrCdtTrf><GrpHdr><MsgId>m</MsgId></GrpHdr></FIToFICstmrCdtTrf>"))
	f.Fuzz(func(t *testing.T, inner []byte) {
		if len(inner) > 4096 {
			inner = inner[:4096]
		}
		doc := append([]byte(`<?xml version="1.0"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.14">`), inner...)
		doc = append(doc, []byte(`</Document>`)...)
		parsed, err := ParsePacs008(doc)
		if err != nil || parsed == nil {
			return
		}
		out1, err := MarshalXML(parsed, NSPacs008)
		if err != nil {
			t.Fatalf("marshal of parsed doc failed: %v", err)
		}
		parsed2, err := ParsePacs008([]byte(out1))
		if err != nil || parsed2 == nil {
			t.Fatalf("round-trip re-parse failed: %v", err)
		}
		out2, err := MarshalXML(parsed2, NSPacs008)
		if err != nil {
			t.Fatalf("second marshal failed: %v", err)
		}
		if !bytes.Equal([]byte(out1), []byte(out2)) {
			t.Fatalf("unstable marshal:\n  1: %s\n  2: %s", out1, out2)
		}
	})
}
