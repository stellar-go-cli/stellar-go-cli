package soroban

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	rpc "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
)

// DeployResult contains the result of a contract deployment.
type DeployResult struct {
	ContractID   string
	WasmHash     string
	UploadTxHash string
	CreateTxHash string
}

// UploadWasm uploads WASM bytecode to the Soroban network and returns the wasm hash.
func (c *Client) UploadWasm(ctx context.Context, kp *keypair.Full, wasmBytes []byte) (wasmHash xdr.Hash, txHash string, err error) {
	sourceAccount, err := c.LoadAccount(ctx, kp.Address())
	if err != nil {
		return xdr.Hash{}, "", fmt.Errorf("failed to load source account: %w", err)
	}

	wasmCopy := wasmBytes
	op := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeUploadContractWasm,
			Wasm: &wasmCopy,
		},
		SourceAccount: kp.Address(),
	}

	assembledTx, simResp, err := c.simulateAndAssemble(ctx, sourceAccount, op, 0)
	if err != nil {
		return xdr.Hash{}, "", err
	}

	// Extract wasm hash from simulation return value
	wasmHash, err = extractWasmHashFromSimResult(simResp)
	if err != nil {
		return xdr.Hash{}, "", fmt.Errorf("failed to extract wasm hash from simulation: %w", err)
	}

	txHash, _, _, err = c.submitAndWait(ctx, assembledTx, kp)
	if err != nil {
		return xdr.Hash{}, txHash, err
	}

	return wasmHash, txHash, nil
}

// CreateContract creates a contract instance from an uploaded wasm hash.
// If constructorArgs is non-nil, uses CreateContractV2 to pass them; otherwise uses V1.
func (c *Client) CreateContract(ctx context.Context, kp *keypair.Full, wasmHash xdr.Hash, salt [32]byte, constructorArgs []xdr.ScVal) (contractID string, txHash string, err error) {
	sourceAccount, err := c.LoadAccount(ctx, kp.Address())
	if err != nil {
		return "", "", fmt.Errorf("failed to load source account: %w", err)
	}

	accountScAddr, err := AccountToScAddress(kp.Address())
	if err != nil {
		return "", "", fmt.Errorf("failed to convert account to ScAddress: %w", err)
	}

	preimage := xdr.ContractIdPreimage{
		Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
		FromAddress: &xdr.ContractIdPreimageFromAddress{
			Address: accountScAddr,
			Salt:    xdr.Uint256(salt),
		},
	}
	executable := xdr.ContractExecutable{
		Type:     xdr.ContractExecutableTypeContractExecutableWasm,
		WasmHash: &wasmHash,
	}

	var op *txnbuild.InvokeHostFunction
	if constructorArgs != nil {
		op = &txnbuild.InvokeHostFunction{
			HostFunction: xdr.HostFunction{
				Type: xdr.HostFunctionTypeHostFunctionTypeCreateContractV2,
				CreateContractV2: &xdr.CreateContractArgsV2{
					ContractIdPreimage: preimage,
					Executable:         executable,
					ConstructorArgs:    constructorArgs,
				},
			},
			SourceAccount: kp.Address(),
		}
	} else {
		op = &txnbuild.InvokeHostFunction{
			HostFunction: xdr.HostFunction{
				Type: xdr.HostFunctionTypeHostFunctionTypeCreateContract,
				CreateContract: &xdr.CreateContractArgs{
					ContractIdPreimage: preimage,
					Executable:         executable,
				},
			},
			SourceAccount: kp.Address(),
		}
	}

	assembledTx, simResp, err := c.simulateAndAssemble(ctx, sourceAccount, op, 0)
	if err != nil {
		return "", "", err
	}

	// Extract contract ID from simulation return value
	contractID, err = extractContractIDFromSimResult(simResp)
	if err != nil {
		return "", "", fmt.Errorf("failed to extract contract ID from simulation: %w", err)
	}

	txHash, _, _, err = c.submitAndWait(ctx, assembledTx, kp)
	if err != nil {
		return "", txHash, err
	}

	return contractID, txHash, nil
}

// Deploy is a convenience wrapper that uploads WASM and creates a contract instance.
func (c *Client) Deploy(ctx context.Context, kp *keypair.Full, wasmBytes []byte) (*DeployResult, error) {
	return c.DeployWithConstructorArgs(ctx, kp, wasmBytes, nil)
}

// DeployWithConstructorArgs uploads WASM and creates a contract instance with constructor args.
func (c *Client) DeployWithConstructorArgs(ctx context.Context, kp *keypair.Full, wasmBytes []byte, constructorArgs []xdr.ScVal) (*DeployResult, error) {
	wasmHash, uploadTxHash, err := c.UploadWasm(ctx, kp, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("upload wasm failed: %w", err)
	}

	salt, err := RandomSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	contractID, createTxHash, err := c.CreateContract(ctx, kp, wasmHash, salt, constructorArgs)
	if err != nil {
		return nil, fmt.Errorf("create contract failed: %w", err)
	}

	return &DeployResult{
		ContractID:   contractID,
		WasmHash:     fmt.Sprintf("%x", wasmHash[:]),
		UploadTxHash: uploadTxHash,
		CreateTxHash: createTxHash,
	}, nil
}

// simulateAndAssembleAndSubmit is a convenience wrapper.
func (c *Client) simulateAndAssembleAndSubmit(
	ctx context.Context,
	sourceAccount txnbuild.Account,
	op *txnbuild.InvokeHostFunction,
	kp *keypair.Full,
	baseFee int64,
) (txHash string, resultXDR string, resultMetaXDR string, err error) {
	assembledTx, _, err := c.simulateAndAssemble(ctx, sourceAccount, op, baseFee)
	if err != nil {
		return "", "", "", err
	}
	return c.submitAndWait(ctx, assembledTx, kp)
}

// extractWasmHashFromSimResult extracts the wasm hash from the simulation response's return value.
// For UploadContractWasm, the return value is an ScVal containing the hash as ScBytes.
func extractWasmHashFromSimResult(simResp *rpc.SimulateTransactionResponse) (xdr.Hash, error) {
	if len(simResp.Results) == 0 {
		return xdr.Hash{}, fmt.Errorf("no simulation results returned")
	}
	if simResp.Results[0].ReturnValueXDR == nil {
		return xdr.Hash{}, fmt.Errorf("no return value in simulation result")
	}

	var retVal xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(*simResp.Results[0].ReturnValueXDR, &retVal); err != nil {
		return xdr.Hash{}, fmt.Errorf("failed to parse return value: %w", err)
	}

	if retVal.Type != xdr.ScValTypeScvBytes || retVal.Bytes == nil {
		return xdr.Hash{}, fmt.Errorf("unexpected return value type: %s (expected bytes)", retVal.Type)
	}

	if len(*retVal.Bytes) != 32 {
		return xdr.Hash{}, fmt.Errorf("wasm hash must be 32 bytes, got %d", len(*retVal.Bytes))
	}

	var hash xdr.Hash
	copy(hash[:], *retVal.Bytes)
	return hash, nil
}

// extractContractIDFromSimResult extracts the contract ID from the simulation response's return value.
// For CreateContract, the return value is an ScVal containing the contract address.
func extractContractIDFromSimResult(simResp *rpc.SimulateTransactionResponse) (string, error) {
	if len(simResp.Results) == 0 {
		return "", fmt.Errorf("no simulation results returned")
	}
	if simResp.Results[0].ReturnValueXDR == nil {
		return "", fmt.Errorf("no return value in simulation result")
	}

	var retVal xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(*simResp.Results[0].ReturnValueXDR, &retVal); err != nil {
		return "", fmt.Errorf("failed to parse return value: %w", err)
	}

	if retVal.Type != xdr.ScValTypeScvAddress || retVal.Address == nil {
		return "", fmt.Errorf("unexpected return value type: %s (expected address)", retVal.Type)
	}

	return EncodeContractID(*retVal.Address)
}
