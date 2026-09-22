package swap_test

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

func ExampleNewService() {
	svc := swap.NewService(models.NetworkStellarTestnet)
	fmt.Println(svc != nil)
	// Output: true
}
