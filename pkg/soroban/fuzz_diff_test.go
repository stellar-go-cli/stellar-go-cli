// fuzz_diff_test.go — Tier-2 differential fuzz targets: this package's
// Go implementation in-process vs the installed `stellar` (Rust) CLI as
// oracle. See docs/audit/differential-fuzzing.md. All targets skip cleanly
// when the binary is absent.
package soroban

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// FuzzScValEncodeDifferential: our XDR encoding of a generated ScVal must be
// byte-identical to `stellar xdr encode` of the same value in serde JSON.
// This catches XDR union-arm, discriminant, and padding divergences.
func FuzzScValEncodeDifferential(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Add(bytes.Repeat([]byte{10}, 64))
	f.Fuzz(func(t *testing.T, data []byte) {
		sv, jv := genScVal(&genDec{data: data}, 3)
		doc, err := json.Marshal(jv)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		ours, err := sv.MarshalBinary()
		if err != nil {
			t.Fatalf("go marshal: %v", err)
		}
		oracleB64, err := xdrEncode(t, "ScVal", string(doc))
		if err != nil {
			t.Fatalf("oracle rejected generated value %s: %v", doc, err)
		}
		oracleXDR, err := base64.StdEncoding.DecodeString(oracleB64)
		if err != nil {
			t.Fatalf("oracle output not base64: %v", err)
		}
		if !bytes.Equal(ours, oracleXDR) {
			t.Fatalf("XDR divergence for %s:\n  go:     %x\n  oracle: %x", doc, ours, oracleXDR)
		}
	})
}

// FuzzScValDecodeDifferential: arbitrary bytes that Go decodes as an ScVal
// must also decode under the oracle, and the decoded JSON structures must
// agree on the subset our codec covers.
func FuzzScValDecodeDifferential(f *testing.F) {
	sv, _ := genScVal(&genDec{data: []byte{1, 2, 3}}, 3)
	seed, _ := sv.MarshalBinary()
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		var goVal xdr.ScVal
		if err := goVal.UnmarshalBinary(data); err != nil {
			t.Skip() // invalid XDR — nothing to compare
		}
		// go-xdr's UnmarshalBinary accepts trailing bytes Rust's decoder
		// rejects. When Go's canonical re-encode doesn't reproduce the input,
		// the decode was lenient — record it, don't diff it. A strict
		// divergence is Go consuming ALL bytes yet the oracle still refusing.
		reenc, err := goVal.MarshalBinary()
		if err != nil {
			t.Fatalf("Go decoded but cannot re-encode %x: %v", data, err)
		}
		if !bytes.Equal(reenc, data) {
			t.Skipf("lenient decode (trailing/non-canonical bytes): %x → %x", data, reenc)
		}
		oracleJSON, err := xdrDecode(t, "ScVal", base64.StdEncoding.EncodeToString(data))
		if err != nil {
			t.Fatalf("Go decoded but oracle rejected XDR %x: %v", data, err)
		}
		var oracleVal interface{}
		if err := json.Unmarshal([]byte(oracleJSON), &oracleVal); err != nil {
			t.Fatalf("oracle JSON invalid: %v", err)
		}
		goJSON, err := scvalToOracleJSON(goVal)
		if err != nil {
			t.Skip() // variant outside codec coverage
		}
		if !deepJSONEqual(goJSON, oracleVal) {
			gb, _ := json.Marshal(goJSON)
			t.Fatalf("decode divergence for XDR %x:\n  go:     %s\n  oracle: %s", data, gb, oracleJSON)
		}
	})
}

// deepJSONEqual normalizes both sides through JSON (float64 canonicalization)
// before comparing.
func deepJSONEqual(a, b interface{}) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	var an interface{}
	if json.Unmarshal(ab, &an) != nil {
		return false
	}
	return reflect.DeepEqual(an, b)
}

// FuzzTxEnvelopeDifferential: a generated transaction envelope's Go-computed
// hash must equal `stellar tx hash`, and Go's envelope decode must accept
// what the oracle accepts.
func FuzzTxEnvelopeDifferential(f *testing.F) {
	f.Add([]byte{1, 100, 7, 42, 9, 0, 255, 64})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			t.Skip()
		}
		// Deterministic structure-aware generation: payment op with
		// fuzz-derived seq/fee/amount; fixed accounts keep generation cheap.
		src, err := keypair.Random()
		if err != nil {
			t.Skip()
		}
		dst, err := keypair.Random()
		if err != nil {
			t.Skip()
		}
		d := &genDec{data: data}
		seq := int64(d.n(8)%1_000_000) + 1
		fee := int64(d.n(4)%10_000) + 100
		amountVal := d.n(8)%1_000_000_000 + 1
		op := &txnbuild.Payment{
			Destination: dst.Address(),
			Amount:      amount.StringFromInt64(int64(amountVal)),
			Asset:       txnbuild.NativeAsset{},
		}
		tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
			SourceAccount:        &txnbuild.SimpleAccount{AccountID: src.Address(), Sequence: seq},
			IncrementSequenceNum: false,
			BaseFee:              fee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			Operations:           []txnbuild.Operation{op},
		})
		if err != nil {
			t.Fatalf("build tx: %v", err)
		}
		b64, err := tx.Base64()
		if err != nil {
			t.Fatalf("encode envelope: %v", err)
		}
		passphrase := network.TestNetworkPassphrase
		goHash, err := tx.HashHex(passphrase)
		if err != nil {
			t.Fatalf("go hash: %v", err)
		}
		oracleHash, err := txHash(t, b64, passphrase)
		if err != nil {
			t.Fatalf("oracle rejected Go-produced envelope: %v", err)
		}
		if goHash != oracleHash {
			t.Fatalf("tx hash divergence:\n  go:     %s\n  oracle: %s\n  xdr: %s", goHash, oracleHash, b64)
		}
		// Oracle decode must also accept our envelope bytes.
		if _, err := txDecode(t, b64); err != nil {
			t.Fatalf("oracle tx decode rejected Go envelope: %v", err)
		}
	})
}

// FuzzStrkeyDifferential: strkey accept/reject and decoded payload must agree
// between the Go SDK and `stellar strkey decode`.
func FuzzStrkeyDifferential(f *testing.F) {
	payload := make([]byte, 32)
	f.Add(payload)
	f.Add(bytes.Repeat([]byte{0xFF}, 32))
	f.Fuzz(func(t *testing.T, data []byte) {
		d := &genDec{data: data}
		// Mix valid encodes with raw-string corruption: half the inputs are
		// generated strkeys, half are byte-mutated for malformed coverage.
		raw := make([]byte, 32)
		copy(raw, data)
		s, err := strkey.Encode(strkey.VersionByteAccountID, raw)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if d.next()%2 == 0 && len(s) > 4 {
			i := int(d.next()) % len(s)
			s = s[:i] + string(65+d.next()%26) + s[i+1:]
		}
		vb, goPayload, goErr := strkey.DecodeAny(s)
		oracle, oracleErr := strkeyDecode(t, s)
		switch {
		case goErr == nil && oracleErr != nil:
			t.Fatalf("Go accepted %q, oracle rejected: %v", s, oracleErr)
		case goErr != nil && oracleErr == nil:
			t.Fatalf("oracle accepted %q, Go rejected: %v", s, goErr)
		case goErr == nil && oracleErr == nil:
			var key string
			switch vb {
			case strkey.VersionByteAccountID:
				key = "public_key_ed25519"
			case strkey.VersionByteContract:
				key = "contract"
			default:
				return // other version bytes: no agreed JSON key
			}
			oh, ok := oracle[key].(string)
			if !ok {
				t.Fatalf("oracle JSON missing %q for %q: %v", key, s, oracle)
			}
			op, err := hex.DecodeString(oh)
			if err != nil || !bytes.Equal(op, goPayload) {
				t.Fatalf("payload divergence for %q:\n  go:     %x\n  oracle: %s", s, goPayload, oh)
			}
		}
	})
}
