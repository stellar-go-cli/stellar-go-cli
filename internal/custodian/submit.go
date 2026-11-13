// submit.go — submit_tx: network submission inside the custodian boundary.
// The daemon only submits payloads it signed itself (E5: the signed-XDR
// digest must match a recorded `signed:` decision) — the caller never gains
// a raw egress path to Horizon.
package custodian

import (
	"fmt"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

// submitTx submits a signed envelope the daemon produced. Authority: the
// payload hash must exist in d.issued (populated at sign time and rebuilt
// from the enforcement log) — a caller can't submit anything else through
// the daemon's egress.
func (d *Daemon) submitTx(p *signsock.SubmitTxParams) (map[string]interface{}, *signsock.RPCError) {
	fail := func(check, msg string) (map[string]interface{}, *signsock.RPCError) {
		d.record(enforcementRecord{
			Timestamp: nowRFC3339(), Tool: signsock.MethodSubmitTx, KeyID: p.Account,
			Checks: map[string]string{"E5": "fail: " + msg},
			Result: "refused:" + msg,
			Policy: d.policyStamp(),
		})
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: msg}
	}

	// Decode the SIGNED envelope — submission must match something we signed.
	gtx, err := txnbuild.TransactionFromXDR(p.SignedXDR)
	if err != nil {
		return fail("E4", "undecodable signed envelope: "+err.Error())
	}
	tx, ok := gtx.Transaction()
	if !ok {
		return fail("E4", "fee-bump envelopes not supported")
	}
	if len(tx.Signatures()) == 0 {
		return fail("E4", "unsigned transaction cannot be submitted")
	}
	source := tx.SourceAccount().AccountID
	if p.Account != "" && !strings.EqualFold(p.Account, source) {
		return fail("E4", "declared account does not match envelope source")
	}
	acc, err := d.svc.GetWalletByAddress(source)
	if err != nil {
		return fail("E2", "unknown wallet "+source)
	}

	// E5 — single-use: only payloads this daemon signed may be submitted,
	// and each signed blob submits once. Check+delete is one atomic step —
	// concurrent submit_tx calls cannot both pass (specula-lite F7).
	hash := payloadHash(p.SignedXDR)
	if !d.issuedTake(hash) {
		return fail("E5", "payload was not signed by this custodian or already submitted")
	}

	var client *horizonclient.Client
	var passphrase string
	switch acc.Network {
	case "stellar-mainnet", "public":
		client = horizonclient.DefaultPublicNetClient
		passphrase = network.PublicNetworkPassphrase
	default:
		client = horizonclient.DefaultTestNetClient
		passphrase = network.TestNetworkPassphrase
	}
	resp, err := client.SubmitTransactionXDR(p.SignedXDR)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			if rc, rcErr := herr.ResultCodes(); rcErr == nil && rc != nil {
				msg := fmt.Sprintf("tx failed — code: %s, ops: %v", rc.TransactionCode, rc.OperationCodes)
				d.record(enforcementRecord{
					Timestamp: nowRFC3339(), Tool: signsock.MethodSubmitTx, KeyID: source,
					PayloadHash: hash,
					Checks:      map[string]string{"E5": "pass"}, Result: "refused:" + msg, Policy: d.policyStamp(),
				})
				return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: msg}
			}
			return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "horizon error: " + herr.Problem.Title}
		}
		return nil, &signsock.RPCError{Code: signsock.ErrRefused, Message: "submit to Horizon: " + err.Error()}
	}

	txHash, err := tx.HashHex(passphrase)
	if err != nil {
		txHash = resp.Hash
	}
	d.record(enforcementRecord{
		Timestamp: nowRFC3339(), Tool: signsock.MethodSubmitTx, KeyID: source,
		PayloadHash: hash,
		Checks:      map[string]string{"E5": "pass"},
		Result:      "submitted:" + txHash,
		Policy:      d.policyStamp(),
	})
	return map[string]interface{}{
		"hash":    txHash,
		"ledger":  resp.Ledger,
		"success": resp.Successful,
	}, nil
}
