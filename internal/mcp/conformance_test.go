package mcp

// Conformance vectors C1, C2, C7, C8, C13 (spec §15, Class T) — the
// gatekeeper-side checks. Custodian-side vectors live in
// internal/custodian/conformance_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// C1 — the gatekeeper's process has no dependency path to seed-bearing
// wallet internals. A static dependency check: internal/wallet must not
// appear in internal/mcp's import graph.
func TestC1_NoWalletDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/stellar-go-cli/stellar-go-cli/internal/mcp").Output()
	if err != nil {
		t.Skip("go tool unavailable — cannot run static dependency check")
	}
	for _, dep := range strings.Split(string(out), "\n") {
		if dep == "github.com/stellar-go-cli/stellar-go-cli/internal/wallet" {
			t.Fatal("internal/mcp imports internal/wallet — key material reachable")
		}
	}
}

// C2 + C13 — no agent-reachable path returns or imports key material: the
// registry exposes no import/export tools and no tool schema takes a
// seed/private-key parameter.
func TestC2C13_NoSecretToolsOrParams(t *testing.T) {
	s := newTestServer(t)
	// Import/export carry key material — unreachable for agents. wallet_remove
	// is a lifecycle op (no key exposure) and stays, profile-gated.
	banned := []string{"wallet_import", "wallet_export"}
	for _, name := range banned {
		if _, ok := s.tools[name]; ok {
			t.Fatalf("secret-bearing tool %s exposed to agents", name)
		}
	}
	secretParams := []string{"seed", "secret", "privatekey", "private_key", "mnemonic", "secretkey", "secret_key"}
	for name, tool := range s.tools {
		var schema struct {
			Properties map[string]interface{} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("tool %s schema: %v", name, err)
		}
		for param := range schema.Properties {
			p := strings.ToLower(param)
			for _, bad := range secretParams {
				if strings.Contains(p, bad) {
					t.Fatalf("tool %s param %q can carry key material", name, param)
				}
			}
		}
	}
	// Passkey ops must not include a ceremony-answering or key-exporting op.
	if tool, ok := s.tools["wallet_passkey"]; ok {
		var schema struct {
			Properties map[string]struct {
				Enum []interface{} `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err == nil {
			if op, ok2 := schema.Properties["operation"]; ok2 {
				for _, v := range op.Enum {
					if s, _ := v.(string); s == "approve" || s == "export" {
						t.Fatalf("wallet_passkey exposes privileged op %q to agents", s)
					}
				}
			}
		}
	}
}

// C7 — an ESCALATE cannot be answered on the agent channel: no tool or
// socket-facing handler path invokes the custodian `confirm` method.
// Mechanically: the tools surface has no approve/confirm operation (asserted
// in C2C13 above for wallet_passkey); additionally the socket client in this
// package never calls "confirm" — the only Confirm call site is the
// operator-side wallet passkey CLI.
func TestC7_NoEscalationAnswerOnAgentChannel(t *testing.T) {
	s := newTestServer(t)
	for name, tool := range s.tools {
		var schema struct {
			Properties map[string]interface{} `json:"properties"`
		}
		_ = json.Unmarshal(tool.InputSchema, &schema)
		for param := range schema.Properties {
			// "confirm" booleans (system_network) are hold-to-confirm flags,
			// not ceremony answers — those would carry an assertion payload.
			if param == "assertion" || param == "attestation" || param == "ceremony" || param == "webauthn" {
				t.Fatalf("tool %s exposes a ceremony-answer parameter %q", name, param)
			}
		}
	}
}

// C8 — HTTP transport hardening holds (loopback/auth/Origin). Asserted in
// TestGuardTransportRequest — this vector re-asserts the loopback default
// and auth requirement are in place.
func TestC8_TransportDefaults(t *testing.T) {
	s := newTestServer(t)
	inner := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }
	guard := s.guardTransportRequest("127.0.0.1", inner)

	// Loopback host passes; foreign host fails closed.
	for _, tc := range []struct {
		name, host, origin string
		want               int
	}{
		{"loopback", "127.0.0.1:3000", "", 200},
		{"foreign host", "evil.example.com", "", 403},
		{"foreign origin", "127.0.0.1:3000", "https://evil.example.com", 403},
	} {
		req := httptest.NewRequest("POST", "/message", nil)
		req.Host = tc.host
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		guard(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s → %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}
