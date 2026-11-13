# Audit review procedure

## 1. Verify record integrity

```bash
stellar-go-cli audit verify        # replays enforcement.log + audit.log chains
```

## 2. Review incident signals

```bash
stellar-go-cli audit events        # §11.4: refusals, verification failures, bursts,
                                   # grant-outside-window, excluded-tool-probing,
                                   # escalation-rate-limited, chain-failure
stellar-go-cli audit events --window 1h --out events.json
```

## 2a. Review agent lifecycle

```bash
stellar-go-cli signer agent list        # enrolled | active | suspended | retired
stellar-go-cli signer agent suspend <did:key>   # incident brake — refuses requests
stellar-go-cli signer agent retire  <did:key>   # terminal + revokes its mandates
```

## 3. Measure revocation latency

```bash
stellar-go-cli audit latency --mandate-ref <vc-id>   # suspends the mandate!
```

## 4. Check policy posture

```bash
stellar-go-cli signer policy status    # unapproved widening?
```

## 5. Run the conformance suite

```bash
go test ./internal/custodian/ -run 'TestC[0-9]' -v
go test ./internal/mcp/ -run 'TestC[0-9]' -v
```

## 6. Update the scope inventory

```bash
stellar-go-cli audit inventory --out inventory.json
```

## 7. Export portable evidence for a transaction

```bash
stellar-go-cli audit evidence --payment payment.json --record <record-hash> --out evidence.xml
```

## Cadence

Per spec §11.3: policy review at each version bump; full procedure at each
release and after any incident.
