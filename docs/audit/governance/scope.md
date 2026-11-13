# Scope declaration template (spec M1 / ISO 42001 §4.3)

> Fill in and review on the §11.3 cadence. `stellar-go-cli audit inventory`
> generates most of the machine-readable half.

## Agents

| Agent | Identity (did:key) | Profile | Mandates held |
|---|---|---|---|
| | | | |

## Tools exposed

Profile: `readonly | transact | admin` — tool set per `internal/mcp/tiers.go`.

## Accounts / networks

| Wallet | Address | Protected | Networks allowed |
|---|---|---|---|
| | | | |

## Out of scope

- Transaction submission through Soroban RPC (caller-side — see architecture doc §12).
- Ledger-level controls (signer rotation, thresholds) — not implemented.
