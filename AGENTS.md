# AGENTS.md — Stellar Go CLI

Guidelines for AI coding agents and documentation of the built-in MCP server for AI assistant integration.

---

## Project Overview

Stellar Go CLI is an open-source Go command-line interface for the Stellar network — wallets, payments, DEX swaps, SEP-41 asset issuance, W3C DIDs and Verifiable Credentials, ISO 20022 compliance reporting, a pure-Go Soroban RPC client for smart contract deployment/invocation, and a built-in MCP server for AI assistant integration.

The reusable building blocks are public Go packages under `pkg/` (importable by other projects); the CLI itself lives under `cmd/` with application internals under `internal/`.

---

## Build & Test Commands

```bash
make build              # Build CLI binary → dist/stellar-go-cli
make install            # Install to $GOPATH/bin
make test               # Run all tests
make lint               # go vet + golangci-lint
make security           # gosec security scanner
go build ./...          # Build all packages (default feature set)
go build -tags extras ./...   # Build including extras-only commands
go vet ./...
go mod tidy
```

Non-core command groups (`chat`, `chat_finetune`, `terminal`, `triangular`, `exchange`) live behind the `extras` build tag. Registration goes through `registerExtras` in `cmd/stellar-go-cli/commands/extras.go` (with a `!extras` stub in `extras_disabled.go`).

---

## Codebase Layout

| Path | Description |
|------|-------------|
| `cmd/stellar-go-cli/` | CLI entrypoint and command definitions |
| `cmd/stellar-go-cli/commands/` | All CLI commands (one file per command group) |
| `cmd/webauthn-server/` | WebAuthn/FIDO2 server for passkey authentication |
| `pkg/soroban/` | Pure-Go Soroban RPC client (deploy, invoke, simulate) — public |
| `pkg/iso20022/` | ISO 20022 pacs.008/002/004/009 XML generation — public |
| `pkg/vc/` | DID creation and VC issuance/verification — public |
| `pkg/swap/` | Stellar DEX path-payment quotes/execution — public |
| `pkg/models/` | Shared data models (Account, Payment, Asset, VC, …) — public |
| `pkg/crypto/` | Key generation, hashing, encoding helpers — public |
| `internal/mcp/` | MCP server, tools, handlers, and skills |
| `internal/wallet/` | Wallet management (Stellar, wwWallet, EOA) |
| `internal/config/` | Config load/save (`~/.stellar-go-cli/config.json`) |
| `internal/ui/` | Terminal UI helpers (colors, formatting) — all output goes to stderr |
| `internal/assets/` | Asset creation (SEP-41 fungible/non-fungible) |
| `internal/payments/` | Payment rails (x402, Tempo FX, direct) |
| `internal/integrations/` | StellarCarbon, x402, Tempo, Alpha Vantage, Finnhub, Tansu |
| `internal/reporting/` | Compliance reports, ISO 20022 formatting |
| `internal/chat/` | AI chat interface (extras only) |
| `internal/contracts/` | Contract management helpers |
| `Dockerfile` | Multi-service Docker builds (CLI, MCP, WebAuthn, VC API) |
| `docker-compose.yml` | Docker Compose orchestration (MCP + WebAuthn) |

---

## Architecture Conventions

### Command Pattern

All CLI commands follow the `Command` struct pattern defined in `cmd/stellar-go-cli/commands/root.go`:

- Each command group is a file in `cmd/stellar-go-cli/commands/` (e.g., `wallet.go`, `contract.go`)
- Each file defines a `newXxxCmd(cfg *config.Config) *Command` function
- Subcommands are added via `cmd.addSub(newXxxSubCmd(cfg))`
- Commands are registered in `NewRootCmd()` in `root.go`
- Flags use the standard `flag.FlagSet` package

### UI Output

- Use `internal/ui` package for all terminal chrome (`ui.Info`, `ui.Success`, `ui.Error`, `ui.Warn`, `ui.Header`, `ui.SectionLabel`)
- `internal/ui` writes to **stderr** so command output can be piped/redirected (`stellar-go-cli report iso20022 > out.xml` produces clean XML)
- Actual data output (XML, JSON, results) uses `fmt.Println` on stdout
- Do not use raw `fmt.Println` for status/progress messages in command files

### Soroban Interactions

- All Soroban RPC interactions use the pure-Go client in `pkg/soroban/`
- **No shell-out to the stellar CLI** — everything is done via the Stellar Go SDK and Soroban RPC
- `xdr.ScVal` construction uses helper functions in `pkg/soroban/helpers.go` (`ScvString`, `ScvSymbol`, `ScvBytes`, `ScvBool`, `ScvU32`, `ScvU64`, `ScvI32`, `ScvI64`, `ScvAddress`)
- Contract deployment uses `HostFunctionTypeCreateContractV2` when constructor args are needed
- WASM hash and contract ID are extracted from `SimulateTransactionResponse.Results[0].ReturnValueXDR`
- Read-only queries use `SimulateOnly()` (no transaction submission, uses a dummy keypair)
- Write operations use `Invoke()` (simulates, assembles, signs, submits, polls)

### Config & State

- Config stored at `~/.stellar-go-cli/config.json` via `internal/config`
- State files in `~/.stellar-go-cli/state/`
- The legacy `~/.mozartpay` directory is migrated to `~/.stellar-go-cli` on first run (one-release migration in `config.migrateLegacyDir`)
- Active wallet's `PrivateKey` (Stellar seed `S...`) is used with `keypair.ParseFull()` for signing
- `cfg.ContractID` stores the last deployed contract ID

### Linting & Code Style

- `.golangci.yml` enables: `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `misspell`
- Formatters: `gci`, `gofmt` (simplify enabled)
- Shadow checking enabled in `govet`
- Test files excluded from `gosec`, `gocyclo`, `errcheck`, `dupl`, `lll`
- `gofmt -l` must return empty — CI enforces it

---

## MCP Server Integration

The CLI includes a built-in MCP (Model Context Protocol) server that exposes functionality as tools for AI assistants.

### Starting the MCP Server

```bash
stellar-go-cli mcp                              # stdio transport (default)
stellar-go-cli mcp --transport sse --port 3000  # SSE transport over HTTP
stellar-go-cli mcp --verbose                    # Enable verbose logging
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

- **No shell-out to stellar CLI** for Soroban operations — use the pure-Go `pkg/soroban/` package
- **No hardcoded contract-specific commands** — `contract deploy`/`invoke` must stay generic (any `.wasm`, any method)
- **No raw `fmt.Println`** for status/progress in command files — use `internal/ui` (stderr) so stdout stays machine-readable
- **No external dependencies** unless absolutely necessary — the project aims for minimal direct deps
- **No hardcoded private keys, API keys, or secrets** in source code
- **No skipping simulation** — all Soroban transactions must be simulated before submission
- **No product/brand names** (downstream-consumer-specific naming, endpoints, or defaults) in this repository — it is a public good; downstream projects configure their own values

---

## Testing on Testnet

```bash
# RPC connectivity check
curl -s -X POST https://soroban-testnet.stellar.org \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}'

# Deploy a contract
stellar-go-cli contract deploy --wasm my_contract.wasm --network stellar-testnet

# Simulate a read-only query (no transaction submitted)
stellar-go-cli contract invoke --id <contract-id> --fn get_state --simulate

# Invoke a write method (submits transaction)
stellar-go-cli contract invoke --id <contract-id> --fn set_state --args <base64-xdr-scval>
```

### Testnet Details

- **RPC URL**: `https://soroban-testnet.stellar.org`
- **Network Passphrase**: Stellar Testnet (`network.TestNetworkPassphrase`)
- **Explorer**: `https://stellar.expert/explorer/testnet/contract/<contract-id>`
- **Faucet**: `stellar-go-cli wallet fund --network stellar-testnet` (Friendbot)

---

## Module Path

```
github.com/stellar-go-cli/stellar-go-cli
```

Go 1.26+. Primary external dependency: `github.com/stellar/go` (Stellar Go SDK with Soroban RPC support).
