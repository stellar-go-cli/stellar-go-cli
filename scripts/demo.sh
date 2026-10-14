#!/usr/bin/env bash
#
# demo.sh — guided tour of stellar-go-cli core features.
#
# By default the demo runs inside a throwaway sandbox (a temporary HOME), so it
# never touches your real ~/.stellar-go-cli config, wallets, or secrets. A fresh
# testnet wallet is created and funded via Friendbot, then everything is wiped
# on exit. Pass --real to run against your actual config instead.
#
# Usage:
#   scripts/demo.sh [options]
#
# Options:
#   --real           Use your real ~/.stellar-go-cli config and existing wallet
#   --no-pause       Don't wait for <enter> between sections
#   --skip-network   Offline mode — only local commands (DID/VC, no testnet)
#   --wasm FILE      Also demo a Soroban contract deploy with this .wasm
#   --fn NAME        Contract method to simulate after deploy (requires --wasm)
#   --out DIR        Directory for generated artifacts (default: ./demo-output)
#   -h, --help       Show this help

set -u

# ─── options ──────────────────────────────────────────────────────────────────

USE_REAL=false
NO_PAUSE=false
SKIP_NETWORK=false
WASM=""
FN=""
OUTDIR="demo-output"
NET="stellar-testnet"

usage() { awk '/^#/{sub(/^# ?/,"");print;next}{exit}' "$0"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--real)         USE_REAL=true ;;
	--no-pause|-y)  NO_PAUSE=true ;;
	--skip-network) SKIP_NETWORK=true ;;
	--wasm)         WASM="${2:?--wasm needs a path}"; shift ;;
	--fn)           FN="${2:?--fn needs a method name}"; shift ;;
	--out)          OUTDIR="${2:?--out needs a directory}"; shift ;;
	-h|--help)      usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

[ -n "$FN" ] && [ -z "$WASM" ] && echo "note: --fn ignored without --wasm" >&2

# ─── presentation helpers ─────────────────────────────────────────────────────

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	BOLD=$'\033[1m'; DIM=$'\033[2m'; CYAN=$'\033[36m'; GREEN=$'\033[32m'
	YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
else
	BOLD=""; DIM=""; CYAN=""; GREEN=""; YELLOW=""; RED=""; RESET=""
fi

section() {
	printf '\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n' "$DIM" "$RESET"
	printf '%s%s  %s%s\n' "$BOLD" "$CYAN" "$*" "$RESET"
	printf '%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n' "$DIM" "$RESET"
}

note()  { printf '%s  %s%s\n' "$DIM" "$*" "$RESET"; }
warn()  { printf '%s  warning: %s%s\n' "$YELLOW" "$*" "$RESET"; }
fail()  { printf '%s  failed: %s%s\n' "$RED" "$*" "$RESET"; }

pause() {
	$NO_PAUSE && return 0
	printf '\n%s── press <enter> to continue ──%s' "$DIM" "$RESET"
	{ read -r _ < /dev/tty; } 2>/dev/null || true
	printf '\n'
}

# Print the command as typed, then run it. Returns the command's exit code.
run() {
	printf '\n%s$ stellar-go-cli' "$GREEN"
	printf ' %q' "$@"
	printf '%s\n' "$RESET"
	"$CLI" "$@"
	local rc=$?
	[ $rc -ne 0 ] && fail "exit code $rc — continuing anyway"
	return $rc
}

# ─── locate the binary ────────────────────────────────────────────────────────

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ -n "${STELLAR_CLI:-}" ]; then
	CLI="$STELLAR_CLI"
elif command -v stellar-go-cli >/dev/null 2>&1; then
	CLI="stellar-go-cli"
elif [ -x "$ROOT/dist/stellar-go-cli" ]; then
	CLI="$ROOT/dist/stellar-go-cli"
else
	note "stellar-go-cli not found — building via make build"
	make -C "$ROOT" build || { echo "build failed" >&2; exit 1; }
	CLI="$ROOT/dist/stellar-go-cli"
fi

# ─── sandboxed HOME ───────────────────────────────────────────────────────────

if $USE_REAL; then
	DEMO_HOME="$HOME"
	note "running against your real config (~/.stellar-go-cli)"
else
	DEMO_HOME="$(mktemp -d "${TMPDIR:-/tmp}/stellar-go-cli-demo.XXXXXX")"
	trap 'rm -rf "$DEMO_HOME"' EXIT
	note "sandboxed HOME: $DEMO_HOME (wiped on exit)"
fi
export HOME="$DEMO_HOME"

CONFIG_JSON="$DEMO_HOME/.stellar-go-cli/config.json"

active_addr() {
	[ -f "$CONFIG_JSON" ] || return 1
	sed -n 's/.*"activeAddress"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CONFIG_JSON" | head -1
}

# ─── the demo ─────────────────────────────────────────────────────────────────

printf '%s%s\n' "$BOLD" "stellar-go-cli demo — wallets, DIDs/VCs, payments, swaps, reports, contracts"
note "binary: $CLI    network: $NET"
pause

# 1 ── identity & credentials (fully offline, instant) ────────────────────────

section "1/6 — Decentralized identity & verifiable credentials (offline)"
note "DID creation and VC issuance are pure cryptography — no network needed."

run did create --method key || true
pause

run did attest --method ebsi --vc kyc --name "Demo User" --country AT || true
pause

# 2 ── init & status ───────────────────────────────────────────────────────────

section "2/6 — Configuration & system status"

run init || true
run status || true
pause

# 3 ── testnet wallet ──────────────────────────────────────────────────────────

ADDR=""
FUNDED=false

if ! $SKIP_NETWORK; then
	section "3/6 — Testnet wallet (keypair + Friendbot faucet)"

	ADDR="$(active_addr || true)"
	if $USE_REAL && [ -n "$ADDR" ]; then
		note "reusing existing active wallet: $ADDR"
	else
		# empty stdin line auto-accepts the "make active?" prompt if it appears
		printf '\n' | run wallet connect --provider stellar --network "$NET"
		ADDR="$(active_addr || true)"
	fi

	if [ -z "$ADDR" ]; then
		warn "no active wallet — skipping fund/pay/report steps"
	else
		run wallet fund --network "$NET" && FUNDED=true
		run wallet show
	fi
	pause
else
	section "3/6 — Testnet wallet"
	note "skipped (--skip-network)"
fi

# 4 ── payment ─────────────────────────────────────────────────────────────────

if $FUNDED; then
	section "4/6 — Payment on the direct Stellar rail"
	note "Sending 1 XLM to the wallet's own address — deterministic and safe."
	note "--vc-attach embeds the credential ID from step 1 in the tx memo."

	run pay send --to "$ADDR" --amount 1 --asset XLM --rail direct --vc-attach
	pause
elif ! $SKIP_NETWORK; then
	section "4/6 — Payment"
	note "skipped (wallet not funded)"
fi

# 5 ── DEX quote (read-only) ───────────────────────────────────────────────────

if ! $SKIP_NETWORK; then
	section "5/6 — DEX path-payment quote (read-only)"
	note "Quotes hit Horizon only — nothing is submitted."

	run swap quote --from XLM --to USDC --amount 10 --network "$NET" || true
	pause
fi

# 6 ── compliance reporting ────────────────────────────────────────────────────

if $FUNDED; then
	section "6/6 — Compliance report + ISO 20022 export"
	note "ui/* writes status to stderr, so stdout stays clean for redirection."

	mkdir -p "$OUTDIR"
	run report generate || true
	printf '\n%s$ stellar-go-cli report iso20022 --type pacs.008 > %s/pacs008.xml%s\n' "$GREEN" "$OUTDIR" "$RESET"
	if "$CLI" report iso20022 --type pacs.008 > "$OUTDIR/pacs008.xml"; then
		note "first lines of $OUTDIR/pacs008.xml:"
		head -8 "$OUTDIR/pacs008.xml" | sed 's/^/    /'
	else
		fail "iso20022 export failed"
	fi
	pause
elif ! $SKIP_NETWORK; then
	section "6/6 — Compliance reporting"
	note "skipped (no payment to report on)"
fi

# ── optional: soroban contract deploy ─────────────────────────────────────────

if [ -n "$WASM" ]; then
	if ! $FUNDED; then
		warn "--wasm given but wallet not funded — skipping contract deploy"
	elif [ ! -f "$WASM" ]; then
		warn "wasm file not found: $WASM"
	else
		section "Optional — Soroban contract deploy (pure Go, no stellar CLI)"
		run contract deploy --wasm "$WASM" --network "$NET" || true
		if [ -n "$FN" ]; then
			note "simulate = read-only, no transaction submitted"
			run contract invoke --fn "$FN" --simulate || true
		fi
		pause
	fi
fi

# ─── outro ────────────────────────────────────────────────────────────────────

section "Done"
cat <<EOF
  What you saw:
    • offline DID creation + VC issuance (did:key, did:ebsi)
    • testnet wallet: keypair generation + Friendbot funding
    • direct-rail XLM payment with a VC attached to the memo
    • read-only DEX path-payment quote
    • post-transaction report → ISO 20022 pacs.008 XML on stdout

  Try next:
    stellar-go-cli mcp                          # MCP server for AI assistants
    stellar-go-cli contract deploy --wasm <f>   # deploy any Soroban contract
    stellar-go-cli asset create-ft --help       # SEP-41 token issuance
    stellar-go-cli pay rails                    # all payment rails compared
EOF
$USE_REAL || note "sandbox wiped — your real config was never touched"
