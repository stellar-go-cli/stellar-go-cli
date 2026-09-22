# Stellar Go CLI

> Open-source Go CLI for Stellar: wallets, payments, swaps, SEP-41 assets, W3C DIDs/Verifiable Credentials, ISO 20022 (pacs) reporting, Soroban RPC and an MCP server for AI assistants. Apache 2.0.

Extracted from [MozartPay Orchestrated Agreements](https://github.com/mozartpay/OAs), which is now a downstream consumer of this repository. Maintained by Olvis E. Gil Ríos (OG Technologies EU) and contributors.

```bash
go install github.com/stellar-go-cli/stellar-go-cli/cmd/stellar-go-cli@latest
stellar-go-cli --help
```

Status: wallet/payments/swaps run on mainnet; DID/VC (Ed25519Signature2020, cryptographic verification) and ISO 20022 pacs.008/002/004/009 (validates against official XSDs) are working prototypes — VCs are self-issued unless an external issuer is configured. Not a Stellar Development Foundation project.

---

## Architecture

```
[01] Identity    →  DID Attestation (web/key/ethr/ebsi) + National ID VC
[02] Wallet      →  wwWallet (passkey) + Stellar (native) + EOA + Testnet Faucet
[03] Payments    →  x402 · Tempo FX · Direct Stellar
     Assets      →  SAC/SEP-41 Fungible + Non-Fungible
[04] Contracts   →  Soroban Smart Contracts (deploy, invoke, simulate)
[05] Integrations → OA Score · StellarCarbon · x402 · Tempo
[06] Reporting   →  VC-linked audit trail · ISO 20022 pacs.008 XML
[07] AI          →  MCP Server (stdio/SSE) · Chat · Terminal
```

## Features

- **DID Attestation** — Create and verify W3C DIDs (web, key, ethr, ebsi) with National ID Verifiable Credentials
- **Wallet Management** — Connect wwWallet, Stellar native, or external EOA; testnet faucet funding
- **Payments** — x402 pay-per-use, Tempo FX quotes, direct Stellar payments
- **Asset Creation** — SEP-41 fungible tokens and non-fungible assets with OA score and carbon credits
- **Soroban Contracts** — Deploy any `.wasm` file, invoke contract methods, simulate read-only queries — all in pure Go
- **MCP Server** — Expose CLI functionality as tools for AI assistants via stdio or SSE transport
- **WebAuthn/FIDO2** — Passkey-based authentication server
- **Compliance Reporting** — VC-linked audit trails, ISO 20022 pacs.008 XML export
- **Integrations** — OA Score, StellarCarbon, x402, Tempo FX

## Requirements

- Go 1.24+
- Rust toolchain (for building smart contracts, optional)
- Docker & Docker Compose (optional, for containerized deployment)

## Build & Installation

```bash
# Build from source
make build              # → dist/mozartpay
make install            # → $(GOPATH)/bin/mozartpay

# Docker
docker compose up       # Start all services (MCP, WebAuthn, PostgreSQL, Redis)

# Kubernetes
# See k8s/README.md for deployment instructions
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

# 3. Deploy a Soroban smart contract
mozartpay contract deploy --wasm contracts/dist/mozartpay_contracts.wasm --network stellar-testnet

# 4. Create an on-chain agreement
mozartpay contract create-agreement --dispute-window 86400

# 5. Create a SEP-41 token with score + carbon
mozartpay asset create-ft --name "MyToken" --symbol MTK \
  --supply 1000000 --with-score --with-carbon

# 6. Make a payment
mozartpay pay send --to <address> --amount 10 --asset USDC --rail x402

# 7. Generate compliance report
mozartpay report generate --vc-attach

# Export ISO 20022
mozartpay report iso20022
```

## Full Demo

```bash
make demo-full
```

## Commands

### Identity

| Command | Description |
|---------|-------------|
| `mozartpay did create` | Create a DID document |
| `mozartpay did attest` | Issue a National ID Verifiable Credential |
| `mozartpay did verify` | Verify a saved VC |

### Wallet

| Command | Description |
|---------|-------------|
| `mozartpay wallet connect` | Connect wwWallet, Stellar, or external EOA |
| `mozartpay wallet fund` | Fund testnet account via faucet |
| `mozartpay wallet balance` | Query account balance |

### Transactions

| Command | Description |
|---------|-------------|
| `mozartpay pay send` | Send payment (x402/tempo/direct) |
| `mozartpay pay quote` | Get Tempo FX rate |
| `mozartpay pay x402` | HTTP 402 pay-per-use flow |
| `mozartpay swap quote` | Get asset swap quote |
| `mozartpay swap execute` | Execute asset swap |
| `mozartpay pool` | Liquidity pool operations |
| `mozartpay trade` | Trading operations |
| `mozartpay asset create-ft` | Create SEP-41 fungible token |
| `mozartpay asset create-nfa` | Create SEP-41 non-fungible asset |
| `mozartpay asset score` | Attach OA score |
| `mozartpay asset carbon` | Attach StellarCarbon credits |
| `mozartpay claimable` | Claimable balance operations |

### Exchanges

| Command | Description |
|---------|-------------|
| `mozartpay exchange` | Exchange management |

### Contracts (Soroban)

| Command | Description |
|---------|-------------|
| `mozartpay contract deploy` | Deploy any `.wasm` contract to Soroban |
| `mozartpay contract set` | Store a contract ID in config |
| `mozartpay contract create-agreement` | Create an on-chain orchestrated agreement |
| `mozartpay contract show` | Display agreement details (simulate-only) |
| `mozartpay contract list` | List agreements by initiator (simulate-only) |
| `mozartpay contract attest-identity` | Attest DID and VC for an agreement (Layer 1) |
| `mozartpay contract connect-wallet` | Connect wallet to an agreement (Layer 2) |
| `mozartpay contract fund-asset` | Fund and set asset for an agreement (Layer 3) |
| `mozartpay contract execute` | Execute an agreement (Layer 5) |
| `mozartpay contract settle` | Settle an agreement |

### Integrations

| Command | Description |
|---------|-------------|
| `mozartpay integrations list` | List all integrations |
| `mozartpay integrations ping` | Health-check integrations |
| `mozartpay integrations score` | Add an OA score to latest asset |
| `mozartpay integrations carbon` | StellarCarbon management |

### Reporting

| Command | Description |
|---------|-------------|
| `mozartpay report generate` | Post-transaction compliance report |
| `mozartpay report iso20022` | Export ISO 20022 pacs.008 XML |

### AI

| Command | Description |
|---------|-------------|
| `mozartpay mcp` | Start MCP server for AI assistant integration (stdio/SSE) |
| `mozartpay chat` | AI chat interface |
| `mozartpay terminal` | Interactive terminal mode |

### System

| Command | Description |
|---------|-------------|
| `mozartpay init` | Initialize configuration |
| `mozartpay status` | Session and system status |
| `mozartpay flow` | Print component flow diagram |
| `mozartpay network` | Network management |
| `mozartpay version` | Print version |

## Soroban Contract Deployment

MozartPay includes a pure-Go Soroban RPC client — no external `stellar` CLI required.

### Deploy a Contract

```bash
# Deploy with deployer as constructor owner (default)
mozartpay contract deploy --wasm contracts/dist/mozartpay_contracts.wasm --network stellar-testnet

# Deploy with specific owner
mozartpay contract deploy --wasm my_contract.wasm --network stellar-testnet --owner GBXXXX...

# Deploy with custom constructor args (base64 XDR ScVal)
mozartpay contract deploy --wasm my_contract.wasm --network stellar-mainnet \
  --constructor-args "AAAAEAAAAAEAAAAA,AAAAEAAAAAEAAAAA"
```

The deploy command:
1. Uploads WASM bytecode to Soroban network
2. Creates a contract instance from the uploaded WASM hash
3. Extracts contract ID from the simulation response
4. Saves the contract ID to config

### Simulate-Only Queries (Read-Only)

```bash
# List agreements by initiator (no transaction submitted)
mozartpay contract list

# Show agreement details
mozartpay contract show --id <agreement-id>
```

### Invoke Write Methods

```bash
# Create an agreement
mozartpay contract create-agreement --dispute-window 86400

# Attest identity
mozartpay contract attest-identity --did did:web:example.com --method web --vc-type national_id

# Connect wallet
mozartpay contract connect-wallet --type wwwallet --passkey

# Fund asset
mozartpay contract fund-asset --asset USDC --amount 100 --locked 0

# Execute agreement
mozartpay contract execute --tx-hash <settlement-hash>

# Settle agreement
mozartpay contract settle
```

### Networks

| Network | RPC URL | Passphrase |
|---------|---------|------------|
| Testnet | `https://soroban-testnet.stellar.org` | Stellar Testnet |
| Mainnet | `https://soroban-mainnet.stellar.org` | Stellar Public Network |

## MCP Server

MozartPay includes a built-in MCP (Model Context Protocol) server for AI assistant integration.

### Starting the MCP Server

```bash
# stdio transport (for direct AI integration, e.g., with Claude/Cursor)
mozartpay mcp

# SSE transport over HTTP
mozartpay mcp --transport sse --port 3000

# Verbose logging
mozartpay mcp --verbose
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `wallet_list` | List all configured wallets |
| `wallet_balance` | Query wallet balance |
| `swap_quote` | Get asset swap quote |
| `swap_execute` | Execute asset swap |
| `pay_send` | Send payment |
| `pay_request` | Request payment |
| `asset_list` | List assets |
| `asset_trust` | Add trustline |
| `system_status` | System status |
| `system_health` | Health check |

### Docker Deployment

```bash
docker compose up mcp  # MCP server on port 3000 with SSE transport
```

## Smart Contracts

The Soroban smart contracts are written in Rust and located in `contracts/`.

- **Source**: `contracts/src/lib.rs`, `contracts/src/orchestrated_agreement.rs`
- **Architecture**: See `contracts/ARCHITECTURE.md` for the full security architecture
- **Build**: `cd contracts && make build` (produces `contracts/dist/*.wasm`)

The main contract implements the 5-layer Orchestrated Agreement flow:
1. **Identity** — DID attestation with Verifiable Credentials
2. **Wallet** — WebAuthn/passkey wallet connection
3. **Assets** — SAC/SEP-41 fungible/non-fungible asset funding
4. **Integrations** — OA Score, StellarCarbon, x402, Tempo
5. **Reporting** — On-chain audit trail, ISO 20022 compliance

## Docker & Kubernetes

### Docker

Multi-service Dockerfile supports three images:
- **CLI** — MozartPay CLI binary
- **MCP** — MCP server with SSE transport (port 3000)
- **WebAuthn** — WebAuthn/FIDO2 server (port 8000)

```bash
docker compose up              # All services
docker compose up mcp          # MCP server only
docker compose up webauthn     # WebAuthn server only
```

### Kubernetes

Kubernetes manifests in `k8s/` include:
- Namespace, PostgreSQL, Redis, WebAuthn, MCP, Horizon, Networking, Monitoring, Ollama

See `k8s/README.md` for detailed deployment instructions.

## Standards Compliance

- **W3C DID Core 1.0** — Decentralized Identifiers
- **W3C Verifiable Credentials** — VC Data Model 2.0
- **EBSI v3** — EU Blockchain Services Infrastructure
- **SEP-41** — Stellar Token Interface
- **ISO 20022** — Financial messaging (pacs.008.001.14)
- **eIDAS 2.0** — EU Digital Identity Framework
- **HTTP 402 / x402** — Pay-per-use web protocol

## Configuration

Config stored in `~/.mozartpay/config.json`.
State files stored in `~/.mozartpay/state/`.

Key config fields:
- `Network` — Stellar network (`stellar-testnet` or `stellar-mainnet`)
- `ContractID` — Last deployed Soroban contract ID
- `LastAgreementID` — Most recent agreement ID
- `AgreementIDs` — All agreement IDs created

## Contributing

- Follow existing code conventions (see `AGENTS.md`)
- Run `make lint` before submitting changes
- Run `make security` for security scanning
- Test on testnet before mainnet
- No shell-out to stellar CLI — use pure-Go Soroban client

---

*Built by OG Technologies EU · Vienna, Austria*  
*Web3 · Payments · Education · Standards*
