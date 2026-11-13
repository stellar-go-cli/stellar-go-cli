# Custodian formal-verification findings (specula-lite run)

TLA+ model-checking of `internal/custodian/` via the specula-lite workflow
(`.specula-lite/` — gitignored; full traces, configs and methodology live in
`.specula-lite/{spec,output,report.md}`). Seven hunt invariants produced
counterexamples; each was mapped to code and reproduced by the executable
evidence suite in `internal/custodian/specula_lite_repro_test.go` (run
`go test ./internal/custodian/ -run TestRepro_ -v`).

**Status: remediated.** The fixes below are implemented and covered by
regression tests (the `TestRepro_*` suite now asserts the *fixed* behavior
and goes red if a defect returns). The TLA+ model was updated to match and
the hunts re-run — see `.specula-lite/report.md` for the verification table.

## Defects and fixes

| ID | Severity | Defect | Fix | Regression test |
|----|----------|--------|-----|-----------------|
| F1 | low | `issued` marked **before** `decisions.claim` — refused sign left a submittable blob | Commit boundary reordered: sign is pure output, `issued` is marked only inside the all-or-nothing commit (`enforce.go:277-298`) | `TestRepro_RefusedClaimLeavesNoResidue` |
| F2 | med | charge-step refusal burned the single-use decision ref | `decisions.release(decisionRef)` on charge failure (`enforce.go:283`, `grants.go` `seenSet.release`) | `TestRepro_RefSurvivesChargeRefusal` |
| F3 | med | multi-op partial charge committed earlier ops on refusal | `chargeSpend` → `chargeAll` validates+commits all ops under one lock on both counters; global failure refunds the mandate charge (`enforce.go`, `policy.go`, `mandate.go`) | `TestRepro_ChargeIsAtomic`, `TestRepro_ChargeAllAtomicUnit`, `TestRepro_MandateChargeAllAndRefund` |
| F4 | med | configured `policy.Escalation.Rate` limiter never engaged — fresh limiter per call | `configuredLimiter` built once under `limiterMu` and reused (`pending.go:67-90`) | `TestRepro_ConfiguredLimiterEngages` |
| F5 | **high** | `agent_set_status` reactivated `suspended→active` with only the socket token — no ceremony | Every transition to `active` routes through the bound `agent_activate` ceremony (`daemon.go:545-559`, `ceremony.go:186-200`); `AgentSetStatus`/`signer agent reactivate` gained `--account` + ceremony relay | `TestRepro_AgentActivationRequiresCeremony` |
| F6 | low-med | `rebuildIssued` resurrected `issued` for already-submitted payloads | `submit_tx` records carry `payload_hash`; rebuild replays the log sequentially — `signed:` adds, `submitted:` tombstones (`submit.go:78,94`, `daemon.go:196-216`) | `TestRepro_SubmittedTombstonedOnRestart` |
| F7 | med | `submit_tx` check-then-delete on `issued` was non-atomic → double submit | `issuedTake` — single critical section check+delete under `issuedMu` (`daemon.go:64-72`) | `TestRepro_IssuedTakeIsAtomic` |
| F8 | **high** | `d.issued`/`d.pendingReq` were unsynchronized maps hit by per-conn goroutines → runtime-fatal `concurrent map writes` | `issuedMu` + `pendingMu` guard all accesses (`daemon.go`, `pending.go`) | `TestRepro_RaceHammer` (permanent `-race` target) |
| F9 | low | authority TOCTOU — E2 sampled grant/escrow once; revocation mid-flight didn't stop the signature | `authorityRevoked` re-validates grant/escrow/mandate/agent state immediately before `d.sign` (`enforce.go:266-274`, `:355-380`) | `TestRepro_AuthorityRecheck` |
| F10 | low | `confirm` minted approvals for any requestHash — escalation funnel was advisory | `MethodConfirm` refuses hashes absent from `pendingReq` (`daemon.go:409-415`) | `TestRepro_ConfirmRequiresPendingEscalation` |
| F11 | low | seen-set aging (>10k, 24h) evicts decision refs → re-claim can re-authorize a decision | **Accepted bound** — documented; mitigated in practice because refused requests no longer burn refs (F2). Revisit if the log should persist first-seen timestamps | `TestRepro_SeenSetAgingBound` (documents the bound) |

## Hygiene fix

`dropWalletGrants` sweeps `mandate:<id>` escrows on `passkey_disable` and
`remove_wallet` — disabled/removed wallets no longer leave usable seed
material escrowed in memory (`daemon.go:74-84`). Covered by
`TestRepro_DropWalletGrantsSweepsEscrows`.

## Cleared (model-verified, unchanged)

Restart relock (memory-only grants rebuilt correctly), approval/ceremony
single-use take-before-check, retire→mandate cascade, `validAgentTransition`
terminality of `retired`.

## Residual bounds (documented, not bugs)

- **F11** seen-set aging (above).
- **F9 commit-tail window**: the `authorityRevoked` re-check shrank the
  revocation window from the whole E2→deliver pipeline (incl. escalation
  waits) to the sign+commit tail (~µs). A revocation landing between the
  re-check and response delivery can still produce a post-revocation
  signature — eliminating it requires a daemon-level lock spanning
  validation→delivery (deferred RWMutex design). The re-run TLC hunts
  (s5_agents, s6_toctou) confirm violations now exist *only* in that tail.
- `pendingReq` expiry is lazy (checked on read); `Restart` clears pending
  escalations, so a `confirm` ceremony must run before restart.

## Verification commands

```bash
go test ./internal/custodian/              # full package suite
go test -race ./internal/custodian/        # race detector (F7/F8/F9 class)
go test ./internal/custodian/ -run TestRepro_ -v   # regression tests
# TLC hunts: java -cp tla2tools.jar tlc2.TLC -deadlock -workers N MC
#   with each .specula-lite/spec/MC_hunt_*.cfg swapped in as MC.cfg
```
