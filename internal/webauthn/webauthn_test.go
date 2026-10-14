package webauthn

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

func testUser() *User {
	return &User{Address: "GTESTADDRESS", Name: "test-user"}
}

func TestCeremonyBindsLoopback(t *testing.T) {
	c, err := StartRegister(testUser())
	if err != nil {
		t.Fatalf("StartRegister: %v", err)
	}
	defer c.Close()

	if !strings.HasPrefix(c.URL, "http://localhost:") {
		t.Fatalf("ceremony URL not loopback: %s", c.URL)
	}
	if !strings.Contains(c.URL, "?t=") {
		t.Fatal("ceremony URL missing session token")
	}
}

func TestCeremonyPageRequiresToken(t *testing.T) {
	c, err := StartRegister(testUser())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	base := c.URL[:strings.Index(c.URL, "?")]

	// No token → 403
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("page without token: got %d, want 403", resp.StatusCode)
	}

	// Wrong token → 403
	resp, err = http.Get(base + "/?t=wrong")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("page with wrong token: got %d, want 403", resp.StatusCode)
	}

	// Correct token → 200 + HTML
	resp, err = http.Get(c.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page with token: got %d, want 200", resp.StatusCode)
	}
}

func TestOptionsEndpointReturnsPublicKeyOptions(t *testing.T) {
	c, err := StartRegister(testUser())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	optionsURL := strings.Replace(c.URL, "/?", "/options?", 1)
	resp, err := http.Get(optionsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("options: got %d", resp.StatusCode)
	}

	var body struct {
		Mode      string `json:"mode"`
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID string `json:"id"`
			} `json:"rp"`
		} `json:"publicKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("options decode: %v", err)
	}
	if body.Mode != "create" {
		t.Fatalf("mode: got %q, want create", body.Mode)
	}
	if body.PublicKey.Challenge == "" {
		t.Fatal("empty challenge")
	}
	if body.PublicKey.RP.ID != "localhost" {
		t.Fatalf("rp.id: got %q, want localhost", body.PublicKey.RP.ID)
	}
}

func TestFinishRejectsMalformedAttestation(t *testing.T) {
	c, err := StartRegister(testUser())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	finishURL := strings.Replace(c.URL, "/?", "/finish?", 1)
	resp, err := http.Post(finishURL, "application/json",
		strings.NewReader(`{"id":"AA","rawId":"AA","type":"public-key","response":{"clientDataJSON":"e30","attestationObject":"AA"}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("malformed attestation accepted")
	}

	// Wait should surface the verification error, not a credential.
	cred, werr := c.Wait(5 * time.Second)
	if werr == nil {
		t.Fatal("Wait returned no error for failed verification")
	}
	if cred != nil {
		t.Fatal("Wait returned credential for failed verification")
	}
}

func TestFinishRejectsMissingToken(t *testing.T) {
	c, err := StartRegister(testUser())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	finishBase := c.URL[:strings.Index(c.URL, "?")] + "finish"
	resp, err := http.Post(finishBase, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("finish without token: got %d, want 403", resp.StatusCode)
	}
}

func userWithCredential() *User {
	u := testUser()
	u.Credentials = []webauthn.Credential{{
		ID:        []byte("existing-cred"),
		PublicKey: []byte{1, 2, 3},
		Transport: []protocol.AuthenticatorTransport{protocol.Internal},
	}}
	return u
}

func TestWaitTimeout(t *testing.T) {
	c, err := StartAssert(userWithCredential())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	cred, err := c.Wait(50 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Wait: got err %v, want timeout", err)
	}
	if cred != nil {
		t.Fatal("Wait returned credential on timeout")
	}
}

func TestAssertWithNoCredentialsFails(t *testing.T) {
	// BeginLogin must reject a user with no registered credentials rather
	// than silently starting an assertion that could accept anything.
	if _, err := StartAssert(testUser()); err == nil {
		t.Fatal("assertion ceremony started with zero credentials")
	}
}

func TestAssertOptionsEnforceUserVerification(t *testing.T) {
	c, err := StartAssert(userWithCredential())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	optionsURL := strings.Replace(c.URL, "/?", "/options?", 1)
	resp, err := http.Get(optionsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body struct {
		Mode      string `json:"mode"`
		PublicKey struct {
			Challenge          string            `json:"challenge"`
			UserVerification   string            `json:"userVerification"`
			AllowedCredentials []json.RawMessage `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Mode != "get" {
		t.Fatalf("mode: got %q, want get", body.Mode)
	}
	if body.PublicKey.Challenge == "" {
		t.Fatal("empty challenge")
	}
	if body.PublicKey.UserVerification != "required" {
		t.Fatalf("userVerification: got %q, want required", body.PublicKey.UserVerification)
	}
	if len(body.PublicKey.AllowedCredentials) != 1 {
		t.Fatalf("allowCredentials: got %d, want 1", len(body.PublicKey.AllowedCredentials))
	}
}

func TestUserAdapter(t *testing.T) {
	u := &User{Address: "GABC", Name: "n"}
	if string(u.WebAuthnID()) != "GABC" {
		t.Fatal("WebAuthnID")
	}
	if u.WebAuthnName() != "n" || u.WebAuthnDisplayName() != "n" {
		t.Fatal("name mapping")
	}
	if len(u.Descriptors()) != 0 {
		t.Fatal("empty credentials should yield no descriptors")
	}
}
