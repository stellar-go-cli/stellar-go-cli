// fuzz_scval_codec_test.go — test-only serde-JSON ↔ xdr.ScVal codec matching
// stellar-cli's JSON representation, plus a deterministic structure-aware
// ScVal generator. The Go SDK's xdr.ScVal has no serde codec, so the
// differential harness needs its own for the generated subset.
package soroban

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// ─── generator: fuzz bytes → deterministic ScVal ──────────────────────────

type genDec struct {
	data []byte
	pos  int
}

func (d *genDec) next() byte {
	if len(d.data) == 0 {
		d.pos++
		return byte(d.pos*31 + 7)
	}
	b := d.data[d.pos%len(d.data)]
	d.pos++
	return b
}

func (d *genDec) n(n int) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		v = v<<8 | uint64(d.next())
	}
	return v
}

// genScVal produces a deterministic ScVal + its serde-JSON form, depth-bounded.
// Variant coverage is restricted to what the codec round-trips.
func genScVal(d *genDec, depth int) (xdr.ScVal, interface{}) {
	variant := int(d.next()) % 12
	if depth <= 0 {
		variant %= 10 // no vec/map recursion at the leaf bound
	}
	switch variant {
	case 0:
		b := d.next()%2 == 0
		return ScvBool(b), map[string]interface{}{"bool": b}
	case 1:
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), "void"
	case 2:
		v := xdr.Uint32(d.n(4))
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvU32, v)), map[string]interface{}{"u32": v}
	case 3:
		v := xdr.Int32(int32(d.n(4)))
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvI32, v)), map[string]interface{}{"i32": v}
	case 4:
		v := xdr.Uint64(d.n(8))
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvU64, v)), map[string]interface{}{"u64": strconv.FormatUint(uint64(v), 10)}
	case 5:
		v := xdr.Int64(int64(d.n(8)))
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvI64, v)), map[string]interface{}{"i64": strconv.FormatInt(int64(v), 10)}
	case 6:
		v := xdr.TimePoint(d.n(8))
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvTimepoint, v)), map[string]interface{}{"timepoint": strconv.FormatUint(uint64(v), 10)}
	case 7:
		n := int(d.next())%16 + 1
		buf := make([]byte, n)
		for i := range buf {
			buf[i] = d.next()
		}
		return ScvBytes(buf), map[string]interface{}{"bytes": hex.EncodeToString(buf)}
	case 8:
		s := fmt.Sprintf("s%x", d.n(4))
		return ScvString(s), map[string]interface{}{"string": s}
	case 9:
		s := fmt.Sprintf("sym%x", d.n(4))
		return ScvSymbol(s), map[string]interface{}{"symbol": s}
	case 10:
		cnt := int(d.next())%4 + 1
		vec := make([]xdr.ScVal, cnt)
		jv := make([]interface{}, cnt)
		for i := 0; i < cnt; i++ {
			vec[i], jv[i] = genScVal(d, depth-1)
		}
		return ScvVec(vec...), map[string]interface{}{"vec": jv}
	default: // 11 — map; keys must stay unique + sorted for ScvMap
		cnt := int(d.next())%4 + 1
		entries := make([]xdr.ScMapEntry, cnt)
		jm := make([]interface{}, cnt)
		for i := 0; i < cnt; i++ {
			k, jk := genScVal(d, 0)
			v, jvv := genScVal(d, depth-1)
			entries[i] = xdr.ScMapEntry{Key: k, Val: v}
			jm[i] = map[string]interface{}{"key": jk, "val": jvv}
		}
		sv, err := tryScvMap(entries)
		if err != nil { // duplicate keys — fall back to a vec of the values
			vec := make([]xdr.ScVal, cnt)
			jv := make([]interface{}, cnt)
			for i, e := range entries {
				vec[i] = e.Val
				jv[i] = jm[i].(map[string]interface{})["val"]
			}
			return ScvVec(vec...), map[string]interface{}{"vec": jv}
		}
		// ScvMap sorted the entries — rebuild the JSON in the same order.
		for i, e := range **sv.Map {
			jk, _ := scvalToOracleJSON(e.Key)
			jvv, _ := scvalToOracleJSON(e.Val)
			jm[i] = map[string]interface{}{"key": jk, "val": jvv}
		}
		return sv, map[string]interface{}{"map": jm}
	}
}

// tryScvMap is ScvMap without the panic on duplicate keys.
func tryScvMap(entries []xdr.ScMapEntry) (xdr.ScVal, error) {
	if err := sortScMapEntries(entries); err != nil {
		return xdr.ScVal{}, err
	}
	m := xdr.ScMap(entries)
	return xdr.NewScVal(xdr.ScValTypeScvMap, &m)
}

// ─── codec: serde JSON → xdr.ScVal ────────────────────────────────────────

func scvalFromOracleJSON(v interface{}) (xdr.ScVal, error) {
	if s, ok := v.(string); ok && s == "void" {
		return mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), nil
	}
	m, ok := v.(map[string]interface{})
	if !ok || len(m) != 1 {
		return xdr.ScVal{}, fmt.Errorf("unsupported scval json: %v", v)
	}
	for k, val := range m {
		switch k {
		case "bool":
			return ScvBool(val == true || val == "true"), nil
		case "void":
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvVoid, nil)), nil
		case "u32":
			n, err := jsonUint(val, 32)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvU32, xdr.Uint32(n))), err
		case "i32":
			n, err := jsonInt(val, 32)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvI32, xdr.Int32(n))), err
		case "u64":
			n, err := jsonUint(val, 64)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvU64, xdr.Uint64(n))), err
		case "i64":
			n, err := jsonInt(val, 64)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvI64, xdr.Int64(n))), err
		case "timepoint":
			n, err := jsonUint(val, 64)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvTimepoint, xdr.TimePoint(n))), err
		case "duration":
			n, err := jsonUint(val, 64)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvDuration, xdr.Duration(n))), err
		case "u128", "i128":
			bi, err := jsonBigInt(val)
			if err != nil {
				return xdr.ScVal{}, err
			}
			if k == "u128" {
				mask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1))
				lo := new(big.Int).And(bi, mask)
				hi := new(big.Int).Rsh(new(big.Int).Set(bi), 64)
				return mustScVal(xdr.NewScVal(xdr.ScValTypeScvU128,
					xdr.UInt128Parts{Hi: xdr.Uint64(hi.Uint64()), Lo: xdr.Uint64(lo.Uint64())})), nil
			}
			return ScvI128(bi), nil
		case "bytes":
			raw, err := hex.DecodeString(fmt.Sprint(val))
			return ScvBytes(raw), err
		case "string":
			return ScvString(fmt.Sprint(val)), nil
		case "symbol":
			return ScvSymbol(fmt.Sprint(val)), nil
		case "vec":
			arr, ok := val.([]interface{})
			if !ok {
				return xdr.ScVal{}, fmt.Errorf("vec not an array")
			}
			vec := make([]xdr.ScVal, len(arr))
			for i, e := range arr {
				sv, err := scvalFromOracleJSON(e)
				if err != nil {
					return xdr.ScVal{}, err
				}
				vec[i] = sv
			}
			return ScvVec(vec...), nil
		case "map":
			arr, ok := val.([]interface{})
			if !ok {
				return xdr.ScVal{}, fmt.Errorf("map not an array")
			}
			entries := make([]xdr.ScMapEntry, len(arr))
			for i, e := range arr {
				em, ok := e.(map[string]interface{})
				if !ok {
					return xdr.ScVal{}, fmt.Errorf("map entry not an object")
				}
				kk, err := scvalFromOracleJSON(em["key"])
				if err != nil {
					return xdr.ScVal{}, err
				}
				vv, err := scvalFromOracleJSON(em["val"])
				if err != nil {
					return xdr.ScVal{}, err
				}
				entries[i] = xdr.ScMapEntry{Key: kk, Val: vv}
			}
			sv, err := tryScvMap(entries)
			if err != nil {
				return xdr.ScVal{}, err
			}
			return sv, nil
		case "address":
			return scAddressFromStrkey(fmt.Sprint(val))
		case "error":
			em, _ := val.(map[string]interface{})
			code, err := jsonUint(em["contract"], 32)
			if err != nil {
				return xdr.ScVal{}, fmt.Errorf("error variant: %w", err)
			}
			c := xdr.Uint32(code)
			return mustScVal(xdr.NewScVal(xdr.ScValTypeScvError,
				xdr.ScError{Type: xdr.ScErrorTypeSceContract, ContractCode: &c})), nil
		}
		return xdr.ScVal{}, fmt.Errorf("unsupported scval variant %q", k)
	}
	return xdr.ScVal{}, fmt.Errorf("empty scval json")
}

// ─── codec: xdr.ScVal → serde JSON ────────────────────────────────────────

func scvalToOracleJSON(v xdr.ScVal) (interface{}, error) {
	switch v.Type {
	case xdr.ScValTypeScvBool:
		return map[string]interface{}{"bool": *v.B}, nil
	case xdr.ScValTypeScvVoid:
		return "void", nil
	case xdr.ScValTypeScvU32:
		return map[string]interface{}{"u32": *v.U32}, nil
	case xdr.ScValTypeScvI32:
		return map[string]interface{}{"i32": *v.I32}, nil
	case xdr.ScValTypeScvU64:
		return map[string]interface{}{"u64": strconv.FormatUint(uint64(*v.U64), 10)}, nil
	case xdr.ScValTypeScvI64:
		return map[string]interface{}{"i64": strconv.FormatInt(int64(*v.I64), 10)}, nil
	case xdr.ScValTypeScvTimepoint:
		return map[string]interface{}{"timepoint": strconv.FormatUint(uint64(*v.Timepoint), 10)}, nil
	case xdr.ScValTypeScvDuration:
		return map[string]interface{}{"duration": strconv.FormatUint(uint64(*v.Duration), 10)}, nil
	case xdr.ScValTypeScvU128:
		return map[string]interface{}{"u128": u128String(*v.U128)}, nil
	case xdr.ScValTypeScvI128:
		return map[string]interface{}{"i128": i128String(*v.I128)}, nil
	case xdr.ScValTypeScvBytes:
		return map[string]interface{}{"bytes": hex.EncodeToString(*v.Bytes)}, nil
	case xdr.ScValTypeScvString:
		return map[string]interface{}{"string": string(*v.Str)}, nil
	case xdr.ScValTypeScvSymbol:
		return map[string]interface{}{"symbol": string(*v.Sym)}, nil
	case xdr.ScValTypeScvVec:
		arr := make([]interface{}, len(**v.Vec))
		for i, e := range **v.Vec {
			jv, err := scvalToOracleJSON(e)
			if err != nil {
				return nil, err
			}
			arr[i] = jv
		}
		return map[string]interface{}{"vec": arr}, nil
	case xdr.ScValTypeScvMap:
		arr := make([]interface{}, len(**v.Map))
		for i, e := range **v.Map {
			k, err := scvalToOracleJSON(e.Key)
			if err != nil {
				return nil, err
			}
			vv, err := scvalToOracleJSON(e.Val)
			if err != nil {
				return nil, err
			}
			arr[i] = map[string]interface{}{"key": k, "val": vv}
		}
		return map[string]interface{}{"map": arr}, nil
	case xdr.ScValTypeScvAddress:
		s, err := DecodeScAddress(v)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"address": s}, nil
	case xdr.ScValTypeScvError:
		if v.Error.Type == xdr.ScErrorTypeSceContract && v.Error.ContractCode != nil {
			return map[string]interface{}{"error": map[string]interface{}{"contract": *v.Error.ContractCode}}, nil
		}
	}
	return nil, fmt.Errorf("unsupported scval type %v", v.Type)
}

// ─── helpers ──────────────────────────────────────────────────────────────

func scAddressFromStrkey(s string) (xdr.ScVal, error) {
	var addr xdr.ScAddress
	var err error
	if strings.HasPrefix(s, "C") {
		addr, err = ParseContractID(s)
	} else {
		addr, err = AccountToScAddress(s)
	}
	if err != nil {
		return xdr.ScVal{}, err
	}
	return ScvAddress(addr), nil
}

func jsonUint(v interface{}, bits int) (uint64, error) {
	switch n := v.(type) {
	case float64:
		return uint64(n), nil
	case string:
		return strconv.ParseUint(n, 10, bits)
	default:
		return 0, fmt.Errorf("not a number: %v", v)
	}
}

func jsonInt(v interface{}, bits int) (int64, error) {
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, bits)
	default:
		return 0, fmt.Errorf("not a number: %v", v)
	}
}

func jsonBigInt(v interface{}) (*big.Int, error) {
	bi, ok := new(big.Int).SetString(fmt.Sprint(v), 10)
	if !ok {
		return nil, fmt.Errorf("not a big int: %v", v)
	}
	return bi, nil
}

func u128String(p xdr.UInt128Parts) string {
	bi := new(big.Int).Lsh(new(big.Int).SetUint64(uint64(p.Hi)), 64)
	bi.Or(bi, new(big.Int).SetUint64(uint64(p.Lo)))
	return bi.String()
}

func i128String(p xdr.Int128Parts) string {
	bi := new(big.Int).Lsh(new(big.Int).SetInt64(int64(p.Hi)), 64)
	bi.Or(bi, new(big.Int).SetUint64(uint64(p.Lo)))
	mod := new(big.Int).Lsh(big.NewInt(1), 128)
	if bi.Cmp(new(big.Int).Lsh(big.NewInt(1), 127)) >= 0 {
		bi.Sub(bi, mod)
	}
	return bi.String()
}
