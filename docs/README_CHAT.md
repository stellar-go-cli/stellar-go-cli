# Stellar Go CLI Chat Interface

A conversational AI interface for the Stellar Go CLI that uses natural language processing to understand user commands and execute them via the MCP (Model Context Protocol) server.

## Features

- 🤖 **Natural Language Processing** - Understands conversational commands
- 💬 **Interactive Chat** - Real-time conversation interface
- 🎯 **Intent Recognition** - Automatically determines what you want to do
- 📊 **Context Awareness** - Maintains conversation context
- 🛡️ **Error Handling** - Graceful handling of misunderstandings
- 🎨 **Rich Responses** - Formatted, helpful responses

## Quick Start

```bash
# Start the chat interface
./chat.sh

# Or run directly
python3 stellar_go_cli_chat.py
```

## Natural Language Commands

### Wallet Management
- "show my wallet" - Display active wallet details
- "what's my balance" - Check wallet balance
- "show my assets" - List wallet assets
- "list all wallets" - Show all registered wallets

### Asset Swaps
- "swap 100 XLM to USDC" - Get swap quote
- "exchange 50 XLM for USDC" - Show exchange options
- "trade XLM to USDC" - Get trading information
- "show swap options" - List available swap pairs

### Payments & Transfers
- "send 100 USDC to GD..." - Send payment to address
- "pay 50 XLM to ADDRESS" - Make a payment
- "transfer 25 USDC to GD..." - Transfer funds

### System Information
- "system status" - Check system health
- "how is the system" - System health check
- "show system info" - Display system details

### General Commands
- "help" - Show available commands
- "what can you do" - Display capabilities
- "hello" / "hi" - Greeting
- "quit" / "exit" - Leave chat

## Example Conversations

```
💬 You: hello
🤖 Assistant: 👋 Hello! I'm your Stellar Go CLI assistant. I can help you with:
• Wallet management (list, balance, assets)
• Asset swaps and quotes
• Payments and transfers
• System status and health
• Asset information

Try asking: 'show my wallet balance' or 'swap 100 XLM to USDC'

💬 You: what's my balance?
🤖 Assistant: **Wallet Balance:**
**address:** GD...
**network:** stellar-testnet
**type:** stellar
**balance:** 1000.0000000 XLM
**funded:** true

💬 You: swap 100 XLM to USDC
🤖 Assistant: **Swap Quote:**
**from:** XLM
**to:** USDC
**amount:** 100
**rate:** 0.118
**estimated:** 11.8 USDC
**price_impact:** 0.1%

💬 You: send 10 USDC to GD...1234
🤖 Assistant: **Payment Simulation:**
**dry_run:** true
**destination:** GD...1234
**asset:** USDC
**amount:** 10
**from:** GD...5678
**note:** Use execute=true to send payment

💬 You: system status
🤖 Assistant: **System Status:**
**version:** 0.1.0-mvp
**network:** stellar-testnet
**active_wallet:** GD...5678
**wallet_connected:** true
**horizon_connected:** true
```

## Architecture

The chat interface consists of:

1. **Intent Parser** - Natural language understanding module
2. **MCP Client** - Communicates with Stellar Go CLI MCP server
3. **Response Formatter** - Formats responses for readability
4. **Context Manager** - Maintains conversation history

### Intent Recognition

The system uses pattern matching and keyword detection to understand:

- **Wallet Operations**: balance, show, list, assets
- **Swap Operations**: swap, exchange, trade, XLM→USDC
- **Payment Operations**: send, pay, transfer, address
- **System Operations**: status, health, system
- **Conversational**: hello, help, goodbye

### MCP Integration

All commands are executed through the MCP server:
- JSON-RPC 2.0 protocol over stdio
- 17 available tools (wallet, swap, asset, payment, system)
- Structured responses with error handling

## Advanced Features

### Parameter Extraction
The chat interface automatically extracts parameters from natural language:

```python
# "swap 100 XLM to USDC" → {'from': 'XLM', 'to': 'USDC', 'amount': '100'}
# "send 50 USDC to GD..." → {'destination': 'GD...', 'asset': 'USDC', 'amount': '50'}
```

### Context Awareness
Maintains conversation history for better understanding:
```python
context = [
    {'role': 'user', 'message': 'show my balance', 'timestamp': '...'},
    {'role': 'assistant', 'message': 'Your balance is...', 'timestamp': '...'}
]
```

### Error Recovery
Handles misunderstandings gracefully:
- Suggests corrections for unknown commands
- Provides help when intent is unclear
- Recovers from MCP server errors

## Customization

### Adding New Intents
To add support for new commands:

1. Update `understand_intent()` method
2. Add intent to response formatter
3. Update help text
4. Add test cases

```python
# Example: Add support for "create wallet"
elif 'create' in message_lower and 'wallet' in message_lower:
    return {'intent': 'wallet_create', 'params': {}}
```

### Extending Responses
Customize response formatting in `format_response()`:

```python
elif intent == 'wallet_create':
    return "🔐 Creating new wallet...\n" + response
```

## Troubleshooting

**"stellar-go-cli binary not found"**
- Run `go build ./cmd/stellar-go-cli` first

**"Failed to initialize MCP server"**
- Check if stellar-go-cli is running properly
- Verify binary permissions

**"I don't understand" responses**
- Try rephrasing your command
- Type "help" to see available commands
- Use more specific language

## Integration

The chat interface can be integrated with:

1. **Terminal Emulators** - Direct terminal usage
2. **Web Interfaces** - Via WebSocket bridge
3. **AI Assistants** - As a tool provider
4. **Mobile Apps** - Through HTTP API wrapper

## Future Enhancements

- 🧠 Machine learning for better intent recognition
- 📝 Command history and favorites
- 🔔 Notifications for transactions
- 🌐 Multi-language support
- 📊 Transaction analytics
- 🔐 Enhanced security features

## Comparison with CLI

| Feature | Traditional CLI | Chat Interface |
|---------|-----------------|----------------|
| Learning Curve | Steep (need to know commands) | Gentle (natural language) |
| Discovery | `--help` flags | Conversational help |
| Error Messages | Technical | User-friendly |
| Context | None | Maintained |
| Accessibility | Command-line knowledge | Natural language |

The chat interface makes Stellar Go CLI accessible to users who prefer conversational interactions over traditional CLI commands.
