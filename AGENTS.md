# AGENTS.md — MozartPay CLI

Guidelines for AI coding agents and documentation of the built-in MCP server for AI assistant integration.

---

## Project Overview

MozartPay CLI is a Go command-line interface for the MozartPay Orchestrated Agreements platform — enabling DID-attested, VC-linked payments and asset issuance on Stellar with OA scoring, StellarCarbon offset integration, x402 micropayments, and ISO 20022 compliance reporting. It includes a pure-Go Soroban RPC client for smart contract deployment and invocation, and a built-in MCP server for AI assistant integration.

---

## Build & Test Commands

```bash
make build              # Build CLI binary → dist/mozartpay
make install            # Install to $GOPATH/bin
make test               # Run all tests
make lint               # go vet + golangci-lint
make security           # gosec security scanner
go build ./...          # Build all packages
go vet ./...            # Vet all packages
go mod tidy             # Tidy modules
```

---

## Codebase Layout

| Path | Description |
|------|-------------|
| `cmd/mozartpay/` | CLI entrypoint and command definitions |
| `cmd/mozartpay/commands/` | All CLI commands (one file per command group) |
| `cmd/webauthn-server/` | WebAuthn/FIDO2 server for passkey authentication |
| `internal/soroban/` | Pure-Go Soroban RPC client (deploy, invoke, simulate) |
| `internal/mcp/` | MCP server, tools, handlers, and skills |
| `internal/wallet/` | Wallet management (Stellar, wwWallet, EOA) |
| `internal/did/` | DID creation and VC attestation |
| `internal/models/` | Data models (Account, Config, etc.) |
| `internal/config/` | Config load/save (`~/.mozartpay/config.json`) |
| `internal/ui/` | Terminal UI helpers (colors, formatting) |
| `internal/assets/` | Asset creation (SEP-41 fungible/non-fungible) |
| `internal/payments/` | Payment rails (x402, Tempo FX, direct) |
| `internal/integrations/` | OA Score, StellarCarbon, x402, Tempo |
| `internal/reporting/` | Compliance reports, ISO 20022 pacs.008 |
| `internal/chat/` | AI chat interface |
| `internal/contracts/` | Contract management helpers |
| `contracts/` | Soroban smart contracts in Rust |
| `contracts/src/` | Rust contract source code |
| `contracts/dist/` | Compiled `.wasm` binaries |
| `k8s/` | Kubernetes deployment manifests |
| `Dockerfile` | Multi-service Docker builds (CLI, MCP, WebAuthn) |
| `docker-compose.yml` | Docker Compose orchestration |

---

## Architecture Conventions

### Command Pattern

All CLI commands follow the `Command` struct pattern defined in `cmd/mozartpay/commands/root.go`:

- Each command group is a file in `cmd/mozartpay/commands/` (e.g., `wallet.go`, `contract.go`)
- Each file defines a `newXxxCmd(cfg *config.Config) *Command` function
- Subcommands are added via `cmd.addSub(newXxxSubCmd(cfg))`
- Commands are registered in `NewRootCmd()` in `root.go`
- Flags use the standard `flag.FlagSet` package

### UI Output

- Use `internal/ui` package for all terminal output (`ui.Info`, `ui.Success`, `ui.Error`, `ui.Warn`, `ui.Header`, `ui.SectionLabel`)
- Do not use raw `fmt.Println` in command files — the `ui` package provides consistent formatting and color

### Soroban Interactions

- All Soroban RPC interactions use the pure-Go client in `internal/soroban/`
- **No shell-out to the stellar CLI** — everything is done via the Stellar Go SDK and Soroban RPC
- `xdr.ScVal` construction uses helper functions in `internal/soroban/helpers.go` (`ScvString`, `ScvSymbol`, `ScvBytes`, `ScvBool`, `ScvU32`, `ScvU64`, `ScvI32`, `ScvI64`, `ScvAddress`)
- Contract deployment uses `HostFunctionTypeCreateContractV2` when constructor args are needed
- WASM hash and contract ID are extracted from `SimulateTransactionResponse.Results[0].ReturnValueXDR`
- Read-only queries use `SimulateOnly()` (no transaction submission, uses a dummy keypair)
- Write operations use `Invoke()` (simulates, assembles, signs, submits, polls)

### Config & State

- Config stored at `~/.mozartpay/config.json` via `internal/config`
- State files in `~/.mozartpay/state/`
- Active wallet's `PrivateKey` (Stellar seed `S...`) is used with `keypair.ParseFull()` for signing
- `cfg.ContractID` stores the last deployed contract ID
- `cfg.LastAgreementID` stores the most recent agreement ID

### Linting & Code Style

- `.golangci.yml` enables: `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `misspell`
- Formatters: `gci`, `gofmt` (simplify enabled)
- Shadow checking enabled in `govet`
- Test files excluded from `gosec`, `gocyclo`, `errcheck`, `dupl`, `lll`

---

## MCP Server Integration

MozartPay includes a built-in MCP (Model Context Protocol) server that exposes CLI functionality as tools for AI assistants.

### Starting the MCP Server

```bash
mozartpay mcp                              # stdio transport (default)
mozartpay mcp --transport sse --port 3000 # SSE transport over HTTP
mozartpay mcp --verbose                    # Enable verbose logging
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

### MCP Architecture

- **Server**: `internal/mcp/server.go` — JSON-RPC server with stdio/SSE transport
- **Tools**: `internal/mcp/tools.go` — Tool definitions and schemas
- **Handlers**: `internal/mcp/handlers.go` — Tool execution handlers
- **Parameter Collection**: `internal/mcp/parameter_collection.go` — Interactive parameter gathering
- **Skills**: `internal/mcp/skills/` — Skill definitions for AI assistants

### Docker Deployment

```bash
docker compose up mcp  # Start MCP server in Docker
```

The MCP server runs on port 3000 with SSE transport in Docker, with health checks configured.

---

## What to Avoid

- **No shell-out to stellar CLI** for Soroban operations — use the pure-Go `internal/soroban/` package
- **No hardcoded deployment** to MozartPay-specific contracts only — the deploy command must support any `.wasm` file
- **No raw `fmt.Println`** in command files — use `internal/ui` package for consistent output
- **No external dependencies** unless absolutely necessary — the project aims for minimal direct deps
- **No hardcoded private keys, API keys, or secrets** in source code
- **No skipping simulation** — all Soroban transactions must be simulated before submission

---

## Testing on Testnet

```bash
# RPC connectivity check
curl -s -X POST https://soroban-testnet.stellar.org \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}'

# Deploy a contract
mozartpay contract deploy --wasm contracts/dist/mozartpay_contracts.wasm --network stellar-testnet

# Simulate a read-only query (no transaction submitted)
mozartpay contract list
mozartpay contract show --id <agreement-id>

# Invoke a write method (submits transaction)
mozartpay contract create-agreement --dispute-window 86400
```

### Testnet Details

- **RPC URL**: `https://soroban-testnet.stellar.org`
- **Network Passphrase**: Stellar Testnet (`network.TestNetworkPassphrase`)
- **Explorer**: `https://stellar.expert/explorer/testnet/contract/<contract-id>`
- **Faucet**: `mozartpay wallet fund --network stellar-testnet`

---

## Smart Contracts

The Soroban smart contracts are written in Rust and located in `contracts/`.

- **Source**: `contracts/src/lib.rs`, `contracts/src/orchestrated_agreement.rs`
- **Architecture**: See `contracts/ARCHITECTURE.md` for the full security architecture
- **Build**: `cd contracts && make build` (produces `contracts/dist/*.wasm`)
- **Contract ID Preimage**: Uses `ContractIdPreimageFromAddress` with a random salt
- **Constructor**: The MozartPay contract's `__constructor(owner: Address)` requires an owner address argument

---

## Module Path

```
github.com/ogtechnologies/mozartpay
```

Go 1.24+. Primary external dependency: `github.com/stellar/go` (Stellar Go SDK with Soroban RPC support).
