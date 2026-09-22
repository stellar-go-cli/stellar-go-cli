package soroban_test

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
)

func ExampleNewClientForNetwork() {
	client := soroban.NewClientForNetwork("stellar-testnet")
	defer client.Close()
	fmt.Println(client.Passphrase())
	// Output: Test SDF Network ; September 2015
}
