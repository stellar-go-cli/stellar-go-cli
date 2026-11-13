#!/usr/bin/env bash
#
# e2e_extended.sh — end-to-end test for CLI features not covered by
# e2e_testnet.sh: DID methods + JWS proofs, VC verification, wallet registry
# ops (incl. import/export/passkey/mandate surfaces), SEP-41 asset issuance,
# trustline lifecycle, DEX swap execution (direct + ZK rails), claimable
# balances, liquidity pools, contract deploy/invoke, MCP stdio + SSE
# transports, audit surfaces, trade records, integrations, network switching,
# and the legacy ~/.mozartpay config migration.
#
# Runs inside a throwaway HOME (temporary directory), so it never touches the
# real ~/.stellar-go-cli config, wallets, or secrets. Two real testnet wallets
# are created and funded via Friendbot. Steps marked "FAIL" fail the run;
# "WARN" steps are tolerated (external services and testnet liquidity vary).
# Exits non-zero if any required step fails.
#
# Usage:
#   scripts/e2e_extended.sh [options]
#
# Options:
#   --offline      Skip every step that needs the network
#   --no-contract  Skip the contract deploy/invoke section
#   --out DIR      Directory for generated artifacts (default: mktemp)
#   -h, --help     Show this help
#
# Environment:
#   STELLAR_CLI    Path to a pre-built binary (default: ./dist/stellar-go-cli,
#                  then PATH lookup, then `make build`)

set -u

# ─── options ──────────────────────────────────────────────────────────────────

OFFLINE=false
CONTRACT=true
OUTDIR=""
NET="stellar-testnet"
# Circle testnet USDC issuer (pkg/swap TestnetAssets; GA5Z... is mainnet).
USDC_ISSUER="GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"

usage() { awk '/^#/{sub(/^# ?/,"");print;next}{exit}' "$0"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--offline)     OFFLINE=true ;;
	--no-contract) CONTRACT=false ;;
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

# check_fail NAME -- CMD... : PASS when CMD exits non-zero.
check_fail() {
	local name="$1"; shift
	[ "$1" = "--" ] && shift
	printf '%s$ stellar-go-cli' "$DIM"
	printf ' %q' "$@"
	printf '%s  (expect failure)\n' "$RESET"
	if "$CLI" "$@" </dev/null >/dev/null 2>&1; then
		fail "$name (expected non-zero exit, got 0)"
	else
		pass "$name"
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
WASM_FIXTURE="$ROOT/scripts/testdata/e2e_hello.wasm"

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

# Real HOME is preserved for the `go run` helper invocation — the sandboxed
# HOME would otherwise make Go create a fresh (read-only) module cache inside
# the sandbox.
REAL_HOME="$HOME"

SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/stellar-go-cli-e2e-x.XXXXXX")"
MCP_PID=""
cleanup() {
	[ -n "$MCP_PID" ] && kill "$MCP_PID" 2>/dev/null
	rm -rf "$SANDBOX"
}
trap cleanup EXIT
export HOME="$SANDBOX"
note "sandboxed HOME: $SANDBOX (wiped on exit)"

[ -z "$OUTDIR" ] && OUTDIR="$(mktemp -d "${TMPDIR:-/tmp}/stellar-e2ex-out.XXXXXX")"
mkdir -p "$OUTDIR"

CONFIG_JSON="$SANDBOX/.stellar-go-cli/config.json"
STATE_DIR="$SANDBOX/.stellar-go-cli/state"

json_field() { # json_field FILE FIELD → first "field": "value"
	sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$1" | head -1
}

# ─── 1: local system commands ─────────────────────────────────────────────────

section "1/17 — local system commands"
check "version"      -- version
check "flow"         -- flow
check "network set"  -- network set "$NET"
check "swap assets"  -- swap assets

# ─── 2: DID methods, JWS proofs, tamper detection (offline) ───────────────────

section "2/17 — DID create / attest / verify (offline, incl. JWS)"

check "did create (key)"  -- did create --method key  --output json
check "did create (ethr)" -- did create --method ethr --output json
check "did create (ebsi)" -- did create --method ebsi --output json
check "did create (web)"  -- did create --method web --domain example.com --output json

"$CLI" did attest --method key --vc kyc --name "E2E Extended" --country AT \
	--output json > "$OUTDIR/vc_key.json" 2>/dev/null \
	&& pass "did attest (ed25519)" || fail "did attest (ed25519)"
check "did verify (ed25519)" -- did verify
check "did verify --vc-file" -- did verify --vc-file "$OUTDIR/vc_key.json"

# Tamper with the signed document → verification must fail (negative check).
sed 's/E2E Extended/Tampered Name/' "$OUTDIR/vc_key.json" > "$OUTDIR/vc_tampered.json"
check_fail "did verify rejects tampered VC" -- did verify --vc-file "$OUTDIR/vc_tampered.json"

# ECDSA/JWS proof path — --method ethr forces --proof-type jws.
"$CLI" did attest --method ethr --vc kyc --name "E2E JWS" \
	--output json > "$OUTDIR/vc_jws.json" 2>/dev/null \
	&& pass "did attest (JWS/ethr)" || fail "did attest (JWS/ethr)"
check "did verify (JWS)" -- did verify

check "did show" -- did show

# ─── 3: local report/audit/pay surfaces ───────────────────────────────────────

section "3/17 — pay rails/quote, iso20022 types, audit (local)"

check "pay rails"   -- pay rails
check "pay quote"   -- pay quote --from XLM --to USDC
check_fail "pay x402 requires --resource" -- pay x402

"$CLI" report generate --output json > "$OUTDIR/report.json" 2>/dev/null \
	&& pass "report generate (prereq)" || fail "report generate (prereq)"
check "report show" -- report show

# Every ISO 20022 message type must emit clean XML on stdout.
for mt in pacs.008 pacs.002 pacs.004 pacs.009 pain.001 pain.002 camt.053 camt.054; do
	if "$CLI" report iso20022 --type "$mt" > "$OUTDIR/iso_$mt.xml" 2>/dev/null \
		&& head -c 5 "$OUTDIR/iso_$mt.xml" | grep -q '<?xml'; then
		pass "iso20022 $mt"
	else
		fail "iso20022 $mt"
	fi
done

check "audit verify" -- audit verify
check "audit events" -- audit events
check_fail "audit latency requires --mandate-ref" -- audit latency
check_fail "audit inventory fails closed (no custodian)" -- audit inventory

# ─── 4: MCP server over stdio ─────────────────────────────────────────────────

section "4/17 — MCP server (stdio JSON-RPC)"

note "Sending initialize + tools/list + tools/call over stdin."
printf '%s\n%s\n%s\n%s\n' \
	'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.1"}}}' \
	'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
	'{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"system_health","arguments":{}}}' \
	'{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nonexistent_tool","arguments":{}}}' \
	| "$CLI" mcp > "$OUTDIR/mcp.jsonl" 2>/dev/null

expect "MCP initialize response"  '"serverInfo"'   "$OUTDIR/mcp.jsonl"
expect "MCP tools/list response"  '"tools"'        "$OUTDIR/mcp.jsonl"
expect "MCP exposes wallet_list"  '"wallet_list"'  "$OUTDIR/mcp.jsonl"
expect "MCP system_health result" '"result"'       "$OUTDIR/mcp.jsonl"
expect "MCP unknown tool errors"  '"error"'        "$OUTDIR/mcp.jsonl"

# wallet_list goes through the custodian gatekeeper; without a running signer
# daemon it may fail closed — tolerated.
printf '%s\n' \
	'{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"wallet_list","arguments":{}}}' \
	| "$CLI" mcp >> "$OUTDIR/mcp.jsonl" 2>/dev/null
if grep -q '"id":5' "$OUTDIR/mcp.jsonl"; then
	pass "MCP wallet_list responded"
else
	warn_ "MCP wallet_list (no response — custodian unavailable)"
fi

# ─── 5: testnet wallet A (keypair + Friendbot) ────────────────────────────────

section "5/17 — testnet wallet A (keypair + Friendbot)"
ADDR_A=""
ADDR_B=""

if $OFFLINE; then
	note "skipped (--offline)"
else
	if printf '\n' | "$CLI" wallet connect --provider stellar --network "$NET" --output json \
		> "$OUTDIR/wallet_a.json" 2>/dev/null; then
		ADDR_A="$(json_field "$OUTDIR/wallet_a.json" address)"
	fi
	[ -n "$ADDR_A" ] && pass "wallet connect A → $ADDR_A" || fail "wallet connect A"
	check "wallet fund A (Friendbot)" -- wallet fund --network "$NET"
	expect "account A funded" '"funded": true' "$STATE_DIR/account.json"
	check "wallet assets" -- wallet assets
fi

# ─── 6: wallet registry ops + import/export/passkey/mandate ───────────────────

section "6/17 — wallet registry ops"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "registry ops (wallet A missing)"
else
	# Second wallet — answer 'n' so it does not become active on its own.
	printf 'n\n' | "$CLI" wallet connect --provider stellar --network "$NET" --output json \
		> "$OUTDIR/wallet_b.json" 2>/dev/null \
		&& ADDR_B="$(json_field "$OUTDIR/wallet_b.json" address)"
	[ -n "$ADDR_B" ] && pass "wallet connect B → $ADDR_B" || fail "wallet connect B"
	expect "A still active" "\"activeAddress\": \"$ADDR_A\"" "$CONFIG_JSON"

	if [ -n "$ADDR_B" ]; then
		check "wallet switch → B" -- wallet switch --address "$ADDR_B"
		expect "B became active" "\"activeAddress\": \"$ADDR_B\"" "$CONFIG_JSON"
		check "wallet switch → A" -- wallet switch --address "$ADDR_A"
		expect "A active again"  "\"activeAddress\": \"$ADDR_A\"" "$CONFIG_JSON"

		check "wallet rename B"  -- wallet rename "$ADDR_B" e2e-temp

		check "wallet export (public)" -- wallet export --address "$ADDR_A"

		# Export B's seed (stderr → file), remove B, re-import via
		# --private-key-file: full seed round-trip without echoing secrets.
		SEED_FILE="$OUTDIR/wallet_b.seed"
		"$CLI" wallet export --address "$ADDR_B" --private 2>&1 \
			| grep -oE 'S[A-Z2-7]{55}' | head -1 > "$SEED_FILE"
		if [ -s "$SEED_FILE" ]; then
			chmod 600 "$SEED_FILE"
			pass "wallet export --private yielded seed"
		else
			fail "wallet export --private (no seed on stderr)"
		fi

		check "wallet remove B" -- wallet remove --address "$ADDR_B" --confirm
		check_fail "removed wallet cannot switch" -- wallet switch --address "$ADDR_B"

		if [ -s "$SEED_FILE" ]; then
			if "$CLI" wallet import --private-key-file "$SEED_FILE" \
				--network "$NET" --no-active --output json \
				> "$OUTDIR/wallet_b_import.json" 2>/dev/null; then
				IMPORTED="$(json_field "$OUTDIR/wallet_b_import.json" address)"
				if [ "$IMPORTED" = "$ADDR_B" ]; then
					pass "wallet import round-trip → $IMPORTED"
				else
					fail "wallet import (address $IMPORTED != $ADDR_B)"
				fi
			else
				fail "wallet import (exit $?)"
			fi
			check "wallet remove B (post-import)" -- wallet remove --address "$ADDR_B" --confirm
		fi
		rm -f "$SEED_FILE"
	fi

	# Wallet-derived issuer VC (plaintext wallet path — no custodian needed).
	"$CLI" did attest --use-wallet-key --method key --vc national-id --name "E2E Wallet Key" \
		--output json > "$OUTDIR/vc_wallet.json" 2>/dev/null \
		&& pass "did attest --use-wallet-key" || fail "did attest --use-wallet-key"
	check "did verify (wallet-key VC)" -- did verify

	check "wallet passkey status" -- wallet passkey status
	soft  "wallet passkey list"   -- wallet passkey list --address "$ADDR_A"
	# Custodian daemon is not running in the sandbox → mandates must fail closed.
	check_fail "wallet mandate list fails closed"  -- wallet mandate list
	check_fail "wallet mandate issue fails closed" -- wallet mandate issue \
		--agent did:key:ze2e --scope pay_send --valid-until 2030-01-01T00:00:00Z
fi

# ─── 7: SEP-41 asset issuance (real SAC deploys) ─────────────────────────────

section "7/17 — asset create-ft + create-nfa (SEP-41 SAC deploys)"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "asset issuance (wallet A missing)"
else
	note "Deploys two real Stellar Asset Contracts — takes a couple of minutes."
	if "$CLI" asset create-ft --name "E2E Token" --symbol E2ET --supply 1000 \
		--network "$NET" --output json > "$OUTDIR/asset.json" 2>"$OUTDIR/asset.err"; then
		pass "asset create-ft"
	else
		fail "asset create-ft (exit $?)"
	fi
	expect "contract ID returned" '"contractId": "C[A-Z2-7]\{55\}"' "$OUTDIR/asset.json"
	expect "deploy tx hash"       '"txHash"'                        "$OUTDIR/asset.json"
	expect "asset state saved"    '"symbol": "E2ET"'                "$STATE_DIR/asset_latest.json"
	check "asset show" -- asset show

	soft "asset carbon (credit attach)" -- asset carbon --amount 0.5

	if "$CLI" asset create-nfa --name "E2E Collectible" --description "e2e nfa" \
		--network "$NET" --output json > "$OUTDIR/nfa.json" 2>"$OUTDIR/nfa.err"; then
		pass "asset create-nfa"
	else
		fail "asset create-nfa (exit $?)"
	fi
	expect "nfa state saved" '"type": "non-fungible"\|"nonfungible"\|"non_fungible"' "$STATE_DIR/asset_latest.json"
	check "asset show (nfa)" -- asset show
fi

# ─── 8: trustline lifecycle (trust → untrust → trust) ─────────────────────────

section "8/17 — trustline lifecycle (USDC)"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "trustlines (wallet A missing)"
else
	check "asset trust USDC"   -- asset trust --code USDC --issuer "$USDC_ISSUER"
	# Zero balance → untrust succeeds immediately, then re-trust for the swap.
	check "asset untrust USDC" -- asset untrust --code USDC --issuer "$USDC_ISSUER" --confirm
	check "asset re-trust USDC" -- asset trust --code USDC --issuer "$USDC_ISSUER"
fi

# ─── 9: DEX execution (real path payments, direct + ZK rails) ────────────────

section "9/17 — swap execute + swap zk (real path payments)"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "swaps (wallet A missing)"
else
	# 'y' answers the "Proceed with this swap?" confirmation.
	if printf 'y\n' | "$CLI" swap execute --from XLM --to USDC --amount 5 \
		--network "$NET" --output json > "$OUTDIR/swap.json" 2>/dev/null; then
		pass "swap execute submitted"
	else
		fail "swap execute (exit $?)"
	fi
	expect "swap confirmed" '"status": "confirmed"' "$OUTDIR/swap.json"
	TXHASH="$(json_field "$OUTDIR/swap.json" txHash)"
	if [ -n "$TXHASH" ]; then
		pass "swap tx hash: $TXHASH"
		note "https://stellar.expert/explorer/testnet/tx/$TXHASH"
	else
		fail "swap tx hash (empty txHash)"
	fi

	# ZK rail — proof generation is simulated locally but the settlement is a
	# real path payment on the same trustline. Two confirmations: one to build
	# the request, one to execute after the quote.
	if printf 'y\ny\n' | "$CLI" swap zk --from XLM --to USDC --amount 2 \
		--network "$NET" --output json > "$OUTDIR/swap_zk.json" 2>/dev/null; then
		pass "swap zk submitted"
	else
		fail "swap zk (exit $?)"
	fi
	expect "swap zk confirmed" '"status": "confirmed"' "$OUTDIR/swap_zk.json"

	soft "pool list" -- pool list --network "$NET" --output json
fi

# ─── 10: claimable balances (helper creates, CLI claims/declines) ────────────

section "10/17 — claimable balances"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "claimables (wallet A missing)"
else
	# The CLI has no create-claimable command — the helper submits a real
	# createClaimableBalance op sponsored by wallet A, claiming back to A.
	# HOME is restored for `go run` so it uses the real Go module cache.
	if ! command -v go >/dev/null 2>&1; then
		warn_ "claimable lifecycle skipped (no go toolchain for the setup helper)"
	else
	SEED_A="$(json_field "$STATE_DIR/wallets/$ADDR_A.json" privateKey)"
	CB_HASH="$(CB_SEED="$SEED_A" HOME="$REAL_HOME" go run "$ROOT/scripts/e2e_helpers/create_claimable" "$ADDR_A" 1 2>"$OUTDIR/cb.err")" || true
	if [ -n "$CB_HASH" ]; then
		pass "claimable balance created (tx $CB_HASH)"
	else
		fail "create claimable balance (see cb.err)"
	fi

	"$CLI" claimable list --network "$NET" --output json \
		> "$OUTDIR/claimables.json" 2>/dev/null
	CB_ID="$(grep -oE '"id": "[0-9a-f]{72}"' "$OUTDIR/claimables.json" | head -1 | cut -d'"' -f4)"
	if [ -n "$CB_ID" ]; then
		pass "claimable list shows balance"
	else
		fail "claimable list (no balance id)"
	fi

	if [ -n "$CB_ID" ]; then
		check "claimable accept" -- claimable accept --id "$CB_ID" --network "$NET"
		check_fail "claimable accept rejects unknown id" \
			-- claimable accept --id "0000000000000000000000000000000000000000000000000000000000000001" --network "$NET"
	fi

	# Second balance → exercise decline (claim-and-return) path.
	CB_HASH2="$(CB_SEED="$SEED_A" HOME="$REAL_HOME" go run "$ROOT/scripts/e2e_helpers/create_claimable" "$ADDR_A" 2 2>>"$OUTDIR/cb.err")" || true
	"$CLI" claimable list --network "$NET" --output json \
		> "$OUTDIR/claimables2.json" 2>/dev/null
	CB_ID2="$(grep -oE '"id": "[0-9a-f]{72}"' "$OUTDIR/claimables2.json" | head -1 | cut -d'"' -f4)"
	if [ -n "$CB_ID2" ]; then
		check "claimable decline" -- claimable decline --id "$CB_ID2" \
			--asset-code XLM --asset-issuer "" --amount 2 --network "$NET"
	else
		warn_ "claimable decline skipped (second balance missing)"
	fi
	fi
fi

# ─── 11: pay zk + audit evidence ─────────────────────────────────────────────

section "11/17 — pay zk + audit evidence"

if $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "pay zk (wallet A missing)"
else
	# Self-payment over the ZK rail — 'y' answers the proof-generation confirm.
	if printf 'y\n' | "$CLI" pay zk --to "$ADDR_A" --amount 0.5 \
		--network "$NET" --output json > "$OUTDIR/pay_zk.json" 2>/dev/null; then
		pass "pay zk submitted"
	else
		fail "pay zk (exit $?)"
	fi
	expect "pay zk confirmed" '"status": "confirmed"' "$OUTDIR/pay_zk.json"

	check "audit evidence (payment bundle)" \
		-- audit evidence --payment "$OUTDIR/pay_zk.json"
fi

# ─── 12: contract deploy + invoke --simulate ─────────────────────────────────

section "12/17 — contract deploy + invoke --simulate"

if ! $CONTRACT; then
	note "skipped (--no-contract)"
elif $OFFLINE || [ -z "$ADDR_A" ]; then
	$OFFLINE && note "skipped (--offline)" || fail "contract deploy (wallet A missing)"
elif [ ! -f "$WASM_FIXTURE" ]; then
	fail "wasm fixture missing: $WASM_FIXTURE"
else
	check "contract deploy" -- contract deploy --wasm "$WASM_FIXTURE" --network "$NET"
	expect "contract ID persisted" '"contractID": "C[A-Z2-7]\{55\}"' "$CONFIG_JSON"

	# hello(to: String) — arg is base64 XDR ScVal{SCV_STRING, "e2e"}.
	SCVAL_TO="AAAADgAAAANlMmUA"
	check "contract invoke --simulate" \
		-- contract invoke --fn hello --args "$SCVAL_TO" --simulate
	check "contract set --id" \
		-- contract set --id "$(json_field "$CONFIG_JSON" contractID)"
fi

# ─── 13: MCP server over SSE transport ───────────────────────────────────────

section "13/17 — MCP server (SSE transport)"

if $OFFLINE; then
	note "skipped (--offline)"
else
	MCP_PORT=$((20000 + RANDOM % 20000))
	"$CLI" mcp --transport sse --port "$MCP_PORT" > "$OUTDIR/mcp_sse.log" 2>&1 &
	MCP_PID=$!
	ready=false
	for _ in $(seq 1 20); do
		if curl -sf -o /dev/null "http://127.0.0.1:$MCP_PORT/health"; then
			ready=true; break
		fi
		sleep 0.5
	done
	if $ready; then
		pass "MCP SSE health endpoint"
		curl -sf -X POST "http://127.0.0.1:$MCP_PORT/message" \
			-H 'Content-Type: application/json' \
			-d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.1"}}}' \
			> "$OUTDIR/mcp_sse_init.json" 2>/dev/null
		expect "MCP SSE initialize" '"serverInfo"' "$OUTDIR/mcp_sse_init.json"
		curl -sf -X POST "http://127.0.0.1:$MCP_PORT/message" \
			-H 'Content-Type: application/json' \
			-d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
			> "$OUTDIR/mcp_sse_tools.json" 2>/dev/null
		expect "MCP SSE tools/list" '"tools"' "$OUTDIR/mcp_sse_tools.json"
	else
		fail "MCP SSE server did not come up (see mcp_sse.log)"
	fi
	kill "$MCP_PID" 2>/dev/null; wait "$MCP_PID" 2>/dev/null
	MCP_PID=""
fi

# ─── 14: integrations ────────────────────────────────────────────────────────

section "14/17 — integrations"

check "integrations list" -- integrations list
if $OFFLINE; then
	note "skipped network-dependent probes (--offline)"
else
	soft "integrations ping"              -- integrations ping
	soft "integrations carbon (list)"     -- integrations carbon --action list
	soft "integrations tansu (list)"      -- integrations tansu list
fi
check "integrations news status (unconfigured)"    -- integrations news --action status
check "integrations finnhub status (unconfigured)" -- integrations finnhub --action status

# ─── 15: trade records (in-memory service) ───────────────────────────────────

section "15/17 — trade records"

check "trade strategy (create)" -- trade strategy --type dca \
	--name "E2E DCA" --params "amount_per_order=10,interval_hours=24"
check "trade list"       -- trade list
check "trade performance" -- trade performance
# status/history require a strategy ID positional — usage errors are expected.
check_fail "trade status requires strategy ID"  -- trade status
check_fail "trade history requires strategy ID" -- trade history
# Strategies live in-process only — a fresh CLI cannot start one it never saw.
check_fail "trade start unknown strategy" -- trade start strategy_deadbeef

# ─── 16: network switching + legacy config migration ─────────────────────────

section "16/17 — network switching + ~/.mozartpay migration"

check "network set mainnet (--force)" -- network set stellar-mainnet --force
expect "config switched to mainnet" '"network": "stellar-mainnet"' "$CONFIG_JSON"
check "network set testnet (--force)" -- network set stellar-testnet --force
expect "config back on testnet" '"network": "stellar-testnet"' "$CONFIG_JSON"

# Interactive menu: option 2 is testnet.
printf '2\n' | "$CLI" network switch >/dev/null 2>&1 \
	&& pass "network switch (interactive)" || fail "network switch (interactive)"

# Legacy migration: a fresh HOME holding only ~/.mozartpay must be copied to
# ~/.stellar-go-cli on first config load (the legacy dir is left in place).
MIGR_HOME="$(mktemp -d "${TMPDIR:-/tmp}/stellar-e2ex-migr.XXXXXX")"
mkdir -p "$MIGR_HOME/.mozartpay/state"
printf '{"network":"stellar-testnet","legacyProbe":true}\n' > "$MIGR_HOME/.mozartpay/config.json"
echo "marker" > "$MIGR_HOME/.mozartpay/state/marker.txt"
HOME="$MIGR_HOME" "$CLI" wallet list >/dev/null 2>&1
if [ -f "$MIGR_HOME/.stellar-go-cli/state/marker.txt" ] \
	&& [ -f "$MIGR_HOME/.mozartpay/state/marker.txt" ]; then
	pass "~/.mozartpay migrated (copied, legacy preserved)"
else
	fail "legacy migration (marker not copied)"
fi
rm -rf "$MIGR_HOME"
export HOME="$SANDBOX"

# ─── 17: summary ─────────────────────────────────────────────────────────────

section "Summary"
printf '  %s%d passed%s, %s%d failed%s, %s%d warned%s\n' \
	"$GREEN"  "$PASS"   "$RESET" \
	"$RED"    "$FAILED" "$RESET" \
	"$YELLOW" "$WARNED" "$RESET"
note "artifacts kept at: $OUTDIR"

if [ "$FAILED" -gt 0 ]; then
	exit 1
fi
