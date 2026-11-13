// ceremony.go — relayed WebAuthn ceremonies. The daemon builds the WebAuthn
// session and keeps it pending; the CLIENT machine runs the browser ceremony
// and posts the credential response back via `assert`. The daemon host never
// needs a browser — headless deployments get full passkey flows.
package custodian

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	wan "github.com/stellar-go-cli/stellar-go-cli/internal/webauthn"
)

// ceremonyTTL bounds how long a pending ceremony stays valid.
const ceremonyTTL = 2 * time.Minute

// pendingCeremony is a daemon-side WebAuthn session awaiting its assertion.
type pendingCeremony struct {
	id       string
	action   string // "unlock" | "issue_mandate" | "approve" | "register" | "reactivate" | "agent_activate"
	account  string
	params   json.RawMessage // action params (signsock.UnlockParams / signsock.IssueMandateParams / signsock.ConfirmParams)
	sub      *mandateSubject // prebuilt mandate subject (issue_mandate)
	vcid     string          // mandate id (reactivate)
	rs       *wan.RelaySession
	deadline time.Time
}

// pendingCeremonies tracks relayed ceremonies; single-use and time-bounded.
type pendingCeremonies struct {
	mu sync.Mutex
	m  map[string]*pendingCeremony
}

func newPendingCeremonies() *pendingCeremonies {
	return &pendingCeremonies{m: map[string]*pendingCeremony{}}
}

func (p *pendingCeremonies) put(c *pendingCeremony) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// opportunistic expiry sweep
	now := time.Now()
	for k, v := range p.m {
		if now.After(v.deadline) {
			delete(p.m, k)
		}
	}
	p.m[c.id] = c
}

// take removes the pending ceremony (single-use — an assertion can't be
// replayed to trigger the action twice).
func (p *pendingCeremonies) take(id string) *pendingCeremony {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.m[id]
	if !ok {
		return nil
	}
	delete(p.m, id)
	if time.Now().After(c.deadline) {
		return nil
	}
	return c
}

// beginCeremony creates a pending relayed ceremony and returns the wire
// response the client uses to drive its browser. challenge binds the
// assertion to the exact intent (empty = random session challenge).
func (d *Daemon) beginCeremony(pend *pendingCeremony, challenge []byte, origin string) (*signsock.Response, *signsock.RPCError) {
	if origin == "" {
		return nil, &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "origin required — ceremonies run on the client machine"}
	}
	var rs *wan.RelaySession
	var optionsJSON []byte
	var err error
	switch pend.action {
	case "unlock", "issue_mandate", "approve", "reactivate", "agent_activate":
		rs, optionsJSON, err = d.svc.BeginAssertRelay(pend.account, challenge, origin)
	case "register":
		rs, optionsJSON, err = d.svc.BeginRegisterRelay(pend.account, origin)
	default:
		err = fmt.Errorf("unknown ceremony action %q", pend.action)
	}
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: err.Error()}
	}
	pend.id = randomHex(16)
	pend.rs = rs
	pend.deadline = time.Now().Add(ceremonyTTL)
	d.pending.put(pend)
	data, err := json.Marshal(map[string]interface{}{
		"ceremony":     true,
		"ceremony_id":  pend.id,
		"options_json": string(optionsJSON),
		"mode":         map[bool]string{true: "create", false: "get"}[pend.action == "register"],
	})
	if err != nil {
		return nil, &signsock.RPCError{Code: signsock.ErrInternal, Message: err.Error()}
	}
	return &signsock.Response{Result: data}, nil
}

// finishCeremony verifies the relayed assertion and runs the bound action.
func (d *Daemon) finishCeremony(p *signsock.AssertParams) *signsock.Response {
	fail := func(msg string) *signsock.Response {
		return &signsock.Response{Error: &signsock.RPCError{Code: signsock.ErrRefused, Message: msg}}
	}
	pend := d.pending.take(p.CeremonyID)
	if pend == nil {
		return fail("unknown or expired ceremony")
	}
	assertJSON := []byte(p.AssertionJSON)

	switch pend.action {
	case "unlock":
		var up signsock.UnlockParams
		if len(pend.params) > 0 {
			// #nosec G104 -- params were validated at challenge time
			json.Unmarshal(pend.params, &up) //nolint:errcheck // params were validated at challenge time
		}
		seed, err := d.svc.FinishAssertRelay(pend.rs, pend.account, assertJSON)
		if err != nil {
			return fail("assertion failed: " + err.Error())
		}
		ttl := time.Duration(up.TTLSec) * time.Second
		if ttl <= 0 {
			ttl = 15 * time.Minute
		}
		d.grants.put(pend.account, seed, ttl)
		wipeString(&seed)
		exp, _ := d.grants.expiry(pend.account)
		return d.ok(map[string]interface{}{"unlocked": true, "address": pend.account, "until": exp.Format(time.RFC3339)})

	case "approve":
		var cp signsock.ConfirmParams
		if len(pend.params) > 0 {
			// #nosec G104 -- params were validated at challenge time
			json.Unmarshal(pend.params, &cp) //nolint:errcheck // validated at challenge time
		}
		if err := d.svc.VerifyAssertRelay(pend.rs, pend.account, assertJSON); err != nil {
			return fail("assertion failed: " + err.Error())
		}
		d.approvals.put(cp.RequestHash)
		return d.ok(map[string]interface{}{"approved": true, "request_hash": cp.RequestHash})

	case "issue_mandate":
		var imp signsock.IssueMandateParams
		if err := json.Unmarshal(pend.params, &imp); err != nil || pend.sub == nil {
			return fail("corrupt mandate params")
		}
		seed, err := d.svc.FinishAssertRelay(pend.rs, pend.account, assertJSON)
		if err != nil {
			return fail("assertion failed: " + err.Error())
		}
		res, rerr := d.completeMandateIssuance(&imp, pend.sub, seed)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return d.ok(res)

	case "reactivate":
		// Assert + decrypt: the seed re-escrows for the mandate's remaining
		// validity and signs the refreshed status lists.
		seed, err := d.svc.FinishAssertRelay(pend.rs, pend.account, assertJSON)
		if err != nil {
			return fail("assertion failed: " + err.Error())
		}
		if rec, lerr := d.mandates.load(pend.vcid); lerr == nil {
			if acc, aerr := d.svc.GetWalletByAddress(rec.Subject.Principal); aerr == nil && acc.PasskeyProtected {
				if ttl := time.Until(rec.Subject.Until); ttl > 0 {
					d.grants.put("mandate:"+pend.vcid, seed, ttl)
				}
			}
		}
		res := d.applyMandateStatus(pend.vcid, mandateActive, seed)
		wipeString(&seed)
		return d.ok(res)

	case "agent_activate":
		// Bound principal ceremony — the only path that may grant an agent
		// signing authority after suspension/enrolment (specula-lite F5).
		var ap signsock.AgentSetStatusParams
		if len(pend.params) > 0 {
			// #nosec G104 -- params were validated at challenge time
			json.Unmarshal(pend.params, &ap) //nolint:errcheck // validated at challenge time
		}
		if err := d.svc.VerifyAssertRelay(pend.rs, pend.account, assertJSON); err != nil {
			return fail("assertion failed: " + err.Error())
		}
		if serr := d.setAgentStatus(ap.DID, agentActive); serr != nil {
			return fail("agent activation failed: " + serr.Error())
		}
		return d.ok(map[string]interface{}{"did": ap.DID, "state": agentActive})

	case "register":
		cred, err := d.svc.FinishRegisterRelay(pend.rs, pend.account, assertJSON)
		if err != nil {
			return fail("registration failed: " + err.Error())
		}
		return d.ok(map[string]interface{}{"protected": true, "address": pend.account, "credential": hex.EncodeToString(cred.ID)})
	}
	return fail("unknown ceremony action")
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b) //nolint:errcheck // crypto/rand; failure is not actionable
	return hex.EncodeToString(b)
}

// randomChallenge for actions not bound to a payload hash (unlock, register).
func randomChallenge() []byte {
	b := make([]byte, 32)
	rand.Read(b) //nolint:errcheck // crypto/rand
	return b
}

// mustJSON marshals v for pending-ceremony params.
func mustJSON(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
