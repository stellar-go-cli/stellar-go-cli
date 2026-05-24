// Package chat provides an interactive chat interface for MozartPay CLI
package chat

import (
	"regexp"
	"strconv"
	"strings"
)

// Intent represents a recognized user intention
type Intent struct {
	Name   string                 `json:"intent"`
	Params map[string]interface{} `json:"params"`
}

// IntentRecognizerInterface defines the contract for intent recognition
type IntentRecognizerInterface interface {
	UnderstandIntent(message string, state *State) *Intent
}

// IntentRecognizer handles understanding user commands
type IntentRecognizer struct {
	typoCorrections map[string]string
}

// NewIntentRecognizer creates a new intent recognizer with typo corrections
func NewIntentRecognizer() *IntentRecognizer {
	return &IntentRecognizer{
		typoCorrections: map[string]string{
			"swwap":         "swap",
			"swapp":         "swap",
			"swaap":         "swap",
			"swapo":         "swap",
			"swpa":          "swap",
			"balnce":        "balance",
			"balace":        "balance",
			"baln":          "balance",
			"walet":         "wallet",
			"wallete":       "wallet",
			"walett":        "wallet",
			"assests":       "assets",
			"assest":        "assets",
			"assetes":       "assets",
			"statu":         "status",
			"satus":         "status",
			"helo":          "help",
			"hellp":         "help",
			"hlp":           "help",
			"netwrk":        "network",
			"netwok":        "network",
			"excange":       "exchange",
			"exchnge":       "exchange",
			"trad":          "trade",
			"trde":          "trade",
			"wlt":           "wallet",
			"cance":         "cancel",
			"cancle":        "cancel",
			"cnacel":        "cancel",
			"trust":         "trustline",
			"trst":          "trustline",
			"triangular":    "triangular_scan",
			"arbitrage":     "triangular_scan",
			"arb":           "triangular_scan",
			"arbitrage-all": "arbitrage_all",
		},
	}
}

// UnderstandIntent analyzes a message and returns the recognized intent
func (ir *IntentRecognizer) UnderstandIntent(message string, state *State) *Intent {
	msg := strings.ToLower(strings.TrimSpace(message))

	// Apply typo corrections
	for typo, correction := range ir.typoCorrections {
		msg = strings.ReplaceAll(msg, typo, correction)
	}

	// Check for greeting commands (highest priority for friendly interaction)
	if ir.containsAny(msg, []string{"hello", "hi", "hey", "greetings", "good morning", "good afternoon", "good evening", "yo", "what's up", "sup"}) {
		return &Intent{Name: "greeting", Params: map[string]interface{}{}}
	}

	// Check for cancel commands (highest priority during parameter collection)
	if ir.isCancelCommand(msg) {
		return &Intent{Name: "cancel_operation", Params: map[string]interface{}{}}
	}

	// Check for swap execution when we have a pending swap
	if state.HasPendingSwap() && ir.isConfirmation(msg) {
		return &Intent{Name: "execute_swap", Params: map[string]interface{}{}}
	}

	// Check for triangular opportunity execution when we have pending opportunities
	if state.HasPendingTriangularOpportunities() {
		// Check if user said a number to select an opportunity
		if num := ir.extractNumber(msg); num > 0 {
			return &Intent{
				Name:   "select_triangular",
				Params: map[string]interface{}{"opportunity_num": num},
			}
		}
		// Check if user confirmed execution of selected opportunity
		if ir.isConfirmation(msg) && state.GetSelectedTriangularOpportunity() != nil {
			return &Intent{Name: "execute_triangular", Params: map[string]interface{}{}}
		}
	}

	// Check if we're collecting parameters for a pending operation
	if state.GetPendingOperation() != nil {
		return &Intent{
			Name:   "provide_parameter",
			Params: map[string]interface{}{"value": message},
		}
	}

	// Check for balance inquiries
	if ir.containsAny(msg, []string{"balance", "bal", "bala", "how much"}) {
		return &Intent{Name: "wallet_balance", Params: map[string]interface{}{}}
	}

	// Check for wallet operations
	if ir.containsAny(msg, []string{"wallet", "walet", "wlt"}) {
		return ir.recognizeWalletIntent(msg)
	}

	// Check for asset operations
	if ir.containsAny(msg, []string{"assets", "asset", "assest", "assests", "trustlines"}) {
		return &Intent{Name: "wallet_assets", Params: map[string]interface{}{}}
	}

	// Check for trustline creation
	if ir.containsAny(msg, []string{"trust", "trustline", "add trust"}) {
		params := ir.extractTrustlineParams(message)
		return &Intent{Name: "create_trustline", Params: params}
	}

	// Check for payment operations (priority over swap)
	if ir.containsAny(msg, []string{"send", "pay", "transfer", "payment"}) {
		params := ir.extractPaymentParams(message)
		return &Intent{Name: "pay_send", Params: params}
	}

	// Check for swap operations
	if ir.containsAny(msg, []string{"swap", "exchange", "trade", "convert"}) {
		params := ir.extractSwapParams(message)
		return &Intent{Name: "swap_quote", Params: params}
	}

	// System operations
	if ir.containsAny(msg, []string{"status", "health", "system", "info"}) {
		if strings.Contains(msg, "health") {
			return &Intent{Name: "system_health", Params: map[string]interface{}{}}
		}
		return &Intent{Name: "system_status", Params: map[string]interface{}{}}
	}

	// Network operations
	if ir.containsAny(msg, []string{"network", "current network", "what network"}) {
		return &Intent{Name: "show_network", Params: map[string]interface{}{}}
	}

	// Network switching
	if strings.Contains(msg, "change to") || strings.Contains(msg, "switch to") ||
		strings.Contains(msg, "set") || (strings.Contains(msg, "use") && ir.containsAny(msg, []string{"testnet", "mainnet"})) {
		return ir.recognizeNetworkSwitch(msg)
	}

	// Exit commands
	if ir.containsAny(msg, []string{"quit", "exit", "bye", "goodbye", "stop"}) {
		return &Intent{Name: "quit", Params: map[string]interface{}{}}
	}

	// Single word commands
	if len(strings.Fields(msg)) == 1 {
		return ir.recognizeSingleWordCommand(msg)
	}

	// Check for triangular arbitrage scan
	if ir.containsAny(msg, []string{"triangular", "arbitrage", "triangular scan", "arb scan", "find arbitrage", "arbitrage-all", "arbitrage all"}) {
		params := ir.extractTriangularParams(message)
		// Check if it's arbitrage-all
		if ir.containsAny(msg, []string{"arbitrage-all", "arbitrage all"}) {
			return &Intent{Name: "arbitrage_all", Params: params}
		}
		return &Intent{Name: "triangular_scan", Params: params}
	}

	// Check for memory operations
	if ir.containsAny(msg, []string{"remember", "save", "memorize", "store"}) {
		params := ir.extractMemoryParams(message)
		return &Intent{Name: "memory_save", Params: params}
	}

	if ir.containsAny(msg, []string{"recall", "retrieve", "what is my", "what's my", "show my"}) {
		params := ir.extractMemoryRecallParams(message)
		return &Intent{Name: "memory_recall", Params: params}
	}

	if ir.containsAny(msg, []string{"forget", "delete memory", "remove memory"}) {
		params := ir.extractMemoryRecallParams(message)
		return &Intent{Name: "memory_delete", Params: params}
	}

	if ir.containsAny(msg, []string{"memories", "my memories", "what do you know", "what do you remember"}) {
		return &Intent{Name: "memory_list", Params: map[string]interface{}{}}
	}

	return &Intent{Name: "unknown", Params: map[string]interface{}{}}
}

// isCancelCommand checks if message is a cancel command
func (ir *IntentRecognizer) isCancelCommand(msg string) bool {
	cancelWords := []string{"cancel", "stop", "abort", "no", "exit", "quit"}
	return ir.containsAny(msg, cancelWords)
}

// isConfirmation checks if message confirms an action
func (ir *IntentRecognizer) isConfirmation(msg string) bool {
	confirmWords := []string{"yes", "ok", "execute", "confirm", "proceed", "swap it", "do it", "go ahead", "sure"}
	return ir.containsAny(msg, confirmWords)
}

// extractNumber extracts a number from message (e.g., "1" or "opportunity 2")
func (ir *IntentRecognizer) extractNumber(msg string) int {
	// Look for standalone numbers or "opportunity X" pattern
	re := regexp.MustCompile(`(?:opportunity|opp|#)?\s*(\d+)`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) > 1 {
		num, _ := strconv.Atoi(matches[1])
		return num
	}
	return 0
}

// recognizeWalletIntent determines the specific wallet operation
func (ir *IntentRecognizer) recognizeWalletIntent(msg string) *Intent {
	if strings.Contains(msg, "show") || strings.Contains(msg, "my") || strings.Contains(msg, "details") {
		return &Intent{Name: "wallet_show", Params: map[string]interface{}{}}
	}
	if strings.Contains(msg, "list") {
		return &Intent{Name: "wallet_list", Params: map[string]interface{}{}}
	}
	if strings.Contains(msg, "switch") || strings.Contains(msg, "change") {
		// Extract wallet number
		re := regexp.MustCompile(`\d+`)
		if match := re.FindString(msg); match != "" {
			return &Intent{
				Name:   "switch_wallet",
				Params: map[string]interface{}{"wallet_num": match},
			}
		}
		return &Intent{Name: "switch_wallet", Params: map[string]interface{}{}}
	}
	// Default to wallet list
	return &Intent{Name: "wallet_list", Params: map[string]interface{}{}}
}

// recognizeNetworkSwitch determines which network to switch to
func (ir *IntentRecognizer) recognizeNetworkSwitch(msg string) *Intent {
	if strings.Contains(msg, "testnet") {
		return &Intent{
			Name:   "set_network",
			Params: map[string]interface{}{"network": "stellar-testnet"},
		}
	}
	if strings.Contains(msg, "mainnet") {
		return &Intent{
			Name:   "set_network",
			Params: map[string]interface{}{"network": "stellar-mainnet"},
		}
	}
	return &Intent{Name: "unknown", Params: map[string]interface{}{}}
}

// recognizeSingleWordCommand handles single-word commands
func (ir *IntentRecognizer) recognizeSingleWordCommand(msg string) *Intent {
	switch msg {
	case "hello", "hi", "hey", "yo", "sup":
		return &Intent{Name: "greeting", Params: map[string]interface{}{}}
	case "balance", "bal":
		return &Intent{Name: "wallet_balance", Params: map[string]interface{}{}}
	case "swap", "trade", "exchange":
		return &Intent{Name: "swap_quote", Params: map[string]interface{}{}}
	case "status", "stat":
		return &Intent{Name: "system_status", Params: map[string]interface{}{}}
	case "help", "hlp":
		return &Intent{Name: "help", Params: map[string]interface{}{}}
	case "wallet", "walet", "wlt":
		return &Intent{Name: "wallet_list", Params: map[string]interface{}{}}
	case "assets", "asset":
		return &Intent{Name: "wallet_assets", Params: map[string]interface{}{}}
	case "cancel", "stop":
		return &Intent{Name: "cancel_operation", Params: map[string]interface{}{}}
	case "quit", "exit", "bye":
		return &Intent{Name: "quit", Params: map[string]interface{}{}}
	case "network", "net":
		return &Intent{Name: "show_network", Params: map[string]interface{}{}}
	}
	return &Intent{Name: "unknown", Params: map[string]interface{}{}}
}

// extractSwapParams extracts swap parameters from message
func (ir *IntentRecognizer) extractSwapParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Pattern: "swap 100 XLM to USDC" or "swap XLM to USDC"
	// Try pattern with amount
	re := regexp.MustCompile(`(?:swap|exchange|trade)\s+(\d+(?:\.\d+)?)\s+([a-z]{3,})\s+(?:to|for|into)\s+([a-z]{3,})`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) == 4 {
		params["amount"] = matches[1]
		params["from"] = strings.ToUpper(matches[2])
		params["to"] = strings.ToUpper(matches[3])
		return params
	}

	// Pattern without amount: "swap XLM to USDC"
	re = regexp.MustCompile(`(?:swap|exchange|trade)\s+([a-z]{3,})\s+(?:to|for|into)\s+([a-z]{3,})`)
	matches = re.FindStringSubmatch(msg)
	if len(matches) == 3 {
		params["from"] = strings.ToUpper(matches[1])
		params["to"] = strings.ToUpper(matches[2])
		return params
	}

	return params
}

// extractPaymentParams extracts payment parameters from message
func (ir *IntentRecognizer) extractPaymentParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Extract amount and asset
	re := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(xlm|usdc|eurc|btc)`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) == 3 {
		params["amount"] = matches[1]
		params["asset"] = strings.ToUpper(matches[2])
	}

	// Extract Stellar address (starts with G)
	re = regexp.MustCompile(`[GD][A-Z2-7]{55}`)
	if match := re.FindString(strings.ToUpper(message)); match != "" {
		params["destination"] = match
	}

	return params
}

// extractTrustlineParams extracts trustline parameters from message
func (ir *IntentRecognizer) extractTrustlineParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Extract asset code
	re := regexp.MustCompile(`(?:trust|trustline|add trust)\s+([a-z]{3,})`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) == 2 {
		params["code"] = strings.ToUpper(matches[1])
	}

	// Extract issuer address (starts with G)
	re = regexp.MustCompile(`[GD][A-Z2-7]{55}`)
	if match := re.FindString(strings.ToUpper(message)); match != "" {
		params["issuer"] = match
	}

	return params
}

// extractTriangularParams extracts triangular arbitrage scan parameters
func (ir *IntentRecognizer) extractTriangularParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Extract amount
	re := regexp.MustCompile(`(\d+(?:\.\d+)?)`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) == 2 {
		params["amount"] = matches[1]
	} else {
		params["amount"] = "10" // Default amount
	}

	return params
}

// extractMemoryParams extracts memory save parameters from message
// Supports formats like: "remember my name is Olvis", "save my address as GD5..."
func (ir *IntentRecognizer) extractMemoryParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Try to extract key-value pairs from patterns like:
	// "remember my name is Olvis" -> key: "name", value: "Olvis"
	// "save my address as GD5..." -> key: "address", value: "GD5..."
	// "remember that I prefer XLM" -> key: "preference", value: "XLM"

	// Pattern 1: "my X is Y" or "my X are Y"
	re := regexp.MustCompile(`my\s+(\w+)\s+(?:is|are|as)\s+(.+?)(?:\s*$|\s+(?:and|for|to)\s+)`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) >= 3 {
		params["key"] = matches[1]
		params["value"] = strings.TrimSpace(matches[2])
		return params
	}

	// Pattern 2: "remember that I X Y" or "save that I X Y"
	re = regexp.MustCompile(`(?:remember|save|memorize|store)\s+that\s+i\s+(\w+)\s+(.+)`)
	matches = re.FindStringSubmatch(msg)
	if len(matches) >= 3 {
		params["key"] = matches[1]
		params["value"] = strings.TrimSpace(matches[2])
		return params
	}

	// Pattern 3: Generic extraction - anything after the command word
	re = regexp.MustCompile(`(?:remember|save|memorize|store)\s+(.+)`)
	matches = re.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		// Try to find "X as Y" or "X is Y" in the remaining text
		text := matches[1]
		re2 := regexp.MustCompile(`(.+?)\s+(?:is|as|are)\s+(.+)`)
		subMatches := re2.FindStringSubmatch(text)
		if len(subMatches) >= 3 {
			params["key"] = strings.TrimSpace(subMatches[1])
			params["value"] = strings.TrimSpace(subMatches[2])
		} else {
			// Store the whole thing with a generic key
			params["key"] = "note"
			params["value"] = strings.TrimSpace(text)
		}
	}

	return params
}

// extractMemoryRecallParams extracts memory recall parameters from message
// Supports formats like: "what is my name", "recall my address", "show my preferences"
func (ir *IntentRecognizer) extractMemoryRecallParams(message string) map[string]interface{} {
	params := make(map[string]interface{})
	msg := strings.ToLower(message)

	// Pattern 1: "what is my X" or "what's my X"
	re := regexp.MustCompile(`what(?:\s+is|\'s)\s+my\s+(\w+)`)
	matches := re.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		params["key"] = matches[1]
		return params
	}

	// Pattern 2: "recall my X" or "retrieve my X" or "show my X"
	re = regexp.MustCompile(`(?:recall|retrieve|show)\s+my\s+(\w+)`)
	matches = re.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		params["key"] = matches[1]
		return params
	}

	// Pattern 3: "forget X" or "delete memory X"
	re = regexp.MustCompile(`(?:forget|delete|remove)\s+(?:memory\s+)?(?:my\s+)?(\w+)`)
	matches = re.FindStringSubmatch(msg)
	if len(matches) >= 2 {
		params["key"] = matches[1]
		return params
	}

	return params
}

// containsAny checks if message contains any of the given words
func (ir *IntentRecognizer) containsAny(msg string, words []string) bool {
	for _, word := range words {
		if strings.Contains(msg, word) {
			return true
		}
	}
	return false
}
