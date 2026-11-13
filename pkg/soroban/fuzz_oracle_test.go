// fuzz_oracle_test.go — the differential oracle adapter (docs/audit/
// differential-fuzzing.md Phase 2). `stellar` (the Rust reference CLI) is
// invoked as a subprocess: xdr encode/decode, strkey decode, tx hash/decode.
// Every oracle-using test skips cleanly when the binary is absent — local
// crash/property fuzz targets never depend on it.
package soroban

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

var (
	oracleOnce    sync.Once
	oraclePath    string
	oracleVersion string
)

// stellarOracle returns the path to the `stellar` binary or skips the test.
// The version is pinned in output — oracle behaviour is versioned behaviour.
func stellarOracle(t *testing.T) string {
	t.Helper()
	oracleOnce.Do(func() {
		p, err := exec.LookPath("stellar")
		if err == nil {
			oraclePath = p
			out, _ := exec.Command(p, "version").Output()
			oracleVersion = strings.TrimSpace(string(out))
		}
	})
	if oraclePath == "" {
		t.Skip("stellar CLI not installed — differential oracle unavailable")
	}
	t.Logf("oracle: %s", oracleVersion)
	return oraclePath
}

// runStellar executes stellar with args, feeding stdin, returning stdout.
func runStellar(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(stellarOracle(t), args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("stellar %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// xdrEncode converts serde JSON to base64 XDR via `stellar xdr encode`.
func xdrEncode(t *testing.T, xdrType, jsonDoc string) (string, error) {
	return runStellar(t, jsonDoc, "xdr", "encode", "--type", xdrType, "--output", "single-base64")
}

// xdrDecode converts base64 XDR to serde JSON via `stellar xdr decode`.
func xdrDecode(t *testing.T, xdrType, b64 string) (string, error) {
	return runStellar(t, b64, "xdr", "decode", "--type", xdrType, "--input", "single-base64")
}

// txHash computes the network-bound hash via `stellar tx hash`.
func txHash(t *testing.T, b64, passphrase string) (string, error) {
	return runStellar(t, b64, "tx", "hash", "--network-passphrase", passphrase)
}

// txDecode decodes an envelope to JSON via `stellar tx decode`.
func txDecode(t *testing.T, b64 string) (string, error) {
	return runStellar(t, b64, "tx", "decode")
}

// strkeyDecode parses a strkey via `stellar strkey decode` → JSON.
func strkeyDecode(t *testing.T, s string) (map[string]interface{}, error) {
	out, err := runStellar(t, "", "strkey", "decode", s)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &m); jerr != nil {
		return nil, fmt.Errorf("strkey decode JSON: %w", jerr)
	}
	return m, nil
}
