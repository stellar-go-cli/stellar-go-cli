package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

// auditLogName is the append-only tools/call record under the operator state
// directory — the gatekeeper-side evidence log per the audit spec §12.
const auditLogName = "audit.log"

// auditRecord is one hash-chained JSONL entry. Argument values are never
// stored — only their SHA-256 digest and key names, so no secret material
// reaches the log.
type auditRecord struct {
	chainlog.Link
	Timestamp   string   `json:"ts"`
	Tool        string   `json:"tool"`
	Transport   string   `json:"transport"`
	Outcome     string   `json:"outcome"`
	DecisionRef string   `json:"decision_ref,omitempty"`
	ArgsSHA256  string   `json:"args_sha256,omitempty"`
	ArgKeys     []string `json:"arg_keys,omitempty"`
}

// openAuditRecorder opens state/audit.log (mode 0600, append) and resumes the
// hash chain from the last record already on disk.
func openAuditRecorder() (*chainlog.Recorder, string, error) {
	dir, err := config.StateDir()
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, auditLogName)
	rec, err := chainlog.Open(path)
	if err != nil {
		return nil, "", err
	}
	return rec, path, nil
}

// record appends one audit entry and returns its chain hash — the decision
// ref a signing call carries to the custodian for E5 linkage.
func recordAudit(r *chainlog.Recorder, tool, transport, outcome, decisionRef string, args map[string]interface{}) (string, error) {
	if r == nil {
		return "", nil
	}
	argsHash, argKeys := digestArgs(args)
	rec := auditRecord{
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		Tool:        tool,
		Transport:   transport,
		Outcome:     outcome,
		DecisionRef: decisionRef,
		ArgsSHA256:  argsHash,
		ArgKeys:     argKeys,
	}
	hash, err := r.Append(&rec, &rec.Link)
	if err != nil {
		return "", fmt.Errorf("write audit record: %w", err)
	}
	return hash, nil
}

// digestArgs hashes the canonical JSON of a call's arguments and returns the
// digest plus sorted key names — enough for forensics, never the values.
func digestArgs(args map[string]interface{}) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	canonical, err := json.Marshal(args) // map keys marshal sorted — deterministic
	if err != nil {
		return "", nil
	}
	sum := sha256.Sum256(canonical)
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return hex.EncodeToString(sum[:]), keys
}
