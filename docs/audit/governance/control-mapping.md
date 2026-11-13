# Control mapping — PME element → implementation → evidence (SoA-style)

| PME element | Implementation | Evidence pointer |
|---|---|---|
| Custody split (K1–K3) | `internal/custodian` daemon + `internal/signsock` client; gatekeeper links no wallet code | `TestC1_NoWalletDependency`; `go list -deps ./internal/mcp` |
| E1–E5 enforcement | `internal/custodian/enforce.go` | `conformance_test.go` C4/C9/C10; enforcement.log |
| Bound ceremonies (E3) | `internal/webauthn` relay + `ceremony.go` | enforcement records `approval:` field |
| Mandates (§6) | `mandate.go` + `delegation.go` + `pkg/vc` | `mandate_test.go`; status-list VCs |
| Holder binding | `internal/mcp/agent_identity.go` PoP | `TestAgentPOP*` |
| Status mechanism (§11.2) | `statuslist.go` BitstringStatusListCredentials | `wallet mandate statuslist` |
| Policy gate (§11.3) | `policy.go` widening diff + approvals | `signer policy status`; `policy_gate_test.go` |
| Records (§12.1) | `internal/chainlog` — enforcement.log + audit.log | `audit verify`; `TestC3` |
| Evidence portability (§12.3) | `pkg/iso20022` `AgentEvidence` SplmtryData | `audit evidence` output |
| Incident signals (§11.4) | `audit events` scan (both logs: refusals, E2/E4 failures, denial bursts, grant-outside-window, excluded-tool-probing, escalation-rate-limited, chain-failure) | `audit events` report |
| Agent lifecycle (§11.1) | `internal/custodian/agents.go` — `state/agents.json`, FSM enrolled→active→suspended→retired; retire cascades mandate revocation | `agents_test.go`; `signer agent` CLI |
| Escalation fatigue (§10.3) | `pending.go` escalationLimiter + `request_info` pending map; `untrusted` flag from gatekeeper | `TestEscalationRateLimited`, `TestPendingRequestTracksEscalation`; `wallet passkey approve` display |
| Artefact integrity (K14) | `recordSelfIntegrity` chain anchor | `TestC14`; `self:` record at chain head |
