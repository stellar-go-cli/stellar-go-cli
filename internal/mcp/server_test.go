package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

func TestMCPServerInitialize(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)

	// Create pipes for stdin/stdout
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	server.stdin = stdin
	server.stdout = stdout

	// Send initialize request
	initReq := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}`),
	}
	data, _ := json.Marshal(initReq)
	stdin.Write(data)
	stdin.WriteByte('\n')

	// Process one message
	server.serveStdio()

	// Check response
	output := stdout.String()
	if !strings.Contains(output, `"protocolVersion":"2024-11-05"`) {
		t.Errorf("Expected protocol version in response, got: %s", output)
	}
	if !strings.Contains(output, `"name":"mozartpay-mcp"`) {
		t.Errorf("Expected server name in response, got: %s", output)
	}
}

func TestMCPServerToolsList(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)
	server.registerTools()

	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	server.stdin = stdin
	server.stdout = stdout

	// Send tools/list request
	toolsReq := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}
	data, _ := json.Marshal(toolsReq)
	stdin.Write(data)
	stdin.WriteByte('\n')

	// Process
	server.serveStdio()

	// Check response contains tools
	output := stdout.String()
	if !strings.Contains(output, `"tools"`) {
		t.Errorf("Expected tools array in response, got: %s", output)
	}

	// Verify some expected tools are present
	expectedTools := []string{"wallet_list", "swap_quote", "system_status"}
	for _, tool := range expectedTools {
		if !strings.Contains(output, tool) {
			t.Errorf("Expected tool %s not found in response", tool)
		}
	}
}

func TestMCPServerToolCall(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg)
	server.registerTools()

	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	server.stdin = stdin
	server.stdout = stdout

	// Send tools/call request for system_status
	callReq := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "system_status",
			"arguments": {}
		}`),
	}
	data, _ := json.Marshal(callReq)
	stdin.Write(data)
	stdin.WriteByte('\n')

	// Process
	server.serveStdio()

	// Check response — tool results wrap the payload as escaped JSON
	// inside content[].text, so the field appears as \"version\".
	output := stdout.String()
	if !strings.Contains(output, `\"version\"`) {
		t.Errorf("Expected version in response, got: %s", output)
	}
}
