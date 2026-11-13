// x402_server is a manual-test helper: a live x402 v2 resource server +
// facilitator for exercising `stellar-go-cli pay x402` end to end on testnet.
//
// Setup (all on live testnet):
//   - issuer/merchant keypairs created and funded via Friendbot
//   - a payer account (provided via --payer-seed, or a fresh one is generated)
//   - X402TEST asset trustlines for payer + merchant, 1000 units issued to payer
//   - the SEP-41 SAC for the asset deployed via pkg/soroban
//
// Serve phase mirrors the e2e facilitator: the first request gets a v2
// PAYMENT-REQUIRED challenge; the signed retry is settled on-chain by
// rebuilding the payer's envelope with the facilitator as fee source.
//
// Run:
//
//	go run ./scripts/e2e_helpers/x402_server --port 8091
//	# import the printed payer seed with `stellar-go-cli wallet import`, then
//	stellar-go-cli pay x402 --resource http://127.0.0.1:8091/premium --network stellar-testnet --payer <addr>
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/x402"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	rpcclient "github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	assetCode    = "X402TEST"
	assetAmount  = "1000"    // issued to payer
	payAtomic    = "1000000" // 0.1 tokens at 7 decimals
	pollInterval = 3 * time.Second
)

func fatal(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func friendbot(ctx context.Context, address string) {
	url := "https://friendbot.stellar.org/?addr=" + address
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			fatal("friendbot request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	fatal("friendbot funding failed for %s", address)
}

func submitClassic(ctx context.Context, horizon *horizonclient.Client, passphrase string, kp *keypair.Full, ops ...txnbuild.Operation) {
	account, err := horizon.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		fatal("load account %s: %v", kp.Address(), err)
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &account,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           ops,
	})
	if err != nil {
		fatal("build transaction: %v", err)
	}
	tx, err = tx.Sign(passphrase, kp)
	if err != nil {
		fatal("sign transaction: %v", err)
	}
	if _, err := horizon.SubmitTransaction(tx); err != nil {
		fatal("submit transaction: %v", err)
	}
}

type facilitator struct {
	rpcURL      string
	passphrase  string
	networkName string
	kp          *keypair.Full
	contractID  string
	payTo       string
	payerAddr   string
	resourceURL string
}

func (f *facilitator) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(x402.HeaderPaymentSignature) == "" {
		required := x402.PaymentRequired{
			X402Version: x402.Version2,
			Resource:    &x402.ResourceInfo{URL: f.resourceURL + r.URL.Path},
			Accepts: []x402.PaymentRequirements{
				{
					Scheme:            "exact",
					Network:           f.networkName,
					Asset:             f.contractID,
					Amount:            payAtomic,
					PayTo:             f.payTo,
					MaxTimeoutSeconds: 300,
					Extra:             map[string]interface{}{"areFeesSponsored": true},
				},
			},
		}
		data, err := json.Marshal(required)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set(x402.HeaderPaymentRequired, base64.StdEncoding.EncodeToString(data))
		w.WriteHeader(http.StatusPaymentRequired)
		return
	}

	payload, err := x402.DecodePaymentSignatureHeader(r.Header.Get(x402.HeaderPaymentSignature))
	if err != nil {
		f.reject(w, "invalid_payload", err.Error())
		return
	}
	txB64, _ := payload.Payload["transaction"].(string)
	if txB64 == "" {
		f.reject(w, "invalid_payload", "payload.transaction is missing")
		return
	}
	txHash, err := f.settle(r.Context(), txB64)
	if err != nil {
		f.reject(w, "settlement_failed", err.Error())
		return
	}
	settlement := x402.SettlementResponse{
		Success:     true,
		Payer:       f.payerAddr,
		Transaction: txHash,
		Network:     f.networkName,
		Amount:      payAtomic,
	}
	data, _ := json.Marshal(settlement)
	w.Header().Set(x402.HeaderPaymentResponse, base64.StdEncoding.EncodeToString(data))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"data":"premium content delivered"}`))
}

func (f *facilitator) reject(w http.ResponseWriter, reason, message string) {
	settlement := x402.SettlementResponse{
		Success:      false,
		ErrorReason:  reason,
		ErrorMessage: message,
		Network:      f.networkName,
	}
	data, _ := json.Marshal(settlement)
	w.Header().Set(x402.HeaderPaymentResponse, base64.StdEncoding.EncodeToString(data))
	w.WriteHeader(http.StatusPaymentRequired)
}

// settle rebuilds the payer-signed invoke op into a facilitator-sourced
// envelope (facilitator pays the fee), signs, submits, and polls.
func (f *facilitator) settle(ctx context.Context, txB64 string) (string, error) {
	var envelope xdr.TransactionEnvelope
	if err := xdr.SafeUnmarshalBase64(txB64, &envelope); err != nil {
		return "", fmt.Errorf("decode transaction envelope: %w", err)
	}
	if envelope.Type != xdr.EnvelopeTypeEnvelopeTypeTx || envelope.V1 == nil {
		return "", fmt.Errorf("expected a v1 transaction envelope, got %s", envelope.Type)
	}
	if len(envelope.V1.Tx.Operations) != 1 {
		return "", fmt.Errorf("expected exactly one operation, got %d", len(envelope.V1.Tx.Operations))
	}
	invokeOp, ok := envelope.V1.Tx.Operations[0].Body.GetInvokeHostFunctionOp()
	if !ok {
		return "", fmt.Errorf("expected an InvokeHostFunction operation, got %s", envelope.V1.Tx.Operations[0].Body.Type)
	}

	op := &txnbuild.InvokeHostFunction{
		HostFunction: invokeOp.HostFunction,
		Auth:         invokeOp.Auth,
		Ext:          envelope.V1.Tx.Ext,
	}
	baseFee := int64(envelope.V1.Tx.Fee)
	if envelope.V1.Tx.Ext.SorobanData != nil {
		baseFee -= int64(envelope.V1.Tx.Ext.SorobanData.ResourceFee)
	}
	if baseFee <= 0 {
		return "", fmt.Errorf("invalid base fee derived from payer transaction")
	}

	rpcClient := rpcclient.NewClient(f.rpcURL, http.DefaultClient)
	defer func() { _ = rpcClient.Close() }()

	account, err := rpcClient.LoadAccount(ctx, f.kp.Address())
	if err != nil {
		return "", fmt.Errorf("load facilitator account: %w", err)
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        account,
		IncrementSequenceNum: true,
		BaseFee:              baseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return "", fmt.Errorf("rebuild transaction: %w", err)
	}
	tx, err = tx.Sign(f.passphrase, f.kp)
	if err != nil {
		return "", fmt.Errorf("sign settlement transaction: %w", err)
	}
	signedB64, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("serialize settlement transaction: %w", err)
	}
	send, err := rpcClient.SendTransaction(ctx, rpc.SendTransactionRequest{Transaction: signedB64})
	if err != nil {
		return "", fmt.Errorf("submit settlement transaction: %w", err)
	}
	if send.Status == "ERROR" {
		return "", fmt.Errorf("settlement rejected: %s", send.ErrorResultXDR)
	}

	for {
		get, err := rpcClient.GetTransaction(ctx, rpc.GetTransactionRequest{Hash: send.Hash})
		if err != nil {
			return send.Hash, fmt.Errorf("poll settlement transaction: %w", err)
		}
		switch get.Status {
		case "SUCCESS":
			return send.Hash, nil
		case "FAILED":
			return send.Hash, fmt.Errorf("settlement failed on-chain: %s", get.ResultXDR)
		}
		select {
		case <-ctx.Done():
			return send.Hash, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func main() {
	port := flag.Int("port", 8091, "HTTP port")
	payerSeed := flag.String("payer-seed", "", "Payer secret seed (S...). If empty, a fresh payer is generated and printed for `wallet import`.")
	flag.Parse()
	ctx := context.Background()
	passphrase := network.TestNetworkPassphrase
	rpcURL := soroban.TestnetRPCURL
	horizon := horizonclient.DefaultTestNetClient

	issuer, err := keypair.Random()
	if err != nil {
		fatal("issuer keypair: %v", err)
	}
	merchant, err := keypair.Random()
	if err != nil {
		fatal("merchant keypair: %v", err)
	}
	var payer *keypair.Full
	if *payerSeed != "" {
		payer, err = keypair.ParseFull(strings.TrimSpace(*payerSeed))
		if err != nil {
			fatal("parse --payer-seed: %v", err)
		}
	} else {
		payer, err = keypair.Random()
		if err != nil {
			fatal("payer keypair: %v", err)
		}
	}
	log.Printf("issuer:    %s", issuer.Address())
	log.Printf("merchant:  %s", merchant.Address())
	log.Printf("payer:     %s", payer.Address())
	if *payerSeed == "" {
		log.Printf("payer seed: %s", payer.Seed())
	}

	log.Printf("funding accounts via Friendbot...")
	for _, kp := range []*keypair.Full{issuer, merchant, payer} {
		friendbot(ctx, kp.Address())
	}

	asset := txnbuild.CreditAsset{Code: assetCode, Issuer: issuer.Address()}
	submitClassic(ctx, horizon, passphrase, payer, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(ctx, horizon, passphrase, merchant, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(ctx, horizon, passphrase, issuer, &txnbuild.Payment{
		Destination: payer.Address(),
		Amount:      assetAmount,
		Asset:       asset,
	})
	log.Printf("trustlines + issuance done (%s %s -> payer)", assetAmount, assetCode)

	sc := soroban.NewClient(rpcURL, passphrase)
	defer func() { _ = sc.Close() }()
	contractID, deployTx, err := sc.CreateContractForAsset(ctx, signer.KeypairSigner{KP: issuer}, asset)
	if err != nil {
		fatal("deploy SAC: %v", err)
	}
	log.Printf("SAC contract: %s (deploy tx %s)", contractID, deployTx)

	host := fmt.Sprintf("http://127.0.0.1:%d", *port)
	f := &facilitator{
		rpcURL:      rpcURL,
		passphrase:  passphrase,
		networkName: x402.NetworkStellarTestnet,
		kp:          issuer,
		contractID:  contractID,
		payTo:       merchant.Address(),
		payerAddr:   payer.Address(),
		resourceURL: host,
	}

	summary := map[string]string{
		"resource":       host + "/premium",
		"network":        "stellar-testnet",
		"payer":          payer.Address(),
		"merchant":       merchant.Address(),
		"issuer":         issuer.Address(),
		"contract":       contractID,
		"pay_atomic":     payAtomic,
		"pay_amount_str": "0.1",
	}
	out, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Printf("\n=== x402 helper ready ===\n%s\n\n", out)
	fmt.Printf("Import payer:  stellar-go-cli wallet import --private-key-file <seedfile> --network stellar-testnet --no-active\n")
	fmt.Printf("Then:          stellar-go-cli pay x402 --resource %s/premium --network stellar-testnet --payer %s\n\n", host, payer.Address())

	if err := http.ListenAndServe(host[7:], http.HandlerFunc(f.serve)); err != nil {
		fatal("http server: %v", err)
	}
}
