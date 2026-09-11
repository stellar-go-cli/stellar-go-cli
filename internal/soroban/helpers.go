package soroban

import (
	"crypto/rand"
	"fmt"

	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
)

const (
	TestnetRPCURL = "https://soroban-testnet.stellar.org"
	MainnetRPCURL = "https://soroban-mainnet.stellar.org"
)

func NetworkPassphrase(net string) string {
	switch net {
	case "stellar-mainnet":
		return network.PublicNetworkPassphrase
	default:
		return network.TestNetworkPassphrase
	}
}

func NetworkRPCURL(net string) string {
	switch net {
	case "stellar-mainnet":
		return MainnetRPCURL
	default:
		return TestnetRPCURL
	}
}

func ParseContractID(id string) (xdr.ScAddress, error) {
	decoded, err := strkey.Decode(strkey.VersionByteContract, id)
	if err != nil {
		return xdr.ScAddress{}, fmt.Errorf("invalid contract ID %q: %w", id, err)
	}
	if len(decoded) != 32 {
		return xdr.ScAddress{}, fmt.Errorf("contract ID must be 32 bytes, got %d", len(decoded))
	}
	var contractID xdr.ContractId
	copy(contractID[:], decoded)
	return xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractID,
	}, nil
}

func EncodeContractID(addr xdr.ScAddress) (string, error) {
	if addr.Type != xdr.ScAddressTypeScAddressTypeContract || addr.ContractId == nil {
		return "", fmt.Errorf("not a contract address")
	}
	return strkey.Encode(strkey.VersionByteContract, addr.ContractId[:])
}

func AccountToScAddress(address string) (xdr.ScAddress, error) {
	accountID, err := xdr.AddressToAccountId(address)
	if err != nil {
		return xdr.ScAddress{}, fmt.Errorf("invalid account address %q: %w", address, err)
	}
	return xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &accountID,
	}, nil
}

func RandomSalt() ([32]byte, error) {
	var salt [32]byte
	_, err := rand.Read(salt[:])
	return salt, err
}

func ScvString(s string) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvString, xdr.ScString(s))
	return v
}

func ScvSymbol(s string) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvSymbol, xdr.ScSymbol(s))
	return v
}

func ScvBytes(b []byte) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvBytes, xdr.ScBytes(b))
	return v
}

func ScvBool(b bool) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvBool, b)
	return v
}

func ScvU32(n uint32) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvU32, xdr.Uint32(n))
	return v
}

func ScvU64(n uint64) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvU64, xdr.Uint64(n))
	return v
}

func ScvI32(n int32) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvI32, xdr.Int32(n))
	return v
}

func ScvI64(n int64) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvI64, xdr.Int64(n))
	return v
}

func ScvAddress(addr xdr.ScAddress) xdr.ScVal {
	v, _ := xdr.NewScVal(xdr.ScValTypeScvAddress, addr)
	return v
}
