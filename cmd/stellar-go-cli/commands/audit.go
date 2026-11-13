package commands

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/custodian"
	"github.com/stellar-go-cli/stellar-go-cli/internal/reporting"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func newAuditCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "audit",
		Short: "Custodian audit & evidence (verify, events, latency, inventory, evidence)",
	}
	cmd.addSub(newAuditVerifyCmd(cfg))
	cmd.addSub(newAuditEventsCmd(cfg))
	cmd.addSub(newAuditLatencyCmd(cfg))
	cmd.addSub(newAuditInventoryCmd(cfg))
	cmd.addSub(newAuditEvidenceCmd(cfg))
	return cmd
}

// ── audit verify ─────────────────────────────────────────────────────────

func newAuditVerifyCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	log := fs.String("log", "", "Log path (default: both state logs)")
	return &Command{
		Name:  "verify",
		Short: "Verify hash-chain integrity of the audit + enforcement logs",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			paths := []string{filepath.Join(dir, "enforcement.log"), filepath.Join(dir, "audit.log")}
			if *log != "" {
				paths = []string{*log}
			}
			failed := false
			for _, p := range paths {
				n, verr := chainlog.VerifyFile(p)
				switch {
				case verr != nil:
					ui.Error(fmt.Sprintf("%s: CHAIN BROKEN — %v", filepath.Base(p), verr))
					failed = true
				case n == 0:
					ui.KV(filepath.Base(p), "empty or absent")
				default:
					ui.Success(fmt.Sprintf("%s: %d records, chain verifies", filepath.Base(p), n))
				}
			}
			if failed {
				return fmt.Errorf("audit chain verification failed")
			}
			return nil
		},
	}
}

// ── audit events ─────────────────────────────────────────────────────────

func newAuditEventsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("audit events", flag.ContinueOnError)
	log := fs.String("log", "", "Enforcement log path (default: state/enforcement.log)")
	auditLog := fs.String("audit-log", "", "Gatekeeper audit log path (default: state/audit.log)")
	window := fs.Duration("window", 30*time.Minute, "Expected session window — grant use older than this is an §11.4 signal")
	out := fs.String("out", "", "Write JSON report to file")
	return &Command{
		Name:  "events",
		Short: "Scan the enforcement log for §11.4 incident signals",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			path := *log
			if path == "" {
				path = filepath.Join(dir, "enforcement.log")
			}
			events := scanEnforcementEvents(path, *window)
			auditPath := *auditLog
			if auditPath == "" {
				auditPath = filepath.Join(dir, "audit.log")
			}
			events = append(events, scanAuditEvents(auditPath)...)
			events = append(events, chainFailureEvents(path, auditPath)...)
			if *out != "" {
				data, merr := json.MarshalIndent(events, "", "  ")
				if merr != nil {
					return merr
				}
				if werr := os.WriteFile(*out, data, 0600); werr != nil {
					return werr
				}
				ui.Success(fmt.Sprintf("%d event(s) → %s", len(events), *out))
				return nil
			}
			if len(events) == 0 {
				ui.Success("No incident signals in enforcement log")
				return nil
			}
			ui.SectionLabel(fmt.Sprintf("%d incident signal(s)", len(events)))
			for _, e := range events {
				ui.Warn(fmt.Sprintf("[%s] %s — %s", e["kind"], e["ts"], e["detail"]))
			}
			return nil
		},
	}
}

// scanEnforcementEvents walks the enforcement log and flags §11.4 signals:
// mandate verification failures, custodian refusals, denial bursts, and
// integrity anomalies.
func scanEnforcementEvents(path string, window time.Duration) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only scan

	var events []map[string]string
	var refusals []time.Time
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec struct {
			TS          string            `json:"ts"`
			Tool        string            `json:"tool"`
			Result      string            `json:"result"`
			Checks      map[string]string `json:"checks"`
			DecisionRef string            `json:"decision_ref"`
			GrantAgeS   int64             `json:"grant_age_s"`
		}
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		// §11.4 — grant use outside the expected session window. Grant age is
		// the implementable proxy for "session": a sign authorized by a grant
		// created long ago indicates re-use beyond one operator session.
		if window > 0 && rec.GrantAgeS > 0 && time.Duration(rec.GrantAgeS)*time.Second > window {
			events = append(events, map[string]string{
				"kind": "grant-outside-window", "ts": rec.TS,
				"detail": fmt.Sprintf("%s used a grant %ds old (window %s)", rec.Tool, rec.GrantAgeS, window),
			})
		}
		if len(rec.Result) > 7 && rec.Result[:7] == "refused" {
			if ts, perr := time.Parse(time.RFC3339Nano, rec.TS); perr == nil {
				refusals = append(refusals, ts)
			}
			kind := "refusal"
			detail := rec.Tool + ": " + rec.Result
			if strings.Contains(rec.Result, "rate limited") {
				kind = "escalation-rate-limited"
			}
			for k, v := range rec.Checks {
				if len(v) > 4 && v[:4] == "fail" && (k == "E2" || k == "E4") {
					kind = "verification-failure"
					detail = fmt.Sprintf("%s (%s: %s)", detail, k, v)
				}
			}
			events = append(events, map[string]string{"kind": kind, "ts": rec.TS, "detail": detail, "decision_ref": rec.DecisionRef})
		}
	}
	// Burst detection: ≥5 refusals inside any rolling 60s window.
	sort.Slice(refusals, func(i, j int) bool { return refusals[i].Before(refusals[j]) })
	for i := 0; i+4 < len(refusals); i++ {
		if refusals[i+4].Sub(refusals[i]) <= time.Minute {
			events = append(events, map[string]string{
				"kind": "denial-burst", "ts": refusals[i].Format(time.RFC3339),
				"detail": fmt.Sprintf("5 refusals in %s", refusals[i+4].Sub(refusals[i])),
			})
			break
		}
	}
	return events
}

// scanAuditEvents walks the gatekeeper's audit.log and flags §11.4
// gatekeeper-side signals: repeated attempts to reach removed or
// profile-excluded tools (deny_unknown_tool / deny_invalid_params).
func scanAuditEvents(path string) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only scan

	const probeThreshold = 3
	attempts := map[string]int{}
	lastTS := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec struct {
			TS      string `json:"ts"`
			Tool    string `json:"tool"`
			Outcome string `json:"outcome"`
		}
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		if rec.Outcome == "deny_unknown_tool" || rec.Outcome == "deny_invalid_params" {
			attempts[rec.Tool]++
			lastTS[rec.Tool] = rec.TS
		}
	}
	var events []map[string]string
	for tool, n := range attempts {
		if n >= probeThreshold {
			events = append(events, map[string]string{
				"kind": "excluded-tool-probing", "ts": lastTS[tool],
				"detail": fmt.Sprintf("%d denied attempts to reach %q (removed or profile-excluded)", n, tool),
			})
		}
	}
	return events
}

// chainFailureEvents verifies both hash chains up front — a broken chain is
// itself the highest-severity §11.4 signal.
func chainFailureEvents(paths ...string) []map[string]string {
	var events []map[string]string
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, verr := chainlog.VerifyFile(p); verr != nil {
			events = append(events, map[string]string{
				"kind": "chain-failure", "ts": time.Now().UTC().Format(time.RFC3339),
				"detail": fmt.Sprintf("%s: %v", filepath.Base(p), verr),
			})
		}
	}
	return events
}

// ── audit latency ────────────────────────────────────────────────────────

func newAuditLatencyCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("audit latency", flag.ContinueOnError)
	mandateRef := fs.String("mandate-ref", "", "Mandate VC id to suspend for the measurement")
	return &Command{
		Name:  "latency",
		Short: "Measure revoke→denial latency (spec §11.2 revocation-latency target)",
		Long: `Suspends the given mandate, then measures how long until the custodian
refuses a status check under it. The mandate is left SUSPENDED — reactivate
via 'stellar-go-cli wallet mandate' (bound ceremony).`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *mandateRef == "" {
				return fmt.Errorf("usage: audit latency --mandate-ref <vc-id>")
			}
			rw, err := custodianRemote()
			if err != nil {
				return err
			}
			t0 := time.Now()
			res, err := rw.SetMandateStatus(*mandateRef, "suspended", wallet.OpenCeremony)
			if err != nil {
				return err
			}
			if e, _ := res["error"].(string); e != "" {
				return fmt.Errorf("suspend failed: %s", e)
			}
			// Poll status until suspension is observable → revocation latency.
			var elapsed time.Duration
			deadline := time.Now().Add(10 * time.Second)
			for {
				list, lerr := rw.ListMandates("")
				if lerr != nil {
					return lerr
				}
				seen := false
				for _, m := range list {
					if m["id"] == *mandateRef && m["status"] == "suspended" {
						seen = true
					}
				}
				elapsed = time.Since(t0)
				if seen || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			ui.KV("Mandate", *mandateRef)
			ui.KV("Suspend→denial latency", elapsed.String())
			ui.Warn("Mandate left SUSPENDED — reactivate via 'wallet mandate' if needed")
			return nil
		},
	}
}

// ── audit inventory ──────────────────────────────────────────────────────

func newAuditInventoryCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("audit inventory", flag.ContinueOnError)
	out := fs.String("out", "", "Write JSON inventory to file")
	return &Command{
		Name:  "inventory",
		Short: "Scope inventory for the AIMS/ISMS (spec M1): wallets, mandates, policy",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			rw, err := custodianRemote()
			if err != nil {
				return err
			}
			wallets, active, err := rw.ListWallets()
			if err != nil {
				return err
			}
			mandates, err := rw.ListMandates("")
			if err != nil {
				return err
			}
			agents, aerr := rw.AgentList()
			if aerr != nil {
				ui.Warn("agent registry unavailable: " + aerr.Error())
			}
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			approved, last := custodian.ApprovedPolicy(dir)
			inv := map[string]interface{}{
				"generated_at":  time.Now().UTC().Format(time.RFC3339),
				"active_wallet": active,
				"wallets":       wallets,
				"mandates":      mandates,
				"agents":        agents,
			}
			if approved != nil {
				pol := map[string]interface{}{"version": approved.Version}
				if last != nil {
					pol["hash"] = last.Hash
					pol["approved_by"] = last.Approver
					pol["approved_at"] = last.ApprovedAt
				}
				inv["policy"] = pol
			}
			data, err := json.MarshalIndent(inv, "", "  ")
			if err != nil {
				return err
			}
			if *out != "" {
				if err := os.WriteFile(*out, data, 0600); err != nil {
					return err
				}
				ui.Success("Inventory → " + *out)
				return nil
			}
			fmt.Println(string(data))
			return nil
		},
	}
}

// ── audit evidence ───────────────────────────────────────────────────────

func newAuditEvidenceCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("audit evidence", flag.ContinueOnError)
	paymentFile := fs.String("payment", "", "Payment JSON file (models.Payment)")
	recordHash := fs.String("record", "", "Enforcement record hash to bind")
	log := fs.String("log", "", "Enforcement log path")
	out := fs.String("out", "", "Output XML file (default: stdout)")
	return &Command{
		Name:  "evidence",
		Short: "Export pacs.008 with PME evidence (InitgPty + SplmtryData, §12.3)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *paymentFile == "" {
				return fmt.Errorf("usage: audit evidence --payment payment.json [--record <hash>]")
			}
			raw, err := os.ReadFile(*paymentFile)
			if err != nil {
				return err
			}
			var payment models.Payment
			if err := json.Unmarshal(raw, &payment); err != nil {
				return fmt.Errorf("parse payment: %w", err)
			}
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			path := *log
			if path == "" {
				path = filepath.Join(dir, "enforcement.log")
			}
			ev := &iso20022.AgentEvidence{TxHash: payment.TxHash}
			rec := findEnforcementRecord(path, *recordHash, payment.TxHash)
			if rec != nil {
				ev.DecisionRef, _ = rec["decision_ref"].(string)
				ev.RecordHash, _ = rec["hash"].(string)
				if s, ok := rec["policy"].(string); ok {
					ev.PolicyStamp = s
				}
				if s, ok := rec["mandate_ref"].(string); ok {
					ev.MandateRef = s
				}
				if s, ok := rec["approval"].(string); ok {
					ev.ApprovalHash = s
				}
			}
			svc := reporting.NewService()
			xml, err := svc.FormatPacs008WithEvidence(&models.TransactionReport{Payment: &payment}, ev)
			if err != nil {
				return err
			}
			if *out != "" {
				return os.WriteFile(*out, []byte(xml), 0600)
			}
			fmt.Println(xml)
			return nil
		},
	}
}

// findEnforcementRecord scans the log for a record matching recordHash
// (chain hash) or, failing that, the record whose result names txHash.
func findEnforcementRecord(path, recordHash, txHash string) map[string]interface{} {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only scan
	var byHash, byTx map[string]interface{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		if h, _ := rec["hash"].(string); recordHash != "" && h == recordHash {
			byHash = rec
		}
		r, _ := rec["result"].(string)
		if txHash != "" && (r == txHash || strings.HasSuffix(r, ":"+txHash)) {
			byTx = rec
		}
	}
	if byHash != nil {
		return byHash
	}
	return byTx
}
