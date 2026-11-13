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
make test               # Run all tests (both modules — see below)
make lint               # go vet + golangci-lint (both modules)
make security           # gosec security scanner (both modules)
make tidy               # go mod tidy (both modules)
go build ./...          # Build all packages (default feature set)
go build -tags extras ./...   # Build including extras-only commands
go vet ./...
```

### Two Go modules

The repo has two Go modules:

- **Root module** `github.com/stellar-go-cli/stellar-go-cli` — CLI and internals (`cmd/`, `internal/`)
- **Library module** `github.com/stellar-go-cli/stellar-go-cli/pkg` — the public `pkg/` packages (`pkg/go.mod`, deps: `stellar/go-stellar-sdk` + `piprate/json-gold` only). The root module requires a published `pkg/v*` version; local builds resolve it to `./pkg` via the committed `go.work` workspace.

Consequences:

- `go test ./...` at the root does **not** run `pkg/` tests — use `make test` or `cd pkg && go test ./...`
- **`pkg/` must never import `internal/`** — it is a standalone public library
- **Releases**: the library is versioned with `pkg/vX.Y.Z` git tags (consumers `go get ...@pkg/v0.1.0`); CLI binary releases keep `vX.Y.Z` tags
- **No `replace`/`exclude` directives in root `go.mod`** — Go rejects `go install`/`go run ...@version` for modules whose go.mod contains `replace` (it would be silently ignored outside the main module). `go.work` provides the local `./pkg` mapping instead; it only applies inside this checkout and is ignored by `@version` installs.
- **The root `pkg` require must point at a published `pkg/v*` tag** — `go install ...@latest` and external consumers resolve that version, not the local tree. Local builds use the workspace, so the two can diverge silently; see the release-ordering rule below.

Non-core command groups (`chat`, `chat_finetune`, `terminal`, `triangular`, `exchange`) live behind the `extras` build tag. Registration goes through `registerExtras` in `cmd/stellar-go-cli/commands/extras.go` (with a `!extras` stub in `extras_disabled.go`).

---

## Codebase Layout

| Path | Description |
|------|-------------|
| `cmd/stellar-go-cli/` | CLI entrypoint and command definitions |
| `cmd/stellar-go-cli/commands/` | All CLI commands (one file per command group) |
| `pkg/soroban/` | Pure-Go Soroban RPC client (deploy, invoke, simulate) — public |
| `pkg/iso20022/` | ISO 20022 pacs.008/002/004/009 XML generation — public |
| `pkg/vc/` | DID creation and VC issuance/verification — public |
| `pkg/swap/` | Stellar DEX path-payment quotes/execution — public |
| `pkg/models/` | Shared data models (Account, Payment, Asset, VC, …) — public |
| `pkg/crypto/` | Key generation, hashing, encoding helpers — public |
| `internal/mcp/` | MCP server, tools, handlers, and skills |
| `internal/wallet/` | Wallet management (Stellar, external EOA) + passkey protection |
| `internal/webauthn/` | Localhost WebAuthn relying party for passkey ceremonies |
| `internal/config/` | Config load/save (`~/.stellar-go-cli/config.json`) |
| `internal/ui/` | Terminal UI helpers (colors, formatting) — all output goes to stderr |
| `internal/assets/` | Asset creation (SEP-41 fungible/non-fungible) |
| `internal/payments/` | Payment rails (x402, Tempo FX, direct) |
| `internal/integrations/` | StellarCarbon, x402, Tempo, Alpha Vantage, Finnhub, Tansu |
| `internal/reporting/` | Compliance reports, ISO 20022 formatting |
| `internal/chat/` | AI chat interface (extras only) |
| `internal/contracts/` | Contract management helpers |
| `Dockerfile` | Multi-service Docker builds (CLI, MCP, VC API) |
| `docker-compose.yml` | Docker Compose orchestration (MCP) |

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
- `xdr.ScVal` construction uses helper functions in `pkg/soroban/helpers.go` (`ScvString`, `ScvSymbol`, `ScvBytes`, `ScvBool`, `ScvU32`, `ScvU64`, `ScvI32`, `ScvI64`, `ScvAddress`, `ScvVec`, `ScvMap`, `ScvPair`); `ToScVal` in `pkg/soroban/values.go` encodes Go structs/maps/slices (structs become ScvMap with `soroban:"name"` symbol keys; map keys sort per `CompareScVal` — identical to Rust `soroban-sdk` encoding)
- Contract events: `Client.GetEvents` wraps RPC `getEvents` (filters, topics, pagination, decoded ScVal topics/values); `EventsFromTransactionMeta` decodes events out of `InvokeResult.ResultMetaXDR` — both in `pkg/soroban/events.go`
- Contract deployment uses `HostFunctionTypeCreateContractV2` when constructor args are needed
- WASM hash and contract ID are extracted from `SimulateTransactionResponse.Results[0].ReturnValueXDR`
- Read-only queries use `SimulateOnly()` (no transaction submission, uses a dummy keypair)
- Write operations use `Invoke()` (simulates, assembles, signs, submits, polls)

### Config & State

- Config stored at `~/.stellar-go-cli/config.json` via `internal/config`
- State files in `~/.stellar-go-cli/state/`
- The legacy `~/.mozartpay` directory is migrated to `~/.stellar-go-cli` on first run (one-release migration in `config.migrateLegacyDir`)
- Seed access is split by custody:
  - **Operator CLI / services** resolve signers via `internal/signresolve` (`ForActive`, `ForAddress`, `ForSwap`, `Resolver`) — protected wallets yield a `custodian.RemoteSigner` over the daemon socket; plaintext wallets yield a local `signer.KeypairSigner`. Services accept signers (`ResolveSigner` field or signer params), never `*keypair.Full` loads; a locked protected wallet must fail, not fall back.
  - **MCP gatekeeper** never imports `internal/wallet` — it reaches wallet state/signing only through `custodian.RemoteWallet`/`RemoteSigner` (sanitized accounts; no `PrivateKey`/`PrivateKeyEnc`).
  - **Custodian daemon** (`stellar-go-cli signer`, `internal/custodian/`) holds grants in memory, enforces `state/policy.json` (E1–E5), and logs hash-chained decisions to `state/enforcement.log`. No API returns seeds. If the socket is absent, signing fails closed.
  - `wallet.ResolveSeed` remains for operator-local reads (export/backup); grants no longer live in the OS keychain. See `docs/passkeys.md`
- `cfg.ContractID` stores the last deployed contract ID

### Linting & Code Style

- `.golangci.yml` enables: `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `misspell`
- Formatters: `gci`, `gofmt` (simplify enabled)
- Shadow checking enabled in `govet`
- Test files excluded from `gosec`, `gocyclo`, `errcheck`, `dupl`, `lll`
- `gofmt -l` must return empty — CI enforces it
- Fuzz targets (when added) live in `*_test.go` in the module owning the code — `pkg/` vs root; run `go test -fuzz=FuzzX` per module. See `docs/audit/differential-fuzzing.md`

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
| `wallet_passkey` | Passkey protection: register/unlock/lock/status/disable |
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
- **Security model**: `docs/audit/agent-policy-engine.md` (policy/custody spec) and `docs/audit/mcp-signer-architecture.md` (gatekeeper/custodian separation design + roadmap) — when adding MCP tools, consider their risk tier (T0–T3) per the table there

### Docker Deployment

```bash
docker compose up mcp  # Start MCP server in Docker
```

The MCP server runs on port 3000 with SSE transport in Docker, with health checks configured.

---

## Releasing

Pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml` (GoReleaser):

- **Binaries**: archives + checksums for linux/darwin/windows on amd64/arm64 → GitHub Release (`.goreleaser.yml`)
- **Homebrew**: formula published to `stellar-go-cli/homebrew-tap` — requires a `HOMEBREW_TAP_TOKEN` repo secret (PAT with `contents:write` on the tap repo)
- **Signing**: cosign keyless `sign-blob` produces a `${artifact}.bundle` per archive/checksum; verify with `cosign verify-blob --bundle <file>.bundle --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate-identity-regexp 'github.com/stellar-go-cli' <file>`
- **SBOM**: syft CycloneDX JSON per archive (`${artifact}.sbom.json`)
- **GHCR images**: `ghcr.io/<owner>/stellar-go-cli-{mcp,cli,vc-api}:<tag>` + `:latest`, built from the matching `Dockerfile` targets and cosign-signed by digest

`pkg/vX.Y.Z` library tags intentionally do **not** trigger this workflow (`v*` filter only matches CLI release tags).

**Release ordering:** if `pkg/` changed, push its `pkg/vX.Y.Z` tag *first* and bump the root `go.mod` `pkg` require to that version before tagging the CLI release. `go install ...@latest` and `go get` resolve the published library version (the committed `go.work` is ignored outside this checkout), while release binaries/CI build the local `./pkg` — a stale require makes those two builds diverge.

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

### Live e2e tests

`make test-e2e` (or `cd pkg && go test -tags e2e ./x402/ -v -timeout 15m`) runs the
x402 end-to-end suite in `pkg/x402/e2e_test.go`. It is gated behind the `e2e`
build tag and skipped by `-short`: it generates fresh wallets, funds them via
Friendbot, deploys a real SAC, performs a full HTTP 402 challenge/payment/retry
flow, and settles on-chain through a test facilitator — real testnet
transactions, several minutes per run.

`make test-e2e-mainnet` runs the same flow on mainnet
(`pkg/x402/e2e_mainnet_test.go`). It requires `X402_MAINNET_SEED` set to the
secret of a funded mainnet account (skipped otherwise) and **spends real XLM**
— a few XLM of account reserves, mostly recovered because the fresh
payer/merchant accounts are merged back into the funder on cleanup. The seed
is read from the environment only, never logged or persisted.

`make test-e2e-cli` (`scripts/e2e_testnet.sh`) is a shell e2e for the CLI
binary itself: it runs in a throwaway `HOME`, creates a fresh testnet wallet,
funds it via Friendbot, submits a real 1 XLM self-payment, and asserts on the
offline/read-only surfaces (init/status, `did create`/`did attest`,
`wallet balance/show/list`, `report generate`/`iso20022`, `swap quote`,
`network show`). Exits non-zero on failure; `--offline` runs only the
network-free steps. It never touches the real `~/.stellar-go-cli`.

`make test-e2e-cli-ext` (`scripts/e2e_extended.sh`) covers the remaining
surface in the same sandboxed style, 17 sections: `version`/`flow`/`network
set`; DID create (key/ethr/ebsi/web), `did attest`/`did verify` roundtrips on
both Ed25519 and JWS/ES256 proofs, tamper rejection, and `did show`; `pay
rails`/`pay quote`/`pay x402` arg checks; all ISO 20022 message types
(pacs.008/002/004/009, pain.001/002, camt.053/054); `audit
verify`/`events`/`latency`/`inventory`/`evidence`; the MCP server over both
stdio JSON-RPC and SSE (`initialize`/`tools/list`/`tools/call`); wallet
registry ops (switch/rename/export/remove/import round-trip), wallet-key
attestation, `passkey status`/`list`, and `mandate` fail-closed paths; real
SEP-41 SAC deploys via `asset create-ft`/`create-nfa`, carbon-credit attach,
and the `asset show` state checks; trustline lifecycle (`asset
trust`/`untrust`); live `swap execute` and `swap zk` path payments; claimable
balance create/list/accept/decline (creation uses the
`scripts/e2e_helpers/create_claimable` helper since the CLI has no create
command); a real `contract deploy` + `contract invoke --simulate` against the
Soroban testnet using the committed `scripts/testdata/e2e_hello.wasm` fixture
(`git add -f`'d past the `*.wasm` gitignore — a 626-byte hello_world built
with `stellar contract build`); `pay zk` settlement; integrations list/ping
and unconfigured-provider status paths; trade strategy records; `network
set`/`switch`; and the legacy `~/.mozartpay` migration. Takes several
minutes — it funds wallets and submits real testnet transactions.
`--no-contract` skips the deploy/invoke section; `--offline` runs only the
network-free steps.

---

## Module Path

```
github.com/stellar-go-cli/stellar-go-cli
```

Go 1.26+. Primary external dependency: `github.com/stellar/go` (Stellar Go SDK with Soroban RPC support).
