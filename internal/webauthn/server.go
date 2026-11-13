package webauthn

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

//go:embed ceremony.html
var ceremonyHTML []byte

// Mode selects the WebAuthn ceremony to run.
type Mode int

const (
	// ModeRegister creates a new credential (navigator.credentials.create).
	ModeRegister Mode = iota
	// ModeAssert authenticates with an existing credential (navigator.credentials.get).
	ModeAssert
)

type ceremonyResult struct {
	credential *webauthn.Credential
	err        error
}

// Ceremony is a running WebAuthn ceremony over a loopback RP server.
// URL is the address to open in the user's browser.
type Ceremony struct {
	URL string

	srv      *http.Server
	ln       net.Listener
	done     chan ceremonyResult
	finished bool
}

// startCeremony binds a loopback listener, builds the RP and session, and
// serves the ceremony page. Call Wait (and Close) to complete it.
// boundChallenge, when non-nil in ModeAssert, replaces the random challenge:
// the signed assertion then commits to caller-chosen bytes (e.g. a request
// hash) — the approval-binding mechanism for custodian escalation.
func startCeremony(mode Mode, u *User, boundChallenge []byte) (*Ceremony, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	origin := fmt.Sprintf("http://localhost:%d", ln.Addr().(*net.TCPAddr).Port)
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          "localhost",
		RPDisplayName: "Stellar Go CLI",
		RPOrigins:     []string{origin},
	})
	if err != nil {
		ln.Close() //nolint:errcheck // best-effort listener cleanup
		return nil, fmt.Errorf("relying party init: %w", err)
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		ln.Close() //nolint:errcheck // best-effort listener cleanup
		return nil, fmt.Errorf("session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	var optionsJSON []byte
	var session *webauthn.SessionData

	switch mode {
	case ModeRegister:
		var creation *protocol.CredentialCreation
		creation, session, err = wa.BeginRegistration(u,
			webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
			webauthn.WithAuthenticatorSelection(
				webauthn.SelectAuthenticator("", nil, string(protocol.VerificationRequired)),
			),
		)
		if err != nil {
			ln.Close() //nolint:errcheck // best-effort listener cleanup
			return nil, fmt.Errorf("begin registration: %w", err)
		}
		optionsJSON, err = json.Marshal(creation.Response)
	case ModeAssert:
		var assertion *protocol.CredentialAssertion
		assertion, session, err = wa.BeginLogin(u,
			webauthn.WithAllowedCredentials(u.Descriptors()),
			webauthn.WithUserVerification(protocol.VerificationRequired),
		)
		if err != nil {
			ln.Close() //nolint:errcheck // best-effort listener cleanup
			return nil, fmt.Errorf("begin assertion: %w", err)
		}
		if len(boundChallenge) > 0 {
			assertion.Response.Challenge = protocol.URLEncodedBase64(boundChallenge)
			session.Challenge = assertion.Response.Challenge.String()
		}
		optionsJSON, err = json.Marshal(assertion.Response)
	default:
		ln.Close() //nolint:errcheck // best-effort listener cleanup
		return nil, fmt.Errorf("unknown ceremony mode %d", mode)
	}
	if err != nil {
		ln.Close() //nolint:errcheck // best-effort listener cleanup
		return nil, fmt.Errorf("marshal options: %w", err)
	}

	modeName := "create"
	if mode == ModeAssert {
		modeName = "get"
	}

	c := &Ceremony{
		URL:  origin + "/?t=" + token,
		ln:   ln,
		done: make(chan ceremonyResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", c.servePage(token))
	mux.HandleFunc("/options", c.serveOptions(token, modeName, optionsJSON))
	mux.HandleFunc("/finish", c.serveFinish(mode, u, wa, session, token))

	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { c.srv.Serve(ln) }() //nolint:errcheck // accept loop; server lifetime managed by Close

	return c, nil
}

func (c *Ceremony) servePage(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("t") != token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(ceremonyHTML) //nolint:errcheck // response write, nothing actionable
	}
}

func (c *Ceremony) serveOptions(token, mode string, optionsJSON []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"mode":%q,"publicKey":%s}`, mode, optionsJSON) //nolint:errcheck // response write, nothing actionable
	}
}

func (c *Ceremony) serveFinish(mode Mode, u *User, wa *webauthn.WebAuthn, session *webauthn.SessionData, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("t") != token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		if c.finished {
			http.Error(w, "ceremony already completed", http.StatusConflict)
			return
		}

		var cred *webauthn.Credential
		var err error
		if mode == ModeRegister {
			cred, err = wa.FinishRegistration(u, *session, r)
		} else {
			cred, err = wa.FinishLogin(u, *session, r)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("verification failed: %v", err), http.StatusBadRequest)
			c.done <- ceremonyResult{err: err}
			return
		}

		c.finished = true
		c.done <- ceremonyResult{credential: cred}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck // response write, nothing actionable
	}
}

// Wait blocks until the ceremony completes, fails, or the timeout elapses.
func (c *Ceremony) Wait(timeout time.Duration) (*webauthn.Credential, error) {
	select {
	case res := <-c.done:
		return res.credential, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out waiting for passkey ceremony")
	}
}

// Close shuts the ceremony server down.
func (c *Ceremony) Close() {
	c.srv.Shutdown(context.Background()) //nolint:errcheck // best-effort shutdown
	c.ln.Close()                         //nolint:errcheck // best-effort close
}

// Register runs a full registration ceremony: it starts the RP server, invokes
// openURL with the browser URL, and waits for the verified credential.
// A nil openURL skips opening a browser (the caller must surface c.URL).
func Register(u *User, openURL func(string) error, timeout time.Duration) (*webauthn.Credential, error) {
	c, err := startCeremony(ModeRegister, u, nil)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if openURL != nil {
		// #nosec G104 -- browser open is best-effort; URL is printed for manual use
		_ = openURL(c.URL) //nolint:errcheck
	}
	return c.Wait(timeout)
}

// Assert runs a full authentication ceremony against the user's stored
// credentials and returns the updated credential on success.
func Assert(u *User, openURL func(string) error, timeout time.Duration) (*webauthn.Credential, error) {
	c, err := startCeremony(ModeAssert, u, nil)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if openURL != nil {
		// #nosec G104 -- browser open is best-effort; URL is printed for manual use
		_ = openURL(c.URL) //nolint:errcheck
	}
	return c.Wait(timeout)
}

// StartRegister starts a registration ceremony without opening a browser.
// The caller must surface c.URL to the user.
func StartRegister(u *User) (*Ceremony, error) { return startCeremony(ModeRegister, u, nil) }

// StartAssert starts an assertion ceremony without opening a browser.
func StartAssert(u *User) (*Ceremony, error) { return startCeremony(ModeAssert, u, nil) }

// Descriptors returns the credential descriptors for allowCredentials lists.
func (u *User) Descriptors() []protocol.CredentialDescriptor {
	descs := make([]protocol.CredentialDescriptor, 0, len(u.Credentials))
	for _, cred := range u.Credentials {
		descs = append(descs, cred.Descriptor())
	}
	return descs
}

// ─── Ceremony relay ─────────────────────────────────────────────────────
// The daemon builds WebAuthn options server-side (BeginRelayFor); a client
// machine runs the actual browser ceremony via RelayServer and posts the raw
// credential response back. The daemon verifies it (FinishRelay*) — the
// daemon host needs no browser at all.

// RelaySession is the daemon-side state of a pending relayed ceremony.
type RelaySession struct {
	WA      *webauthn.WebAuthn
	Session *webauthn.SessionData
	User    *User
	Mode    Mode
}

// BeginRelayFor builds relay session + options for the client origin (the
// loopback origin the client's RelayServer bound, e.g. http://localhost:PORT).
// WebAuthn requires the RP origin to match the page origin exactly, so the
// client binds first and tells the daemon its port.
func BeginRelayFor(mode Mode, u *User, boundChallenge []byte, origin string) (*RelaySession, []byte, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          "localhost",
		RPDisplayName: "Stellar Go CLI",
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("relying party init: %w", err)
	}
	var optionsJSON []byte
	var session *webauthn.SessionData
	switch mode {
	case ModeRegister:
		var creation *protocol.CredentialCreation
		creation, session, err = wa.BeginRegistration(u,
			webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
			webauthn.WithAuthenticatorSelection(
				webauthn.SelectAuthenticator("", nil, string(protocol.VerificationRequired)),
			),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("begin registration: %w", err)
		}
		optionsJSON, err = json.Marshal(creation.Response)
	case ModeAssert:
		var assertion *protocol.CredentialAssertion
		assertion, session, err = wa.BeginLogin(u,
			webauthn.WithAllowedCredentials(u.Descriptors()),
			webauthn.WithUserVerification(protocol.VerificationRequired),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("begin assertion: %w", err)
		}
		if len(boundChallenge) > 0 {
			assertion.Response.Challenge = protocol.URLEncodedBase64(boundChallenge)
			session.Challenge = assertion.Response.Challenge.String()
		}
		optionsJSON, err = json.Marshal(assertion.Response)
	default:
		err = fmt.Errorf("unknown ceremony mode %d", mode)
	}
	if err != nil {
		return nil, nil, err
	}
	return &RelaySession{WA: wa, Session: session, User: u, Mode: mode}, optionsJSON, nil
}

// FinishRelayAssert verifies a relayed assertion body — the same checks the
// local path performs, challenge binding included.
func FinishRelayAssert(rs *RelaySession, assertionJSON []byte) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(assertionJSON))
	if err != nil {
		return nil, fmt.Errorf("parse assertion: %w", err)
	}
	cred, err := rs.WA.ValidateLogin(rs.User, *rs.Session, parsed)
	if err != nil {
		return nil, fmt.Errorf("assertion verification failed: %w", err)
	}
	return cred, nil
}

// FinishRelayRegister verifies a relayed registration body.
func FinishRelayRegister(rs *RelaySession, creationJSON []byte) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(creationJSON))
	if err != nil {
		return nil, fmt.Errorf("parse registration: %w", err)
	}
	cred, err := rs.WA.CreateCredential(rs.User, *rs.Session, parsed)
	if err != nil {
		return nil, fmt.Errorf("registration verification failed: %w", err)
	}
	return cred, nil
}

// RelayServer is the client side of a relayed ceremony: it binds a loopback
// listener FIRST (the origin is needed by the daemon's BeginRelayFor), serves
// the ceremony page with daemon-provided options, and captures the browser's
// raw credential response.
type RelayServer struct {
	Origin string

	ln       net.Listener
	token    string
	done     chan []byte
	errs     chan error
	srv      *http.Server
	finished bool
}

// StartRelay binds the loopback listener for a relayed ceremony and returns
// the origin the daemon needs for BeginRelayFor.
func StartRelay() (*RelayServer, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		ln.Close() //nolint:errcheck // best-effort listener cleanup
		return nil, err
	}
	return &RelayServer{
		Origin: fmt.Sprintf("http://localhost:%d", port),
		ln:     ln,
		token:  base64.RawURLEncoding.EncodeToString(tokenBytes),
		done:   make(chan []byte, 1),
		errs:   make(chan error, 1),
	}, nil
}

// Serve starts the relay HTTP endpoints with the daemon's options and
// returns the URL to open in the browser.
func (rs *RelayServer) Serve(mode string, optionsJSON []byte) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("t") != rs.token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(ceremonyHTML) //nolint:errcheck // response write, nothing actionable
	})
	mux.HandleFunc("/options", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != rs.token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"mode":%q,"publicKey":%s}`, mode, optionsJSON) //nolint:errcheck // response write, nothing actionable
	})
	mux.HandleFunc("/finish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("t") != rs.token {
			http.Error(w, "invalid session", http.StatusForbidden)
			return
		}
		if rs.finished {
			http.Error(w, "ceremony already completed", http.StatusConflict)
			return
		}
		rs.finished = true
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			rs.errs <- err
			return
		}
		rs.done <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck // response write, nothing actionable
	})
	rs.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { rs.srv.Serve(rs.ln) }() //nolint:errcheck // accept loop; lifetime managed by Close
	return rs.Origin + "/?t=" + rs.token
}

// Wait returns the browser's raw credential response body.
func (rs *RelayServer) Wait(timeout time.Duration) ([]byte, error) {
	select {
	case body := <-rs.done:
		return body, nil
	case err := <-rs.errs:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out waiting for passkey ceremony")
	}
}

// Close shuts the relay server down.
func (rs *RelayServer) Close() {
	if rs.srv != nil {
		rs.srv.Shutdown(context.Background()) //nolint:errcheck // best-effort shutdown
	}
	rs.ln.Close() //nolint:errcheck // best-effort close
}

// AssertBound runs an assertion ceremony whose challenge is caller-supplied:
// the returned credential's signature commits to the challenge bytes (e.g. a
// 32-byte request hash for custodian approval binding — dynamic linking per
// spec §10.2). Verification still runs through the normal session path.
func AssertBound(u *User, challenge []byte, openURL func(string) error, timeout time.Duration) (*webauthn.Credential, error) {
	c, err := startCeremony(ModeAssert, u, challenge)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if openURL != nil {
		// #nosec G104 -- browser open is best-effort; URL is printed for manual use
		_ = openURL(c.URL) //nolint:errcheck
	}
	return c.Wait(timeout)
}
