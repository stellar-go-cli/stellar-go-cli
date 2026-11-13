// fuzz_test.go — crash fuzz targets for x402 header/amount parsing
// (docs/audit/differential-fuzzing.md target #7). Local-only.
package x402

import (
	"encoding/base64"
	"testing"
)

// FuzzDecodeHeaders feeds arbitrary strings through the three payment-header
// decoders: base64+JSON+schema is a classic crash surface.
func FuzzDecodeHeaders(f *testing.F) {
	f.Add(base64.StdEncoding.EncodeToString([]byte(`{"x402Version":1,"accepts":[]}`)))
	f.Add("not-base64!!!")
	f.Add(base64.StdEncoding.EncodeToString([]byte(`{"a":{"b":{"c":[1,2,{"d":"x"}]}}}`)))
	f.Fuzz(func(t *testing.T, header string) {
		if _, err := DecodePaymentRequiredHeader(header); err == nil {
			// decoded — ValidateStellarRequirement must also survive
			// whatever the decoder produced.
			if req, derr := DecodePaymentRequiredHeader(header); derr == nil && req != nil {
				for _, acc := range req.Accepts {
					_ = ValidateStellarRequirement(&acc)
				}
			}
		}
		_, _ = DecodePaymentResponseHeader(header)
		_, _ = DecodePaymentSignatureHeader(header)
	})
}

// FuzzParseAtomicAmount exercises the decimal→atomic-units conversion.
func FuzzParseAtomicAmount(f *testing.F) {
	f.Add("1.5")
	f.Add("0.0000001")
	f.Add("1e30")
	f.Add("-42")
	f.Add(".")
	f.Add("9" + string(make([]byte, 0)))
	f.Fuzz(func(t *testing.T, v string) {
		if len(v) > 256 {
			t.Skip() // bound: amounts beyond this are rejected by callers anyway
		}
		n, err := ParseAtomicAmount(v)
		if err != nil {
			return
		}
		if n == nil {
			t.Fatalf("nil big.Int for %q", v)
		}
		if n.Sign() < 0 {
			t.Fatalf("negative atomic amount accepted for %q", v)
		}
	})
}
