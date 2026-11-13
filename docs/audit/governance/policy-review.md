# Policy lifecycle workflow (spec §11.3)

## Mechanics

- Policy: `state/policy.json`; version + hash stamped on every enforcement record.
- Approved snapshot: `state/policy.approved.json`; approval log: `state/policy-approvals.json`.

## Workflow

1. Draft policy change (edit `policy.json`).
2. `stellar-go-cli signer policy status` — shows the widening diff vs the approved snapshot.
3. **Widening** (cap raised/added, network added, deny removed, escalation threshold raised/removed, allow_unprotected/allow_unclassified_ops enabled): record approval — `stellar-go-cli signer policy approve`. The daemon refuses to start on unapproved widening.
4. **Narrowing**: applies immediately; the daemon records an automatic approval on next start.
5. Record the review date + approver in this file's log below.

## Review log

| Date | Version | Approver | Widening? | Notes |
|---|---|---|---|---|
| | | | | |
