// Package chat provides an interactive chat interface for MozartPay CLI
package chat

import (
	"fmt"
	"strings"
	"time"
)

// OperationResult tracks the result of an operation for context awareness
type OperationResult struct {
	Command   string
	Success   bool
	Error     string
	Timestamp time.Time
	Context   map[string]interface{}
}

// SuggestionContext represents the current context for generating suggestions
type SuggestionContext struct {
	LastOperation  *OperationResult
	CommandHistory []string
	WalletBalance  float64
	Network        string
	HasPendingSwap bool
	HasPendingOp   bool
	AssetCount     int
	WalletCount    int
}

// Suggester generates contextual prompt suggestions
type Suggester struct {
	commandHistory []string
	lastOperation  *OperationResult
	userPatterns   map[string]int
}

// NewSuggester creates a new suggestion generator
func NewSuggester() *Suggester {
	return &Suggester{
		commandHistory: make([]string, 0, 10),
		userPatterns:   make(map[string]int),
	}
}

// GenerateSmartSuggestions creates intelligent suggestions based on context
func (s *Suggester) GenerateSmartSuggestions(state *State, walletBalance float64, network string, assetCount, walletCount int) string {
	context := &SuggestionContext{
		LastOperation:  s.lastOperation,
		CommandHistory: s.commandHistory,
		WalletBalance:  walletBalance,
		Network:        network,
		HasPendingSwap: state.HasPendingSwap(),
		HasPendingOp:   state.GetPendingOperation() != nil,
		AssetCount:     assetCount,
		WalletCount:    walletCount,
	}

	suggestions := s.getSmartSuggestions(context)
	tip := s.getContextualTip(context)

	result := fmt.Sprintf("\n💡 **Try:** %s", strings.Join(suggestions, ", "))
	if tip != "" {
		result += fmt.Sprintf("\n💡 **Tip:** %s", tip)
	}

	return result
}

// getSmartSuggestions returns intelligent suggestions based on context
func (s *Suggester) getSmartSuggestions(context *SuggestionContext) []string {
	// Priority 1: Pending operations
	if context.HasPendingSwap {
		return []string{"yes, execute", "no, cancel", "check network"}
	}

	if context.HasPendingOp {
		return []string{"cancel", "help", "status"}
	}

	// Priority 2: Context-aware suggestions based on last operation
	if context.LastOperation != nil {
		return s.getSuggestionsForLastOperation(context)
	}

	// Priority 3: State-based suggestions
	return s.getStateBasedSuggestions(context)
}

// getSuggestionsForLastOperation returns suggestions based on the last operation
func (s *Suggester) getSuggestionsForLastOperation(context *SuggestionContext) []string {
	op := context.LastOperation

	if !op.Success {
		return s.getTroubleshootingSuggestions(context, op)
	}

	// Successful operation suggestions
	switch op.Command {
	case "swap":
		return []string{"send payment", "check balance", "another swap"}
	case "pay_send":
		return []string{"check balance", "send another", "view assets"}
	case "wallet_balance":
		return []string{"swap XLM to USDC", "send payment", "view assets"}
	case "assets":
		return []string{"create trustline", "swap to new asset", "send payment"}
	case "set_network":
		return []string{"check balance", "swap XLM to USDC", "view wallet"}
	default:
		return []string{"check balance", "swap XLM to USDC", "wallet"}
	}
}

// getTroubleshootingSuggestions returns suggestions for failed operations
func (s *Suggester) getTroubleshootingSuggestions(context *SuggestionContext, op *OperationResult) []string {
	switch op.Command {
	case "swap":
		if strings.Contains(op.Error, "insufficient") || context.WalletBalance < 10 {
			return []string{"fund wallet", "try smaller amount", "check other wallets"}
		}
		if strings.Contains(op.Error, "not found") {
			return []string{"check network", "verify wallet", "switch network"}
		}
		return []string{"check network", "verify balance", "try again"}
	case "pay_send":
		if strings.Contains(op.Error, "insufficient") {
			return []string{"check balance", "try smaller amount", "fund wallet"}
		}
		return []string{"verify address", "check network", "try again"}
	default:
		return []string{"check status", "help", "try again"}
	}
}

// getStateBasedSuggestions returns suggestions based on current state
func (s *Suggester) getStateBasedSuggestions(context *SuggestionContext) []string {
	suggestions := []string{}

	// Low balance suggestions
	if context.WalletBalance < 5 {
		suggestions = append(suggestions, "fund wallet")
	} else if context.WalletBalance < 50 {
		suggestions = append(suggestions, "swap XLM to USDC")
	}

	// Asset-based suggestions
	if context.AssetCount == 0 {
		suggestions = append(suggestions, "create trustline")
	} else if context.AssetCount > 2 {
		suggestions = append(suggestions, "triangular arbitrage")
	}

	// Wallet-based suggestions
	if context.WalletCount > 1 {
		suggestions = append(suggestions, "switch wallet")
	}

	// Network-based suggestions
	if context.Network == "stellar-testnet" {
		suggestions = append(suggestions, "use mainnet")
	} else {
		suggestions = append(suggestions, "use testnet")
	}

	// Ensure we have some basic suggestions
	if len(suggestions) == 0 {
		suggestions = []string{"check balance", "swap XLM to USDC", "wallet"}
	}

	// Limit to 3 suggestions
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}

	return suggestions
}

// getContextualTip returns a helpful tip based on context
func (s *Suggester) getContextualTip(context *SuggestionContext) string {
	if context.LastOperation != nil && !context.LastOperation.Success {
		switch context.LastOperation.Command {
		case "swap":
			if strings.Contains(context.LastOperation.Error, "insufficient") {
				return "Swaps require XLM for fees. Try funding your wallet or using a smaller amount."
			}
			return "Swaps may fail due to network issues or insufficient funds. Check your network and balance."
		case "pay_send":
			return "Payments require sufficient funds and valid addresses. Verify the recipient address and your balance."
		}
	}

	if context.WalletBalance < 10 {
		return "Your wallet balance is low. Consider funding it to perform swaps or payments."
	}

	if context.AssetCount == 0 {
		return "You can create trustlines to hold assets like USDC and EURC beyond just XLM."
	}

	return ""
}

// GenerateSuggestions creates contextual suggestions based on chat state (backward compatibility)
func (s *Suggester) GenerateSuggestions(state *State) string {
	// Use basic suggestions for now - this will be enhanced in chat.go integration
	suggestions := []string{"check balance", "swap XLM to USDC", "wallet"}
	return fmt.Sprintf("\n💡 **Try:** %s", strings.Join(suggestions, ", "))
}

// TrackOperation records an operation for context awareness
func (s *Suggester) TrackOperation(command string, success bool, error string, context map[string]interface{}) {
	s.lastOperation = &OperationResult{
		Command:   command,
		Success:   success,
		Error:     error,
		Timestamp: time.Now(),
		Context:   context,
	}

	// Update command history (keep last 10)
	s.commandHistory = append(s.commandHistory, command)
	if len(s.commandHistory) > 10 {
		s.commandHistory = s.commandHistory[1:]
	}

	// Update user patterns
	s.userPatterns[command]++
}

// GenerateParameterSuggestions returns suggestions for parameter collection
func (s *Suggester) GenerateParameterSuggestions(paramName string) string {
	switch paramName {
	case "amount":
		return "\n💡 **Examples:** 100, 50.5, 1000"
	case "from", "to":
		return "\n💡 **Common assets:** XLM, USDC, EURC"
	case "destination":
		return "\n💡 **Format:** G... (56 character Stellar address)"
	default:
		return "\n💡 **Tip:** Type 'cancel' to stop this operation"
	}
}

// GenerateWalletSuggestions returns suggestions after wallet operations
func (s *Suggester) GenerateWalletSuggestions() string {
	return "\n💡 **Try:** balance, swap XLM to USDC, switch to wallet 2"
}

// GenerateSwapSuggestions returns suggestions after swap operations
func (s *Suggester) GenerateSwapSuggestions() string {
	return "\n💡 **Try:** balance, swap XLM to USDC, wallet"
}

// GenerateNetworkSuggestions returns suggestions for network switching
func (s *Suggester) GenerateNetworkSuggestions(currentNetwork string) string {
	if strings.Contains(currentNetwork, "testnet") {
		return "\n💡 **Try:** balance, swap XLM to USDC, use mainnet"
	}
	return "\n💡 **Try:** balance, swap XLM to USDC, use testnet"
}
