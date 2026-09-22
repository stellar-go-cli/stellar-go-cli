# Swap Operations Skills

## Overview
Advanced swap and arbitrage capabilities for the Stellar Go CLI platform, including quoting, execution, monitoring, and automated trading strategies across multiple asset pairs and networks.

## Skills

### swap_quote
**Description**: Get a swap quote for converting between assets using Stellar path payments
**Category**: swap_operations
**MCP Tool**: swap_quote

**Parameters**:
- `from` (string, required): Source asset code (e.g., XLM, USDC)
- `to` (string, required): Destination asset code (e.g., USDC, EURC)
- `amount` (string, required): Amount to swap
- `type` (enum): strict-send | strict-receive (default: strict-send)

**Natural Language Patterns**:
- "swap 100 XLM to USDC"
- "get quote for XLM to USDC exchange"
- "how much USDC for 50 XLM"
- "quote for swapping XLM to EURC"
- "exchange rate XLM to USDC"

**Examples**:
```bash
# Basic quote
"swap 100 XLM to USDC"

# Different swap type
"strict receive 50 USDC from XLM"

# Complex assets
"quote for 200 XLM to EURC"
```

**Response Elements**:
- Exchange rate and fees
- Estimated amount received
- Price impact and slippage
- Path details
- Execution time estimate

**Follow-up Questions**:
- "Would you like to execute this swap?"
- "Do you want to see alternative paths?"
- "Should I set up a limit order?"

---

### swap_execute
**Description**: Execute an asset swap using Stellar path payments
**Category**: swap_operations
**MCP Tool**: swap_execute

**Parameters**:
- `from` (string, required): Source asset code
- `to` (string, required): Destination asset code
- `amount` (string, required): Amount to swap
- `type` (enum): strict-send | strict-receive (default: strict-send)
- `slippage` (number): Maximum slippage percentage (optional)
- `execute` (boolean): Actually execute the transaction (default: true)

**Natural Language Patterns**:
- "execute swap 100 XLM to USDC"
- "swap 50 XLM for USDC now"
- "perform the swap we quoted"
- "exchange XLM to USDC"
- "complete the swap transaction"

**Examples**:
```bash
# Execute swap
"execute swap 100 XLM to USDC"

# With slippage protection
"swap 200 XLM to USDC with 2% slippage"

# Dry run
"simulate swap 50 XLM to USDC"
```

**Execution Process**:
1. Validate current market conditions
2. Check slippage limits
3. Build transaction path
4. Submit to network
5. Monitor confirmation
6. Report results

**Follow-up Actions**:
- "Would you like to set a price alert?"
- "Should I monitor this transaction?"
- "Do you want to save this swap as a template?"

---

### swap_scan
**Description**: Scan for profitable swap opportunities across different paths
**Category**: swap_operations
**MCP Tool**: swap_scan

**Parameters**:
- `amount` (string): Amount to scan with (default: 10)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)
- `output` (enum): pretty | json (default: pretty)

**Natural Language Patterns**:
- "scan for swap opportunities"
- "find profitable swaps"
- "scan market for good rates"
- "look for arbitrage opportunities"
- "check current swap rates"

**Examples**:
```bash
# Basic scan
"scan for swap opportunities"

# With specific amount
"scan for swaps with 100 XLM"

# Specific network
"scan testnet for swap opportunities"
```

**Scan Results**:
- Best available rates
- Multiple path options
- Profitability analysis
- Liquidity information
- Market depth data

**Follow-up Questions**:
- "Would you like to execute the best opportunity?"
- "Should I set up monitoring for these pairs?"
- "Do you want to see historical data?"

---

### swap_monitor
**Description**: Monitor swap opportunities and alert when profitable conditions appear
**Category**: swap_operations
**MCP Tool**: swap_monitor

**Parameters**:
- `pairs` (string): Asset pairs to monitor (comma-separated)
- `min_profit` (number): Minimum profit threshold (optional)
- `duration` (number): Monitor duration in seconds (default: 60)

**Natural Language Patterns**:
- "monitor XLM/USDC swaps"
- "watch for profitable opportunities"
- "set up swap monitoring"
- "alert me when rates are good"
- "monitor market for arbitrage"

**Examples**:
```bash
# Monitor specific pairs
"monitor XLM/USDC and USDC/EURC swaps"

# With profit threshold
"monitor swaps with at least 1% profit"

# Extended monitoring
"monitor market for 10 minutes"
```

**Monitoring Features**:
- Real-time rate tracking
- Profit threshold alerts
- Market condition analysis
- Liquidity monitoring
- Historical comparison

**Alert Types**:
- Profit opportunity detected
- Significant rate changes
- Liquidity warnings
- Market anomalies

---

### swap_arbitrage_all
**Description**: Comprehensive arbitrage scan across all asset pairs and paths
**Category**: swap_operations
**MCP Tool**: swap_arbitrage_all

**Parameters**:
- `min_profit_xlm` (number): Minimum profit in XLM (optional)
- `max_depth` (number): Maximum search depth (default: 3)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)

**Natural Language Patterns**:
- "scan for arbitrage opportunities"
- "find all profitable paths"
- "comprehensive arbitrage scan"
- "search for multi-asset arbitrage"
- "find complex arbitrage opportunities"

**Examples**:
```bash
# Full scan
"scan for all arbitrage opportunities"

# With profit threshold
"find arbitrage with at least 5 XLM profit"

# Deep search
"comprehensive arbitrage scan with depth 5"
```

**Arbitrage Types**:
- Simple 2-asset arbitrage
- 3-asset triangular arbitrage
- Multi-asset complex paths
- Cross-network opportunities
- Time-based arbitrage

**Analysis Metrics**:
- Profit calculations
- Risk assessment
- Liquidity requirements
- Execution complexity
- Time sensitivity

---

### swap_triangular
**Description**: Perform triangular arbitrage with 3-leg cycles (XLM→USDC→yXLM→XLM)
**Category**: swap_operations
**MCP Tool**: swap_triangular

**Parameters**:
- `action` (enum): scan | monitor | backtest (default: scan)
- `amount` (string): Starting XLM amount (default: 10)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)

**Natural Language Patterns**:
- "scan for triangular arbitrage"
- "find 3-leg arbitrage opportunities"
- "triangular arbitrage with 100 XLM"
- "monitor triangular cycles"
- "backtest triangular strategies"

**Examples**:
```bash
# Scan for opportunities
"scan for triangular arbitrage opportunities"

# With specific amount
"triangular arbitrage with 50 XLM"

# Monitor mode
"monitor triangular arbitrage cycles"
```

**Triangular Patterns**:
- XLM → USDC → yXLM → XLM
- XLM → BTC → USDC → XLM
- XLM → EURC → USDC → XLM
- Custom 3-asset cycles

**Performance Metrics**:
- Cycle profitability
- Execution time
- Slippage impact
- Success rate
- Risk factors

---

### swap_assets
**Description**: List available assets and their swap pairs
**Category**: swap_operations
**MCP Tool**: swap_assets

**Natural Language Patterns**:
- "list available swap assets"
- "show all tradable assets"
- "what assets can I swap?"
- "list swap pairs"
- "show market assets"

**Examples**:
```bash
# List all assets
"show all swap assets"

# Specific query
"what assets are available for swapping?"
```

**Asset Information**:
- Available asset codes
- Trading pairs
- Liquidity status
- Market depth
- Recent activity

---

## Workflows

### Quick Swap Execution
1. **Quote**: "Get me a quote for 100 XLM to USDC"
2. **Review**: "Show me the details and fees"
3. **Execute**: "Execute the swap"
4. **Confirm**: "Monitor the transaction"

### Arbitrage Detection
1. **Scan**: "Scan for arbitrage opportunities"
2. **Analyze**: "Show me the most profitable paths"
3. **Evaluate**: "What are the risks and requirements?"
4. **Execute**: "Execute the best arbitrage opportunity"

### Market Monitoring
1. **Setup**: "Monitor XLM/USDC and USDC/EURC"
2. **Configure**: "Alert me for profits over 1%"
3. **Monitor**: "Watch the market for 30 minutes"
4. **Act**: "Execute when good opportunities appear"

### Portfolio Rebalancing
1. **Assess**: "Show my current asset allocation"
2. **Identify**: "Find swaps to rebalance my portfolio"
3. **Execute**: "Execute the rebalancing swaps"
4. **Verify**: "Confirm the new allocation"

## Advanced Concepts

### Path Payments
Understanding how Stellar path payments work:
- Multi-asset routing
- Liquidity aggregation
- Fee optimization
- Slippage management

### Arbitrage Strategies
- Simple arbitrage (2 assets)
- Triangular arbitrage (3 assets)
- Complex arbitrage (4+ assets)
- Cross-network arbitrage
- Time-based arbitrage

### Risk Management
- Slippage protection
- Market depth analysis
- Liquidity assessment
- Execution timing
- Portfolio impact

## Common Questions

**Q: What's the difference between strict-send and strict-receive?**
A: Strict-send guarantees the amount you send, strict-receive guarantees the amount you receive.

**Q: How accurate are the profit calculations?**
A: They're based on current market conditions, but actual profits may vary due to slippage and timing.

**Q: Can I lose money with arbitrage?**
A: Yes, market conditions can change quickly, and execution costs may exceed profits.

**Q: What's the minimum amount for profitable arbitrage?**
A: It depends on the assets and market conditions, but typically 100+ XLM is needed.

## Tips

- Always check current market conditions before executing
- Use slippage protection for larger swaps
- Monitor multiple asset pairs for better opportunities
- Consider transaction costs in profit calculations
- Set realistic profit thresholds
- Diversify arbitrage strategies
- Keep track of successful and failed trades
- Learn from market patterns and trends
