//go:build e2e

// End-to-end test for the x402 Stellar exact scheme against Stellar testnet.
//
// The test uses real infrastructure throughout:
//   - fresh payer/issuer/merchant keypairs funded via Friendbot;
//   - a classic testnet asset whose SEP-41 Stellar Asset Contract (SAC) is
//     deployed on-chain;
//   - an httptest resource server that issues a real v2 PAYMENT-REQUIRED
//     challenge and doubles as the facilitator — it rebuilds the payer-signed
//     transaction with its own source account and submits it to testnet;
//   - the real Soroban RPC for simulation, auth recording and settlement.
//
// Run it explicitly (it makes live testnet transactions and takes minutes):
//
//	cd pkg && go test -tags e2e ./x402/ -run TestX402EndToEndTestnet -v -timeout 10m
//
// Only public addresses and transaction hashes are logged. Secret seeds never
// leave the test process.
package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	rpcclient "github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	e2eAssetCode    = "X402TEST"
	e2ePayment      = "1000000" // 0.1 tokens at 7 decimals
	e2eFundRetries  = 3
	e2ePollInterval = 3 * time.Second
)

func TestX402EndToEndTestnet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live testnet e2e in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	// Fresh wallets: payer pays, issuer issues the asset and acts as the
	// facilitator (pays the settlement transaction fee), merchant receives.
	payer, err := keypair.Random()
	if err != nil {
		t.Fatalf("generate payer keypair: %v", err)
	}
	issuer, err := keypair.Random()
	if err != nil {
		t.Fatalf("generate issuer keypair: %v", err)
	}
	merchant, err := keypair.Random()
	if err != nil {
		t.Fatalf("generate merchant keypair: %v", err)
	}
	t.Logf("payer address:    %s", payer.Address())
	t.Logf("issuer address:   %s", issuer.Address())
	t.Logf("merchant address: %s", merchant.Address())

	for _, kp := range []*keypair.Full{payer, issuer, merchant} {
		fundTestnetAccount(t, ctx, kp.Address())
	}

	horizon := horizonclient.DefaultTestNetClient
	asset := txnbuild.CreditAsset{Code: e2eAssetCode, Issuer: issuer.Address()}

	// Payer and merchant need trustlines; the issuer funds the payer's balance.
	submitClassic(t, horizon, network.TestNetworkPassphrase, payer, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(t, horizon, network.TestNetworkPassphrase, merchant, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(t, horizon, network.TestNetworkPassphrase, issuer, &txnbuild.Payment{
		Destination: payer.Address(),
		Amount:      "1000",
		Asset:       asset,
	})

	// Deploy the SEP-41 SAC for the asset so the x402 exact scheme can settle
	// via a contract transfer.
	sorobanClient := soroban.NewClient(soroban.TestnetRPCURL, network.TestNetworkPassphrase)
	defer func() { _ = sorobanClient.Close() }()

	contractID, deployTx, err := sorobanClient.CreateContractForAsset(ctx, issuer, asset)
	if err != nil {
		t.Fatalf("deploy SAC: %v", err)
	}
	t.Logf("SAC contract: %s (deploy tx %s)", contractID, deployTx)

	payment, ok := new(big.Int).SetString(e2ePayment, 10)
	if !ok {
		t.Fatalf("invalid payment amount %q", e2ePayment)
	}
	balanceBefore := readSACBalance(t, ctx, sorobanClient, contractID, merchant.Address())

	facilitator := &e2eFacilitator{
		t:            t,
		rpcURL:       soroban.TestnetRPCURL,
		passphrase:   network.TestNetworkPassphrase,
		networkName:  "stellar:testnet",
		facilitator:  issuer,
		contractID:   contractID,
		payTo:        merchant.Address(),
		payer:        payer.Address(),
		atomicAmount: e2ePayment,
	}
	server := httptest.NewServer(http.HandlerFunc(facilitator.serve))
	defer server.Close()
	facilitator.resourceURL = server.URL

	// Real x402 client flow against the live testnet RPC.
	client, builderRPC, err := NewStellarClient("stellar:testnet", payer, &http.Client{Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("create x402 client: %v", err)
	}
	defer func() { _ = builderRPC.Close() }()

	result, err := client.Fetch(ctx, Request{
		Method: http.MethodGet,
		URL:    server.URL + "/premium",
	}, FetchOptions{})
	if err != nil {
		t.Fatalf("x402 fetch failed: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 after payment, got %d (body %q)", result.StatusCode, result.BodyText)
	}
	if !result.PaymentAttempted {
		t.Fatal("expected a payment attempt")
	}
	if result.Settlement == nil || !result.Settlement.Success {
		t.Fatalf("expected successful settlement, got %+v", result.Settlement)
	}
	if result.Settlement.Transaction == "" {
		t.Fatal("settlement is missing the on-chain transaction hash")
	}
	t.Logf("settled on-chain: tx %s", result.Settlement.Transaction)

	// Verify the merchant's on-chain SAC balance increased by the payment.
	balanceAfter := readSACBalance(t, ctx, sorobanClient, contractID, merchant.Address())
	delta := new(big.Int).Sub(balanceAfter, balanceBefore)
	if delta.Cmp(payment) != 0 {
		t.Fatalf("merchant balance moved %s, expected %s", delta, payment)
	}
	t.Logf("merchant balance delta: %s stroops-equivalent units", delta)
}

// fundTestnetAccount funds a fresh account through Friendbot with retries.
func fundTestnetAccount(t *testing.T, ctx context.Context, address string) {
	t.Helper()
	url := "https://friendbot.stellar.org/?addr=" + address
	var lastErr error
	for attempt := 1; attempt <= e2eFundRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build friendbot request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Logf("friendbot funded %s", address)
				return
			}
			err = fmt.Errorf("friendbot status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if readErr != nil {
				err = fmt.Errorf("friendbot status %d (read error: %v)", resp.StatusCode, readErr)
			}
		}
		lastErr = err
		select {
		case <-ctx.Done():
			t.Fatalf("friendbot funding timed out for %s: %v", address, ctx.Err())
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	t.Fatalf("friendbot funding failed for %s: %v", address, lastErr)
}

// submitClassic builds, signs and submits a single-operation classic Stellar
// transaction through Horizon.
func submitClassic(t *testing.T, horizon *horizonclient.Client, passphrase string, kp *keypair.Full, ops ...txnbuild.Operation) {
	t.Helper()
	account, err := horizon.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		t.Fatalf("load account %s: %v", kp.Address(), err)
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &account,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           ops,
	})
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}
	tx, err = tx.Sign(passphrase, kp)
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	resp, err := horizon.SubmitTransaction(tx)
	if err != nil {
		t.Fatalf("submit transaction: %v", err)
	}
	t.Logf("submitted tx %s", resp.Hash)
}

// readSACBalance simulates the SEP-41 balance method and returns the i128.
func readSACBalance(t *testing.T, ctx context.Context, client *soroban.Client, contractID, address string) *big.Int {
	t.Helper()
	addr, err := soroban.AccountToScAddress(address)
	if err != nil {
		t.Fatalf("convert address: %v", err)
	}
	retXDR, err := client.SimulateOnly(ctx, contractID, "balance", []xdr.ScVal{soroban.ScvAddress(addr)})
	if err != nil {
		t.Fatalf("simulate balance for %s: %v", address, err)
	}
	var val xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(retXDR, &val); err != nil {
		t.Fatalf("decode balance result: %v", err)
	}
	parts, ok := val.GetI128()
	if !ok {
		t.Fatalf("balance returned %s, expected i128", val.Type)
	}
	balance := new(big.Int).Lsh(big.NewInt(int64(parts.Hi)), 64)
	balance.Add(balance, new(big.Int).SetUint64(uint64(parts.Lo)))
	return balance
}

// e2eFacilitator is the resource server and facilitator for the e2e test. On
// the first request it returns a v2 PAYMENT-REQUIRED challenge; on the retry
// it verifies and settles the payment on the live network by rebuilding the
// payer's transaction envelope with its own source account.
type e2eFacilitator struct {
	t            *testing.T
	rpcURL       string
	passphrase   string
	networkName  string
	facilitator  *keypair.Full
	contractID   string
	payTo        string
	payer        string
	atomicAmount string
	resourceURL  string
}

func (f *e2eFacilitator) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(HeaderPaymentSignature) == "" {
		f.serveChallenge(w)
		return
	}
	f.serveSettle(w, r)
}

func (f *e2eFacilitator) serveChallenge(w http.ResponseWriter) {
	required := PaymentRequired{
		X402Version: Version2,
		Resource:    &ResourceInfo{URL: f.resourceURL + "/premium"},
		Accepts: []PaymentRequirements{
			{
				Scheme:            "exact",
				Network:           f.networkName,
				Asset:             f.contractID,
				Amount:            f.atomicAmount,
				PayTo:             f.payTo,
				MaxTimeoutSeconds: 300,
				Extra:             map[string]interface{}{"areFeesSponsored": true},
			},
		},
	}
	data, err := json.Marshal(required)
	if err != nil {
		f.t.Logf("marshal challenge: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(HeaderPaymentRequired, base64.StdEncoding.EncodeToString(data))
	w.WriteHeader(http.StatusPaymentRequired)
}

func (f *e2eFacilitator) serveSettle(w http.ResponseWriter, r *http.Request) {
	payload, err := DecodePaymentSignatureHeader(r.Header.Get(HeaderPaymentSignature))
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

	settlement := SettlementResponse{
		Success:     true,
		Payer:       f.payer,
		Transaction: txHash,
		Network:     f.networkName,
		Amount:      f.atomicAmount,
	}
	data, _ := json.Marshal(settlement)
	w.Header().Set(HeaderPaymentResponse, base64.StdEncoding.EncodeToString(data))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"data":"premium content delivered"}`))
}

func (f *e2eFacilitator) reject(w http.ResponseWriter, reason, message string) {
	settlement := SettlementResponse{
		Success:      false,
		ErrorReason:  reason,
		ErrorMessage: message,
		Network:      f.networkName,
	}
	data, _ := json.Marshal(settlement)
	w.Header().Set(HeaderPaymentResponse, base64.StdEncoding.EncodeToString(data))
	w.WriteHeader(http.StatusPaymentRequired)
}

// settle rebuilds the payer-signed transaction with the facilitator account as
// the envelope source (so the facilitator pays the fee), signs and submits it
// to testnet, then polls for confirmation.
func (f *e2eFacilitator) settle(ctx context.Context, txB64 string) (string, error) {
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

	account, err := rpcClient.LoadAccount(ctx, f.facilitator.Address())
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
	tx, err = tx.Sign(f.passphrase, f.facilitator)
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
		case <-time.After(e2ePollInterval):
		}
	}
}
