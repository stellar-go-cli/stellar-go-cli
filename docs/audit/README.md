# Security Assessment — stellar-go-cli

| | |
|---|---|
| **Engagement** | Security assessment by [Runtime Verification](https://runtimeverification.com) |
| **Assessment date** | 13 October 2026 |
| **Subject** | `stellar-go-cli` — wallets, payments, DEX swaps, Soroban contracts, DIDs/VCs, ISO 20022, and the built-in MCP server for AI-agent integration |
| **Scope** | Key custody, the agent/MCP boundary, delegated authority, and the verification strategy for the target architecture |

This folder contains the security-assessment documentation set. Each
document is self-contained; this page is the map.

---

## Contents

| Document | Role in the assessment |
|---|---|
| [`agent-policy-engine.md`](agent-policy-engine.md) | **Policy & Management Engine (PME) specification** — the security model under assessment. Implementation-agnostic, RFC 2119 language: actors and the social model, delegated authority (mandates as VCs), the decide/enforce split (gatekeeper PDP vs custodian PEP, E1–E5 invariants), key-management requirements K1–K14, risk tiers, human-oversight rules, audit/evidence, conformance classes, and ISO/IEC 42001:2023 + ISO/IEC 27001:2022 control mappings with an EU regulatory annex (PSD2 RTS, proposed PSD3/PSR, eIDAS 2.0, MiCA, GDPR). |
| [`mcp-signer-architecture.md`](mcp-signer-architecture.md) | **Repository architecture + roadmap** — how the PME model maps onto this codebase. Contains the current-state gap analysis (the findings), the target gatekeeper/custodian separation design, the signer socket API, key-management design, and the phased implementation roadmap. |
| [`differentialfuzzing/differential-fuzzing.md`](differentialfuzzing/differential-fuzzing.md) | **Verification strategy — input space (implemented Phases 1–3).** Differential fuzzing between the Rust `stellar` CLI (reference oracle) and this Go CLI (target): shipped `testing.F` targets for ScVal/XDR/strkey/tx-envelope differential, iso20022/x402/MCP/config crash fuzzing, Tier-1 corpus runner, CI + nightly campaigns. |
| [`specula/specula-tla-formal-verification.md`](specula/specula-tla-formal-verification.md) | **Verification strategy — state space.** Adoption analysis for specula-org/Specula (agentic TLA+ spec synthesis + TLC model checking): ranked module candidates with invariants, adoption approaches (interactive/auto/CI/BYOM), draft guidance file, caveats. |
| [`specula/specula-lite-custodian-findings.md`](specula/specula-lite-custodian-findings.md) | **State-space verification — results.** Specula-Lite findings against the custodian lifecycle (the "specula-lite F*” references in code trace to this report). |
| [`conformance.md`](conformance.md) | **Conformance suite (C1–C14).** The checks that demonstrate each shipped phase satisfies the corresponding spec requirements, and how to run them. |
| [`governance/`](governance/) | **AIMS/ISMS governance evidence pack** — scope, policy review, control mappings (ISO/IEC 42001:2023, 27001:2022), impact assessment, and audit procedure for the AI-management-system angle of the assessment. |

## How the pieces fit

```
        ┌─────────────────────────────────────────────┐
        │  WHAT is being secured                       │
        │  agent-policy-engine.md    (the model)       │
        │  mcp-signer-architecture.md (design + gaps)  │
        └──────────────────┬──────────────────────────┘
                           │ verified by
              ┌────────────┴─────────────┐
              ▼                          ▼
  differential-fuzzing.md      specula-tla-formal-verification.md
  input-space correctness      state-space correctness
  (encoding, parsing,          (interleavings, lifecycle,
   boundary divergence)         invariants, authorization)
```

- The **spec** defines the security properties the system must satisfy —
  the checklist an auditor verifies against.
- The **architecture doc** records where the current code deviates from
  that model (the findings) and the phased path to conformance.
- The **two verification docs** define the complementary evidence engines:
  differential fuzzing for input-space correctness, TLA+ model checking
  for state-space and authorization invariants.

## Headline findings (summary — detail in `mcp-signer-architecture.md` §2)

- Single trust domain: the MCP server process could hold wallet state and
  sign — no boundary between the agent-facing surface and key custody.
  *Remediated (Phase 3): the `signer` custodian daemon holds all key
  material behind a Unix socket (`internal/custodian/`); the gatekeeper
  uses `RemoteSigner`/`RemoteWallet`, drops `internal/wallet` entirely,
  and fails closed when the socket is absent. Phase 4 adds
  delegated-authority mandates: holder-bound `MandateCredential` VCs
  (open + closed, single-use on a payload digest), verified
  custodian-side against a gatekeeper `did:key` + PoP, with a local
  status registry, mandate-scoped seed escrow for protected wallets,
  and `sign_credential` so protected-wallet VC issuance stays inside
  the daemon. Residual: peer auth is same-UID —
  different-OS-principal isolation is the deployment step; mandate
  **Phase 5 shipped**: bitstring status-list credentials (§11.2), ISO 20022
  `AgentEvidence` mapping (§12.3), enforced policy widening gate (§11.3),
  `audit` command group (verify/events/latency/inventory/evidence),
  C1–C14 conformance suite, and the `governance/` evidence pack. The signsock
  client split removes all wallet code from the gatekeeper binary (C1).*
- Passkey unlock grants stored a *plaintext seed* in the OS keyring,
  readable by any same-UID process during the TTL.
  *Remediated (Phase 3): grants live in custodian memory only; the
  keyring `unlock-*` items are removed.*
- The HTTP transport previously bound all interfaces with no
  authentication and wildcard CORS — anyone reaching the port could
  invoke any tool.
  *Remediated: loopback bind by default, non-loopback refused without a
  bearer token, auth on `/sse`+`/message` (v0.7.1,
  GHSA-x2fr-2mv2-8fc8); `Origin`/`Host` validation and restricted CORS
  added post-v0.7.1.*
- Agent-facing tools previously accepted and returned secrets
  (`wallet_import`/`export`), steered endpoints (`rpc_url`), and returned
  unlabelled third-party content. *Remediated post-v0.7.1: the
  secret-bearing tools are removed from the agent registry (operator CLI
  only), `rpc_url`/`allow_private_resource` are operator flags,
  `response_body` is labelled untrusted, and a `--profile` flag
  (`readonly`/`transact`/`admin`) provides least-privilege tool
  exposure.*
- `pay_x402` dry-run previously produced a signed payload — a bearer
  instrument misclassified as read-only. *Remediated: unsigned previews
  (v0.7.1, GHSA-x2fr-2mv2-8fc8), now fully key-free — dry-runs resolve
  only the payer's public address via `NewStellarPreviewClient` and the
  signing-refusing `signer.AddressSigner`; `execute=true` requires a cap
  clamped by the operator's `--x402-max-amount`.*

## Status conventions

Sections in the architecture doc marked *current* describe shipped code;
*target* describes unimplemented design. The spec's SHALL/SHOULD statements
are conformance requirements for the target architecture — they describe
what the system **must become**, not what it is today. Neither document
claims certification under any standard.

## Related

- [`../passkeys.md`](../passkeys.md) — current passkey/grant mechanics
  referenced by the findings.
- [`../../SECURITY.md`](../../SECURITY.md) — project security policy.
