# Wallet Operations Skills

## Overview
Comprehensive wallet management capabilities for the MozartPay platform, including connection, import, funding, and maintenance of wallets across different providers and networks.

## Skills

### wallet_connect
**Description**: Connect wallet via passkey, stellar keypair, or external address provider
**Category**: wallet_operations
**MCP Tool**: wallet_connect

**Parameters**:
- `provider` (enum): wwwallet | stellar | external (default: wwwallet)
- `network` (enum): stellar-testnet | stellar-mainnet | evm-sepolia | evm-mainnet (default: stellar-testnet)
- `address` (string): External address for external provider (optional)

**Natural Language Patterns**:
- "connect my wallet with passkey"
- "import stellar wallet on testnet"
- "add external wallet address"
- "set up wwwallet connection"
- "connect wallet on mainnet"

**Examples**:
```bash
# Passkey connection
"connect my wallet using passkey"

# Stellar import
"import my stellar wallet to testnet"

# External address
"add external wallet GD...ADDRESS"
```

**Follow-up Questions**:
- "Would you like to set this as your active wallet?"
- "Do you need to fund this wallet from the faucet?"
- "Should I generate a backup of your wallet keys?"

**Error Handling**:
- Missing provider: "I need to know which wallet provider you'd like to use (passkey, stellar, or external)"
- Network issues: "Let me check the network connection and try again"
- Invalid address: "That address doesn't look valid. Can you double-check it?"

---

### wallet_import
**Description**: Import wallet from secret key or recovery phrase
**Category**: wallet_operations
**MCP Tool**: wallet_import

**Parameters**:
- `secret` (string, required): Secret key or recovery phrase
- `name` (string): Wallet name for easy identification (optional)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-testnet)

**Natural Language Patterns**:
- "import wallet with secret key"
- "restore wallet from recovery phrase"
- "add wallet using private key"
- "import my backup wallet"
- "restore from seed phrase"

**Examples**:
```bash
# Secret key import
"import wallet with secret S..."

# Recovery phrase
"restore wallet from phrase: twelve words here"

# Named wallet
"import wallet named 'My Trading Wallet'"
```

**Security Notes**:
- Always confirm before handling secret keys
- Warn about security implications
- Suggest secure storage after import

**Follow-up Questions**:
- "Would you like me to back up this wallet securely?"
- "Should I set this as your active wallet?"
- "Do you want to fund this wallet from the testnet faucet?"

---

### wallet_fund
**Description**: Fund wallet from testnet faucet (only works on testnet)
**Category**: wallet_operations
**MCP Tool**: wallet_fund

**Parameters**:
- `address` (string): Wallet address to fund (optional, uses active if omitted)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-testnet)

**Natural Language Patterns**:
- "fund my wallet from faucet"
- "get testnet XLM"
- "add funds to my wallet"
- "fund wallet with testnet lumens"
- "get free XLM from faucet"

**Examples**:
```bash
# Fund active wallet
"fund my wallet from the testnet faucet"

# Fund specific wallet
"fund wallet GD...ADDRESS with testnet XLM"
```

**Limitations**:
- Only available on testnet
- Rate limited by faucet
- Requires wallet to be created first

**Error Handling**:
- Mainnet attempt: "Funding is only available on testnet. Would you like to switch to testnet?"
- Rate limit: "The faucet has a rate limit. Please try again in a few minutes."
- Invalid address: "That wallet address doesn't appear to be valid."

---

### wallet_switch
**Description**: Switch to a different wallet as the active wallet
**Category**: wallet_operations
**MCP Tool**: wallet_switch

**Parameters**:
- `address` (string, required): Wallet address to switch to

**Natural Language Patterns**:
- "switch to my other wallet"
- "change active wallet"
- "use wallet GD...ADDRESS"
- "set wallet as active"
- "switch to trading wallet"

**Examples**:
```bash
# Switch by address
"switch to wallet GD...ADDRESS"

# Switch by name (if available)
"switch to my trading wallet"
```

**Context Awareness**:
- Remember previous active wallet
- Show wallet details before switching
- Confirm the switch action

**Follow-up Questions**:
- "Would you like to see the balance of this wallet?"
- "Do you want to perform any operations with this wallet?"

---

### wallet_rename
**Description**: Rename an existing wallet for easier identification
**Category**: wallet_operations
**MCP Tool**: wallet_rename

**Parameters**:
- `address` (string, required): Wallet address to rename
- `name` (string, required): New wallet name

**Natural Language Patterns**:
- "rename my wallet to 'Trading'"
- "change wallet name"
- "call this wallet 'Main Account'"
- "rename wallet GD...ADDRESS"
- "set wallet nickname"

**Examples**:
```bash
# Rename with name
"rename my wallet to 'Day Trading'"

# Rename specific wallet
"rename wallet GD...ADDRESS to 'Savings'"
```

**Validation**:
- Check if wallet exists
- Validate name format
- Prevent duplicate names

**Follow-up Questions**:
- "Would you like to see all your renamed wallets?"
- "Do you want to organize your wallets with categories?"

---

### wallet_remove
**Description**: Remove a wallet from the registry (does not delete from blockchain)
**Category**: wallet_operations
**MCP Tool**: wallet_remove

**Parameters**:
- `address` (string, required): Wallet address to remove
- `confirm` (boolean): Confirmation required for safety

**Natural Language Patterns**:
- "remove my wallet"
- "delete wallet from registry"
- "unregister wallet"
- "remove wallet GD...ADDRESS"
- "delete wallet from list"

**Security Measures**:
- Require explicit confirmation
- Warn about consequences
- Suggest backup before removal
- Cannot remove active wallet

**Examples**:
```bash
# Remove with confirmation
"remove wallet GD...ADDRESS - I confirm"

# Remove with warning
"delete my old trading wallet - yes I'm sure"
```

**Error Handling**:
- Active wallet: "Cannot remove the active wallet. Please switch to another wallet first."
- Missing confirmation: "For security, please confirm you want to remove this wallet."
- Wallet not found: "I couldn't find that wallet in your registry."

---

### wallet_export
**Description**: Export wallet private key (use with extreme caution)
**Category**: wallet_operations
**MCP Tool**: wallet_export

**Parameters**:
- `address` (string): Wallet address to export (optional, uses active if omitted)
- `confirm` (boolean): Confirmation required for security

**Natural Language Patterns**:
- "export my private key"
- "show wallet secret key"
- "backup my wallet keys"
- "export wallet GD...ADDRESS"
- "get private key for backup"

**Security Warnings**:
- Multiple confirmation steps
- Security best practices advice
- Warning about exposure risks
- Suggest secure storage

**Examples**:
```bash
# Export with confirmation
"export my private key - I understand the risks"

# Export specific wallet
"export wallet GD...ADDRESS - confirmed"
```

**Follow-up Actions**:
- Suggest secure storage methods
- Offer to generate paper wallet
- Recommend hardware wallet backup

---

### wallet_passkey
**Description**: Manage passkey authentication for wallet
**Category**: wallet_operations
**MCP Tool**: wallet_passkey

**Parameters**:
- `action` (enum): register | verify | remove (default: register)
- `address` (string): Wallet address (optional, uses active if omitted)

**Natural Language Patterns**:
- "register passkey for wallet"
- "verify my passkey"
- "remove passkey authentication"
- "set up biometric login"
- "enable passkey for wallet"

**Examples**:
```bash
# Register passkey
"register passkey for my wallet"

# Verify passkey
"verify my passkey authentication"

# Remove passkey
"remove passkey from wallet"
```

**Passkey Benefits**:
- Enhanced security
- Biometric authentication
- No password memorization
- Cross-device synchronization

**Error Handling**:
- Device not supported: "Your device doesn't support passkeys. Would you like to use an alternative method?"
- Registration failed: "Passkey registration failed. Let's try again."

---

## Workflows

### Complete Wallet Setup
1. **Connect**: "I want to set up a new wallet"
2. **Import**: "Import with secret key or create new?"
3. **Fund**: "Fund from testnet faucet?"
4. **Verify**: "Check balance and confirm setup"
5. **Backup**: "Export backup keys securely?"

### Wallet Switching
1. **List**: "Show all my wallets"
2. **Select**: "Switch to trading wallet"
3. **Verify**: "Confirm wallet details"
4. **Update**: "Set as active wallet"

### Security Maintenance
1. **Audit**: "Review wallet security"
2. **Backup**: "Export important keys"
3. **Update**: "Refresh passkey authentication"
4. **Clean**: "Remove unused wallets"

## Common Questions

**Q: How many wallets can I have?**
A: You can have unlimited wallets, but it's best to keep them organized with clear names.

**Q: Are my funds safe if I remove a wallet?**
A: Removing a wallet only removes it from your local list. Your funds remain on the blockchain.

**Q: Can I use the same wallet on multiple devices?**
A: Yes, export your private key and import it on other devices, but keep it secure!

**Q: What's the difference between testnet and mainnet?**
A: Testnet uses fake XLM for testing, mainnet uses real XLM with actual value.

## Tips

- Always back up important wallets before making changes
- Use descriptive names to organize your wallets
- Enable passkey authentication for enhanced security
- Keep testnet and mainnet wallets separate
- Regularly audit your wallet collection
