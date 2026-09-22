package soroban

import (
	"context"
	"fmt"
	"net/http"
	"time"

	rpcclient "github.com/stellar/go/clients/rpcclient"
	"github.com/stellar/go/keypair"
	rpc "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
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

// simulateAndAssemble builds a tx with the given operation, simulates it,
// then rebuilds with the SorobanTransactionData and auth from the simulation.
func (c *Client) simulateAndAssemble(
	ctx context.Context,
	sourceAccount txnbuild.Account,
	op *txnbuild.InvokeHostFunction,
	baseFee int64,
) (*txnbuild.Transaction, *rpc.SimulateTransactionResponse, error) {
	if baseFee <= 0 {
		baseFee = defaultBaseFee
	}

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        sourceAccount,
		IncrementSequenceNum: true,
		BaseFee:              baseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build initial transaction: %w", err)
	}

	txB64, err := tx.Base64()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	simResp, err := c.rpc.SimulateTransaction(ctx, rpc.SimulateTransactionRequest{
		Transaction: txB64,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("simulation failed: %w", err)
	}
	if simResp.Error != "" {
		return nil, nil, fmt.Errorf("simulation error: %s", simResp.Error)
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
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to rebuild transaction with simulation data: %w", err)
	}

	return rebuiltTx, &simResp, nil
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
