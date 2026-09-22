package iso20022_test

import (
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/iso20022"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func ExampleBuildPacs008() {
	payment := &models.Payment{
		ID:        "pay_001",
		From:      "GA7QJ4ZJ4ZYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJYJ",
		To:        "GBZKH4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4ZK4Z",
		Amount:    "100.0000000",
		Asset:     "USDC",
		Status:    models.PaymentConfirmed,
		Network:   models.NetworkStellarTestnet,
		TxHash:    "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	xmlStr, err := iso20022.BuildPacs008(payment, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(xmlStr) > 0)
	// Output: true
}
