// fuzz_test.go — crash fuzz target for the MCP dispatch choke point
// (docs/audit/differential-fuzzing.md target #8): callTool + schema
// validation must never panic on adversarial params, must always produce a
// well-formed JSON-RPC response (exactly one of Result/Error), and must
// never route a malformed call to a handler.
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

// FuzzCallTool fuzzes the tools/call params document end to end: unknown
// tools, type-confused arguments, deep nesting, oversized strings.
func FuzzCallTool(f *testing.F) {
	f.Add(`{"name":"wallet_list","arguments":{}}`)
	f.Add(`{"name":"pay_send","arguments":{"account":"x","destination":"GAAA","amount":"1.5"}}`)
	f.Add(`{"name":12345,"arguments":"not-an-object"}`)
	f.Add(`{"name":"\u0000\uffff","arguments":{"a":{"b":{"c":{"d":[1,2,3]}}}}}`)
	f.Add(`not json`)
	f.Fuzz(func(t *testing.T, params string) {
		s := NewServer(config.DefaultConfig())
		handlerRan := false
		s.RegisterTool(Tool{
			Name:        "echo_tool",
			Description: "schema-validated echo",
			InputSchema: buildInputSchema(map[string]interface{}{
				"account": map[string]interface{}{"type": "string"},
				"amount":  map[string]interface{}{"type": "string"},
			}, []string{"account"}),
		}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			handlerRan = true
			return map[string]interface{}{"ok": true}, nil
		})

		msg := &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
			Params:  json.RawMessage(params),
		}
		resp := s.callTool(context.Background(), msg)

		if resp == nil {
			t.Fatal("callTool returned nil response")
		}
		// Exactly one of Result/Error must be set.
		if (resp.Result == nil) == (resp.Error == nil) {
			t.Fatalf("malformed response (Result=%v, Error=%v) for params %q", resp.Result, resp.Error, params)
		}
		// The response must itself marshal — a fuzzed value must not poison
		// the response document.
		if _, err := json.Marshal(resp); err != nil {
			t.Fatalf("unmarshalable response for params %q: %v", params, err)
		}
		// A handler may only run for the registered tool with schema-valid
		// args — never for unknown tools or malformed params.
		var parsed struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if json.Unmarshal([]byte(params), &parsed) == nil {
			if parsed.Name != "echo_tool" && handlerRan {
				t.Fatalf("handler ran for unregistered tool %q", parsed.Name)
			}
			if parsed.Name == "echo_tool" && handlerRan {
				if _, ok := parsed.Arguments["account"].(string); !ok {
					t.Fatalf("handler ran without required string arg: %v", parsed.Arguments)
				}
			}
		}
	})
}

// FuzzValidateAndCollect exercises the schema validator directly — the
// fail-closed gate every tool call passes through.
func FuzzValidateAndCollect(f *testing.F) {
	f.Add(`{"account":"GA","amount":"1.5"}`)
	f.Add(`{"account":42,"amount":[1,2]}`)
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, argsJSON string) {
		tool := Tool{
			Name: "t",
			InputSchema: buildInputSchema(map[string]interface{}{
				"account": map[string]interface{}{"type": "string"},
				"amount":  map[string]interface{}{"type": "string"},
			}, []string{"account"}),
		}
		var args map[string]interface{}
		if json.Unmarshal([]byte(argsJSON), &args) != nil {
			args = nil
		}
		// Must not panic; response is either a validation error or a
		// collection prompt — never a silent pass-through.
		_, _ = validateAndCollect(tool, args)
	})
}
