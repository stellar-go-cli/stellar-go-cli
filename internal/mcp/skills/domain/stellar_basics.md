# Stellar Basics Knowledge

## Overview
Fundamental knowledge about the Stellar network, its architecture, operations, and key concepts that underpin the MozartPay platform's functionality.

## Core Concepts

### What is Stellar?
Stellar is an open-source, distributed payment network that enables fast, low-cost, cross-border payments between any pair of currencies. It's designed to connect financial systems and create a more inclusive global economy.

**Key Characteristics:**
- **Fast**: 3-5 second confirmation times
- **Low-cost**: Minimal transaction fees (0.00001 XLM)
- **Scalable**: Handles thousands of transactions per second
- **Open**: Anyone can join and build on the network
- **Inclusive**: Designed for financial inclusion

### Network Architecture

#### Consensus Mechanism
- **Stellar Consensus Protocol (SCP)**: Federated Byzantine Agreement
- **No mining**: No proof-of-work required
- **Energy efficient**: Environmentally friendly
- **Fast finality**: Quick transaction confirmation

#### Key Components
- **Nodes**: Network participants that validate transactions
- **Horizon**: HTTP API server for network interaction
- **Ledger**: Immutable record of all transactions
- **Anchors**: Bridges to traditional financial systems

### Native Asset: XLM (Lumens)

#### Purpose
- **Network Fee**: Pays for transaction fees
- **Account Reserve**: Minimum balance requirement
- **Multi-currency Bridge**: Facilitates currency exchange
- **Network Security**: Incentivizes honest behavior

#### XLM Properties
- **Total Supply**: 50 billion XLM (initially)
- **Inflation**: 1% annual inflation (historical)
- **Decimal Places**: 7 decimal places
- **Symbol**: XLM
- **Code**: XLM (native)

#### XLM Uses
1. **Transaction Fees**: Each transaction costs 0.00001 XLM
2. **Account Minimum**: Each account needs 1 XLM reserve
3. **Trustline Reserve**: Each trustline needs 0.5 XLM
4. **Offers**: Each offer needs 0.5 XLM
5. **Multi-signature**: Additional signers require reserves

### Accounts and Addresses

#### Stellar Address Format
- **Format**: Starts with 'G' followed by 56 characters
- **Example**: GD5DQYPNQFSX4TVWV5EHG63IZW5KQ5B4R6J3AY3ARJ2V2QZVX6A3Q
- **Encoding**: Base32Check encoding
- **Uniqueness**: Each address is globally unique
- **Derivation**: Derived from public key

#### Account Structure
- **Key Pair**: Public/private key pair
- **Sequence Number**: Prevents replay attacks
- **Balance**: Account balance in various assets
- **Trustlines**: Asset trust relationships
- **Signers**: Multi-signature configuration
- **Data**: Account metadata storage

#### Account Requirements
- **Minimum Balance**: 1 XLM base reserve
- **Trustline Reserve**: 0.5 XLM per asset
- **Signer Reserve**: 0.5 XLM per additional signer
- **Data Reserve**: 1 KB per data entry

### Transactions

#### Transaction Types
1. **Payment**: Direct asset transfer
2. **Path Payment**: Multi-asset payment with conversion
3. **Create Account**: Create new Stellar account
4. **Change Trust**: Establish/modify trustline
5. **Allow Trust**: Authorize trustline
6. **Set Options**: Configure account settings
7. **Manage Offer**: Create/modify/delete offer
8. **Manage Data**: Store/retrieve account data
9. **Bump Sequence**: Update account sequence
10. **Account Merge**: Merge account into another
11. **Inflation**: Vote for inflation destination
12. **Manage Buy Offer**: Create/modify/delete buy offer

#### Transaction Structure
- **Source Account**: Transaction originator
- **Fee**: Transaction fee (0.00001 XLM per operation)
- **Sequence Number**: Prevents replay attacks
- **Memo**: Optional transaction memo
- **Operations**: List of transaction operations
- **Signatures**: Transaction signatures
- **Time Bounds**: Optional time constraints

#### Transaction Lifecycle
1. **Creation**: Build transaction with operations
2. **Signing**: Add required signatures
3. **Submission**: Submit to network
4. **Validation**: Network validates transaction
5. **Inclusion**: Transaction added to ledger
6. **Confirmation**: Transaction is confirmed

### Operations

#### Payment Operation
- **Direct Payment**: Simple asset transfer
- **Path Payment**: Multi-asset payment
- **Destination**: Recipient address
- **Asset**: Asset code and issuer
- **Amount**: Payment amount
- **Source**: Source asset for path payments

#### Path Payments
- **Purpose**: Exchange assets during payment
- **Path**: Sequence of assets for conversion
- **Source Asset**: Asset to send
- **Destination Asset**: Asset to receive
- **Destination Amount**: Amount to receive
- **Send Max**: Maximum amount to send (strict-receive)
- **Dest Min**: Minimum amount to receive (strict-send)

#### Trustline Operations
- **Create Trustline**: Allow receiving specific asset
- **Modify Trustline**: Update trustline limits
- **Delete Trustline**: Remove asset trust
- **Asset Code**: Asset identifier
- **Asset Issuer**: Asset creator address
- **Limit**: Maximum trustline balance
- **Authorization**: Issuer authorization status

### Assets and Tokens

#### Native Asset (XLM)
- **Built-in**: Native to Stellar network
- **No Issuer**: Network-issued
- **Liquidity**: Highest liquidity
- **Universal**: Accepted everywhere

#### Custom Assets
- **SAC**: Stellar Asset Contract
- **SEP-41**: Asset standard
- **Issuer**: Asset creator
- **Code**: Asset identifier (max 12 chars)
- **Trustline**: Required for receiving

#### Asset Types
- **Fungible Tokens**: Interchangeable units
- **Non-Fungible Assets**: Unique items
- **Stablecoins**: Fiat-pegged tokens
- **Utility Tokens**: Platform-specific tokens
- **Security Tokens**: Investment tokens

#### Asset Properties
- **Code**: Asset identifier (case-sensitive)
- **Issuer**: Creator's Stellar address
- **Supply**: Total token supply
- **Decimals**: Decimal places (0-255)
- **Metadata**: Asset information
- **Trustlines**: User trust relationships

### Decentralized Exchange (DEX)

#### Order Book Model
- **Offers**: Limit orders on the order book
- **Buying**: Offers to buy specific assets
- **Selling**: Offers to sell specific assets
- **Price**: Exchange rate
- **Amount**: Offer quantity

#### Trading Pairs
- **XLM/USDC**: Lumens to USD Coin
- **USDC/EURC**: USD Coin to EUR Coin
- **XLM/BTC**: Lumens to Bitcoin
- **Custom Pairs**: Any supported assets

#### Path Payments
- **Multi-Asset**: Exchange through intermediate assets
- **Optimization**: Find best conversion paths
- **Slippage**: Price impact of trades
- **Liquidity**: Available trading volume

### Network Operations

#### Horizon API
- **RESTful API**: HTTP interface to network
- **Endpoints**: Account, transaction, asset data
- **Streaming**: Real-time updates
- **Rate Limiting**: API usage limits
- **Authentication**: No authentication required

#### Stellar Core
- **Node Software**: Network node implementation
- **Consensus**: Participates in SCP
- **Validation**: Validates transactions
- **Ledger Storage**: Maintains ledger copy
- **Network Communication**: Peer-to-peer protocol

#### Federation Protocol
- **Stellar Address**: Human-readable addresses
- **Federation Server**: Address resolution
- **Domain Mapping**: Domain to account mapping
- **Email Addresses**: Email-style addresses

### Security Features

#### Multi-Signature
- **Multiple Signers**: Require multiple approvals
- **Thresholds**: Minimum signatures required
- **Weight Distribution**: Signer power allocation
- **Backup Options**: Recovery signers

#### Account Security
- **Private Keys**: Secret key protection
- **Seed Phrases**: 24-word recovery phrase
- **Hardware Wallets**: Cold storage options
- **Encryption**: Data encryption

#### Network Security
- **Consensus Security**: Byzantine fault tolerance
- **Transaction Finality**: Immutable ledger
- **Replay Protection**: Sequence numbers
- **Spam Prevention**: Minimum fees and reserves

### Fees and Economics

#### Transaction Fees
- **Base Fee**: 0.00001 XLM per operation
- **Multi-Op Fee**: Fee per operation
- **Fee Bumps**: Increase fee for faster processing
- **Fee Statistics**: Network fee information

#### Reserve Requirements
- **Account Reserve**: 1 XLM minimum
- **Trustline Reserve**: 0.5 XLM per asset
- **Signer Reserve**: 0.5 XLM per signer
- **Data Reserve**: 1 KB per data entry

#### Economic Incentives
- **Fee Distribution**: Fees distributed to voters
- **Inflation**: Historical 1% annual inflation
- **Lumen Holding**: Encourages network participation
- **Network Security**: Economic incentives for honesty

### Best Practices

#### Account Management
- **Secure Storage**: Protect private keys
- **Backup Strategy**: Regular key backups
- **Multi-Sig**: Use multi-signature for important accounts
- **Regular Monitoring**: Check account activity

#### Transaction Management
- **Fee Optimization**: Use appropriate fees
- **Memo Usage**: Include relevant memos
- **Sequence Management**: Handle sequence numbers properly
- **Error Handling**: Handle failed transactions gracefully

#### Asset Management
- **Trustline Management**: Only trust reputable issuers
- **Diversification**: Spread risk across assets
- **Research**: Due diligence on custom assets
- **Liquidity**: Consider asset liquidity

#### Security Practices
- **Key Protection**: Never share private keys
- **Network Verification**: Verify network endpoints
- **Software Updates**: Keep software current
- **Phishing Protection**: Beware of scams

## Common Questions

**Q: What's the difference between XLM and other cryptocurrencies?**
A: XLM is Stellar's native asset designed for network fees and bridges, not primarily for speculation like Bitcoin.

**Q: How are Stellar transactions so fast?**
A: Stellar uses a federated consensus protocol that doesn't require mining, enabling 3-5 second confirmations.

**Q: Do I need XLM to use Stellar?**
A: Yes, you need XLM for account reserves and transaction fees, but the amounts are very small.

**Q: Can I create my own token on Stellar?**
A: Yes, you can create custom assets using the Stellar Asset Contract standard.

**Q: How secure is Stellar?**
A: Stellar uses proven cryptography and consensus mechanisms, with strong security features like multi-signature support.

## Tips for Users

- **Start Small**: Begin with small amounts to learn the system
- **Use Testnet**: Practice on testnet before using real funds
- **Secure Keys**: Protect your private keys and seed phrases
- **Check Fees**: Understand fee structures before transactions
- **Verify Addresses**: Double-check recipient addresses
- **Use Memos**: Include relevant information in transaction memos
- **Monitor Network**: Check network status before important transactions
- **Diversify**: Don't keep all assets in one place
- **Stay Informed**: Keep up with network developments
- **Use Reputable Services**: Choose well-known exchanges and services
