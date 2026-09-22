# Skills Framework Index

## Overview
Comprehensive index of all Stellar Go CLI MCP skills, organized by category and capability, enabling AI assistants to quickly locate and utilize appropriate skills for user interactions.

## Skills Directory Structure

```
stellar-go-cli/internal/mcp/skills/
├── core/                    # Core operational skills
│   ├── wallet_operations.md
│   ├── swap_operations.md
│   ├── asset_management.md
│   ├── payment_operations.md
│   └── system_management.md
├── domain/                  # Domain-specific knowledge
│   ├── stellar_basics.md
│   ├── asset_standards.md
│   ├── payment_rails.md
│   ├── did_credentials.md
│   └── carbon_credits.md
├── conversation/            # Conversational patterns
│   ├── greetings.md
│   ├── help_responses.md
│   ├── error_handling.md
│   ├── follow_up_questions.md
│   └── contextual_responses.md
├── workflows/               # Multi-step workflows
│   ├── wallet_setup.md
│   ├── asset_creation.md
│   ├── arbitrage_detection.md
│   ├── payment_execution.md
│   └── reporting_compliance.md
└── templates/               # Response templates
    ├── success_responses.md
    ├── error_responses.md
    ├── progress_updates.md
    └── confirmation_requests.md
```

## Core Skills Summary

### Wallet Operations (12 skills)
**File**: `core/wallet_operations.md`

| Skill | MCP Tool | Description | Complexity |
|-------|----------|-------------|------------|
| wallet_connect | wallet_connect | Connect wallet via passkey/stellar/external | Medium |
| wallet_import | wallet_import | Import wallet from secret key or recovery phrase | High |
| wallet_fund | wallet_fund | Fund wallet from testnet faucet | Low |
| wallet_switch | wallet_switch | Switch active wallet | Low |
| wallet_rename | wallet_rename | Rename wallet for identification | Low |
| wallet_remove | wallet_remove | Remove wallet from registry | Medium |
| wallet_export | wallet_export | Export private key (high security) | High |
| wallet_passkey | wallet_passkey | Manage passkey authentication | Medium |
| wallet_list | wallet_list | List all connected wallets | Low |
| wallet_show | wallet_show | Show active wallet details | Low |
| wallet_balance | wallet_balance | Get wallet balances | Low |
| wallet_assets | wallet_assets | List trusted assets | Low |

### Swap Operations (8 skills)
**File**: `core/swap_operations.md`

| Skill | MCP Tool | Description | Complexity |
|-------|----------|-------------|------------|
| swap_quote | swap_quote | Get swap quote for asset conversion | Medium |
| swap_execute | swap_execute | Execute swap transaction | High |
| swap_scan | swap_scan | Scan for profitable swap opportunities | Medium |
| swap_monitor | swap_monitor | Monitor swap opportunities in real-time | High |
| swap_arbitrage_all | swap_arbitrage_all | Comprehensive arbitrage scan | High |
| swap_triangular | swap_triangular | Triangular arbitrage operations | High |
| swap_arbitrage_scan | swap_arbitrage_scan | Scan for arbitrage opportunities | Medium |
| swap_assets | swap_assets | List available swap assets | Low |

### Asset Management (8 skills)
**File**: `core/asset_management.md`

| Skill | MCP Tool | Description | Complexity |
|-------|----------|-------------|------------|
| asset_create_ft | asset_create_ft | Create fungible token (SAC/SEP-41) | High |
| asset_create_nfa | asset_create_nfa | Create non-fungible asset (NFA) | High |
| asset_trust | asset_trust | Establish trustline for asset | Medium |
| asset_info | asset_info | Get detailed asset information | Low |
| asset_score | asset_score | Get asset quality and risk score | Medium |
| asset_carbon | asset_carbon | Attach carbon credits to asset | Medium |
| asset_show | asset_show | Show comprehensive asset details | Low |
| asset_list | asset_list | List all known assets | Low |

### Payment Operations (7 skills)
**File**: `core/payment_operations.md`

| Skill | MCP Tool | Description | Complexity |
|-------|----------|-------------|------------|
| pay_send | pay_send | Send payment across multiple rails | High |
| pay_quote | pay_quote | Get payment quote across rails | Medium |
| pay_request | pay_request | Generate payment request/QR code | Medium |
| pay_history | pay_history | Get payment history | Low |
| pay_x402 | pay_x402 | Execute x402 micropayment | Medium |
| pay_zk | pay_zk | Execute zero-knowledge payment | High |
| pay_rails | pay_rails | Show payment rail information | Low |

### System Management (6 skills)
**File**: `core/system_management.md`

| Skill | MCP Tool | Description | Complexity |
|-------|----------|-------------|------------|
| system_status | system_status | Get system status and version | Low |
| system_network | system_network | Get/set network configuration | Medium |
| system_health | system_health | Check external service health | Medium |
| system_init | system_init | Initialize system configuration | High |
| system_version | system_version | Show version and build info | Low |
| system_flow | system_flow | Manage background workflows | Medium |

## Domain Knowledge Summary

### Stellar Basics
**File**: `domain/stellar_basics.md`

**Topics Covered:**
- Network architecture and consensus
- XLM native asset and economics
- Account structure and addresses
- Transaction types and operations
- Asset standards and creation
- Decentralized exchange (DEX)
- Security features and best practices
- Fees and economic incentives

### Asset Standards
**File**: `domain/asset_standards.md` *[To be created]*

**Topics Covered:**
- SEP-41 Stellar Asset Contract
- Fungible vs Non-Fungible Assets
- Token metadata standards
- Trustline requirements
- Asset verification processes

### Payment Rails
**File**: `domain/payment_rails.md` *[To be created]*

**Topics Covered:**
- Direct Stellar payments
- x402 micropayment protocol
- Tempo FX remittance
- Zero-knowledge payments
- Cross-rail compatibility

### DID Credentials
**File**: `domain/did_credentials.md` *[To be created]*

**Topics Covered:**
- DID methods (web/key/ethr/ebsi)
- Verifiable Credentials
- Attestation processes
- Identity verification

### Carbon Credits
**File**: `domain/carbon_credits.md` *[To be created]*

**Topics Covered:**
- StellarCarbon protocol
- Credit verification
- On-chain recording
- ESG compliance

## Conversation Patterns Summary

### Greetings
**File**: `conversation/greetings.md` *[To be created]*

**Patterns:**
- Initial greetings with capability overview
- Contextual greetings for returning users
- Time-based greetings
- Personalized welcome messages

### Help Responses
**File**: `conversation/help_responses.md` *[To be created]*

**Patterns:**
- General capability overview
- Feature-specific help
- Step-by-step guidance
- Educational explanations

### Error Handling
**File**: `conversation/error_handling.md` *[To be created]*

**Patterns:**
- Parameter error responses
- Network error handling
- Balance/insufficient funds errors
- Asset/trustline error handling

### Follow-up Questions
**File**: `conversation/follow_up_questions.md` *[To be created]*

**Patterns:**
- Post-operation suggestions
- Information request follow-ups
- Error recovery options
- Contextual next steps

### Contextual Responses
**File**: `conversation/contextual_responses.md` *[To be created]*

**Patterns:**
- Multi-turn conversation support
- Personalized responses
- Memory-aware interactions
- Adaptive communication

## Workflows Summary

### Wallet Setup
**File**: `workflows/wallet_setup.md`

**Steps:**
1. Initial assessment and recommendations
2. Wallet creation (passkey/stellar/external)
3. Security configuration
4. Initial funding
5. Verification and testing
6. Configuration and preferences

### Asset Creation
**File**: `workflows/asset_creation.md` *[To be created]*

**Steps:**
1. Asset design and planning
2. Token creation (FT/NFA)
3. Metadata configuration
4. Carbon credit integration
5. Distribution setup
6. Market preparation

### Arbitrage Detection
**File**: `workflows/arbitrage_detection.md` *[To be created]*

**Steps:**
1. Market scanning setup
2. Opportunity identification
3. Risk assessment
4. Execution planning
5. Monitoring and adjustment
6. Performance analysis

### Payment Execution
**File**: `workflows/payment_execution.md` *[To be created]*

**Steps:**
1. Payment method selection
2. Quote comparison
3. Recipient verification
4. Transaction execution
5. Confirmation tracking
6. Record keeping

### Reporting Compliance
**File**: `workflows/reporting_compliance.md` *[To be created]*

**Steps:**
1. Transaction data collection
2. Compliance verification
3. Report generation
4. Audit trail creation
5. Regulatory submission
6. Record maintenance

## Response Templates Summary

### Success Responses
**File**: `templates/success_responses.md`

**Categories:**
- Transaction success (payments, swaps, asset creation)
- Information display (balance, status, asset info)
- Configuration success (network, security)
- Monitoring and alerts
- Workflow completion
- Educational achievements
- Personalization updates

### Error Responses
**File**: `templates/error_responses.md` *[To be created]*

**Categories:**
- Parameter errors
- Network errors
- Balance/insufficient funds
- Asset/trustline issues
- Security violations
- System failures

### Progress Updates
**File**: `templates/progress_updates.md` *[To be created]*

**Categories:**
- Long-running operations
- Background processes
- Multi-step workflows
- Batch operations
- Learning progress

### Confirmation Requests
**File**: `templates/confirmation_requests.md` *[To be created]*

**Categories:**
- High-risk operations
- Irreversible actions
- Security-sensitive operations
- Network changes
- Large transactions

## Skill Integration Guide

### Natural Language Processing
1. **Intent Recognition**: Map user input to appropriate skill
2. **Parameter Extraction**: Extract required parameters from natural language
3. **Context Management**: Maintain conversation context across interactions
4. **Error Recovery**: Handle missing/invalid parameters gracefully

### Skill Selection Logic
```go
func SelectSkill(userInput string, context ConversationContext) Skill {
    // 1. Analyze intent
    intent := AnalyzeIntent(userInput)
    
    // 2. Consider context
    if context.ActiveWallet == "" && intent.IsWalletOperation() {
        return GetSkill("wallet_setup")
    }
    
    // 3. Check prerequisites
    if intent.RequiresActiveWallet() && context.ActiveWallet == "" {
        return GetSkill("wallet_connect")
    }
    
    // 4. Select appropriate skill
    return GetSkill(intent.PrimarySkill)
}
```

### Response Generation
```go
func GenerateResponse(skill Skill, result interface{}, context ConversationContext) string {
    // 1. Select template
    template := SelectTemplate(skill.Type, result.Status)
    
    // 2. Fill variables
    response := FillTemplate(template, result, context)
    
    // 3. Add contextual suggestions
    response += GenerateSuggestions(skill, context)
    
    // 4. Include educational content
    if context.UserExperience == "beginner" {
        response += GenerateEducationalContent(skill)
    }
    
    return response
}
```

## Usage Examples

### Basic Interaction
```
User: "show my balance"
→ Intent: wallet_balance
→ Skill: wallet_balance (core/wallet_operations.md)
→ Response: Balance information template
→ Follow-up: Suggest funding if low
```

### Complex Workflow
```
User: "I want to create a new token"
→ Intent: asset_create_ft
→ Skill: asset_creation_workflow (workflows/asset_creation.md)
→ Multi-step: Design → Create → Configure → Distribute
→ Templates: Progress updates at each step
→ Final: Success response with next steps
```

### Error Recovery
```
User: "send 1000 USDC" (insufficient balance)
→ Error: Insufficient funds
→ Template: Error response (templates/error_responses.md)
→ Recovery: Suggest funding, swap, or reduce amount
→ Education: Explain balance requirements
```

## Implementation Status

### ✅ Completed
- **Core Skills**: All 41 core operational skills documented
- **Domain Knowledge**: Stellar basics completed
- **Conversation Patterns**: Comprehensive patterns documented
- **Workflows**: Wallet setup workflow completed
- **Templates**: Success response templates completed

### 🔄 In Progress
- **Domain Knowledge**: Asset standards, payment rails, DID, carbon credits
- **Conversation Patterns**: Individual pattern files
- **Workflows**: Asset creation, arbitrage, payment, reporting
- **Templates**: Error, progress, confirmation templates

### 📋 Planned
- **Integration Layer**: Skill selection and response generation
- **Learning System**: User preference adaptation
- **Analytics**: Skill usage and effectiveness tracking
- **Multi-language**: Internationalization support

## Best Practices

### Skill Design
- **Clear Descriptions**: Use simple, actionable language
- **Parameter Validation**: Comprehensive input validation
- **Error Handling**: Graceful failure with helpful recovery
- **Context Awareness**: Consider user experience and history
- **Educational Value**: Include learning opportunities

### Template Design
- **Dynamic Variables**: Use consistent variable naming
- **Context Adaptation**: Adjust based on user context
- **Tone Consistency**: Maintain helpful, professional tone
- **Action Guidance**: Provide clear next steps
- **Multi-format Support**: Text, voice, visual compatibility

### Workflow Design
- **Step-by-Step**: Clear progression with checkpoints
- **Flexibility**: Allow user control and customization
- **Recovery Options**: Handle failures gracefully
- **Educational Integration**: Learning opportunities throughout
- **Completion Metrics**: Clear success indicators

This skills framework provides the foundation for intelligent, context-aware natural language interactions with the Stellar Go CLI platform.
