package mcp

import (
	"encoding/json"
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
		{Name: "provider", Type: "string", Required: false, Description: "Wallet provider: stellar | external", Default: "stellar", Enum: []string{"stellar", "external"}},
		{Name: "network", Type: "string", Required: false, Description: "Network: stellar-testnet | stellar-mainnet | evm-sepolia | evm-mainnet", Default: "stellar-testnet", Enum: []string{"stellar-testnet", "stellar-mainnet", "evm-sepolia", "evm-mainnet"}},
		{Name: "address", Type: "string", Required: false, Description: "External address for external provider", Examples: []string{"GD5DQYPNQFSX4TVWV5EHG63IZW5KQ5B4R6J3AY3ARJ2V2QZVX6A3Q..."}},
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
	"wallet_passkey": {
		{Name: "action", Type: "string", Required: false, Description: "Action: register | unlock | lock | status | disable", Default: "status", Enum: []string{"register", "unlock", "lock", "status", "disable"}},
		{Name: "address", Type: "string", Required: false, Description: "Wallet address (uses active if omitted)"},
		{Name: "ttl_minutes", Type: "string", Required: false, Description: "Unlock duration in minutes (unlock only)", Default: "15"},
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
		{Name: "execute", Type: "boolean", Required: false, Description: "Actually execute the transaction", Default: false},
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
		{Name: "execute", Type: "boolean", Required: false, Description: "Execute the transaction", Default: false},
	},
}

// inputSchemaProperty is the subset of a JSON Schema property used for
// fail-closed tools/call validation.
type inputSchemaProperty struct {
	Type        string        `json:"type"`
	Description string        `json:"description"`
	Enum        []interface{} `json:"enum"`
	Default     interface{}   `json:"default"`
}

// toolInputSchema is the registered InputSchema of a tool — the single
// source of truth for required parameters and argument types.
type toolInputSchema struct {
	Type       string                         `json:"type"`
	Required   []string                       `json:"required"`
	Properties map[string]inputSchemaProperty `json:"properties"`
}

// validateAndCollect checks a tools/call request against the tool's own
// registered InputSchema. Unlike the legacy name-keyed ParameterSchemas
// lookup this is fail-closed: every registered tool must carry a valid
// object schema, so there is no unvalidated dispatch path. ParameterSchemas
// remains only as prompt-presentation metadata (examples, context strings).
func validateAndCollect(tool Tool, providedArgs map[string]interface{}) (*ParameterCollectionResponse, error) {
	if len(tool.InputSchema) == 0 {
		return nil, fmt.Errorf("tool %q has no input schema — refusing to dispatch", tool.Name)
	}
	var schema toolInputSchema
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		return nil, fmt.Errorf("tool %q has an invalid input schema: %w", tool.Name, err)
	}
	if schema.Type != "object" {
		return nil, fmt.Errorf("tool %q input schema must declare type \"object\"", tool.Name)
	}
	if providedArgs == nil {
		providedArgs = map[string]interface{}{}
	}

	// Type and enum checks on every provided argument.
	for name, value := range providedArgs {
		prop, declared := schema.Properties[name]
		if !declared {
			continue // JSON Schema default: additional properties allowed
		}
		if err := checkArgType(tool.Name, name, value, prop); err != nil {
			return nil, err
		}
	}

	// Required-parameter check, in declared order for stable prompts.
	var missing []ParameterInfo
	for _, name := range schema.Required {
		if v, ok := providedArgs[name]; ok && v != nil && v != "" {
			continue
		}
		info := parameterInfoFor(tool.Name, name, schema.Properties[name])
		missing = append(missing, info)
	}

	if len(missing) == 0 {
		return nil, nil
	}
	return &ParameterCollectionResponse{
		Type:           "missing_parameters",
		ToolName:       tool.Name,
		MissingParams:  missing,
		Prompt:         generateParameterPrompt(tool.Name, missing),
		ContextMessage: generateContextMessage(tool.Name),
		CanProceed:     false,
	}, nil
}

// checkArgType validates one argument against its declared JSON Schema
// property: primitive type plus enum membership.
func checkArgType(toolName, name string, value interface{}, prop inputSchemaProperty) error {
	typeOK := true
	switch prop.Type {
	case "string":
		_, typeOK = value.(string)
	case "number", "integer":
		switch value.(type) {
		case float64, float32, int, int64, json.Number:
		default:
			typeOK = false
		}
	case "boolean":
		_, typeOK = value.(bool)
	case "object":
		_, typeOK = value.(map[string]interface{})
	case "array":
		_, typeOK = value.([]interface{})
	case "":
		// No declared type — nothing to check.
	}
	if !typeOK {
		return fmt.Errorf("%s: parameter %q must be of type %s", toolName, name, prop.Type)
	}
	if len(prop.Enum) == 0 {
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return nil // enum applies to string values
	}
	for _, e := range prop.Enum {
		if es, ok := e.(string); ok && es == str {
			return nil
		}
	}
	return fmt.Errorf("%s: parameter %q must be one of the declared enum values", toolName, name)
}

// parameterInfoFor builds prompt-facing ParameterInfo for a missing required
// parameter, preferring the registered schema and enriching it with the
// legacy ParameterSchemas metadata (examples, descriptions) when present.
func parameterInfoFor(toolName, name string, prop inputSchemaProperty) ParameterInfo {
	info := ParameterInfo{
		Name:        name,
		Type:        prop.Type,
		Required:    true,
		Description: prop.Description,
		Default:     prop.Default,
	}
	for _, e := range prop.Enum {
		if es, ok := e.(string); ok {
			info.Enum = append(info.Enum, es)
		}
	}
	for _, mp := range ParameterSchemas[toolName] {
		if mp.Name == name {
			if info.Description == "" {
				info.Description = mp.Description
			}
			info.Examples = mp.Examples
			break
		}
	}
	return info
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
