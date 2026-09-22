package mcp

import (
	"fmt"
	"strings"
)

// ParameterInfo contains metadata about a tool parameter
type ParameterInfo struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Required    bool                   `json:"required"`
	Description string                 `json:"description"`
	Default     interface{}            `json:"default,omitempty"`
	Enum        []string               `json:"enum,omitempty"`
	Examples    []string               `json:"examples,omitempty"`
	Validation  map[string]interface{} `json:"validation,omitempty"`
}

// PendingOperation tracks operations waiting for missing parameters
type PendingOperation struct {
	ToolName      string                 `json:"tool_name"`
	ProvidedArgs  map[string]interface{} `json:"provided_args"`
	MissingParams []ParameterInfo        `json:"missing_params"`
	Timestamp     int64                  `json:"timestamp"`
	SessionID     string                 `json:"session_id"`
}

// ParameterCollectionResponse represents a response asking for missing parameters
type ParameterCollectionResponse struct {
	Type           string          `json:"type"` // "missing_parameters"
	ToolName       string          `json:"tool_name"`
	MissingParams  []ParameterInfo `json:"missing_params"`
	Prompt         string          `json:"prompt"`
	ContextMessage string          `json:"context_message"`
	CanProceed     bool            `json:"can_proceed"` // false until all params provided
}

// ParameterSchemas defines all tool parameter schemas for validation
var ParameterSchemas = map[string][]ParameterInfo{
	"wallet_connect": {
		{Name: "provider", Type: "string", Required: false, Description: "Wallet provider: wwwallet | stellar | external", Default: "wwwallet", Enum: []string{"wwwallet", "stellar", "external"}},
		{Name: "network", Type: "string", Required: false, Description: "Network: stellar-testnet | stellar-mainnet | evm-sepolia | evm-mainnet", Default: "stellar-testnet", Enum: []string{"stellar-testnet", "stellar-mainnet", "evm-sepolia", "evm-mainnet"}},
		{Name: "address", Type: "string", Required: false, Description: "External address for external provider", Examples: []string{"GD5DQYPNQFSX4TVWV5EHG63IZW5KQ5B4R6J3AY3ARJ2V2QZVX6A3Q..."}},
	},
	"wallet_import": {
		{Name: "secret", Type: "string", Required: true, Description: "Secret key or recovery phrase", Examples: []string{"S...", "word1 word2 word3..."}},
		{Name: "name", Type: "string", Required: false, Description: "Wallet name for easy identification", Examples: []string{"My Trading Wallet", "Savings Account"}},
		{Name: "network", Type: "string", Required: false, Description: "Network", Default: "stellar-testnet", Enum: []string{"stellar-testnet", "stellar-mainnet"}},
	},
	"wallet_fund": {
		{Name: "address", Type: "string", Required: false, Description: "Wallet address to fund (uses active if omitted)"},
		{Name: "network", Type: "string", Required: false, Description: "Network", Default: "stellar-testnet", Enum: []string{"stellar-testnet", "stellar-mainnet"}},
	},
	"wallet_switch": {
		{Name: "address", Type: "string", Required: true, Description: "Wallet address to switch to", Examples: []string{"GD5DQYPNQFSX4TVWV5EHG63IZW5KQ5B4R6J3AY3ARJ2V2QZVX6A3Q..."}},
	},
	"wallet_rename": {
		{Name: "address", Type: "string", Required: true, Description: "Wallet address to rename"},
		{Name: "name", Type: "string", Required: true, Description: "New wallet name", Examples: []string{"Trading Wallet", "Main Account"}},
	},
	"wallet_remove": {
		{Name: "address", Type: "string", Required: true, Description: "Wallet address to remove"},
		{Name: "confirm", Type: "boolean", Required: false, Description: "Confirm removal for safety"},
	},
	"wallet_export": {
		{Name: "address", Type: "string", Required: false, Description: "Wallet address to export (uses active if omitted)"},
		{Name: "confirm", Type: "boolean", Required: false, Description: "Confirm export for security"},
	},
	"wallet_passkey": {
		{Name: "action", Type: "string", Required: false, Description: "Action: register | verify | remove", Default: "register", Enum: []string{"register", "verify", "remove"}},
		{Name: "address", Type: "string", Required: false, Description: "Wallet address (uses active if omitted)"},
	},
	"swap_quote": {
		{Name: "from", Type: "string", Required: true, Description: "Source asset code", Examples: []string{"XLM", "USDC", "EURC"}},
		{Name: "to", Type: "string", Required: true, Description: "Destination asset code", Examples: []string{"USDC", "EURC", "BTC"}},
		{Name: "amount", Type: "string", Required: true, Description: "Amount to swap", Examples: []string{"100", "50.5"}},
		{Name: "type", Type: "string", Required: false, Description: "Swap type", Default: "strict-send", Enum: []string{"strict-send", "strict-receive"}},
	},
	"swap_execute": {
		{Name: "from", Type: "string", Required: true, Description: "Source asset code"},
		{Name: "to", Type: "string", Required: true, Description: "Destination asset code"},
		{Name: "amount", Type: "string", Required: true, Description: "Amount to swap"},
		{Name: "type", Type: "string", Required: false, Description: "Swap type", Default: "strict-send", Enum: []string{"strict-send", "strict-receive"}},
		{Name: "slippage", Type: "number", Required: false, Description: "Maximum slippage percentage"},
		{Name: "execute", Type: "boolean", Required: false, Description: "Actually execute the transaction", Default: true},
	},
	"pay_send": {
		{Name: "destination", Type: "string", Required: true, Description: "Recipient Stellar address", Examples: []string{"GD5DQYPNQFSX4TVWV5EHG63IZW5KQ5B4R6J3AY3ARJ2V2QZVX6A3Q..."}},
		{Name: "amount", Type: "string", Required: true, Description: "Amount to send", Examples: []string{"100", "50.5"}},
		{Name: "asset", Type: "string", Required: false, Description: "Asset code", Default: "XLM", Examples: []string{"XLM", "USDC", "EURC"}},
		{Name: "rail", Type: "string", Required: false, Description: "Payment rail", Default: "direct", Enum: []string{"direct", "x402", "tempo", "zk"}},
		{Name: "memo", Type: "string", Required: false, Description: "Optional payment memo", Examples: []string{"Invoice #123", "Rent payment"}},
	},
	"pay_quote": {
		{Name: "to", Type: "string", Required: true, Description: "Recipient address"},
		{Name: "amount", Type: "string", Required: true, Description: "Amount to send"},
		{Name: "asset", Type: "string", Required: true, Description: "Asset code"},
		{Name: "rail", Type: "string", Required: false, Description: "Payment rail", Default: "direct", Enum: []string{"direct", "x402", "tempo", "zk"}},
	},
	"pay_request": {
		{Name: "asset", Type: "string", Required: true, Description: "Asset code to request"},
		{Name: "amount", Type: "string", Required: true, Description: "Amount to request"},
		{Name: "memo", Type: "string", Required: false, Description: "Optional memo"},
	},
	"asset_create_ft": {
		{Name: "name", Type: "string", Required: true, Description: "Token name", Examples: []string{"MyToken", "Reward Token"}},
		{Name: "symbol", Type: "string", Required: true, Description: "Token symbol (max 12 chars)", Examples: []string{"MTK", "RWD"}},
		{Name: "supply", Type: "string", Required: false, Description: "Total supply", Default: "1000000"},
		{Name: "decimals", Type: "number", Required: false, Description: "Decimal places", Default: 7},
		{Name: "network", Type: "string", Required: false, Description: "Network", Default: "stellar-testnet", Enum: []string{"stellar-testnet", "stellar-mainnet"}},
		{Name: "with_carbon", Type: "boolean", Required: false, Description: "Attach carbon credits"},
		{Name: "carbon_amt", Type: "number", Required: false, Description: "Carbon credit amount in tCO2e"},
	},
	"asset_trust": {
		{Name: "code", Type: "string", Required: true, Description: "Asset code", Examples: []string{"USDC", "EURC"}},
		{Name: "issuer", Type: "string", Required: true, Description: "Asset issuer address"},
		{Name: "limit", Type: "string", Required: false, Description: "Trustline limit"},
		{Name: "execute", Type: "boolean", Required: false, Description: "Execute the transaction", Default: true},
	},
}

// ValidateAndCollectParameters checks for missing parameters and returns a collection response if needed
func ValidateAndCollectParameters(toolName string, providedArgs map[string]interface{}) (*ParameterCollectionResponse, error) {
	schema, exists := ParameterSchemas[toolName]
	if !exists {
		// No schema defined, allow execution
		return nil, nil
	}

	var missingParams []ParameterInfo

	for _, param := range schema {
		if !param.Required {
			continue // Skip optional parameters
		}

		// Check if required parameter is provided
		value, exists := providedArgs[param.Name]
		if !exists || value == nil || value == "" {
			missingParams = append(missingParams, param)
		}
	}

	// If no missing parameters, return nil to indicate execution can proceed
	if len(missingParams) == 0 {
		return nil, nil
	}

	// Generate conversational prompt
	prompt := generateParameterPrompt(toolName, missingParams)
	contextMsg := generateContextMessage(toolName)

	return &ParameterCollectionResponse{
		Type:           "missing_parameters",
		ToolName:       toolName,
		MissingParams:  missingParams,
		Prompt:         prompt,
		ContextMessage: contextMsg,
		CanProceed:     false,
	}, nil
}

// generateParameterPrompt creates a user-friendly prompt for missing parameters
func generateParameterPrompt(toolName string, missingParams []ParameterInfo) string {
	var prompts []string

	for _, param := range missingParams {
		question := generateParameterQuestion(param)
		prompts = append(prompts, question)
	}

	return strings.Join(prompts, "\n\n")
}

// generateParameterQuestion creates a single parameter question
func generateParameterQuestion(param ParameterInfo) string {
	baseQuestion := ""

	switch param.Name {
	case "destination", "to", "address":
		baseQuestion = fmt.Sprintf("📍 Who would you like to send to? (%s)", param.Description)
	case "amount":
		baseQuestion = fmt.Sprintf("💰 How much would you like to send? (%s)", param.Description)
	case "from":
		baseQuestion = fmt.Sprintf("🏦 What asset are you sending from? (%s)", param.Description)
	case "asset":
		baseQuestion = fmt.Sprintf("🪙 Which asset? (%s)", param.Description)
	case "secret":
		baseQuestion = fmt.Sprintf("🔐 Please provide your %s", param.Description)
	case "code", "symbol":
		baseQuestion = fmt.Sprintf("🏷️ What's the %s?", param.Description)
	case "name":
		baseQuestion = fmt.Sprintf("📛 What %s?", param.Description)
	case "provider":
		baseQuestion = fmt.Sprintf("📱 Which %s?", param.Description)
	default:
		baseQuestion = fmt.Sprintf("❓ Please provide %s (%s)", param.Name, param.Description)
	}

	// Add examples if available
	if len(param.Examples) > 0 {
		baseQuestion += fmt.Sprintf("\n   Example: %s", strings.Join(param.Examples, " or "))
	}

	// Add enum options if available
	if len(param.Enum) > 0 {
		baseQuestion += fmt.Sprintf("\n   Options: %s", strings.Join(param.Enum, ", "))
	}

	return baseQuestion
}

// generateContextMessage creates a helpful context message for the operation
func generateContextMessage(toolName string) string {
	messages := map[string]string{
		"wallet_connect":  "💼 I can help you connect a wallet! Let me get a few details:",
		"wallet_import":   "🔑 I'll help you import your wallet. I need some information:",
		"wallet_fund":     "💸 Let's fund your wallet with testnet XLM!",
		"wallet_switch":   "🔄 I can switch your active wallet. Which one?",
		"wallet_rename":   "✏️ Let's rename your wallet. What would you like to call it?",
		"swap_quote":      "💱 I'll get you a swap quote! I need to know what you're swapping:",
		"swap_execute":    "🔄 Ready to execute a swap! Let me confirm the details:",
		"pay_send":        "💸 I'll help you send a payment! I need a few details:",
		"pay_quote":       "📊 I'll get you a payment quote! What are the details?",
		"pay_request":     "📋 I'll create a payment request for you! What do you need?",
		"asset_create_ft": "🎨 Let's create your token! I need some information:",
		"asset_trust":     "🔒 I'll help you establish a trustline! Which asset?",
	}

	if msg, exists := messages[toolName]; exists {
		return msg
	}

	return fmt.Sprintf("🤖 I'm ready to help with %s! I need some information:", strings.ReplaceAll(toolName, "_", " "))
}

// HasRequiredParams checks if all required parameters are present for a tool
func HasRequiredParams(toolName string, args map[string]interface{}) bool {
	response, _ := ValidateAndCollectParameters(toolName, args) //nolint:errcheck // nil response means no collection needed
	return response == nil
}

// GetParameterHelp returns helpful information about a specific parameter
func GetParameterHelp(toolName, paramName string) string {
	schema, exists := ParameterSchemas[toolName]
	if !exists {
		return fmt.Sprintf("Parameter '%s' information not available", paramName)
	}

	for _, param := range schema {
		if param.Name == paramName {
			help := param.Description
			if len(param.Examples) > 0 {
				help += fmt.Sprintf("\nExamples: %s", strings.Join(param.Examples, ", "))
			}
			if len(param.Enum) > 0 {
				help += fmt.Sprintf("\nValid options: %s", strings.Join(param.Enum, ", "))
			}
			return help
		}
	}

	return fmt.Sprintf("Parameter '%s' not found in %s", paramName, toolName)
}
