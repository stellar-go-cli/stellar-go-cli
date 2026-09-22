//go:build extras

package commands

import "github.com/stellar-go-cli/stellar-go-cli/internal/config"

// registerExtras registers non-core command groups that are only compiled
// with the "extras" build tag (go build -tags extras).
func (r *RootCmd) registerExtras(cfg *config.Config) {
	r.register(newChatCmd(cfg))
	r.register(newTerminalCmd(cfg))
	r.register(newExchangeCmd(cfg))
}

// addSwapExtras wires extras-only swap subcommands.
func addSwapExtras(cmd *Command, cfg *config.Config) {
	cmd.addSub(newTriangularCmd(cfg))
}
