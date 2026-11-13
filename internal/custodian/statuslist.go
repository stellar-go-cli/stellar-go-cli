package custodian

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	vc2 "github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
)

// Status-list export (spec §11.2): each mandate issuer (wallet did:key) owns
// a revocation list and a suspension list, re-issued as signed
// BitstringStatusListCredentials on every status change. The local registry
// stays authoritative at decision time — these are the resolvable view an
// external verifier can fetch and check.
//
// Layout under state/statuslists/<safe-issuer>/:
//   index.json       — {indexes: {mandate_id: i}, next: N}
//   revocation.json  — signed BitstringStatusListCredential document
//   suspension.json  — signed BitstringStatusListCredential document

type statusListIndex struct {
	Indexes map[string]int `json:"indexes"`
	Next    int            `json:"next"`
}

func safeDID(did string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ':', '/', '\\':
			return '-'
		}
		return r
	}, did)
}

func (d *Daemon) statusDir(issuerDID string) string {
	return filepath.Join(d.stateDir, "statuslists", safeDID(issuerDID))
}

func (d *Daemon) loadStatusIndex(issuerDID string) (*statusListIndex, error) {
	path := filepath.Join(d.statusDir(issuerDID), "index.json")
	data, err := os.ReadFile(path) // #nosec G304 -- issuer-sanitized path inside status dir
	if err != nil {
		if os.IsNotExist(err) {
			return &statusListIndex{Indexes: map[string]int{}}, nil
		}
		return nil, err
	}
	var idx statusListIndex
	if uerr := json.Unmarshal(data, &idx); uerr != nil {
		return nil, uerr
	}
	if idx.Indexes == nil {
		idx.Indexes = map[string]int{}
	}
	return &idx, nil
}

func (d *Daemon) saveStatusIndex(issuerDID string, idx *statusListIndex) error {
	dir := d.statusDir(issuerDID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.json"), data, 0600)
}

// assignStatusIndex allocates the mandate's stable bit position for the
// issuer's lists — one index serves both purposes.
func (d *Daemon) assignStatusIndex(issuerDID, mandateID string) (int, error) {
	idx, err := d.loadStatusIndex(issuerDID)
	if err != nil {
		return -1, err
	}
	if i, ok := idx.Indexes[mandateID]; ok {
		return i, nil
	}
	i := idx.Next
	idx.Indexes[mandateID] = i
	idx.Next++
	return i, d.saveStatusIndex(issuerDID, idx)
}

// rekeyStatusIndex moves an index reservation from the provisional key used
// at issuance time to the final VC id.
func (d *Daemon) rekeyStatusIndex(issuerDID, oldKey, newKey string) {
	idx, err := d.loadStatusIndex(issuerDID)
	if err != nil {
		return
	}
	i, ok := idx.Indexes[oldKey]
	if !ok {
		return
	}
	delete(idx.Indexes, oldKey)
	idx.Indexes[newKey] = i
	if err := d.saveStatusIndex(issuerDID, idx); err != nil {
		d.logger.Printf("status index rekey failed: %v", err)
	}
}

// statusListID is the stable URN of a purpose list for an issuer.
func statusListID(issuerDID, purpose string) string {
	sum := []byte(issuerDID)
	var h [8]byte
	for i, b := range sum {
		h[i%8] ^= b
	}
	return fmt.Sprintf("urn:statuslist:%x:%s", h, purpose)
}

// statusEntries builds the credentialStatus array for a mandate VC — one
// entry per purpose list, per the W3C multi-status pattern.
func statusEntries(issuerDID string, index int) []interface{} {
	return []interface{}{
		vc2.StatusListEntry(statusListID(issuerDID, vc2.StatusPurposeRevocation), vc2.StatusPurposeRevocation, index),
		vc2.StatusListEntry(statusListID(issuerDID, vc2.StatusPurposeSuspension), vc2.StatusPurposeSuspension, index),
	}
}

// reissueStatusLists rebuilds and signs both purpose lists for the issuer
// from the current mandate registry. Seed is the issuer wallet's seed —
// resolved by the caller (grant/escrow path) since the lists are signed by
// the wallet's own DID.
func (d *Daemon) reissueStatusLists(issuerDID, seed string) error {
	idx, err := d.loadStatusIndex(issuerDID)
	if err != nil {
		return err
	}
	revoked, suspended := map[int]bool{}, map[int]bool{}
	for mandateID, i := range idx.Indexes {
		rec, lerr := d.mandates.load(mandateID)
		if lerr != nil {
			continue
		}
		switch rec.Status {
		case "revoked":
			revoked[i] = true
		case "suspended":
			suspended[i] = true
		}
	}

	svc, serr := vc2.NewServiceWithStellarSeed(seed)
	if serr != nil {
		return serr
	}
	dir := d.statusDir(issuerDID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, purpose := range []string{vc2.StatusPurposeRevocation, vc2.StatusPurposeSuspension} {
		set := revoked
		if purpose == vc2.StatusPurposeSuspension {
			set = suspended
		}
		subject := map[string]interface{}{
			"type":          "BitstringStatusList",
			"statusPurpose": purpose,
			"encodedList":   vc2.EncodeBitstring(set, 0),
		}
		listVC, ierr := svc.IssueVCWithExtras(issuerDID, "BitstringStatusListCredential", subject,
			time.Now().UTC().Add(24*time.Hour), map[string]interface{}{"id": statusListID(issuerDID, purpose)})
		if ierr != nil {
			return ierr
		}
		data, merr := json.Marshal(listVC)
		if merr != nil {
			return merr
		}
		if werr := os.WriteFile(filepath.Join(dir, purpose+".json"), data, 0600); werr != nil {
			return werr
		}
	}
	return nil
}

// exportStatusLists returns the signed list documents for an issuer —
// {revocation: <vc doc>, suspension: <vc doc>}.
func (d *Daemon) exportStatusLists(issuerDID string) (map[string]interface{}, error) {
	dir := d.statusDir(issuerDID)
	out := map[string]interface{}{}
	for _, purpose := range []string{vc2.StatusPurposeRevocation, vc2.StatusPurposeSuspension} {
		data, err := os.ReadFile(filepath.Join(dir, purpose+".json")) // #nosec G304 -- fixed names inside issuer dir
		if err != nil {
			continue
		}
		var doc map[string]interface{}
		if uerr := json.Unmarshal(data, &doc); uerr == nil {
			out[purpose] = doc
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no status list for issuer %s", issuerDID)
	}
	return out, nil
}
