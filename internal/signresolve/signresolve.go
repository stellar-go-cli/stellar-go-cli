// Package signresolve resolves a signer.Signer for a wallet account: a
// custodian RemoteSigner for passkey-protected wallets (key material never
// enters this process), or a local KeypairSigner for plaintext wallets
// (operator trusted surface).
package signresolve

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/signer"
	"github.com/stellar/go-stellar-sdk/keypair"
)

// tool labels the CLI signing channel in custodian records.
const cliTool = "cli"

// ForActive resolves the signer for the active wallet.
func ForActive() (signer.Signer, error) {
	acc, err := wallet.FindStellarAccount()
	if err != nil {
		return nil, err
	}
	return ForAccount(acc)
}

// ForAddress resolves the signer for a specific wallet address.
func ForAddress(address string) (signer.Signer, error) {
	acc, err := wallet.FindStellarAccountForAddress(address)
	if err != nil {
		return nil, err
	}
	return ForAccount(acc)
}

// ForSwap resolves the signer for swap operations (active wallet).
func ForSwap() (signer.Signer, error) {
	acc, err := wallet.FindStellarAccountForSwap()
	if err != nil {
		return nil, err
	}
	return ForAccount(acc)
}

// ForAccount picks the signer for an account record.
func ForAccount(acc *models.Account) (signer.Signer, error) {
	if acc == nil {
		return nil, fmt.Errorf("no wallet account")
	}
	if acc.PasskeyProtected {
		// Protected wallets sign inside the custodian daemon — Dial is lazy,
		// so an unreachable daemon surfaces at sign time with an actionable
		// "start stellar-go-cli signer" error.
		sockPath, err := signsock.DefaultSocketPath()
		if err != nil {
			return nil, err
		}
		return signsock.NewRemoteSigner(signsock.Dial(sockPath), acc.Address, cliTool, ""), nil
	}
	if acc.PrivateKey == "" {
		return nil, fmt.Errorf("wallet %s has no usable key material", acc.Address)
	}
	kp, err := keypair.ParseFull(acc.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("stored seed for %s is invalid: %w", acc.Address, err)
	}
	if kp.Address() != acc.Address {
		return nil, fmt.Errorf("stored seed produces %s, not wallet %s", kp.Address(), acc.Address)
	}
	return signer.KeypairSigner{KP: kp}, nil
}

// Resolver returns a ResolveSigner callback for services — empty address
// resolves the active wallet.
func Resolver() func(address string) (signer.TxSigner, error) {
	return func(address string) (signer.TxSigner, error) {
		if address == "" {
			return ForActive()
		}
		return ForAddress(address)
	}
}

// ForSwapOrNil resolves the swap signer, returning nil on any error —
// callers get a quote-only swap service (the pre-custodian fallback).
func ForSwapOrNil() signer.Signer {
	sg, err := ForSwap()
	if err != nil {
		return nil
	}
	return sg
}
