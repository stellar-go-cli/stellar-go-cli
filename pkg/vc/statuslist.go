package vc

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// W3C Bitstring Status List support (spec §11.2 — a resolvable status
// mechanism for mandate revocation/suspension).
//
// Encoding: bit i of the bitstring is set when entry i has the status. The
// bitstring serializes little-endian-per-byte into a byte array which is
// gzip-compressed when that shrinks it, then base64url-encoded and prefixed
// with the multibase 'u' character — the `encodedList` form the status-list
// vocabulary defines.

// StatusListMinBits is the spec minimum bitstring size (16 KiB).
const StatusListMinBits = 131072

const (
	// StatusPurposeRevocation marks a permanently revoked entry.
	StatusPurposeRevocation = "revocation"
	// StatusPurposeSuspension marks a reversibly suspended entry.
	StatusPurposeSuspension = "suspension"
)

// EncodeBitstring renders a bit position set as the multibase encodedList
// form. size is rounded up to StatusListMinBits; gzip applies only when it
// shrinks the output.
func EncodeBitstring(set map[int]bool, size int) string {
	if size < StatusListMinBits {
		size = StatusListMinBits
	}
	bits := make([]byte, (size+7)/8)
	for idx, on := range set {
		if !on || idx < 0 {
			continue
		}
		if idx >= len(bits)*8 {
			bits = append(bits, make([]byte, idx/8-len(bits)+1)...)
		}
		bits[idx/8] |= 1 << (idx % 8)
	}
	payload := bits
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write(bits); err == nil && w.Close() == nil && gz.Len() < len(bits) {
		payload = gz.Bytes()
	}
	return "u" + base64.RawURLEncoding.EncodeToString(payload)
}

// DecodeBitstring decodes an encodedList value and reports whether idx is
// set. Multibase 'u' and bare base64url inputs are accepted; a leading gzip
// stream is inflated.
func DecodeBitstring(encodedList string, idx int) (bool, error) {
	s := strings.TrimPrefix(encodedList, "u")
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return false, fmt.Errorf("encodedList decode: %w", err)
	}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		r, gerr := gzip.NewReader(bytes.NewReader(raw))
		if gerr != nil {
			return false, fmt.Errorf("encodedList gzip: %w", gerr)
		}
		defer r.Close() //nolint:errcheck // read-only inflate
		inflated, rerr := io.ReadAll(io.LimitReader(r, 1<<20))
		if rerr != nil {
			return false, fmt.Errorf("encodedList inflate: %w", rerr)
		}
		raw = inflated
	}
	if idx < 0 || idx >= len(raw)*8 {
		return false, nil
	}
	return raw[idx/8]&(1<<(idx%8)) != 0, nil
}

// StatusListEntry builds a BitstringStatusListEntry credentialStatus object
// for embedding in a credential.
func StatusListEntry(listCredentialID, purpose string, index int) map[string]interface{} {
	return map[string]interface{}{
		"id":                   fmt.Sprintf("%s#%d", listCredentialID, index),
		"type":                 "BitstringStatusListEntry",
		"statusPurpose":        purpose,
		"statusListCredential": listCredentialID,
		"statusListIndex":      strconv.Itoa(index),
	}
}

// StatusEntryIndex parses the statusListIndex of a status entry — the spec
// allows string or number forms.
func StatusEntryIndex(entry map[string]interface{}) int {
	switch v := entry["statusListIndex"].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return -1
}
