package signer

import (
	"errors"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func testEntry(t *testing.T, kp *keypair.Full) xdr.SorobanAuthorizationEntry {
	t.Helper()
	fromID, err := xdr.AddressToAccountId(kp.Address())
	if err != nil {
		t.Fatalf("payer account id: %v", err)
	}
	from := xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &fromID}
	toID, err := xdr.AddressToAccountId(keypair.MustRandom().Address())
	if err != nil {
		t.Fatalf("recipient account id: %v", err)
	}
	to := xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &toID}
	var contractID xdr.ContractId
	contract := xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &contractID}

	fromVal, err := xdr.NewScVal(xdr.ScValTypeScvAddress, from)
	if err != nil {
		t.Fatalf("from ScVal: %v", err)
	}
	toVal, err := xdr.NewScVal(xdr.ScValTypeScvAddress, to)
	if err != nil {
		t.Fatalf("to ScVal: %v", err)
	}

	return xdr.SorobanAuthorizationEntry{
		Credentials: xdr.SorobanCredentials{
			Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
			Address: &xdr.SorobanAddressCredentials{
				Address:   from,
				Signature: xdr.ScVal{Type: xdr.ScValTypeScvVoid},
			},
		},
		RootInvocation: xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: contract,
					FunctionName:    xdr.ScSymbol("transfer"),
					Args:            xdr.ScVec{fromVal, toVal},
				},
			},
		},
	}
}

func TestKeypairSignerSignsTransaction(t *testing.T) {
	kp := keypair.MustRandom()
	s := KeypairSigner{KP: kp}

	if s.Address() != kp.Address() {
		t.Fatalf("Address() = %s, want %s", s.Address(), kp.Address())
	}

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &txnbuild.SimpleAccount{AccountID: kp.Address(), Sequence: 1},
		IncrementSequenceNum: false,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		Operations:           []txnbuild.Operation{&txnbuild.BumpSequence{BumpTo: 5}},
	})
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}

	signed, err := s.Sign(tx, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sigs := signed.Signatures()
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(sigs))
	}
	hashBytes, err := tx.Hash(network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("tx hash: %v", err)
	}
	if err := kp.Verify(hashBytes[:], sigs[0].Signature); err != nil {
		t.Fatalf("signature does not verify against payer key: %v", err)
	}
}

func TestKeypairSignerSignsAuthEntry(t *testing.T) {
	kp := keypair.MustRandom()
	s := KeypairSigner{KP: kp}
	entry := testEntry(t, kp)

	signed, err := s.SignAuthEntry(entry, network.TestNetworkPassphrase, 123456)
	if err != nil {
		t.Fatalf("SignAuthEntry: %v", err)
	}
	creds := signed.Credentials.Address
	if creds == nil {
		t.Fatal("missing address credentials")
	}
	if creds.SignatureExpirationLedger != 123456 {
		t.Fatalf("expiration ledger = %d, want 123456", creds.SignatureExpirationLedger)
	}
	if creds.Signature.Type != xdr.ScValTypeScvVec {
		t.Fatalf("signature is %s, want ScvVec", creds.Signature.Type)
	}
	// The input entry must not be mutated.
	if entry.Credentials.Address.SignatureExpirationLedger != 0 {
		t.Fatal("SignAuthEntry mutated the input entry")
	}
}

func TestKeypairSignerRejectsWrongSigner(t *testing.T) {
	kp := keypair.MustRandom()
	s := KeypairSigner{KP: keypair.MustRandom()}
	entry := testEntry(t, kp)

	if _, err := s.SignAuthEntry(entry, network.TestNetworkPassphrase, 123456); err == nil {
		t.Fatal("expected signer/address mismatch error")
	}
}

func TestKeypairSignerNilKeypair(t *testing.T) {
	s := KeypairSigner{}
	entry := testEntry(t, keypair.MustRandom())
	if _, err := s.SignAuthEntry(entry, network.TestNetworkPassphrase, 123456); err == nil {
		t.Fatal("expected error for nil keypair")
	}
}

func TestAddressSignerRefusesToSign(t *testing.T) {
	kp := keypair.MustRandom()
	s := AddressSigner{Addr: kp.Address()}

	if s.Address() != kp.Address() {
		t.Fatalf("Address() = %s, want %s", s.Address(), kp.Address())
	}
	if _, err := s.Sign(nil, network.TestNetworkPassphrase); !errors.Is(err, ErrNoKeyMaterial) {
		t.Fatalf("Sign error = %v, want ErrNoKeyMaterial", err)
	}
	entry := testEntry(t, kp)
	if _, err := s.SignAuthEntry(entry, network.TestNetworkPassphrase, 1); !errors.Is(err, ErrNoKeyMaterial) {
		t.Fatalf("SignAuthEntry error = %v, want ErrNoKeyMaterial", err)
	}
}

func TestAddressCredentialsArms(t *testing.T) {
	kp := keypair.MustRandom()
	entry := testEntry(t, kp)
	if AddressCredentials(&entry) == nil {
		t.Fatal("Address arm should return credentials")
	}
	entry.Credentials = xdr.SorobanCredentials{Type: xdr.SorobanCredentialsTypeSorobanCredentialsSourceAccount}
	if AddressCredentials(&entry) != nil {
		t.Fatal("source-account arm should return nil")
	}
}

// Interface conformance — the custodian seam.
var (
	_ TxSigner        = KeypairSigner{}
	_ AuthEntrySigner = KeypairSigner{}
	_ Signer          = KeypairSigner{}
	_ TxSigner        = AddressSigner{}
	_ AuthEntrySigner = AddressSigner{}
)
