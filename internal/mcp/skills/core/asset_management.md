# Asset Management Skills

## Overview
Comprehensive asset creation, management, and analysis capabilities for the Stellar Go CLI platform, including token issuance, trustline management, asset scoring, and carbon credit integration.

## Skills

### asset_create_ft
**Description**: Create a fungible token (SAC/SEP-41) on Stellar
**Category**: asset_management
**MCP Tool**: asset_create_ft

**Parameters**:
- `name` (string, required): Token name
- `symbol` (string, required): Token symbol, max 12 chars
- `supply` (string): Total supply (default: 1000000)
- `decimals` (number): Decimal places (default: 7)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-testnet)
- `with_carbon` (boolean): Attach StellarCarbon offset credits (default: false)
- `carbon_amt` (number): Carbon credit amount in tCO2e (default: 1.0)

**Natural Language Patterns**:
- "create a token called 'MyToken'"
- "issue fungible token with symbol MTK"
- "create 1 million tokens with 7 decimals"
- "launch a carbon-neutral token"
- "create token with carbon credits"

**Examples**:
```bash
# Basic token creation
"create token 'MyToken' with symbol MTK"

# With specific supply
"create 5 million tokens called 'RewardToken' with symbol RWD"

# Carbon-neutral token
"create carbon-neutral token 'GreenCoin' with symbol GRN"
```

**Creation Process**:
1. Validate token parameters
2. Create Stellar Asset Contract
3. Set up metadata
4. Attach carbon credits (if requested)
5. Register on network
6. Provide deployment details

**Follow-up Questions**:
- "Would you like to set up a trustline for this token?"
- "Should I create a liquidity pool?"
- "Do you want to distribute tokens to holders?"

---

### asset_create_nfa
**Description**: Create a non-fungible asset (NFA) using SEP-41
**Category**: asset_management
**MCP Tool**: asset_create_nfa

**Parameters**:
- `name` (string, required): Asset name
- `symbol` (string, required): Asset symbol
- `uri` (string, required): Metadata URI
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-testnet)

**Natural Language Patterns**:
- "create NFT called 'Digital Art'"
- "issue non-fungible token with metadata"
- "create NFA collection"
- "mint unique asset with metadata"
- "create NFT with metadata URI"

**Examples**:
```bash
# Basic NFA creation
"create NFT 'Digital Art' with symbol ART and metadata at https://..."

# Collection creation
"create NFA collection 'MyCollection' with symbol MYC"
```

**NFA Features**:
- Unique asset identification
- Metadata linking
- Ownership tracking
- Transfer capabilities
- Collection management

**Metadata Requirements**:
- Valid URI format
- JSON schema compliance
- Asset description
- Media references
- Creator information

**Follow-up Questions**:
- "Would you like to mint multiple NFTs?"
- "Should I set up royalty distribution?"
- "Do you want to create a marketplace listing?"

---

### asset_trust
**Description**: Establish a trustline for an asset (required before receiving)
**Category**: asset_management
**MCP Tool**: asset_trust

**Parameters**:
- `code` (string, required): Asset code (e.g., USDC)
- `issuer` (string, required): Asset issuer address
- `limit` (string): Trustline limit (optional)
- `execute` (boolean): Actually execute the transaction (default: true)

**Natural Language Patterns**:
- "trust USDC from issuer"
- "create trustline for token"
- "enable receiving USDC"
- "set up trustline for asset"
- "trust asset with limit"

**Examples**:
```bash
# Basic trustline
"trust USDC from issuer GD...ISSUER"

# With limit
"create trustline for USDC with limit 10000"

# Multiple assets
"trust USDC, EURC, and BTC from their issuers"
```

**Trustline Management**:
- Asset authorization
- Limit setting
- Fee management
- Revocation options
- Balance tracking

**Security Considerations**:
- Issuer verification
- Risk assessment
- Limit recommendations
- Fee implications

**Follow-up Questions**:
- "Would you like to check the asset details first?"
- "Should I set a specific trustline limit?"
- "Do you want to see other trusted assets?"

---

### asset_info
**Description**: Get detailed information about a specific asset
**Category**: asset_management
**MCP Tool**: asset_info

**Parameters**:
- `code` (string, required): Asset code
- `issuer` (string): Asset issuer (optional for native assets)

**Natural Language Patterns**:
- "show me information about USDC"
- "get details for token MTK"
- "what is this asset?"
- "show asset metadata"
- "tell me about token X"

**Examples**:
```bash
# Basic info
"show information about USDC"

# With issuer
"get details for USDC from issuer GD...ISSUER"

# Native asset
"show information about XLM"
```

**Asset Information**:
- Basic details (code, issuer, supply)
- Market data (price, volume, market cap)
- Trustline requirements
- Fee structure
- Compliance status

**Market Analysis**:
- Price history
- Trading volume
- Liquidity metrics
- Holder distribution
- Network activity

**Follow-up Questions**:
- "Would you like to create a trustline for this asset?"
- "Should I show you similar assets?"
- "Do you want to see recent transactions?"

---

### asset_score
**Description**: Get asset quality and risk score analysis
**Category**: asset_management
**MCP Tool**: asset_score

**Parameters**:
- `code` (string, required): Asset code
- `issuer` (string): Asset issuer (optional for native assets)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)

**Natural Language Patterns**:
- "score the risk of USDC"
- "analyze asset quality"
- "get risk assessment for token"
- "evaluate asset safety"
- "show asset score and rating"

**Examples**:
```bash
# Risk scoring
"score the risk of USDC token"

# Quality analysis
"analyze quality of MTK token"

# Comprehensive assessment
"evaluate USDC safety and risk factors"
```

**Scoring Factors**:
- **Liquidity** (0-100): Market depth and trading volume
- **Volatility** (0-100): Price stability and predictability
- **Trust Score** (0-100): Issuer reputation and compliance
- **Market Cap** (0-100): Total market value and stability
- **Network Activity** (0-100): On-chain activity and usage

**Risk Categories**:
- **Low Risk** (80-100): Established, stable assets
- **Medium Risk** (60-79): Moderate volatility, good liquidity
- **High Risk** (40-59): High volatility, limited liquidity
- **Very High Risk** (0-39): New or unproven assets

**Recommendations**:
- Investment suitability
- Portfolio allocation
- Risk mitigation strategies
- Monitoring requirements

---

### asset_carbon
**Description**: Attach StellarCarbon offset credits to an asset
**Category**: asset_management
**MCP Tool**: asset_carbon

**Parameters**:
- `code` (string, required): Asset code
- `amount` (number, required): Carbon credit amount in tCO2e
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)

**Natural Language Patterns**:
- "attach carbon credits to USDC"
- "make token carbon neutral"
- "offset carbon for asset"
- "add carbon credits to token"
- "green my token with carbon offsets"

**Examples**:
```bash
# Basic carbon attachment
"attach 10 tCO2e carbon credits to USDC"

# Make carbon neutral
"make MTK token carbon neutral with carbon credits"

# Large scale offset
"offset 1000 tCO2e for our token ecosystem"
```

**Carbon Benefits**:
- Environmental responsibility
- ESG compliance
- Green branding
- Regulatory advantages
- Investor appeal

**Verification Process**:
- Credit validation
- On-chain recording
- Certification linking
- Audit trail creation
- Public transparency

**Follow-up Questions**:
- "Would you like to see the carbon credit certificate?"
- "Should I create a green asset report?"
- "Do you want to promote the carbon-neutral status?"

---

### asset_show
**Description**: Show comprehensive asset details including metadata and performance
**Category**: asset_management
**MCP Tool**: asset_show

**Parameters**:
- `code` (string, required): Asset code
- `issuer` (string): Asset issuer (optional for native assets)
- `network` (enum): stellar-testnet | stellar-mainnet (default: stellar-mainnet)

**Natural Language Patterns**:
- "show comprehensive details about USDC"
- "display full asset information"
- "show everything about token X"
- "complete asset analysis"
- "detailed asset report"

**Examples**:
```bash
# Comprehensive view
"show comprehensive details about USDC"

# Performance focus
"show performance data for MTK token"

# Metadata deep dive
"display full metadata and asset information"
```

**Comprehensive Details**:
- Basic asset information
- Market performance data
- Metadata and documentation
- Trustline and compliance info
- Carbon credit status
- Historical performance
- Community metrics

**Performance Metrics**:
- Price charts and trends
- Volume analysis
- Market cap evolution
- Holder growth
- Network adoption
- Development activity

**Follow-up Actions**:
- "Would you like to create a trustline?"
- "Should I set up price alerts?"
- "Do you want to see similar assets?"

---

### asset_list
**Description**: List all known assets on the configured network
**Category**: asset_management
**MCP Tool**: asset_list

**Natural Language Patterns**:
- "list all available assets"
- "show all tradable tokens"
- "what assets are on this network?"
- "list market assets"
- "show all tokens I can use"

**Examples**:
```bash
# List all assets
"list all available assets"

# Filter by type
"show all stablecoin assets"

# Network specific
"list assets on stellar mainnet"
```

**Asset Categories**:
- **Native Assets**: XLM and network-native tokens
- **Stablecoins**: USDC, EURC, and other fiat-pegged tokens
- **DeFi Tokens**: Yield farming and governance tokens
- **NFT Collections**: Non-fungible token collections
- **Utility Tokens**: Platform-specific utility tokens

**Listing Options**:
- Sort by market cap
- Filter by asset type
- Show only trusted assets
- Include performance metrics
- Display carbon-neutral assets

---

## Workflows

### Token Creation Process
1. **Research**: "Analyze market for similar tokens"
2. **Design**: "Create token with symbol XYZ and 1B supply"
3. **Create**: "Issue the token on testnet"
4. **Verify**: "Check token details and metadata"
5. **Deploy**: "Launch on mainnet with carbon credits"
6. **Market**: "Set up liquidity and distribution"

### Asset Trust Management
1. **Research**: "Show information about USDC token"
2. **Evaluate**: "Score the risk and quality"
3. **Trust**: "Create trustline for USDC"
4. **Monitor**: "Track asset performance"
5. **Manage**: "Adjust trustline limits as needed"

### Carbon Neutral Assets
1. **Assess**: "Calculate carbon footprint"
2. **Offset**: "Attach carbon credits to token"
3. **Verify**: "Show carbon credit certification"
4. **Promote**: "Highlight green credentials"
5. **Report**: "Generate ESG compliance report"

### Portfolio Asset Management
1. **Inventory**: "List all my trusted assets"
2. **Analyze**: "Score risk and quality of holdings"
3. **Rebalance**: "Adjust asset allocations"
4. **Optimize**: "Improve portfolio efficiency"
5. **Monitor**: "Track performance and changes"

## Advanced Concepts

### Stellar Asset Contract (SAC)
Understanding SAC implementation:
- Contract deployment
- Metadata management
- Upgrade capabilities
- Governance mechanisms

### SEP-41 Standards
- Fungible token standards
- Non-fungible token standards
- Metadata schemas
- Compliance requirements

### Carbon Credit Integration
- StellarCarbon protocol
- Credit verification
- On-chain recording
- ESG compliance

### Asset Scoring Methodology
- Risk assessment algorithms
- Market analysis metrics
- Quality evaluation criteria
- Predictive modeling

## Common Questions

**Q: What's the difference between FT and NFA?**
A: FT (Fungible Token) are interchangeable like currency, NFA (Non-Fungible Asset) are unique like collectibles.

**Q: Do I need a trustline for every asset?**
A: Yes, you need a trustline before receiving any non-native asset on Stellar.

**Q: How are carbon credits verified?**
A: Through StellarCarbon's verification system with on-chain recording and off-chain certification.

**Q: What makes a good asset score?**
A: High liquidity, low volatility, reputable issuer, and strong market adoption.

## Tips

- Always research assets before creating trustlines
- Use asset scoring to evaluate investment risks
- Consider carbon credits for ESG compliance
- Monitor asset performance regularly
- Diversify across different asset types
- Keep track of trustline limits and fees
- Use metadata to provide clear asset information
- Consider market conditions when creating new tokens
- Leverage carbon-neutral branding for investor appeal
- Maintain proper documentation for compliance
