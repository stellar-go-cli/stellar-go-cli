# Memory Skills

## Overview
Memory management capabilities for the MozartPay MCP server, enabling AI assistants to remember user preferences, conversation context, and operational history across sessions.

## Skills

### memory_save
**Description**: Save information to the conversation memory for future reference
**Category**: memory_operations
**MCP Tool**: memory_save

**Parameters**:
- `key` (string, required): Unique identifier for the memory entry
- `value` (any, required): Data to store (can be any JSON-serializable value)
- `category` (string): Memory category for organization (default: "general")
- `ttl` (number): Time-to-live in seconds (optional, for temporary memories)
- `scope` (enum): Memory scope - "session" | "user" | "global" (default: "session")

**Natural Language Patterns**:
- "remember that I prefer XLM for payments"
- "save this wallet address for later"
- "remember my favorite swap pairs"
- "store my preferred network as testnet"
- "memorize my frequently used assets"

**Examples**:
```bash
# Save preference
"remember that I prefer fast swaps over best rates"

# Store address
"save this address as my main wallet: GD5DQYPNQ..."

# Remember settings
"remember my default slippage is 2%"

# Store context
"save that I'm working on arbitrage strategies"
```

**Response Format**:
```json
{
  "status": "saved",
  "key": "user_preference_payment_asset",
  "category": "preferences",
  "expires_at": "2024-01-01T12:00:00Z"
}
```

---

### memory_get
**Description**: Retrieve information from the conversation memory
**Category**: memory_operations
**MCP Tool**: memory_get

**Parameters**:
- `key` (string, required): Key of the memory entry to retrieve
- `category` (string): Filter by memory category (optional)

**Natural Language Patterns**:
- "what did I say about my preferences?"
- "recall my main wallet address"
- "what was my default slippage?"
- "remember what I told you about swaps?"
- "what did we discuss about assets?"

**Examples**:
```bash
# Retrieve specific memory
"what's my main wallet address?"

# Get by category
"show me my saved preferences"

# Recall context
"what was I working on last time?"
```

**Response Format**:
```json
{
  "key": "user_preference_payment_asset",
  "value": "XLM",
  "category": "preferences",
  "saved_at": "2024-01-01T10:00:00Z",
  "expires_at": null
}
```

---

### memory_list
**Description**: List all memories or filter by category
**Category**: memory_operations
**MCP Tool**: memory_list

**Parameters**:
- `category` (string): Filter by specific category (optional)
- `scope` (enum): Filter by scope - "session" | "user" | "global" (optional)
- `include_expired` (boolean): Include expired memories (default: false)

**Natural Language Patterns**:
- "show me what you remember"
- "list my saved preferences"
- "what memories do we have?"
- "show my conversation history"
- "display stored information"

**Examples**:
```bash
# List all memories
"what do you remember about me?"

# Filter by category
"show my wallet-related memories"

# Check session history
"what have we discussed today?"
```

**Response Format**:
```json
{
  "memories": [
    {
      "key": "wallet_main",
      "category": "wallets",
      "value_preview": "GD5DQYPNQ...",
      "saved_at": "2024-01-01T10:00:00Z"
    },
    {
      "key": "preference_swap_speed",
      "category": "preferences",
      "value_preview": "fast",
      "saved_at": "2024-01-01T11:00:00Z"
    }
  ],
  "total": 2,
  "categories": ["wallets", "preferences"]
}
```

---

### memory_delete
**Description**: Delete a specific memory entry
**Category**: memory_operations
**MCP Tool**: memory_delete

**Parameters**:
- `key` (string, required): Key of the memory entry to delete
- `confirm` (boolean): Confirm deletion (default: false)

**Natural Language Patterns**:
- "forget my old wallet address"
- "delete that preference"
- "remove my stored slippage setting"
- "clear memory of X"
- "erase my saved asset list"

**Examples**:
```bash
# Delete specific memory
"forget my old main wallet"

# With confirmation
"delete my preference for XLM - confirmed"
```

**Response Format**:
```json
{
  "status": "deleted",
  "key": "wallet_old",
  "deleted_at": "2024-01-01T12:00:00Z"
}
```

---

### memory_clear
**Description**: Clear all memories within a scope or category
**Category**: memory_operations
**MCP Tool**: memory_clear

**Parameters**:
- `scope` (enum): Clear scope - "session" | "user" | "category" (default: "session")
- `category` (string): Specific category to clear (if scope is "category")
- `confirm` (boolean): Confirm clearing (required for user/global scope)

**Natural Language Patterns**:
- "clear all my memories"
- "forget everything from this session"
- "reset my preferences"
- "clear all wallet information"
- "start fresh - erase everything"

**Examples**:
```bash
# Clear session
"clear this conversation's memory"

# Clear category
"forget all my swap preferences"

# Full reset with confirmation
"clear all my user data - I confirm"
```

**Response Format**:
```json
{
  "status": "cleared",
  "scope": "session",
  "cleared_count": 5,
  "categories_affected": ["preferences", "wallets"]
}
```

---

### memory_analyze
**Description**: Analyze memory patterns and provide insights
**Category**: memory_operations
**MCP Tool**: memory_analyze

**Parameters**:
- `analysis_type` (enum): Type of analysis - "patterns" | "preferences" | "frequency" (default: "patterns")
- `time_range` (string): Time range to analyze - "session" | "today" | "week" | "all" (default: "session")
- `category` (string): Limit analysis to specific category (optional)

**Natural Language Patterns**:
- "analyze my usage patterns"
- "what do I do most frequently?"
- "show my preferences analysis"
- "what are my common behaviors?"
- "identify my trading patterns"

**Examples**:
```bash
# Pattern analysis
"what patterns do you see in my behavior?"

# Preference insights
"analyze my asset preferences"

# Frequency analysis
"what operations do I perform most?"
```

**Response Format**:
```json
{
  "analysis_type": "patterns",
  "time_range": "today",
  "insights": {
    "most_used_operations": ["swap_quote", "wallet_balance"],
    "preferred_assets": ["XLM", "USDC"],
    "common_patterns": [
      "User frequently checks balance before swaps",
      "User prefers fast execution over best rates"
    ],
    "recommendations": [
      "Consider setting up automated balance alerts",
      "Explore arbitrage opportunities with XLM/USDC"
    ]
  }
}
```

---

### memory_search
**Description**: Search memories by content or keywords
**Category**: memory_operations
**MCP Tool**: memory_search

**Parameters**:
- `query` (string, required): Search query string
- `category` (string): Limit search to category (optional)
- `fuzzy` (boolean): Enable fuzzy matching (default: true)

**Natural Language Patterns**:
- "search for my wallet information"
- "find memories about swaps"
- "look up my asset preferences"
- "search for XLM related info"
- "find my payment settings"

**Examples**:
```bash
# Simple search
"search for wallet addresses"

# Category-specific search
"find my swap-related memories"

# Fuzzy search
"look up anything about payments"
```

**Response Format**:
```json
{
  "query": "wallet",
  "results": [
    {
      "key": "wallet_main",
      "relevance": 1.0,
      "preview": "GD5DQYPNQ...",
      "category": "wallets"
    },
    {
      "key": "wallet_backup",
      "relevance": 0.8,
      "preview": "GABC123...",
      "category": "wallets"
    }
  ],
  "total_results": 2
}
```

---

## Memory Categories

### Standard Categories
- **preferences**: User preferences and settings
- **wallets**: Wallet addresses and information
- **assets**: Asset-related memories
- **operations**: Recent operations and transactions
- **patterns**: Behavioral patterns and habits
- **context**: Conversation context and history
- **session**: Temporary session-specific data

### Custom Categories
Users can define custom categories for organization:
- Trading strategies
- Research notes
- Contact addresses
- Configuration presets

## Memory Scopes

### Session Scope
- **Lifetime**: Current conversation only
- **Use Case**: Temporary context, working notes
- **Example**: Current operation parameters, temporary addresses

### User Scope
- **Lifetime**: Persistent across sessions
- **Use Case**: Long-term preferences, recurring settings
- **Example**: Preferred assets, default slippage, main wallet

### Global Scope
- **Lifetime**: Shared across all users (system-wide)
- **Use Case**: Common knowledge, market data, best practices
- **Example**: Asset information, network status, common addresses

## Workflows

### Preference Learning Workflow
1. **Observe**: Track user choices during operations
2. **Extract**: Identify patterns in user behavior
3. **Store**: Save preferences to memory
4. **Apply**: Use preferences in future interactions
5. **Refine**: Update preferences based on feedback

**Example**:
```
User: "swap 100 XLM to USDC"
[Assistant observes user chose specific slippage]
Assistant: "I notice you prefer 1% slippage. Should I remember this?"
User: "yes"
[Preference saved to memory]

Later...
User: "swap XLM to USDC"
Assistant: [Automatically applies 1% slippage based on memory]
```

### Context Restoration Workflow
1. **Detect**: Recognize returning user/session
2. **Retrieve**: Load relevant memories
3. **Restore**: Re-establish conversation context
4. **Confirm**: Verify context is still valid
5. **Continue**: Resume from previous state

**Example**:
```
User: "continue from where we left off"
Assistant: "Last time you were setting up arbitrage monitoring for XLM/USDC pairs. 
            Should I restore that configuration?"
```

### Memory Cleanup Workflow
1. **Identify**: Find expired or obsolete memories
2. **Review**: Present list to user for confirmation
3. **Clean**: Remove confirmed memories
4. **Optimize**: Archive important but old memories
5. **Update**: Refresh time-sensitive information

## Best Practices

### For Users
- **Regular Review**: Periodically check stored memories
- **Categorization**: Use categories for better organization
- **Expiration**: Set TTL for temporary information
- **Privacy**: Be mindful of what you store (especially addresses)
- **Cleanup**: Remove obsolete memories regularly

### For AI Assistants
- **Proactive Storage**: Save useful context automatically
- **Confirmation**: Confirm before storing sensitive data
- **Relevance**: Only store information that aids future interactions
- **Privacy**: Respect user privacy and data boundaries
- **Helpfulness**: Use memories to improve user experience

## Integration with Other Skills

### Wallet Operations
- Remember user's wallets and their purposes
- Store frequently used external addresses
- Track preferred funding sources

### Swap Operations
- Store preferred slippage settings
- Remember favorite swap pairs
- Save arbitrage preferences

### Payment Operations
- Remember frequent recipients
- Store preferred payment rails
- Save memo templates

### Asset Management
- Remember assets of interest
- Store token creation preferences
- Track asset scoring criteria

## Common Questions

**Q: How long do memories last?**
A: Depends on scope:
- Session: Until conversation ends
- User: Persistent until explicitly deleted
- Global: System-defined lifetime

**Q: Can I export my memories?**
A: Use `memory_list` and save the output, or future export functionality.

**Q: Is my memory data secure?**
A: Memory is local to your session. Sensitive data should be handled carefully.

**Q: Can I share memories between devices?**
A: Currently memories are local. Future versions may support sync.

**Q: What if I forget what I saved?**
A: Use `memory_list` or `memory_search` to find saved information.

## Tips

- **Name clearly**: Use descriptive keys for easy recall
- **Categorize**: Organize memories by category
- **Set expiration**: Use TTL for temporary information
- **Review regularly**: Clean up obsolete memories
- **Use search**: Find memories quickly with search
- **Analyze patterns**: Use analysis to understand your behavior
- **Export important**: Backup critical memories

## Examples in Action

### Smart Preference Application
```
User: "send payment"
Assistant: "I'll help you send a payment! I see from your preferences 
            that you typically send XLM using the direct rail to GD5DQY...
            
            Would you like to:
            1. Use your usual settings (100 XLM to GD5DQY...)
            2. Modify the details"
```

### Context-Aware Assistance
```
User: "check my balance"
Assistant: "Your main wallet (GD5DQY...) has 35.07 XLM.
            
            I notice you usually check balance before swaps. 
            Would you like me to check swap opportunities too?"
```

### Pattern Recognition
```
User: "what should I do today?"
Assistant: "Based on your patterns, you typically:
            - Check balance first thing
            - Look for XLM/USDC arbitrage
            - Review asset performance
            
            Would you like me to run your morning routine?"
```

## Memory Analytics

### Usage Metrics
- Number of memories stored
- Most frequently accessed memories
- Memory categories breakdown
- Average memory lifetime

### Pattern Detection
- Preferred operation times
- Common asset pairs
- Typical transaction amounts
- Favorite features

### Recommendation Engine
- Suggest optimizations based on patterns
- Recommend new features
- Identify learning opportunities
- Propose automation workflows

This memory system transforms MozartPay from a transactional tool into a personalized assistant that truly understands and remembers your needs.
