# Specula Adoption Analysis — TLA+ Formal Verification for stellar-go-cli

| | |
|---|---|
| **Status** | Analysis + adoption proposal. Specula is an external tool; nothing here is implemented. |
| **Version** | 0.1 — 2026-11-09 |
| **Upstream** | [`specula-org/Specula`](https://github.com/specula-org/Specula) (Apache-2.0) |

---

## 1. What Specula is

Specula is an **agentic** formal-verification tool: it drives coding agents
to synthesize TLA+ specifications *from source code* — both the invariants
the code should satisfy and a formal model of how it actually behaves —
then **model-checks the spec with TLC** and uses a trace-debugger tool to
**reproduce violations back at code level**.

```
 source code ──(agent: code-analysis)──▶ modeling brief
           ──(agent: spec-generation)──▶ TLA+ invariants + model
           ──(harness-generation)──────▶ executable harness
           ──(validation-workflow)────▶ TLC model checking
           ──(bug-confirmation)───────▶ code-level reproduction of violations
```

Key facts (verified from the repo):

- **Install**: `uv tool install -e .` then `specula setup` — installs agent
  skills + MCP tools (`trace_debugger`, `spec_analyzer` [a control-flow
  analysis tool built with Maven], `inv_checking_tool`).
- **Prerequisites**: Python 3.10+ with pip, `uv`, Java 21+ with Maven,
  GitHub CLI `gh`, and a supported coding agent (Claude Code, Codex,
  Copilot CLI, OpenCode, Pi). Recommended machine: ~32 GB RAM, ~100 GB
  disk. Windows requires WSL2.
- **Modes**:
  - **Auto** — `specula run <name> --artifact=<repo>
    --guidance=<md>`; outputs land in `runs/<run-id>/` with a
    `pipeline-summary.md`.
  - **Interactive** — invoke the skills in sequence inside a coding agent:
    `code-analysis → spec-generation → harness-generation →
    validation-workflow → bug-confirmation`.
  - **Continuous** — `specula run --ci-init --ci-dir=<dir>` then
    `--incremental` on later changes; a GitHub Actions workflow is
    provided. Keep `ci-dir` outside the source checkout.
  - **BYOM** — import an existing hand-written TLA+ model instead of
    synthesizing one.
- A `--guidance` markdown file scopes runs to target modules/scenarios so
  modeling stays consistent across runs (template + example ship in
  `docs/` upstream).

---

## 2. Where it fits in this repo

TLA+ shines on **state machines and concurrency/interleavings** — the bug
class input-space fuzzing misses. Ranked candidates:

| # | Module | State machine | Candidate invariants |
|---|---|---|---|
| 1 | Passkey grant lifecycle — `internal/wallet/passkey.go` | locked → ceremony → grant(TTL) → resolve → lock/expire → disable | No seed resolution while locked; no signing after grant expiry; `disable` restores plaintext only after a valid assertion; `ResolveSeed` never falls back silently |
| 2 | **PME decision pipeline** — `docs/agent-policy-engine.md` | agent/mandate/grant/escalation states; E1–E5 enforcement | *No signature without authority* under all interleavings; revocation wins races with in-flight requests; mandates can't be self-issued. **Model-first**: verify the design before the Phase-3 signer exists — cheapest place to catch a design flaw |
| 3 | x402 payment flow — `pkg/x402` | fetch → 402 → build/sign auth → paid retry → settle | Spend caps enforced every path; single-payment (no double-pay on retry); no replay of a settled payload |
| 4 | MCP elicitation/dispatch — `internal/mcp`, `parameter_collection.go` | request → collect params → validate → dispatch | Handlers never run on unvalidated args (the fail-open path in `mcp-signer-architecture.md` §2.2); session isolation |
| 5 | Wallet registry — `internal/wallet/service.go` | connect/switch/remove with `wallets.json` persistence | Active wallet always resolves or is absent; remove purges file + keyring + grant + passkey records |
| 6 | Swap execution — `pkg/swap` | quote → build → sign → submit path-payment | Never execute outside quoted bounds; slippage caps hold |

**Best pilot**: #1 (passkey lifecycle) — small, stateful, security-critical,
self-contained. **Highest long-term value**: #2 — the PME spec we already
wrote defines E1–E5 and the state machines in prose; Specula turns that
prose into a checked model *before* implementation locks the design in.

---

## 3. Adoption approaches

| Approach | How | When |
|---|---|---|
| **Interactive skill chain** | Open a supported agent in a Specula checkout; run `code-analysis` → … → `bug-confirmation` on `internal/wallet/passkey.go` | Pilot — learning the tool, small scope |
| **Auto mode + guidance file** | Commit a `docs/specula-guidance.md` (draft below); run `specula run stellar-go-cli --artifact=. --guidance=docs/specula-guidance.md` | Once targets are chosen — repeatable runs |
| **CI incremental** | `specula run --ci-init --ci-dir=/work/specula-ci` + `--incremental` via GitHub Actions | After a stable model exists; nightly, not per-PR |
| **BYOM** | Hand-maintain a TLA+ model of the PME state machine (from the spec doc) and import it | For the PME — the spec already exists in prose, a hand model is tractable and reviewable |

---

## 4. Draft guidance file (what `docs/specula-guidance.md` would contain)

```markdown
# Specula modeling guidance — stellar-go-cli

## Target modules (priority order)
1. internal/wallet/passkey.go — unlock grant lifecycle (register, unlock,
   resolve, lock, expire, disable). Focus: concurrent unlock/lock/resolve.
2. pkg/x402 — fetch → 402 → sign → retry → settle. Focus: caps, single-pay.
3. internal/mcp — tools/call dispatch incl. parameter collection.
   Focus: no dispatch on unvalidated args.

## Out of scope
- Network I/O, RPC clients (model as environment actions).
- CLI flag parsing, UI output.
- pkg/iso20022, pkg/vc (parsers — better served by fuzzing).

## Invariants to check
- ResolveSeed returns no material while locked.
- No signing after grant expiry.
- x402 never exceeds MaxAtomicAmount; never double-pays.
- MCP handlers never run before schema validation succeeds.

## Environment assumptions
- Single OS user; keyring ops succeed/fail nondeterministically.
- Clock advances arbitrarily within TTL bounds.
```

---

## 5. Runbook (once prerequisites are met)

```bash
git clone https://github.com/specula-org/Specula.git && cd Specula
uv tool install -e .
specula setup            # installs agent skills + MCP tools (needs Java 21+/Maven)

# auto mode against this repo:
specula run stellar-go-cli \
  --artifact=/path/to/stellar-go-cli \
  --guidance=/path/to/stellar-go-cli/docs/specula-guidance.md

# read results:
cat runs/latest/pipeline-summary.md
```

Workflow per finding: TLC counterexample trace → `bug-confirmation` /
`trace_debugger` reproduces at code level → write a regression test → fix →
re-run. Runs are expensive (minutes–hours + a large machine); keep them
out of the per-commit path.

---

## 6. Honest caveats

- **Model quality is agent-dependent.** The synthesized spec can be wrong
  in both directions — false violations (model doesn't match code) and
  false confidence (model misses real behavior). Human review of the
  generated TLA+ is mandatory, not optional.
- **It models the design, not the binary.** TLA+ checks an abstraction;
  `trace_debugger` helps bridge back to code, but this is a complement to
  tests and fuzzing — not a replacement. Fuzzing finds input-space bugs;
  model checking finds interleaving/state-space bugs. Different bug
  classes, both needed.
- **Heavyweight.** Java 21+/Maven, Python/uv, a supported coding agent
  (which may carry its own licensing/access requirements), and a ~32 GB
  machine. Not a casual CI step.
- **Cost/latency.** Start nightly or on-demand; per-PR incremental runs
  only after a model proves stable and cheap enough.
- **The repo has almost no concurrency today.** The strongest candidates
  are state machines (grant lifecycle, x402 flow), not lock-free
  concurrency — expect modest but real yields, and the biggest payoff on
  the *future* PME where the state space is being designed now.

---

## 7. Phased plan

| Phase | Scope | Acceptance |
|---|---|---|
| **1 — Pilot (interactive)** | Specula skill chain on `internal/wallet/passkey.go` grant lifecycle | A checked TLA+ model; zero or triaged violations; documented review of the model |
| **2 — Auto mode** | Guidance file committed; runs on x402 + MCP dispatch | `pipeline-summary.md` reviewed; findings → regression tests |
| **3 — Model-first PME** | TLA+ model of `agent-policy-engine.md` state machines + E1–E5 before the Phase-3 signer daemon is built | "No signature without authority" verified; design issues fed back into the spec |
| **4 — CI incremental** | `--ci-init`/`--incremental` in Actions, nightly | Stable model, bounded runtime, findings gate merges |

## See also

- [`agent-policy-engine.md`](agent-policy-engine.md) — the state machines
  and invariants Specula would check (Phase 3).
- [`differential-fuzzing.md`](differential-fuzzing.md) — the complementary
  input-space technique.
