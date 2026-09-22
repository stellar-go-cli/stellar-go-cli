//go:build extras

package commands

import (
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/terminal"
)

func newTerminalCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "terminal",
		Short: "Launch interactive Bloomberg-style terminal UI with live tickers",
		Long: `Launch an interactive terminal interface with real-time data:
- Live exchange rate tickers with 5-second refresh
- Wallet balance and transaction feed
- Bloomberg-style color scheme (orange headers, green/red price changes)
- Keyboard shortcuts: q=quit, r=reload, tab=switch panel, ?=help`,
		cfg: cfg,
		Run: func(c *Command, args []string) error {
			return terminal.Run(cfg)
		},
	}
}
