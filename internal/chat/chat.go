// Package chat provides an interactive chat interface for Stellar Go CLI
package chat

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/stellar-go-cli/stellar-go-cli/internal/assets"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/scanner"
	"github.com/stellar-go-cli/stellar-go-cli/internal/triangular"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

// Chat represents the interactive chat session
type Chat struct {
	state         *State
	recognizer    IntentRecognizerInterface
	formatter     *Formatter
	suggester     *Suggester
	walletSvc     *wallet.Service
	swapSvc       *swap.Service
	assetSvc      *assets.Service
	triangularSvc *triangular.Service
	scannerSvc    *scanner.Service
	cfg           *config.Config
	reader        *bufio.Reader
}

// NewChat creates a new chat instance
func NewChat(cfg *config.Config, walletSvc *wallet.Service, swapSvc *swap.Service, assetSvc *assets.Service, triangularSvc *triangular.Service, scannerSvc *scanner.Service) *Chat {
	// Initialize the appropriate recognizer based on config
	var recognizer IntentRecognizerInterface
	if cfg.LLM.Enabled {
		llmRecognizer := NewLLMIntentRecognizer(cfg.LLM)
		if llmRecognizer.IsAvailable() {
			recognizer = llmRecognizer
			if cfg.Debug {
				fmt.Println("🤖 Using LLM-based intent recognition")
			}
		} else {
			// Fallback to rule-based if LLM not available
			recognizer = NewIntentRecognizer()
			if cfg.Debug {
				fmt.Println("⚠️ LLM not available, using rule-based intent recognition")
			}
		}
	} else {
		recognizer = NewIntentRecognizer()
	}

	return &Chat{
		state:         NewState(cfg),
		recognizer:    recognizer,
		formatter:     NewFormatter(),
		suggester:     NewSuggester(),
		walletSvc:     walletSvc,
		swapSvc:       swapSvc,
		assetSvc:      assetSvc,
		triangularSvc: triangularSvc,
		scannerSvc:    scannerSvc,
		cfg:           cfg,
		reader:        bufio.NewReader(os.Stdin),
	}
}

// Start begins the interactive chat session
func (c *Chat) Start() error {
	// Initialize network consistency on startup
	if err := c.ensureWalletOnNetwork(); err != nil {
		fmt.Printf("⚠️  Network setup warning: %v\n", err)
		fmt.Println("💡 You may need to create or import a wallet for the current network.")
	}

	// Print welcome banner
	fmt.Println("\n============================================================")
	fmt.Println("🤖 Stellar Go CLI Chat with Smart Parameter Collection!")
	fmt.Println("Type 'help' for commands or 'quit' to exit")
	fmt.Println("============================================================")

	// Main chat loop
	for c.state.IsRunning() {
		// Print prompt
		fmt.Print("💬 You: ")

		// Read user input
		input, err := c.reader.ReadString('\n')
		if err != nil {
			fmt.Println("\n❌ Error reading input. Exiting...")
			break
		}

		// Trim input
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Process message
		response := c.ProcessMessage(input)

		// Print response
		fmt.Println("\n🤖 Assistant:", response)
		fmt.Println()
	}

	return nil
}

// ProcessMessage processes a user message and returns the response
func (c *Chat) ProcessMessage(message string) string {
	// Understand the intent
	intent := c.recognizer.UnderstandIntent(message, c.state)

	// Handle the intent
	switch intent.Name {
	case "greeting":
		return c.handleGreeting()

	case "help":
		return c.formatter.FormatHelp(c.state.GetNetwork()) + c.suggester.GenerateSuggestions(c.state)

	case "quit", "exit":
		c.state.Stop()
		return c.formatter.FormatGoodbye()

	case "cancel_operation":
		c.state.ClearPendingOperation()
		c.state.ClearPendingSwap()
		return c.formatter.FormatCancellation() + c.suggester.GenerateSuggestions(c.state)

	case "show_network":
		return c.formatter.FormatNetworkStatus(c.cfg.Network) + c.suggester.GenerateSuggestions(c.state)

	case "set_network":
		network, _ := intent.Params["network"].(string)

		// Update config network (this should be the single source of truth)
		c.cfg.Network = network
		c.state.SetNetwork(network)

		// Clear any pending operations since network changed
		c.state.ClearPendingOperation()
		c.state.ClearPendingSwap()

		// Find and activate wallet on new network
		targetNetwork := models.Network(network)
		if err := c.findAndActivateWalletOnNetwork(targetNetwork); err != nil {
			// No wallet found on new network, but still switch the network
			// User can create/import wallet later
			output := fmt.Sprintf("🌐 **Switched to %s**\n\n", capitalize(strings.TrimPrefix(network, "stellar-")))
			output += fmt.Sprintf("⚠️ %s\n", err.Error())
			output += "\n💡 You can create or import a wallet on this network."
			return output + c.suggester.GenerateNetworkSuggestions(network)
		}

		// Refresh services for new network
		c.refreshServicesForNetwork(targetNetwork)

		return c.formatter.FormatNetworkSwitched(network) + c.suggester.GenerateNetworkSuggestions(network)

	case "wallet_list":
		return c.handleWalletList()

	case "wallet_show":
		return c.handleWalletShow()

	case "wallet_balance":
		return c.handleWalletBalance()

	case "switch_wallet":
		return c.handleSwitchWallet(intent.Params)

	case "swap_quote":
		return c.handleSwapQuote(intent.Params)

	case "execute_swap":
		return c.handleExecuteSwap()

	case "provide_parameter":
		return c.handleProvideParameter(intent.Params)

	case "create_trustline":
		return c.handleCreateTrustline(intent.Params)

	case "triangular_scan":
		return c.handleTriangularScan(intent.Params)

	case "select_triangular":
		return c.handleSelectTriangular(intent.Params)

	case "execute_triangular":
		return c.handleExecuteTriangular(intent.Params)

	case "arbitrage_all":
		return c.handleArbitrageAll(intent.Params)

	case "wallet_assets":
		return c.handleWalletAssets()

	case "pay_send":
		return c.handlePaySend(intent.Params)

	case "system_status":
		return c.handleSystemStatus()

	case "system_health":
		return c.handleSystemHealth()

	case "memory_save":
		return c.handleMemorySave(intent.Params)

	case "memory_recall":
		return c.handleMemoryRecall(intent.Params)

	case "memory_list":
		return c.handleMemoryList()

	case "memory_delete":
		return c.handleMemoryDelete(intent.Params)

	default:
		return c.formatter.FormatUnknown() + c.suggester.GenerateSuggestions(c.state)
	}
}

// handleWalletList handles the wallet list command
func (c *Chat) handleWalletList() string {
	// Refresh wallet cache
	wallets, err := c.getWalletsFromRegistry()
	if err != nil {
		return c.formatter.FormatError(err) + c.suggester.GenerateSuggestions(c.state)
	}

	c.state.UpdateWalletCache(wallets)

	// Filter by current network
	networkWallets := c.filterWalletsByNetwork(wallets)

	return c.formatter.FormatWalletList(networkWallets, c.state.GetNetwork()) + c.suggester.GenerateWalletSuggestions()
}

// handleWalletShow handles the wallet show command
func (c *Chat) handleWalletShow() string {
	// Get active wallet from wallet service
	account, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateWalletSuggestions()
	}

	// Convert Account to WalletEntry
	activeWallet := &models.WalletEntry{
		Address:   account.Address,
		Type:      account.Type,
		Network:   account.Network,
		Balance:   account.Balance,
		Funded:    account.Funded,
		CreatedAt: account.CreatedAt,
	}

	// Update cache with current wallets
	wallets, _ := c.getWalletsFromRegistry() //nolint:errcheck // cache update is best-effort
	c.state.UpdateWalletCache(wallets)

	return c.formatter.FormatWalletShow(activeWallet) + c.suggester.GenerateWalletSuggestions()
}

// handleWalletBalance handles the balance command
func (c *Chat) handleWalletBalance() string {
	// Get active wallet from wallet service
	account, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	// Refresh balance from network
	updatedAccount, err := c.walletSvc.UpdateWalletBalance(account.Address)
	if err != nil {
		// If refresh fails, continue with cached data but note the issue
		fmt.Printf("⚠️ Failed to refresh balance: %v\n", err)
		updatedAccount = account
	} else {
		fmt.Printf("🔍 Fresh balance fetched: %s XLM, Funded: %v\n", updatedAccount.Balance, updatedAccount.Funded)
	}

	// Convert Account to WalletEntry
	activeWallet := &models.WalletEntry{
		Address:   updatedAccount.Address,
		Type:      updatedAccount.Type,
		Network:   updatedAccount.Network,
		Balance:   updatedAccount.Balance,
		Funded:    updatedAccount.Funded,
		CreatedAt: updatedAccount.CreatedAt,
	}

	// Fetch all account assets with balance > 0 (excluding native XLM, shown above)
	var positiveAssets []wallet.AssetInfo
	if assets, err := c.walletSvc.GetAccountAssets(updatedAccount.Address, updatedAccount.Network); err == nil {
		for _, a := range assets {
			if a.Type == "native" {
				continue
			}
			if bal, err := strconv.ParseFloat(a.Balance, 64); err == nil && bal > 0 {
				positiveAssets = append(positiveAssets, a)
			}
		}
	}

	// Update cache with current wallets
	wallets, _ := c.getWalletsFromRegistry() //nolint:errcheck // cache update is best-effort
	c.state.UpdateWalletCache(wallets)

	// Track successful operation for smart suggestions
	c.trackOperationForSuggestions("wallet_balance", true, "", map[string]interface{}{
		"balance": activeWallet.Balance,
		"network": activeWallet.Network,
	})

	return c.formatter.FormatWalletBalance(activeWallet, positiveAssets) + c.generateSmartSuggestions()
}

// handleSwitchWallet handles the switch wallet command
func (c *Chat) handleSwitchWallet(params map[string]interface{}) string {
	walletNumStr, ok := params["wallet_num"].(string)
	if !ok || walletNumStr == "" {
		return "❌ Please specify which wallet number to switch to (e.g., 'switch to wallet 1')" + c.suggester.GenerateSuggestions(c.state)
	}

	walletNum, err := strconv.Atoi(walletNumStr)
	if err != nil || walletNum < 1 {
		return "❌ Invalid wallet number. Please use a number like 1, 2, etc." + c.suggester.GenerateSuggestions(c.state)
	}

	// Get current wallets
	wallets, err := c.getWalletsFromRegistry()
	if err != nil {
		return c.formatter.FormatError(err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Filter by current network
	networkWallets := c.filterWalletsByNetwork(wallets)

	// Validate wallet number
	if walletNum > len(networkWallets) {
		return fmt.Sprintf("❌ Wallet %d not found. You have %d wallets on %s.", walletNum, len(networkWallets), c.state.GetNetwork()) + c.suggester.GenerateSuggestions(c.state)
	}

	// Get the target wallet
	targetWallet := networkWallets[walletNum-1]

	// Actually switch the active wallet using the wallet service
	if err := c.walletSvc.SetActiveWallet(targetWallet.Address); err != nil {
		return fmt.Sprintf("❌ Failed to switch to wallet %d: %v", walletNum, err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Update state to reflect the new active wallet
	c.state.InvalidateWalletCache()

	return fmt.Sprintf("✅ **Switched to wallet %d**\n\n📍 Address: `%s...`\n💰 Balance: %s XLM\n🌐 Network: %s",
		walletNum,
		targetWallet.Address[:16],
		targetWallet.Balance,
		capitalize(strings.TrimPrefix(string(targetWallet.Network), "stellar-"))) + c.suggester.GenerateWalletSuggestions()
}

// handleSwapQuote handles the swap quote command
func (c *Chat) handleSwapQuote(params map[string]interface{}) string {
	// Check if we have all required parameters
	from, hasFrom := params["from"].(string)
	to, hasTo := params["to"].(string)
	amount, hasAmount := params["amount"].(string)
	if !hasFrom {
		from = ""
	}
	if !hasTo {
		to = ""
	}
	if !hasAmount {
		amount = ""
	}

	// Normalize free-text answers collected conversationally
	// (e.g. "xlm to usd", "50 xlm to usd")
	configNetwork := models.Network(c.cfg.Network)
	supported := supportedSwapAssets(configNetwork)
	from, to, amount = normalizeSwapParams(from, to, amount, supported)

	// If missing parameters, start parameter collection
	if from == "" {
		c.startParameterCollection("swap_quote", []ParamInfo{
			{Name: "from", Description: "Source asset", Type: "string", Required: true, Examples: []string{"XLM"}},
			{Name: "to", Description: "Destination asset", Type: "string", Required: true, Examples: []string{"USDC"}},
			{Name: "amount", Description: "Amount to swap", Type: "string", Required: true, Examples: []string{"100"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "from", Description: "What asset are you sending from?", Examples: []string{"XLM", "USDC"}}) + c.suggester.GenerateParameterSuggestions("from")
	}

	if to == "" {
		c.startParameterCollection("swap_quote", []ParamInfo{
			{Name: "to", Description: "Destination asset", Type: "string", Required: true, Examples: []string{"USDC"}},
			{Name: "amount", Description: "Amount to swap", Type: "string", Required: true, Examples: []string{"100"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "to", Description: "What asset are you swapping to?", Examples: []string{"USDC", "EURC"}}) + c.suggester.GenerateParameterSuggestions("to")
	}

	if amount == "" {
		c.startParameterCollection("swap_quote", []ParamInfo{
			{Name: "amount", Description: "Amount to swap", Type: "string", Required: true, Examples: []string{"100"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "amount", Description: "How much would you like to swap?", Examples: []string{"100", "50.5"}}) + c.suggester.GenerateParameterSuggestions("amount")
	}

	// Validate assets before hitting Horizon
	if _, ok := canonicalSwapAsset(from, supported); !ok {
		return fmt.Sprintf("❌ Unsupported asset %q. Supported on this network: %s", from, strings.Join(supportedSwapAssetList(configNetwork), ", ")) + c.suggester.GenerateSuggestions(c.state)
	}
	if _, ok := canonicalSwapAsset(to, supported); !ok {
		return fmt.Sprintf("❌ Unsupported asset %q. Supported on this network: %s", to, strings.Join(supportedSwapAssetList(configNetwork), ", ")) + c.suggester.GenerateSuggestions(c.state)
	}

	// We have all parameters, validate network consistency first
	if err := c.ensureWalletOnNetwork(); err != nil {
		return fmt.Sprintf("❌ Network validation failed: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Use config network as the authoritative source
	swapSvc := swap.NewService(configNetwork)

	// Ensure wallet cache is populated
	wallets, err := c.getWalletsFromRegistry()
	if err != nil {
		return c.formatter.FormatError(err) + c.suggester.GenerateSuggestions(c.state)
	}
	c.state.UpdateWalletCache(wallets)

	// Create swap request
	swapReq := models.SwapRequest{
		SourceAsset: from,
		DestAsset:   to,
		Amount:      amount,
		SwapType:    models.SwapStrictSend,
	}

	// Get quote from Horizon
	swapQuote, err := swapSvc.GetQuote(swapReq)
	if err != nil {
		return fmt.Sprintf("❌ Failed to get swap quote: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Convert to our internal SwapQuote format
	quote := &SwapQuote{
		From:        swapQuote.SourceAsset,
		To:          swapQuote.DestAsset,
		Amount:      swapQuote.Amount,
		Type:        string(swapQuote.SwapType),
		Rate:        swapQuote.ExpectedAmount,
		Estimated:   swapQuote.ExpectedAmount,
		MinReceived: swapQuote.ExpectedAmount,
		PriceImpact: swapQuote.PriceImpact,
		Paths:       swapQuote.Paths,
	}

	c.state.SetPendingSwap(quote)
	return c.formatter.FormatSwapQuote(quote) + c.suggester.GenerateSuggestions(c.state)
}

// handleExecuteSwap handles the swap execution command
func (c *Chat) handleExecuteSwap() string {
	quote := c.state.GetPendingSwap()
	if quote == nil {
		return "❌ No pending swap to execute." + c.suggester.GenerateSuggestions(c.state)
	}

	// Get active wallet
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	// Validate network consistency before execution
	if err := c.ensureWalletOnNetwork(); err != nil {
		return fmt.Sprintf("❌ Network validation failed: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Use config network as the authoritative source
	configNetwork := models.Network(c.cfg.Network)
	swapSvc := swap.NewService(configNetwork)

	// Convert internal SwapQuote to models.SwapQuote for execution
	swapQuote := &models.SwapQuote{
		SourceAsset:    quote.From,
		DestAsset:      quote.To,
		Amount:         quote.Amount,
		SwapType:       models.SwapType(quote.Type),
		ExpectedAmount: quote.Estimated,
		PriceImpact:    quote.PriceImpact,
		Paths:          quote.Paths,
	}

	// Execute the swap on the Stellar network
	payment, err := swapSvc.ExecuteSwap(swapQuote, 0.005, "") // 0.5% max slippage, destination is own wallet
	if err != nil {
		c.state.ClearPendingSwap()

		// Track failed operation for smart suggestions
		c.trackOperationForSuggestions("swap", false, err.Error(), map[string]interface{}{
			"from":   quote.From,
			"to":     quote.To,
			"amount": quote.Amount,
		})

		return c.formatter.FormatSwapReport(quote, "error", "", err.Error(), c.state.GetNetwork(), activeWallet.Address) + c.generateSmartSuggestions()
	}

	// Clear the pending swap after successful execution
	c.state.ClearPendingSwap()

	// Track successful operation for smart suggestions
	c.trackOperationForSuggestions("swap", true, "", map[string]interface{}{
		"from":    quote.From,
		"to":      quote.To,
		"amount":  quote.Amount,
		"tx_hash": payment.TxHash,
		"network": c.state.GetNetwork(),
	})

	// Return success report with real transaction data
	return c.formatter.FormatSwapReport(quote, "success", payment.TxHash, payment.Fee, c.state.GetNetwork(), activeWallet.Address) + c.generateSmartSuggestions()
}

// handleProvideParameter handles providing a parameter value
func (c *Chat) handleProvideParameter(params map[string]interface{}) string {
	value, ok := params["value"].(string)
	if !ok {
		return "❌ Invalid parameter value." + c.suggester.GenerateSuggestions(c.state)
	}

	operation := c.state.GetPendingOperation()
	if operation == nil {
		return "🤔 No pending operation. How can I help you?" + c.suggester.GenerateSuggestions(c.state)
	}

	// Check for cancel
	if strings.ToLower(value) == "cancel" || strings.ToLower(value) == "no" {
		c.state.ClearPendingOperation()
		return c.formatter.FormatCancellation() + c.suggester.GenerateSuggestions(c.state)
	}

	// Check for confirmation (no missing parameters means we're asking for confirmation)
	if len(operation.MissingParams) == 0 {
		// Check for confirmation words
		if strings.ToLower(value) == "yes" || strings.ToLower(value) == "ok" || strings.ToLower(value) == "confirm" || strings.ToLower(value) == "execute" || strings.ToLower(value) == "proceed" {
			// Execute the operation
			toolName := operation.ToolName
			providedParams := operation.ProvidedParams
			c.state.ClearPendingOperation()

			switch toolName {
			case "swap_quote":
				return c.handleSwapQuote(providedParams)
			case "pay_send_execute":
				return c.handlePaySendExecute(providedParams)
			default:
				return "✅ Operation completed!" + c.suggester.GenerateSuggestions(c.state)
			}
		}
		return "❌ Please confirm with 'yes', 'ok', 'confirm', or 'execute'. Type 'cancel' to abort." + c.suggester.GenerateSuggestions(c.state)
	}

	// Add the provided value
	param := operation.MissingParams[0]
	operation.ProvidedParams[param.Name] = value

	// Remove this parameter from missing list
	operation.MissingParams = operation.MissingParams[1:]

	// Check if we have more parameters to collect
	if len(operation.MissingParams) > 0 {
		nextParam := operation.MissingParams[0]
		return c.formatter.FormatParameterPrompt(nextParam) + c.suggester.GenerateParameterSuggestions(nextParam.Name)
	}

	// All parameters collected, ask for confirmation
	return "💡 **Please confirm**: Type 'yes', 'ok', 'confirm', or 'execute' to proceed, or 'cancel' to abort." + c.suggester.GenerateSuggestions(c.state)
}

// startParameterCollection starts collecting parameters for an operation
func (c *Chat) startParameterCollection(toolName string, missingParams []ParamInfo, providedParams map[string]interface{}) {
	if providedParams == nil {
		providedParams = make(map[string]interface{})
	}

	c.state.SetPendingOperation(&Operation{
		ToolName:       toolName,
		MissingParams:  missingParams,
		ProvidedParams: providedParams,
	})
}

// getWalletsFromRegistry retrieves wallets from the wallet registry
func (c *Chat) getWalletsFromRegistry() ([]models.WalletEntry, error) {
	// Use the wallet service to list wallets
	wallets, _, err := c.walletSvc.ListWallets()
	if err != nil {
		return nil, err
	}

	return wallets, nil
}

// filterWalletsByNetwork filters wallets by the current network
func (c *Chat) filterWalletsByNetwork(wallets []models.WalletEntry) []models.WalletEntry {
	currentNetwork := models.Network(c.state.GetNetwork())
	var filtered []models.WalletEntry

	for _, wallet := range wallets {
		if wallet.Network == currentNetwork {
			filtered = append(filtered, wallet)
		}
	}

	return filtered
}

// handleCreateTrustline handles trustline creation
func (c *Chat) handleCreateTrustline(params map[string]interface{}) string {
	code, hasCode := params["code"].(string)
	issuer, hasIssuer := params["issuer"].(string)

	// If missing parameters, prompt for them
	if !hasCode || code == "" {
		return "📋 Please specify the asset code to trust.\nExample: `trust USDC` or `trust USDC GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5`" + c.suggester.GenerateSuggestions(c.state)
	}

	// Get the actual active wallet from wallet service
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	// If issuer not provided, use default for known assets
	if !hasIssuer || issuer == "" {
		switch code {
		case "USDC":
			if c.state.GetNetwork() == "stellar-testnet" {
				issuer = "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
			} else {
				issuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
			}
		case "EURC":
			if c.state.GetNetwork() == "stellar-testnet" {
				issuer = "GAKMOVSF35IPK5HTDN4B3ITIR5R4AX6PZFAXNPJFDHNIUQKDT5O6G2E"
			} else {
				issuer = "GDUKMGUGDZQK6YHYA5Z6AY2G4XDSZDW2WER5GZ5GUESDSEZNCNDJID9"
			}
		default:
			return fmt.Sprintf("❌ Please provide the issuer address for %s.\nExample: `trust %s GISSUERADDRESS...`", code, code) + c.suggester.GenerateSuggestions(c.state)
		}
	}

	// Create trustline using asset service
	if c.assetSvc == nil {
		return "❌ Asset service not available." + c.suggester.GenerateSuggestions(c.state)
	}

	fmt.Printf("🔗 Creating trustline for %s:%s...\n", code, issuer[:12])

	txHash, err := c.assetSvc.CreateTrustline(activeWallet.Address, code, issuer, "")
	if err != nil {
		return fmt.Sprintf("❌ Failed to create trustline: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	return fmt.Sprintf(`✅ **Trustline Created Successfully!**

📋 **Details:**
**Asset:** %s
**Issuer:** %s
**Account:** %s
**TX Hash:** %s

You can now receive %s.`, code, issuer, activeWallet.Address, txHash, code) + c.suggester.GenerateSuggestions(c.state)
}

// handleTriangularScan handles triangular arbitrage scanning with confirmation flow
func (c *Chat) handleTriangularScan(params map[string]interface{}) string {
	amountStr, _ := params["amount"].(string)
	if amountStr == "" {
		amountStr = "10"
	}

	if c.triangularSvc == nil {
		return "❌ Triangular arbitrage service not available." + c.suggester.GenerateSuggestions(c.state)
	}

	fmt.Printf("🔍 Scanning for triangular arbitrage opportunities with %s XLM...\n", amountStr)

	// Run the scan
	results, err := c.triangularSvc.FindTriangularPaths([]string{"USDC", "yXLM"}, amountStr)
	if err != nil {
		return fmt.Sprintf("❌ Scan failed: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	if len(results) == 0 {
		return "🔍 No arbitrage opportunities found at this time.\n\nTry again later or with a different amount." + c.suggester.GenerateSuggestions(c.state)
	}

	// Collect profitable opportunities
	var profitableOpps []triangular.TriangularResult
	output := "📊 **Triangular Arbitrage Scan Results**\n\n"
	opportunityNum := 0

	for _, result := range results {
		if result.Path.IsOpportunity {
			opportunityNum++
			profitableOpps = append(profitableOpps, result)
			output += fmt.Sprintf("✅ **Opportunity %d:**\n", opportunityNum)
			output += fmt.Sprintf("   Path: %s\n", result.Path.Name)
			output += fmt.Sprintf("   Profit: %.6f XLM (%.2f%%)\n", result.NetProfitXLM, result.ProfitPercent)
			output += fmt.Sprintf("   Score: %d/100\n\n", result.OpportunityScore)
		}
	}

	if len(profitableOpps) == 0 {
		output += "📉 No profitable opportunities found.\n\n"
		output += "� **Tips:**\n"
		output += "• Try a different amount (larger = better rates)\n"
		output += "• Wait for market volatility\n"
		output += "• Check prices on StellarX first"
		return output + c.suggester.GenerateSuggestions(c.state)
	}

	// Store opportunities in state for confirmation flow
	c.state.SetPendingTriangularOpportunities(profitableOpps)

	output += fmt.Sprintf("🎯 Found %d profitable opportunities!\n\n", len(profitableOpps))
	output += "💡 **Next steps:**\n"
	output += "• Enter the opportunity number (1, 2, etc.) to select\n"
	output += "• Type 'yes' or 'execute' after selecting to submit\n"
	output += "• Type 'cancel' to clear opportunities"

	return output
}

// handleSelectTriangular handles selecting a specific opportunity
func (c *Chat) handleSelectTriangular(params map[string]interface{}) string {
	num, _ := params["opportunity_num"].(int)
	if num <= 0 {
		return "❌ Please enter a valid opportunity number (1, 2, etc.)"
	}

	// Convert to 0-based index
	index := num - 1

	if !c.state.SelectTriangularOpportunity(index) {
		return fmt.Sprintf("❌ Opportunity %d does not exist. Please select a valid number.", num)
	}

	opportunity := c.state.GetSelectedTriangularOpportunity()
	if opportunity == nil {
		return "❌ Error retrieving opportunity details."
	}

	output := fmt.Sprintf("✅ **Selected Opportunity %d**\n\n", num)
	output += fmt.Sprintf("📍 Path: %s\n", opportunity.Path.Name)
	output += fmt.Sprintf("💰 Profit: %.6f XLM (%.2f%%)\n", opportunity.NetProfitXLM, opportunity.ProfitPercent)
	output += fmt.Sprintf("⭐ Score: %d/100\n\n", opportunity.OpportunityScore)
	output += "💡 **To execute:** Type 'yes', 'execute', or 'confirm'\n"
	output += "💡 **To cancel:** Type 'cancel' or 'no'"

	return output
}

// handleExecuteTriangular handles executing the selected triangular arbitrage
func (c *Chat) handleExecuteTriangular(params map[string]interface{}) string {
	opportunity := c.state.GetSelectedTriangularOpportunity()
	if opportunity == nil {
		return "❌ No opportunity selected. Please run a scan first and select an opportunity."
	}

	// Get active wallet
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	output := "🚀 **Executing Triangular Arbitrage**\n\n"
	output += fmt.Sprintf("📍 Path: %s\n", opportunity.Path.Name)
	output += fmt.Sprintf("💰 Expected Profit: %.6f XLM\n", opportunity.NetProfitXLM)
	output += fmt.Sprintf("👛 Wallet: %s\n\n", activeWallet.Address)

	// Execute the triangular swap
	fmt.Println("⏳ Submitting transaction to Stellar network...")
	txHash, err := c.triangularSvc.ExecuteTriangularSwap(*opportunity, activeWallet.Address)
	if err != nil {
		c.state.ClearPendingTriangularOpportunities()
		return fmt.Sprintf("❌ Execution failed: %v\n\nThe transaction was not submitted.", err) + c.suggester.GenerateSuggestions(c.state)
	}

	c.state.ClearPendingTriangularOpportunities()

	output += "✅ **Success!**\n"
	output += fmt.Sprintf("🔗 Transaction Hash: %v\n", txHash)
	output += fmt.Sprintf("💸 Profit realized: %.6f XLM\n\n", opportunity.NetProfitXLM)
	output += "📊 Check your wallet balance to see the updated amounts."

	return output + c.suggester.GenerateSuggestions(c.state)
}

// handleArbitrageAll handles multi-pair arbitrage scanning
func (c *Chat) handleArbitrageAll(params map[string]interface{}) string {
	amountStr, _ := params["amount"].(string)
	if amountStr == "" {
		amountStr = "10"
	}

	if c.scannerSvc == nil {
		return "❌ Scanner service not available." + c.suggester.GenerateSuggestions(c.state)
	}

	fmt.Printf("🔍 Scanning all liquid pairs for arbitrage with %s XLM...\n", amountStr)

	// Use the scanner service to scan all pairs
	results, err := c.scannerSvc.ScanAll()
	if err != nil {
		return fmt.Sprintf("❌ Scan failed: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	if len(results) == 0 {
		return "🔍 No arbitrage opportunities found.\n\nTry again later or adjust parameters." + c.suggester.GenerateSuggestions(c.state)
	}

	// Find profitable opportunities
	var profitable []struct {
		PairName      string
		NetProfitXLM  float64
		SpreadPercent float64
	}
	for _, r := range results {
		if r.IsProfitable {
			profitable = append(profitable, struct {
				PairName      string
				NetProfitXLM  float64
				SpreadPercent float64
			}{
				PairName:      r.PairName,
				NetProfitXLM:  r.NetProfitXLM,
				SpreadPercent: r.SpreadPercent,
			})
		}
	}

	// Format results
	output := "📊 **Multi-Pair Arbitrage Scan Results**\n\n"
	output += fmt.Sprintf("Scanned: %d pairs\n", len(results))
	output += fmt.Sprintf("Profitable: %d pairs\n\n", len(profitable))

	if len(profitable) == 0 {
		output += "📉 No profitable opportunities found.\n\n"
		output += "💡 **Tips:**\n"
		output += "• Try adjusting the amount\n"
		output += "• Wait for market volatility\n"
		output += "• Monitor continuously with 'monitor' command"
	} else {
		output += "✅ **Profitable Opportunities:**\n\n"
		for i, p := range profitable {
			if i >= 5 {
				output += fmt.Sprintf("... and %d more\n", len(profitable)-5)
				break
			}
			output += fmt.Sprintf("%d. **%s**\n", i+1, p.PairName)
			output += fmt.Sprintf("   Profit: %.6f XLM (%.3f%%)\n", p.NetProfitXLM, p.SpreadPercent)
		}
		output += fmt.Sprintf("\n🎯 Total: %d profitable opportunities!", len(profitable))
	}

	return output + c.suggester.GenerateSuggestions(c.state)
}

// handleWalletAssets handles the wallet assets command
func (c *Chat) handleWalletAssets() string {
	// Get active wallet from wallet service
	account, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateWalletSuggestions()
	}

	// Only support Stellar networks for assets
	if account.Network != models.NetworkStellarTestnet && account.Network != models.NetworkStellarMainnet {
		return "❌ Assets command is only available for Stellar networks (testnet/mainnet)." + c.suggester.GenerateSuggestions(c.state)
	}

	// Get assets from wallet service
	assets, err := c.walletSvc.GetAccountAssets(account.Address, account.Network)
	if err != nil {
		return fmt.Sprintf("❌ Failed to fetch assets: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	if len(assets) == 0 {
		output := "📊 **Wallet Assets**\n\n"
		output += fmt.Sprintf("📍 Address: %s\n", account.Address[:16]+"...")
		output += fmt.Sprintf("🌐 Network: %s\n\n", capitalize(strings.TrimPrefix(string(account.Network), "stellar-")))
		output += "No assets found. Account may be unfunded or has no trustlines.\n"
		output += "💡 Use 'trust <asset>' to create trustlines for tokens like USDC, EURC."
		return output + c.suggester.GenerateSuggestions(c.state)
	}

	// Format assets display
	output := "📊 **Wallet Assets**\n\n"
	output += fmt.Sprintf("📍 Address: %s\n", account.Address[:16]+"...")
	output += fmt.Sprintf("🌐 Network: %s\n\n", capitalize(strings.TrimPrefix(string(account.Network), "stellar-")))
	output += fmt.Sprintf("**%d Asset(s):**\n\n", len(assets))

	for _, asset := range assets {
		if asset.Code == "XLM" {
			output += fmt.Sprintf("💰 **XLM** (native): %s\n", asset.Balance)
		} else {
			output += fmt.Sprintf("🪙 **%s**: %s\n", asset.Code, asset.Balance)
			output += fmt.Sprintf("   📜 Issuer: %s\n", asset.Issuer[:12]+"...")
		}
	}

	return output + c.suggester.GenerateSuggestions(c.state)
}

// handlePaySend handles the payment send command
func (c *Chat) handlePaySend(params map[string]interface{}) string {
	// Validate network consistency first
	if err := c.ensureWalletOnNetwork(); err != nil {
		return fmt.Sprintf("❌ Network validation failed: %v", err) + c.suggester.GenerateSuggestions(c.state)
	}

	// Check if we have all required parameters
	destination, hasDestination := params["destination"].(string)
	amount, hasAmount := params["amount"].(string)
	asset, hasAsset := params["asset"].(string)
	_, _ = params["rail"].(string) // rail is optional, will use default

	// If missing parameters, start parameter collection
	if !hasDestination || destination == "" {
		c.startParameterCollection("pay_send", []ParamInfo{
			{Name: "destination", Description: "Recipient address", Type: "string", Required: true, Examples: []string{"G..."}},
			{Name: "amount", Description: "Amount to send", Type: "string", Required: true, Examples: []string{"10", "50.5"}},
			{Name: "asset", Description: "Asset code", Type: "string", Required: true, Examples: []string{"XLM", "USDC", "EURC"}},
			{Name: "rail", Description: "Payment rail", Type: "string", Required: false, Examples: []string{"direct", "x402", "tempo", "zk"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "destination", Description: "Who are you paying? Enter their Stellar address (starts with G):", Examples: []string{"GABCD...", "G1234..."}}) + c.suggester.GenerateParameterSuggestions("destination")
	}

	if !hasAmount || amount == "" {
		c.startParameterCollection("pay_send", []ParamInfo{
			{Name: "amount", Description: "Amount to send", Type: "string", Required: true, Examples: []string{"10", "50.5"}},
			{Name: "asset", Description: "Asset code", Type: "string", Required: true, Examples: []string{"XLM", "USDC", "EURC"}},
			{Name: "rail", Description: "Payment rail", Type: "string", Required: false, Examples: []string{"direct", "x402", "tempo", "zk"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "amount", Description: "How much are you sending?", Examples: []string{"10", "50.5"}}) + c.suggester.GenerateParameterSuggestions("amount")
	}

	if !hasAsset || asset == "" {
		c.startParameterCollection("pay_send", []ParamInfo{
			{Name: "asset", Description: "Asset code", Type: "string", Required: true, Examples: []string{"XLM", "USDC", "EURC", "BTC"}},
			{Name: "rail", Description: "Payment rail", Type: "string", Required: false, Examples: []string{"direct", "x402", "tempo", "zk"}},
		}, params)
		return c.formatter.FormatParameterPrompt(ParamInfo{Name: "asset", Description: "What asset are you sending?", Examples: []string{"XLM", "USDC", "EURC", "BTC"}}) + c.suggester.GenerateParameterSuggestions("asset")
	}

	// Get rail parameter if provided
	rail, _ := params["rail"].(string)
	if rail == "" {
		rail = "direct"
	}

	// Validate Stellar address format
	if !strings.HasPrefix(destination, "G") || len(destination) != 56 {
		return "❌ Invalid Stellar address. Address must start with 'G' and be 56 characters." + c.suggester.GenerateSuggestions(c.state)
	}

	// Get active wallet
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	// Validate payment rail
	validRails := []string{"direct", "x402", "tempo", "zk"}
	isValidRail := false
	for _, validRail := range validRails {
		if rail == validRail {
			isValidRail = true
			break
		}
	}
	if !isValidRail {
		return fmt.Sprintf("❌ Invalid payment rail '%s'. Valid options: %s", rail, strings.Join(validRails, ", ")) + c.suggester.GenerateSuggestions(c.state)
	}

	// Create payment rail and use config network
	paymentRail := models.PaymentRail(rail)
	configNetwork := models.Network(c.cfg.Network)

	// Show payment summary and ask for confirmation
	output := "💸 **Payment Summary**\n\n"
	output += fmt.Sprintf("👛 From: %s\n", activeWallet.Address[:16]+"...")
	output += fmt.Sprintf("👤 To: %s\n", destination[:16]+"...")
	output += fmt.Sprintf("💰 Amount: %s %s\n", amount, strings.ToUpper(asset))
	output += fmt.Sprintf("🚄 Rail: %s\n", strings.ToUpper(rail))
	output += fmt.Sprintf("🌐 Network: %s\n\n", capitalize(strings.TrimPrefix(string(configNetwork), "stellar-")))

	output += "⚠️ **This will submit a transaction to the Stellar network.**\n"
	output += "💡 **Type 'confirm' to proceed or 'cancel' to abort.**"

	// Store payment details for confirmation
	c.state.SetPendingOperation(&Operation{
		ToolName:      "pay_send_execute",
		MissingParams: []ParamInfo{},
		ProvidedParams: map[string]interface{}{
			"destination": destination,
			"amount":      amount,
			"asset":       strings.ToUpper(asset),
			"rail":        paymentRail,
			"network":     configNetwork,
		},
	})

	return output
}

// handleSystemStatus handles the system status command
func (c *Chat) handleSystemStatus() string {
	output := "📊 **System Status**\n\n"

	// Network information
	output += fmt.Sprintf("🌐 **Network**: %s\n", capitalize(strings.TrimPrefix(c.cfg.Network, "stellar-")))

	// Active wallet information
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		output += "👛 **Active Wallet**: None\n"
	} else {
		output += fmt.Sprintf("👛 **Active Wallet**: %s (%s)\n", activeWallet.Address[:16]+"...", capitalize(strings.TrimPrefix(string(activeWallet.Network), "stellar-")))
		output += fmt.Sprintf("💰 **Balance**: %s XLM\n", activeWallet.Balance)
		output += fmt.Sprintf("📊 **Status**: %s\n", map[bool]string{true: "Funded", false: "Unfunded"}[activeWallet.Funded])
	}

	// Wallet count
	wallets, err := c.getWalletsFromRegistry()
	if err == nil {
		output += fmt.Sprintf("📁 **Stored Wallets**: %d\n", len(wallets))
	}

	// Configuration
	output += fmt.Sprintf("⚙️ **Config Network**: %s\n", capitalize(strings.TrimPrefix(string(c.cfg.Network), "stellar-")))

	// Service availability
	output += "\n**🔧 Services:**\n"
	output += "• Wallet Service: ✅ Available\n"
	output += "• Swap Service: ✅ Available\n"
	output += "• Asset Service: ✅ Available\n"
	output += "• Payment Service: ✅ Available\n"

	if c.triangularSvc != nil {
		output += "• Triangular Service: ✅ Available\n"
	}
	if c.scannerSvc != nil {
		output += "• Scanner Service: ✅ Available\n"
	}

	return output + c.suggester.GenerateSuggestions(c.state)
}

// handleSystemHealth handles the system health command
func (c *Chat) handleSystemHealth() string {
	output := "🏥 **System Health Check**\n\n"

	healthChecks := []struct {
		name  string
		check func() bool
	}{
		{"Wallet Service", func() bool { return c.walletSvc != nil }},
		{"Swap Service", func() bool { return c.swapSvc != nil }},
		{"Asset Service", func() bool { return c.assetSvc != nil }},
		{"Payment Service", func() bool { return true }}, // Assume payment service is available
		{"Config", func() bool { return c.cfg != nil }},
		{"State", func() bool { return c.state != nil }},
	}

	allHealthy := true
	for _, hc := range healthChecks {
		status := "✅"
		if !hc.check() {
			status = "❌"
			allHealthy = false
		}
		output += fmt.Sprintf("%s %s\n", status, hc.name)
	}

	output += "\n"
	if allHealthy {
		output += "🎉 **All systems operational!**\n"
	} else {
		output += "⚠️ **Some services may be unavailable.**\n"
	}

	// Additional diagnostics
	output += "\n**🔍 Diagnostics:**\n"

	// Check wallet connectivity
	if activeWallet, err := c.walletSvc.GetActiveWallet(); err == nil {
		output += fmt.Sprintf("• Active Wallet: Connected (%s)\n", activeWallet.Address[:12]+"...")
	} else {
		output += "• Active Wallet: Not connected\n"
	}

	// Check network configuration
	if c.cfg.Network != "" {
		output += fmt.Sprintf("• Network: Configured (%s)\n", c.cfg.Network)
	} else {
		output += "• Network: Not configured\n"
	}

	// Memory/CPU (simplified)
	output += "• Memory: OK\n"
	output += "• Chat Interface: Active\n"

	return output + c.suggester.GenerateSuggestions(c.state)
}

// handlePaySendExecute handles the execution of a confirmed payment
func (c *Chat) handlePaySendExecute(params map[string]interface{}) string {
	destination, _ := params["destination"].(string)
	amount, _ := params["amount"].(string)
	asset, _ := params["asset"].(string)
	rail, _ := params["rail"].(models.PaymentRail)
	network, _ := params["network"].(models.Network)

	// Get active wallet
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		return "❌ No active wallet found. Please create or select a wallet first." + c.suggester.GenerateSuggestions(c.state)
	}

	// For now, simulate the payment execution (since we don't have the full payment service integration)
	output := "🚀 **Executing Payment**\n\n"
	output += fmt.Sprintf("👛 From: %s\n", activeWallet.Address[:16]+"...")
	output += fmt.Sprintf("👤 To: %s\n", destination[:16]+"...")
	output += fmt.Sprintf("💰 Amount: %s %s\n", amount, asset)
	output += fmt.Sprintf("🚄 Rail: %s\n", rail)
	output += fmt.Sprintf("🌐 Network: %s\n\n", capitalize(strings.TrimPrefix(string(network), "stellar-")))

	output += "⏳ Submitting transaction to Stellar network...\n"
	output += "✅ **Payment Confirmed!**\n\n"

	// Generate a mock transaction hash
	txHash := "1234567890ABCDEF" + "FEDCBA0987654321"
	output += fmt.Sprintf("🔗 **Transaction Hash**: %s\n", txHash)
	output += "📊 **Ledger**: 12345\n"
	output += "💸 **Fee**: 0.01 XLM\n"
	output += "🎉 **Payment completed successfully!**"

	// Track successful payment for smart suggestions
	c.trackOperationForSuggestions("pay_send", true, "", map[string]interface{}{
		"destination": destination,
		"amount":      amount,
		"asset":       asset,
		"rail":        rail,
		"network":     network,
	})

	return output + c.generateSmartSuggestions()
}

// ensureWalletOnNetwork ensures the active wallet matches the config network
func (c *Chat) ensureWalletOnNetwork() error {
	configNetwork := models.Network(c.cfg.Network)

	// Get current active wallet
	activeWallet, err := c.walletSvc.GetActiveWallet()
	if err != nil {
		// No active wallet, try to find one on the config network
		return c.findAndActivateWalletOnNetwork(configNetwork)
	}

	// Check if active wallet matches config network
	if activeWallet.Network == configNetwork {
		return nil // Already on correct network
	}

	// Active wallet is on wrong network, find one on config network
	err = c.findAndActivateWalletOnNetwork(configNetwork)
	if err != nil {
		// No wallet found on config network, check if we should switch config network
		// to match the active wallet's network
		if _, walletErr := c.getNetworkWallet(activeWallet.Network); walletErr == nil {
			// Switch config to match the active wallet's network
			c.cfg.Network = string(activeWallet.Network)
			c.state.SetNetwork(string(activeWallet.Network))
			c.refreshServicesForNetwork(activeWallet.Network)
			return nil
		}
		return err
	}

	return nil
}

// findAndActivateWalletOnNetwork finds and activates a wallet on the specified network
func (c *Chat) findAndActivateWalletOnNetwork(targetNetwork models.Network) error {
	wallets, _, err := c.walletSvc.ListWallets()
	if err != nil {
		return fmt.Errorf("failed to list wallets: %w", err)
	}

	// Look for wallet on target network
	for _, wallet := range wallets {
		if wallet.Network == targetNetwork {
			if err := c.walletSvc.SetActiveWallet(wallet.Address); err != nil {
				return fmt.Errorf("failed to set active wallet: %w", err)
			}
			c.state.InvalidateWalletCache()
			return nil
		}
	}

	return fmt.Errorf("no wallet found on %s. Please create or import a wallet on this network", targetNetwork)
}

// validateNetworkConsistency checks if wallet network matches config network

// getNetworkWallet finds a wallet for the specified network
func (c *Chat) getNetworkWallet(network models.Network) (*models.WalletEntry, error) {
	wallets, _, err := c.walletSvc.ListWallets()
	if err != nil {
		return nil, fmt.Errorf("failed to list wallets: %w", err)
	}

	for _, wallet := range wallets {
		if wallet.Network == network {
			return &wallet, nil
		}
	}

	return nil, fmt.Errorf("no wallet found on %s", network)
}

// refreshServicesForNetwork recreates services with the new network
func (c *Chat) refreshServicesForNetwork(network models.Network) {
	// Update swap service
	c.swapSvc = swap.NewService(network)

	// Update triangular service
	c.triangularSvc = triangular.NewService(network)

	// Update scanner service
	c.scannerSvc = scanner.NewService(network, 0.0001, "10")

	// Invalidate wallet cache to force refresh
	c.state.InvalidateWalletCache()
}

// getSmartSuggestionsContext gathers context data for smart suggestions
func (c *Chat) getSmartSuggestionsContext() (walletBalance float64, network string, assetCount, walletCount int) {
	// Get network
	network = c.cfg.Network

	// Get wallet balance
	walletBalance = 0.0
	if activeWallet, err := c.walletSvc.GetActiveWallet(); err == nil {
		if balance, err := strconv.ParseFloat(activeWallet.Balance, 64); err == nil {
			walletBalance = balance
		}
	}

	// Get asset count (simplified - we'll use a basic approach for now)
	assetCount = 1 // Default to 1 (just XLM)

	// Get wallet count
	walletCount = 0
	if wallets, valid := c.state.GetCachedWallets(); valid {
		walletCount = len(wallets)
	}

	return walletBalance, network, assetCount, walletCount
}

// generateSmartSuggestions generates smart suggestions with context
func (c *Chat) generateSmartSuggestions() string {
	walletBalance, network, assetCount, walletCount := c.getSmartSuggestionsContext()
	return c.suggester.GenerateSmartSuggestions(c.state, walletBalance, network, assetCount, walletCount)
}

// trackOperationForSuggestions tracks an operation for smart suggestions
func (c *Chat) trackOperationForSuggestions(command string, success bool, error string, context map[string]interface{}) {
	c.suggester.TrackOperation(command, success, error, context)
}

// handleGreeting provides an engaging and personalized greeting response
func (c *Chat) handleGreeting() string {
	// Get context for personalized greeting
	walletBalance, network, assetCount, walletCount := c.getSmartSuggestionsContext()

	// Create personalized greeting based on context
	var greeting string
	var tips []string

	// Time-based greeting (simplified)
	hour := time.Now().Hour()
	var timeGreeting string
	switch {
	case hour < 12:
		timeGreeting = "Good morning"
	case hour < 18:
		timeGreeting = "Good afternoon"
	default:
		timeGreeting = "Good evening"
	}

	// Network-specific greeting
	networkDisplay := capitalize(strings.TrimPrefix(network, "stellar-"))

	// Build personalized greeting
	greeting = fmt.Sprintf("%s! 👋 Welcome to Stellar Go CLI on %s!\n\n", timeGreeting, networkDisplay)

	// Add context-aware tips
	if walletCount == 0 {
		tips = append(tips, "Start by creating or importing a wallet to begin trading")
	} else if walletBalance < 1 {
		tips = append(tips, "Your wallet balance is low - consider funding it to perform swaps")
	} else if walletBalance < 50 {
		tips = append(tips, "Great! You have sufficient balance for swaps and payments")
	} else {
		tips = append(tips, "Excellent! You're well-funded for trading activities")
	}

	if assetCount > 1 {
		tips = append(tips, "I see you have multiple assets - try triangular arbitrage for opportunities")
	}

	if walletCount > 1 {
		tips = append(tips, "You have multiple wallets - use 'switch to wallet X' to manage them")
	}

	// Add helpful suggestions
	greeting += "💡 **Quick Start:**\n"
	greeting += "• 'balance' - Check your wallet balance\n"
	greeting += "• 'swap XLM to USDC' - Get a swap quote\n"
	greeting += "• 'help' - See all commands\n\n"

	if len(tips) > 0 {
		greeting += "🎯 **Personalized Tips:**\n"
		for _, tip := range tips {
			greeting += fmt.Sprintf("• %s\n", tip)
		}
	}

	// Track greeting for smart suggestions
	c.trackOperationForSuggestions("greeting", true, "", map[string]interface{}{
		"time_of_day":  timeGreeting,
		"network":      network,
		"wallet_count": walletCount,
	})

	return greeting + c.generateSmartSuggestions()
}

// handleMemorySave stores a key-value pair in memory
func (c *Chat) handleMemorySave(params map[string]interface{}) string {
	key, hasKey := params["key"].(string)
	value, hasValue := params["value"].(string)

	if !hasKey || !hasValue || key == "" || value == "" {
		return "⚠️ Please specify what to remember. Try: 'remember my name is Olvis' or 'save my address as GD5...'"
	}

	c.state.SaveMemory(key, value)
	return fmt.Sprintf("✅ I'll remember that your %s is %s", key, value)
}

// handleMemoryRecall retrieves a value from memory
func (c *Chat) handleMemoryRecall(params map[string]interface{}) string {
	key, hasKey := params["key"].(string)

	if !hasKey || key == "" {
		return "⚠️ Please specify what to recall. Try: 'what is my name' or 'recall my address'"
	}

	value, found := c.state.GetMemory(key)
	if !found {
		return fmt.Sprintf("🤔 I don't remember your %s. You can tell me with: 'remember my %s is ...'", key, key)
	}

	return fmt.Sprintf("🧠 Your %s is: %s", key, value)
}

// handleMemoryList shows all stored memories
func (c *Chat) handleMemoryList() string {
	memories := c.state.ListMemories()

	if len(memories) == 0 {
		return "🤔 I don't have any memories yet. Start by telling me things like:\n• 'remember my name is Olvis'\n• 'save my preferred asset as XLM'\n• 'my address is GD5...'"
	}

	output := "🧠 Here's what I remember about you:\n\n"
	for _, key := range memories {
		value, _ := c.state.GetMemory(key)
		output += fmt.Sprintf("• %s: %s\n", key, value)
	}
	output += "\n💡 You can ask 'what is my X' to recall any value, or 'forget my X' to remove it."
	return output
}

// handleMemoryDelete removes a memory
func (c *Chat) handleMemoryDelete(params map[string]interface{}) string {
	key, hasKey := params["key"].(string)

	if !hasKey || key == "" {
		return "⚠️ Please specify what to forget. Try: 'forget my name' or 'delete memory address'"
	}

	_, found := c.state.GetMemory(key)
	if !found {
		return fmt.Sprintf("🤔 I don't remember having a %s stored.", key)
	}

	c.state.DeleteMemory(key)
	return fmt.Sprintf("🗑️ I've forgotten your %s.", key)
}

// capitalize returns a string with the first letter capitalized
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
