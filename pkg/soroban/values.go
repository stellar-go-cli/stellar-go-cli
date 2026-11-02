package soroban

import (
	"bytes"
	"cmp"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// sortScMapEntries sorts entries in place by canonical ScVal key ordering and
// reports duplicate keys as an error. Soroban requires map keys to be sorted
// and unique; the ordering is the one the Soroban host and stellar-xdr's
// ScVal Ord implementation use.
func sortScMapEntries(entries []xdr.ScMapEntry) error {
	slices.SortFunc(entries, func(a, b xdr.ScMapEntry) int {
		return CompareScVal(a.Key, b.Key)
	})
	for i := 1; i < len(entries); i++ {
		if CompareScVal(entries[i-1].Key, entries[i].Key) == 0 {
			return fmt.Errorf("duplicate map key at index %d", i)
		}
	}
	return nil
}

// CompareScVal compares two ScVals using Soroban's canonical ordering — the
// same order the Soroban host applies when normalizing map keys and the order
// stellar-xdr's Rust Ord implementation produces. Values of different types
// order by ScValType discriminant; values of the same type order by payload.
// It returns a negative number, zero, or a positive number when a is less
// than, equal to, or greater than b.
func CompareScVal(a, b xdr.ScVal) int {
	if a.Type != b.Type {
		return cmp.Compare(a.Type, b.Type)
	}
	switch a.Type {
	case xdr.ScValTypeScvBool:
		return cmpOpt(a.B, b.B, func(x, y bool) int { return cmp.Compare(boolInt(x), boolInt(y)) })
	case xdr.ScValTypeScvVoid, xdr.ScValTypeScvLedgerKeyContractInstance:
		return 0
	case xdr.ScValTypeScvError:
		return cmpOpt(a.Error, b.Error, compareScError)
	case xdr.ScValTypeScvU32:
		return cmpOpt(a.U32, b.U32, func(x, y xdr.Uint32) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvI32:
		return cmpOpt(a.I32, b.I32, func(x, y xdr.Int32) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvU64:
		return cmpOpt(a.U64, b.U64, func(x, y xdr.Uint64) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvI64:
		return cmpOpt(a.I64, b.I64, func(x, y xdr.Int64) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvTimepoint:
		return cmpOpt(a.Timepoint, b.Timepoint, func(x, y xdr.TimePoint) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvDuration:
		return cmpOpt(a.Duration, b.Duration, func(x, y xdr.Duration) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvU128:
		return cmpOpt(a.U128, b.U128, compareU128)
	case xdr.ScValTypeScvI128:
		return cmpOpt(a.I128, b.I128, compareI128)
	case xdr.ScValTypeScvU256:
		return cmpOpt(a.U256, b.U256, compareU256)
	case xdr.ScValTypeScvI256:
		return cmpOpt(a.I256, b.I256, compareI256)
	case xdr.ScValTypeScvBytes:
		return cmpOpt(a.Bytes, b.Bytes, func(x, y xdr.ScBytes) int { return bytes.Compare(x, y) })
	case xdr.ScValTypeScvString:
		return cmpOpt(a.Str, b.Str, func(x, y xdr.ScString) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvSymbol:
		return cmpOpt(a.Sym, b.Sym, func(x, y xdr.ScSymbol) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvExecutableTag:
		return cmpOpt(a.ExecutableTag, b.ExecutableTag, func(x, y xdr.ScString) int { return cmp.Compare(x, y) })
	case xdr.ScValTypeScvVec:
		return cmpOpt(a.Vec, b.Vec, compareScVec)
	case xdr.ScValTypeScvMap:
		return cmpOpt(a.Map, b.Map, compareScMap)
	case xdr.ScValTypeScvAddress:
		return cmpOpt(a.Address, b.Address, compareScAddress)
	case xdr.ScValTypeScvContractInstance:
		return cmpOpt(a.Instance, b.Instance, compareContractInstance)
	case xdr.ScValTypeScvLedgerKeyNonce:
		return cmpOpt(a.NonceKey, b.NonceKey, func(x, y xdr.ScNonceKey) int { return cmp.Compare(x.Nonce, y.Nonce) })
	default:
		// Unknown future type: fall back to a deterministic byte ordering so
		// maps stay sortable even if this library predates the ScVal variant.
		ax, errA := a.MarshalBinary()
		bx, errB := b.MarshalBinary()
		if errA != nil || errB != nil {
			return boolInt(errA != nil) - boolInt(errB != nil)
		}
		return bytes.Compare(ax, bx)
	}
}

// cmpOpt dereferences an optional union arm: nil sorts before non-nil, two
// nils are equal. A nil arm is malformed XDR but sorting must not panic.
func cmpOpt[T any](a, b *T, cmpFn func(T, T) int) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		return cmpFn(*a, *b)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func compareScError(a, b xdr.ScError) int {
	if a.Type != b.Type {
		return cmp.Compare(a.Type, b.Type)
	}
	if a.Type == xdr.ScErrorTypeSceContract {
		return cmpOpt(a.ContractCode, b.ContractCode, func(x, y xdr.Uint32) int { return cmp.Compare(x, y) })
	}
	return cmpOpt(a.Code, b.Code, func(x, y xdr.ScErrorCode) int { return cmp.Compare(x, y) })
}

func compareU128(a, b xdr.UInt128Parts) int {
	if c := cmp.Compare(a.Hi, b.Hi); c != 0 {
		return c
	}
	return cmp.Compare(a.Lo, b.Lo)
}

func compareI128(a, b xdr.Int128Parts) int {
	if c := cmp.Compare(a.Hi, b.Hi); c != 0 {
		return c
	}
	return cmp.Compare(a.Lo, b.Lo)
}

func compareU256(a, b xdr.UInt256Parts) int {
	if c := cmp.Compare(a.HiHi, b.HiHi); c != 0 {
		return c
	}
	if c := cmp.Compare(a.HiLo, b.HiLo); c != 0 {
		return c
	}
	if c := cmp.Compare(a.LoHi, b.LoHi); c != 0 {
		return c
	}
	return cmp.Compare(a.LoLo, b.LoLo)
}

func compareI256(a, b xdr.Int256Parts) int {
	if c := cmp.Compare(a.HiHi, b.HiHi); c != 0 {
		return c
	}
	if c := cmp.Compare(a.HiLo, b.HiLo); c != 0 {
		return c
	}
	if c := cmp.Compare(a.LoHi, b.LoHi); c != 0 {
		return c
	}
	return cmp.Compare(a.LoLo, b.LoLo)
}

func compareScVec(a, b *xdr.ScVec) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	for i := 0; i < len(*a) && i < len(*b); i++ {
		if c := CompareScVal((*a)[i], (*b)[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(*a), len(*b))
}

func compareScMap(a, b *xdr.ScMap) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	for i := 0; i < len(*a) && i < len(*b); i++ {
		if c := CompareScVal((*a)[i].Key, (*b)[i].Key); c != 0 {
			return c
		}
		if c := CompareScVal((*a)[i].Val, (*b)[i].Val); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(*a), len(*b))
}

func compareScAddress(a, b xdr.ScAddress) int {
	if a.Type != b.Type {
		return cmp.Compare(a.Type, b.Type)
	}
	switch a.Type {
	case xdr.ScAddressTypeScAddressTypeAccount:
		return cmpOpt(a.AccountId, b.AccountId, compareAccountID)
	case xdr.ScAddressTypeScAddressTypeContract:
		return cmpOpt(a.ContractId, b.ContractId, func(x, y xdr.ContractId) int { return bytes.Compare(x[:], y[:]) })
	case xdr.ScAddressTypeScAddressTypeMuxedAccount:
		return cmpOpt(a.MuxedAccount, b.MuxedAccount, func(x, y xdr.MuxedEd25519Account) int {
			if c := cmp.Compare(x.Id, y.Id); c != 0 {
				return c
			}
			return bytes.Compare(x.Ed25519[:], y.Ed25519[:])
		})
	case xdr.ScAddressTypeScAddressTypeClaimableBalance:
		return cmpOpt(a.ClaimableBalanceId, b.ClaimableBalanceId, func(x, y xdr.ClaimableBalanceId) int {
			if x.Type != y.Type {
				return cmp.Compare(x.Type, y.Type)
			}
			return cmpOpt(x.V0, y.V0, func(p, q xdr.Hash) int { return bytes.Compare(p[:], q[:]) })
		})
	case xdr.ScAddressTypeScAddressTypeLiquidityPool:
		return cmpOpt(a.LiquidityPoolId, b.LiquidityPoolId, func(x, y xdr.PoolId) int { return bytes.Compare(x[:], y[:]) })
	default:
		return 0
	}
}

func compareAccountID(a, b xdr.AccountId) int {
	if a.Type != b.Type {
		return cmp.Compare(a.Type, b.Type)
	}
	return cmpOpt(a.Ed25519, b.Ed25519, func(x, y xdr.Uint256) int { return bytes.Compare(x[:], y[:]) })
}

func compareContractInstance(a, b xdr.ScContractInstance) int {
	if c := compareContractExecutable(a.Executable, b.Executable); c != 0 {
		return c
	}
	return compareScMap(a.Storage, b.Storage)
}

func compareContractExecutable(a, b xdr.ContractExecutable) int {
	if a.Type != b.Type {
		return cmp.Compare(a.Type, b.Type)
	}
	switch a.Type {
	case xdr.ContractExecutableTypeContractExecutableWasm:
		return cmpOpt(a.WasmHash, b.WasmHash, func(x, y xdr.Hash) int { return bytes.Compare(x[:], y[:]) })
	case xdr.ContractExecutableTypeContractExecutableStellarAsset:
		return 0
	case xdr.ContractExecutableTypeContractExecutableExternalRef:
		return cmpOpt(a.ExternalRef, b.ExternalRef, func(x, y xdr.ContractExecutableExternalRef) int {
			if c := compareScAddress(x.ExecutableOwner, y.ExecutableOwner); c != 0 {
				return c
			}
			return cmp.Compare(x.Tag, y.Tag)
		})
	default:
		return 0
	}
}

var (
	scValType     = reflect.TypeOf(xdr.ScVal{})
	scAddressType = reflect.TypeOf(xdr.ScAddress{})
	scMapType     = reflect.TypeOf(xdr.ScMap{})
	scSymbolType  = reflect.TypeOf(xdr.ScSymbol(""))
	scStringType  = reflect.TypeOf(xdr.ScString(""))
	scBytesType   = reflect.TypeOf(xdr.ScBytes{})
	bigIntType    = reflect.TypeOf(big.Int{})
	bigIntPtrType = reflect.TypeOf(&big.Int{})
)

// ToScVal encodes a Go value into an xdr.ScVal.
//
// Type mapping:
//
//	xdr.ScVal                   -> passed through unchanged
//	xdr.ScAddress               -> ScvAddress
//	xdr.ScSymbol / ScString     -> ScvSymbol / ScvString
//	xdr.ScBytes / []byte        -> ScvBytes
//	big.Int / *big.Int          -> ScvI128
//	bool                        -> ScvBool
//	string                      -> ScvString (ScvSymbol with the `,symbol` tag option)
//	int8, int16, int32          -> ScvI32
//	int, int64                  -> ScvI64
//	uint8, uint16, uint32       -> ScvU32
//	uint, uint64                -> ScvU64
//	slice, array                -> ScvVec
//	map                         -> ScvMap (keys sorted canonically)
//	struct                      -> ScvMap with ScvSymbol keys
//	pointer                     -> the pointee, or ScvVoid when nil
//
// Struct fields are encoded with a ScvSymbol key taken from the
// `soroban:"name"` tag, or the field name when no tag is present —
// matching how #[contracttype] structs serialize in Rust. `soroban:"-"`
// skips a field. A string field tagged `soroban:"name,symbol"` encodes as
// ScvSymbol instead of ScvString. Map keys are sorted with CompareScVal,
// the ordering the Soroban host requires, so the XDR produced here is
// identical to what soroban-sdk emits from Rust.
func ToScVal(v any) (xdr.ScVal, error) {
	return encodeReflectValue(reflect.ValueOf(v), "")
}

func encodeReflectValue(rv reflect.Value, fieldName string) (xdr.ScVal, error) {
	if !rv.IsValid() {
		return xdr.ScVal{}, fmt.Errorf("%s: cannot encode nil value", fieldName)
	}
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return xdr.ScVal{}, fmt.Errorf("%s: cannot encode nil value", fieldName)
		}
		return encodeReflectValue(rv.Elem(), fieldName)
	}

	switch rv.Type() {
	case scValType:
		return rv.Interface().(xdr.ScVal), nil
	case scAddressType:
		return ScvAddress(rv.Interface().(xdr.ScAddress)), nil
	case scMapType:
		entries := slices.Clone(rv.Interface().(xdr.ScMap))
		if err := sortScMapEntries(entries); err != nil {
			return xdr.ScVal{}, fmt.Errorf("%s: %w", fieldName, err)
		}
		return ScvMap(entries...), nil
	case scSymbolType:
		return ScvSymbol(rv.String()), nil
	case scStringType:
		return ScvString(rv.String()), nil
	case scBytesType:
		return ScvBytes(rv.Bytes()), nil
	case bigIntType:
		n := rv.Interface().(big.Int)
		return ScvI128(&n), nil
	}

	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), nil
		}
		if rv.Type() == bigIntPtrType {
			return ScvI128(rv.Interface().(*big.Int)), nil
		}
		return encodeReflectValue(rv.Elem(), fieldName)
	case reflect.Bool:
		return ScvBool(rv.Bool()), nil
	case reflect.String:
		return ScvString(rv.String()), nil
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return ScvI32(int32(rv.Int())), nil
	case reflect.Int, reflect.Int64:
		return ScvI64(rv.Int()), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return ScvU32(uint32(rv.Uint())), nil
	case reflect.Uint, reflect.Uint64:
		return ScvU64(rv.Uint()), nil
	case reflect.Slice, reflect.Array:
		return encodeSlice(rv, fieldName)
	case reflect.Map:
		return encodeMap(rv, fieldName)
	case reflect.Struct:
		return encodeStruct(rv, fieldName)
	default:
		return xdr.ScVal{}, fmt.Errorf("%s: unsupported Go type %s for ScVal encoding", fieldName, rv.Type())
	}
}

func encodeSlice(rv reflect.Value, fieldName string) (xdr.ScVal, error) {
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		buf := make([]byte, rv.Len())
		for i := range buf {
			buf[i] = byte(rv.Index(i).Uint())
		}
		return ScvBytes(buf), nil
	}
	elemName := fieldName + "[]"
	vals := make([]xdr.ScVal, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		v, err := encodeReflectValue(rv.Index(i), elemName)
		if err != nil {
			return xdr.ScVal{}, err
		}
		vals = append(vals, v)
	}
	return ScvVec(vals...), nil
}

func encodeMap(rv reflect.Value, fieldName string) (xdr.ScVal, error) {
	entries := make([]xdr.ScMapEntry, 0, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		key, err := encodeReflectValue(iter.Key(), fieldName+"<key>")
		if err != nil {
			return xdr.ScVal{}, err
		}
		val, err := encodeReflectValue(iter.Value(), fieldName+"<val>")
		if err != nil {
			return xdr.ScVal{}, err
		}
		entries = append(entries, ScvPair(key, val))
	}
	if err := sortScMapEntries(entries); err != nil {
		return xdr.ScVal{}, fmt.Errorf("%s: %w", fieldName, err)
	}
	return ScvMap(entries...), nil
}

func encodeStruct(rv reflect.Value, fieldName string) (xdr.ScVal, error) {
	rt := rv.Type()
	entries := make([]xdr.ScMapEntry, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		name, asSymbol, skip := parseSorobanTag(field)
		if skip {
			continue
		}
		if name == "" {
			name = field.Name
		}
		child := name
		if fieldName != "" {
			child = fieldName + "." + name
		}
		fv := rv.Field(i)
		val, err := encodeReflectValue(fv, child)
		if err != nil {
			return xdr.ScVal{}, err
		}
		if asSymbol {
			if fv.Kind() != reflect.String {
				return xdr.ScVal{}, fmt.Errorf("%s: `,symbol` requires a string field, got %s", child, fv.Type())
			}
			val = ScvSymbol(fv.String())
		}
		entries = append(entries, ScvPair(ScvSymbol(name), val))
	}
	if err := sortScMapEntries(entries); err != nil {
		return xdr.ScVal{}, fmt.Errorf("%s: %w", fieldName, err)
	}
	return ScvMap(entries...), nil
}

// parseSorobanTag reads the `soroban` struct tag. The first comma-separated
// part is the map key; "symbol" marks a string field for ScvSymbol encoding;
// "-" skips the field entirely.
func parseSorobanTag(field reflect.StructField) (name string, asSymbol, skip bool) {
	tag, ok := field.Tag.Lookup("soroban")
	if !ok {
		return "", false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	for _, opt := range parts[1:] {
		if opt == "symbol" {
			asSymbol = true
		}
	}
	return parts[0], asSymbol, false
}
