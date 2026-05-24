// Package chat provides an interactive chat interface for MozartPay CLI
package chat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ogtechnologies/mozartpay/internal/models"
)

// Formatter handles output formatting with emojis and styling
type Formatter struct{}

// NewFormatter creates a new output formatter
func NewFormatter() *Formatter {
	return &Formatter{}
}

// FormatGreeting returns a greeting message
func (f *Formatter) FormatGreeting(network string) string {
	networkDisplay := strings.TrimPrefix(network, "stellar-")
	return fmt.Sprintf(`👋 Hello! I'm your MozartPay assistant on %s.
Try asking:
• 'what's my balance'
• 'swap XLM to USDC'
• 'network' to check current network
• 'help' for more commands`, strings.Title(networkDisplay))
}

// FormatWalletList formats wallet list for display
func (f *Formatter) FormatWalletList(wallets []models.WalletEntry, network string) string {
	if len(wallets) == 0 {
		networkDisplay := strings.TrimPrefix(network, "stellar-")
		return fmt.Sprintf("📋 **No wallets found on %s**\n💡 Use 'use testnet' or 'use mainnet' to switch networks", strings.Title(networkDisplay))
	}

	networkDisplay := strings.TrimPrefix(network, "stellar-")
	lines := []string{
		fmt.Sprintf("📋 **Wallets on %s** (%d found)", strings.Title(networkDisplay), len(wallets)),
		"",
	}

	for i, wallet := range wallets {
		activeMarker := fmt.Sprintf("%d. ", i+1)
		name := wallet.Name
		if name == "" {
			name = "Unnamed"
		}
		lines = append(lines, fmt.Sprintf("%s**%s**", activeMarker, name))
		lines = append(lines, fmt.Sprintf("📍 Address: `%s...`", f.truncateAddress(wallet.Address, 20)))
		lines = append(lines, fmt.Sprintf("💰 Balance: %s XLM", wallet.Balance))
		lines = append(lines, fmt.Sprintf("🌐 Network: %s", strings.Title(strings.TrimPrefix(string(wallet.Network), "stellar-"))))

		status := "❌ Not funded"
		if wallet.Funded {
			status = "✅ Funded"
		}
		lines = append(lines, fmt.Sprintf("✅ Status: %s", status))
		lines = append(lines, "")
	}

	lines = append(lines, "💡 **Tips:**")
	lines = append(lines, "• Use 'switch to wallet <number>' to change active wallet")
	lines = append(lines, "• Use 'show my wallet' for detailed active wallet info")
	lines = append(lines, "• Use 'use testnet' or 'use mainnet' to switch networks")

	return strings.Join(lines, "\n")
}

// FormatWalletShow formats single wallet details
func (f *Formatter) FormatWalletShow(wallet *models.WalletEntry) string {
	if wallet == nil {
		return "❌ No active wallet found on current network"
	}

	name := wallet.Name
	if name == "" {
		name = "Unnamed"
	}

	funded := "❌ No"
	if wallet.Funded {
		funded = "✅ Yes"
	}

	return fmt.Sprintf(`📋 **Wallet Details:**
**Name:** %s
**Address:** %s
**Network:** %s
**Type:** %s
**Balance:** %s XLM
**Funded:** %s`,
		name,
		wallet.Address,
		strings.Title(strings.TrimPrefix(string(wallet.Network), "stellar-")),
		wallet.Type,
		wallet.Balance,
		funded,
	)
}

// FormatWalletBalance formats wallet balance display
func (f *Formatter) FormatWalletBalance(wallet *models.WalletEntry) string {
	if wallet == nil {
		return "❌ No active wallet found on current network"
	}

	funded := "❌ Not funded"
	if wallet.Funded {
		funded = "✅ Funded"
	}

	return fmt.Sprintf(`💰 **Wallet Balance:**
**Address:** %s
**Balance:** %s XLM
**Network:** %s
**Status:** %s`,
		wallet.Address,
		wallet.Balance,
		strings.Title(strings.TrimPrefix(string(wallet.Network), "stellar-")),
		funded,
	)
}

// FormatNetworkStatus formats current network display
func (f *Formatter) FormatNetworkStatus(network string) string {
	networkDisplay := strings.TrimPrefix(network, "stellar-")
	return fmt.Sprintf("🌐 **Current Network:** %s\n💡 Use 'use testnet' or 'use mainnet' to switch networks", strings.Title(networkDisplay))
}

// FormatNetworkSwitched formats network switch confirmation
func (f *Formatter) FormatNetworkSwitched(network string) string {
	networkDisplay := strings.TrimPrefix(network, "stellar-")
	return fmt.Sprintf("🌐 Network switched to %s", strings.Title(networkDisplay))
}

// FormatSwapQuote formats swap quote for display
func (f *Formatter) FormatSwapQuote(swap *SwapQuote) string {
	if swap == nil {
		return "❌ No swap quote available"
	}

	lines := []string{
		"💱 **Swap Quote:**",
		fmt.Sprintf("**From:** %s → **To:** %s", swap.From, swap.To),
		fmt.Sprintf("**Amount:** %s", swap.Amount),
	}

	if swap.Rate != "" {
		lines = append(lines, fmt.Sprintf("**Rate:** %s", swap.Rate))
	}
	if swap.Estimated != "" {
		lines = append(lines, fmt.Sprintf("**Estimated:** %s", swap.Estimated))
	}
	if swap.MinReceived != "" {
		lines = append(lines, fmt.Sprintf("**Min Received:** %s", swap.MinReceived))
	}
	if swap.PriceImpact > 0 {
		lines = append(lines, fmt.Sprintf("**Price Impact:** %.2f%%", swap.PriceImpact))
	}

	lines = append(lines, "")
	lines = append(lines, "🔄 **Execute this swap?**")
	lines = append(lines, "Type: **yes** to execute, **no** to cancel")

	return strings.Join(lines, "\n")
}

// FormatSwapReport formats swap execution report with Stellar Expert link
func (f *Formatter) FormatSwapReport(swap *SwapQuote, status string, txHash string, fees string, network string, walletAddress string) string {
	// Determine explorer URL based on network
	var explorerURL string
	if strings.Contains(network, "testnet") {
		explorerURL = fmt.Sprintf("https://stellar.expert/explorer/testnet/tx/%s", txHash)
	} else {
		explorerURL = fmt.Sprintf("https://stellar.expert/explorer/public/tx/%s", txHash)
	}

	switch status {
	case "success":
		return fmt.Sprintf(`🎉 **Swap Executed Successfully!**

💱 **Swap Details:**
**From:** %s → **To:** %s
**Amount:** %s %s
**Wallet:** %s

📊 **Transaction Information:**
**Status:** ✅ Confirmed on-ledger
**Transaction Hash:** %s
**Fees:** %s

🔗 **View on Stellar Expert:**
%s`,
			swap.From, swap.To, swap.Amount, swap.From, walletAddress, txHash, fees, explorerURL)

	case "cancelled":
		return fmt.Sprintf(`❌ **Swap Cancelled**

💱 **Swap Details:**
**From:** %s → **To:** %s
**Amount:** %s
**Wallet:** %s

The swap was cancelled by user request.`,
			swap.From, swap.To, swap.Amount, walletAddress)

	case "error":
		return fmt.Sprintf(`❌ **Swap Failed**

💱 **Swap Details:**
**From:** %s → **To:** %s
**Amount:** %s
**Wallet:** %s

**Error:** %s

🔗 **View Account on Stellar Expert:**
%s

Please check your wallet balance and network connectivity, then try again.`,
			swap.From, swap.To, swap.Amount, walletAddress, fees, explorerURL)

	default:
		return fmt.Sprintf(`❓ **Swap Status Unknown**

💱 **Swap Details:**
**From:** %s → **To:** %s
**Amount:** %s
**Wallet:** %s

The swap execution status could not be determined. Please check your transaction history.`,
			swap.From, swap.To, swap.Amount, walletAddress)
	}
}

// FormatHelp returns the help text
func (f *Formatter) FormatHelp(currentNetwork string) string {
	networkDisplay := strings.TrimPrefix(currentNetwork, "stellar-")
	return fmt.Sprintf(`🤖 **Available Commands:**
• 'balance' or 'what's my balance' - Check wallet balance
• 'wallet' or 'wallet list' - Show all wallets on current network
• 'show my wallet' or 'wallet show' - Display active wallet details
• 'swap XLM to USDC' or 'swap 100 XLM to USDC' - Get swap quote
• 'yes' or 'ok' - Execute pending swap
• 'no' or 'cancel' - Cancel pending operation
• 'network' or 'current network' - Show current network
• 'use testnet' or 'use mainnet' - Switch network
• 'change to testnet' or 'switch to mainnet' - Switch networks
• 'send 100 USDC to GADDRESS...' - Send payment
• 'assets' - Show wallet assets
• 'system status' - Check system
• 'help' - Show this help
• 'quit' - Exit chat

**Tips:**
• I can guide you through complex operations step-by-step
• Just start a command and I'll ask for missing details
• After swap quotes, type 'yes' to execute or 'no' to cancel
• Switch networks anytime with 'use testnet' or 'change to mainnet'
• Type 'cancel' anytime to stop a multi-step operation

🌐 **Current Network:** %s`, strings.Title(networkDisplay))
}

// FormatParameterPrompt generates a prompt for collecting a parameter
func (f *Formatter) FormatParameterPrompt(param ParamInfo) string {
	var prompt string

	switch param.Name {
	case "from":
		prompt = "🏦 What asset are you sending from?"
	case "to":
		prompt = "📍 What asset are you swapping to?"
	case "amount":
		prompt = "💰 How much would you like to swap?"
	case "destination":
		prompt = "📍 What's the recipient address?"
	default:
		prompt = fmt.Sprintf("📌 %s", param.Description)
	}

	if len(param.Examples) > 0 {
		prompt += fmt.Sprintf("\n   Example: %s", param.Examples[0])
	}

	if len(param.Enum) > 0 {
		prompt += fmt.Sprintf("\n   Options: %s", strings.Join(param.Enum, ", "))
	}

	prompt += "\n\n_(Type your answer, or 'cancel' to stop)_"

	return prompt
}

// FormatError formats an error message
func (f *Formatter) FormatError(err error) string {
	return fmt.Sprintf("❌ Error: %s", err.Error())
}

// FormatSuccess formats a success message
func (f *Formatter) FormatSuccess(msg string) string {
	return fmt.Sprintf("✅ %s", msg)
}

// FormatCancellation formats a cancellation message
func (f *Formatter) FormatCancellation() string {
	return "❌ Operation cancelled. What would you like to do instead?"
}

// FormatUnknown formats unknown command response
func (f *Formatter) FormatUnknown() string {
	return "🤔 I'm not sure how to help with that. Try 'help' to see what I can do, or just start describing what you want to accomplish!"
}

// FormatGoodbye formats goodbye message
func (f *Formatter) FormatGoodbye() string {
	return "👋 Goodbye!"
}

// truncateAddress truncates an address for display
func (f *Formatter) truncateAddress(address string, length int) string {
	if len(address) <= length {
		return address
	}
	return address[:length]
}

// FormatPoolList formats a list of liquidity pools for display
func (f *Formatter) FormatPoolList(pools []models.LiquidityPool, network string) string {
	if len(pools) == 0 {
		networkDisplay := strings.TrimPrefix(network, "stellar-")
		return fmt.Sprintf("🏊 **No liquidity pools found on %s**\n💡 Pools are created when users deposit assets into the AMM", strings.Title(networkDisplay))
	}

	networkDisplay := strings.TrimPrefix(network, "stellar-")
	lines := []string{
		fmt.Sprintf("🏊 **Liquidity Pools on %s** (%d found)", strings.Title(networkDisplay), len(pools)),
		"",
	}

	for i, pool := range pools {
		// Extract asset pair
		pair := f.formatPoolPair(pool)
		feePercent := float64(pool.FeeBP) / 100.0

		// Calculate reserves
		var reserveInfo string
		if len(pool.Reserves) == 2 {
			reserveA := f.formatReserve(pool.Reserves[0])
			reserveB := f.formatReserve(pool.Reserves[1])
			reserveInfo = fmt.Sprintf("📊 %s / %s", reserveA, reserveB)
		} else {
			reserveInfo = "📊 Multiple reserves"
		}

		lines = append(lines, fmt.Sprintf("%d. **%s**", i+1, pair))
		lines = append(lines, fmt.Sprintf("   🔑 ID: `%s...`", f.truncateAddress(pool.ID, 12)))
		lines = append(lines, fmt.Sprintf("   %s", reserveInfo))
		lines = append(lines, fmt.Sprintf("   💧 Fee: %.2f%% | Shares: %s", feePercent, f.formatShares(pool.TotalShares)))
		lines = append(lines, "")
	}

	lines = append(lines, "💡 **Commands:**")
	lines = append(lines, "• 'pool list' - Show all pools")
	lines = append(lines, "• 'pool list --asset XLM' - Filter by asset")
	lines = append(lines, "• 'pool info <id>' - Pool details")

	return strings.Join(lines, "\n")
}

// FormatPoolDetail formats detailed pool information
func (f *Formatter) FormatPoolDetail(pool *models.LiquidityPool, network string) string {
	if pool == nil {
		return "❌ Pool not found"
	}

	pair := f.formatPoolPair(*pool)
	feePercent := float64(pool.FeeBP) / 100.0

	var explorerURL string
	if strings.Contains(network, "testnet") {
		explorerURL = fmt.Sprintf("https://stellar.expert/explorer/testnet/liquidity-pool/%s", pool.ID)
	} else {
		explorerURL = fmt.Sprintf("https://stellar.expert/explorer/public/liquidity-pool/%s", pool.ID)
	}

	lines := []string{
		fmt.Sprintf("🏊 **Pool Details: %s**", pair),
		"",
		fmt.Sprintf("🔑 **Pool ID:** `%s`", pool.ID),
		fmt.Sprintf("📋 **Type:** %s", pool.Type),
		fmt.Sprintf("💧 **Fee:** %.2f%% (%d basis points)", feePercent, pool.FeeBP),
		fmt.Sprintf("🎫 **Total Shares:** %s", f.formatShares(pool.TotalShares)),
		"",
		"**Reserves:**",
	}

	for _, reserve := range pool.Reserves {
		asset := f.formatAssetName(reserve.Asset)
		lines = append(lines, fmt.Sprintf("  • %s: %s", asset, reserve.Amount))
	}

	// Calculate price if 2 reserves
	if len(pool.Reserves) == 2 {
		lines = append(lines, "")
		lines = append(lines, "**Prices:**")
		assetA := f.formatAssetName(pool.Reserves[0].Asset)
		assetB := f.formatAssetName(pool.Reserves[1].Asset)

		reserveAAmount, _ := strconv.ParseFloat(pool.Reserves[0].Amount, 64)
		reserveBAmount, _ := strconv.ParseFloat(pool.Reserves[1].Amount, 64)

		if reserveAAmount > 0 {
			priceAtoB := reserveBAmount / reserveAAmount
			lines = append(lines, fmt.Sprintf("  • 1 %s = %.7f %s", assetA, priceAtoB, assetB))
		}
		if reserveBAmount > 0 {
			priceBtoA := reserveAAmount / reserveBAmount
			lines = append(lines, fmt.Sprintf("  • 1 %s = %.7f %s", assetB, priceBtoA, assetA))
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("🔗 **View on Stellar Expert:**\n%s", explorerURL))

	return strings.Join(lines, "\n")
}

// Helper methods for pool formatting
func (f *Formatter) formatPoolPair(pool models.LiquidityPool) string {
	if len(pool.Reserves) < 2 {
		return "Unknown Pair"
	}
	a := f.formatAssetName(pool.Reserves[0].Asset)
	b := f.formatAssetName(pool.Reserves[1].Asset)
	return fmt.Sprintf("%s/%s", a, b)
}

func (f *Formatter) formatReserve(reserve models.PoolReserve) string {
	asset := f.formatAssetName(reserve.Asset)
	// Truncate amount to reasonable precision
	amount := reserve.Amount
	if len(amount) > 12 {
		amount = amount[:12]
	}
	return fmt.Sprintf("%s %s", amount, asset)
}

func (f *Formatter) formatAssetName(asset string) string {
	if asset == "native" {
		return "XLM"
	}
	if strings.Contains(asset, ":") {
		parts := strings.Split(asset, ":")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return asset
}

func (f *Formatter) formatShares(shares string) string {
	// Truncate to reasonable length
	if len(shares) > 15 {
		return shares[:15] + "..."
	}
	return shares
}
