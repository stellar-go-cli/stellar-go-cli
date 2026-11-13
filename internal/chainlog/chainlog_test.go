package chainlog

import (
	"bytes"
	"strings"
	"testing"
)

type testRec struct {
	Link
	What string `json:"what"`
}

func TestChainIntegrity(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, "", 0)

	r1 := &testRec{What: "first"}
	if _, err := w.Append(r1, &r1.Link); err != nil {
		t.Fatal(err)
	}
	r2 := &testRec{What: "second"}
	if _, err := w.Append(r2, &r2.Link); err != nil {
		t.Fatal(err)
	}

	if r1.Seq != 1 || r2.Seq != 2 {
		t.Fatalf("seqs = %d,%d", r1.Seq, r2.Seq)
	}
	if r1.Prev != "" {
		t.Fatalf("first record prev = %q", r1.Prev)
	}
	if r2.Prev != r1.Hash {
		t.Fatal("chain broken: r2.prev != r1.hash")
	}
	// Recompute over the record with an empty Hash field — must match.
	verify := &testRec{Link: Link{Seq: r1.Seq, Prev: r1.Prev}, What: "first"}
	if Compute(r1.Prev, verify) != r1.Hash {
		t.Fatal("hash mismatch")
	}
}

func TestResumeFromTail(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, "", 0)
	r1 := &testRec{What: "first"}
	w.Append(r1, &r1.Link) //nolint:errcheck // test

	// New writer resuming from the last record's link.
	w2 := NewWriter(&buf, r1.Hash, r1.Seq)
	r2 := &testRec{What: "second"}
	if _, err := w2.Append(r2, &r2.Link); err != nil {
		t.Fatal(err)
	}
	if r2.Seq != 2 || r2.Prev != r1.Hash {
		t.Fatalf("resumed chain broken: seq=%d prev=%q", r2.Seq, r2.Prev)
	}
}

func TestAppendSerializes(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, "", 0)
	r := &testRec{What: "x"}
	if _, err := w.Append(r, &r.Link); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"seq":1`) || !strings.Contains(out, `"hash":"`) || !strings.HasSuffix(out, "\n") {
		t.Fatalf("record encoding: %q", out)
	}
}
