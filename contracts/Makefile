# Makefile for MozartPay Soroban Smart Contracts
# Compiles Rust contracts to WASM for Stellar deployment

.PHONY: all build check clean test deploy-local

# Default target
all: check build

# Check Rust toolchain and dependencies
check:
	@echo "Checking Rust toolchain..."
	@rustup target list --installed | grep -q wasm32-unknown-unknown || (echo "Installing wasm32 target..." && rustup target add wasm32-unknown-unknown)
	@echo "Toolchain ready ✓"

# Build all contracts to WASM
build:
	@echo "Building contracts to WASM..."
	@cargo build --release --target wasm32-unknown-unknown
	@echo ""
	@echo "Build complete! WASM files:"
	@ls -la target/wasm32-unknown-unknown/release/*.wasm 2>/dev/null || echo "No WASM files found"
	@mkdir -p dist
	@cp target/wasm32-unknown-unknown/release/*.wasm dist/ 2>/dev/null || true
	@echo ""
	@echo "WASM files copied to dist/"

# Build individual contracts
zk-verifier:
	@cargo build --bin zk_verifier --release --target wasm32-unknown-unknown

orchestrated-agreement:
	@cargo build --bin orchestrated_agreement --release --target wasm32-unknown-unknown

# Clean build artifacts
clean:
	@cargo clean
	@rm -rf dist/
	@echo "Build artifacts cleaned"

# Run contract tests
test:
	@cargo test --lib

# Format code
fmt:
	@cargo fmt

# Check code without building
check-only:
	@cargo check --target wasm32-unknown-unknown

# Show contract sizes (for optimization review)
sizes: build
	@echo ""
	@echo "Contract WASM Sizes:"
	@ls -lh dist/*.wasm | awk '{print $$9, "->", $$5}'

# Local deployment info (manual step reference)
deploy-info:
	@echo ""
	@echo "=== Deployment Commands ==="
	@echo ""
	@echo "1. Install Stellar CLI:"
	@echo "   cargo install stellar-cli --locked"
	@echo ""
	@echo "2. Configure testnet:"
	@echo "   stellar network add testnet --rpc-url https://soroban-testnet.stellar.org --network-passphrase 'Test SDF Network ; September 2015'"
	@echo ""
	@echo "3. Deploy ZK Verifier:"
	@echo "   stellar contract deploy --wasm dist/zk_verifier.wasm --source <YOUR_KEY> --network testnet"
	@echo ""
	@echo "4. Deploy Orchestrated Agreement:"
	@echo "   stellar contract deploy --wasm dist/orchestrated_agreement.wasm --source <YOUR_KEY> --network testnet"
	@echo ""

# Help
help:
	@echo "Available targets:"
	@echo "  make build       - Build all contracts to WASM"
	@echo "  make zk-verifier - Build only ZK verifier contract"
	@echo "  make orchestrated-agreement - Build only OA contract"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make test        - Run contract tests"
	@echo "  make sizes       - Show optimized WASM sizes"
	@echo "  make deploy-info - Show deployment instructions"
