package soroban

import (
	"context"
	"fmt"
	"net/http"
	"time"

	rpcclient "github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	defaultPollInterval = 3 * time.Second
	defaultBaseFee      = int64(100_000)
)

type Client struct {
	rpc        *rpcclient.Client
	passphrase string
	rpcURL     string
}

func NewClient(rpcURL, passphrase string) *Client {
	httpClient := &http.Client{}
	return &Client{
		rpc:        rpcclient.NewClient(rpcURL, httpClient),
		passphrase: passphrase,
		rpcURL:     rpcURL,
	}
}

func NewClientForNetwork(network string) *Client {
	return NewClient(NetworkRPCURL(network), NetworkPassphrase(network))
}

func (c *Client) Close() error {
	return c.rpc.Close()
}

func (c *Client) Passphrase() string {
	return c.passphrase
}

func (c *Client) LoadAccount(ctx context.Context, address string) (txnbuild.Account, error) {
	return c.rpc.LoadAccount(ctx, address)
}

// LatestLedger returns the latest ledger known by the configured RPC server.
func (c *Client) LatestLedger(ctx context.Context) (uint32, error) {
	resp, err := c.rpc.GetLatestLedger(ctx)
	if err != nil {
		return 0, err
	}
	return resp.Sequence, nil
}

// EstimatedLedgerSeconds estimates ledger close time from recent ledgers. It
// falls back to the five-second network estimate used by the x402 Stellar
// specification when RPC ledger history is unavailable.
func (c *Client) EstimatedLedgerSeconds(ctx context.Context) int64 {
	const fallback int64 = 5

	latest, err := c.LatestLedger(ctx)
	if err != nil || latest == 0 {
		return fallback
	}
	start := uint32(1)
	if latest > 20 {
		start = latest - 20
	}
	resp, err := c.rpc.GetLedgers(ctx, rpc.GetLedgersRequest{
		StartLedger: start,
		Pagination:  &rpc.LedgerPaginationOptions{Limit: 20},
	})
	if err != nil || len(resp.Ledgers) < 2 {
		return fallback
	}
	first := resp.Ledgers[0]
	last := resp.Ledgers[len(resp.Ledgers)-1]
	seconds := (last.LedgerCloseTime - first.LedgerCloseTime + int64(len(resp.Ledgers)-2)) / int64(len(resp.Ledgers)-1)
	if seconds <= 0 {
		return fallback
	}
	return seconds
}

// SimulateTransaction submits an already-built transaction to Soroban RPC.
func (c *Client) SimulateTransaction(ctx context.Context, tx *txnbuild.Transaction, authMode string) (*rpc.SimulateTransactionResponse, error) {
	txB64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}
	resp, err := c.rpc.SimulateTransaction(ctx, rpc.SimulateTransactionRequest{
		Transaction: txB64,
		AuthMode:    authMode,
	})
	if err != nil {
		return nil, fmt.Errorf("simulation failed: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("simulation error: %s", resp.Error)
	}
	return &resp, nil
}

// SimulateAndAssemble builds a tx with the given operation, simulates it, and
// returns the transaction assembled with the simulation's Soroban data/auth.
func (c *Client) SimulateAndAssemble(
	ctx context.Context,
	sourceAccount txnbuild.Account,
	op *txnbuild.InvokeHostFunction,
	baseFee int64,
	authMode string,
) (*txnbuild.Transaction, *rpc.SimulateTransactionResponse, error) {
	return c.simulateAndAssemble(ctx, sourceAccount, op, baseFee, 0, authMode)
}

// SimulateAndAssembleWithTimeout is SimulateAndAssemble with an explicit
// transaction time bound in seconds.
func (c *Client) SimulateAndAssembleWithTimeout(
	ctx context.Context,
	sourceAccount txnbuild.Account,
	op *txnbuild.InvokeHostFunction,
	baseFee int64,
	timeoutSeconds int64,
	authMode string,
) (*txnbuild.Transaction, *rpc.SimulateTransactionResponse, error) {
	return c.simulateAndAssemble(ctx, sourceAccount, op, baseFee, timeoutSeconds, authMode)
}

// simulateAndAssemble builds a tx with the given operation, simulates it,
// then rebuilds with the SorobanTransactionData and auth from the simulation.
func (c *Client) simulateAndAssemble(
	ctx context.Context,
	sourceAccount txnbuild.Account,
	op *txnbuild.InvokeHostFunction,
	baseFee int64,
	timeoutSeconds int64,
	authMode string,
) (*txnbuild.Transaction, *rpc.SimulateTransactionResponse, error) {
	if baseFee <= 0 {
		baseFee = defaultBaseFee
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        sourceAccount,
		IncrementSequenceNum: true,
		BaseFee:              baseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(timeoutSeconds)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build initial transaction: %w", err)
	}

	simResp, err := c.SimulateTransaction(ctx, tx, authMode)
	if err != nil {
		return nil, nil, err
	}

	var sorobanData xdr.SorobanTransactionData
	if simResp.TransactionDataXDR != "" {
		if err := xdr.SafeUnmarshalBase64(simResp.TransactionDataXDR, &sorobanData); err != nil {
			return nil, nil, fmt.Errorf("failed to parse SorobanTransactionData: %w", err)
		}
	}

	if len(simResp.Results) > 0 && simResp.Results[0].AuthXDR != nil {
		authEntries := make([]xdr.SorobanAuthorizationEntry, 0, len(*simResp.Results[0].AuthXDR))
		for _, authB64 := range *simResp.Results[0].AuthXDR {
			var entry xdr.SorobanAuthorizationEntry
			if err := xdr.SafeUnmarshalBase64(authB64, &entry); err != nil {
				return nil, nil, fmt.Errorf("failed to parse auth entry: %w", err)
			}
			authEntries = append(authEntries, entry)
		}
		op.Auth = authEntries
	}

	op.Ext = xdr.TransactionExt{
		V:           1,
		SorobanData: &sorobanData,
	}

	rebuiltTx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: tx.SourceAccount().AccountID,
			Sequence:  tx.SequenceNumber(),
		},
		IncrementSequenceNum: false,
		BaseFee:              baseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(timeoutSeconds)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to rebuild transaction with simulation data: %w", err)
	}

	return rebuiltTx, simResp, nil
}

// submitAndWait signs the transaction with the given keypair, submits it,
// and polls until it's confirmed on-chain.
func (c *Client) submitAndWait(ctx context.Context, tx *txnbuild.Transaction, kp *keypair.Full) (txHash string, resultXDR string, resultMetaXDR string, err error) {
	signedTx, err := tx.Sign(c.passphrase, kp)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	txB64, err := signedTx.Base64()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to serialize signed transaction: %w", err)
	}

	sendResp, err := c.rpc.SendTransaction(ctx, rpc.SendTransactionRequest{
		Transaction: txB64,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	switch sendResp.Status {
	case "ERROR":
		return "", "", "", fmt.Errorf("transaction rejected: %s", sendResp.ErrorResultXDR)
	case "DUPLICATE":
		return sendResp.Hash, "", "", nil
	}

	txHash = sendResp.Hash

	for {
		getResp, err := c.rpc.GetTransaction(ctx, rpc.GetTransactionRequest{
			Hash: txHash,
		})
		if err != nil {
			return txHash, "", "", fmt.Errorf("failed to get transaction: %w", err)
		}

		switch getResp.Status {
		case "SUCCESS":
			return txHash, getResp.ResultXDR, getResp.ResultMetaXDR, nil
		case "FAILED":
			return txHash, getResp.ResultXDR, getResp.ResultMetaXDR, fmt.Errorf("transaction failed on-chain")
		}

		select {
		case <-ctx.Done():
			return txHash, "", "", ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}
