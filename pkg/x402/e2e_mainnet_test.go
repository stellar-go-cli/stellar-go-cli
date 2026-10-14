//go:build e2e

// Mainnet end-to-end test for the x402 Stellar exact scheme.
//
// Unlike the testnet e2e, mainnet has no Friendbot: the test requires a
// funded mainnet account whose secret seed is provided via the
// X402_MAINNET_SEED environment variable. That account funds the fresh
// wallets, issues the test asset, deploys its SAC, and acts as the settlement
// facilitator. The seed is never logged.
//
// Fresh payer/merchant accounts are created for the run; on cleanup their
// token balances are refunded to the issuer, trustlines removed, and the
// accounts merged back into the funder so reserves are recovered.
//
// Run explicitly (spends real XLM — a few XLM of reserves, mostly refunded):
//
//	X402_MAINNET_SEED=S... go test -tags e2e ./x402/ \
//	    -run TestX402EndToEndMainnet -v -timeout 15m
package x402

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	e2eMainnetSeedEnv  = "X402_MAINNET_SEED"
	e2eMainnetFunding  = "3" // XLM per fresh account: base + trustline reserve + fees
	e2eMainnetAsset    = "X402E2E"
	e2eMainnetIssuance = "10" // tokens paid to the payer (0.1 is settled)
)

func TestX402EndToEndMainnet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live mainnet e2e in short mode")
	}
	seed := os.Getenv(e2eMainnetSeedEnv)
	if seed == "" {
		t.Skipf("set %s to a funded mainnet account secret to run", e2eMainnetSeedEnv)
	}
	funder, err := keypair.ParseFull(seed)
	if err != nil {
		t.Fatalf("invalid %s: %v", e2eMainnetSeedEnv, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	payer, err := keypair.Random()
	if err != nil {
		t.Fatalf("generate payer keypair: %v", err)
	}
	merchant, err := keypair.Random()
	if err != nil {
		t.Fatalf("generate merchant keypair: %v", err)
	}
	t.Logf("funder address:   %s", funder.Address())
	t.Logf("payer address:    %s", payer.Address())
	t.Logf("merchant address: %s", merchant.Address())

	// Escrow the fresh seeds in a 0600 temp file until their accounts have
	// been merged back — if cleanup fails or the process dies, the funds are
	// still recoverable. The file (never the seeds) is the only thing logged.
	escrow, err := os.CreateTemp("", "x402-e2e-mainnet-*.json")
	if err != nil {
		t.Fatalf("create seed escrow file: %v", err)
	}
	if err := escrow.Chmod(0o600); err != nil {
		t.Fatalf("secure seed escrow file: %v", err)
	}
	escrowPath := escrow.Name()
	if err := json.NewEncoder(escrow).Encode(map[string]string{
		"payer":    payer.Seed(),
		"merchant": merchant.Seed(),
	}); err != nil {
		_ = escrow.Close()
		t.Fatalf("write seed escrow: %v", err)
	}
	if err := escrow.Close(); err != nil {
		t.Fatalf("close seed escrow: %v", err)
	}
	escrowNeeded := true
	t.Cleanup(func() {
		if escrowNeeded {
			t.Logf("WARNING: cleanup incomplete — fresh-account seeds kept at %s for manual recovery; delete when done", escrowPath)
			return
		}
		if err := os.Remove(escrowPath); err != nil {
			t.Logf("cleanup: could not remove seed escrow %s: %v", escrowPath, err)
		}
	})

	horizon := horizonclient.DefaultPublicNetClient
	passphrase := network.PublicNetworkPassphrase
	asset := txnbuild.CreditAsset{Code: e2eMainnetAsset, Issuer: funder.Address()}

	sorobanClient := soroban.NewClient(soroban.MainnetRPCURL, passphrase)
	defer func() { _ = sorobanClient.Close() }()

	// Fund fresh accounts, create trustlines, and issue the payer's balance.
	submitClassic(t, horizon, passphrase, funder,
		&txnbuild.CreateAccount{Destination: payer.Address(), Amount: e2eMainnetFunding},
		&txnbuild.CreateAccount{Destination: merchant.Address(), Amount: e2eMainnetFunding},
	)
	submitClassic(t, horizon, passphrase, payer, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(t, horizon, passphrase, merchant, &txnbuild.ChangeTrust{
		Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
		Limit: "1000000",
	})
	submitClassic(t, horizon, passphrase, funder, &txnbuild.Payment{
		Destination: payer.Address(),
		Amount:      e2eMainnetIssuance,
		Asset:       asset,
	})

	// SAC contract IDs are deterministic per asset, so the contract may
	// already exist from a previous run — probe it and deploy only if missing.
	contractID, err := soroban.ContractIDForAsset(passphrase, asset)
	if err != nil {
		t.Fatalf("derive SAC contract ID: %v", err)
	}
	if _, err := sorobanClient.SimulateOnly(ctx, contractID, "name", nil); err != nil {
		_, deployTx, err := sorobanClient.CreateContractForAsset(ctx, funder, asset)
		if err != nil {
			t.Fatalf("deploy SAC: %v", err)
		}
		t.Logf("SAC contract deployed: %s (tx %s)", contractID, deployTx)
		if _, err := sorobanClient.SimulateOnly(ctx, contractID, "name", nil); err != nil {
			t.Fatalf("SAC %s not usable after deploy: %v", contractID, err)
		}
	} else {
		t.Logf("SAC contract already deployed: %s", contractID)
	}

	// Best-effort refund: return token balances to the issuer, drop the
	// trustlines, and merge the fresh accounts back into the funder.
	t.Cleanup(func() {
		okPayer := mergeBack(t, horizon, sorobanClient, passphrase, contractID, asset, payer, funder.Address())
		okMerchant := mergeBack(t, horizon, sorobanClient, passphrase, contractID, asset, merchant, funder.Address())
		escrowNeeded = !(okPayer && okMerchant)
	})

	payment, ok := new(big.Int).SetString(e2ePayment, 10)
	if !ok {
		t.Fatalf("invalid payment amount %q", e2ePayment)
	}
	balanceBefore := readSACBalance(t, ctx, sorobanClient, contractID, merchant.Address())

	facilitator := &e2eFacilitator{
		t:            t,
		rpcURL:       soroban.MainnetRPCURL,
		passphrase:   passphrase,
		networkName:  NetworkStellarPubnet,
		facilitator:  funder,
		contractID:   contractID,
		payTo:        merchant.Address(),
		payer:        payer.Address(),
		atomicAmount: e2ePayment,
	}
	server := httptest.NewServer(http.HandlerFunc(facilitator.serve))
	defer server.Close()
	facilitator.resourceURL = server.URL

	client, builderRPC, err := NewStellarClient("stellar:pubnet", payer, &http.Client{Timeout: 60 * time.Second})
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

	balanceAfter := readSACBalance(t, ctx, sorobanClient, contractID, merchant.Address())
	delta := new(big.Int).Sub(balanceAfter, balanceBefore)
	if delta.Cmp(payment) != 0 {
		t.Fatalf("merchant balance moved %s, expected %s", delta, payment)
	}
	t.Logf("merchant balance delta: %s atomic units", delta)
}

// mergeBack refunds the account's asset balance to the issuer, removes the
// trustline, and merges the account into the funder — as a single atomic
// transaction so a partial teardown cannot strand the account. Returns true
// when the account was merged; failures are logged, not fatal.
func mergeBack(t *testing.T, horizon *horizonclient.Client, client *soroban.Client, passphrase, contractID string, asset txnbuild.CreditAsset, kp *keypair.Full, funderAddress string) bool {
	t.Helper()
	name := kp.Address()

	var balance *big.Int
	for attempt := 0; attempt < 4 && balance == nil; attempt++ {
		balance = readSACBalanceSafe(t, client, contractID, name)
		if balance == nil {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
	}

	var ops []txnbuild.Operation
	if balance != nil && balance.Sign() > 0 {
		ops = append(ops, &txnbuild.Payment{
			Destination: asset.GetIssuer(),
			Amount:      atomicToDecimal(balance, 7),
			Asset:       asset,
		})
	} else if balance == nil {
		t.Logf("cleanup: could not read %s balance; attempting trustline removal without refund", name)
	}
	ops = append(ops,
		&txnbuild.ChangeTrust{
			Line:  txnbuild.ChangeTrustAssetWrapper{Asset: asset},
			Limit: "0",
		},
		&txnbuild.AccountMerge{Destination: funderAddress},
	)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := submitCleanupTx(horizon, passphrase, kp, ops...); err == nil {
			t.Logf("merged %s back into funder", name)
			return true
		} else {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
	}
	t.Logf("cleanup: could not merge %s back into funder (account left on-chain): %v", name, lastErr)
	return false
}

// submitCleanupTx submits a cleanup transaction and returns the error (unlike
// submitClassic, it does not fail the test).
func submitCleanupTx(horizon *horizonclient.Client, passphrase string, kp *keypair.Full, ops ...txnbuild.Operation) error {
	account, err := horizon.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &account,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           ops,
	})
	if err != nil {
		return fmt.Errorf("build transaction: %w", err)
	}
	tx, err = tx.Sign(passphrase, kp)
	if err != nil {
		return fmt.Errorf("sign transaction: %w", err)
	}
	if _, err := horizon.SubmitTransaction(tx); err != nil {
		return fmt.Errorf("submit transaction: %w", err)
	}
	return nil
}

// readSACBalanceSafe reads the SEP-41 balance without failing; returns nil on
// any error so cleanup can proceed.
func readSACBalanceSafe(t *testing.T, client *soroban.Client, contractID, address string) *big.Int {
	t.Helper()
	addr, err := soroban.AccountToScAddress(address)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	retXDR, err := client.SimulateOnly(ctx, contractID, "balance", []xdr.ScVal{soroban.ScvAddress(addr)})
	if err != nil {
		return nil
	}
	var val xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(retXDR, &val); err != nil {
		return nil
	}
	parts, ok := val.GetI128()
	if !ok {
		return nil
	}
	balance := new(big.Int).Lsh(big.NewInt(int64(parts.Hi)), 64)
	balance.Add(balance, new(big.Int).SetUint64(uint64(parts.Lo)))
	return balance
}

// atomicToDecimal converts an integer atomic amount to a decimal string with
// the given number of fractional digits (7 for SEP-41 assets).
func atomicToDecimal(atomic *big.Int, decimals int) string {
	s := atomic.String()
	if decimals == 0 {
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	intPart := s[:len(s)-decimals]
	fracPart := s[len(s)-decimals:]
	for len(fracPart) > 0 && fracPart[len(fracPart)-1] == '0' {
		fracPart = fracPart[:len(fracPart)-1]
	}
	if fracPart == "" {
		return intPart
	}
	return fmt.Sprintf("%s.%s", intPart, fracPart)
}
