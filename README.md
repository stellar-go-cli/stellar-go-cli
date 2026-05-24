# MozartPay CLI — Orchestrated Agreements

> **v0.1.0-mvp** · Built in Go · Zero external dependencies · OG Technologies EU

A command-line interface for the MozartPay Orchestrated Agreements platform — enabling
DID-attested, VC-linked payments and asset issuance on Stellar with OA scoring,
StellarCarbon offset integration, x402 micropayments, and ISO 20022 compliance reporting.

---

## Architecture

```
[01] Identity    →  DID Attestation (web/key/ethr/ebsi) + National ID VC
[02] Wallet      →  wwWallet (passkey) + Stellar (native) + EOA + Testnet Faucet
[03] Payments    →  x402 · Tempo FX · Direct Stellar
     Assets      →  SAC/SEP-41 Fungible + Non-Fungible
[04] Integrations → OA Score · StellarCarbon · x402 · Tempo
[05] Reporting   →  VC-linked audit trail · ISO 20022 pacs.008 XML
```

## Requirements

- Go 1.22+
- No external dependencies (pure stdlib)

## Build

```bash
make build          # → dist/mozartpay
make install        # → $(GOPATH)/bin/mozartpay
```

## Quick Start

```bash
# Initialize
mozartpay init

# 1. Create identity
mozartpay did attest --method ebsi --vc national-id --name "Your Name" --country AT

# 2. Connect wallet
mozartpay wallet connect --provider stellar --network stellar-testnet
mozartpay wallet fund --network stellar-testnet

# 3. Create a SEP-41 token with score + carbon
mozartpay asset create-ft --name "MyToken" --symbol MTK \
  --supply 1000000 --with-score --with-carbon

# 4. Make a payment
mozartpay pay send --to <address> --amount 10 --asset USDC --rail x402

# 5. Generate compliance report
mozartpay report generate --vc-attach

# Export ISO 20022
mozartpay report iso20022
```

## Full Demo

```bash
make demo-full
```

## Commands

| Command | Description |
|---------|-------------|
| `mozartpay did create` | Create a DID document |
| `mozartpay did attest` | Issue a National ID Verifiable Credential |
| `mozartpay did verify` | Verify a saved VC |
| `mozartpay wallet connect` | Connect wwWallet, Stellar, or external EOA |
| `mozartpay wallet fund` | Fund testnet account via faucet |
| `mozartpay wallet balance` | Query account balance |
| `mozartpay pay send` | Send payment (x402/tempo/direct) |
| `mozartpay pay quote` | Get Tempo FX rate |
| `mozartpay pay x402` | HTTP 402 pay-per-use flow |
| `mozartpay asset create-ft` | Create SEP-41 fungible token |
| `mozartpay asset create-nfa` | Create SEP-41 non-fungible asset |
| `mozartpay asset score` | Attach OA score |
| `mozartpay asset carbon` | Attach StellarCarbon credits |
| `mozartpay integrations list` | List all integrations |
| `mozartpay integrations ping` | Health-check integrations |
| `mozartpay integrations score` | Add an OA score to latest asset |
| `mozartpay integrations carbon` | StellarCarbon management |
| `mozartpay report generate` | Post-transaction compliance report |
| `mozartpay report iso20022` | Export ISO 20022 pacs.008 XML |
| `mozartpay flow` | Print component flow diagram |
| `mozartpay status` | Session and system status |
| `mozartpay init` | Initialize configuration |

## Standards Compliance

- **W3C DID Core 1.0** — Decentralized Identifiers
- **W3C Verifiable Credentials** — VC Data Model 2.0
- **EBSI v3** — EU Blockchain Services Infrastructure
- **SEP-41** — Stellar Token Interface
- **ISO 20022** — Financial messaging (pacs.008.001.08)
- **eIDAS 2.0** — EU Digital Identity Framework
- **HTTP 402 / x402** — Pay-per-use web protocol

## Configuration

Config stored in `~/.mozartpay/config.json`.
State files stored in `~/.mozartpay/state/`.

---

*Built by OG Technologies EU · Vienna, Austria*  
*Web3 · Payments · Education · Standards*
