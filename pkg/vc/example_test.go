package vc_test

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/vc"
)

func Example() {
	svc, err := vc.NewService()
	if err != nil {
		panic(err)
	}

	doc, err := svc.CreateDID(models.DIDMethodKey)
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Method == models.DIDMethodKey)
	// Output: true
}
