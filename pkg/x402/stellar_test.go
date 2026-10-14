package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/hash"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func testAuthorizationEntry(t *testing.T, signer *keypair.Full) xdr.SorobanAuthorizationEntry {
	t.Helper()
	from, err := ParseDestinationAddress(signer.Address())
	if err != nil {
		t.Fatalf("parse signer address: %v", err)
	}
	to, err := ParseDestinationAddress(keypair.MustRandom().Address())
	if err != nil {
		t.Fatalf("parse recipient address: %v", err)
	}
	contract, err := soroban.ParseContractID(testContractID(t))
	if err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	amount := soroban.ScvI128(big.NewInt(1000000))

	return xdr.SorobanAuthorizationEntry{
		Credentials: xdr.SorobanCredentials{
			Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
			Address: &xdr.SorobanAddressCredentials{
				Address:                   from,
				Nonce:                     12345,
				SignatureExpirationLedger: 0,
				Signature:                 xdr.ScVal{Type: xdr.ScValTypeScvVoid},
			},
		},
		RootInvocation: xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: contract,
					FunctionName:    xdr.ScSymbol(stellarTransferMethod),
					Args: xdr.ScVec{
						soroban.ScvAddress(from),
						soroban.ScvAddress(to),
						amount,
					},
				},
			},
		},
	}
}

func TestSignAuthorizationEntry(t *testing.T) {
	signer := keypair.MustRandom()
	entry := testAuthorizationEntry(t, signer)
	signed, err := SignAuthorizationEntry(entry, signer, network.TestNetworkPassphrase, 123456)
	if err != nil {
		t.Fatalf("SignAuthorizationEntry: %v", err)
	}
	credentials := signed.Credentials.Address
	if credentials == nil {
		t.Fatal("signed entry has no address credentials")
	}
	if credentials.SignatureExpirationLedger != 123456 {
		t.Fatalf("unexpected expiration ledger %d", credentials.SignatureExpirationLedger)
	}

	signatureVecPtr, ok := credentials.Signature.GetVec()
	if !ok || signatureVecPtr == nil || len(*signatureVecPtr) != 1 {
		t.Fatalf("signature is not a one-item vector: %v", credentials.Signature.Type)
	}
	signatureMapPtr, ok := (*signatureVecPtr)[0].GetMap()
	if !ok || signatureMapPtr == nil || len(*signatureMapPtr) != 2 {
		t.Fatalf("signature value is not a two-entry map")
	}

	var publicKey []byte
	var signature []byte
	for _, pair := range *signatureMapPtr {
		key, ok := pair.Key.GetSym()
		if !ok {
			t.Fatal("signature map key is not a symbol")
		}
		value, ok := pair.Val.GetBytes()
		if !ok {
			t.Fatalf("signature map value for %s is not bytes", key)
		}
		switch string(key) {
		case "public_key":
			publicKey = value
		case "signature":
			signature = value
		default:
			t.Fatalf("unexpected signature map key %q", key)
		}
	}
	if len(publicKey) != 32 || len(signature) != 64 {
		t.Fatalf("unexpected public key/signature lengths %d/%d", len(publicKey), len(signature))
	}
	encodedPublicKey, err := strkey.Encode(strkey.VersionByteAccountID, publicKey)
	if err != nil {
		t.Fatalf("encode public key: %v", err)
	}
	if encodedPublicKey != signer.Address() {
		t.Fatalf("signature public key %s does not match signer", encodedPublicKey)
	}

	preimage := xdr.HashIdPreimage{
		Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorization,
		SorobanAuthorization: &xdr.HashIdPreimageSorobanAuthorization{
			NetworkId:                 network.ID(network.TestNetworkPassphrase),
			Nonce:                     credentials.Nonce,
			SignatureExpirationLedger: credentials.SignatureExpirationLedger,
			Invocation:                signed.RootInvocation,
		},
	}
	preimageBytes, err := preimage.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal authorization preimage: %v", err)
	}
	payload := hash.Hash(preimageBytes)
	if err := signer.Verify(payload[:], signature); err != nil {
		t.Fatalf("verify signed authorization: %v", err)
	}
}

func TestSignAuthorizationEntryRejectsWrongSigner(t *testing.T) {
	entry := testAuthorizationEntry(t, keypair.MustRandom())
	other := keypair.MustRandom()
	if _, err := SignAuthorizationEntry(entry, other, network.TestNetworkPassphrase, 123456); err == nil {
		t.Fatal("expected signer/address mismatch error")
	}
}

func TestValidateTransferAuthEntry(t *testing.T) {
	signer := keypair.MustRandom()
	entry := testAuthorizationEntry(t, signer)
	from := entry.Credentials.Address.Address
	to := entry.RootInvocation.Function.ContractFn.Args[1]
	toAddress, ok := to.GetAddress()
	if !ok {
		t.Fatal("decode destination argument")
	}
	contract := entry.RootInvocation.Function.ContractFn.ContractAddress
	amount := big.NewInt(1000000)
	if err := validateTransferAuthEntry(entry, contract, from, toAddress, amount, signer.Address()); err != nil {
		t.Fatalf("validate auth entry: %v", err)
	}
}

func TestNormalizeRPCURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty"},
		{name: "https", raw: "https://mainnet.sorobanrpc.com"},
		{name: "local http", raw: "http://localhost:8000/soroban/rpc"},
		{name: "remote http", raw: "http://rpc.example.com", wantErr: true},
		{name: "credentials", raw: "https://user:pass@mainnet.sorobanrpc.com", wantErr: true},
		{name: "missing host", raw: "https://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeRPCURL(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeRPCURL(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestBuildTransferTransactionWithMockRPC(t *testing.T) {
	signer := keypair.MustRandom()
	recipient := keypair.MustRandom().Address()
	contractID := testContractID(t)
	authEntry := testAuthorizationEntry(t, signer)
	recipientAddress, err := ParseDestinationAddress(recipient)
	if err != nil {
		t.Fatalf("parse recipient: %v", err)
	}
	authEntry.RootInvocation.Function.ContractFn.Args[1] = soroban.ScvAddress(recipientAddress)
	contractAddress, err := soroban.ParseContractID(contractID)
	if err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	authEntry.RootInvocation.Function.ContractFn.ContractAddress = contractAddress

	sorobanData := xdr.SorobanTransactionData{
		Resources: xdr.SorobanResources{
			Instructions:  1000,
			DiskReadBytes: 10,
			WriteBytes:    10,
		},
		ResourceFee: 100,
	}
	sorobanDataBytes, err := sorobanData.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal Soroban data: %v", err)
	}
	authBytes, err := authEntry.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal auth entry: %v", err)
	}

	var recordCalls atomic.Int32
	var enforceCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode JSON-RPC request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		writeResult := func(result interface{}) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  result,
			})
		}
		writeError := func() {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]interface{}{"code": -32601, "message": "not implemented"},
			})
		}

		switch request.Method {
		case "getLatestLedger":
			writeResult(map[string]interface{}{
				"id":              strings.Repeat("0", 64),
				"protocolVersion": 23,
				"sequence":        1000,
			})
		case "getLedgers":
			writeError()
		case "simulateTransaction":
			var params rpc.SimulateTransactionRequest
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatalf("decode simulate params: %v", err)
			}
			switch params.AuthMode {
			case rpc.AuthModeRecordAllowNonroot:
				recordCalls.Add(1)
				auth := []string{base64.StdEncoding.EncodeToString(authBytes)}
				writeResult(rpc.SimulateTransactionResponse{
					TransactionDataXDR: base64.StdEncoding.EncodeToString(sorobanDataBytes),
					Results: []rpc.SimulateHostFunctionResult{
						{AuthXDR: &auth},
					},
					LatestLedger: 1000,
				})
			case rpc.AuthModeEnforce:
				enforceCalls.Add(1)
				writeResult(rpc.SimulateTransactionResponse{
					TransactionDataXDR: base64.StdEncoding.EncodeToString(sorobanDataBytes),
					LatestLedger:       1000,
				})
			default:
				t.Fatalf("unexpected auth mode %q", params.AuthMode)
			}
		default:
			writeError()
		}
	}))
	defer server.Close()

	rpcClient := soroban.NewClient(server.URL, network.PublicNetworkPassphrase)
	defer rpcClient.Close()
	builder, err := NewStellarExactBuilder(signer, rpcClient)
	if err != nil {
		t.Fatalf("NewStellarExactBuilder: %v", err)
	}
	requirement := validRequirement(t, NetworkStellarPubnet)
	requirement.Asset = contractID
	requirement.PayTo = recipient
	requirement.Amount = "1000000"
	requirement.MaxTimeoutSeconds = 60

	transaction, err := builder.BuildTransferTransaction(context.Background(), &requirement)
	if err != nil {
		t.Fatalf("BuildTransferTransaction: %v", err)
	}
	if recordCalls.Load() != 1 || enforceCalls.Load() != 1 {
		t.Fatalf("unexpected simulation calls: record=%d enforce=%d", recordCalls.Load(), enforceCalls.Load())
	}

	var envelope xdr.TransactionEnvelope
	if err := xdr.SafeUnmarshalBase64(transaction, &envelope); err != nil {
		t.Fatalf("decode transaction envelope: %v", err)
	}
	if len(envelope.V1.Signatures) != 0 {
		t.Fatal("x402 payment transaction must not be envelope-signed by the payer")
	}
	if envelope.V1.Tx.Fee != xdr.Uint32(stellarX402BaseFee+100) {
		t.Fatalf("transaction fee %d does not include Soroban resource fee", envelope.V1.Tx.Fee)
	}
	if envelope.V1.Tx.Cond.TimeBounds == nil ||
		int64(envelope.V1.Tx.Cond.TimeBounds.MaxTime) > time.Now().Unix()+60 {
		t.Fatal("transaction time bound does not respect maxTimeoutSeconds")
	}
	if len(envelope.V1.Tx.Operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(envelope.V1.Tx.Operations))
	}
	op := envelope.V1.Tx.Operations[0].Body.InvokeHostFunctionOp
	if op == nil || len(op.Auth) != 1 {
		t.Fatalf("expected one invokeHostFunction auth entry")
	}
	credentials := op.Auth[0].Credentials.Address
	if credentials == nil || credentials.SignatureExpirationLedger == 0 {
		t.Fatal("signed payer authorization entry is missing credentials")
	}
	if credentials.SignatureExpirationLedger > 1012 {
		t.Fatalf("expiration ledger %d exceeds timeout bound", credentials.SignatureExpirationLedger)
	}
}
