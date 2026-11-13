// Command create_claimable is an e2e setup helper. The CLI has no command to
// create claimable balances (only list/accept/decline), so the extended e2e
// suite uses this helper to submit a real createClaimableBalance operation on
// Stellar testnet, giving `claimable list/accept/decline` a real balance to
// act on.
//
// Usage:
//
//	CB_SEED=S... go run ./scripts/e2e_helpers/create_claimable <claimant-address> <amount>
//
// The sponsor is the account that owns CB_SEED; the claimant is listed with an
// unconditional predicate. Prints the submitted transaction hash on stdout.
package main

import (
	"fmt"
	"os"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
)

func main() {
	seed := os.Getenv("CB_SEED")
	if seed == "" || len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: CB_SEED=S... create_claimable <claimant-address> <amount>")
		os.Exit(2)
	}
	claimant, amount := os.Args[1], os.Args[2]

	kp, err := keypair.ParseFull(seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse seed:", err)
		os.Exit(1)
	}

	client := horizonclient.DefaultTestNetClient
	src, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch sponsor account:", err)
		os.Exit(1)
	}

	op := &txnbuild.CreateClaimableBalance{
		Destinations: []txnbuild.Claimant{txnbuild.NewClaimant(claimant, &txnbuild.UnconditionalPredicate)},
		Asset:        txnbuild.NativeAsset{},
		Amount:       amount,
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &src,
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "build tx:", err)
		os.Exit(1)
	}
	tx, err = tx.Sign(network.TestNetworkPassphrase, kp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign tx:", err)
		os.Exit(1)
	}
	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "submit tx:", err)
		os.Exit(1)
	}
	fmt.Println(resp.Hash)
}
