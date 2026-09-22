# MozartPay MCP Assistant

An interactive command-line assistant that provides a user-friendly interface to the MozartPay MCP (Model Context Protocol) server.

## Features

- 🤖 Interactive chat interface with natural language commands
- 📦 Access to all 17 MCP tools (wallet, swap, asset, payment, system)
- 💱 Smart command parsing and argument handling
- 🎨 Formatted output with emojis and readable responses
- 🛡️ Error handling and graceful recovery
- 📝 Command history and tab completion

## Quick Start

### Option 1: Use the shell wrapper (recommended)
```bash
./mcp_assistant.sh
```

### Option 2: Run directly with Python
```bash
python3 mcp_assistant.py
```

### Option 3: Build and run manually
```bash
cd mozartpay
go build ./cmd/stellar-go-cli
cd ..
python3 mcp_assistant.py
```

## Available Commands

### System Commands
- `status` - Check system status
- `health` - Check system health
- `help` - Show available commands
- `tools` - List all MCP tools
- `quit` or `exit` - Exit assistant

### Wallet Commands
- `wallet list` - List all wallets
- `wallet show` - Show active wallet details
- `wallet balance` - Get wallet balance
- `wallet assets` - List wallet assets

### Swap Commands
- `swap quote XLM USDC 100` - Get swap quote
- `swap assets` - Show available swap assets
- `swap arbitrage` - Scan for arbitrage opportunities

### Asset Commands
- `asset list` - List available assets
- `asset info USDC` - Get asset information

### Payment Commands
- `pay send <address> <asset> <amount>` - Send payment
- `pay history` - Show payment history

### Direct Tool Calls
You can also call MCP tools directly:
```bash
system_status
wallet_list
swap_quote from=XLM to=USDC amount=100
```

## Examples

```bash
🤖 MozartPay MCP Assistant Ready!
Type 'help' for commands or 'quit' to exit

🤔 > status
📊 **System Status:**
**version:** 0.1.0-mvp
**network:** stellar-testnet
**active_wallet:** 
**wallet_connected:** false
**horizon_connected:** true

🤔 > wallet list
💼 **Wallet List:**
**count:** 0
**wallets:** []

🤔 > swap quote XLM USDC 100
💱 **Swap Quote:**
**from:** XLM
**to:** USDC
**amount:** 100
**type:** strict-send
**rate:** 0.118
**estimated:** 100 USDC

🤔 > quit
👋 Goodbye!
```

## Architecture

The assistant consists of:

1. **MCP Server** (`./mozartpay mcp`) - JSON-RPC 2.0 server exposing CLI tools
2. **Python Assistant** (`mcp_assistant.py`) - Interactive client with command parsing
3. **Shell Wrapper** (`mcp_assistant.sh`) - Easy launcher with auto-build

## MCP Protocol

The assistant communicates with the MozartPay MCP server using:
- **Transport:** stdio (stdin/stdout)
- **Protocol:** JSON-RPC 2.0
- **Version:** MCP 2024-11-05
- **Capabilities:** tools, logging

## Error Handling

- Connection failures are detected and reported
- Invalid commands show helpful error messages
- Server crashes trigger automatic reconnection
- Graceful shutdown on Ctrl+C

## Development

To extend the assistant:

1. Add new command mappings in `process_command()`
2. Update help text in `show_help()`
3. Add completion options in `_completer()`
4. Test with the provided test scripts

## Troubleshooting

**"mozartpay binary not found"**
- Run `go build ./cmd/stellar-go-cli` in the mozartpay directory

**"Failed to initialize MCP server"**
- Check if mozartpay binary exists and is executable
- Verify you're in the correct directory

**"Lost connection to MCP server"**
- The server may have crashed, restart the assistant
- Check for error messages in the output

## Integration with AI Assistants

The MCP server can be integrated with AI assistants like Claude, ChatGPT, etc.:

```json
{
  "mcpServers": {
    "mozartpay": {
      "command": "./mozartpay/mozartpay",
      "args": ["mcp"]
    }
  }
}
```

This exposes all MozartPay CLI functionality as tools to the AI assistant.
