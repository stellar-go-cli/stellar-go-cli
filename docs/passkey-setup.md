# Passkey Setup Guide for wwWallet

This guide covers setting up WebAuthn passkey authentication with wwWallet in MozartPay CLI.

## What is wwWallet?

wwWallet is a WebAuthn-based wallet solution that uses hardware-backed passkeys for secure authentication. Unlike traditional private key wallets, wwWallet leverages your device's built-in security features (Touch ID, Face ID, Windows Hello, YubiKey) for cryptographic operations.

## Prerequisites

### System Requirements
- **macOS**: Touch ID or Face ID enabled
- **Windows**: Windows Hello configured
- **Linux**: Compatible security key (YubiKey, etc.)
- **Browser**: Modern browser with WebAuthn support

### Software Requirements
- MozartPay CLI v0.1.0-mvp or later
- Go 1.22+ (if building from source)

## Quick Start

### Method 1: Interactive Setup (Recommended)

```bash
# Build the CLI
make build

# Start interactive wallet setup
./mozartpay wallet connect
```

When prompted:
1. Select "1) wwWallet (WebAuthn passkeys)"
2. Choose your network (stellar-testnet recommended)
3. Follow the passkey creation prompts

### Method 2: Direct Commands

```bash
# Create wwWallet with passkey
./mozartpay wallet connect --provider wwwallet --network stellar-testnet --passkey

# Fund the wallet
./mozartpay wallet fund

# Show wallet details
./mozartpay wallet show
```

### Method 3: Using Scripts

```bash
# Make scripts executable
chmod +x scripts/*.sh

# Run multi-wallet manager
./scripts/multi-wallet.sh
# Select option 2 for wwWallet
```

## Detailed Setup Process

### Step 1: Initialize wwWallet

```bash
./mozartpay wallet connect --provider wwwallet --network stellar-testnet
```

**Expected Output:**
```
Connect Wallet
─────────────────────────────────
Select Wallet Type
1) wwWallet (WebAuthn passkeys)
2) Stellar (Native Stellar keypair)
3) External EOA
Choose option [1-3]: 1

Initiating WebAuthn passkey ceremony... ✓ wwWallet connected via passkey
```

### Step 2: Passkey Creation

The system will simulate a WebAuthn ceremony and create:
- **Credential ID**: Unique identifier for your passkey
- **Public Key**: Cryptographic public key
- **Algorithm**: ES256 signature algorithm
- **Origin**: wwWallet application origin

### Step 3: Wallet Verification

```bash
./mozartpay wallet show
```

**Expected Output:**
```
Wallet Info
─────────────────────────────────
Connected Account
Address: GDQ5ENR7YRYH4DAKZ2YQ3S2N5DQX2W3FMIPRGDZJAVJNGXQJQYQQ
Network: Testnet
Type: wwwallet
Balance: 0 XLM
Funded: false

Passkey Credential
Credential ID: pkcpub_abc123def456...
Algorithm: ES256
Origin: https://wwwallet.app
```

### Step 4: Fund Your Wallet

```bash
./mozartpay wallet fund
```

This will request testnet funds from the Stellar Friendbot faucet.

## Passkey Management Commands

### View Passkey Details

```bash
./mozartpay wallet show
# Shows passkey credential information
```

### Export Wallet (Backup)

```bash
./mozartpay wallet export --private
# ⚠️  Use with caution - shows sensitive information
```

### Switch Between Wallets

```bash
./mozartpay wallet list
./mozartpay wallet switch <address>
```

## Security Best Practices

### Passkey Security
- ✅ **Hardware-Backed**: Uses device secure enclave
- ✅ **Biometric**: Requires your fingerprint/face
- ✅ **No Private Keys**: No seed phrases to store
- ✅ **Phishing Resistant**: Bound to your domain

### Backup Strategy
1. **Device Backup**: Ensure your device is backed up
2. **Multiple Devices**: Set up wwWallet on multiple devices
3. **Recovery Options**: Keep alternative wallet types as backup

### Recovery Options
If you lose access to your passkey:
1. Use another device with the same passkey
2. Create a new wwWallet on a different device
3. Use a traditional Stellar wallet as fallback

## Troubleshooting

### Common Issues

#### "Passkey ceremony failed"
**Solution:**
- Ensure your device has biometric authentication enabled
- Check that your browser supports WebAuthn
- Try using a different browser

#### "Wallet not funded"
**Solution:**
- Run `./mozartpay wallet fund`
- Check network connectivity
- Verify you're using testnet

#### "Passkey not recognized"
**Solution:**
- Ensure you're on the same device that created the passkey
- Check that biometric authentication is working
- Try recreating the passkey

### Debug Mode

Enable debug logging for troubleshooting:

```bash
./mozartpay wallet connect --provider wwwallet --debug
```

### Reset Passkey

If you need to reset your passkey:

```bash
# Create a new wwWallet (old one remains inactive)
./mozartpay wallet connect --provider wwwallet

# Or remove old wallet and create new one
./mozartpay wallet list
./mozartpay wallet connect --provider wwwallet
```

## Advanced Configuration

### Custom Networks

```bash
# Mainnet setup (when ready)
./mozartpay wallet connect --provider wwwallet --network stellar-mainnet
```

### Multiple Passkeys

You can create multiple wwWallet instances with different passkeys:

```bash
# First wallet
./mozartpay wallet connect --provider wwwallet --network stellar-testnet

# Second wallet (different passkey)
./mozartpay wallet connect --provider wwwallet --network stellar-testnet
```

## Integration with Other Features

### DID Integration

Your wwWallet can be linked to Decentralized Identifiers:

```bash
# Create DID first
./mozartpay did create --method key

# Then connect wallet (will auto-link)
./mozartpay wallet connect --provider wwwallet
```

### Asset Creation

Use your wwWallet for asset issuance:

```bash
./mozartpay asset create-ft --name "MyToken" --symbol "MTK"
```

### Payment Operations

Send payments using your wwWallet:

```bash
./mozartpay pay send --to <address> --amount 10 --asset XLM
```

## Developer Information

### Passkey Structure

```go
type PasskeyCredential struct {
    CredentialID string    `json:"credentialId"`
    PublicKey    string    `json:"publicKey"`
    Algorithm    string    `json:"algorithm"`
    Origin       string    `json:"origin"`
    CreatedAt    time.Time `json:"createdAt"`
}
```

### Storage Location

Passkey credentials are stored locally at:
```
~/.mozartpay/state/wallets/<address>.json
```

### WebAuthn Flow

1. **Credential Creation**: Passkey created via WebAuthn API
2. **Authentication**: Biometric verification required
3. **Signing**: Cryptographic operations using passkey
4. **Storage**: Secure local storage with device permissions

## Support

If you encounter issues:

1. Check this guide for common solutions
2. Enable debug mode for detailed logs
3. Review the troubleshooting section
4. Check the MozartPay documentation

## Next Steps

After setting up your wwWallet:

1. Fund your wallet with testnet XLM
2. Create your first asset
3. Explore payment features
4. Set up DID integration

For more advanced usage, see the [wwWallet Integration Guide](wwallet-integration.md).
