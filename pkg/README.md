# stellar-go-cli/pkg — Stellar building blocks for Go services

Public, importable Go packages extracted from the Stellar Go CLI. They depend
only on the [Stellar Go SDK](https://github.com/stellar/go) (plus `json-gold`
for `vc`) — no CLI internals, no filesystem state, no terminal output.

```go
import "github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
```

Pin a release with the library module tag:

```
go get github.com/stellar-go-cli/stellar-go-cli/pkg@pkg/v0.1.0
```

| Package | Purpose |
|---------|---------|
| `soroban` | Pure-Go Soroban RPC client — deploy, invoke, simulate |
| `iso20022` | ISO 20022 pacs.008/002/004/009, pain.001/002, camt.053/054 generation **and parsing** |
| `vc` | DID creation and Verifiable Credential issuance/verification |
| `swap` | Stellar DEX path-payment quotes, transaction building, execution |
| `models` | Shared data models (Account, Payment, Asset, VC, swap types) |
| `crypto` | Key generation, hashing, encoding helpers |

## swap — quotes, unsigned XDR, execution

### Quote-only (no signer, no wallet)

```go
svc := swap.NewService(models.NetworkStellarMainnet)
quote, err := svc.GetQuote(models.SwapRequest{
    SourceAsset: "XLM",
    DestAsset:   "USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", // any CODE:ISSUER
    Amount:      "10",
    SwapType:    models.SwapStrictSend,
})
```

Strict-receive quotes need a source account (for Horizon's `/paths` request) —
pass `SwapRequest.SourceAccount` or attach a signer.

### Non-custodial: build unsigned XDR for a wallet to sign

```go
acct, _ := svc.FetchAccount("GABC...")
txB64, err := svc.BuildSwapTransactionXDR(quote, acct, 1.0 /* slippage % */, "GDEST...")
// return txB64 to the client; the wallet signs and submits
```

`txnbuild.Account` is an interface — pass `txnbuild.SimpleAccount{AccountID, Sequence}`
if you track sequence numbers yourself.

### Custodial: inject your signer

```go
type kmsSigner struct{ keyID string }
func (k kmsSigner) Address() string { ... }
func (k kmsSigner) Sign(tx *txnbuild.Transaction, passphrase string) (*txnbuild.Transaction, error) { ... }

svc := swap.NewService(models.NetworkStellarMainnet,
    swap.WithClient(myHorizonClient),   // self-hosted Horizon
    swap.WithSigner(kmsSigner{keyID: "stellar/treasury"}),
)
payment, err := svc.ExecuteSwap(quote, 1.0, "GDEST...")
```

## iso20022 — disbursement reporting & reconciliation

```go
// pain.001 batch file for a disbursement
xml, err := iso20022.BuildPain001(instrs, &iso20022.Pain001Options{InitiatingParty: org})

// ingest the bank's pain.002 status report back
doc, err := iso20022.ParsePain002(data)
reports := doc.StatusReports()

// match reports to the sent instructions; map ISO codes to your state
// machine (ACSC→success, RJCT→failed, PDNG/ACSP→pending, PART→mixed)
sum := iso20022.ReconcileInstructions(instrs, reports)
for _, r := range sum.Results {
    if r.Matched { /* update payment status */ }
}

// large batches: stream without building the whole document in memory
totals, _ := iso20022.ComputeBatchTotals(instrs)
sw, _ := iso20022.NewPain001StreamWriter(w, opts, totals, instrs[0])
for _, instr := range instrs { sw.WriteInstruction(instr) }
sw.Close()
```

## Fit for SDF services

| Service | What fits | Notes |
|---------|-----------|-------|
| `stellar-disbursement-platform-backend` | `iso20022` — pain.001/pacs.008 export, pain.002/pacs.002 ingest, `ReconcileInstructions`, camt.053/054 | `CreditTransferInstruction` maps directly onto a disbursement payment (`Payment.ID`, `Amount`, `StellarTransactionID`, receiver `Party` incl. phone/email proxy accounts). Reconciliation mirrors their Circle reconciliation job pattern. |
| `wallet-backend`, `freighter-backend-v2` | `swap` (unsigned-XDR path), `models`, `soroban` (reference) | These are indexing services on the official SDK's `rpcclient` — `soroban` is a reference client, not a replacement. `swap` fits a future server-side quote endpoint: `GetQuote` + `FetchAccount` + `BuildSwapTransactionXDR` returns unsigned XDR for the extension to sign. |

## Versioning

`pkg/` is a separate Go module (`github.com/stellar-go-cli/stellar-go-cli/pkg`)
versioned independently of the CLI via `pkg/v*` git tags. Breaking changes in
pre-1.0 releases land in minor bumps and are documented in the changelog.
