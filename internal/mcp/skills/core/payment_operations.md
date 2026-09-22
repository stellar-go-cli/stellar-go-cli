# Payment Operations Skills

## Overview
Comprehensive payment capabilities for the Stellar Go CLI platform, including multi-rail payments, quoting, request generation, and payment history tracking across different payment protocols and networks.

## Skills

### pay_send
**Description**: Send a payment to another address using various payment rails
**Category**: payment_operations
**MCP Tool**: pay_send

**Parameters**:
- `destination` (string, required): Recipient address
- `amount` (string, required): Amount to send
- `asset` (string): Asset code (default: XLM)
- `rail` (enum): direct | x402 | tempo | zk (default: direct)
- `memo` (string): Optional payment memo
- `network` (string): Network (default: stellar-testnet)
- `execute` (boolean): Execute payment (default: true)

**Natural Language Patterns**:
- "send 100 XLM to GD...ADDRESS"
- "pay 50 USDC to my friend"
- "transfer 200 XLM using x402"
- "send payment with memo 'rent payment'"
- "make private payment using ZK"

**Examples**:
```bash
# Basic payment
"send 100 XLM to GD...DESTINATION"

# With specific asset
"pay 50 USDC to GD...ADDRESS"

# With memo
"send 100 XLM to GD...ADDRESS with memo 'invoice #123'"

# Different rail
"send 50 XLM using x402 micropayments"

# Private payment
"send 100 XLM privately using ZK"
```

**Payment Rails**:
- **Direct**: Standard Stellar network payment
- **x402**: Micropayment protocol with streaming
- **Tempo**: Cross-border remittance service
- **ZK**: Zero-knowledge privacy payment

**Execution Process**:
1. Validate recipient address
2. Check balance and fees
3. Build transaction
4. Submit to network
5. Monitor confirmation
6. Provide receipt

**Follow-up Questions**:
- "Would you like to add a memo to this payment?"
- "Should I save this recipient for future payments?"
- "Do you want to set up recurring payments?"

---

### pay_quote
**Description**: Get payment quote across different rails (direct, x402, tempo, zk)
**Category**: payment_operations
**MCP Tool**: pay_quote

**Parameters**:
- `to` (string, required): Recipient address
- `amount` (string, required): Amount to send
- `asset` (string, required): Asset code
- `rail` (enum): direct | x402 | tempo | zk (default: direct)

**Natural Language Patterns**:
- "quote payment to GD...ADDRESS"
- "get quote for sending 100 XLM"
- "compare payment rail costs"
- "show payment fees and timing"
- "quote payment using different rails"

**Examples**:
```bash
# Basic quote
"quote payment of 100 XLM to GD...ADDRESS"

# Compare rails
"show quotes for all payment rails to GD...ADDRESS"

# Specific rail
"quote x402 payment of 50 USDC"
```

**Quote Information**:
- Total fees (network + rail)
- Estimated delivery time
- Exchange rates (if applicable)
- Privacy level
- Confirmation requirements

**Rail Comparisons**:
- **Cost**: Fee structures across rails
- **Speed**: Delivery time estimates
- **Privacy**: Anonymity levels
- **Reliability**: Success rates and guarantees

**Follow-up Questions**:
- "Would you like to execute the best quote?"
- "Should I show you the detailed breakdown?"
- "Do you want to save this quote for later?"

---

### pay_request
**Description**: Generate a payment request (QR code or link)
**Category**: payment_operations
**MCP Tool**: pay_request

**Parameters**:
- `asset` (string, required): Asset code to request
- `amount` (string, required): Amount to request
- `memo` (string): Optional payment memo
- `expires` (string): Expiration time (optional)

**Natural Language Patterns**:
- "request 100 XLM payment"
- "generate payment request for 50 USDC"
- "create invoice for 200 XLM"
- "make payment request with memo"
- "generate QR code for payment"

**Examples**:
```bash
# Basic request
"request 100 XLM payment"

# With memo
"create invoice for 50 USDC with memo 'product purchase'"

# QR code generation
"generate QR code for 200 XLM request"
```

**Request Features**:
- QR code generation
- Payment link creation
- Memo inclusion
- Expiration settings
- Multi-language support

**Request Types**:
- **One-time**: Single payment request
- **Recurring**: Repeating payment schedule
- **Conditional**: Payment upon conditions
- **Multi-asset**: Request multiple assets

**Follow-up Questions**:
- "Would you like to share this request?"
- "Should I set up notifications for payment?"
- "Do you want to create a recurring request?"

---

### pay_history
**Description**: Get recent payment history for the active wallet
**Category**: payment_operations
**MCP Tool**: pay_history

**Parameters**:
- `limit` (number): Number of payments to retrieve (default 20)
- `cursor` (string): Pagination cursor (optional)
- `asset` (string): Filter by asset code (optional)
- `rail` (string): Filter by payment rail (optional)

**Natural Language Patterns**:
- "show my payment history"
- "list recent transactions"
- "show last 10 payments"
- "filter payment history by USDC"
- "show x402 payment history"

**Examples**:
```bash
# Basic history
"show my payment history"

# Limited results
"show last 10 payments"

# Filtered by asset
"show USDC payment history"

# By rail
"show x402 payment transactions"
```

**History Information**:
- Transaction details
- Payment status
- Fees and timing
- Counterparty information
- Memo and metadata

**Filtering Options**:
- By date range
- By asset type
- By payment rail
- By transaction status
- By counterparty

**Follow-up Questions**:
- "Would you like to see details for a specific transaction?"
- "Should I export this history?"
- "Do you want to analyze spending patterns?"

---

### pay_x402
**Description**: Execute payment via x402 micropayment protocol
**Category**: payment_operations
**MCP Tool**: pay_x402

**Parameters**:
- `to` (string, required): Recipient address
- `amount` (string, required): Amount to send
- `asset` (string, required): Asset code
- `execute` (boolean): Execute payment (dry-run if false)

**Natural Language Patterns**:
- "send 100 XLM using x402"
- "pay with micropayment streaming"
- "x402 payment to GD...ADDRESS"
- "stream 50 XLM payment"
- "micropayment of 10 USDC"

**Examples**:
```bash
# x402 payment
"send 100 XLM using x402 to GD...ADDRESS"

# Streaming payment
"stream 50 XLM payment using x402"

# Dry run
"simulate x402 payment of 25 USDC"
```

**x402 Features**:
- Micropayment streaming
- Real-time settlement
- Low fees
- Automatic routing
- Streaming controls

**Use Cases**:
- Content monetization
- API usage billing
- Subscription payments
- Pay-per-use services
- Real-time transfers

**Follow-up Questions**:
- "Would you like to set up streaming controls?"
- "Should I monitor the payment progress?"
- "Do you want to save this recipient?"

---

### pay_zk
**Description**: Execute payment using zero-knowledge proofs
**Category**: payment_operations
**MCP Tool**: pay_zk

**Parameters**:
- `to` (string, required): Recipient address
- `amount` (string, required): Amount to send
- `asset` (string, required): Asset code
- `prove` (boolean): Generate ZK proof (required)

**Natural Language Patterns**:
- "send 100 XLM privately"
- "make anonymous payment"
- "ZK payment to GD...ADDRESS"
- "private transfer of 50 USDC"
- "send payment with privacy"

**Examples**:
```bash
# Private payment
"send 100 XLM privately to GD...ADDRESS"

- "make anonymous payment of 50 USDC"

# With proof
"send 200 XLM with zero-knowledge proof"
```

**ZK Features**:
- Transaction privacy
- Amount concealment
- Sender anonymity
- Proof generation
- Verifiable privacy

**Privacy Levels**:
- **Full**: Complete transaction privacy
- **Partial**: Amount or sender privacy
- **Selective**: Choose privacy elements
- **Auditable**: Privacy with audit capability

**Use Cases**:
- Private business transactions
- Personal privacy protection
- Competitive payment hiding
- Sensitive transfers
- Regulatory compliance

**Follow-up Questions**:
- "Would you like to save the privacy proof?"
- "Should I set up recurring private payments?"
- "Do you need auditable privacy?"

---

### pay_rails
**Description**: Get information about available payment rails and their capabilities
**Category**: payment_operations
**MCP Tool**: pay_rails

**Parameters**:
- `rail` (string): Specific rail to query (optional)

**Natural Language Patterns**:
- "show available payment rails"
- "compare payment methods"
- "what payment rails are available?"
- "show x402 rail information"
- "list all payment options"

**Examples**:
```bash
# All rails
"show all available payment rails"

# Specific rail
"show information about x402 rail"

# Comparison
"compare payment rail features"
```

**Rail Capabilities**:
- **Direct**: Standard Stellar payments
- **x402**: Micropayment streaming
- **Tempo**: Cross-border remittance
- **ZK**: Privacy-preserving payments

**Comparison Metrics**:
- Fee structures
- Delivery speed
- Privacy level
- Geographic availability
- Asset support
- Reliability guarantees

**Rail Selection Guide**:
- **Small amounts**: x402 for low fees
- **International**: Tempo for FX conversion
- **Privacy needed**: ZK for anonymity
- **Standard use**: Direct for reliability

**Follow-up Questions**:
- "Which rail is best for my payment?"
- "Should I compare costs for my specific transfer?"
- "Do you need help choosing a payment method?"

---

## Workflows

### Standard Payment Execution
1. **Quote**: "Get quote for sending 100 XLM to GD...ADDRESS"
2. **Compare**: "Show me all payment rail options"
3. **Select**: "Use direct rail for this payment"
4. **Execute**: "Send 100 XLM to GD...ADDRESS"
5. **Confirm**: "Show payment confirmation and receipt"

### Private Payment Process
1. **Assess**: "I need to make a private payment"
2. **Configure**: "Set up ZK payment with full privacy"
3. **Execute**: "Send 100 XLM privately to GD...ADDRESS"
4. **Verify**: "Show privacy proof and confirmation"
5. **Save**: "Save privacy settings for future payments"

### Recurring Payment Setup
1. **Create**: "Generate payment request for monthly subscription"
2. **Configure**: "Set up recurring 50 USDC payments"
3. **Schedule**: "Start on the 1st of each month"
4. **Monitor**: "Track payment history and status"
5. **Manage**: "Modify or cancel recurring payments"

### Cross-Border Remittance
1. **Evaluate**: "Compare international payment options"
2. **Select**: "Use Tempo rail for EUR to USD conversion"
3. **Quote**: "Get quote for 1000 EUR to USD"
4. **Execute**: "Send 1000 EUR to US recipient"
5. **Track**: "Monitor international transfer status"

### Business Payment Management
1. **Invoice**: "Create invoice for 500 USDC"
2. **Request**: "Generate payment request with QR code"
3. **Receive**: "Monitor incoming payment status"
4. **Record**: "Add to accounting records"
5. **Report**: "Generate monthly payment report"

## Advanced Concepts

### Payment Rail Architecture
Understanding different payment protocols:
- Network layer protocols
- Routing mechanisms
- Fee structures
- Settlement processes
- Privacy implementations

### Cross-Rail Compatibility
- Rail switching capabilities
- Asset conversion
- Fee optimization
- Speed vs cost trade-offs
- Privacy vs transparency

### Payment Security
- Address validation
- Double-spending prevention
- Replay attack protection
- Network security measures
- Privacy protection methods

### Regulatory Compliance
- KYC/AML requirements
- Transaction monitoring
- Reporting obligations
- Privacy regulations
- Cross-border compliance

## Common Questions

**Q: Which payment rail should I use?**
A: Direct for reliability, x402 for micropayments, Tempo for international, ZK for privacy.

**Q: Are ZK payments really private?**
A: Yes, they use zero-knowledge proofs to hide transaction details while maintaining verifiability.

**Q: How fast are x402 payments?**
A: x402 offers real-time streaming payments, typically settling in 1-2 seconds.

**Q: Can I reverse payments?**
A: Most payments are irreversible on Stellar, but some rails offer dispute resolution.

**Q: What are the fees for different rails?**
A: Fees vary: Direct (~0.00001 XLM), x402 (~0.001 XLM), Tempo (~0.5%), ZK (~0.002 XLM).

## Tips

- Always compare rail costs before large payments
- Use x402 for frequent small payments
- Consider ZK for sensitive transactions
- Use Tempo for international transfers
- Save frequently used recipients
- Set up payment notifications
- Keep records for tax purposes
- Use memos for payment identification
- Monitor payment history for unusual activity
- Consider privacy implications of transactions
