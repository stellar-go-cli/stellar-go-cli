# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x (latest) | ✅ |
| < 0.1.0 | ❌ |

## Reporting a Vulnerability

If you discover a security vulnerability in Stellar Go CLI, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Email **hi@ogtechnologies.co** with a description of the vulnerability, reproduction steps, and potential impact
3. Include the relevant component (CLI, Soroban client, MCP server, WebAuthn server, Docker)
4. You will receive an acknowledgment within 48 hours
5. A fix or mitigation will be prioritized based on severity

We follow responsible disclosure. Security researchers will be credited (with permission) in release notes.

---

## Stellar Key Management

### Local Storage

- Private keys (Stellar seeds, `S...`) are stored locally in `~/.stellar-go-cli/config.json`
- Keys are **never** transmitted to external services unless explicitly signing a transaction
- The `keypair.Full` type is used for in-memory signing — private key material is never logged or printed
- Config file permissions should be restricted to the owner (`chmod 600 ~/.stellar-go-cli/config.json`)

### Wallet Operations

- `keypair.ParseFull(account.PrivateKey)` loads the keypair for signing — the seed is not exposed beyond this point
- Testnet faucet funding (`stellar-go-cli wallet fund`) only works on `stellar-testnet`
- Mainnet operations require an explicitly connected wallet with a funded account
- The active wallet address (`G...`) is safe to display; the private seed (`S...`) is never shown

### Soroban Transaction Security

- All Soroban transactions are **simulated before submission** — the simulation response provides `SorobanTransactionData` and auth entries that are attached to the final transaction
- Transactions are signed with the network passphrase (`network.TestNetworkPassphrase` or `network.PublicNetworkPassphrase`) to prevent cross-network replay attacks
- `submitAndWait()` polls the RPC until the transaction is confirmed on-chain (`SUCCESS` or `FAILED`)
- Default transaction base fee: 100,000 stroops
- Default transaction timeout: 300 seconds
- Default poll interval: 3 seconds

### Simulate-Only (Read-Only) Queries

- `SimulateOnly()` uses a randomly generated dummy keypair (`keypair.Random()`) — no real private keys are needed
- The dummy address is only used to satisfy the transaction builder's source account requirement
- No transaction is submitted to the network — the simulation response is returned directly
- This is safe for read-only contract queries like `get_agreement`, `get_initiator_agreements`, `owner`, `is_paused`

---

## WebAuthn / FIDO2

### Passkey Authentication

- The WebAuthn server (`cmd/webauthn-server/`) provides FIDO2/WebAuthn credential registration and verification
- Passkeys are device-bound and require user presence (touch/biometric) for authentication
- The server runs as a separate Docker service (`Dockerfile.webauthn`) on port 8000
- Health checks are configured for Docker deployments

### Credential Storage

- WebAuthn credentials are associated with the user's Stellar account
- Credential registration requires a signed challenge from the user's device
- Authentication verifies the signed challenge against the registered credential

---

## Docker Security

- **Multi-stage builds** — Build stage uses `golang:1.26-alpine`, final images use `alpine:latest` (minimal attack surface)
- **CGO disabled** — `CGO_ENABLED=0` ensures static binaries with no C dependency vulnerabilities
- **Health checks** — All services include health check endpoints
- **Network isolation** — Docker Compose uses a dedicated `stellar-go-cli` bridge network
- **No secrets in images** — Secrets are passed via environment variables at runtime, never baked into images

### Services

| Service | Port | Purpose |
|---------|------|---------|
| WebAuthn | 8000 | FIDO2/passkey authentication |
| MCP | 3000 | AI assistant integration (SSE) |
| VC API | 4000 | W3C VC API test endpoints |

---

## Supply Chain Security

### Go Modules

- All dependencies are pinned in `go.sum` with cryptographic hashes
- `go mod tidy` ensures minimal dependency tree
- Primary external dependency: `github.com/stellar/go` (Stellar Go SDK with Soroban RPC support)
- Direct dependencies are minimal: `charmbracelet` (TUI), `go-chi` (HTTP), `stellar/go` (Stellar), `prometheus` (metrics), `google/uuid`, `mattn/go-sqlite3`

### Dependency Scanning

```bash
make security    # Run gosec security scanner
make lint        # Run golangci-lint (includes staticcheck)
go list -m all   # List all dependencies
```

### Vulnerability Checking

```bash
govulncheck ./...  # Run Go vulnerability checker (if installed)
```

---

## Security Tooling

### golangci-lint

Configured in `.golangci.yml` with the following enabled linters:
- `errcheck` — Check for unchecked errors
- `govet` — Go vet checks (including shadow detection)
- `staticcheck` — Static analysis
- `ineffassign` — Ineffective assignments
- `unused` — Unused code detection
- `misspell` — Spelling mistakes

Formatters:
- `gci` — Import ordering
- `gofmt` — Code formatting (simplify enabled)

### gosec

```bash
make security    # Run gosec security scanner
```

gosec scans for:
- Hardcoded credentials
- SQL injection
- Weak crypto
- Command injection
- File path traversal
- Other common Go security issues

---

## Best Practices for Contributors

### Key & Secret Handling

- **Never** hardcode private keys, API keys, or secrets in source code
- **Never** log or print private key material (`S...` seeds)
- Use environment variables for secrets in Docker/k8s deployments
- Validate all user inputs (Stellar addresses, amounts, contract IDs)

### Soroban Transactions

- **Always** simulate before submitting — never skip the simulation step
- Use `SimulateOnly()` for read-only queries (no transaction submission needed)
- Use `Invoke()` for write operations (simulates, assembles, signs, submits, polls)
- Extract return values from `SimulateTransactionResponse.Results[0].ReturnValueXDR`
- Use `HostFunctionTypeCreateContractV2` when constructor args are needed

### Code Quality

- Use `internal/ui` package for terminal output (avoids leaking sensitive data to stdout)
- Follow the `Command` struct pattern for all new CLI commands
- Use `soroban.Scv*` helper functions for `xdr.ScVal` construction
- Run `make lint` and `make security` before submitting changes
- Test on testnet before mainnet

### Input Validation

- Validate Stellar addresses using `xdr.AddressToAccountId()` or `strkey.Decode()`
- Validate contract IDs using `strkey.Decode(strkey.VersionByteContract, ...)`
- Validate amounts and timestamps as positive integers
- Sanitize all user-provided strings before use in transactions

---

## Incident Response

If a security incident occurs:

1. **Assess** — Determine the scope and severity of the incident
2. **Contain** — Rotate affected keys, revoke compromised credentials
3. **Notify** — Email hi@ogtechnologies.co with incident details
4. **Fix** — Patch the vulnerability and release a new version
5. **Review** — Post-mortem analysis to prevent recurrence

### Key Rotation

If a Stellar private key is compromised:
1. Create a new wallet: `stellar-go-cli wallet connect --provider stellar --network stellar-testnet`
2. Fund the new account: `stellar-go-cli wallet fund --network stellar-testnet`
3. Transfer assets from the compromised account to the new account
4. Remove the compromised key from `~/.stellar-go-cli/config.json`

---

*Security is a shared responsibility. Report issues responsibly and help keep Stellar Go CLI secure.*
