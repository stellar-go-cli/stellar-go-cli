//go:build extras

package commands

import (
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

func newExchangeCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "exchange",
		Short: "Exchange functionality (currently unavailable)",
		Long:  "Exchange functionality has been temporarily removed. It will be re-implemented with a lighter-weight solution in a future release.",
		cfg:   cfg,
	}
	cmd.Run = func(c *Command, args []string) error {
		fmt.Println("Exchange functionality is currently unavailable.")
		fmt.Println("The CCXT-based implementation was removed due to build memory constraints.")
		fmt.Println("A new lightweight exchange integration will be available in a future release.")
		return nil
	}
	return cmd
}
