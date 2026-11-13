# Conformance results — Class T (spec §15)

Each vector maps to an automated test. Run:

```bash
go test ./internal/custodian/ -run 'TestC[0-9]' -v   # C3–C6, C9–C12, C14
go test ./internal/mcp/ -run 'TestC[0-9]' -v         # C1, C2, C7, C8, C13
```

| Vector | Requirement | Test | Status |
|---|---|---|---|
| C1 | Gatekeeper has no read path to key material | `TestC1_NoWalletDependency` — `go list -deps` proves internal/mcp's import graph contains no `internal/wallet` (post-split: signsock client package carries no wallet code) | ✅ |
| C2 | No agent-reachable path returns key material | `TestC2C13_NoSecretToolsOrParams` — registry contains no import/export tools | ✅ |
| C3 | Every request → decision + enforcement record; chain verifies | `TestC3_RecordsAndChainVerify` — sign + refusal both record; `chainlog.VerifyFile` replays the chain | ✅ |
| C4 | T3 without authority is never ALLOW | `TestC4_T3WithoutAuthorityNeverSigns` — above-threshold with grant → `escalation_required`, never a signature | ✅ |
| C5 | Within-mandate executes; outside denied | `TestC5_MandateBounds` — scope-covered sign succeeds; uncovered tool refused | ✅ |
| C6 | Revocation within declared latency | `TestC6_RevocationLatency` — revoke→refusal measured < 2 s (synchronous local registry; `audit latency` measures live) | ✅ |
| C7 | ESCALATE unanswerable on agent channel | `TestC7_NoEscalationAnswerOnAgentChannel` — no tool schema carries a ceremony-answer parameter | ✅ |
| C8 | HTTP: loopback, auth, Origin validation | `TestC8_TransportDefaults` + `TestGuardTransportRequest` — foreign Host/Origin → 403 | ✅ |
| C9 | Expired/revoked/suspended authority can't sign | `TestC9_NoSignatureWithoutLiveAuthority` — expired grant, suspended + expired mandate all refused | ✅ |
| C10 | Custodian refuses mismatched/undecodable payloads | `TestC10_PayloadTruth` — wrong request hash + garbage XDR refused (E4) | ✅ |
| C11 | Records alone attribute agent + mandate + approver | `TestC11_Attribution` — record carries decision_ref, key_id, mandate_ref, result | ✅ |
| C12 | Lock/revoke/suspend usable while ops denied | `TestC12_PrivilegeReductionAlwaysWorks` — locked protected wallet: sign refused, suspend/revoke succeed | ✅ |
| C13 | No key-import path via agent params | `TestC2C13_NoSecretToolsOrParams` — no tool schema param can carry seed/key material | ✅ |
| C14 | Custodian artefact integrity-verified | `TestC14_SelfIntegrityRecord` — daemon anchors `self:<sha256>` of its binary at chain head on every start | ✅ |

**Class M** is a management-system property — the evidence pack under
`governance/` provides the artefacts M1–M5 consume; it is not a test
property of this repo.
