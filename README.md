# Stellar Go CLI

> Open-source Go CLI for Stellar: wallets, payments, swaps, SEP-41 assets, W3C DIDs/Verifiable Credentials, ISO 20022 (pacs/pain/camt) reporting, a pure-Go Soroban RPC client, and an MCP server for AI assistants. Apache 2.0.

```bash
go install github.com/stellar-go-cli/stellar-go-cli/cmd/stellar-go-cli@latest
stellar-go-cli --help
```

## Install

```bash
# Homebrew
brew install stellar-go-cli/tap/stellar-go-cli

# go install
go install github.com/stellar-go-cli/stellar-go-cli/cmd/stellar-go-cli@latest
```

(`go install` needs a release after v0.6.0 — v0.6.0's go.mod contained a `replace` directive, which Go rejects for `@version` installs. On v0.6.0 use Homebrew above, or `git clone` + `make install`.)

Prebuilt binaries for linux/darwin/windows (amd64/arm64) are attached to every
[GitHub Release](https://github.com/stellar-go-cli/stellar-go-cli/releases)
with checksums, a CycloneDX SBOM, and cosign signature bundles:

```bash
cosign verify-blob --bundle stellar-go-cli_<ver>_checksums.txt.bundle \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'github.com/stellar-go-cli' \
  stellar-go-cli_<ver>_checksums.txt
```

Container images are published to GHCR as `ghcr.io/stellar-go-cli/stellar-go-cli-{cli,mcp,vc-api}` — see [Docker](#docker).

**Status:** wallet/payments/swaps run against mainnet and testnet. DID/VC issuance (Ed25519Signature2020 and ES256 JsonWebSignature2020 proofs, cryptographic verification) and ISO 20022 pacs.008/002/004/009 export (validated against the official XSDs) are working prototypes — VCs are self-issued unless an external issuer is configured. This is not a Stellar Development Foundation project.

The codebase was originally developed as part of a downstream commercial product and was extracted into this standalone public repository. [MozartPay Orchestrated Agreements](https://github.com/mozartpay/OAs) is the first downstream consumer.

## Features

- **Wallets** — Create and manage Stellar wallets, import seeds, testnet faucet funding, optional passkey (WebAuthn) protection that encrypts the seed at rest and gates signing behind a biometric/hardware assertion ([docs/passkeys.md](docs/passkeys.md))
- **Payments** — Direct Stellar payments, x402 pay-per-use flow, FX quotes
- **Swaps** — Path-payment quotes and execution on the Stellar DEX
- **Assets** — SEP-41 fungible tokens and non-fungible assets (SAC), trustlines, claimable balances, liquidity pools
- **DIDs & VCs** — Create `did:web`/`did:key`/`did:ethr`/`did:ebsi` documents and issue and cryptographically verify W3C Verifiable Credentials (Ed25519Signature2020 for Ed25519 issuers, ES256 JsonWebSignature2020 for ECDSA issuers; `did:ethr`/`did:ebsi` embed the key and are locally derived)
- **Soroban contracts** — Deploy any `.wasm` contract and invoke methods — pure Go, no `stellar` CLI required
- **ISO 20022 reporting** — Export transactions as `pacs.008`/`pacs.002`/`pacs.004`/`pacs.009` XML; `pkg/iso20022` also builds batch `pain.001` disbursement initiations, `pain.002` status reports, `camt.053` statements, and `camt.054` settlement notifications
- **MCP server** — Expose CLI functionality as tools for AI assistants over stdio or SSE

## Requirements

- Go 1.26+
- Docker & Docker Compose (optional, for the MCP service)

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
stellar-go-cli did attest --name "Your Name" --country AT

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
| `wallet` | `connect`, `fund`, `balance`, `list`, `switch`, `passkey` (`register`/`unlock`/`lock`/`status`/`disable`), … |
| `pay` | `send`, `quote`, `x402`, … |
| `swap` | `quote`, `execute`, `zk`, `arbitrage`, `scan`, `monitor`, `assets` (+ `triangular` with `-tags extras`) |
| `pool` | Liquidity pool operations |
| `trade` | Trading strategies (arbitrage, mean_reversion, momentum, grid_trading, dca, breakout, scalping) |
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
| `pkg/soroban` | Pure-Go Soroban RPC client: upload WASM, deploy, invoke, simulate, query contract events (`getEvents`) |
| `pkg/iso20022` | ISO 20022 XML generation — pacs.008/002/004/009, batch pacs.008/002, pain.001/002, camt.053/054 |
| `pkg/vc` | W3C DID creation and Verifiable Credential issuance/verification; local resolution for `did:key`, `did:ebsi` and public-key-form `did:ethr`, `did:web` over HTTPS |
| `pkg/swap` | Stellar DEX path-payment quoting and execution |
| `pkg/x402` | x402 v2 HTTP 402 pay-per-use protocol for Stellar (client + facilitator) |
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

Tools include `wallet_list`, `wallet_balance`, `wallet_passkey`, `swap_quote`, `swap_execute`, `pay_send`, `pay_request`, `asset_list`, `asset_trust`, `system_status`, `system_health`, plus Tansu governance queries.

Security model and roadmap for separating the MCP server from key custody: [docs/audit/agent-policy-engine.md](docs/audit/agent-policy-engine.md) (spec) and [docs/audit/mcp-signer-architecture.md](docs/audit/mcp-signer-architecture.md) (implementation).

Verification roadmaps: [docs/audit/differentialfuzzing/differential-fuzzing.md](docs/audit/differentialfuzzing/differential-fuzzing.md) (differential fuzzing vs the Rust `stellar` CLI) and [docs/audit/specula/specula-tla-formal-verification.md](docs/audit/specula/specula-tla-formal-verification.md) (TLA+ model checking via Specula). Security assessment documentation: [docs/audit/](docs/audit/).

## Docker

```bash
# Published images (built and cosign-signed on every release tag)
docker pull ghcr.io/stellar-go-cli/stellar-go-cli-mcp:latest
docker run -p 3000:3000 ghcr.io/stellar-go-cli/stellar-go-cli-mcp:latest

# Or pull via compose (docker compose up --build still builds from source)
docker compose pull mcp
docker compose up mcp
```

Published images: `stellar-go-cli-mcp` (SSE server, :3000), `stellar-go-cli-cli`, `stellar-go-cli-vc-api` (:4000).

## Configuration

Config is stored at `~/.stellar-go-cli/config.json`; state files under `~/.stellar-go-cli/state/`. On first run the legacy `~/.mozartpay` directory (from before the extraction) is migrated automatically.

## Standards

- **W3C DID Core 1.0** / **VC Data Model 2.0** (Ed25519Signature2020, JsonWebSignature2020/ES256)
- **EBSI v3** did:ebsi identifier format (key-embedded, locally derived — no registry anchoring)
- **SEP-41** Stellar token interface (SAC)
- **ISO 20022** — pacs.008.001.14, pacs.002.001.16, pacs.004.001.15, pacs.009.001.13, pain.001.001.13, pain.002.001.10, camt.053.001.13, camt.054.001.14
- **x402 / HTTP 402** payment flow

## Downstream users

- [MozartPay Orchestrated Agreements](https://github.com/mozartpay/OAs) — orchestrated-agreement payments on Stellar; consumes the `pkg/` libraries and wraps the CLI.

## Known limitations

- **Self-issued VCs are not KYC** — credentials are self-issued (they prove key control, not verified identity) unless an external issuer is configured.
- **Identity trust anchor is off-chain** — DID/VC issuance and verification resolve off-chain; nothing is anchored to the Stellar ledger.
- **`did:ethr`/`did:ebsi` are locally derived identifier formats** — they embed the public key and verify cryptographically (ES256 JWS), but are never registered on Ethereum or EBSI, so they prove key control, not registry-backed identity.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md). Run `make lint` and `make test` before opening a PR.

## License

[Apache 2.0](LICENSE)
