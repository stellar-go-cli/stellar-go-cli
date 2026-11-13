// pending.go — §10.3 escalation fatigue controls. Two mechanisms:
//
//   - escalationLimiter: sliding-window cap on escalation-request generation
//     per account. Escalation prompts are a principal-attention resource;
//     an agent spamming them is mounting a fatigue attack, so bursts are
//     refused (and surface as §11.4 incident signals via the record).
//   - pendingRequests: bounded map of E3-escalated requests keyed by request
//     hash. `wallet passkey approve` queries it (request_info) to show the
//     operator the decoded request — including the untrusted-content flag —
//     before the bound ceremony runs.
package custodian

import (
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
)

// pendingRequestTTL bounds how long an escalation stays queryable; the
// approval itself expires per the approvalStore TTL.
const pendingRequestTTL = 30 * time.Minute
const pendingRequestCap = 256

type escalationLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time // account → escalation timestamps
}

func newEscalationLimiter(max int, window time.Duration) *escalationLimiter {
	if max <= 0 {
		max = 5
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	return &escalationLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

// allow records an escalation request and reports whether it is within the
// configured rate. Denied attempts still count — a burst doesn't get to
// probe the limit for free.
func (l *escalationLimiter) allow(account string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	kept := l.hits[account][:0]
	for _, ts := range l.hits[account] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.hits[account] = kept
		return false
	}
	l.hits[account] = append(kept, now)
	return true
}

// limiterFor returns the effective limiter — policy-configured values
// override the daemon defaults. The configured limiter is built once and
// reused: a fresh instance per call would always see an empty hit window
// and never engage (specula-lite F4).
func (d *Daemon) limiterFor() *escalationLimiter {
	rate := d.policy.Escalation.Rate
	if rate.Count <= 0 && rate.WindowSec <= 0 {
		return d.escalations
	}
	d.limiterMu.Lock()
	defer d.limiterMu.Unlock()
	if d.configuredLimiter == nil {
		count := rate.Count
		if count <= 0 {
			count = 5
		}
		win := time.Duration(rate.WindowSec) * time.Second
		if win <= 0 {
			win = 10 * time.Minute
		}
		d.configuredLimiter = newEscalationLimiter(count, win)
	}
	return d.configuredLimiter
}

// trackPending records an E3-escalated request for operator lookup. Bounded:
// evicts the oldest entries at capacity; expired entries are dropped on read.
func (d *Daemon) trackPending(pr *signsock.PendingRequest) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()
	if d.pendingReq == nil {
		d.pendingReq = map[string]*signsock.PendingRequest{}
	}
	if len(d.pendingReq) >= pendingRequestCap {
		var oldest string
		var oldestTS time.Time
		for k, v := range d.pendingReq {
			ts, err := time.Parse(time.RFC3339Nano, v.Timestamp)
			if err != nil || oldest == "" || ts.Before(oldestTS) {
				oldest, oldestTS = k, ts
			}
		}
		delete(d.pendingReq, oldest)
	}
	d.pendingReq[pr.RequestHash] = pr
}

// pendingRequest returns the tracked escalation, dropping it if expired.
func (d *Daemon) pendingRequest(hash string) (*signsock.PendingRequest, bool) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()
	pr, ok := d.pendingReq[hash]
	if !ok {
		return nil, false
	}
	ts, err := time.Parse(time.RFC3339Nano, pr.Timestamp)
	if err != nil || time.Since(ts) > pendingRequestTTL {
		delete(d.pendingReq, hash)
		return nil, false
	}
	return pr, true
}
