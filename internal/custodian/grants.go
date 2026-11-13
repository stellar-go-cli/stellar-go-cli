package custodian

import (
	"sync"
	"time"
)

// grant is an unlock grant held in custodian memory — replacing the OS
// keyring grant store (K4). Daemon restart or expiry relocks the wallet.
type grant struct {
	seed    string
	expires time.Time
	created time.Time
}

// grantStore holds decrypted seeds for passkey-unlocked wallets, in memory
// only. Nothing on disk or in the keyring exposes them.
type grantStore struct {
	mu     sync.Mutex
	grants map[string]grant
}

func newGrantStore() *grantStore {
	return &grantStore{grants: map[string]grant{}}
}

// put installs a grant. The seedBytes copy is owned by the store — callers
// must wipe their own copies.
func (g *grantStore) put(address, seed string, ttl time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.grants[address] = grant{seed: seed, expires: time.Now().Add(ttl), created: time.Now()}
}

// seed returns the live grant's seed, or "" if absent/expired (expired
// entries are purged eagerly).
func (g *grantStore) seed(address string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	gr, ok := g.grants[address]
	if !ok {
		return ""
	}
	if time.Now().After(gr.expires) {
		delete(g.grants, address)
		return ""
	}
	return gr.seed
}

// age reports how long the live grant has existed (§11.4 session-window
// signal); ok=false when no live grant is held.
func (g *grantStore) age(address string) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gr, ok := g.grants[address]
	if !ok || time.Now().After(gr.expires) {
		return 0, false
	}
	return time.Since(gr.created), true
}

// expiry reports the grant expiry for status calls.
func (g *grantStore) expiry(address string) (time.Time, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gr, ok := g.grants[address]
	if !ok || time.Now().After(gr.expires) {
		return time.Time{}, false
	}
	return gr.expires, true
}

// drop removes a grant (lock).
func (g *grantStore) drop(address string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.grants, address)
}

// approval is a single-use, short-lived bound approval created by a
// passkey ceremony whose challenge committed to the request hash (E3).
type approval struct {
	requestHash string
	expires     time.Time
}

// approvalStore holds pending bound approvals. Consuming an approval deletes
// it — each covers exactly one signing request.
type approvalStore struct {
	mu        sync.Mutex
	approvals map[string]approval
	ttl       time.Duration
}

func newApprovalStore(ttl time.Duration) *approvalStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &approvalStore{approvals: map[string]approval{}, ttl: ttl}
}

func (a *approvalStore) put(requestHash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.approvals[requestHash] = approval{requestHash: requestHash, expires: time.Now().Add(a.ttl)}
}

// consume removes and returns the approval for a request hash, if live.
func (a *approvalStore) consume(requestHash string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	ap, ok := a.approvals[requestHash]
	if !ok {
		return false
	}
	delete(a.approvals, requestHash)
	return time.Now().Before(ap.expires)
}

// seenSet tracks single-use decision refs (E5) — each decision may
// authorize exactly one sign.
type seenSet struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newSeenSet() *seenSet { return &seenSet{seen: map[string]time.Time{}} }

// claim marks a ref used; false means already used (replay).
func (s *seenSet) claim(ref string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[ref]; ok {
		return false
	}
	s.seen[ref] = time.Now()
	// Bound the map: drop entries older than a day.
	if len(s.seen) > 10000 {
		cutoff := time.Now().Add(-24 * time.Hour)
		for k, t := range s.seen {
			if t.Before(cutoff) {
				delete(s.seen, k)
			}
		}
	}
	return true
}

// release un-claims a ref when the request that claimed it refuses before
// delivery — a refused sign must not burn the operator's decision.
func (s *seenSet) release(ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, ref)
}
