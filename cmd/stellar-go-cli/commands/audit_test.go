package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
)

func writeLines(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func kinds(events []map[string]string) map[string]int {
	out := map[string]int{}
	for _, e := range events {
		out[e["kind"]]++
	}
	return out
}

// §11.4 — grant use outside the expected session window.

func TestScanGrantOutsideWindow(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	p := writeLines(t, dir, "enforcement.log", []string{
		`{"ts":"` + ts + `","tool":"pay_send","result":"signed:abc","grant_age_s":4000,"checks":{}}`,
		`{"ts":"` + ts + `","tool":"pay_send","result":"signed:def","grant_age_s":60,"checks":{}}`,
	})
	events := scanEnforcementEvents(p, 30*time.Minute)
	k := kinds(events)
	if k["grant-outside-window"] != 1 {
		t.Fatalf("events: %v", events)
	}
}

// §11.4 — escalation rate-limited refusals get a distinct event kind.

func TestScanEscalationRateLimited(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	p := writeLines(t, dir, "enforcement.log", []string{
		`{"ts":"` + ts + `","tool":"pay_send","result":"refused:escalation rate limited — too many approval requests","checks":{"E3":"fail: escalation rate limited"}}`,
	})
	events := scanEnforcementEvents(p, 30*time.Minute)
	if kinds(events)["escalation-rate-limited"] != 1 {
		t.Fatalf("events: %v", events)
	}
}

// §11.4 — repeated attempts to reach removed/profile-excluded tools.

func TestScanExcludedToolProbing(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	var lines []string
	for i := 0; i < 4; i++ {
		lines = append(lines, `{"ts":"`+ts+`","tool":"wallet_export","outcome":"deny_unknown_tool"}`)
	}
	lines = append(lines, `{"ts":"`+ts+`","tool":"pay_send","outcome":"allow"}`)
	lines = append(lines, `{"ts":"`+ts+`","tool":"swap_quote","outcome":"deny_invalid_params"}`)
	p := writeLines(t, dir, "audit.log", lines)

	events := scanAuditEvents(p)
	k := kinds(events)
	if k["excluded-tool-probing"] != 1 {
		t.Fatalf("events: %v", events)
	}
	if !strings.Contains(events[0]["detail"], "wallet_export") {
		t.Fatalf("detail names wrong tool: %v", events[0])
	}
}

// §11.4 — a broken hash chain is itself an event.

func TestChainFailureEvents(t *testing.T) {
	dir := t.TempDir()

	// Valid chain — no event.
	var b strings.Builder
	w := chainlog.NewWriter(&b, "", 0)
	rec := struct {
		chainlog.Link
		K string `json:"k"`
	}{K: "v"}
	if _, err := w.Append(&rec, &rec.Link); err != nil {
		t.Fatal(err)
	}
	good := writeLines(t, dir, "good.log", []string{strings.TrimSpace(b.String())})
	if ev := chainFailureEvents(good); len(ev) != 0 {
		t.Fatalf("valid chain flagged: %v", ev)
	}

	// Corrupted chain — chain-failure event.
	bad := writeLines(t, dir, "bad.log", []string{
		`{"seq":2,"prev":"x","hash":"` + strings.Repeat("0", 64) + `","k":"v"}`,
	})
	ev := chainFailureEvents(bad)
	if len(ev) != 1 || ev[0]["kind"] != "chain-failure" {
		t.Fatalf("events: %v", ev)
	}
}
