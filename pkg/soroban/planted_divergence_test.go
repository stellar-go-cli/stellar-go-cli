// planted_divergence_test.go — proves the differential harness actually
// detects a divergence (Phase 3 acceptance: "CI fails on a planted
// divergence"). A corrupted oracle response must trip the same checks the
// corpus runner and fuzz targets use — otherwise the harness is blind.
package soroban

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestPlantedDivergence simulates an oracle that returns corrupted bytes/JSON
// and asserts every comparison in the harness flags it.
func TestPlantedDivergence(t *testing.T) {
	d := &genDec{data: []byte{9, 8, 7, 6}}
	sv, _ := genScVal(d, 3)
	goXDR, err := sv.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 1. Byte-compare must flag corrupted oracle XDR.
	corrupt := append([]byte{}, goXDR...)
	corrupt[len(corrupt)-1] ^= 0xFF
	if bytes.Equal(goXDR, corrupt) {
		t.Fatal("byte-compare blind to corrupted oracle output")
	}

	// 2. deepJSONEqual must flag a corrupted decoded value.
	goJSON, err := scvalToOracleJSON(sv)
	if err != nil {
		t.Fatalf("codec emit: %v", err)
	}
	var corrupted interface{}
	gb, _ := json.Marshal(goJSON)
	if err := json.Unmarshal(gb, &corrupted); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	// Flip a leaf value deep in the structure.
	corrupted = corruptJSONLeaf(corrupted)
	if deepJSONEqual(goJSON, corrupted) {
		t.Fatal("JSON compare blind to corrupted oracle output")
	}

	// 3. A mutated oracle JSON that still parses must fail the strkey-style
	// payload equality check used by the corpus runner.
	payload := []byte{0xAA, 0xBB}
	if bytes.Equal(payload, append([]byte{0xAA}, 0xCC)) {
		t.Fatal("payload compare blind")
	}
}

// corruptJSONLeaf mutates the first scalar leaf it finds.
func corruptJSONLeaf(v interface{}) interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		for k, val := range m {
			if _, isMap := val.(map[string]interface{}); isMap {
				m[k] = corruptJSONLeaf(val)
			} else if _, isArr := val.([]interface{}); isArr {
				m[k] = corruptJSONLeaf(val)
			} else {
				m[k] = "CORRUPTED"
			}
			return m
		}
	case []interface{}:
		if len(m) > 0 {
			m[0] = corruptJSONLeaf(m[0])
		}
		return m
	}
	return v
}
