# Contributing to Stellar Go CLI

Thanks for helping improve Stellar Go CLI — an open-source public good for the Stellar ecosystem.

## Development Setup

```bash
git clone https://github.com/stellar-go-cli/stellar-go-cli.git
cd stellar-go-cli
make build   # → dist/stellar-go-cli
make test    # run the test suite
make lint    # go vet + golangci-lint
```

Requirements: Go 1.26+. Docker is optional (MCP/WebAuthn services).

## Pull Request Guidelines

- Keep PRs focused — one feature or fix per PR
- Follow the `Command` struct pattern in `cmd/stellar-go-cli/commands/` for new CLI commands
- Public library code belongs in `pkg/`; application internals stay in `internal/`
- All terminal chrome goes through `internal/ui` (stderr); stdout is reserved for data output (XML, JSON)
- Run `gofmt -l .` (must print nothing), `go vet ./...`, and `go test ./...` before submitting
- Test Soroban changes on testnet before touching mainnet paths
- Never commit private keys, seeds, API keys, or other secrets
- No shell-out to the `stellar` CLI — use `pkg/soroban`

## Build Tags

Non-core command groups (`chat`, `chat_finetune`, `terminal`, `triangular`, `exchange`) are gated behind the `extras` build tag:

```bash
go build -tags extras ./cmd/stellar-go-cli
```

If you add a command group that isn't core Stellar functionality, gate it the same way via `registerExtras` in `cmd/stellar-go-cli/commands/extras.go`.

## ISO 20022 XSD Tests

`TestXSDValidation_*` tests validate generated XML against the official ISO 20022 schemas. The XSDs cannot be vendored (iso20022.org redistribution terms) — see `pkg/iso20022/testdata/xsd/README.md` for where to place them. CI downloads them automatically.

## Reporting Issues

- Bugs and feature requests: GitHub Issues
- Security vulnerabilities: see [SECURITY.md](SECURITY.md) — do not open a public issue

## License

By contributing you agree your contributions are licensed under the [Apache License 2.0](LICENSE).
