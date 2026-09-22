package soroban

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	rpc "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
)

// InvokeResult contains the result of a contract invocation.
type InvokeResult struct {
	TxHash        string
	ResultXDR     string
	ResultMetaXDR string
}

// Invoke submits a contract method call transaction to the network.
func (c *Client) Invoke(
	ctx context.Context,
	kp *keypair.Full,
	contractID string,
	method string,
	args []xdr.ScVal,
) (*InvokeResult, error) {
	contractAddr, err := ParseContractID(contractID)
	if err != nil {
		return nil, fmt.Errorf("invalid contract ID: %w", err)
	}

	sourceAccount, err := c.LoadAccount(ctx, kp.Address())
	if err != nil {
		return nil, fmt.Errorf("failed to load source account: %w", err)
	}

	op := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractAddr,
				FunctionName:    xdr.ScSymbol(method),
				Args:            args,
			},
		},
		SourceAccount: kp.Address(),
	}

	txHash, resultXDR, resultMetaXDR, err := c.simulateAndAssembleAndSubmit(ctx, sourceAccount, op, kp, 0)
	if err != nil {
		return nil, err
	}

	return &InvokeResult{
		TxHash:        txHash,
		ResultXDR:     resultXDR,
		ResultMetaXDR: resultMetaXDR,
	}, nil
}

// SimulateOnly runs a read-only contract method call via simulation (no transaction submitted).
func (c *Client) SimulateOnly(
	ctx context.Context,
	contractID string,
	method string,
	args []xdr.ScVal,
) (returnValueXDR string, err error) {
	contractAddr, err := ParseContractID(contractID)
	if err != nil {
		return "", fmt.Errorf("invalid contract ID: %w", err)
	}

	op := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractAddr,
				FunctionName:    xdr.ScSymbol(method),
				Args:            args,
			},
		},
	}

	dummyKP, err := keypair.Random()
	if err != nil {
		return "", fmt.Errorf("generate dummy keypair: %w", err)
	}

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &txnbuild.SimpleAccount{AccountID: dummyKP.Address(), Sequence: 0},
		IncrementSequenceNum: true,
		BaseFee:              defaultBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	txB64, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	simResp, err := c.rpc.SimulateTransaction(ctx, rpc.SimulateTransactionRequest{
		Transaction: txB64,
	})
	if err != nil {
		return "", fmt.Errorf("simulation failed: %w", err)
	}
	if simResp.Error != "" {
		return "", fmt.Errorf("simulation error: %s", simResp.Error)
	}

	if len(simResp.Results) == 0 {
		return "", fmt.Errorf("no simulation results returned")
	}

	if simResp.Results[0].ReturnValueXDR == nil {
		return "", fmt.Errorf("no return value in simulation result")
	}

	return *simResp.Results[0].ReturnValueXDR, nil
}
