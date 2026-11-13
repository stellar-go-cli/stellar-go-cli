# Impact assessment (spec §13.2 — required before enabling T3/value-moving profiles)

## Affected parties

- Principal (wallet owner)
- Counterparties receiving misdirected value
- Downstream agents/services
- Operator

## Harms

| Harm | Severity driver | Control |
|---|---|---|
| Irreversible transfer (DLT finality) | value at risk, network | E1 hard caps, E3 bound approval, mandate limits |
| Key compromise cascade | custody model | K1–K3 custody split; seeds never leave custodian |
| Mandate misuse beyond intent | mandate breadth | §6.4 verification, holder PoP, scope/limits/status checks |
| Audit-trail poisoning | — | hash-chained logs; `audit verify` |

## Residual risk acceptance

| Residual | Owner | Review date |
|---|---|---|
| Same-UID token file readable by co-resident processes | | |
| Soroban submission egress outside custodian | | |
