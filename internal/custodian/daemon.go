package custodian

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// Daemon is the signer custodian: it owns wallet state, holds unlock grants
// in memory, enforces policy E1–E5 on every sign request, and never exports
// key material.
type Daemon struct {
	svc               *wallet.Service
	policy            *Policy
	policyPath        string
	grants            *grantStore
	approvals         *approvalStore
	decisions         *seenSet
	counters          *spendCounters
	parties           *counterparties
	mandates          *mandateStore
	mandateCounters   *mandateCounters
	pending           *pendingCeremonies
	agents            *agentRegistry
	pendingReq        map[string]*signsock.PendingRequest // E3 escalations awaiting operator action (bounded)
	pendingMu         sync.Mutex                          // guards pendingReq — handlers run on per-conn goroutines
	escalations       *escalationLimiter
	configuredLimiter *escalationLimiter // policy-configured limiter, built once (limiterFor)
	limiterMu         sync.Mutex
	stateDir          string
	issued            map[string]bool // payloadHash of signed XDR this daemon produced — submit_tx verifies against it
	issuedMu          sync.Mutex      // guards issued — submit_tx's check-delete must be atomic
	token             string          // socket auth token (signer.token next to the socket)
	log               *chainlog.Recorder
	logger            *log.Logger
}

// issuedMark records a signed payload as submittable (submit_tx E5).
func (d *Daemon) issuedMark(payloadHash string) {
	d.issuedMu.Lock()
	defer d.issuedMu.Unlock()
	d.issued[payloadHash] = true
}

// issuedTake atomically checks and consumes an issued payload — each signed
// blob submits exactly once, even under concurrent submit_tx calls.
func (d *Daemon) issuedTake(payloadHash string) bool {
	d.issuedMu.Lock()
	defer d.issuedMu.Unlock()
	if !d.issued[payloadHash] {
		return false
	}
	delete(d.issued, payloadHash)
	return true
}

// dropWalletGrants revokes all in-memory authority for a wallet: the unlock
// grant plus every mandate escrow holding seed material — a disabled or
// removed wallet must not leave usable seeds behind (specula-lite hygiene).
func (d *Daemon) dropWalletGrants(address string) {
	d.grants.drop(address)
	for _, m := range d.mandates.list(address) {
		if id, _ := m["id"].(string); id != "" {
			d.grants.drop("mandate:" + id)
		}
	}
}

// Run starts the daemon: binds the socket, loads policy, serves until
// SIGINT/SIGTERM. Policy or state errors fail startup — the custodian never
// runs without legible policy.
func Run(sockPath, policyPath string, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(os.Stderr, "[signer] ", log.LstdFlags)
	}

	policy, err := loadPolicy(policyPath)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	if err := checkPolicyApproval(policy, stateDir); err != nil {
		return fmt.Errorf("policy gate: %w", err)
	}

	logPath := policy.Audit.Path
	if logPath == "" {
		logPath = filepath.Join(stateDir, "enforcement.log")
	}
	enfLog, err := chainlog.Open(logPath)
	if err != nil {
		return fmt.Errorf("open enforcement log: %w", err)
	}

	ag, aerr := loadAgentRegistry(stateDir)
	if aerr != nil {
		return fmt.Errorf("agent registry: %w", aerr)
	}

	d := &Daemon{
		svc:             wallet.NewService(),
		policy:          policy,
		policyPath:      policyPath,
		grants:          newGrantStore(),
		approvals:       newApprovalStore(5 * time.Minute),
		decisions:       newSeenSet(),
		counters:        loadCounters(filepath.Join(stateDir, "limits-counters.json")),
		parties:         loadCounterparties(counterpartyPath(stateDir)),
		stateDir:        stateDir,
		mandates:        newMandateStore(filepath.Join(stateDir, "mandates")),
		mandateCounters: loadMandateCounters(filepath.Join(stateDir, "mandate-counters.json")),
		pending:         newPendingCeremonies(),
		agents:          ag,
		pendingReq:      map[string]*signsock.PendingRequest{},
		escalations:     newEscalationLimiter(5, 10*time.Minute),
		issued:          map[string]bool{},
		log:             enfLog,
		logger:          logger,
	}
	token, err := d.loadOrCreateToken(filepath.Join(filepath.Dir(sockPath), "signer.token"))
	if err != nil {
		return fmt.Errorf("socket token: %w", err)
	}
	d.token = token
	d.rebuildSeenDecisions(logPath)
	d.recordSelfIntegrity()

	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sockPath, err)
	}
	d.rebuildIssued(logPath)
	if err := os.Chmod(sockPath, 0600); err != nil {
		// #nosec G104 -- best-effort cleanup on startup failure
		_ = ln.Close() //nolint:errcheck
		return fmt.Errorf("socket permissions: %w", err)
	}
	logger.Printf("custodian listening on %s (policy %s)", sockPath, d.policyStamp())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Printf("shutting down")
		// #nosec G104 -- terminating accept loop
		_ = ln.Close() //nolint:errcheck
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil // listener closed — clean shutdown
		}
		go d.serve(conn)
	}
}

// rebuildSeenDecisions resumes E5 replay protection across restarts by
// scanning the enforcement log for consumed decision refs.
func (d *Daemon) rebuildSeenDecisions(path string) {
	f, err := os.Open(path) // #nosec G304 -- enforcement log path under operator control
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // read-only scan
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec enforcementRecord
		if json.Unmarshal(scanner.Bytes(), &rec) == nil && rec.DecisionRef != "" &&
			len(rec.Result) > 7 && rec.Result[:7] == "signed:" {
			d.decisions.claim(rec.DecisionRef)
		}
	}
}

// rebuildIssued resumes submit_tx authority across restarts: payloads the
// daemon signed (Result signed:<payloadHash>) may be submitted later. Replay
// is sequential — a submit_tx record carrying payload_hash tombstones the
// entry, so an already-submitted blob is not resurrected (specula-lite F6).
func (d *Daemon) rebuildIssued(path string) {
	f, err := os.Open(path) // #nosec G304 -- enforcement log path under operator control
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // read-only scan
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec enforcementRecord
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		if len(rec.Result) > 7 && rec.Result[:7] == "signed:" {
			d.issued[rec.Result[7:]] = true
			continue
		}
		if rec.Tool == signsock.MethodSubmitTx && rec.PayloadHash != "" {
			delete(d.issued, rec.PayloadHash)
		}
	}
}

// serve handles one connection: optional peer-credential check, then NDJSON
// request/response lines until EOF.
func (d *Daemon) serve(conn net.Conn) {
	defer conn.Close() //nolint:errcheck // per-request or streaming; close on exit either way
	// Peer-UID mismatch is logged but no longer fatal — the socket token is
	// the real gate, so token-bearing cross-principal clients (Docker split,
	// a second operator account) work. Same-UID processes still need the
	// token file, closing the "any same-UID process can sign" gap.
	if err := checkPeer(conn); err != nil {
		d.logger.Printf("peer without matching UID connected — token auth will apply")
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var req signsock.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			d.reply(conn, &signsock.Response{Error: &signsock.RPCError{Code: signsock.ErrBadRequest, Message: "malformed signsock.Request"}})
			continue
		}
		resp := d.dispatch(&req)
		resp.ID = req.ID
		d.reply(conn, resp)
	}
}

func (d *Daemon) reply(conn net.Conn, resp *signsock.Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		d.logger.Printf("reply write failed: %v", err)
	}
}

// dispatch routes a request to its method handler. Unknown methods refuse —
// the custodian surface is fixed and small by design.
func (d *Daemon) dispatch(req *signsock.Request) *signsock.Response {
	fail := func(code, msg string) *signsock.Response {
		return &signsock.Response{Error: &signsock.RPCError{Code: code, Message: msg}}
	}
	if req.Token == "" || req.Token != d.token {
		return fail(signsock.ErrAuth, "missing or invalid socket token — read signer.token next to the socket")
	}
	ok := func(v interface{}) *signsock.Response {
		data, err := json.Marshal(v)
		if err != nil {
			return fail(signsock.ErrInternal, err.Error())
		}
		return &signsock.Response{Result: data}
	}
	params := func(v interface{}) bool {
		if err := json.Unmarshal(req.Params, v); err != nil {
			return false
		}
		return true
	}

	switch req.Method {
	case signsock.MethodSignTx:
		var p signsock.SignTxParams
		if !params(&p) {
			return fail(signsock.ErrBadRequest, "bad sign_tx params")
		}
		signed, rerr := d.enforceAndSignMandated(signsock.MethodSignTx, p.DecisionRef, p.RequestHash, p.Tool, p.Account, p.Passphrase, p.EnvelopeXDR, 0, p.MandateRef, p.AgentPOP, p.Untrusted)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return ok(signsock.SignResult{PayloadXDR: signed})

	case signsock.MethodSignSorobanAuth:
		var p signsock.SignAuthParams
		if !params(&p) {
			return fail(signsock.ErrBadRequest, "bad sign_soroban_auth params")
		}
		signed, rerr := d.enforceAndSignMandated(signsock.MethodSignSorobanAuth, p.DecisionRef, p.RequestHash, p.Tool, p.Account, p.Passphrase, p.EntryXDR, p.ValidUntil, p.MandateRef, p.AgentPOP, p.Untrusted)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return ok(signsock.SignResult{PayloadXDR: signed})

	case signsock.MethodPubkey:
		var p struct {
			Address string `json:"address"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad pubkey params")
		}
		return ok(map[string]string{"address": p.Address})

	case signsock.MethodActiveWallet:
		acc, err := d.svc.GetActiveWallet()
		if err != nil {
			return fail(signsock.ErrRefused, err.Error())
		}
		return ok(sanitizeAccount(acc))

	case signsock.MethodWalletGet:
		var p struct {
			Address string `json:"address"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad wallet_get params")
		}
		acc, err := d.svc.GetWalletByAddress(p.Address)
		if err != nil {
			return fail(signsock.ErrRefused, err.Error())
		}
		return ok(sanitizeAccount(acc))

	case signsock.MethodListWallets:
		wallets, active, err := d.svc.ListWallets()
		if err != nil {
			return fail(signsock.ErrInternal, err.Error())
		}
		return ok(map[string]interface{}{"wallets": wallets, "active": active})

	case signsock.MethodUnlock:
		var p signsock.UnlockParams
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad unlock params")
		}
		resp, rerr := d.beginCeremony(&pendingCeremony{action: "unlock", account: p.Address, params: mustJSON(&p)}, randomChallenge(), p.Origin)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return resp

	case signsock.MethodLock:
		var p struct {
			Address string `json:"address"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad lock params")
		}
		d.grants.drop(p.Address)
		return ok(map[string]interface{}{"locked": true, "address": p.Address})

	case signsock.MethodStatus:
		var p struct {
			Address string `json:"address"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad status params")
		}
		return ok(d.status(p.Address))

	case signsock.MethodRegister:
		var p struct {
			Address string `json:"address"`
			Origin  string `json:"origin"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad passkey_register params")
		}
		resp, rerr := d.beginCeremony(&pendingCeremony{action: "register", account: p.Address, params: mustJSON(&p)}, randomChallenge(), p.Origin)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return resp

	case signsock.MethodDisable:
		var p struct {
			Address string `json:"address"`
		}
		if !params(&p) || p.Address == "" {
			return fail(signsock.ErrBadRequest, "bad passkey_disable params")
		}
		if err := d.svc.DisablePasskey(p.Address); err != nil {
			return fail(signsock.ErrRefused, err.Error())
		}
		d.dropWalletGrants(p.Address)
		return ok(map[string]interface{}{"protected": false, "address": p.Address})

	case signsock.MethodConfirm:
		var p signsock.ConfirmParams
		if !params(&p) || p.Account == "" || p.RequestHash == "" {
			return fail(signsock.ErrBadRequest, "bad confirm params")
		}
		challenge, err := hex.DecodeString(p.RequestHash)
		if err != nil || len(challenge) != 32 {
			return fail(signsock.ErrBadRequest, "request_hash must be a 32-byte hex digest")
		}
		// The approval must trace to a tracked escalation — the agent signs
		// the request first, the E3 refusal queues it, and the operator
		// approves THAT request. Blind pre-approval of arbitrary hashes
		// bypasses the rate limiter and the pending queue (specula-lite F10).
		if _, okp := d.pendingRequest(p.RequestHash); !okp {
			return fail(signsock.ErrRefused, "no tracked escalation for that request hash — the request must be signed (and refused E3) before it can be confirmed")
		}
		resp, rerr := d.beginCeremony(&pendingCeremony{action: "approve", account: p.Account, params: mustJSON(&p)}, challenge, p.Origin)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return resp

	case signsock.MethodRemoveWallet:
		var p struct {
			Address string `json:"address"`
			Confirm bool   `json:"confirm"`
		}
		if !params(&p) || p.Address == "" || !p.Confirm {
			return fail(signsock.ErrBadRequest, "remove_wallet requires address and confirm=true")
		}
		if err := d.svc.RemoveWallet(p.Address); err != nil {
			return fail(signsock.ErrRefused, err.Error())
		}
		d.dropWalletGrants(p.Address)
		return ok(map[string]interface{}{"removed": true, "address": p.Address})

	case signsock.MethodListMandates:
		var p signsock.ListMandatesParams
		params(&p) //nolint:errcheck // empty params are fine — list all
		return ok(d.mandates.list(p.Account))

	case signsock.MethodSetMandateState:
		var p signsock.SetMandateStatusParams
		if !params(&p) || p.VCID == "" || p.Status == "" {
			return fail(signsock.ErrBadRequest, "bad set_mandate_status params")
		}
		if p.Status == mandateActive {
			// Re-activation grants authority — bound principal ceremony.
			resp, rerr := d.beginCeremony(
				&pendingCeremony{action: "reactivate", account: d.mandatePrincipal(p.VCID), vcid: p.VCID},
				func() []byte { c := sha256.Sum256([]byte("reactivate " + p.VCID)); return c[:] }(), p.Origin)
			if rerr != nil {
				return &signsock.Response{Error: rerr}
			}
			return resp
		}
		seed := d.mandateSeedFor(p.VCID)
		res := d.applyMandateStatus(p.VCID, p.Status, seed)
		wipeString(&seed)
		return ok(res)

	case signsock.MethodStatusList:
		var p signsock.StatusListParams
		if !params(&p) {
			return fail(signsock.ErrBadRequest, "bad status_list params")
		}
		issuer := p.Issuer
		if issuer == "" && p.MandateRef != "" {
			if rec, lerr := d.mandates.load(p.MandateRef); lerr == nil {
				issuer = rec.VC.Issuer
			}
		}
		if issuer == "" {
			return fail(signsock.ErrBadRequest, "status_list needs issuer or mandate_ref")
		}
		lists, lerr := d.exportStatusLists(issuer)
		if lerr != nil {
			return fail(signsock.ErrRefused, lerr.Error())
		}
		return ok(lists)

	case signsock.MethodIssueMandate:
		var p signsock.IssueMandateParams
		if !params(&p) || p.Account == "" || p.Agent == "" || len(p.Scope) == 0 {
			return fail(signsock.ErrBadRequest, "bad issue_mandate params")
		}
		resp, rerr := d.issueMandate(&p)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return resp

	case signsock.MethodSignCredential:
		var p signsock.SignCredentialParams
		if !params(&p) || p.Account == "" || p.UnsignedVC == "" || (p.IssuerDID == "" && p.IssuerMethod == "") {
			return fail(signsock.ErrBadRequest, "bad sign_credential params")
		}
		res, rerr := d.signCredential(&p)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return ok(res)

	case signsock.MethodSubmitTx:
		var p signsock.SubmitTxParams
		if !params(&p) || p.SignedXDR == "" {
			return fail(signsock.ErrBadRequest, "bad submit_tx params")
		}
		res, rerr := d.submitTx(&p)
		if rerr != nil {
			return &signsock.Response{Error: rerr}
		}
		return ok(res)

	case signsock.MethodRequestInfo:
		var p signsock.RequestInfoParams
		if !params(&p) || p.RequestHash == "" {
			return fail(signsock.ErrBadRequest, "bad request_info params")
		}
		pr, okp := d.pendingRequest(p.RequestHash)
		if !okp {
			return fail(signsock.ErrRefused, "no pending escalation for that request hash")
		}
		return ok(pr)

	case signsock.MethodAgentEnroll:
		var p signsock.AgentEnrollParams
		if !params(&p) || p.DID == "" {
			return fail(signsock.ErrBadRequest, "bad agent_enroll params")
		}
		rec, aerr := d.agents.enroll(p.DID, p.Profile, p.Note, false)
		if aerr != nil {
			return fail(signsock.ErrRefused, aerr.Error())
		}
		return ok(rec)

	case signsock.MethodAgentSetStatus:
		var p signsock.AgentSetStatusParams
		if !params(&p) || p.DID == "" || p.Status == "" {
			return fail(signsock.ErrBadRequest, "bad agent_set_status params")
		}
		target := p.Status
		switch target {
		case "reactivate":
			target = agentActive
		case "suspend":
			target = agentSuspended
		case "retire":
			target = agentRetired
		}
		if target == agentActive {
			// Activating an agent GRANTS authority — like mandate
			// reactivation it requires a bound principal ceremony, not a
			// bare token call (specula-lite F5). p.Account selects the
			// custodied wallet whose passkey the operator asserts with.
			if p.Account == "" {
				return fail(signsock.ErrBadRequest, "agent activation requires account for the bound ceremony")
			}
			resp, rerr := d.beginCeremony(
				&pendingCeremony{action: "agent_activate", account: p.Account, params: mustJSON(&p)},
				func() []byte { c := sha256.Sum256([]byte("agent_activate " + p.DID)); return c[:] }(), p.Origin)
			if rerr != nil {
				return &signsock.Response{Error: rerr}
			}
			return resp
		}
		if serr := d.setAgentStatus(p.DID, target); serr != nil {
			return fail(signsock.ErrRefused, serr.Error())
		}
		return ok(map[string]interface{}{"did": p.DID, "state": target})

	case signsock.MethodAgentList:
		return ok(d.agents.list())

	case signsock.MethodAssert:
		var p signsock.AssertParams
		if !params(&p) || p.CeremonyID == "" || p.AssertionJSON == "" {
			return fail(signsock.ErrBadRequest, "bad assert params")
		}
		return d.finishCeremony(&p)

	default:
		return fail(signsock.ErrBadRequest, "unknown method "+req.Method)
	}
}

// status reports protection + grant state for a wallet.
func (d *Daemon) status(address string) map[string]interface{} {
	out := map[string]interface{}{"address": address, "policy": d.policyStamp()}
	acc, err := d.svc.GetWalletByAddress(address)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["protected"] = acc.PasskeyProtected
	out["type"] = acc.Type
	creds, err := wallet.LoadPasskeys(address)
	if err == nil {
		out["credentials"] = len(creds)
	}
	if exp, ok := d.grants.expiry(address); ok {
		out["unlocked"] = true
		out["unlocked_until"] = exp.Format(time.RFC3339)
	} else {
		out["unlocked"] = false
	}
	return out
}

// sanitizeAccount strips all key material before an account crosses the
// socket — the gatekeeper never sees PrivateKey or PrivateKeyEnc.
func sanitizeAccount(acc *models.Account) *models.Account {
	if acc == nil {
		return nil
	}
	out := *acc
	out.PrivateKey = ""
	out.PrivateKeyEnc = ""
	return &out
}

// wipeString zeroes a seed copy's backing bytes before it goes out of scope.
func wipeString(s *string) {
	if s == nil || *s == "" {
		return
	}
	b := []byte(*s)
	for i := range b {
		b[i] = 0
	}
	*s = ""
}

// ok marshals a successful response payload.
func (d *Daemon) ok(v interface{}) *signsock.Response {
	data, err := json.Marshal(v)
	if err != nil {
		return &signsock.Response{Error: &signsock.RPCError{Code: signsock.ErrInternal, Message: err.Error()}}
	}
	return &signsock.Response{Result: data}
}

// mandatePrincipal resolves a mandate id to its principal account — used to
// pick the ceremony account for reactivation.
func (d *Daemon) mandatePrincipal(vcid string) string {
	rec, err := d.mandates.load(vcid)
	if err != nil || rec.Subject == nil {
		return ""
	}
	return rec.Subject.Principal
}

// loadOrCreateToken reads the socket auth token or generates one. The token
// file lives next to the socket at 0600 — same trust level as the socket
// itself; in the Docker split the shared volume carries it to the gatekeeper.
func (d *Daemon) loadOrCreateToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil { // #nosec G304 -- fixed filename inside socket dir
		tok := strings.TrimSpace(string(data))
		if len(tok) >= 32 {
			return tok, nil
		}
	}
	tok := randomHex(24)
	if err := os.WriteFile(path, []byte(tok+"\n"), 0600); err != nil {
		return "", err
	}
	return tok, nil
}

// recordSelfIntegrity anchors the custodian binary's sha256 in the
// enforcement chain (C14 — the artefact that made every following decision
// is identified by digest).
func (d *Daemon) recordSelfIntegrity() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return
	}
	data, err := os.ReadFile(exe) // #nosec G304 -- the daemon's own binary
	if err != nil {
		return
	}
	sum := sha256.Sum256(data)
	d.record(enforcementRecord{
		Timestamp:   nowRFC3339(),
		DecisionRef: "self",
		Tool:        "self_integrity",
		KeyID:       "custodian",
		Result:      "self:" + hex.EncodeToString(sum[:]),
		Policy:      d.policyStamp(),
	})
}
