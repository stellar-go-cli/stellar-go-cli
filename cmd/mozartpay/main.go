package main

import (
	"fmt"
	"os"

	"github.com/ogtechnologies/mozartpay/cmd/mozartpay/commands"
	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/ui"
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
