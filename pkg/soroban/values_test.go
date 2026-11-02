package soroban

import (
	"math/big"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func mustMarshalB64(t *testing.T, v xdr.ScVal) string {
	t.Helper()
	b64, err := xdr.MarshalBase64(v)
	if err != nil {
		t.Fatalf("marshal ScVal: %v", err)
	}
	return b64
}

// manualScMap builds a ScvMap the verbose way — entries already in canonical
// order — so tests can compare it against encoder output built through a
// different code path.
func manualScMap(entries ...xdr.ScMapEntry) xdr.ScVal {
	m := xdr.ScMap(entries)
	v, err := xdr.NewScVal(xdr.ScValTypeScvMap, &m)
	if err != nil {
		panic(err)
	}
	return v
}

func TestScvVec(t *testing.T) {
	got := ScvVec(ScvU32(1), ScvSymbol("two"), ScvBool(true))
	if got.Type != xdr.ScValTypeScvVec {
		t.Fatalf("expected ScvVec, got %s", got.Type)
	}
	vec := *got.Vec
	if len(*vec) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(*vec))
	}
}

func TestScvMapSortsEntriesCanonically(t *testing.T) {
	// Canonical order: ScvU32 (3) < ScvU64 (5) < ScvSymbol (15).
	got := ScvMap(
		ScvPair(ScvSymbol("b"), ScvU32(2)),
		ScvPair(ScvU64(9), ScvU32(0)),
		ScvPair(ScvSymbol("a"), ScvU32(1)),
		ScvPair(ScvU32(7), ScvU32(0)),
	)
	want := manualScMap(
		xdr.ScMapEntry{Key: ScvU32(7), Val: ScvU32(0)},
		xdr.ScMapEntry{Key: ScvU64(9), Val: ScvU32(0)},
		xdr.ScMapEntry{Key: ScvSymbol("a"), Val: ScvU32(1)},
		xdr.ScMapEntry{Key: ScvSymbol("b"), Val: ScvU32(2)},
	)
	gotB64, wantB64 := mustMarshalB64(t, got), mustMarshalB64(t, want)
	if gotB64 != wantB64 {
		t.Fatalf("canonical ordering mismatch:\n got %s\nwant %s", gotB64, wantB64)
	}
}

func TestScvMapRejectsDuplicateKeys(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate keys")
		}
	}()
	ScvMap(
		ScvPair(ScvSymbol("k"), ScvU32(1)),
		ScvPair(ScvSymbol("k"), ScvU32(2)),
	)
}

func TestCompareScVal(t *testing.T) {
	neg1 := big.NewInt(-1)
	pos1 := big.NewInt(1)
	cases := []struct {
		name string
		a, b xdr.ScVal
		want int
	}{
		{"bool lt", ScvBool(false), ScvBool(true), -1},
		{"bool gt", ScvBool(true), ScvBool(false), 1},
		{"void eq", mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), 0},
		{"u32 lt", ScvU32(1), ScvU32(2), -1},
		{"i32 sign", ScvI32(-5), ScvI32(5), -1},
		{"u64 gt", ScvU64(10), ScvU64(9), 1},
		{"i64 eq", ScvI64(-42), ScvI64(-42), 0},
		{"i128 sign", ScvI128(neg1), ScvI128(pos1), -1},
		{"type rank u32<i32", ScvU32(99), ScvI32(-1), -1},
		{"type rank sym<vec", ScvSymbol("zzz"), ScvVec(ScvU32(1)), -1},
		{"bytes lt", ScvBytes([]byte{0x01}), ScvBytes([]byte{0x02}), -1},
		{"bytes prefix", ScvBytes([]byte{0x01}), ScvBytes([]byte{0x01, 0x00}), -1},
		{"string lt", ScvString("abc"), ScvString("abd"), -1},
		{"symbol eq", ScvSymbol("sym"), ScvSymbol("sym"), 0},
		{"vec elem", ScvVec(ScvU32(1), ScvU32(9)), ScvVec(ScvU32(2), ScvU32(0)), -1},
		{"vec prefix", ScvVec(ScvU32(1)), ScvVec(ScvU32(1), ScvU32(0)), -1},
		{"map key", ScvMap(ScvPair(ScvSymbol("a"), ScvU32(1))), ScvMap(ScvPair(ScvSymbol("b"), ScvU32(1))), -1},
		{"map val", ScvMap(ScvPair(ScvSymbol("a"), ScvU32(1))), ScvMap(ScvPair(ScvSymbol("a"), ScvU32(2))), -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sign(CompareScVal(tc.a, tc.b)); got != tc.want {
				t.Fatalf("CompareScVal = %d, want sign %d", got, tc.want)
			}
			if got := sign(CompareScVal(tc.b, tc.a)); got != -tc.want {
				t.Fatalf("CompareScVal swapped = %d, want sign %d", got, -tc.want)
			}
		})
	}
}

func TestCompareScValAddress(t *testing.T) {
	acct, err := AccountToScAddress(keypair.MustRandom().Address())
	if err != nil {
		t.Fatalf("account to sc address: %v", err)
	}
	contract, err := ParseContractID("CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC")
	if err != nil {
		t.Fatalf("parse contract id: %v", err)
	}
	// Account addresses (0) sort before contract addresses (1).
	if sign(CompareScVal(ScvAddress(acct), ScvAddress(contract))) != -1 {
		t.Fatal("expected account address < contract address")
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// vaeltrix-style types: field names mirror the Rust contracttype structs that
// vaeltrix-types freezes test vectors for.
type slaTemplate struct {
	Active     bool   `soroban:"active"`
	GraceMs    uint64 `soroban:"grace_ms"`
	TargetMs   uint64 `soroban:"target_ms"`
	TemplateID uint32 `soroban:"template_id"`
}

type commitmentParams struct {
	Amount      *big.Int `soroban:"amount"`
	ProviderID  string   `soroban:"provider_id"`
	SlaTemplate slaTemplate
	Status      string `soroban:"status,symbol"`
	Internal    string `soroban:"-"`
}

// TestToScValStructMatchesCanonicalEncoding asserts a tagged Go struct encodes
// byte-for-byte like the equivalent Rust #[contracttype] struct: a ScvMap with
// ScvSymbol keys in canonical order.
func TestToScValStructMatchesCanonicalEncoding(t *testing.T) {
	tmpl := slaTemplate{Active: true, GraceMs: 500, TargetMs: 3000, TemplateID: 1}
	want := manualScMap(
		xdr.ScMapEntry{Key: ScvSymbol("active"), Val: ScvBool(true)},
		xdr.ScMapEntry{Key: ScvSymbol("grace_ms"), Val: ScvU64(500)},
		xdr.ScMapEntry{Key: ScvSymbol("target_ms"), Val: ScvU64(3000)},
		xdr.ScMapEntry{Key: ScvSymbol("template_id"), Val: ScvU32(1)},
	)
	got, err := ToScVal(tmpl)
	if err != nil {
		t.Fatalf("ToScVal: %v", err)
	}
	if gotB64, wantB64 := mustMarshalB64(t, got), mustMarshalB64(t, want); gotB64 != wantB64 {
		t.Fatalf("encoding mismatch:\n got %s\nwant %s", gotB64, wantB64)
	}
}

func TestToScValNestedStructAndTags(t *testing.T) {
	params := commitmentParams{
		Amount:     big.NewInt(10_000),
		ProviderID: "7c2e41",
		SlaTemplate: slaTemplate{
			Active: true, GraceMs: 500, TargetMs: 3000, TemplateID: 1,
		},
		Status:   "active",
		Internal: "skipped",
	}
	tmplMap := manualScMap(
		xdr.ScMapEntry{Key: ScvSymbol("active"), Val: ScvBool(true)},
		xdr.ScMapEntry{Key: ScvSymbol("grace_ms"), Val: ScvU64(500)},
		xdr.ScMapEntry{Key: ScvSymbol("target_ms"), Val: ScvU64(3000)},
		xdr.ScMapEntry{Key: ScvSymbol("template_id"), Val: ScvU32(1)},
	)
	// Canonical key order: "SlaTemplate" (uppercase) sorts before the
	// lowercase tag-named keys.
	want := manualScMap(
		xdr.ScMapEntry{Key: ScvSymbol("SlaTemplate"), Val: tmplMap},
		xdr.ScMapEntry{Key: ScvSymbol("amount"), Val: ScvI128(big.NewInt(10_000))},
		xdr.ScMapEntry{Key: ScvSymbol("provider_id"), Val: ScvString("7c2e41")},
		xdr.ScMapEntry{Key: ScvSymbol("status"), Val: ScvSymbol("active")},
	)
	got, err := ToScVal(params)
	if err != nil {
		t.Fatalf("ToScVal: %v", err)
	}
	if gotB64, wantB64 := mustMarshalB64(t, got), mustMarshalB64(t, want); gotB64 != wantB64 {
		t.Fatalf("encoding mismatch:\n got %s\nwant %s", gotB64, wantB64)
	}
}

func TestToScValScalarsAndContainers(t *testing.T) {
	one := big.NewInt(1)
	cases := []struct {
		name string
		in   any
		want xdr.ScVal
	}{
		{"scval passthrough", ScvU32(7), ScvU32(7)},
		{"bool", true, ScvBool(true)},
		{"string", "hi", ScvString("hi")},
		{"int32", int32(-3), ScvI32(-3)},
		{"int64", int64(-3), ScvI64(-3)},
		{"int", 5, ScvI64(5)},
		{"uint32", uint32(5), ScvU32(5)},
		{"uint64", uint64(5), ScvU64(5)},
		{"bytes", []byte{1, 2, 3}, ScvBytes([]byte{1, 2, 3})},
		{"byte array", [2]byte{9, 8}, ScvBytes([]byte{9, 8})},
		{"bigint", *one, ScvI128(one)},
		{"bigint ptr", one, ScvI128(one)},
		{"nil ptr", (*big.Int)(nil), mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil))},
		{"slice", []uint32{1, 2}, ScvVec(ScvU32(1), ScvU32(2))},
		{"scval slice", []xdr.ScVal{ScvBool(true)}, ScvVec(ScvBool(true))},
		{"string map", map[string]uint32{"b": 2, "a": 1},
			manualScMap(
				xdr.ScMapEntry{Key: ScvString("a"), Val: ScvU32(1)},
				xdr.ScMapEntry{Key: ScvString("b"), Val: ScvU32(2)},
			)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToScVal(tc.in)
			if err != nil {
				t.Fatalf("ToScVal: %v", err)
			}
			if gotB64, wantB64 := mustMarshalB64(t, got), mustMarshalB64(t, tc.want); gotB64 != wantB64 {
				t.Fatalf("encoding mismatch:\n got %s\nwant %s", gotB64, wantB64)
			}
		})
	}
}

func TestToScValErrors(t *testing.T) {
	cases := []struct {
		name string
		in   any
	}{
		{"nil", nil},
		{"float", 1.5},
		{"chan", make(chan int)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ToScVal(tc.in); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
