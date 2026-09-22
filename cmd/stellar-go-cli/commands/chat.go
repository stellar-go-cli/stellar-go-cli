//go:build extras

package commands

import (
	"flag"
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/internal/assets"
	"github.com/stellar-go-cli/stellar-go-cli/internal/chat"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/scanner"
	"github.com/stellar-go-cli/stellar-go-cli/internal/triangular"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

func newChatCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)

	cmd := &Command{
		Name:  "chat",
		Short: "Start interactive chat interface",
		Long: `Start an interactive chat session for Stellar Go CLI.

The chat interface provides a conversational way to interact with your wallets,
execute swaps, check balances, and manage your assets.

Features:
  • Natural language commands ("swap 100 XLM to USDC")
  • Parameter collection for incomplete commands
  • Contextual prompt suggestions
  • Network switching (testnet/mainnet)
  • Swap quote and execution with confirmation
  • Unified wallet display

Example usage:
  stellar-go-cli chat                    # Start interactive chat
  
Chat commands:
  balance, wallet, swap, send, network, help, quit

The chat will guide you through complex operations step-by-step.`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Chat Interface")

			// Initialize wallet service
			walletSvc := wallet.NewService()

			// Initialize swap service with current network
			var swapSvc *swap.Service
			if cfg.Network == "" {
				cfg.Network = string(models.NetworkStellarTestnet)
			}
			swapSvc = swap.NewService(models.Network(cfg.Network))

			// Initialize asset service
			assetSvc := assets.NewService()

			// Initialize triangular arbitrage service
			triangularSvc := triangular.NewService(models.Network(cfg.Network))

			// Initialize scanner service for arbitrage
			scannerSvc := scanner.NewService(models.Network(cfg.Network), 0.0001, "10")

			// Create and start chat
			chatSession := chat.NewChat(cfg, walletSvc, swapSvc, assetSvc, triangularSvc, scannerSvc)
			if err := chatSession.Start(); err != nil {
				return fmt.Errorf("chat session failed: %w", err)
			}

			return nil
		},
	}

	// Add fine-tuning subcommands
	cmd.addSub(newChatTrainDataCmd(cfg))
	cmd.addSub(newChatFinetuneCmd(cfg))
	cmd.addSub(newChatModelCmd(cfg))

	return cmd
}
