package webauthn

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
func startCeremony(mode Mode, u *User) (*Ceremony, error) {
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
	c, err := startCeremony(ModeRegister, u)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if openURL != nil {
		openURL(c.URL) //nolint:errcheck // browser open is best-effort; URL is printed for manual use
	}
	return c.Wait(timeout)
}

// Assert runs a full authentication ceremony against the user's stored
// credentials and returns the updated credential on success.
func Assert(u *User, openURL func(string) error, timeout time.Duration) (*webauthn.Credential, error) {
	c, err := startCeremony(ModeAssert, u)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if openURL != nil {
		openURL(c.URL) //nolint:errcheck // browser open is best-effort; URL is printed for manual use
	}
	return c.Wait(timeout)
}

// StartRegister starts a registration ceremony without opening a browser.
// The caller must surface c.URL to the user.
func StartRegister(u *User) (*Ceremony, error) { return startCeremony(ModeRegister, u) }

// StartAssert starts an assertion ceremony without opening a browser.
func StartAssert(u *User) (*Ceremony, error) { return startCeremony(ModeAssert, u) }

// Descriptors returns the credential descriptors for allowCredentials lists.
func (u *User) Descriptors() []protocol.CredentialDescriptor {
	descs := make([]protocol.CredentialDescriptor, 0, len(u.Credentials))
	for _, cred := range u.Credentials {
		descs = append(descs, cred.Descriptor())
	}
	return descs
}
