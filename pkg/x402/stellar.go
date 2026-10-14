package x402

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/url"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go-stellar-sdk/hash"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	// simulationSourceAddress is the all-zero sentinel account used by Stellar
	// contract tooling for simulations where the payer is a non-invoker.
	simulationSourceAddress = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"
	stellarTransferMethod   = "transfer"
	stellarX402BaseFee      = int64(10_000)
)

// StellarExactBuilder implements the x402 v2 `exact` scheme for Stellar. It
// builds a simulated SEP-41 transfer invocation and signs only the payer's
// authorization entries; the facilitator signs/submits the transaction
// envelope after verification.
type StellarExactBuilder struct {
	signer *keypair.Full
	rpc    *soroban.Client
}

func NewStellarExactBuilder(signer *keypair.Full, rpcClient *soroban.Client) (*StellarExactBuilder, error) {
	if signer == nil {
		return nil, fmt.Errorf("Stellar signer keypair is required")
	}
	if rpcClient == nil {
		return nil, fmt.Errorf("Soroban RPC client is required")
	}
	return &StellarExactBuilder{signer: signer, rpc: rpcClient}, nil
}

// Payer returns the signing account address.
func (b *StellarExactBuilder) Payer() string {
	return b.signer.Address()
}

// CreatePaymentPayload builds the x402 v2 payload with a base64 transaction
// containing one invokeHostFunction(transfer) and signed payer auth entries.
func (b *StellarExactBuilder) CreatePaymentPayload(
	ctx context.Context,
	required *PaymentRequired,
	accepted *PaymentRequirements,
) (*PaymentPayload, error) {
	if required == nil {
		return nil, fmt.Errorf("payment required challenge is empty")
	}
	if err := ValidateStellarRequirement(accepted); err != nil {
		return nil, err
	}

	transaction, err := b.BuildTransferTransaction(ctx, accepted)
	if err != nil {
		return nil, err
	}

	return &PaymentPayload{
		X402Version: Version2,
		Payload: map[string]interface{}{
			"transaction": transaction,
		},
		Accepted:   *accepted,
		Resource:   required.Resource,
		Extensions: required.Extensions,
	}, nil
}

// BuildTransferTransaction returns the unsigned transaction envelope XDR with
// signed authorization entries expected by a Stellar x402 facilitator.
func (b *StellarExactBuilder) BuildTransferTransaction(ctx context.Context, req *PaymentRequirements) (string, error) {
	if err := ValidateStellarRequirement(req); err != nil {
		return "", err
	}

	contractAddress, err := soroban.ParseContractID(req.Asset)
	if err != nil {
		return "", fmt.Errorf("invalid token contract: %w", err)
	}
	fromAddress, err := ParseDestinationAddress(b.signer.Address())
	if err != nil {
		return "", fmt.Errorf("invalid payer address: %w", err)
	}
	toAddress, err := ParseDestinationAddress(req.PayTo)
	if err != nil {
		return "", fmt.Errorf("invalid payTo address: %w", err)
	}
	amount, err := ParseAtomicAmount(req.Amount)
	if err != nil {
		return "", err
	}

	op := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractAddress,
				FunctionName:    xdr.ScSymbol(stellarTransferMethod),
				Args: xdr.ScVec{
					soroban.ScvAddress(fromAddress),
					soroban.ScvAddress(toAddress),
					soroban.ScvI128(amount),
				},
			},
		},
	}

	// Use the simulation sentinel as the transaction source so the payer is a
	// non-invoker and recording-mode simulation returns address credentials.
	source := &txnbuild.SimpleAccount{AccountID: simulationSourceAddress, Sequence: 0}
	assembled, simResp, err := b.rpc.SimulateAndAssembleWithTimeout(
		ctx, source, op, stellarX402BaseFee, int64(req.MaxTimeoutSeconds), rpc.AuthModeRecordAllowNonroot,
	)
	if err != nil {
		return "", err
	}
	if simResp == nil || simResp.RestorePreamble != nil {
		return "", fmt.Errorf("token simulation requires a ledger-entry restore preamble")
	}
	if len(simResp.Results) == 0 || simResp.Results[0].AuthXDR == nil || len(*simResp.Results[0].AuthXDR) == 0 {
		return "", fmt.Errorf("token simulation returned no authorization entries")
	}
	if len(op.Auth) != 1 {
		return "", fmt.Errorf("expected one payer authorization entry, got %d", len(op.Auth))
	}
	if err := validateTransferAuthEntry(op.Auth[0], contractAddress, fromAddress, toAddress, amount, b.signer.Address()); err != nil {
		return "", err
	}

	latestLedger := simResp.LatestLedger
	if latestLedger == 0 {
		latestLedger, err = b.rpc.LatestLedger(ctx)
		if err != nil {
			return "", fmt.Errorf("get latest ledger: %w", err)
		}
	}
	ledgerSeconds := b.rpc.EstimatedLedgerSeconds(ctx)
	ledgers := (uint64(req.MaxTimeoutSeconds) + uint64(ledgerSeconds) - 1) / uint64(ledgerSeconds)
	if ledgers > uint64(math.MaxUint32-latestLedger) {
		return "", fmt.Errorf("payment timeout exceeds ledger range")
	}
	maxLedger := latestLedger + uint32(ledgers)

	signedAuth, err := SignAuthorizationEntry(op.Auth[0], b.signer, b.rpc.Passphrase(), maxLedger)
	if err != nil {
		return "", err
	}
	op.Auth = []xdr.SorobanAuthorizationEntry{signedAuth}

	signedTx, err := rebuildInvokeTransaction(assembled, op, stellarX402BaseFee, int64(req.MaxTimeoutSeconds))
	if err != nil {
		return "", err
	}
	enforced, err := b.rpc.SimulateTransaction(ctx, signedTx, rpc.AuthModeEnforce)
	if err != nil {
		return "", fmt.Errorf("signed authorization simulation failed: %w", err)
	}
	if enforced != nil && enforced.RestorePreamble != nil {
		return "", fmt.Errorf("signed authorization simulation requires a ledger-entry restore preamble")
	}
	if enforced != nil && enforced.TransactionDataXDR != "" {
		var data xdr.SorobanTransactionData
		if err := xdr.SafeUnmarshalBase64(enforced.TransactionDataXDR, &data); err != nil {
			return "", fmt.Errorf("parse enforced Soroban transaction data: %w", err)
		}
		op.Ext = xdr.TransactionExt{V: 1, SorobanData: &data}
		if signedTx, err = rebuildInvokeTransaction(signedTx, op, stellarX402BaseFee, int64(req.MaxTimeoutSeconds)); err != nil {
			return "", err
		}
	}

	encoded, err := signedTx.Base64()
	if err != nil {
		return "", fmt.Errorf("serialize payment transaction: %w", err)
	}
	return encoded, nil
}

// SignAuthorizationEntry signs the payer's Soroban authorization payload and
// embeds the standard Ed25519 account signature ScVal shape:
// vec[{public_key, signature}]. Both credential arms are supported:
// SOROBAN_CREDENTIALS_ADDRESS signs the envelope type 9 preimage
// (SorobanAuthorization); SOROBAN_CREDENTIALS_ADDRESS_V2 (CAP-71, protocol 27+)
// signs the envelope type 10 preimage (SorobanAuthorizationWithAddress), which
// additionally binds the credential's address.
func SignAuthorizationEntry(
	entry xdr.SorobanAuthorizationEntry,
	signer *keypair.Full,
	passphrase string,
	validUntilLedger uint32,
) (xdr.SorobanAuthorizationEntry, error) {
	if signer == nil {
		return entry, fmt.Errorf("signer keypair is required")
	}
	credentials := addressCredentials(&entry)
	if credentials == nil {
		return entry, fmt.Errorf("unsupported Soroban credentials type %s", entry.Credentials.Type.String())
	}
	address, err := credentials.Address.String()
	if err != nil {
		return entry, fmt.Errorf("invalid authorization address: %w", err)
	}
	if !strings.EqualFold(address, signer.Address()) {
		return entry, fmt.Errorf("authorization entry address %s does not match signer %s", address, signer.Address())
	}
	if validUntilLedger == 0 {
		return entry, fmt.Errorf("signature expiration ledger is required")
	}

	encoded, err := entry.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("marshal authorization entry: %w", err)
	}
	var clone xdr.SorobanAuthorizationEntry
	if err := clone.UnmarshalBinary(encoded); err != nil {
		return entry, fmt.Errorf("clone authorization entry: %w", err)
	}

	credentials = addressCredentials(&clone)
	credentials.SignatureExpirationLedger = xdr.Uint32(validUntilLedger)
	if credentials.Nonce == 0 {
		nonce, err := randomNonce()
		if err != nil {
			return entry, err
		}
		credentials.Nonce = xdr.Int64(nonce)
	}

	var preimage xdr.HashIdPreimage
	if entry.Credentials.Type == xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2 {
		preimage = xdr.HashIdPreimage{
			Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorizationWithAddress,
			SorobanAuthorizationWithAddress: &xdr.HashIdPreimageSorobanAuthorizationWithAddress{
				NetworkId:                 network.ID(passphrase),
				Nonce:                     credentials.Nonce,
				SignatureExpirationLedger: credentials.SignatureExpirationLedger,
				Address:                   credentials.Address,
				Invocation:                clone.RootInvocation,
			},
		}
	} else {
		preimage = xdr.HashIdPreimage{
			Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorization,
			SorobanAuthorization: &xdr.HashIdPreimageSorobanAuthorization{
				NetworkId:                 network.ID(passphrase),
				Nonce:                     credentials.Nonce,
				SignatureExpirationLedger: credentials.SignatureExpirationLedger,
				Invocation:                clone.RootInvocation,
			},
		}
	}
	preimageBytes, err := preimage.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("marshal authorization preimage: %w", err)
	}
	payload := hash.Hash(preimageBytes)

	signature, err := signer.Sign(payload[:])
	if err != nil {
		return entry, fmt.Errorf("sign authorization payload: %w", err)
	}
	if err := signer.Verify(payload[:], signature); err != nil {
		return entry, fmt.Errorf("verify authorization signature: %w", err)
	}

	publicKeyBytes, err := strkey.Decode(strkey.VersionByteAccountID, signer.Address())
	if err != nil {
		return entry, fmt.Errorf("decode signer public key: %w", err)
	}
	publicKey := xdr.ScBytes(publicKeyBytes)
	signatureBytes := xdr.ScBytes(signature)
	publicKeySymbol := xdr.ScSymbol("public_key")
	signatureSymbol := xdr.ScSymbol("signature")

	signatureMap := xdr.ScMap{
		{
			Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &publicKeySymbol},
			Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &publicKey},
		},
		{
			Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &signatureSymbol},
			Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &signatureBytes},
		},
	}
	mapValue, err := xdr.NewScVal(xdr.ScValTypeScvMap, &signatureMap)
	if err != nil {
		return entry, fmt.Errorf("build signature map: %w", err)
	}
	signatureVec := xdr.ScVec{mapValue}
	credentials.Signature, err = xdr.NewScVal(xdr.ScValTypeScvVec, &signatureVec)
	if err != nil {
		return entry, fmt.Errorf("build signature value: %w", err)
	}
	return clone, nil
}

// addressCredentials returns the SorobanAddressCredentials for the supported
// address-credentialled arms (legacy Address and CAP-71 AddressV2), or nil for
// any other credentials type.
func addressCredentials(entry *xdr.SorobanAuthorizationEntry) *xdr.SorobanAddressCredentials {
	switch entry.Credentials.Type {
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddress:
		return entry.Credentials.Address
	case xdr.SorobanCredentialsTypeSorobanCredentialsAddressV2:
		return entry.Credentials.AddressV2
	default:
		return nil
	}
}

func randomNonce() (int64, error) {
	max := new(big.Int).SetInt64(int64(^uint64(0) >> 1))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, fmt.Errorf("generate authorization nonce: %w", err)
	}
	return n.Int64(), nil
}

func validateTransferAuthEntry(
	entry xdr.SorobanAuthorizationEntry,
	contractAddress xdr.ScAddress,
	fromAddress xdr.ScAddress,
	toAddress xdr.ScAddress,
	amount *big.Int,
	payer string,
) error {
	credentials := addressCredentials(&entry)
	if credentials == nil {
		return fmt.Errorf("simulation returned unsupported authorization credentials %s", entry.Credentials.Type.String())
	}
	address, err := credentials.Address.String()
	if err != nil {
		return fmt.Errorf("invalid authorization entry address: %w", err)
	}
	if !strings.EqualFold(address, payer) {
		return fmt.Errorf("simulation requires authorization from %s, not payer %s", address, payer)
	}
	if len(entry.RootInvocation.SubInvocations) != 0 {
		return fmt.Errorf("authorization entry contains unexpected sub-invocations")
	}
	contractFn := entry.RootInvocation.Function.ContractFn
	if entry.RootInvocation.Function.Type != xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn || contractFn == nil {
		return fmt.Errorf("authorization entry does not authorize a contract function")
	}
	if !contractFn.ContractAddress.Equals(contractAddress) {
		return fmt.Errorf("authorization entry contract does not match payment asset")
	}
	if string(contractFn.FunctionName) != stellarTransferMethod {
		return fmt.Errorf("authorization entry function %q is not transfer", contractFn.FunctionName)
	}
	if len(contractFn.Args) != 3 {
		return fmt.Errorf("authorization entry transfer has %d arguments, expected 3", len(contractFn.Args))
	}
	from, err := soroban.DecodeScAddress(contractFn.Args[0])
	if err != nil {
		return fmt.Errorf("authorization transfer from argument: %w", err)
	}
	to, err := soroban.DecodeScAddress(contractFn.Args[1])
	if err != nil {
		return fmt.Errorf("authorization transfer to argument: %w", err)
	}
	expectedFrom, err := fromAddress.String()
	if err != nil {
		return fmt.Errorf("invalid expected payer address: %w", err)
	}
	if !strings.EqualFold(from, expectedFrom) {
		return fmt.Errorf("authorization transfer from %s does not match payer %s", from, expectedFrom)
	}
	expectedTo, err := toAddress.String()
	if err != nil {
		return fmt.Errorf("invalid expected payTo address: %w", err)
	}
	if !strings.EqualFold(to, expectedTo) {
		return fmt.Errorf("authorization transfer recipient %s does not match payTo %s", to, expectedTo)
	}
	argAmount, ok := contractFn.Args[2].GetI128()
	if !ok || !i128Equal(argAmount, amount) {
		return fmt.Errorf("authorization transfer amount does not match the payment requirement")
	}
	return nil
}

func i128Equal(parts xdr.Int128Parts, expected *big.Int) bool {
	actual := new(big.Int).Lsh(big.NewInt(int64(parts.Hi)), 64)
	actual.Add(actual, new(big.Int).SetUint64(uint64(parts.Lo)))
	return actual.Cmp(expected) == 0
}

func rebuildInvokeTransaction(tx *txnbuild.Transaction, op *txnbuild.InvokeHostFunction, baseFee int64, timeoutSeconds int64) (*txnbuild.Transaction, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: tx.SourceAccount().AccountID,
			Sequence:  tx.SequenceNumber(),
		},
		IncrementSequenceNum: false,
		BaseFee:              baseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(timeoutSeconds)},
		Operations:           []txnbuild.Operation{op},
	})
}

// NewStellarClient constructs an HTTP x402 client and the Stellar exact
// payment builder for a CLI or CAIP-2 Stellar network name. The caller
// must close the returned Soroban RPC client.
func NewStellarClient(networkName string, signer *keypair.Full, httpClient HTTPClient) (*Client, *soroban.Client, error) {
	return NewStellarClientWithRPCURL(networkName, signer, httpClient, "")
}

func NewStellarClientWithRPCURL(networkName string, signer *keypair.Full, httpClient HTTPClient, rpcURL string) (*Client, *soroban.Client, error) {
	canonical, err := CanonicalNetwork(networkName)
	if err != nil {
		return nil, nil, err
	}
	internalNetwork, err := InternalNetwork(canonical)
	if err != nil {
		return nil, nil, err
	}
	rpcURL, err = normalizeRPCURL(rpcURL)
	if err != nil {
		return nil, nil, err
	}
	if rpcURL == "" {
		rpcURL = soroban.NetworkRPCURL(internalNetwork)
	}
	rpcClient := soroban.NewClient(rpcURL, soroban.NetworkPassphrase(internalNetwork))
	builder, err := NewStellarExactBuilder(signer, rpcClient)
	if err != nil {
		rpcClient.Close()
		return nil, nil, err
	}
	client, err := NewClient(Config{
		HTTPClient:     httpClient,
		PaymentBuilder: builder,
		Network:        canonical,
	})
	if err != nil {
		rpcClient.Close()
		return nil, nil, err
	}
	return client, rpcClient, nil
}

func normalizeRPCURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("Soroban RPC URL must be a valid HTTP or HTTPS URL without credentials")
	}
	if u.Scheme == "http" && !isLoopbackRPCURL(u.Hostname()) {
		return "", fmt.Errorf("Soroban RPC URL must use HTTPS unless it targets localhost")
	}
	return u.String(), nil
}

func isLoopbackRPCURL(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
