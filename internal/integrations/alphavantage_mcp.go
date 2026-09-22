package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AlphaVantageMCPClient implements an MCP client for Alpha Vantage's MCP server
type AlphaVantageMCPClient struct {
	apiKey      string
	httpClient  *http.Client
	baseURL     string
	initialized bool
	tools       []MCPTool
}

// MCPMessage represents a JSON-RPC 2.0 message
type MCPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPTool represents an MCP tool definition
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event string
	Data  string
}

// NewAlphaVantageMCPClient creates a new MCP client for Alpha Vantage
func NewAlphaVantageMCPClient(apiKey string) *AlphaVantageMCPClient {
	return &AlphaVantageMCPClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:     "https://mcp.alphavantage.co",
		initialized: false,
	}
}

// Initialize connects to the MCP server and discovers available tools
func (c *AlphaVantageMCPClient) Initialize() error {
	if c.apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// For Alpha Vantage MCP, we can use a simplified REST-like approach
	// as they support both SSE and direct tool calling
	c.initialized = true

	// Discover available tools by calling tools/list
	tools, err := c.listTools()
	if err != nil {
		// If tools/list fails, assume NEWS_SENTIMENT is available
		// Alpha Vantage's MCP may not expose tools/list in the same way
		c.tools = []MCPTool{
			{
				Name:        "NEWS_SENTIMENT",
				Description: "Fetches news and sentiment data for specified keywords",
				InputSchema: json.RawMessage(`{"type": "object", "properties": {"keywords": {"type": "string"}, "limit": {"type": "integer"}}}`),
			},
		}
	} else {
		c.tools = tools
	}

	return nil
}

// listTools discovers available tools from the MCP server
func (c *AlphaVantageMCPClient) listTools() ([]MCPTool, error) {
	url := fmt.Sprintf("%s/mcp?apikey=%s", c.baseURL, c.apiKey)

	reqBody := MCPMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort close

	var result struct {
		Tools []MCPTool `json:"tools"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Tools, nil
}

// ToolCall executes an MCP tool call using Alpha Vantage's TOOL_CALL wrapper
func (c *AlphaVantageMCPClient) ToolCall(function string, kwargs map[string]interface{}) (*MCPMessage, error) {
	if !c.initialized {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	url := fmt.Sprintf("%s/mcp?apikey=%s", c.baseURL, c.apiKey)

	// Alpha Vantage uses TOOL_CALL wrapper with function and kwargs
	params := map[string]interface{}{
		"function": function,
		"kwargs":   kwargs,
	}

	paramsData, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	reqBody := MCPMessage{
		JSONRPC: "2.0",
		ID:      time.Now().Unix(),
		Method:  "TOOL_CALL",
		Params:  paramsData,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort close

	var response MCPMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// FetchNewsMCP fetches news using the NEWS_SENTIMENT tool via MCP
func (c *AlphaVantageMCPClient) FetchNewsMCP(tickers string, limit int) ([]NewsArticle, error) {
	if !c.initialized {
		if err := c.Initialize(); err != nil {
			return nil, err
		}
	}

	// Call the NEWS_SENTIMENT tool using TOOL_CALL wrapper
	kwargs := map[string]interface{}{
		"tickers": tickers,
		"sort":    "LATEST",
		"limit":   limit,
	}

	response, err := c.ToolCall("NEWS_SENTIMENT", kwargs)
	if err != nil {
		return nil, fmt.Errorf("NEWS_SENTIMENT tool call failed: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", response.Error.Message)
	}

	// Parse the result
	return c.parseNewsResponse(response.Result)
}

// parseNewsResponse converts MCP response to NewsArticle slice
func (c *AlphaVantageMCPClient) parseNewsResponse(result interface{}) ([]NewsArticle, error) {
	if result == nil {
		return nil, fmt.Errorf("empty result")
	}

	// Handle different response formats
	switch v := result.(type) {
	case map[string]interface{}:
		// Check for content array (MCP standard format)
		if content, ok := v["content"].([]interface{}); ok {
			var articles []NewsArticle
			for _, item := range content {
				if contentItem, ok := item.(map[string]interface{}); ok {
					if contentType, ok := contentItem["type"].(string); ok && contentType == "text" {
						// Try to parse the text as JSON
						text, _ := contentItem["text"].(string)
						var feedData struct {
							Feed []struct {
								Title         string `json:"title"`
								URL           string `json:"url"`
								Summary       string `json:"summary"`
								Source        string `json:"source"`
								TimePublished string `json:"time_published"`
							} `json:"feed"`
						}
						if err := json.Unmarshal([]byte(text), &feedData); err == nil {
							for _, item := range feedData.Feed {
								publishedTime, perr := time.Parse("20060102T150405", item.TimePublished)
								if perr != nil {
									publishedTime = time.Time{}
								}
								articles = append(articles, NewsArticle{
									Title:       item.Title,
									URL:         item.URL,
									Summary:     truncateSummary(item.Summary),
									Source:      item.Source,
									PublishedAt: publishedTime,
									Relevance:   0.8, // Default relevance for MCP results
								})
							}
						}
					}
				}
			}
			return articles, nil
		}

		// Check for direct feed format (similar to REST API)
		if feed, ok := v["feed"].([]interface{}); ok {
			return c.parseFeedArray(feed)
		}

	case []interface{}:
		return c.parseFeedArray(v)
	}

	return nil, fmt.Errorf("unexpected response format")
}

// parseFeedArray converts feed array to NewsArticle slice
func (c *AlphaVantageMCPClient) parseFeedArray(feed []interface{}) ([]NewsArticle, error) {
	var articles []NewsArticle

	for _, item := range feed {
		if feedItem, ok := item.(map[string]interface{}); ok {
			article := NewsArticle{
				Title:     getString(feedItem, "title"),
				URL:       getString(feedItem, "url"),
				Summary:   truncateSummary(getString(feedItem, "summary")),
				Source:    getString(feedItem, "source"),
				Relevance: 0.8,
			}

			if timeStr := getString(feedItem, "time_published"); timeStr != "" {
				if t, err := time.Parse("20060102T150405", timeStr); err == nil {
					article.PublishedAt = t
				}
			}

			articles = append(articles, article)
		}
	}

	return articles, nil
}

// getString safely extracts a string from a map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// truncateSummary limits summary length for display
func truncateSummary(summary string) string {
	if len(summary) > 150 {
		return summary[:147] + "..."
	}
	return summary
}

// IsInitialized returns whether the client has been initialized
func (c *AlphaVantageMCPClient) IsInitialized() bool {
	return c.initialized
}
