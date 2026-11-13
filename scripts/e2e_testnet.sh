#!/usr/bin/env bash
#
# e2e_testnet.sh — live end-to-end test of the CLI's main features.
#
# Runs inside a throwaway HOME (temporary directory), so it never touches the
# real ~/.stellar-go-cli config, wallets, or secrets. It creates a fresh
# testnet keypair, funds it via Friendbot, submits a real 1 XLM self-payment,
# and exercises the offline and read-only features around it. Any step marked
# "FAIL" fails the run; "WARN" steps are tolerated (testnet liquidity and
# faucet availability vary). Exits non-zero if any required step fails.
#
# Usage:
#   scripts/e2e_testnet.sh [options]
#
# Options:
#   --offline      Skip every step that needs the network (DID/VC/report only)
#   --out DIR      Directory for generated artifacts (default: mktemp)
#   -h, --help     Show this help
#
# Environment:
#   STELLAR_CLI    Path to a pre-built binary (default: ./dist/stellar-go-cli,
#                  then PATH lookup, then `make build`)

set -u

# ─── options ──────────────────────────────────────────────────────────────────

OFFLINE=false
OUTDIR=""
NET="stellar-testnet"

usage() { awk '/^#/{sub(/^# ?/,"");print;next}{exit}' "$0"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--offline)     OFFLINE=true ;;
	--out)         OUTDIR="${2:?--out needs a directory}"; shift ;;
	-h|--help)     usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

# ─── presentation & assertion helpers ─────────────────────────────────────────

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	BOLD=$'\033[1m'; DIM=$'\033[2m'; CYAN=$'\033[36m'; GREEN=$'\033[32m'
	YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
else
	BOLD=""; DIM=""; CYAN=""; GREEN=""; YELLOW=""; RED=""; RESET=""
fi

PASS=0
FAILED=0
WARNED=0

section() {
	printf '\n%s━━ %s%s\n' "$BOLD$CYAN" "$*" "$RESET"
}

pass()  { PASS=$((PASS+1));   printf '%s  PASS%s  %s\n' "$GREEN"  "$RESET" "$*"; }
fail()  { FAILED=$((FAILED+1)); printf '%s  FAIL%s  %s\n' "$RED"   "$RESET" "$*"; }
warn_() { WARNED=$((WARNED+1)); printf '%s  WARN%s  %s\n' "$YELLOW" "$RESET" "$*"; }
note()  { printf '%s       %s%s\n' "$DIM" "$*" "$RESET"; }

# check NAME -- CMD... : run CMD, PASS on exit 0, FAIL otherwise.
check() {
	local name="$1"; shift
	[ "$1" = "--" ] && shift
	printf '%s$ stellar-go-cli' "$DIM"
	printf ' %q' "$@"
	printf '%s\n' "$RESET"
	if "$CLI" "$@"; then
		pass "$name"
	else
		fail "$name (exit $?)"
	fi
}

# soft NAME -- CMD... : like check but only WARN on failure.
soft() {
	local name="$1"; shift
	[ "$1" = "--" ] && shift
	printf '%s$ stellar-go-cli' "$DIM"
	printf ' %q' "$@"
	printf '%s\n' "$RESET"
	if "$CLI" "$@"; then
		pass "$name"
	else
		warn_ "$name (exit $?)"
	fi
}

# expect NAME PATTERN FILE : PASS if FILE greps for PATTERN.
expect() {
	local name="$1" pattern="$2" file="$3"
	if grep -q "$pattern" "$file" 2>/dev/null; then
		pass "$name"
	else
		fail "$name (pattern '$pattern' not found in $file)"
	fi
}

# ─── locate the binary ────────────────────────────────────────────────────────

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ -n "${STELLAR_CLI:-}" ]; then
	CLI="$STELLAR_CLI"
elif [ -x "$ROOT/dist/stellar-go-cli" ]; then
	CLI="$ROOT/dist/stellar-go-cli"
elif command -v stellar-go-cli >/dev/null 2>&1; then
	CLI="stellar-go-cli"
else
	note "stellar-go-cli not found — building via make build"
	make -C "$ROOT" build || { echo "build failed" >&2; exit 1; }
	CLI="$ROOT/dist/stellar-go-cli"
fi

# ─── sandboxed HOME & artifact dir ────────────────────────────────────────────

SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/stellar-go-cli-e2e.XXXXXX")"
trap 'rm -rf "$SANDBOX"' EXIT
export HOME="$SANDBOX"
note "sandboxed HOME: $SANDBOX (wiped on exit)"

[ -z "$OUTDIR" ] && OUTDIR="$(mktemp -d "${TMPDIR:-/tmp}/stellar-e2e-out.XXXXXX")"
mkdir -p "$OUTDIR"

CONFIG_JSON="$SANDBOX/.stellar-go-cli/config.json"
STATE_DIR="$SANDBOX/.stellar-go-cli/state"

json_field() { # json_field FILE FIELD → first "field": "value"
	sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$1" | head -1
}

printf '%s%s\n' "$BOLD" "stellar-go-cli e2e — testnet wallet, payment, DIDs/VCs, swap quote, reports"
note "binary: $CLI    network: $NET    artifacts: $OUTDIR"

# ─── 1: init & status ─────────────────────────────────────────────────────────

section "1/7 — init & system status"
check "init"   -- init
check "status" -- status

# ─── 2: offline identity & credentials ────────────────────────────────────────

section "2/7 — DID create + VC attest (offline)"

"$CLI" did create --method key --output json > "$OUTDIR/did_key.json" 2>/dev/null \
	&& pass "did create (key)" || fail "did create (key)"
expect "did:key document emitted" 'did:key' "$OUTDIR/did_key.json"

"$CLI" did attest --method key --vc kyc --name "E2E Test" --country AT --output json \
	> "$OUTDIR/vc.json" 2>/dev/null \
	&& pass "did attest (kyc VC)" || fail "did attest (kyc VC)"
expect "VC issued" '"type"' "$OUTDIR/vc.json"
[ -f "$STATE_DIR/vc_latest.json" ] && pass "vc_latest state saved" \
	|| fail "vc_latest state saved"

# ─── 3: testnet wallet ────────────────────────────────────────────────────────

section "3/7 — testnet wallet (keypair + Friendbot)"
ADDR=""

if $OFFLINE; then
	note "skipped (--offline)"
else
	# fresh sandbox has no other wallets, so no "make active?" prompt appears;
	# the piped newline is belt-and-braces in case that changes.
	if printf '\n' | "$CLI" wallet connect --provider stellar --network "$NET" --output json \
		> "$OUTDIR/wallet.json" 2>/dev/null; then
		ADDR="$(json_field "$OUTDIR/wallet.json" address)"
	fi
	if [ -n "$ADDR" ]; then
		pass "wallet connect → $ADDR"
	else
		fail "wallet connect (no address in output)"
	fi
	expect "address is a valid Stellar pubkey" '"address": "G[A-Z2-7]\{55\}"' "$OUTDIR/wallet.json"
	expect "wallet set active in config" "\"activeAddress\": \"$ADDR\"" "$CONFIG_JSON"

	if [ -n "$ADDR" ]; then
		check "wallet fund (Friendbot)" -- wallet fund --network "$NET"
		expect "account marked funded" '"funded": true' "$STATE_DIR/account.json"
		check "wallet balance" -- wallet balance --address "$ADDR"
		check "wallet show"    -- wallet show
		check "wallet list"    -- wallet list
	fi
fi

# ─── 4: payment on the direct rail ────────────────────────────────────────────

section "4/7 — payment (direct rail)"

if $OFFLINE; then
	note "skipped (--offline)"
elif [ -z "$ADDR" ]; then
	fail "pay send (no funded wallet — see step 3)"
else
	note "Sending 1 XLM to self with the VC from step 2 in the tx memo."
	if "$CLI" pay send --to "$ADDR" --amount 1 --asset XLM --rail direct --vc-attach \
		--output json > "$OUTDIR/payment.json" 2>/dev/null; then
		pass "pay send submitted"
	else
		fail "pay send (exit $?)"
	fi
	expect "payment confirmed" '"status": "confirmed"' "$OUTDIR/payment.json"
	TXHASH="$(json_field "$OUTDIR/payment.json" txHash)"
	if [ -n "$TXHASH" ]; then
		pass "on-ledger tx hash: $TXHASH"
		note "https://stellar.expert/explorer/testnet/tx/$TXHASH"
	else
		fail "on-ledger tx hash (empty txHash)"
	fi
fi

# ─── 5: DEX quote (read-only) ─────────────────────────────────────────────────

section "5/7 — DEX path-payment quote (read-only)"

if $OFFLINE; then
	note "skipped (--offline)"
else
	# Soft: the testnet XLM→USDC orderbook is thin — a missing path fails the
	# command without indicating a CLI bug.
	soft "swap quote XLM→USDC" -- swap quote --from XLM --to USDC --amount 10 --network "$NET"
fi

# ─── 6: compliance reporting ──────────────────────────────────────────────────

section "6/7 — report + ISO 20022 export"

"$CLI" report generate --output json > "$OUTDIR/report.json" 2>/dev/null \
	&& pass "report generate" || fail "report generate"
expect "report has an ID" '"reportId"' "$OUTDIR/report.json"

if "$CLI" report iso20022 --type pacs.008 > "$OUTDIR/pacs008.xml" 2>/dev/null; then
	pass "report iso20022 (pacs.008)"
else
	fail "report iso20022 (pacs.008)"
fi
expect "clean XML on stdout"   '<?xml'              "$OUTDIR/pacs008.xml"
expect "pacs.008 root element" 'FIToFICstmrCdtTrf'  "$OUTDIR/pacs008.xml"

# ─── 7: network config (read-only) ────────────────────────────────────────────

section "7/7 — network config (read-only)"

# Purely local — no --offline gate.
check "network show" -- network show

# ─── summary ──────────────────────────────────────────────────────────────────

section "Summary"
printf '  %s%d passed%s, %s%d failed%s, %s%d warned%s\n' \
	"$GREEN"  "$PASS"   "$RESET" \
	"$RED"    "$FAILED" "$RESET" \
	"$YELLOW" "$WARNED" "$RESET"
note "artifacts kept at: $OUTDIR"

if [ "$FAILED" -gt 0 ]; then
	exit 1
fi
