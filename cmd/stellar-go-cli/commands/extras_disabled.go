//go:build !extras

package commands

import "github.com/stellar-go-cli/stellar-go-cli/internal/config"

// registerExtras is a no-op without the "extras" build tag.
func (r *RootCmd) registerExtras(cfg *config.Config) {}

// addSwapExtras is a no-op without the "extras" build tag.
func addSwapExtras(cmd *Command, cfg *config.Config) {}
