// Package mcp implements the Model Context Protocol server for MozartPay CLI.
// It exposes CLI functionality as MCP tools via stdio or SSE transport using JSON-RPC 2.0.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
)

const (
	jsonRPCVersion = "2.0"
	mcpVersion     = "2024-11-05"
)

// JSONRPCMessage represents a JSON-RPC 2.0 message
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Error codes per JSON-RPC 2.0 spec
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Server handles MCP protocol communication
type Server struct {
	cfg       *config.Config
	walletSvc *wallet.Service
	tools     map[string]Tool
	handlers  map[string]ToolHandler
	logger    *log.Logger
	mu        sync.RWMutex
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolHandler is the function signature for handling tool calls
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// NewServer creates a new MCP server instance
func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg:       cfg,
		walletSvc: wallet.NewService(),
		tools:     make(map[string]Tool),
		handlers:  make(map[string]ToolHandler),
		logger:    log.New(os.Stderr, "[mcp] ", log.LstdFlags),
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}
}

// RegisterTool registers a tool with its handler
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// Start begins the MCP server with the specified transport
func (s *Server) Start(transport string, port int) error {
	s.logger.Printf("Starting MCP server (transport: %s)\n", transport)

	// Register all tools
	s.registerTools()

	switch transport {
	case "stdio":
		return s.serveStdio()
	case "sse":
		return s.serveSSE(port)
	default:
		return fmt.Errorf("unsupported transport: %s", transport)
	}
}

// serveStdio handles MCP communication over stdin/stdout
func (s *Server) serveStdio() error {
	scanner := bufio.NewScanner(s.stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg JSONRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			s.sendError(nil, ParseError, "Parse error", err.Error())
			continue
		}

		if msg.JSONRPC != jsonRPCVersion {
			s.sendError(msg.ID, InvalidRequest, "Invalid JSON-RPC version", nil)
			continue
		}

		if err := s.handleMessage(&msg); err != nil {
			s.logger.Printf("Error handling message: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

// handleMessage processes a single JSON-RPC message
func (s *Server) handleMessage(msg *JSONRPCMessage) error {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "initialized":
		return s.handleInitialized(msg)
	case "tools/list":
		return s.handleToolsList(msg)
	case "tools/call":
		return s.handleToolsCall(msg)
	case "ping":
		return s.handlePing(msg)
	default:
		return s.sendError(msg.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", msg.Method), nil)
	}
}

// handleInitialize processes the initialize request
func (s *Server) handleInitialize(msg *JSONRPCMessage) error {
	var params struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, InvalidParams, "Invalid params", err.Error())
	}

	s.logger.Printf("Client connected: %s v%s (protocol: %s)\n",
		params.ClientInfo.Name, params.ClientInfo.Version, params.ProtocolVersion)

	result := map[string]interface{}{
		"protocolVersion": mcpVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
			"logging": map[string]interface{}{},
		},
		"serverInfo": map[string]string{
			"name":    "mozartpay-mcp",
			"version": config.Version,
		},
	}

	return s.sendResponse(msg.ID, result)
}

// handleInitialized processes the client initialized notification
func (s *Server) handleInitialized(msg *JSONRPCMessage) error {
	s.logger.Println("Client initialized")
	// This is a notification, no response needed
	return nil
}

// handleToolsList returns the list of available tools
func (s *Server) handleToolsList(msg *JSONRPCMessage) error {
	s.mu.RLock()
	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}
	s.mu.RUnlock()

	result := map[string]interface{}{
		"tools": tools,
	}

	return s.sendResponse(msg.ID, result)
}

// handleToolsCall executes a tool call with parameter validation
func (s *Server) handleToolsCall(msg *JSONRPCMessage) error {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
		Meta      map[string]interface{} `json:"_meta,omitempty"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, InvalidParams, "Invalid params", err.Error())
	}

	// Check if we have all required parameters
	collectionResponse, err := ValidateAndCollectParameters(params.Name, params.Arguments)
	if err != nil {
		return s.sendToolError(msg.ID, err)
	}

	// If there are missing parameters, return a collection prompt instead of executing
	if collectionResponse != nil {
		// Return a special response asking for missing parameters
		result := map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": collectionResponse.ContextMessage + "\n\n" + collectionResponse.Prompt,
				},
			},
			"isError": false,
			"_meta": map[string]interface{}{
				"requiresUserInput": true,
				"missingParams":     collectionResponse.MissingParams,
				"toolName":          collectionResponse.ToolName,
			},
		}
		return s.sendResponse(msg.ID, result)
	}

	s.mu.RLock()
	handler, ok := s.handlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		return s.sendError(msg.ID, MethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}

	ctx := context.Background()
	result, err := handler(ctx, params.Arguments)
	if err != nil {
		return s.sendToolError(msg.ID, err)
	}

	return s.sendToolResult(msg.ID, result)
}

// handlePing responds to ping requests
func (s *Server) handlePing(msg *JSONRPCMessage) error {
	return s.sendResponse(msg.ID, map[string]interface{}{})
}

// sendResponse sends a successful JSON-RPC response
func (s *Server) sendResponse(id interface{}, result interface{}) error {
	resp := JSONRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
	return s.writeMessage(&resp)
}

// sendError sends a JSON-RPC error response
func (s *Server) sendError(id interface{}, code int, message string, data interface{}) error {
	resp := JSONRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	return s.writeMessage(&resp)
}

// sendToolResult sends a tool result response
func (s *Server) sendToolResult(id interface{}, result interface{}) error {
	content := make([]map[string]interface{}, 0)

	// Convert result to content array
	switch v := result.(type) {
	case string:
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": v,
		})
	case map[string]interface{}:
		// If it's already a map with type field, use it directly
		if _, ok := v["type"]; ok {
			content = append(content, v)
		} else {
			// Otherwise wrap as text with JSON
			jsonBytes, _ := json.MarshalIndent(v, "", "  ")
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": string(jsonBytes),
			})
		}
	default:
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": string(jsonBytes),
		})
	}

	toolResult := map[string]interface{}{
		"content": content,
		"isError": false,
	}

	return s.sendResponse(id, toolResult)
}

// sendToolError sends a tool error response
func (s *Server) sendToolError(id interface{}, err error) error {
	content := []map[string]interface{}{
		{
			"type": "text",
			"text": fmt.Sprintf("Error: %v", err),
		},
	}

	toolResult := map[string]interface{}{
		"content": content,
		"isError": true,
	}

	return s.sendResponse(id, toolResult)
}

// writeMessage writes a JSON-RPC message to stdout
func (s *Server) writeMessage(msg *JSONRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := fmt.Fprintln(s.stdout, string(data)); err != nil {
		return err
	}

	return nil
}

// serveSSE handles SSE transport over HTTP
func (s *Server) serveSSE(port int) error {
	mux := http.NewServeMux()

	// SSE endpoint for server-to-client streaming
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		s.handleSSEConnection(w, r)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// JSON-RPC message endpoint (for client-to-server)
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var msg JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			s.sendHTTPError(w, http.StatusBadRequest, ParseError, "Parse error", err.Error())
			return
		}

		if msg.JSONRPC != jsonRPCVersion {
			s.sendHTTPError(w, http.StatusBadRequest, InvalidRequest, "Invalid JSON-RPC version", nil)
			return
		}

		// Handle the message and return response
		response := s.handleMessageHTTP(&msg)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	s.logger.Printf("MCP SSE server listening on http://localhost:%d\n", port)
	s.logger.Printf("  - SSE endpoint: http://localhost:%d/sse\n", port)
	s.logger.Printf("  - Message endpoint: http://localhost:%d/message\n", port)
	s.logger.Printf("  - Health check: http://localhost:%d/health\n", port)

	return server.ListenAndServe()
}

// handleSSEConnection handles Server-Sent Events connections
func (s *Server) handleSSEConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial endpoint information
	endpoint := map[string]string{
		"event": "endpoint",
		"data":  "/message",
	}
	json.NewEncoder(w).Encode(endpoint)
	flusher.Flush()

	// Keep connection alive and handle messages
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// handleMessageHTTP handles JSON-RPC messages and returns responses for HTTP transport
func (s *Server) handleMessageHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	switch msg.Method {
	case "initialize":
		return s.handleInitializeHTTP(msg)
	case "initialized":
		return s.handleInitializedHTTP(msg)
	case "tools/list":
		return s.handleToolsListHTTP(msg)
	case "tools/call":
		return s.handleToolsCallHTTP(msg)
	case "ping":
		return s.handlePingHTTP(msg)
	default:
		return s.createErrorResponse(msg.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", msg.Method), nil)
	}
}

// HTTP handler variants that return responses
func (s *Server) handleInitializeHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	var params struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		Capabilities    map[string]interface{} `json:"capabilities"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.createErrorResponse(msg.ID, InvalidParams, "Invalid params", err.Error())
	}

	s.logger.Printf("Client connected: %s v%s (protocol: %s)\n",
		params.ClientInfo.Name, params.ClientInfo.Version, params.ProtocolVersion)

	result := map[string]interface{}{
		"protocolVersion": mcpVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
			"logging": map[string]interface{}{},
		},
		"serverInfo": map[string]string{
			"name":    "mozartpay-mcp",
			"version": config.Version,
		},
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleInitializedHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	s.logger.Println("Client initialized")
	return s.createResponse(msg.ID, map[string]interface{}{})
}

func (s *Server) handleToolsListHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleToolsCallHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
		Meta      map[string]interface{} `json:"_meta,omitempty"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.createErrorResponse(msg.ID, InvalidParams, "Invalid params", err.Error())
	}

	// Check if we have all required parameters
	collectionResponse, err := ValidateAndCollectParameters(params.Name, params.Arguments)
	if err != nil {
		return s.createToolErrorResponse(msg.ID, err)
	}

	// If there are missing parameters, return a collection prompt instead of executing
	if collectionResponse != nil {
		result := map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": collectionResponse.ContextMessage + "\n\n" + collectionResponse.Prompt,
				},
			},
			"isError": false,
			"_meta": map[string]interface{}{
				"requiresUserInput": true,
				"missingParams":     collectionResponse.MissingParams,
				"toolName":          collectionResponse.ToolName,
			},
		}
		return s.createResponse(msg.ID, result)
	}

	s.mu.RLock()
	handler, ok := s.handlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		return s.createErrorResponse(msg.ID, MethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}

	ctx := context.Background()
	result, err := handler(ctx, params.Arguments)
	if err != nil {
		return s.createToolErrorResponse(msg.ID, err)
	}

	return s.createToolResultResponse(msg.ID, result)
}

func (s *Server) handlePingHTTP(msg *JSONRPCMessage) *JSONRPCMessage {
	return s.createResponse(msg.ID, map[string]interface{}{})
}

// HTTP response helpers
func (s *Server) createResponse(id interface{}, result interface{}) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
}

func (s *Server) createErrorResponse(id interface{}, code int, message string, data interface{}) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func (s *Server) createToolResultResponse(id interface{}, result interface{}) *JSONRPCMessage {
	content := make([]map[string]interface{}, 0)

	switch v := result.(type) {
	case string:
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": v,
		})
	case map[string]interface{}:
		if _, ok := v["type"]; ok {
			content = append(content, v)
		} else {
			jsonBytes, _ := json.MarshalIndent(v, "", "  ")
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": string(jsonBytes),
			})
		}
	default:
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": string(jsonBytes),
		})
	}

	toolResult := map[string]interface{}{
		"content": content,
		"isError": false,
	}

	return s.createResponse(id, toolResult)
}

func (s *Server) createToolErrorResponse(id interface{}, err error) *JSONRPCMessage {
	content := []map[string]interface{}{
		{
			"type": "text",
			"text": fmt.Sprintf("Error: %v", err),
		},
	}

	toolResult := map[string]interface{}{
		"content": content,
		"isError": true,
	}

	return s.createResponse(id, toolResult)
}

func (s *Server) sendHTTPError(w http.ResponseWriter, statusCode, jsonRPCCode int, message string, data interface{}) {
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&JSONRPCMessage{
		JSONRPC: jsonRPCVersion,
		Error: &JSONRPCError{
			Code:    jsonRPCCode,
			Message: message,
			Data:    data,
		},
	})
}

// registerTools registers all available tools
func (s *Server) registerTools() {
	s.registerWalletTools()
	s.registerSwapTools()
	s.registerAssetTools()
	s.registerPayTools()
	s.registerSystemTools()
	s.registerTansuTools()
}
