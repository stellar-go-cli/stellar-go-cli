// Package mcp implements the Model Context Protocol server for Stellar Go CLI.
// It exposes CLI functionality as MCP tools via stdio or SSE transport using JSON-RPC 2.0.
package mcp

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chainlog"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

const (
	jsonRPCVersion = "2.0"
	mcpVersion     = "2024-11-05"

	defaultSSEBind = "127.0.0.1"
)

// transportCtxKey marks which transport dispatched a tool call. Handlers use
// it to restrict sensitive operations on the network-exposed HTTP transport.
type transportCtxKey struct{}

const transportHTTP = "http"

func isHTTPTransport(ctx context.Context) bool {
	t, _ := ctx.Value(transportCtxKey{}).(string)
	return t == transportHTTP
}

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
	cfg      *config.Config
	tools    map[string]Tool
	handlers map[string]ToolHandler
	logger   *log.Logger
	mu       sync.RWMutex
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	profile  string
	// Operator-controlled policy — see OperatorOptions. Agent arguments can
	// never widen these.
	x402RPCURL            string
	x402MaxAtomicAmount   string
	allowPrivateResources bool
	allowedOrigins        []string
	custodianSock         string
	audit                 *chainlog.Recorder
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
		cfg:      cfg,
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		logger:   log.New(os.Stderr, "[mcp] ", log.LstdFlags),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
	}
}

// RegisterTool registers a tool with its handler
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// Start begins the MCP server with the specified transport. For the "sse"
// transport, bind selects the listen address (loopback by default) and token,
// when non-empty, requires Authorization: Bearer <token> on all endpoints
// except /health. Binding a non-loopback address without a token is refused.
func (s *Server) Start(transport string, port int, bind, token string) error {
	s.logger.Printf("Starting MCP server (transport: %s)\n", transport)

	// Open the append-only, hash-chained tools/call audit log — the
	// gatekeeper-side evidence record. Failing to open it fails startup:
	// an agent-facing dispatch without audit is not an acceptable mode.
	recorder, _, err := openAuditRecorder()
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	s.audit = recorder

	// Register all tools, then filter by the operator's profile
	s.registerTools()
	s.applyProfile()

	switch transport {
	case "stdio":
		return s.serveStdio()
	case "sse":
		return s.serveSSE(port, bind, token)
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
			// #nosec G104 -- client connection may already be dead
			s.sendError(nil, ParseError, "Parse error", err.Error()) //nolint:errcheck
			continue
		}

		if msg.JSONRPC != jsonRPCVersion {
			// #nosec G104 -- client connection may already be dead
			s.sendError(msg.ID, InvalidRequest, "Invalid JSON-RPC version", nil) //nolint:errcheck
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
	case "initialized", "notifications/initialized":
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
			"name":    "stellar-go-cli-mcp",
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

// handleToolsCall executes a tool call on the stdio transport. The ctx has no
// transport marker — handlers treat it as the operator-local stdio channel.
func (s *Server) handleToolsCall(msg *JSONRPCMessage) error {
	return s.writeMessage(s.callTool(context.Background(), msg))
}

// callTool is the single dispatch choke point for tools/call on both
// transports: schema validation → missing-parameter collection prompt →
// handler. It returns a complete JSON-RPC response message so the stdio and
// HTTP paths can never diverge in validation or dispatch.
func (s *Server) callTool(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
		Meta      map[string]interface{} `json:"_meta,omitempty"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.recordCall(transportOf(ctx), "", "deny_invalid_params", nil, "")
		return s.createErrorResponse(msg.ID, InvalidParams, "Invalid params", err.Error())
	}

	s.mu.RLock()
	tool, ok := s.tools[params.Name]
	handler := s.handlers[params.Name]
	s.mu.RUnlock()

	if !ok || handler == nil {
		s.recordCall(transportOf(ctx), params.Name, "deny_unknown_tool", params.Arguments, "")
		return s.createErrorResponse(msg.ID, MethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}

	// Validate against the tool's registered InputSchema (fail-closed: every
	// registered tool carries a schema, so there is no pass-through path).
	collectionResponse, err := validateAndCollect(tool, params.Arguments)
	if err != nil {
		s.recordCall(transportOf(ctx), params.Name, "deny_invalid_args", params.Arguments, "")
		return s.createToolErrorResponse(msg.ID, err)
	}

	// If there are missing parameters, return a collection prompt instead of executing
	if collectionResponse != nil {
		s.recordCall(transportOf(ctx), params.Name, "collect_params", params.Arguments, "")
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

	// Mint the decision ref: the audit record's link to any custodian signing
	// request this call produces.
	decisionRef := mintDecisionRef(transportOf(ctx), params.Name, params.Arguments)
	ctx = context.WithValue(ctx, decisionCtxKey{}, decisionRef)

	result, err := handler(ctx, params.Arguments)
	if err != nil {
		s.recordCall(transportOf(ctx), params.Name, "error", params.Arguments, decisionRef)
		return s.createToolErrorResponse(msg.ID, err)
	}

	s.recordCall(transportOf(ctx), params.Name, "allow", params.Arguments, decisionRef)
	return s.createToolResultResponse(msg.ID, result)
}

// transportOf maps the dispatch context back to a transport label for audit.
func transportOf(ctx context.Context) string {
	if isHTTPTransport(ctx) {
		return transportHTTP
	}
	return "stdio"
}

// recordCall writes one audit entry; a write failure is logged, never silent.
// decisionRef links the record to custodian signing requests ("" for denials
// that never reach a handler).
func (s *Server) recordCall(transport, tool, outcome string, args map[string]interface{}, decisionRef string) {
	if s.audit == nil {
		return
	}
	if _, err := recordAudit(s.audit, tool, transport, outcome, decisionRef, args); err != nil {
		s.logger.Printf("audit record failed (%s %s): %v\n", tool, outcome, err)
	}
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
func (s *Server) serveSSE(port int, bind, token string) error {
	if bind == "" {
		bind = defaultSSEBind
	}
	loopback, err := isLoopbackBind(bind)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}
	if !loopback && token == "" {
		return fmt.Errorf("refusing to bind %s without authentication — pass --token (or set MCP_TOKEN) to expose the MCP server beyond loopback", bind)
	}

	mux := http.NewServeMux()

	// SSE endpoint for server-to-client streaming
	mux.HandleFunc("/sse", s.guardTransportRequest(bind, s.requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
		s.handleSSEConnection(w, r)
	})))

	// Health check endpoint (unauthenticated: exposes only a static status)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// #nosec G104 -- best-effort response write
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

	// JSON-RPC message endpoint (for client-to-server)
	mux.HandleFunc("/message", s.guardTransportRequest(bind, s.requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
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

		// Handle the message and return response — propagate the request
		// context so cancellation reaches the tool handler.
		response := s.handleMessageHTTP(r.Context(), &msg)
		w.Header().Set("Content-Type", "application/json")
		// #nosec G104 -- best-effort response write
		json.NewEncoder(w).Encode(response) //nolint:errcheck
	})))

	server := &http.Server{
		Addr:              net.JoinHostPort(bind, strconv.Itoa(port)),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	base := fmt.Sprintf("http://%s:%d", bind, port)
	s.logger.Printf("MCP SSE server listening on %s\n", base)
	s.logger.Printf("  - SSE endpoint: %s/sse\n", base)
	s.logger.Printf("  - Message endpoint: %s/message\n", base)
	s.logger.Printf("  - Health check: %s/health\n", base)
	if token != "" {
		s.logger.Println("  - Bearer token authentication enabled")
	} else {
		s.logger.Println("  - No auth token configured (loopback only)")
	}

	return server.ListenAndServe()
}

// requireAuth wraps a handler with Bearer-token authentication. An empty
// token disables the check (only permitted on loopback binds).
func (s *Server) requireAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	if token == "" {
		return next
	}
	const prefix = "Bearer "
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, prefix)), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// isLoopbackBind reports whether a bind address resolves exclusively to
// loopback interfaces.
func isLoopbackBind(addr string) (bool, error) {
	host := strings.TrimSpace(addr)
	if host == "" {
		return false, nil
	}
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback(), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false, fmt.Errorf("cannot resolve bind host %q", host)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false, nil
		}
	}
	return true, nil
}

// guardTransportRequest enforces Host and Origin validation on the
// network-facing endpoints — the DNS-rebinding and cross-origin defense.
// Host must target the bind address or loopback (or an operator-allowlisted
// name); Origin, when present, must be loopback or allowlisted.
func (s *Server) guardTransportRequest(bind string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host, bind) {
			http.Error(w, "Forbidden: unexpected Host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !s.originAllowed(origin) {
			http.Error(w, "Forbidden: origin not allowed", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// hostAllowed reports whether the request's Host header targets this server:
// loopback, the configured bind host, or an operator allowlist entry.
func (s *Server) hostAllowed(host, bind string) bool {
	h := normalizeOriginHost(host)
	if h == "" {
		return false
	}
	if isLoopbackHostname(h) || strings.EqualFold(h, normalizeOriginHost(bind)) {
		return true
	}
	for _, allowed := range s.allowedOrigins {
		if strings.EqualFold(h, normalizeOriginHost(allowed)) {
			return true
		}
	}
	return false
}

// originAllowed reports whether a request Origin is loopback or on the
// operator allowlist.
func (s *Server) originAllowed(origin string) bool {
	h := normalizeOriginHost(origin)
	if h == "" {
		return false
	}
	if isLoopbackHostname(h) {
		return true
	}
	for _, allowed := range s.allowedOrigins {
		if strings.EqualFold(h, normalizeOriginHost(allowed)) {
			return true
		}
	}
	return false
}

// normalizeOriginHost reduces a Host header, host[:port], or full origin URL
// to a lowercase hostname.
func normalizeOriginHost(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if u, err := url.Parse(v); err == nil && u.Host != "" {
		v = u.Host
	}
	if h, _, err := net.SplitHostPort(v); err == nil {
		v = h
	}
	return strings.ToLower(strings.Trim(v, "[]"))
}

// isLoopbackHostname reports whether a hostname resolves to loopback without
// a DNS lookup: "localhost" or a literal loopback IP.
func isLoopbackHostname(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// handleSSEConnection handles Server-Sent Events connections
func (s *Server) handleSSEConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS is restricted, never wildcard: echo the origin only when the guard
	// already accepted it (loopback or operator allowlist).
	if origin := r.Header.Get("Origin"); origin != "" && s.originAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}

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
	// #nosec G104 -- best-effort response write
	json.NewEncoder(w).Encode(endpoint) //nolint:errcheck
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
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n") //nolint:errcheck // best-effort SSE keep-alive
			flusher.Flush()
		}
	}
}

// handleMessageHTTP handles JSON-RPC messages and returns responses for HTTP transport
func (s *Server) handleMessageHTTP(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	switch msg.Method {
	case "initialize":
		return s.handleInitializeHTTP(msg)
	case "initialized", "notifications/initialized":
		return s.handleInitializedHTTP(msg)
	case "tools/list":
		return s.handleToolsListHTTP(msg)
	case "tools/call":
		return s.handleToolsCallHTTP(ctx, msg)
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
			"name":    "stellar-go-cli-mcp",
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

func (s *Server) handleToolsCallHTTP(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	// Mark the request context as arriving over HTTP so handlers can restrict
	// transport-sensitive operations; the request context itself is preserved
	// so client cancellation propagates into the handler.
	ctx = context.WithValue(ctx, transportCtxKey{}, transportHTTP)
	return s.callTool(ctx, msg)
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
			jsonBytes, _ := json.MarshalIndent(v, "", "  ") //nolint:errcheck // marshal failure yields empty text fallback
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": string(jsonBytes),
			})
		}
	default:
		jsonBytes, _ := json.MarshalIndent(result, "", "  ") //nolint:errcheck // marshal failure yields empty text fallback
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
	// #nosec G104 -- best-effort response write
	json.NewEncoder(w).Encode(&JSONRPCMessage{ //nolint:errcheck
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
