// Package chainlog provides an append-only, hash-chained JSONL recorder —
// the shared evidence-record primitive for the gatekeeper audit log and the
// custodian enforcement log (audit spec §12). Each record embeds a Link whose
// hash commits to the previous record's hash, making deletion or reordering
// detectable.
package chainlog

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Link carries the chain fields every record embeds. Records that embed Link
// marshal seq/prev/hash inline alongside their own fields.
type Link struct {
	Seq  uint64 `json:"seq"`
	Prev string `json:"prev"`
	Hash string `json:"hash"`
}

// Recorder appends hash-chained records to an io.Writer. The chain head
// persists across restarts: on open, the last record's hash becomes the new
// chain's prev.
type Recorder struct {
	mu   sync.Mutex
	w    io.Writer
	seq  uint64
	prev string
}

// Open opens path (mode 0600, append, parent dir 0700) and resumes the hash
// chain from the last record already on disk.
func Open(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	prev, seq := LastHead(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) // #nosec G304 -- fixed filename inside the operator state dir
	if err != nil {
		return nil, err
	}
	return &Recorder{w: f, seq: seq, prev: prev}, nil
}

// NewWriter returns a recorder writing to w with the given chain head — for
// tests and in-memory verification.
func NewWriter(w io.Writer, prev string, seq uint64) *Recorder {
	return &Recorder{w: w, seq: seq, prev: prev}
}

// LastHead scans an existing log for the final record's hash and sequence so
// a reopened log continues the chain.
func LastHead(path string) (hash string, seq uint64) {
	f, err := os.Open(path) // #nosec G304 -- fixed filename inside the operator state dir
	if err != nil {
		return "", 0
	}
	defer f.Close() //nolint:errcheck // read-only scan

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Link
		}
		if json.Unmarshal(line, &rec) == nil && rec.Hash != "" {
			hash, seq = rec.Hash, rec.Seq
		}
	}
	return hash, seq
}

// Append assigns seq/prev/hash on link, then writes the marshaled record as
// one JSONL line. rec must be (or contain) the record whose link is passed —
// the link must be embedded so the marshaled line carries the chain fields.
// Write failures are reported to the caller, never silently dropped.
func (r *Recorder) Append(rec interface{}, link *Link) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	link.Seq = r.seq
	link.Prev = r.prev
	link.Hash = ""
	link.Hash = Compute(link.Prev, rec)
	r.prev = link.Hash

	data, err := json.Marshal(rec)
	if err != nil {
		return "", fmt.Errorf("marshal record: %w", err)
	}
	if _, err := r.w.Write(append(data, '\n')); err != nil {
		return "", fmt.Errorf("write record: %w", err)
	}
	return link.Hash, nil
}

// Compute returns the chain hash for a record whose Hash field is empty —
// sha256 over prev prefixed to the canonical record JSON. Exported so
// verifiers and tests can re-check chain integrity.
func Compute(prev string, rec interface{}) string {
	data, err := json.Marshal(rec)
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyFile replays a chain log and confirms seq/prev/hash integrity: each
// record's hash must equal Compute(prev, record) with the stored hash
// cleared, and each prev must equal the prior record's hash. Returns the
// record count.
func VerifyFile(path string) (int, error) {
	f, err := os.Open(path) // #nosec G304 -- fixed filename inside the operator state dir
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only scan

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	prev := ""
	var seq uint64
	count := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var rec struct {
			Seq  uint64 `json:"seq"`
			Prev string `json:"prev"`
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(raw, &rec); err != nil {
			return count, fmt.Errorf("line %d: unmarshal: %w", lineNo, err)
		}
		if rec.Seq != seq+1 {
			return count, fmt.Errorf("line %d: sequence gap: got %d, want %d", lineNo, rec.Seq, seq+1)
		}
		if rec.Prev != prev {
			return count, fmt.Errorf("line %d: prev mismatch", lineNo)
		}
		// Patch the stored hash out of the raw JSON — the original Compute
		// ran over the record with hash empty, so replaying the patched line
		// via RawMessage reproduces the exact marshaled bytes.
		var firstHash bool
		patched := hashFieldRe.ReplaceAllFunc(raw, func(b []byte) []byte {
			if firstHash {
				return b
			}
			firstHash = true
			return []byte(`"hash":""`)
		})
		if got := Compute(rec.Prev, json.RawMessage(patched)); got != rec.Hash {
			return count, fmt.Errorf("line %d: hash mismatch (record tampered or corrupt)", lineNo)
		}
		prev, seq = rec.Hash, rec.Seq
		count++
	}
	return count, scanner.Err()
}

var hashFieldRe = regexp.MustCompile(`"hash":"[0-9a-f]+"`)
