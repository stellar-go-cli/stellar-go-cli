package mcp

import (
	"encoding/json"
	"fmt"
)

// createStringSchema creates a JSON Schema for a string parameter
func createStringSchema(description string, required bool) map[string]interface{} {
	schema := map[string]interface{}{
		"type":        "string",
		"description": description,
	}
	if required {
		schema["default"] = nil
	}
	return schema
}

// createNumberSchema creates a JSON Schema for a number parameter
func createNumberSchema(description string, required bool) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
	}
}

// createBooleanSchema creates a JSON Schema for a boolean parameter
func createBooleanSchema(description string, defaultValue bool) map[string]interface{} {
	return map[string]interface{}{
		"type":        "boolean",
		"description": description,
		"default":     defaultValue,
	}
}

// createEnumSchema creates a JSON Schema for an enum parameter
func createEnumSchema(description string, values []string, defaultValue string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
	if defaultValue != "" {
		schema["default"] = defaultValue
	}
	return schema
}

// buildInputSchema creates a complete input schema from parameters
func buildInputSchema(properties map[string]interface{}, required []string) json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	data, _ := json.Marshal(schema)
	return data
}

// ============================================
// Wallet Tools
// ============================================

func (s *Server) registerWalletTools() {
	// wallet_list - List connected wallets
	s.RegisterTool(Tool{
		Name:        "wallet_list",
		Description: "List all connected wallets with their addresses and networks",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleWalletList)

	// wallet_show - Show active wallet details
	s.RegisterTool(Tool{
		Name:        "wallet_show",
		Description: "Show details of the currently active wallet",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleWalletShow)

	// wallet_balance - Get wallet balances
	s.RegisterTool(Tool{
		Name:        "wallet_balance",
		Description: "Get asset balances for a wallet (defaults to active wallet)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address (optional, uses active if omitted)", false),
		}, nil),
	}, s.handleWalletBalance)

	// wallet_assets - List trusted assets
	s.RegisterTool(Tool{
		Name:        "wallet_assets",
		Description: "List all trusted assets (trustlines) for the active wallet",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleWalletAssets)

	// wallet_connect - Connect wallet via passkey, stellar, or external
	s.RegisterTool(Tool{
		Name:        "wallet_connect",
		Description: "Connect wallet via passkey, stellar keypair, or external address",
		InputSchema: buildInputSchema(map[string]interface{}{
			"provider": createEnumSchema("Wallet provider", []string{"wwwallet", "stellar", "external"}, ""),
			"network":  createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet", "evm-sepolia", "evm-mainnet"}, "stellar-testnet"),
			"address":  createStringSchema("External address (for --provider external)", false),
		}, []string{}),
	}, s.handleWalletConnect)

	// wallet_import - Import wallet from secret key or recovery phrase
	s.RegisterTool(Tool{
		Name:        "wallet_import",
		Description: "Import wallet from secret key or recovery phrase",
		InputSchema: buildInputSchema(map[string]interface{}{
			"secret":  createStringSchema("Secret key or recovery phrase (required)", true),
			"name":    createStringSchema("Wallet name (optional)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-testnet"),
		}, []string{"secret"}),
	}, s.handleWalletImport)

	// wallet_fund - Fund wallet from faucet
	s.RegisterTool(Tool{
		Name:        "wallet_fund",
		Description: "Fund wallet from testnet faucet (only works on testnet)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address (optional, uses active if omitted)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-testnet"),
		}, []string{}),
	}, s.handleWalletFund)

	// wallet_switch - Switch active wallet
	s.RegisterTool(Tool{
		Name:        "wallet_switch",
		Description: "Switch to a different wallet as the active wallet",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address to switch to (required)", true),
		}, []string{"address"}),
	}, s.handleWalletSwitch)

	// wallet_rename - Rename a wallet
	s.RegisterTool(Tool{
		Name:        "wallet_rename",
		Description: "Rename an existing wallet",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address to rename (required)", true),
			"name":    createStringSchema("New wallet name (required)", true),
		}, []string{"address", "name"}),
	}, s.handleWalletRename)

	// wallet_remove - Remove a wallet
	s.RegisterTool(Tool{
		Name:        "wallet_remove",
		Description: "Remove a wallet from the registry (does not delete from blockchain)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address to remove (required)", true),
			"confirm": createBooleanSchema("Confirm removal (required for safety)", false),
		}, []string{"address"}),
	}, s.handleWalletRemove)

	// wallet_export - Export wallet private key
	s.RegisterTool(Tool{
		Name:        "wallet_export",
		Description: "Export wallet private key (use with caution)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"address": createStringSchema("Wallet address to export (optional, uses active if omitted)", false),
			"confirm": createBooleanSchema("Confirm export (required for security)", false),
		}, []string{}),
	}, s.handleWalletExport)

	// wallet_passkey - Manage passkey authentication
	s.RegisterTool(Tool{
		Name:        "wallet_passkey",
		Description: "Manage passkey authentication for wallet",
		InputSchema: buildInputSchema(map[string]interface{}{
			"action":  createEnumSchema("Action", []string{"register", "verify", "remove"}, "register"),
			"address": createStringSchema("Wallet address (optional, uses active if omitted)", false),
		}, []string{}),
	}, s.handleWalletPasskey)
}

// ============================================
// Swap Tools
// ============================================

func (s *Server) registerSwapTools() {
	// swap_quote - Get swap quote
	s.RegisterTool(Tool{
		Name:        "swap_quote",
		Description: "Get a swap quote for converting between assets using Stellar path payments",
		InputSchema: buildInputSchema(map[string]interface{}{
			"from":   createStringSchema("Source asset code (e.g., XLM, USDC)", true),
			"to":     createStringSchema("Destination asset code", true),
			"amount": createStringSchema("Amount to swap", true),
			"type":   createEnumSchema("Swap type", []string{"strict-send", "strict-receive"}, "strict-send"),
		}, []string{"from", "to", "amount"}),
	}, s.handleSwapQuote)

	// swap_execute - Execute a swap
	s.RegisterTool(Tool{
		Name:        "swap_execute",
		Description: "Execute an asset swap using Stellar path payments",
		InputSchema: buildInputSchema(map[string]interface{}{
			"from":     createStringSchema("Source asset code", true),
			"to":       createStringSchema("Destination asset code", true),
			"amount":   createStringSchema("Amount to swap", true),
			"type":     createEnumSchema("Swap type", []string{"strict-send", "strict-receive"}, "strict-send"),
			"slippage": createNumberSchema("Maximum slippage percentage", false),
			"execute":  createBooleanSchema("Actually execute the transaction (dry-run if false)", false),
		}, []string{"from", "to", "amount"}),
	}, s.handleSwapExecute)

	// swap_arbitrage_scan - Scan for arbitrage opportunities
	s.RegisterTool(Tool{
		Name:        "swap_arbitrage_scan",
		Description: "Scan for arbitrage opportunities between asset pairs",
		InputSchema: buildInputSchema(map[string]interface{}{
			"min_profit_xlm": createNumberSchema("Minimum profit in XLM to report", false),
			"pairs":          createStringSchema("Comma-separated asset pairs to scan (e.g., XLM/USDC,USDC/EURC)", false),
		}, nil),
	}, s.handleSwapArbitrageScan)

	// swap_assets - List available swap paths
	s.RegisterTool(Tool{
		Name:        "swap_assets",
		Description: "List available assets and their swap pairs",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleSwapAssets)

	// swap_scan - Scan for swap opportunities
	s.RegisterTool(Tool{
		Name:        "swap_scan",
		Description: "Scan for profitable swap opportunities across different paths",
		InputSchema: buildInputSchema(map[string]interface{}{
			"amount":  createStringSchema("Amount to scan with (default: 10)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
			"output":  createEnumSchema("Output format", []string{"pretty", "json"}, "pretty"),
		}, []string{}),
	}, s.handleSwapScan)

	// swap_monitor - Monitor swap opportunities in real-time
	s.RegisterTool(Tool{
		Name:        "swap_monitor",
		Description: "Monitor swap opportunities and alert when profitable conditions appear",
		InputSchema: buildInputSchema(map[string]interface{}{
			"pairs":      createStringSchema("Asset pairs to monitor (comma-separated)", false),
			"min_profit": createNumberSchema("Minimum profit threshold", false),
			"duration":   createNumberSchema("Monitor duration in seconds (default: 60)", false),
		}, []string{}),
	}, s.handleSwapMonitor)

	// swap_arbitrage_all - Comprehensive arbitrage scan
	s.RegisterTool(Tool{
		Name:        "swap_arbitrage_all",
		Description: "Comprehensive arbitrage scan across all asset pairs and paths",
		InputSchema: buildInputSchema(map[string]interface{}{
			"min_profit_xlm": createNumberSchema("Minimum profit in XLM", false),
			"max_depth":      createNumberSchema("Maximum search depth (default: 3)", false),
			"network":        createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
		}, []string{}),
	}, s.handleSwapArbitrageAll)

	// swap_triangular - Triangular arbitrage operations
	s.RegisterTool(Tool{
		Name:        "swap_triangular",
		Description: "Perform triangular arbitrage with 3-leg cycles (XLM→USDC→yXLM→XLM)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"action":  createEnumSchema("Action", []string{"scan", "monitor", "backtest"}, "scan"),
			"amount":  createStringSchema("Starting XLM amount (default: 10)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
		}, []string{}),
	}, s.handleSwapTriangular)
}

// ============================================
// Asset Tools
// ============================================

func (s *Server) registerAssetTools() {
	// asset_list - List all assets
	s.RegisterTool(Tool{
		Name:        "asset_list",
		Description: "List all known assets on the configured network",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleAssetList)

	// asset_trust - Add trustline
	s.RegisterTool(Tool{
		Name:        "asset_trust",
		Description: "Establish a trustline for an asset (required before receiving)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"code":    createStringSchema("Asset code (e.g., USDC)", true),
			"issuer":  createStringSchema("Asset issuer address", true),
			"limit":   createStringSchema("Trust limit (omit for no limit)", false),
			"execute": createBooleanSchema("Actually execute the transaction", false),
		}, []string{"code", "issuer"}),
	}, s.handleAssetTrust)

	// asset_info - Get asset info
	s.RegisterTool(Tool{
		Name:        "asset_info",
		Description: "Get detailed information about a specific asset",
		InputSchema: buildInputSchema(map[string]interface{}{
			"code":   createStringSchema("Asset code", true),
			"issuer": createStringSchema("Asset issuer (optional for native assets)", false),
		}, []string{"code"}),
	}, s.handleAssetInfo)

	// asset_create_ft - Create fungible token
	s.RegisterTool(Tool{
		Name:        "asset_create_ft",
		Description: "Create a fungible token (SAC/SEP-41) on Stellar",
		InputSchema: buildInputSchema(map[string]interface{}{
			"name":        createStringSchema("Token name (required)", true),
			"symbol":      createStringSchema("Token symbol, max 12 chars (required)", true),
			"supply":      createStringSchema("Total supply (default: 1000000)", false),
			"decimals":    createNumberSchema("Decimal places (default: 7)", false),
			"network":     createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-testnet"),
			"with_carbon": createBooleanSchema("Attach StellarCarbon offset credits", false),
			"carbon_amt":  createNumberSchema("Carbon credit amount in tCO2e", false),
		}, []string{"name", "symbol"}),
	}, s.handleAssetCreateFT)

	// asset_create_nfa - Create non-fungible asset
	s.RegisterTool(Tool{
		Name:        "asset_create_nfa",
		Description: "Create a non-fungible asset (NFA) using SEP-41",
		InputSchema: buildInputSchema(map[string]interface{}{
			"name":    createStringSchema("Asset name (required)", true),
			"symbol":  createStringSchema("Asset symbol (required)", true),
			"uri":     createStringSchema("Metadata URI (required)", true),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-testnet"),
		}, []string{"name", "symbol", "uri"}),
	}, s.handleAssetCreateNFA)

	// asset_score - Score asset for risk/quality assessment
	s.RegisterTool(Tool{
		Name:        "asset_score",
		Description: "Get asset quality and risk score analysis",
		InputSchema: buildInputSchema(map[string]interface{}{
			"code":    createStringSchema("Asset code (required)", true),
			"issuer":  createStringSchema("Asset issuer (optional for native assets)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
		}, []string{"code"}),
	}, s.handleAssetScore)

	// asset_carbon - Attach carbon credits to asset
	s.RegisterTool(Tool{
		Name:        "asset_carbon",
		Description: "Attach StellarCarbon offset credits to an asset",
		InputSchema: buildInputSchema(map[string]interface{}{
			"code":    createStringSchema("Asset code (required)", true),
			"amount":  createNumberSchema("Carbon credit amount in tCO2e (required)", true),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
		}, []string{"code", "amount"}),
	}, s.handleAssetCarbon)

	// asset_show - Show detailed asset information
	s.RegisterTool(Tool{
		Name:        "asset_show",
		Description: "Show comprehensive asset details including metadata and performance",
		InputSchema: buildInputSchema(map[string]interface{}{
			"code":    createStringSchema("Asset code (required)", true),
			"issuer":  createStringSchema("Asset issuer (optional for native assets)", false),
			"network": createEnumSchema("Network", []string{"stellar-testnet", "stellar-mainnet"}, "stellar-mainnet"),
		}, []string{"code"}),
	}, s.handleAssetShow)
}

// ============================================
// Payment Tools
// ============================================

func (s *Server) registerPayTools() {
	// pay_send - Send payment
	s.RegisterTool(Tool{
		Name:        "pay_send",
		Description: "Send a payment to another address",
		InputSchema: buildInputSchema(map[string]interface{}{
			"destination": createStringSchema("Destination address", true),
			"asset":       createStringSchema("Asset code to send", true),
			"amount":      createStringSchema("Amount to send", true),
			"memo":        createStringSchema("Optional memo", false),
			"execute":     createBooleanSchema("Actually execute the transaction", false),
		}, []string{"destination", "asset", "amount"}),
	}, s.handlePaySend)

	// pay_request - Generate payment request
	s.RegisterTool(Tool{
		Name:        "pay_request",
		Description: "Generate a payment request (QR code or link)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"asset":  createStringSchema("Asset code to request", true),
			"amount": createStringSchema("Amount to request", true),
			"memo":   createStringSchema("Optional memo/description", false),
		}, []string{"asset", "amount"}),
	}, s.handlePayRequest)

	// pay_history - Get payment history
	s.RegisterTool(Tool{
		Name:        "pay_history",
		Description: "Get recent payment history for the active wallet",
		InputSchema: buildInputSchema(map[string]interface{}{
			"limit":  createNumberSchema("Number of payments to retrieve (default 20)", false),
			"cursor": createStringSchema("Pagination cursor", false),
		}, []string{}),
	}, s.handlePayHistory)

	// pay_quote - Get payment quote
	s.RegisterTool(Tool{
		Name:        "pay_quote",
		Description: "Get payment quote across different rails (direct, x402, tempo, zk)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"to":     createStringSchema("Recipient address (required)", true),
			"amount": createStringSchema("Amount to send (required)", true),
			"asset":  createStringSchema("Asset code (required)", true),
			"rail":   createEnumSchema("Payment rail", []string{"direct", "x402", "tempo", "zk"}, "direct"),
		}, []string{"to", "amount", "asset"}),
	}, s.handlePayQuote)

	// pay_x402 - x402 micropayments
	s.RegisterTool(Tool{
		Name:        "pay_x402",
		Description: "Execute payment via x402 micropayment protocol",
		InputSchema: buildInputSchema(map[string]interface{}{
			"to":      createStringSchema("Recipient address (required)", true),
			"amount":  createStringSchema("Amount to send (required)", true),
			"asset":   createStringSchema("Asset code (required)", true),
			"execute": createBooleanSchema("Execute payment (dry-run if false)", false),
		}, []string{"to", "amount", "asset"}),
	}, s.handlePayX402)

	// pay_zk - Zero-knowledge payments
	s.RegisterTool(Tool{
		Name:        "pay_zk",
		Description: "Execute payment using zero-knowledge proofs",
		InputSchema: buildInputSchema(map[string]interface{}{
			"to":     createStringSchema("Recipient address (required)", true),
			"amount": createStringSchema("Amount to send (required)", true),
			"asset":  createStringSchema("Asset code (required)", true),
			"prove":  createBooleanSchema("Generate ZK proof", true),
		}, []string{"to", "amount", "asset"}),
	}, s.handlePayZK)

	// pay_rails - Payment rail information
	s.RegisterTool(Tool{
		Name:        "pay_rails",
		Description: "Get information about available payment rails and their capabilities",
		InputSchema: buildInputSchema(map[string]interface{}{
			"rail": createEnumSchema("Specific rail to query", []string{"direct", "x402", "tempo", "zk"}, ""),
		}, []string{}),
	}, s.handlePayRails)
}

// ============================================
// System Tools
// ============================================

func (s *Server) registerSystemTools() {
	// system_status - Get system status
	s.RegisterTool(Tool{
		Name:        "system_status",
		Description: "Get MozartPay system status including network connection and version",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleSystemStatus)

	// system_network - Network operations
	s.RegisterTool(Tool{
		Name:        "system_network",
		Description: "Get or set the active network (testnet/mainnet)",
		InputSchema: buildInputSchema(map[string]interface{}{
			"network": createEnumSchema("Network to switch to", []string{"stellar-testnet", "stellar-mainnet"}, ""),
		}, nil),
	}, s.handleSystemNetwork)

	// system_health - Health check
	s.RegisterTool(Tool{
		Name:        "system_health",
		Description: "Check health of external services (Horizon, etc.)",
		InputSchema: buildInputSchema(map[string]interface{}{}, nil),
	}, s.handleSystemHealth)

	// ============================================
	// Memory Tools
	// ============================================

	// memory_save - Save information to memory
	s.RegisterTool(Tool{
		Name:        "memory_save",
		Description: "Save information to the conversation memory for future reference",
		InputSchema: buildInputSchema(map[string]interface{}{
			"key":      createStringSchema("Unique identifier for the memory entry", true),
			"value":    createStringSchema("Data to store (JSON-serializable)", true),
			"category": createStringSchema("Memory category for organization", false),
			"ttl":      createNumberSchema("Time-to-live in seconds (optional)", false),
			"scope":    createEnumSchema("Memory scope", []string{"session", "user", "global"}, "session"),
		}, []string{"key", "value"}),
	}, s.handleMemorySave)

	// memory_get - Retrieve information from memory
	s.RegisterTool(Tool{
		Name:        "memory_get",
		Description: "Retrieve information from the conversation memory",
		InputSchema: buildInputSchema(map[string]interface{}{
			"key":      createStringSchema("Key of the memory entry to retrieve", true),
			"category": createStringSchema("Filter by memory category", false),
		}, []string{"key"}),
	}, s.handleMemoryGet)

	// memory_list - List all memories
	s.RegisterTool(Tool{
		Name:        "memory_list",
		Description: "List all memories or filter by category",
		InputSchema: buildInputSchema(map[string]interface{}{
			"category":        createStringSchema("Filter by specific category", false),
			"scope":           createEnumSchema("Filter by scope", []string{"session", "user", "global"}, ""),
			"include_expired": createBooleanSchema("Include expired memories", false),
		}, []string{}),
	}, s.handleMemoryList)

	// memory_delete - Delete a specific memory
	s.RegisterTool(Tool{
		Name:        "memory_delete",
		Description: "Delete a specific memory entry",
		InputSchema: buildInputSchema(map[string]interface{}{
			"key":     createStringSchema("Key of the memory entry to delete", true),
			"confirm": createBooleanSchema("Confirm deletion", false),
		}, []string{"key"}),
	}, s.handleMemoryDelete)

	// memory_search - Search memories by content
	s.RegisterTool(Tool{
		Name:        "memory_search",
		Description: "Search memories by content or keywords",
		InputSchema: buildInputSchema(map[string]interface{}{
			"query":    createStringSchema("Search query string", true),
			"category": createStringSchema("Limit search to category", false),
			"fuzzy":    createBooleanSchema("Enable fuzzy matching", false),
		}, []string{"query"}),
	}, s.handleMemorySearch)
}

// Helper function to safely extract string argument
func getStringArg(args map[string]interface{}, key string) (string, bool) {
	val, ok := args[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// Helper function to safely extract number argument
func getNumberArg(args map[string]interface{}, key string) (float64, bool) {
	val, ok := args[key]
	if !ok {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// Helper function to safely extract bool argument
func getBoolArg(args map[string]interface{}, key string) bool {
	val, ok := args[key]
	if !ok {
		return false
	}
	b, ok := val.(bool)
	return b && ok
}

// formatError creates a formatted error response
func formatError(err error) map[string]interface{} {
	return map[string]interface{}{
		"error": fmt.Sprintf("%v", err),
	}
}
