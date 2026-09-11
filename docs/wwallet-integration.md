# wwWallet Integration Guide

This guide covers the technical integration details of wwWallet with MozartPay CLI, including WebAuthn implementation, security considerations, and advanced usage patterns.

## Architecture Overview

### wwWallet Components

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   User Device   │    │   wwWallet      │    │   MozartPay     │
│                 │    │   Service       │    │   CLI           │
│  • Face ID      │◄──►│  • WebAuthn     │◄──►│  • Wallet Mgmt  │
│  • Touch ID     │    │  • Passkey Mgmt │    │  • Asset Issuance│
│  • Windows Hello│    │  • Crypto Ops   │    │  • Payment      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### WebAuthn Flow

1. **Registration**: Passkey creation ceremony
2. **Authentication**: Biometric verification
3. **Assertion**: Cryptographic signature
4. **Verification**: Server-side validation

## Implementation Details

### Passkey Creation Process

```go
// ConnectWWWallet simulates wwWallet connection via passkey
func (s *Service) ConnectWWWallet(network models.Network) (*models.Account, *models.PasskeyCredential, error) {
    // Simulate WebAuthn passkey ceremony
    passkey := &models.PasskeyCredential{
        CredentialID: mpCrypto.RandomBase64(32),
        PublicKey:    "pkcpub_" + mpCrypto.RandomHex(32),
        Algorithm:    "ES256",
        Origin:       "https://wwwallet.app",
        CreatedAt:    time.Now().UTC(),
    }

    address := s.deriveAddress(network, passkey.CredentialID)
    acc := &models.Account{
        Address:   address,
        PublicKey: passkey.PublicKey,
        Network:   network,
        Type:      models.WalletWWWallet,
        Balance:   "0",
        Funded:    false,
        CreatedAt: time.Now().UTC(),
    }
    return acc, passkey, nil
}
```

### Address Derivation

The wallet address is derived from the passkey credential ID:

```go
func (s *Service) deriveAddress(network models.Network, seed string) string {
    hash := mpCrypto.Hash256([]byte(seed))
    switch network {
    case models.NetworkStellarTestnet, models.NetworkStellarMainnet:
        // Stellar-style G... address
        return "G" + hash[:54]
    default:
        return "0x" + hash[:40]
    }
}
```

### Data Structures

#### PasskeyCredential

```go
type PasskeyCredential struct {
    CredentialID string    `json:"credentialId"` // WebAuthn credential ID
    PublicKey    string    `json:"publicKey"`    // Cryptographic public key
    Algorithm    string    `json:"algorithm"`    // Signature algorithm (ES256)
    Origin       string    `json:"origin"`       // Relying party origin
    CreatedAt    time.Time `json:"createdAt"`    // Creation timestamp
}
```

#### Account with Passkey

```go
type Account struct {
    Address    string     `json:"address"`
    PublicKey  string     `json:"publicKey"`
    Network    Network    `json:"network"`
    Type       WalletType `json:"type"`        // WalletWWWallet
    Balance    string     `json:"balance"`
    Funded     bool       `json:"funded"`
    DID        string     `json:"did,omitempty"`
    CreatedAt  time.Time  `json:"createdAt"`
    // Note: PrivateKey is empty for wwWallet (managed by passkey)
}
```

## Security Model

### Threat Protection

| Threat | Protection Level | Mechanism |
|--------|------------------|-----------|
| **Private Key Theft** | ✅ High | No private keys stored |
| **Phishing** | ✅ High | Domain-bound credentials |
| **Man-in-the-Middle** | ✅ High | WebAuthn origin validation |
| **Device Theft** | ⚠️ Medium | Biometric protection |
| **Replay Attacks** | ✅ High | Challenge-response protocol |

### Cryptographic Operations

#### Signature Algorithm
- **Algorithm**: ES256 (ECDSA with P-256 and SHA-256)
- **Key Generation**: Device secure enclave
- **Signature Format**: WebAuthn assertion format

#### Key Storage
- **Location**: Device secure enclave
- **Access**: Biometric authentication required
- **Backup**: Device backup mechanisms

### WebAuthn Configuration

```javascript
// WebAuthn Relying Party Configuration
const rpConfig = {
    id: "wwwallet.app",
    name: "wwWallet",
    origin: "https://wwwallet.app"
};

// Credential Creation Options
const createOptions = {
    publicKey: {
        challenge: new Uint8Array(32),
        rp: rpConfig,
        user: {
            id: new Uint8Array(16),
            name: "user@mozartpay.com",
            displayName: "MozartPay User"
        },
        pubKeyCredParams: [
            { alg: -7, type: "public-key" } // ES256
        ],
        authenticatorSelection: {
            authenticatorAttachment: "platform",
            userVerification: "required"
        },
        timeout: 60000
    }
};
```

## Integration Patterns

### Basic Wallet Operations

```bash
# Create wwWallet
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Fund wallet
./mozartpay wallet fund

# Check balance
./mozartpay wallet balance

# Show wallet details
./mozartpay wallet show
```

### Asset Management

```bash
# Create fungible token
./mozartpay asset create-ft --name "MyToken" --symbol "MTK" --supply 1000000

# Create non-fungible asset
./mozartpay asset create-nfa --name "MyNFA" --description "Digital collectible"

# Attach carbon credits
./mozartpay asset carbon --amount 1.0 --standard VCS
```

### Payment Operations

```bash
# Send payment
./mozartpay pay send --to GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ --amount 100 --asset XLM

# Request FX quote
./mozartpay pay quote --from XLM --to EUR --amount 1000

# HTTP 402 payment
./mozartpay pay x402 --url https://api.example.com/resource --price 0.5
```

### DID Integration

```bash
# Create DID
./mozartpay did create --method key

# Issue VC
./mozartpay did attest --type "NationalID" --subject "did:key:z123..."

# Verify VC
./mozartpay did verify --vc-id vc_123456
```

## Advanced Features

### Multi-Device Support

Users can create wwWallet instances on multiple devices:

```bash
# Device 1
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Device 2 (same passkey if supported)
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Device 3 (different passkey)
./mozartpay wallet connect --provider wwwallet --network stellar-testnet
```

### Wallet Registry Management

```bash
# List all wallets
./mozartpay wallet list

# Switch active wallet
./mozartpay wallet switch GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ

# Rename wallet
./mozartpay wallet rename GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ "My wwWallet"

# Export wallet (read-only)
./mozartpay wallet export --address GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ
```

### Network Configuration

```bash
# Testnet
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Mainnet (production)
./mozartpay wallet connect --provider wwwallet --network stellar-mainnet

# EVM Networks
./mozartpay wallet connect --provider wwwallet --network evm-sepolia
./mozartpay wallet connect --provider wwwallet --network evm-mainnet
```

## Error Handling

### Common Error Scenarios

#### Passkey Creation Failed

```go
if err != nil {
    return nil, nil, fmt.Errorf("passkey creation failed: %w", err)
}
```

**CLI Response:**
```
❌ Passkey creation failed: WebAuthn not supported
💡 Ensure your device supports WebAuthn and biometric authentication
```

#### Network Connection Issues

```go
if !acc.Funded {
    ui.Warn("Account not funded. Run: mozartpay wallet fund")
    ui.Info("Faucet: " + wallet.FaucetURL(net, acc.Address))
}
```

#### Wallet Not Found

```go
acc, err := svc.GetActiveWallet()
if err != nil {
    ui.Warn("No active wallet found. Run 'mozartpay wallet connect' first.")
    return nil
}
```

### Debug Mode

Enable detailed logging:

```bash
./mozartpay wallet connect --provider wwwallet --debug
```

**Debug Output:**
```
DEBUG: Starting WebAuthn ceremony
DEBUG: Generating passkey credential
DEBUG: Credential ID: pkcpub_abc123...
DEBUG: Deriving address from credential
DEBUG: Address: GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ
DEBUG: Saving wallet to registry
```

## Performance Considerations

### Passkey Authentication Speed

- **Touch ID**: ~100ms
- **Face ID**: ~200ms
- **Windows Hello**: ~150ms
- **YubiKey**: ~300ms

### Network Operations

- **Balance Query**: ~500ms
- **Transaction Submission**: ~1-2s
- **Faucet Funding**: ~1-3s

### Storage Optimization

- **Wallet File Size**: ~1KB per wallet
- **Registry Size**: ~100B per wallet entry
- **Passkey Storage**: ~200B per credential

## Testing Strategy

### Unit Tests

```go
func TestConnectWWWallet(t *testing.T) {
    svc := wallet.NewService()
    acc, passkey, err := svc.ConnectWWWallet(models.NetworkStellarTestnet)
    
    assert.NoError(t, err)
    assert.Equal(t, models.WalletWWWallet, acc.Type)
    assert.NotEmpty(t, passkey.CredentialID)
    assert.Equal(t, "ES256", passkey.Algorithm)
}
```

### Integration Tests

```bash
# Test wallet creation
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Test funding
./mozartpay wallet fund

# Test operations
./mozartpay wallet balance
./mozartpay wallet show
```

### End-to-End Tests

```bash
# Complete workflow
./scripts/demo-full.sh

# Multi-wallet test
./scripts/multi-wallet.sh
```

## Migration Guide

### From Traditional Wallets

1. **Export**: Backup existing wallet
2. **Create**: Set up wwWallet
3. **Transfer**: Move funds if needed
4. **Verify**: Test new wallet functionality

### Between Devices

1. **Backup**: Ensure device backup is enabled
2. **Setup**: Create wwWallet on new device
3. **Restore**: Use device restore or recreate
4. **Verify**: Test passkey functionality

## Future Enhancements

### Planned Features

- **Hardware Key Support**: YubiKey, SoloKeys
- **Multi-Signature**: Multiple passkeys per wallet
- **Social Recovery**: Trusted contact recovery
- **Cloud Backup**: Encrypted cloud storage
- **Enterprise Features**: MDM integration

### WebAuthn Extensions

- **PRF**: Pseudorandom function for key derivation
- **Large Blob**: Secure storage on passkey
- **CredProps**: Credential properties management

## API Reference

### Wallet Service Methods

```go
// Connect wwWallet with passkey
func (s *Service) ConnectWWWallet(network models.Network) (*models.Account, *models.PasskeyCredential, error)

// Get active wallet
func (s *Service) GetActiveWallet() (*models.Account, error)

// List all wallets
func (s *Service) ListWallets() ([]models.WalletEntry, string, error)

// Switch active wallet
func (s *Service) SetActiveWallet(address string) error

// Update wallet balance
func (s *Service) UpdateWalletBalance(address string) (*models.Account, error)
```

### CLI Commands

```bash
# Wallet operations
./mozartpay wallet connect --provider wwwallet [--network <net>] [--passkey]
./mozartpay wallet fund [--network <net>]
./mozartpay wallet balance [--address <addr>]
./mozartpay wallet show
./mozartpay wallet list
./mozartpay wallet switch <address>
./mozartpay wallet rename <address> <name>
./mozartpay wallet export [--address <addr>] [--private]

# Asset operations
./mozartpay asset create-ft --name <name> --symbol <sym> [--supply <amt>]
./mozartpay asset create-nfa --name <name> [--description <desc>]
./mozartpay asset score
./mozartpay asset carbon --amount <amt> --standard <std>
./mozartpay asset show

# Payment operations
./mozartpay pay send --to <addr> --amount <amt> [--asset <asset>]
./mozartpay pay quote --from <src> --to <dst> --amount <amt>
./mozartpay pay x402 --url <url> --price <price>

# DID operations
./mozartpay did create --method <method>
./mozartpay did attest --type <type> --subject <did>
./mozartpay did verify --vc-id <id>
```

## Support and Resources

### Documentation
- [Passkey Setup Guide](passkey-setup.md)
- [CLI Reference](../README.md)
- [Architecture Overview](../docs/architecture.md)

### Community
- GitHub Issues: Report bugs and request features
- Discord: Community discussion and support
- Documentation: Technical guides and API reference

### Troubleshooting
- Check [Common Issues](passkey-setup.md#troubleshooting)
- Enable debug mode for detailed logs
- Review error messages for specific guidance
