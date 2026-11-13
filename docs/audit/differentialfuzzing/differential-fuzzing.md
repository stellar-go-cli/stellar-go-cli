# Differential Fuzzing: stellar-cli vs stellar-go-cli

| | |
|---|---|
| **Status** | Implemented — Phases 1–3 shipped. Local crash/property targets run everywhere; differential targets run against the installed `stellar` oracle and self-skip without it. Phase 4 (e2e localnet) deferred. |
| **Version** | 0.2 — 2026-11-12 |
| **Scope** | `stellar` (Rust, official Stellar CLI — the Soroban contract multi-tool) is the **reference oracle**; `stellar-go-cli` is the **target under test**. |

---

## 1. What differential fuzzing means here

Feed identical inputs to two independent implementations of the same
operation and diff their normalized outputs. A divergence means one side is
wrong — and since `stellar-cli` is the reference implementation, divergence
almost always means *our* side is wrong.

Two distinct disciplines are covered, and the doc keeps them separate:

- **Differential fuzzing** — comparing outputs against the oracle. Catches
  *semantic* divergence (wrong XDR encoding, wrong parse, wrong math).
- **Crash fuzzing** — single-implementation, looking for panics/hangs.
  Catches robustness bugs the oracle can't see (there is no "correct"
  output for malformed input, only "don't crash"). Applies to surfaces
  with no oracle: iso20022, VC, x402, swap, MCP.

---

## 2. Recommended tool

**Primary recommendation: a purpose-built differential harness on Go's
native `testing.F`, following the CDF architecture.**

There is no mature off-the-shelf "differential CLI fuzzer," and the
evaluated alternatives each lose on a specific axis (table below). The
winning combination is already in our toolchain:

```
   go test -fuzz (coverage-guided mutation, corpus mgmt, minimization)
        │
        ▼
  fuzz target (in-process, our Go code)
        │ same input
        ├──▶ our implementation → normalize ─┐
        │                                    ├─▶ compare → divergence?
        └──▶ exec `stellar <cmd>` → normalize ┘      (report + minimize)
```

- **Why `testing.F`**: coverage guidance walks *our* code paths for free;
  seed corpus, crash minimization, and `-fuzztime` CI are built in; zero
  new dependencies (required by `AGENTS.md`); fuzz files live in
  `*_test.go` in whichever module owns the target.
- **Why the CDF pattern** (Crypto Differential Fuzzing, Aumasson 2017):
  language-agnostic driver + a small **adapter per artifact type** that
  runs each implementation → **normalizer/comparator**. CDF itself is
  crypto-ops-specific (hash/RSA/ECDSA interfaces) and unmaintained — we
  adopt the architecture, not the binary.
- **Why a subprocess oracle is acceptable**: `stellar` can't be linked
  from Go, so `exec` is unavoidable for cross-language comparison. It
  costs ~milliseconds per invocation — slower than in-process but fine
  for a semantic oracle. Where `stellar` accepts multiple inputs or
  stdin streams, batch to amortize.

### Alternatives considered

| Tool | What it is | Why not primary |
|---|---|---|
| `difftest` (crates.io) | Runs two programs on the same inputs, diffs stdout/stderr/exit | Shallow input generation, no coverage guidance, no mutation engine — good for canned-case smoke diffs, not campaigns |
| `uufuzz` (uutils coreutils) | Rust library for CLI differential fuzzing, compares stdout/stderr/exit | Correct idea, wrong side of the language boundary — puts our harness in Rust and can't cover Go internals |
| SemSan (LibAFL) | Coverage-guided differential fuzzer, dual executors (fork-server/QEMU) with shared-memory output comparison | Research-grade; needs AFL-instrumented harness binaries for *both* sides; heavy for the marginal gain over `testing.F` |
| Hypothesis (Python) | Property-based strategies + subprocess both CLIs | Nice ergonomics, but adds a Python dependency and gets no coverage guidance inside Go code |
| `cargo fuzz` | Rust-side fuzzing of stellar-cli | Wrong target — `stellar` is the oracle, not the system under test |

### Tiered rollout of tooling

1. **Tier 1 — canned diffs.** A fixed corpus of artifacts (XDR blobs,
   strkeys, tx envelopes, ScVal JSON) replayed through both CLIs and
   diffed — shell loop or `difftest`. Zero code, catches gross divergence
   day one.
2. **Tier 2 — `testing.F` differential harness.** The recommended tool:
   Go's fuzz engine generates/mutates inputs, exercises our decode path
   in-process, and the adapter diffs against `stellar`. This is the bulk
   of the work and the real payoff.
3. **Tier 3 — dual-instrumented fuzzing (SemSan-class).** Only if Tier 2
   saturates and deeper oracle-side exploration is ever needed.

---

## 2.1 Implementation status (v0.2)

| # | Target | Shipped as | Mode |
|---|---|---|---|
| 1 | `CompareScVal`/`ToScVal`/`ScvMap` | `pkg/soroban/fuzz_values_test.go` — `FuzzCompareScVal` (reflexivity/antisymmetry/transitivity/totality), `FuzzToScVal`, `FuzzScMapSortDeterminism` | local |
| 2 | ScVal JSON↔XDR | `pkg/soroban/fuzz_diff_test.go` — `FuzzScValEncodeDifferential`, `FuzzScValDecodeDifferential` | differential |
| 3 | Tx envelope | `FuzzTxEnvelopeDifferential` (Go `HashHex` vs `stellar tx hash`, plus `tx decode` acceptance) | differential |
| 4 | strkey | `FuzzStrkeyDifferential` (accept/reject + payload vs `stellar strkey decode`) | differential |
| 6 | `pkg/iso20022` | `pkg/iso20022/fuzz_test.go` — `FuzzParseDocuments`, `FuzzMarshalUnmarshalRoundTrip` | local |
| 7 | `pkg/x402` | `pkg/x402/fuzz_test.go` — `FuzzDecodeHeaders`, `FuzzParseAtomicAmount` | local |
| 8 | `internal/mcp` | `internal/mcp/fuzz_test.go` — `FuzzCallTool`, `FuzzValidateAndCollect` | local |
| 9 | `internal/config` | `internal/config/fuzz_test.go` — `FuzzConfigLoad`, `FuzzLoadState` | local |

Harness internals (all test-only, `pkg/soroban/`):

- `fuzz_oracle_test.go` — the CDF adapter: `exec` wrappers for `xdr
  encode/decode`, `strkey decode`, `tx hash/decode`; logs and effectively
  pins the oracle via `stellar version` output on every run.
- `fuzz_scval_codec_test.go` — serde-JSON ↔ `xdr.ScVal` codec for the
  generated subset (the Go SDK ships none) + the deterministic
  structure-aware generator (`genDec`/`genScVal`, depth-bounded, duplicate
  map keys fall back to vecs).
- `diff_corpus_test.go` — Tier-1 canned runner: every artifact under
  `pkg/soroban/testdata/diff-corpus/` (`scval-*.json`, `strkey-*.txt`,
  `tx-*.b64`) replays through both implementations, including the
  negative/malformed strkey case.
- `planted_divergence_test.go` — proves the comparators flag corrupted
  oracle output (the Phase-3 "CI fails on planted divergence" gate).

**Observed divergences: one triaged, zero unexplained.** The only defect in
our own code was a harness bug (`ScError` `ContractCode` arm in the codec,
caught by the corpus runner before any real run). The differential targets
surfaced one genuine behavioral divergence, triaged as **SDK strictness,
not a bug**: go-xdr's `UnmarshalBinary` accepts trailing bytes after a
complete value where the Rust decoder rejects the buffer. The decode
target now only diffs inputs whose canonical re-encode is byte-identical
to the input (lenient decodes are logged+skipped); this property is the
documented boundary of the comparison.

Commands: `make fuzz` (all targets, `FUZZTIME` override, oracle-aware),
`make diff-corpus`. CI: `fuzz` job in `ci.yml` (seed corpus + bounded
local targets per PR); `fuzz-nightly.yml` installs `stellar` 28.0.0 and
runs 5–10min/target campaigns incl. differential, uploading
`testdata/fuzz/` artifacts on failure.

**Honest gaps:** batch oracle calls are not implemented (per-invocation
exec — the dominant cost, ~10–60 execs/sec on differential targets vs
~25k/sec local); `stellar xdr encode` does not canonicalize ScMap order,
so `CompareScVal` is verified by ordering laws, not byte-diff; Phase 4
e2e (`stellar container` localnet compare of simulate/invoke outcomes)
remains unimplemented; targets #5 (amount) and VC/swap surfaces are not
yet covered.

---

## 3. Oracle surface map

Every `stellar` subcommand usable as an oracle, mapped to our code path:

| `stellar` command | Oracle for | Our code |
|---|---|---|
| `xdr decode --type <T>` / `xdr encode` | XDR → JSON decoding and JSON → XDR encoding for any XDR type | `pkg/soroban` (helpers, values), SDK `xdr` package use throughout |
| `strkey decode` / `strkey encode` | strkey parse/encode incl. muxed accounts, contract IDs | seed/address handling (`internal/wallet`, SDK `strkey`) |
| `tx decode` / `tx encode` / `tx hash` | Envelope XDR↔JSON, tx hash computation | `pkg/soroban` transaction assembly, `submitAndWait` |
| `tx sign` / `tx simulate` | Signature decoration, simulation request/response shape | `Invoke`/`Deploy` simulate→sign→submit flow |
| `keys` | Seed→address derivation, key handling | `keypair` usage (`internal/wallet/loader.go`) |
| `contract` | Deploy/invoke/inspect argument handling | `cmd/.../contract.go`, `pkg/soroban` invoke/deploy |
| `container` | Local network for e2e differential runs | RPC client paths (`pkg/soroban/client.go`) |
| `message` (SEP-53) | Message sign/verify | any message-signing path we add |
| `snapshot` / `ledger` / `events` | Fetch real ledger artifacts for seed corpus | corpus generation only |

**Non-overlapping surface** (crash-fuzz only, no oracle): `pkg/iso20022`
XML parse/marshal, `pkg/vc` DID/VC parsing, `pkg/x402` types/client,
`pkg/swap` quote math, `internal/mcp` JSON-RPC + parameter collection,
`internal/config`/state loading.

---

## 4. Ranked fuzz targets

| # | Target | Why first | Oracle? |
|---|---|---|---|
| 1 | `CompareScVal` + `ToScVal` + `Scv*` (`pkg/soroban/values.go:37,312`, `helpers.go:85-147`) | `CompareScVal` defines the map-key sort order — a divergence silently mis-encodes every ScMap we build; AGENTS.md claims Rust-`soroban-sdk` identical ordering, which makes it *the* thing to verify differentially | `stellar xdr` on ScVal/ScMap |
| 2 | ScVal JSON↔XDR | Contract args/returns; encoding bugs are silent corruption | `stellar xdr decode/encode --type ScVal` |
| 3 | Transaction envelope build/sign | Wrong envelope = rejected or mis-signed txs; byte-strict oracle when source/seq/fee are pinned | `stellar tx decode/encode/hash` |
| 4 | strkey edge cases | Muxed `M...`, contract `C...`, claimable `B...`, malformed padding/checksums | `stellar strkey` |
| 5 | Amount/decimal handling | Classic bug spot — precision, rounding, overflow | `stellar tx`/`token` paths |
| 6 | `pkg/iso20022` XML parse/marshal | Round-trip invariants; injection/malformed-XML robustness | none — crash-fuzz |
| 7 | `pkg/x402` types/payload parsing | 402 response parsing, amount/cap handling | none — crash-fuzz |
| 8 | `internal/mcp` JSON-RPC + `parameter_collection.go` | Malformed requests, schema bypass (the fail-open path noted in `mcp-signer-architecture.md` §2.2) | none — crash-fuzz |
| 9 | `internal/config` / state file loading | Corrupt/truncated JSON, pathological values | none — crash-fuzz |

---

## 5. Strategies

- **Structure-aware generation, not raw byte mutation.** XDR is framed —
  random bytes die at the first length field. Generate valid-ish values
  through the SDK types (`xdr.ScVal`, `Transaction`, `Strkey`), mutate
  *fields*, then re-encode. The fuzz engine still mutates bytes at the
  corpus layer; the target converts bytes → structured input via a
  deterministic decoder first.
- **Seed corpus from real artifacts.** `stellar tx fetch`, `stellar
  snapshot`, and recorded testnet traffic give realistic XDR/strkey/JSON
  seeds. Minimized crash corpus lives in `testdata/fuzz/<Target>/` and is
  committed.
- **Properties worth asserting** (beyond "no divergence"):
  `CompareScVal` is a total order (antisymmetry, transitivity, totality —
  a partial order breaks map encoding); encode→decode→encode is the
  identity; parse never panics.
- **Determinism pinning.** Timestamps, sequence numbers, base fees, and
  network passphrase must be fixed inputs or excluded from comparison —
  otherwise the diff is noise.
- **Batch oracle calls.** Where `stellar xdr decode` accepts multiple
  inputs/stdin, amortize subprocess cost; keep per-invocation exec as the
  fallback.
- **Mock the network.** Simulate/invoke paths get fixture RPC responses —
  fuzz must never hit the live network.

---

## 6. Phased adoption plan

| Phase | Scope | Acceptance |
|---|---|---|
| **1 — Local fuzz targets** | `testing.F` targets for targets #1, #6, #8 (no oracle needed to start); `testdata/fuzz/` dirs; Makefile `fuzz` target | `go test -fuzz` runs; seed corpus committed |
| **2 — Differential harness** | Oracle adapter (`exec stellar …`) + per-artifact normalizer for ScVal/XDR/strkey/tx (targets #2–#5); Tier-1 corpus diff runner | A divergence produces a failing, minimized case |
| **3 — CI gate** | `-fuzztime` bounded run per PR (short) + nightly longer campaign; divergence artifacts uploaded | CI fails on a planted divergence |
| **4 — E2E differential** | `stellar container` local net + our RPC client: same ops → compare simulate/invoke outcomes | Script runs against local net |

Acceptance criteria for the program: zero unexplained ScVal/XDR
divergences; `CompareScVal` proven a total order over the generated
corpus; crash corpus regressions covered by unit tests.

---

## 7. Caveats and non-goals

- **Two-module layout.** Fuzz files go in `pkg/` or root matching the
  code under test; `go test -fuzz` must run per module.
- **Subprocess latency.** The oracle call dominates iteration cost —
  batch, and keep oracle-fuzzing on fewer targets than crash-fuzzing.
- **The oracle isn't infallible.** A divergence is a *finding to triage*,
  not automatically our bug — `stellar` versions matter (pin the oracle
  version in the harness).
- **Non-goals**: fuzzing `stellar-cli` itself; coverage instrumentation
  of the Rust binary; fuzzing network-facing servers.

## See also

- [`mcp-signer-architecture.md`](mcp-signer-architecture.md) §5–6 — the
  `callTool` choke point and transport surfaces that become fuzz targets
  (#8).
- [`specula-tla-formal-verification.md`](specula-tla-formal-verification.md)
  — complementary technique: model-checking state machines vs input-space
  fuzzing.
