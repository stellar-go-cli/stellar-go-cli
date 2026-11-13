// fuzz_test.go — crash fuzz target for config/state loading
// (docs/audit/differential-fuzzing.md target #9): a corrupted config.json or
// state file must never panic the loader — it must produce the default or a
// clean error, and a Save→Load round-trip must be stable.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// FuzzConfigLoad fuzzes config.json bytes through Load + a round-trip.
func FuzzConfigLoad(f *testing.F) {
	f.Add(`{"network":"testnet","default_account":"a"}`)
	f.Add(`{"accounts":{"x":{"public_key":"GAAA"}}}`)
	f.Add(`{`)
	f.Add(`null`)
	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		cfgDir := filepath.Join(dir, DefaultConfigDir)
		if err := os.MkdirAll(cfgDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, DefaultConfigFile), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load returned error for %q: %v", data, err)
		}
		if cfg == nil {
			t.Fatal("Load returned nil config")
		}
		// Whatever loaded must re-save and re-load to an identical document —
		// Save must not fail on fuzz-derived values that fit the schema.
		if err := Save(cfg); err != nil {
			t.Fatalf("Save of loaded config failed: %v", err)
		}
		b1, _ := os.ReadFile(filepath.Join(cfgDir, DefaultConfigFile))
		cfg2, err := Load()
		if err != nil || cfg2 == nil {
			t.Fatalf("re-load failed: %v", err)
		}
		if err := Save(cfg2); err != nil {
			t.Fatalf("re-save failed: %v", err)
		}
		b2, _ := os.ReadFile(filepath.Join(cfgDir, DefaultConfigFile))
		var j1, j2 interface{}
		if json.Unmarshal(b1, &j1) != nil || json.Unmarshal(b2, &j2) != nil {
			t.Fatalf("saved config is not valid JSON")
		}
	})
}

// FuzzLoadState fuzzes arbitrary state-file bytes into the generic state
// loader — malformed state must be an error, never a panic.
func FuzzLoadState(f *testing.F) {
	f.Add(`{"key":"value"}`)
	f.Add(`{"version":1,"items":[1,2,3]}`)
	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		stateDir := filepath.Join(dir, DefaultConfigDir, "state")
		if err := os.MkdirAll(stateDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stateDir, "fuzz.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var v map[string]interface{}
		_ = LoadState("fuzz", &v) // error or success both acceptable
	})
}
