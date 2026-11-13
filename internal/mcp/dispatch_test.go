package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

func toolCallMsg(id int, name string, args map[string]interface{}) *JSONRPCMessage {
	params, _ := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	return &JSONRPCMessage{JSONRPC: "2.0", ID: id, Method: "tools/call", Params: params}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := NewServer(config.DefaultConfig())
	s.registerTools()
	return s
}

// TestCallToolParity verifies stdio and HTTP dispatch hit the same choke
// point: identical calls produce identical results, and only the HTTP path
// marks the transport context.
func TestCallToolParity(t *testing.T) {
	s := newTestServer(t)
	var sawHTTP bool
	s.RegisterTool(Tool{
		Name:        "probe_transport",
		Description: "reports transport marker",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		sawHTTP = isHTTPTransport(ctx)
		return map[string]interface{}{"http": sawHTTP}, nil
	})
	// Registered post-registerTools → must classify it for the profile.
	toolTiers["probe_transport"] = tierT0
	defer delete(toolTiers, "probe_transport")
	s.applyProfile()

	msg := toolCallMsg(1, "probe_transport", nil)

	stdioResp := s.callTool(context.Background(), msg)
	if stdioResp.Error != nil {
		t.Fatalf("stdio call errored: %v", stdioResp.Error)
	}
	if sawHTTP {
		t.Fatal("stdio dispatch must not mark the HTTP transport")
	}

	httpResp := s.handleToolsCallHTTP(context.Background(), msg)
	if httpResp.Error != nil {
		t.Fatalf("http call errored: %v", httpResp.Error)
	}
	if !sawHTTP {
		t.Fatal("HTTP dispatch must mark the transport context")
	}
}

// TestCallToolHTTPContextPropagation verifies the request context reaches the
// handler (cancellation/values propagate rather than context.Background()).
func TestCallToolHTTPContextPropagation(t *testing.T) {
	s := newTestServer(t)
	type ctxKey struct{}
	want := "request-value"
	s.RegisterTool(Tool{
		Name:        "probe_ctx",
		Description: "reads request context",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		v, _ := ctx.Value(ctxKey{}).(string)
		return map[string]interface{}{"value": v}, nil
	})
	toolTiers["probe_ctx"] = tierT0
	defer delete(toolTiers, "probe_ctx")
	s.applyProfile()

	ctx := context.WithValue(context.Background(), ctxKey{}, want)
	resp := s.handleToolsCallHTTP(ctx, toolCallMsg(2, "probe_ctx", nil))
	if resp.Error != nil {
		t.Fatalf("call errored: %v", resp.Error)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), want) {
		t.Fatalf("request context value did not propagate: %s", data)
	}
}

// TestValidateAndCollectFailClosed covers the schema-driven validation:
// required params, wrong types, and missing schemas all refuse or prompt.
func TestValidateAndCollectFailClosed(t *testing.T) {
	s := newTestServer(t)

	// Missing required param → collection prompt, not dispatch.
	resp := s.callTool(context.Background(), toolCallMsg(3, "pay_request", map[string]interface{}{
		"asset": "XLM",
	}))
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %v", resp.Error)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "requiresUserInput") {
		t.Fatalf("expected collection prompt for missing param, got %s", data)
	}

	// Wrong type → tool error (fail closed, no dispatch).
	resp = s.callTool(context.Background(), toolCallMsg(4, "pay_request", map[string]interface{}{
		"asset":  123,
		"amount": "5",
	}))
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %v", resp.Error)
	}
	data, _ = json.Marshal(resp.Result)
	if !strings.Contains(string(data), `"isError":true`) {
		t.Fatalf("expected tool error for wrong type, got %s", data)
	}

	// Unregistered tool → MethodNotFound.
	resp = s.callTool(context.Background(), toolCallMsg(5, "nonexistent_tool", nil))
	if resp.Error == nil || resp.Error.Code != MethodNotFound {
		t.Fatalf("expected MethodNotFound, got %+v", resp.Error)
	}

	// A registered tool with a corrupt InputSchema fails closed.
	s.tools["broken"] = Tool{Name: "broken", InputSchema: json.RawMessage(`{"type":"array"}`)}
	s.handlers["broken"] = func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	}
	resp = s.callTool(context.Background(), toolCallMsg(6, "broken", nil))
	data, _ = json.Marshal(resp.Result)
	if !strings.Contains(string(data), `"isError":true`) {
		t.Fatalf("expected tool error for non-object schema, got %s", data)
	}
	delete(s.tools, "broken")
	delete(s.handlers, "broken")
}

// TestApplyProfile verifies tier-based exposure and fail-closed handling of
// unclassified tools.
func TestApplyProfile(t *testing.T) {
	s := newTestServer(t)
	if err := s.SetProfile("readonly"); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	s.RegisterTool(Tool{
		Name:        "unclassified_tool",
		Description: "no tier entry",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) { return nil, nil })
	s.applyProfile()

	if _, ok := s.tools["wallet_list"]; !ok {
		t.Error("readonly profile should expose T0 wallet_list")
	}
	if _, ok := s.tools["pay_send"]; ok {
		t.Error("readonly profile must not expose T3 pay_send")
	}
	if _, ok := s.tools["memory_save"]; ok {
		t.Error("readonly profile must not expose T1 memory_save")
	}
	if _, ok := s.tools["unclassified_tool"]; ok {
		t.Error("unclassified tool must fail closed — not registered")
	}

	if err := s.SetProfile("bogus"); err == nil {
		t.Error("SetProfile should reject unknown profile names")
	}
}

// TestGuardTransportRequest covers Host/Origin validation on HTTP endpoints.
func TestGuardTransportRequest(t *testing.T) {
	s := newTestServer(t)
	s.allowedOrigins = []string{"mcp.example.com"}
	inner := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }
	guard := s.guardTransportRequest("127.0.0.1", inner)

	cases := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{"loopback host", "127.0.0.1:3000", "", 200},
		{"localhost", "localhost:3000", "", 200},
		{"foreign host", "evil.example.com", "", 403},
		{"loopback origin", "127.0.0.1:3000", "http://localhost:8080", 200},
		{"allowlisted origin", "127.0.0.1:3000", "https://mcp.example.com", 200},
		{"foreign origin", "127.0.0.1:3000", "https://evil.example.com", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/message", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			guard(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("host=%q origin=%q → %d, want %d", tc.host, tc.origin, rec.Code, tc.want)
			}
		})
	}
}

// TestAuditChainAndSanitization verifies hash chaining and that argument
// values never reach the log — only digests and key names.
func TestAuditChainAndSanitization(t *testing.T) {
	var buf bytes.Buffer
	rec := chainlog.NewWriter(&buf, "", 0)

	secret := "SSECRETKEYMATERIAL"
	args := map[string]interface{}{"secret": secret, "amount": "5"}
	if _, err := recordAudit(rec, "wallet_test", "stdio", "allow", "dec-test", args); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := recordAudit(rec, "wallet_test", "http", "deny_invalid_args", "", args); err != nil {
		t.Fatalf("record: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 records, got %d", len(lines))
	}
	var r1, r2 auditRecord
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil {
		t.Fatalf("record 1 unmarshal: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil {
		t.Fatalf("record 2 unmarshal: %v", err)
	}

	// Chain integrity.
	if r1.Prev != "" {
		t.Fatalf("first record prev must be empty, got %q", r1.Prev)
	}
	if r2.Prev != r1.Hash {
		t.Fatal("second record does not chain to first")
	}
	clone := r2
	clone.Hash = ""
	if expected := chainlog.Compute(r2.Prev, &clone); expected != r2.Hash {
		t.Fatal("record hash does not verify")
	}

	// Sanitization: no argument values in the log.
	if strings.Contains(buf.String(), secret) || strings.Contains(buf.String(), `"5"`) {
		t.Fatal("argument values leaked into audit log")
	}
	if r1.ArgsSHA256 == "" || len(r1.ArgKeys) != 2 {
		t.Fatal("expected args digest and key list")
	}
}

// TestSystemNetworkHTTPRefusal verifies the transport gate on network switch.
func TestSystemNetworkHTTPRefusal(t *testing.T) {
	s := newTestServer(t)

	// HTTP + network arg → refused.
	httpCtx := context.WithValue(context.Background(), transportCtxKey{}, transportHTTP)
	_, err := s.handleSystemNetwork(httpCtx, map[string]interface{}{"network": "stellar-mainnet"})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected HTTP refusal, got %v", err)
	}

	// Stdio without confirm → refused.
	_, err = s.handleSystemNetwork(context.Background(), map[string]interface{}{"network": "stellar-mainnet"})
	if err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("expected confirmation requirement, got %v", err)
	}

	// Read-only form still works on HTTP.
	res, err := s.handleSystemNetwork(httpCtx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("read form should work on HTTP: %v", err)
	}
	if m, ok := res.(map[string]interface{}); !ok || m["network"] == nil {
		t.Fatalf("expected current network, got %v", res)
	}
}

// TestMinAtomicAmount covers the effective-cap computation.
func TestMinAtomicAmount(t *testing.T) {
	got, err := minAtomicAmount("1000", "500")
	if err != nil || got != "500" {
		t.Fatalf("min(1000,500) = %q, %v", got, err)
	}
	got, err = minAtomicAmount("", "500")
	if err != nil || got != "500" {
		t.Fatalf("min(,500) = %q, %v", got, err)
	}
	got, err = minAtomicAmount("42", "")
	if err != nil || got != "42" {
		t.Fatalf("min(42,) = %q, %v", got, err)
	}
	if _, err = minAtomicAmount("abc", "500"); err == nil {
		t.Fatal("expected error for non-numeric amount")
	}
}

// TestDaemonlessFailClosed verifies signing-path tools refuse cleanly when
// the custodian socket is absent — no in-process fallback.
func TestDaemonlessFailClosed(t *testing.T) {
	s := newTestServer(t)
	absent := filepath.Join(t.TempDir(), "absent.sock")
	if err := s.ApplyOperatorOptions(OperatorOptions{SignerSocket: absent}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		tool string
		args map[string]interface{}
	}{
		{"wallet_passkey", map[string]interface{}{"action": "status", "address": "GABC"}},
		{"wallet_passkey", map[string]interface{}{"action": "unlock", "address": "GABC"}},
	} {
		resp := s.callTool(context.Background(), toolCallMsg(1, tc.tool, tc.args))
		data, _ := json.Marshal(resp.Result)
		if !strings.Contains(string(data), `"isError":true`) || !strings.Contains(string(data), "stellar-go-cli signer") {
			t.Fatalf("%s did not fail closed: %s", tc.tool, data)
		}
	}
}
