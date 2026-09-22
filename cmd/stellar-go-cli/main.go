package main

import (
	"fmt"
	"os"

	"github.com/stellar-go-cli/stellar-go-cli/cmd/stellar-go-cli/commands"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	root := commands.NewRootCmd(cfg)
	if err := root.Execute(); err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}
}
