// diff_corpus_test.go — Tier-1 canned-diff runner (docs/audit/
// differential-fuzzing.md): fixed artifacts under testdata/diff-corpus/ are
// replayed through both this Go implementation and the `stellar` oracle.
// Every artifact is a checked-in, human-reviewable case — fuzz crashes and
// hand-picked edge cases land here permanently.
package soroban

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// TestCorpusDifferential replays every artifact in testdata/diff-corpus:
//
//	scval-*.json   — serde JSON: our codec→XDR must byte-match `xdr encode`
//	strkey-*.txt   — strkey: Go vs `strkey decode` accept/reject + payload
//	tx-*.b64       — envelope: tx.HashHex must match `tx hash` (testnet)
func TestCorpusDifferential(t *testing.T) {
	stellarOracle(t)
	dir := "testdata/diff-corpus"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	ran := 0
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := strings.TrimSpace(string(data))
		switch {
		case strings.HasPrefix(e.Name(), "scval-") && strings.HasSuffix(e.Name(), ".json"):
			corpusScval(t, e.Name(), content)
			ran++
		case strings.HasPrefix(e.Name(), "strkey-") && strings.HasSuffix(e.Name(), ".txt"):
			corpusStrkey(t, e.Name(), content)
			ran++
		case strings.HasPrefix(e.Name(), "tx-") && strings.HasSuffix(e.Name(), ".b64"):
			corpusTx(t, e.Name(), content)
			ran++
		}
	}
	if ran == 0 {
		t.Fatal("no corpus artifacts replayed")
	}
	t.Logf("replayed %d corpus artifacts", ran)
}

func corpusScval(t *testing.T, name, doc string) {
	var jv interface{}
	if err := json.Unmarshal([]byte(doc), &jv); err != nil {
		t.Fatalf("%s: invalid corpus JSON: %v", name, err)
	}
	sv, err := scvalFromOracleJSON(jv)
	if err != nil {
		t.Fatalf("%s: codec rejected corpus value: %v", name, err)
	}
	ours, err := sv.MarshalBinary()
	if err != nil {
		t.Fatalf("%s: Go marshal failed: %v", name, err)
	}
	oracleB64, err := xdrEncode(t, "ScVal", doc)
	if err != nil {
		t.Fatalf("%s: oracle rejected corpus value: %v", name, err)
	}
	oracleXDR, err := base64.StdEncoding.DecodeString(oracleB64)
	if err != nil {
		t.Fatalf("%s: oracle output not base64: %v", name, err)
	}
	if !bytes.Equal(ours, oracleXDR) {
		t.Fatalf("%s: XDR divergence:\n  go:     %x\n  oracle: %x", name, ours, oracleXDR)
	}
	// Reverse direction: oracle decode of our bytes must equal codec output.
	oracleJSON, err := xdrDecode(t, "ScVal", base64.StdEncoding.EncodeToString(ours))
	if err != nil {
		t.Fatalf("%s: oracle decode of Go XDR failed: %v", name, err)
	}
	var ov interface{}
	if err := json.Unmarshal([]byte(oracleJSON), &ov); err != nil {
		t.Fatalf("%s: oracle JSON invalid: %v", name, err)
	}
	goJSON, err := scvalToOracleJSON(sv)
	if err != nil {
		t.Fatalf("%s: codec emit failed: %v", name, err)
	}
	if !deepJSONEqual(goJSON, ov) {
		gb, _ := json.Marshal(goJSON)
		t.Fatalf("%s: decode divergence:\n  go:     %s\n  oracle: %s", name, gb, oracleJSON)
	}
}

func corpusStrkey(t *testing.T, name, s string) {
	vb, goPayload, goErr := strkey.DecodeAny(s)
	oracle, oracleErr := strkeyDecode(t, s)
	switch {
	case goErr == nil && oracleErr != nil:
		t.Fatalf("%s: Go accepted, oracle rejected: %v", name, oracleErr)
	case goErr != nil && oracleErr == nil:
		t.Fatalf("%s: oracle accepted, Go rejected: %v", name, goErr)
	case goErr == nil && oracleErr == nil:
		var key string
		switch vb {
		case strkey.VersionByteAccountID:
			key = "public_key_ed25519"
		case strkey.VersionByteContract:
			key = "contract"
		default:
			return
		}
		oh, _ := oracle[key].(string)
		op, err := hex.DecodeString(oh)
		if err != nil || !bytes.Equal(op, goPayload) {
			t.Fatalf("%s: payload divergence: go=%x oracle=%s", name, goPayload, oh)
		}
	}
}

func corpusTx(t *testing.T, name, b64 string) {
	tx, err := txnbuild.TransactionFromXDR(b64)
	if err != nil {
		t.Fatalf("%s: Go envelope decode failed: %v", name, err)
	}
	gt, ok := tx.Transaction()
	if !ok {
		t.Fatalf("%s: not a transaction envelope", name)
	}
	goHash, err := gt.HashHex(network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("%s: Go hash failed: %v", name, err)
	}
	oracleHash, err := txHash(t, b64, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("%s: oracle rejected envelope: %v", name, err)
	}
	if goHash != oracleHash {
		t.Fatalf("%s: hash divergence: go=%s oracle=%s", name, goHash, oracleHash)
	}
	// Go re-encode must be byte-stable with the corpus artifact.
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("%s: corpus not base64: %v", name, err)
	}
	var env xdr.TransactionEnvelope
	if err := env.UnmarshalBinary(raw); err != nil {
		t.Fatalf("%s: xdr decode failed: %v", name, err)
	}
	if _, err := xdrDecode(t, "TransactionEnvelope", b64); err != nil {
		t.Fatalf("%s: oracle xdr decode rejected envelope: %v", name, err)
	}
}
