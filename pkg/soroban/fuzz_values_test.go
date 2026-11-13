// fuzz_values_test.go — local (oracle-free) fuzz targets for pkg/soroban:
// CompareScVal total-order laws, ToScVal never-panic, and ScvMap sort
// determinism. These run everywhere, including CI without the stellar CLI.
package soroban

import (
	"bytes"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// FuzzCompareScVal asserts the total-order laws the Rust soroban-sdk ordering
// relies on: reflexivity, antisymmetry, and totality. Transitivity is asserted
// over triples. Any violation breaks ScvMap canonicalization.
func FuzzCompareScVal(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add(bytes.Repeat([]byte{11}, 96))
	f.Fuzz(func(t *testing.T, data []byte) {
		d := &genDec{data: data}
		a, _ := genScVal(d, 3)
		b, _ := genScVal(d, 3)
		c, _ := genScVal(d, 3)

		if got := CompareScVal(a, a); got != 0 {
			t.Fatalf("reflexivity violated: CompareScVal(a,a)=%d", got)
		}
		if ab, ba := CompareScVal(a, b), CompareScVal(b, a); ab != -ba {
			t.Fatalf("antisymmetry violated: cmp(a,b)=%d cmp(b,a)=%d", ab, ba)
		}
		if ab, bc := CompareScVal(a, b), CompareScVal(b, c); ab < 0 && bc < 0 && CompareScVal(a, c) >= 0 {
			t.Fatalf("transitivity violated: a<b b<c but !(a<c)")
		}
		if ab := CompareScVal(a, b); ab != 0 && CompareScVal(b, a) == 0 {
			t.Fatalf("totality violated: cmp(a,b)=%d but cmp(b,a)=0", ab)
		}
	})
}

// FuzzToScVal exercises the struct/map/slice → ScVal encoder with fuzz-
// constructed inputs; it must never panic on supported types.
func FuzzToScVal(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		d := &genDec{data: data}
		// Mix of supported shapes: strings, ints, bools, byte slices, maps
		// with string keys, slices, nested maps.
		var v any
		switch d.next() % 6 {
		case 0:
			v = map[string]any{"k" + string('a'+d.next()%26): int64(d.n(4))}
		case 1:
			v = []any{"s", int64(d.n(4)), d.next()%2 == 0}
		case 2:
			v = struct {
				Name string `soroban:"name"`
				Val  int64  `soroban:"val"`
			}{Name: "x", Val: int64(d.n(8))}
		case 3:
			v = map[string]any{
				"nested": map[string]any{"a": uint64(d.n(8))},
				"bytes":  data,
			}
		case 4:
			v = int64(d.n(8))
		default:
			v = string(data[:min(len(data), 32)])
		}
		sv, err := ToScVal(v)
		if err != nil {
			return // unsupported input kinds may error — that's fine
		}
		if _, err := sv.MarshalBinary(); err != nil {
			t.Fatalf("produced ScVal fails XDR marshal for %T: %v", v, err)
		}
	})
}

// FuzzScMapSortDeterminism: sorting the same entry set from different input
// orders must produce byte-identical XDR — the canonical-map invariant.
func FuzzScMapSortDeterminism(f *testing.F) {
	f.Add([]byte{5, 6, 7, 8})
	f.Fuzz(func(t *testing.T, data []byte) {
		d := &genDec{data: data}
		cnt := int(d.next())%4 + 1
		entries := make([]xdr.ScMapEntry, cnt)
		ok := true
		for i := range entries {
			k, _ := genScVal(d, 0)
			v, _ := genScVal(d, 2)
			entries[i] = xdr.ScMapEntry{Key: k, Val: v}
		}
		sv1, err1 := tryScvMap(append([]xdr.ScMapEntry{}, entries...))
		if err1 != nil {
			return // duplicate keys legitimately reject
		}
		// Reverse the input order — sorted output must be identical.
		rev := make([]xdr.ScMapEntry, cnt)
		for i, e := range entries {
			rev[cnt-1-i] = e
		}
		sv2, err2 := tryScvMap(rev)
		if !ok || err2 != nil {
			t.Fatalf("order-dependent map acceptance: %v vs %v", err1, err2)
		}
		b1, _ := sv1.MarshalBinary()
		b2, _ := sv2.MarshalBinary()
		if !bytes.Equal(b1, b2) {
			t.Fatalf("non-canonical map sort:\n  fwd: %x\n  rev: %x", b1, b2)
		}
	})
}
