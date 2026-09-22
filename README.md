# Stellar Go CLI

> Open-source Go CLI for Stellar: wallets, payments, swaps, SEP-41 assets, W3C DIDs/Verifiable Credentials, ISO 20022 (pacs/pain/camt) reporting, a pure-Go Soroban RPC client, and an MCP server for AI assistants. Apache 2.0.

```bash
go install github.com/stellar-go-cli/stellar-go-cli/cmd/stellar-go-cli@latest
stellar-go-cli --help
```

**Status:** wallet/payments/swaps run against mainnet and testnet. DID/VC issuance (Ed25519Signature2020 proofs, cryptographic verification) and ISO 20022 pacs.008/002/004/009 export (validated against the official XSDs) are working prototypes — VCs are self-issued unless an external issuer is configured. This is not a Stellar Development Foundation project.

The codebase was originally developed as part of a downstream commercial product and was extracted into this standalone public repository. [MozartPay Orchestrated Agreements](https://github.com/mozartpay/OAs) is the first downstream consumer.

## Features

- **Wallets** — Create and manage Stellar wallets, import seeds, testnet faucet funding
- **Payments** — Direct Stellar payments, x402 pay-per-use flow, FX quotes
- **Swaps** — Path-payment quotes and execution on the Stellar DEX
- **Assets** — SEP-41 fungible tokens and non-fungible assets (SAC), trustlines, claimable balances, liquidity pools
- **DIDs & VCs** — Create `did:web`/`did:key`/`did:ethr`/`did:ebsi` documents; issue and verify W3C Verifiable Credentials
- **Soroban contracts** — Deploy any `.wasm` contract and invoke methods — pure Go, no `stellar` CLI required
- **ISO 20022 reporting** — Export transactions as `pacs.008`/`pacs.002`/`pacs.004`/`pacs.009` XML; `pkg/iso20022` also builds batch `pain.001` disbursement initiations and `camt.054` settlement notifications
- **MCP server** — Expose CLI functionality as tools for AI assistants over stdio or SSE
- **WebAuthn server** — Standalone FIDO2/passkey server (`cmd/webauthn-server`)

## Requirements

- Go 1.26+
- Docker & Docker Compose (optional, for the MCP and WebAuthn services)

## Build & Install

```bash
make build              # → dist/stellar-go-cli
make install            # → $(GOPATH)/bin/stellar-go-cli
make test               # run all tests
make lint               # go vet + golangci-lint
```

Optional command groups (chat, chat_finetune, terminal, triangular, exchange) are compiled with the `extras` build tag:

```bash
go build -tags extras -o dist/stellar-go-cli ./cmd/stellar-go-cli
```

## Quick Start

```bash
# Initialize configuration (~/.stellar-go-cli/config.json)
stellar-go-cli init

# Create a DID and issue a verifiable credential
stellar-go-cli did create --method key
stellar-go-cli did attest --method ebsi --name "Your Name" --country AT

# Create and fund a testnet wallet
stellar-go-cli wallet connect --provider stellar --network stellar-testnet
stellar-go-cli wallet fund --network stellar-testnet

# Deploy a Soroban contract
stellar-go-cli contract deploy --wasm my_contract.wasm --network stellar-testnet

# Make a payment and export an ISO 20022 report
stellar-go-cli pay send --to <address> --amount 10 --asset USDC
stellar-go-cli report iso20022 --type pacs.008 > report.xml
```

## Commands

| Group | Commands |
|-------|----------|
| `did` | `create`, `attest`, `verify`, `show` |
| `wallet` | `connect`, `fund`, `balance`, `list`, `switch`, `passkey`, … |
| `pay` | `send`, `quote`, `x402`, … |
| `swap` | `quote`, `execute`, `zk`, `arbitrage`, `scan`, `monitor`, `assets` (+ `triangular` with `-tags extras`) |
| `pool` | Liquidity pool operations |
| `trade` | Trading strategies (dca, grid, momentum, …) |
| `asset` | `create-ft`, `create-nfa`, `trust`, `untrust`, `carbon`, `show` |
| `claimable` | Claimable balance operations |
| `contract` | `deploy`, `invoke` (`--simulate` for read-only), `set` |
| `integrations` | `list`, `ping`, `carbon`, `news`, `finnhub`, `tansu` |
| `report` | `generate`, `show`, `iso20022` |
| `mcp` | MCP server (stdio/SSE) |
| `vc-api` | W3C VC API test server (`/credentials/issue`, `/credentials/verify`) |
| `chat`, `terminal`, `exchange` | extras build tag only |
| `init`, `status`, `flow`, `network`, `version` | system |

## Go library

Reusable packages are public under `pkg/` and render on [pkg.go.dev](https://pkg.go.dev/github.com/stellar-go-cli/stellar-go-cli):

| Package | Description |
|---------|-------------|
| `pkg/soroban` | Pure-Go Soroban RPC client: upload WASM, deploy, invoke, simulate |
| `pkg/iso20022` | ISO 20022 XML generation — pacs.008/002/004/009, batch pacs.008/002, pain.001, camt.054 |
| `pkg/vc` | W3C DID creation and Verifiable Credential issuance/verification |
| `pkg/swap` | Stellar DEX path-payment quoting and execution |
| `pkg/models` | Shared types (Payment, Asset, DIDDocument, VerifiableCredential, …) |
| `pkg/crypto` | Key generation, hashing, and encoding helpers |

```go
import (
	"github.com/stellar-go-cli/stellar-go-cli/pkg/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
)

// Soroban RPC client — deploy/invoke contracts without the stellar CLI
client := soroban.NewClientForNetwork("stellar-testnet")
defer client.Close()

result, err := client.Deploy(ctx, keypair, wasmBytes) // *DeployResult{ContractID, TxHash, …}

// ISO 20022 pacs.008 XML from a payment
xmlDoc, err := iso20022.BuildPacs008(&models.Payment{
	ID:     "pay-001",
	From:   "GDEBTOR…",
	To:     "GCREDITOR…",
	Amount: "10.0000000",
	Asset:  "USDC",
	TxHash: "abc123…",
}, nil)

// ISO 20022 pain.001 disbursement initiation — one PmtInf, N receivers.
// Creditor carries receiver identity (name, phone/email, wallet proxy);
// NbOfTxs/CtrlSum are computed and asset issuers are preserved.
batch, err := iso20022.BuildPain001([]*iso20022.CreditTransferInstruction{
	{Payment: &models.Payment{ID: "pay-001", To: "GRCVR1…", Amount: "10", Asset: "USDC"},
		Creditor: &iso20022.Party{Name: "Receiver One", Phone: "+221-771234567"}},
	{Payment: &models.Payment{ID: "pay-002", To: "GRCVR2…", Amount: "20", Asset: "USDC"},
		Creditor: &iso20022.Party{AcctProxyID: "wallet.handle", AcctProxyTp: "WALLET"}},
}, &iso20022.Pain001Options{
	InitiatingParty: &iso20022.Party{Name: "Relief Org"},
	Debtor:          &iso20022.Party{AcctID: "GORG…", AgentBIC: "DEUTDEFF"},
})
```

## MCP Server

```bash
stellar-go-cli mcp                            # stdio transport
stellar-go-cli mcp --transport sse --port 3000 # SSE over HTTP
```

Tools include `wallet_list`, `wallet_balance`, `swap_quote`, `swap_execute`, `pay_send`, `pay_request`, `asset_list`, `asset_trust`, `system_status`, `system_health`, plus Tansu governance queries.

## Docker

```bash
docker compose up mcp        # MCP server on :3000 (SSE)
docker compose up webauthn   # WebAuthn server on :8000
```

## Configuration

Config is stored at `~/.stellar-go-cli/config.json`; state files under `~/.stellar-go-cli/state/`. On first run the legacy `~/.mozartpay` directory (from before the extraction) is migrated automatically.

## Standards

- **W3C DID Core 1.0** / **VC Data Model 2.0** (Ed25519Signature2020, JWS)
- **EBSI v3** DID method support
- **SEP-41** Stellar token interface (SAC)
- **ISO 20022** — pacs.008.001.14, pacs.002.001.16, pacs.004.001.15, pacs.009.001.13, pain.001.001.13, camt.054.001.14
- **x402 / HTTP 402** payment flow

## Downstream users

- [MozartPay Orchestrated Agreements](https://github.com/mozartpay/OAs) — orchestrated-agreement payments on Stellar; consumes the `pkg/` libraries and wraps the CLI.

## Known limitations

- **Self-issued VCs are not KYC** — credentials are self-issued (they prove key control, not verified identity) unless an external issuer is configured.
- **Identity trust anchor is off-chain** — DID/VC issuance and verification resolve off-chain; nothing is anchored to the Stellar ledger.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md). Run `make lint` and `make test` before opening a PR.

## License

[Apache 2.0](LICENSE)
