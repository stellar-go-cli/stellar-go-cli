// Package chat provides an interactive chat interface for Stellar Go CLI
package chat

import (
	"fmt"
	"strings"
)

// PromptBuilder creates structured prompts for LLM inference
type PromptBuilder struct{}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// IntentClassificationPrompt creates a prompt for intent classification
func (p *PromptBuilder) IntentClassificationPrompt(message string) string {
	var b strings.Builder

	b.WriteString("You are an intent classifier for a cryptocurrency payment CLI called Stellar Go CLI.\n")
	b.WriteString("Analyze the user message and classify it into exactly one of these intents:\n\n")

	intents := []struct {
		name        string
		description string
		examples    []string
	}{
		{"greeting", "User greets the assistant", []string{"hello", "hi", "hey", "good morning"}},
		{"help", "User asks for help or available commands", []string{"help", "what can you do", "commands"}},
		{"quit", "User wants to exit", []string{"quit", "exit", "bye", "goodbye"}},
		{"cancel_operation", "User wants to cancel current operation", []string{"cancel", "stop", "abort"}},
		{"wallet_list", "User wants to see all wallets", []string{"list wallets", "show all wallets", "my wallets"}},
		{"wallet_show", "User wants to see active wallet details", []string{"show wallet", "my wallet", "wallet details"}},
		{"wallet_balance", "User wants to check wallet balance", []string{"balance", "what's my balance", "how much do I have"}},
		{"wallet_assets", "User wants to see wallet assets", []string{"show assets", "my assets", "what tokens do I have"}},
		{"switch_wallet", "User wants to switch to a different wallet", []string{"switch wallet", "use wallet 2", "change wallet"}},
		{"swap_quote", "User wants a swap price estimate", []string{"swap 100 XLM to USDC", "exchange XLM for USDC", "quote"}},
		{"execute_swap", "User wants to execute a pending swap", []string{"execute swap", "confirm swap", "yes"}},
		{"pay_send", "User wants to send a payment", []string{"send 100 USDC to ADDRESS", "pay", "transfer"}},
		{"create_trustline", "User wants to create a trustline for an asset", []string{"trust USDC", "add trustline", "trustline"}},
		{"triangular_scan", "User wants to scan for triangular arbitrage", []string{"scan for arbitrage", "triangular scan", "find opportunities"}},
		{"select_triangular", "User selects a triangular opportunity", []string{"1", "2", "select 1"}},
		{"execute_triangular", "User wants to execute triangular arbitrage", []string{"execute", "go", "run arbitrage"}},
		{"arbitrage_all", "User wants to scan all pairs for arbitrage", []string{"scan all", "arbitrage all", "find all opportunities"}},
		{"show_network", "User wants to see current network", []string{"network", "what network", "current network"}},
		{"set_network", "User wants to change network", []string{"switch to mainnet", "use testnet", "set network"}},
		{"system_status", "User wants system status", []string{"status", "system status"}},
		{"system_health", "User wants health check", []string{"health", "how is the system", "check health"}},
		{"memory_save", "User wants to save something to memory", []string{"remember", "save to memory", "note this"}},
		{"memory_recall", "User wants to recall from memory", []string{"recall", "what did I save", "remember"}},
		{"memory_list", "User wants to list memories", []string{"list memories", "show memories", "what do you remember"}},
		{"memory_delete", "User wants to delete a memory", []string{"forget", "delete memory", "remove"}},
		{"unknown", "None of the above match", []string{}},
	}

	for _, intent := range intents {
		fmt.Fprintf(&b, "- %s: %s\n", intent.name, intent.description)
		if len(intent.examples) > 0 {
			fmt.Fprintf(&b, "  Examples: %s\n", strings.Join(intent.examples, ", "))
		}
	}

	b.WriteString("\nRespond with ONLY a JSON object in this exact format:\n")
	b.WriteString(`{"intent": "INTENT_NAME", "confidence": 0.0-1.0}`)
	b.WriteString("\n\nUser message: \"")
	b.WriteString(message)
	b.WriteString("\"\n")
	b.WriteString("\nIntent JSON: ")

	return b.String()
}

// ParameterExtractionPrompt creates a prompt for extracting parameters
func (p *PromptBuilder) ParameterExtractionPrompt(intent string, message string, state *State) string {
	var b strings.Builder

	b.WriteString("You are a parameter extractor for a cryptocurrency payment CLI called Stellar Go CLI.\n")
	fmt.Fprintf(&b, "The user intent has been classified as: %s\n\n", intent)

	// Define expected parameters for each intent
	params := p.getExpectedParams(intent)
	if len(params) > 0 {
		b.WriteString("Extract the following parameters from the user message:\n")
		for _, param := range params {
			fmt.Fprintf(&b, "- %s (%s): %s\n", param.name, param.paramType, param.description)
		}
	}

	// Add context about current state
	b.WriteString("\nCurrent state information:\n")
	if state.GetNetwork() != "" {
		fmt.Fprintf(&b, "- Current network: %s\n", state.GetNetwork())
	}
	if state.HasPendingSwap() {
		b.WriteString("- There is a pending swap operation\n")
	}

	b.WriteString("\nRespond with ONLY a JSON object containing the extracted parameters.\n")
	b.WriteString("Use null for missing optional parameters. Omit parameters that aren't mentioned.\n")
	b.WriteString("Example: {\"amount\": \"100\", \"asset\": \"XLM\"}\n")
	b.WriteString("\nUser message: \"")
	b.WriteString(message)
	b.WriteString("\"\n")
	b.WriteString("\nParameters JSON: ")

	return b.String()
}

// paramInfo describes an expected parameter
type paramInfo struct {
	name        string
	paramType   string
	description string
	required    bool
}

// getExpectedParams returns the expected parameters for a given intent
func (p *PromptBuilder) getExpectedParams(intent string) []paramInfo {
	switch intent {
	case "swap_quote":
		return []paramInfo{
			{"from", "string", "Source asset code (e.g., XLM, USDC)", true},
			{"to", "string", "Destination asset code (e.g., USDC, EURC)", true},
			{"amount", "string", "Amount to swap as a number", true},
		}
	case "pay_send":
		return []paramInfo{
			{"destination", "string", "Recipient wallet address (starts with G)", true},
			{"amount", "string", "Amount to send as a number", true},
			{"asset", "string", "Asset code to send (e.g., XLM, USDC)", true},
		}
	case "create_trustline":
		return []paramInfo{
			{"code", "string", "Asset code to trust (e.g., USDC, EURC)", true},
			{"issuer", "string", "Asset issuer address (optional, omit if not provided)", false},
		}
	case "set_network":
		return []paramInfo{
			{"network", "string", "Network name (stellar-mainnet or stellar-testnet)", true},
		}
	case "switch_wallet":
		return []paramInfo{
			{"wallet_num", "string", "Wallet number to switch to (1, 2, etc.)", true},
		}
	case "triangular_scan", "arbitrage_all":
		return []paramInfo{
			{"amount", "string", "Amount to scan with (default 10)", false},
		}
	case "select_triangular":
		return []paramInfo{
			{"opportunity_num", "int", "Opportunity number selected (1, 2, etc.)", true},
		}
	case "memory_save":
		return []paramInfo{
			{"key", "string", "Memory key/identifier", true},
			{"value", "string", "Value to remember", true},
		}
	case "memory_recall", "memory_delete":
		return []paramInfo{
			{"key", "string", "Memory key to recall or delete", true},
		}
	default:
		return nil
	}
}

// ResponseGenerationPrompt creates a prompt for generating natural responses
func (p *PromptBuilder) ResponseGenerationPrompt(intent string, params map[string]interface{}, result string) string {
	var b strings.Builder

	b.WriteString("You are a helpful assistant for Stellar Go CLI, a cryptocurrency payment CLI.\n")
	b.WriteString("Generate a friendly, concise response based on the operation result.\n\n")
	fmt.Fprintf(&b, "Intent: %s\n", intent)

	if len(params) > 0 {
		b.WriteString("Parameters:\n")
		for k, v := range params {
			fmt.Fprintf(&b, "- %s: %v\n", k, v)
		}
	}

	fmt.Fprintf(&b, "\nOperation result:\n%s\n\n", result)
	b.WriteString("Provide a helpful, conversational response. Keep it under 3 sentences if possible.\n")
	b.WriteString("Response: ")

	return b.String()
}

// SuggestionPrompt creates a prompt for generating context-aware suggestions
func (p *PromptBuilder) SuggestionPrompt(state *State, lastOperation string) string {
	var b strings.Builder

	b.WriteString("You are a suggestion generator for Stellar Go CLI, a cryptocurrency payment CLI.\n")
	b.WriteString("Based on the user's current state and recent activity, suggest what they might want to do next.\n\n")

	b.WriteString("Current state:\n")
	fmt.Fprintf(&b, "- Network: %s\n", state.GetNetwork())
	fmt.Fprintf(&b, "- Has pending swap: %v\n", state.HasPendingSwap())
	fmt.Fprintf(&b, "- Has pending opportunities: %v\n", state.HasPendingTriangularOpportunities())

	if lastOperation != "" {
		fmt.Fprintf(&b, "- Last operation: %s\n", lastOperation)
	}

	b.WriteString("\nSuggest 3 natural follow-up actions the user might want to take.\n")
	b.WriteString("Respond with a JSON array of suggestions. Each suggestion should have:\n")
	b.WriteString(`- "text": Natural language suggestion (e.g., "Check your wallet balance")`)
	b.WriteString("\n")
	b.WriteString(`- "command": The actual command (e.g., "balance")`)
	b.WriteString("\n\n")
	b.WriteString("Suggestions JSON: ")

	return b.String()
}

// IntentResult represents the LLM's intent classification result
type IntentResult struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
}

// ParameterResult represents extracted parameters
type ParameterResult map[string]interface{}

// Suggestion represents a generated suggestion
type Suggestion struct {
	Text    string `json:"text"`
	Command string `json:"command"`
}

// SuggestionResult represents multiple suggestions
type SuggestionResult struct {
	Suggestions []Suggestion `json:"suggestions"`
}
