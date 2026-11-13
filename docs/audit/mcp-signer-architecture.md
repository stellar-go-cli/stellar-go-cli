# MCP / Signer Separation Architecture

| | |
|---|---|
| **Status** | Design + roadmap. Sections marked *current* reflect shipped code; *target* is unimplemented design. |
| **Version** | 0.9 — 2026-11-12 |
| **Companion** | [`agent-policy-engine.md`](agent-policy-engine.md) — the implementation-agnostic spec this realizes (spec § references throughout point there) |

---

## 1. Purpose

The PME spec defines the trust model: the agent owns nothing; a **gatekeeper**
(the MCP server) analyses requests and decides; a **custodian** (a signer
daemon) alone holds keys and independently enforces. This document maps that
model onto the actual codebase — what exists today, what violates the model,
and the phased path to conformance.

| Spec concept | Repo realization (target) |
|---|---|
| Gatekeeper (PDP) | `stellar-go-cli mcp` — request intake, analysis, decision |
| Custodian (PEP) | new `stellar-go-cli signer` daemon — owns `~/.stellar-go-cli`, keyring items, grants, ceremonies, audit log |
| Channel | Unix socket `signer.sock` (0600 + peer-UID check) or inherited stdio |
| Decision pipeline | single `callTool` choke point used by both transports |
| Mandates | `pkg/vc` credentials + `internal/webauthn` issuance ceremonies |
| Trusted surface | existing localhost WebAuthn RP, extended to transaction approval |
| Records | append-only hash-chained `state/audit.log`, custodian-owned |
| Evidence carriers | `pkg/iso20022` `InitgPty`/`SplmtryData`; on-chain tx hash linkage |

---

## 2. Current state (as-is)

### 2.1 Single trust domain

`stellar-go-cli mcp` builds one `Server` holding config, the wallet service,
the tool registry, handlers, and both transports in a single process
(`internal/mcp/server.go:57-91`). The MCP front-end and key custody are the
same security principal — spec §8.2 K1–K3 unmet.

Consequences:

- **Seed reachability.** *Remediated in Phase 3.* `handlePayX402` now
  signs via `custodian.RemoteSigner` over the Unix socket — the gatekeeper
  process never loads a keypair (`internal/mcp/handlers.go`,
  `internal/mcp/custodian.go`). Dry-run previews resolve only the payer's
  *address* (`pkg/x402/stellar.go`). The custodian refuses to sign when its
  socket is absent — there is no in-process fallback.
- **Grant exposure.** *Remediated in Phase 3.* Unlock grants live in
  custodian memory (`internal/custodian/grants.go`) — the OS-keyring
  `unlock-*` items are gone. The gatekeeper cannot reach grants; only the
  custodian process holds decrypted seeds, and only for the grant TTL.
  *Residual:* peer authentication is same-UID (`SO_PEERCRED` /
  `LOCAL_PEERCRED`), so other processes running as the same OS user can
  still *request* signatures — the win is that they can no longer read the
  seed or the grant. Full different-principal isolation is the Docker/ops
  deployment step.
- **Plaintext default.** `wallets/<addr>.json` stores `privateKey` unencrypted
  (0600) until the user opts into `wallet passkey register`
  (`internal/wallet/service.go`, `pkg/models/types.go`). *Residual:* the
  custodian reads these files directly; `allow_unprotected` in the policy
  file gates whether it may sign for plaintext wallets at all.
- **Credential keys ride the seed.** *Remediated post-v0.7.1 for protected
  wallets* — `did attest --use-wallet-key` on a passkey-protected wallet now
  calls `sign_credential`: the custodian derives the wallet's VC key and
  signs inside the daemon; foreign-issuer DIDs are refused. Plaintext
  wallets still sign locally on the trusted surface (JWS/ECDSA issuers are
  plaintext-only for now — custodian `sign_credential` is Ed25519 did:key).

### 2.2 Agent-facing surface

- **All tools, one process.** *Partially remediated post-v0.7.1* — the
  `--profile` flag (`mcp --profile readonly|transact|admin`) filters the
  registry by the §5.2 tier map (`internal/mcp/tiers.go`), and unclassified
  tools fail closed. The default remains `admin` (full surface) and the
  profile is still host-wide, not per-agent — per-agent enrolment is
  Phase 3 (spec §9.4 partially met).
- **Network exposure.** *Remediated.* The HTTP transport binds `127.0.0.1`
  by default and refuses non-loopback binds without a bearer token
  (`internal/mcp/server.go`); `/sse` and `/message` are gated by
  `guardTransportRequest` — Host must target the bind address or loopback
  and `Origin`, when present, must be loopback or operator-allowlisted
  (`--allowed-origins`/`MCP_ALLOWED_ORIGINS`) — and the SSE endpoint echoes
  an allowlisted origin instead of `*`. Compose publishes
  `127.0.0.1:3000` with a mandatory `MCP_TOKEN`.
- **Legacy transport.** The server speaks the superseded HTTP+SSE transport
  (protocol version `2024-11-05`), not the current Streamable HTTP transport —
  Origin/auth are now enforced on the legacy transport; migration to
  Streamable HTTP remains open.
- **Duplicated dispatch.** *Remediated post-v0.7.1.* Both transports funnel
  through one `callTool` choke point
  (`internal/mcp/server.go`) — validation, dispatch, and audit are shared;
  the HTTP path propagates the request context into the handler.
- **Fail-open parameter validation.** *Remediated post-v0.7.1.* Dispatch
  validates against each tool's registered `InputSchema` — required params,
  argument types, and enum membership — and a tool without a valid object
  schema cannot be dispatched (`parameter_collection.go`,
  `validateAndCollect`). `ParameterSchemas` survives only as prompt text.
- **Secret-bearing tools.** *Remediated post-v0.7.1.* `wallet_import` and
  `wallet_export` are removed from the agent registry entirely — they exist
  only as operator CLI commands (`stellar-go-cli wallet import|export`),
  and are deliberately absent from the tier map so re-registration fails
  closed (`internal/mcp/tools.go`, `internal/mcp/tiers.go`).
- **Agent-steerable endpoints.** *Partially remediated post-v0.7.1.*
  `rpc_url` and `allow_private_resource` moved out of the agent schema to
  operator options (`mcp --x402-rpc-url`, `--allow-private-resources`,
  `OperatorOptions` in `internal/mcp/tiers.go`); `response_body` is now
  returned with an explicit untrusted-content label. `pay_x402` still
  accepts `resource` (arbitrary URL) and `headers` under the SSRF guard —
  that is the tool's intended function — and the DNS TOCTOU between
  check-time resolution and fetch remains (`internal/mcp/ssrf.go:20-22`).
- **Passkey ceremonies over HTTP.** *Remediated in v0.7.1* —
  `wallet_passkey` `register`/`unlock`/`disable` are refused on the HTTP
  transport (`internal/mcp/handlers.go:900-909`, `isHTTPTransport`),
  closing the remote prompt-bombing vector.
- **Dry run still signs.** *Remediated.* Dry-run fetches build an unsigned
  preview payload via `CreateUnsignedPaymentPayload`
  (`pkg/x402/stellar.go`), run through a key-free address-only builder
  (`NewStellarPreviewBuilder`/`NewStellarPreviewClient`) backed by
  `signer.AddressSigner`, which structurally refuses to sign
  (`pkg/signer`). `execute=true` requires `max_atomic_amount`, clamped to
  the operator hard cap `--x402-max-amount` when set — effective cap
  min(request, operator). Residual: a `PaymentBuilder` not implementing
  `unsignedPayloadBuilder` still falls back to the signed path on dry-run
  (`pkg/x402/client.go`). Per spec §7.4 a signed authorization is a bearer
  instrument — the T3 classification in §5.2 stands for any signing call.
- **Metadata inconsistency.** *Remediated post-v0.7.1.* Validation is now
  driven by each tool's registered `InputSchema`, so there is one source of
  truth; the stale `execute: true` defaults in `ParameterSchemas` were
  corrected.
- **Config mutation by agent.** *Partially remediated post-v0.7.1.*
  `handleSystemNetwork` refuses network switching on the HTTP transport
  outright and requires `confirm=true` on stdio (`handlers.go`). The write
  itself is still an agent-reachable mutation on stdio — by design at T1,
  now gated.
- **No decision stage.** *Partially remediated post-v0.7.1.* Dispatch still
  has no ALLOW/DENY/ESCALATE engine (Phase 3), but the single `callTool`
  path now classifies tiers at registration (`internal/mcp/tiers.go`) and
  appends a hash-chained JSONL record of every `tools/call` — including
  denials — to `state/audit.log` (`internal/mcp/audit.go`); argument values
  are stored as digests plus key names only.

### 2.3 Deployment

- Images run as **root** (`WORKDIR /root/`, no `USER` directive in
  `Dockerfile`/`Dockerfile.mcp`).
- Alpine images ship no Secret Service/dbus — passkey protection cannot
  function in-container; any wallet created there is necessarily plaintext
  and (no state volume is defined) ephemeral.
- If a state volume is later mounted, it lands in the same container as the
  unauthenticated network listener — worst of both worlds.

### 2.4 Assets already aligned with the model

- `ResolveSeed` is the single choke point for all seed reads
  (`internal/wallet/passkey.go:369-385`) — custody is already centralized.
- `pkg/signer` is the signing-oracle package — `TxSigner`,
  `AuthEntrySigner`, `KeypairSigner` (in-process adapter), and
  `AddressSigner` (key-free preview that structurally refuses to sign).
  `pkg/soroban`, `pkg/x402`, and `pkg/swap` all dispatch through it
  (`pkg/swap.Signer` is now an alias) — the exact seam the custodian
  plugs into.
- `internal/webauthn` hosts a localhost RP with real browser ceremonies — the
  trusted surface already exists.
- `ValidateAndCollectParameters` already intercepts every `tools/call` before
  dispatch (`parameter_collection.go:125`) — the natural insertion point for
  the decision stage (once made fail-closed).
- `LoadStellarKeypairForAddress` verifies loaded key ↔ intended address
  (`internal/wallet/loader.go:151-153`) — spec K9 partially met.
- `pkg/x402` validates the signed authorization entry against contract,
  method, source, destination, amount, and payer before use — a working
  precedent for the custodian's E4 decode-and-match.
- `pkg/vc.VerifyDocument` preserves `credentialStatus`/`evidence` fields when
  hashing externally-issued VCs (`pkg/vc/service.go:306-327`) — mandate VCs
  with status-list revocation verify correctly; status *resolution* is the
  remaining piece.
- `pkg/iso20022` models `InitgPty` and `SplmtryData` — the carriers spec §12.3
  maps mandate/evidence references onto.
- Releases ship cosign signatures, checksums, and SBOMs per artifact
  (`.goreleaser.yml`) — the K14 integrity basis.

---

## 3. Target architecture

```
  AI client (agent host)                        principal / operator (browser)
        │ MCP: stdio, or Streamable HTTP              ▲ passkey ceremony
        │ on 127.0.0.1 + Origin check + auth          │ (loopback WebAuthn RP)
        ▼                                             ▼ reached via URL-mode elicitation
  ┌──────────────────────────┐                  ┌──────────────────────────┐
  │ stellar-go-cli mcp       │   unix socket    │ stellar-go-cli signer    │
  │ GATEKEEPER (PDP)         │   0600 + peer    │ CUSTODIAN (PEP)          │
  │ · single callTool path   │   credentials    │ · state dir, KEK, grants │
  │ · tiers, mandate check   │─────────────────▶│ · E1–E5 enforcement      │
  │ · decide ALLOW/DENY/ESC  │  sign_* request  │ · hosts ceremonies       │
  │ · filters responses      │◀─────────────────│ · owns audit log         │
  │ · holds no secrets       │ signed or refused│ · authoritative policy   │
  └────────────┬─────────────┘                  └────────────┬─────────────┘
               │ read-only: Horizon, quotes,                 │ Soroban RPC, submission
               │ x402 fetch (egress policy)                  │ (operator endpoints only)
               ▼                                             ▼
        external services                             Stellar network
```

Trust boundaries:

1. **Agent ↔ gatekeeper** — MCP transport; everything inbound is untrusted
   input.
2. **Gatekeeper ↔ custodian** — peer-authenticated local channel (Unix socket
   + `SO_PEERCRED`/`LOCAL_PEERCRED`, or inherited stdio). Carries decisions
   and signatures, never keys.
3. **Custodian ↔ principal** — trusted surface only (browser ceremony); the
   agent can initiate but never answer.
4. **Outbound** — read-only queries and gated fetches from the gatekeeper
   under egress policy; signing and submission only from the custodian, to
   operator-configured endpoints.

The gatekeeper runs the full §7 pipeline and produces the decision — "MCP
takes the request, analyses it, and decides." The custodian runs *only* the
E1–E5 invariants (spec §5.2): it is deliberately too simple to be steered.

### 3.1 Custodian socket API

| Call | In | Out | Guard |
|---|---|---|---|
| `sign_tx` | decision ref + unsigned envelope | signed envelope or refusal | E1–E5; decodes ops/amounts/counterparties/passphrase vs bound context |
| `sign_soroban_auth` | decision ref + auth entry | signed entry or refusal | E1–E5; validates contract/method/args like x402 does today |
| `sign_credential` | unsigned VC + issuer (did or method+proof) | signed VC + issuer_did or refusal | issuer must derive from the wallet's key (ed25519 + ECDSA); grant or mandate authority |
| `submit_tx` | signed envelope XDR | tx hash + ledger | only custodian-signed payloads (digest match, single-use) |
| `assert` | ceremony_id + assertion JSON | bound action result | single-use, 2-min TTL pending ceremonies |
| `issue_mandate` | mandate request | mandate VC or refusal | always a principal ceremony (spec §6.3) |
| `set_mandate_status` | mandate ref + suspend/revoke | updated status | revoke/suspend: always allowed (P6); reinstate: ceremony |
| `pubkey` | account ref | `G...` address | no secret material |
| `unlock` / `lock` | account ref | grant state | `unlock` → ceremony; `lock` → immediate |
| `status` | account ref | protection/grant/limits state | read-only |
| `append_record` | decision record | chain hash | custodian appends to its own log |

There is deliberately **no `get_seed`, `export_key`, or `decrypt`** call — at
any privilege level (spec K2).

---

## 4. Key management design (target)

Mapping to spec §8:

| Req | Design |
|---|---|
| K1–K3 custody | `stellar-go-cli signer` daemon owns `~/.stellar-go-cli/state`, keyring items (`wallet-key-*`, `unlock-*`), and `ResolveSeed`, under a different principal than `stellar-go-cli mcp`. The agent-facing binary drops seed-path imports entirely. |
| K2 sign-only | Socket API above — no export call. `wallet_export`/`wallet_import` move to the operator CLI (trusted surface), never the agent channel. |
| K4 grants | `unlockGrant` moves from the shared OS keyring into custodian memory (or a custodian-exclusive keyring). Gatekeeper can't reach it at OS level; daemon restart re-locks everything. |
| K5 at rest | Keep AES-256-GCM `PrivateKeyEnc` + keyring KEK; flip the default so new wallets are protected unless the operator opts out (dev/testnet flag). |
| K6/K7 in use | Effective cap = min(mandate, hard limit). TTL grants retained for not-present flows; transaction-bound ceremonies for escalations — challenge commits to the request hash. |
| K8/K9 | Seed buffers wiped post-sign inside the daemon; keep the address-match check; passphrase bound via `tx.Sign` as today. |
| K10–K12 | Import only via CLI ceremony, scrubbed from records; documented `S...`-seed recovery path exercised on a schedule; `RemoveWallet` already deletes file + keyring + passkey records (`service.go:705-721`) — extend to custodian grants. |
| K13 | Mandate revocation independent of account key; account-key compromise → documented re-issue/migrate flow. |
| K14 | Custodian binary verified from cosign signatures + SBOM (existing release machinery). |

### The `Signer` interface

*Phase 2 seam landed post-v0.7.1:* `pkg/signer` defines the shared
abstraction — `TxSigner` (`Address()`, `Sign(tx, passphrase)`),
`AuthEntrySigner` (`SignAuthEntry`), and the combined `Signer`.
`pkg/soroban` (`Invoke`, `Deploy`, `submitAndWait`), `pkg/x402`
(`NewStellarClient[WithRPCURL]`, `StellarExactBuilder`), and `pkg/swap`
(via `swap.Signer` as an alias) all dispatch through it; wallet loaders
produce signers via `internal/wallet` `LoadStellarSigner*`.

Two implementations today: `KeypairSigner` (in-process — CLI and tests;
satisfies K2's shape only) and `AddressSigner` (address-only, refuses to
sign — the key-free dry-run path used by `NewStellarPreviewClient`, so
previews are key-free end to end). `RemoteSigner` (socket client —
satisfies K3 when the daemon runs under a separate principal) is the
remaining Phase-3 piece; call sites stay identical and the binary mode
picks the implementation.

---

## 5. Decision pipeline implementation (target)

### 5.1 Single `callTool` choke point

Both transports currently duplicate dispatch. Collapse them into one
`callTool(ctx, agentIdentity, params)` that always runs:

```
intake    → schema validation (fail-closed: unknown tool → error,
            missing schema → error, not silent allow)
analysis  → bind context (account, network, counterparty); classify tier;
            resolve mandate/grants; provenance flags
decision  → policy.Decide → ALLOW | DENY | ESCALATE
enforce   → dispatch handler (T0/T1) or custodian sign_* call (T3) 
evidence  → append decision record; custodian appends enforcement record
response  → strip secrets; label third-party content untrusted
```

```go
decision, err := s.policy.Decide(ctx, PolicyRequest{
    Agent:   agentIdentity,
    Tool:    params.Name,
    Args:    params.Arguments,
    Tier:    tierOf(params.Name),
    Bound:   boundContext,          // resolved account/network/counterparty
    Mandate: verifiedMandate,       // VC-verified or nil
})
```

### 5.2 Tool risk tiers over the current registry

| Tier | Tools |
|---|---|
| **T0 read** | `wallet_list`, `wallet_show`, `wallet_balance`, `wallet_assets`; `swap_quote`, `swap_assets`, `swap_scan`, `swap_monitor`, `swap_arbitrage_scan`, `swap_arbitrage_all`, `swap_triangular` (scan/monitor/backtest actions); `asset_list`, `asset_info`, `asset_score`, `asset_show`; `pay_request`, `pay_history`, `pay_quote`, `pay_rails`; `system_status`, `system_health`, `system_network` (read form); `memory_get`, `memory_list`, `memory_search`; all `tansu_*` |
| **T1 low-impact mutation** | `wallet_rename`, `memory_save`, `memory_delete`, `wallet_fund` (testnet faucet), `system_network` (switch form — recorded; flipping to mainnet SHOULD escalate) |
| **T2 key/security lifecycle** | `wallet_connect`, `wallet_switch`, `wallet_import`, `wallet_export`, `wallet_remove`, `wallet_passkey` (all actions) |
| **T3 value transfer** | `pay_send` (execute), `pay_x402` (**any** call that signs — dry-run produces an unsigned preview since v0.7.1), `pay_zk`, `swap_execute`, `swap_triangular` (execute action), `asset_trust` (execute), `asset_create_ft`, `asset_create_nfa`, `asset_carbon` |

### 5.3 x402-specific rules

- Remove `rpc_url` from the agent schema — RPC endpoint is operator config.
- Keep `max_atomic_amount`, but the effective cap is `min(request, mandate,
  hard limit)`.
- Wrap `response_body` in an explicitly-labelled untrusted field with a cap;
  provenance-flag it for downstream analysis.
- Resource URL fetched under §9.5 egress policy (deny private ranges unless
  allowlisted).
- Route through `sign_soroban_auth`; the existing x402 validation becomes the
  custodian's E4 implementation.

### 5.4 Policy artefact

Operator-owned; the custodian holds the authoritative copy, the gatekeeper a
read-only view for decisions:

```json
{
  "version": "pme-2026-11",
  "profiles": { "readonly": ["T0"], "transact": ["T0","T1","T3"], "admin": ["T0","T1","T2","T3"] },
  "hard_limits": {
    "per_tx": { "XLM": "500" }, "per_day": { "XLM": "2000" },
    "networks": ["stellar-testnet"], "deny": []
  },
  "escalation": {
    "tiers": ["T2"], "t3_above": { "XLM": "50" },
    "new_counterparty": true, "inconclusive": true, "timeout_sec": 300
  },
  "allowlists": { "destinations": [], "contracts": [], "resource_hosts": [] },
  "endpoints": { "rpc": "...", "horizon": "..." },
  "audit": { "path": "state/audit.log", "hash_chain": true }
}
```

---

## 6. Transport and deployment

| Transport | Current | Target |
|---|---|---|
| stdio | inherited streams | unchanged — preferred agent transport |
| HTTP | legacy HTTP+SSE, `127.0.0.1` default bind + bearer auth required for non-loopback (v0.7.1), `Origin` unchecked, CORS `*` | Streamable HTTP, `127.0.0.1` default (flag to widen), auth per connection, `Origin`/`Host` validation, restricted CORS |
| gatekeeper↔custodian | same process | Unix socket `signer.sock` (0600, peer-UID) or spawned stdio; never a network listener |

**Docker** (`docker-compose.yml`): two services — `mcp` without the state
volume (and ideally non-root, since it holds no secrets), `signer` with the
state volume and a shared socket mount. The Alpine/no-Secret-Service issue
means container custody needs a non-keyring KEK source (vault transit, KMS,
or mounted secret file owned by the signer UID) — an open decision, §10.
Publishing `3000` stays loopback-bound even inside compose until auth lands.

---

## 7. Ledger-level enforcement (defence in depth, optional)

Software policy can be complemented by on-chain constraints:

- **Budget accounts** — fund a segregated agent account with a capped balance;
  cumulative spend is limited by consensus, not just policy.
- **Multisig co-signing** — the agent account requires the custodian's (or the
  principal's) signature above thresholds.
- **Soroban custom accounts** — `__check_auth` can enforce per-transaction and
  per-period limits on-chain; combined with `secp256r1verify` (CAP-0051), a
  passkey credential can sign directly — the strongest realization of K7.

These are optional hardening, noted because the on-chain layer is the only
boundary an off-chain compromise cannot weaken.

---

## 8. Roadmap

| Phase | Scope | Satisfies | Effort |
|---|---|---|---|
| **Phase 1 — Surface hardening** | Loopback bind + auth + mandatory x402 spend cap + SSRF guard + passkey-ceremony block on HTTP (**shipped v0.7.1, GHSA-x2fr-2mv2-8fc8**); `Origin`/`Host` validation + restricted CORS, single `callTool` path with request-context propagation, fail-closed `InputSchema` validation, `--profile` filtering, `wallet_export`/`wallet_import`/`rpc_url`/`allow_private_resource` removed from agent reach, `response_body` labelled, operator x402 cap, `asset_trust`/`swap_execute` metadata fix, hash-chained `tools/call` audit log, `system_network` HTTP refusal (**shipped post-v0.7.1**) | C7, C8; tiers; §12 records | Small — no new components |
| **Phase 2 — Signer abstraction** | `pkg/signer` (`TxSigner`/`AuthEntrySigner`/`KeypairSigner`/`AddressSigner`); all `keypair.Full` call sites converted; key-free dry-run path (**shipped post-v0.7.1**) — remaining: `RemoteSigner` socket client | K2 shape; enables Phase 3 | Medium — mechanical refactor over ~8 call-site packages |
| **Phase 3 — Custodian daemon** | `stellar-go-cli signer` daemon + NDJSON socket protocol (`internal/custodian/`); grants in custodian memory; `RemoteSigner`/`RemoteWallet` client; gatekeeper drops `internal/wallet` entirely; `state/policy.json` E1–E5 enforcement with hash-chained `enforcement.log`; `wallet passkey approve` for challenge-bound escalation; `wallet passkey` ops + `wallet export` rerouted; `signresolve` picks `RemoteSigner` for protected wallets across all services; Docker split (`signer` + `mcp` services, shared socket volume, non-root) (**shipped post-v0.7.1**) | C1, C2, C4, C5, C9, C10; K1–K9; spec §5, §9 | Large — new daemon + protocol |
| **Phase 4 — Delegation & evidence** | Mandate VCs (`MandateCredential`) per spec §6.1 — open (standing) + closed (payload-digest, single-use); holder binding via gatekeeper `did:key` + Ed25519 PoP over `sha256(tool|account|payloadXDR)`; local custodian status registry (active/suspended/revoked, checked at every decision); mandate-scoped seed escrow for protected wallets (TTL = validity, dropped on revoke); `issue_mandate`/`set_mandate_status`/`list_mandates`/`sign_credential` socket methods; mandate-aware E2 + per-mandate cumulative counters; `wallet mandate` CLI; `did attest --use-wallet-key` on protected wallets signs inside the custodian; sub-delegation prohibited per spec §4.4 (**shipped post-v0.7.1**). *Deferred:* ISO 20022 mapping, bitstring status-list credential export, conformance vectors (C1–C14) | C3–C6, C11; spec §6, §12.3 | Large — new credential type + verifier |
| **Phase 5 — Governance outputs** | Impact-assessment template (spec §13.2); revocation-latency measurement; policy review workflow + enforced approval gate; Class-M evidence pack; conformance suite C1–C14; bitstring status lists; ISO 20022 evidence (**shipped**). *Deferred:* optional ledger-level controls (§7) | M1–M5; spec §11–15 | Medium — shipped |

Ordering rationale: Phase 1 closes the live exposure (unauthenticated network
listener + full tool surface) with no architectural dependency — the
listener half shipped as the v0.7.1 security patch under
GHSA-x2fr-2mv2-8fc8 and the tool-surface half shipped post-v0.7.1
(`callTool` choke point, fail-closed validation, profiles, audit log).
Phase 2's interface and key-free path have landed; `RemoteSigner` over the
custodian socket has landed too (Phase 3). Phase 4 has shipped: mandate
VCs with holder-bound PoP, mandate-scoped escrow, custodian `sign_credential`,
and a `wallet mandate` CLI. Residual fixes shipped post-v0.7.1:

- **Cross-principal socket auth** — the socket now requires a `signer.token`
  (generated 0600 next to the socket; `STELLAR_SIGNER_TOKEN` env override).
  Peer-UID mismatch is advisory-only: token-bearing cross-principal clients
  (Docker split, second operator account) work; same-UID processes without
  the token are refused.
- **Submission inside the custodian** — `submit_tx` submits only envelopes
  the daemon itself signed (sha256 payload digest must match a recorded
  `signed:` decision, single-use), records `submitted:<hash>`, and returns
  the tx hash. `internal/payments` and `pkg/swap` auto-route remote-signed
  submissions through it (signer `Submit` interface). Soroban submissions
  stay caller-side — they need RPC `getTransaction` polling for result meta
  XDR that Horizon doesn't return; x402 settlement is facilitator-mediated
  by protocol.
- **Headless ceremonies** — ceremonies are relayed: the daemon builds the
  WebAuthn session (`challenge`-style flow inside `unlock`, `confirm`,
  `passkey_register`, `issue_mandate`, mandate reactivation) and the
  CLIENT machine runs the browser ceremony (`internal/webauthn`
  `RelayServer` + `assert`). The daemon never opens a browser — a headless
  custodian gets full passkey flows.
- **ECDSA credential signing** — `sign_credential` derives both key classes:
  ed25519 did:key and ECDSA did:key/did:ethr/did:ebsi (`issuer_method` +
  `proof_type` params; exact `issuer_did` still verified against
  wallet-derived candidates). JWS credentials for protected wallets now
  sign inside the daemon.
- **Offline JSON-LD** — `pkg/crypto` vendors the immutable W3C/W3ID contexts
  (credentials v2, examples v2, ed25519-2020, jws-2020, did v1) into a
  shared caching document loader — VC issuance/verification no longer hits
  the network.

Phase 5 residuals closed:

- **Bitstring status lists** (§11.2) — each mandate issuer gets signed
  `BitstringStatusListCredential` revocation + suspension lists, re-issued
  on every status change; mandate VCs embed `credentialStatus` entries at
  issuance; `status_list` socket method + `wallet mandate statuslist` export.
- **ISO 20022 evidence** (§12.3) — `AgentEvidence` element in pacs.008
  `SplmtryData` (mandate_ref, decision_ref, record hash, policy stamp,
  agent DID, tx hash); `InitgPty` names the agent; `audit evidence` exports.
- **Policy approval gate** (§11.3) — widening diffs refuse daemon start
  until `signer policy approve`; narrowing auto-approves; approvals logged.
- **Conformance suite** — C1–C14 automated (`conformance_test.go` in
  custodian + mcp); C14 anchors `self:<sha256>` of the daemon binary in the
  enforcement chain. **C1 caught a real regression**: `internal/mcp` pulled
  `internal/wallet` transitively through `internal/custodian` — fixed by
  splitting the socket client into `internal/signsock` (no wallet import);
  the gatekeeper binary no longer contains a code path to key material.
- **Governance pack** — `docs/audit/governance/` (scope, impact assessment,
  policy review, control mapping, audit procedure) + `audit` command group
  (verify / events / latency / inventory / evidence).

Spec-conformance finish (post-v0.8):

- **§4.4 sub-delegation** — the spec now prohibits it outright (mandates
  never carry delegation permission; agents return to the principal for new
  mandates), matching the shipped refuse-by-design enforcement.
- **§10.3 escalation fatigue controls** — escalation requests are
  rate-limited per account (`policy.json` `escalation.rate`, default
  5/10min); over-limit refusals are distinct §11.4 events. The approval
  surface shows the decoded request via `request_info` before the bound
  ceremony and banners values flagged `untrusted` (x402/merchant-dictated).
- **§11.1 agent lifecycle** — custodian registry (`state/agents.json`):
  `enrolled → active → suspended → retired`; mandate issuance auto-enrols;
  only `active` agents exercise mandate authority (E2); retire cascades
  mandate revocation; `signer agent` CLI + `audit inventory` coverage.
- **§11.4 incident signals** — `audit events` now scans both logs: refusal
  and verification-failure events, denial bursts, `grant-outside-window`
  (grant_age beyond a session window), `excluded-tool-probing` (repeated
  deny_unknown_tool on audit.log), `escalation-rate-limited`, and
  `chain-failure` (either hash chain broken).

Remaining: a same-UID process that deliberately reads the operator's
`signer.token` can still reach the socket (OS-limited — the Docker split
is the real boundary); Soroban submissions stay caller-side (RPC
result-meta polling); optional ledger-level controls (§7) not implemented.

Non-goals for these phases: changing wallet file formats beyond the
protection-by-default flag, replacing the keyring dependency, on-chain
smart-account custody (§7 notes it as a future direction only).

---

## 9. Traceability

| Spec section | Implemented by |
|---|---|
| §4 social model | Phase 3 split + Phase 4 mandates |
| §5 decision authority (E1–E5) | Phase 3 custodian enforcement |
| §6 mandates | **shipped** — `mandate.go` + `delegation.go` + `IssueVCWithExtras`; bitstring status-list export shipped |
| §7 pipeline, outcomes, tiers | Phase 1 `callTool` + profiles; Phase 3 policy engine; this doc §5 |
| §8 key management | Phase 2 signer interface; Phase 3 daemon custody; this doc §4 |
| §9 transports/profiles/egress | Phase 1 transport + profiles + egress policy; this doc §6 |
| §10 human oversight | existing `internal/webauthn` + Phase 3 ceremony binding |
| §11 management plane | **shipped** — status lists (§11.2), policy approval gate (§11.3), `audit events` signals (§11.4) |
| §12 audit/evidence | **shipped** — hash-chained logs + `AgentEvidence` ISO 20022 export + `audit verify` |
| §13 governance | **shipped** — `docs/audit/governance/` evidence pack + templates |
| §15 conformance | Phase 5 test vectors + evidence |

## 10. Open questions and implementer notes

**Open decisions**

- Headless deployments: browser ceremonies don't work without a local
  browser — options are custodian-only mode (human-not-present under a
  mandate — now shipped: a covering mandate authorizes signing without any
  live session grant) or a remote trusted surface; document the posture
  per deployment.
- ~~Agent identity mechanism~~ — picked and shipped in Phase 4: the
  gatekeeper holds a persistent Ed25519 `did:key` agent identity
  (`state/agent-key.json`) and proves holder binding with a PoP over
  `sha256(tool|account|payloadXDR)`, verified custodian-side against the
  mandate's `agent` field.
- Non-keyring KEK for containers (§6).
- One custodian serving several gatekeepers: cumulative limits must then be
  enforced custodian-side, which the design already assumes.

**Notes for implementers**

- Never log `args` wholesale — `secret`-bearing and `confirm`-style
  parameters are hashed/sanitized in records (spec §12).
- Keep secrets out of `parameter_collection.go` prompts — those strings are
  echoed to the agent.
- Tool annotations are hints for clients, not authorization (spec §7.4) —
  the tier table is authoritative.
- Every new tool gets a tier in §5.2 and `internal/mcp/tiers.go` at
  registration time; unclassified tools fail closed in `applyProfile`.
- HTTP dispatch propagates `r.Context()` through `callTool`, so request
  cancellation reaches handlers — keep handler-side timeouts regardless.
