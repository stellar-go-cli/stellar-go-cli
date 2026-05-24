BINARY     = mozartpay
MODULE     = github.com/ogtechnologies/mozartpay
CMD        = ./cmd/mozartpay
VERSION    = 0.1.0-mvp
BUILD_DIR  = ./dist
GOFLAGS    =

LDFLAGS = -ldflags "-X $(MODULE)/internal/config.Version=$(VERSION) -s -w"

.PHONY: all build run clean test lint install help

## all: Build the binary (default)
all: build

## build: Compile the CLI binary
build:
	@echo "→ Building $(BINARY) v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD)
	@echo "✓ Binary ready: $(BUILD_DIR)/$(BINARY)"

## run: Build and run with no arguments (shows help)
run: build
	@$(BUILD_DIR)/$(BINARY)

## install: Install binary to GOPATH/bin
install:
	@echo "→ Installing $(BINARY)..."
	@go install $(GOFLAGS) $(LDFLAGS) $(CMD)
	@echo "✓ Installed to $$(go env GOPATH)/bin/$(BINARY)"

## test: Run all tests
test:
	@echo "→ Running tests..."
	@go test ./... -v

## lint: Run go vet + golangci-lint
lint:
	@echo "→ Linting..."
	@go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not installed, skipping. Install with: brew install golangci-lint"; \
	fi
	@echo "✓ Linting complete"

## security: Run gosec security scanner
security:
	@echo "→ Running security scan..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "⚠️  gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
		exit 0; \
	fi
	@echo "✓ Security scan complete"

## clean: Remove build artifacts and state
clean:
	@rm -rf $(BUILD_DIR)
	@echo "✓ Cleaned"

## tidy: Tidy go.mod
tidy:
	@go mod tidy

# ─── Demo targets ────────────────────────────

## demo-did: Demo DID creation
demo-did: build
	@$(BUILD_DIR)/$(BINARY) did create --method ebsi --output pretty

## demo-attest: Demo DID attestation with VC
demo-attest: build
	@$(BUILD_DIR)/$(BINARY) did attest --method ebsi --name "Olvis E. Gil Ríos" --country AT

## demo-wallet: Demo wallet connect + fund
demo-wallet: build
	@$(BUILD_DIR)/$(BINARY) wallet connect --provider wwwallet --network stellar-testnet
	@$(BUILD_DIR)/$(BINARY) wallet fund --network stellar-testnet

## demo-asset: Demo asset creation with score and carbon
demo-asset: build
	@$(BUILD_DIR)/$(BINARY) asset create-ft \
		--name "OGTechToken" \
		--symbol OGT \
		--supply 10000000 \
		--with-score \
		--with-carbon \
		--carbon-amount 5.0

## demo-pay: Demo payment via x402
demo-pay: build
	@$(BUILD_DIR)/$(BINARY) pay x402 \
		--resource "https://api.mozartpay.com/v1/feed" \
		--price 0.001 \
		--asset USDC

## demo-report: Demo full compliance report
demo-report: build
	@$(BUILD_DIR)/$(BINARY) report generate --vc-attach

## demo-flow: Show component flow diagram
demo-flow: build
	@$(BUILD_DIR)/$(BINARY) flow

## demo-full: Run the full end-to-end orchestrated flow
demo-full: build
	@echo ""
	@echo "════════════════════════════════════════════"
	@echo " MozartPay — Full Orchestrated Flow Demo"
	@echo "════════════════════════════════════════════"
	@$(BUILD_DIR)/$(BINARY) init
	@$(BUILD_DIR)/$(BINARY) did attest --method ebsi --name "Olvis E. Gil Ríos" --country AT
	@$(BUILD_DIR)/$(BINARY) wallet connect --provider wwwallet --network stellar-testnet
	@$(BUILD_DIR)/$(BINARY) wallet fund --network stellar-testnet
	@$(BUILD_DIR)/$(BINARY) asset create-ft --name "EduToken" --symbol EDU --supply 5000000 --with-score --with-carbon
	@$(BUILD_DIR)/$(BINARY) pay x402 --resource "https://api.mozartpay.com/v1/edudata" --price 0.001 --asset USDC
	@$(BUILD_DIR)/$(BINARY) report generate --vc-attach
	@$(BUILD_DIR)/$(BINARY) status

## help: Show this help
help:
	@echo "MozartPay CLI — Available targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | column -t -s ':'
