package commands

import (
	"flag"
	"fmt"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/ui"
)

func newNetworkCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "network",
		Short: "Manage Stellar network (testnet/mainnet)",
		Long:  "Switch between Stellar testnet and mainnet networks. Testnet is for development with free faucet funding. Mainnet is for production with real assets.",
		cfg:   cfg,
	}
	cmd.addSub(newNetworkShowCmd(cfg))
	cmd.addSub(newNetworkSetCmd(cfg))
	cmd.addSub(newNetworkSwitchCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── network show ─────────────────────────────

func newNetworkShowCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "show",
		Short: "Show current network configuration",
		Run: func(c *Command, args []string) error {
			ui.Header("Network Configuration")
			fmt.Println()

			ui.SectionLabel("Active Network")
			if cfg.Network == "stellar-mainnet" {
				ui.KVColor("Current", "stellar-mainnet", ui.BrightGreen)
				ui.KV("Type", "Production")
				ui.KV("Horizon URL", "https://horizon.stellar.org")
				ui.KV("Passphrase", "Public Global Stellar Network")
			} else if cfg.Network == "stellar-testnet" {
				ui.KVColor("Current", "stellar-testnet", ui.BrightYellow)
				ui.KV("Type", "Development/Testing")
				ui.KV("Horizon URL", "https://horizon-testnet.stellar.org")
				ui.KV("Passphrase", "Test SDF Network")
			} else {
				ui.KV("Current", cfg.Network)
			}

			fmt.Println()
			ui.SectionLabel("Available Networks")
			ui.KV("  1. stellar-mainnet", "Production network with real assets")
			ui.KV("  2. stellar-testnet", "Development network with free faucet")

			fmt.Println()
			ui.Info("Use 'mozartpay network set <network>' to switch")
			return nil
		},
	}
}

// ─── network set ──────────────────────────────

func newNetworkSetCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	force := fs.Bool("force", false, "Skip confirmation prompt")

	return &Command{
		Name:  "set",
		Short: "Set active network (testnet/mainnet)",
		Long:  "Switch the CLI to use either Stellar testnet or mainnet. This affects all swap, payment, and wallet operations.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if len(args) == 0 || (args[0] != "stellar-mainnet" && args[0] != "stellar-testnet") {
				ui.Error("Usage: mozartpay network set <stellar-mainnet|stellar-testnet>")
				fmt.Println()
				ui.Info("Available networks:")
				ui.KV("  stellar-mainnet", "Production network (real assets)")
				ui.KV("  stellar-testnet", "Development network (free testing)")
				return nil
			}

			network := args[0]
			oldNetwork := cfg.Network

			if oldNetwork == network {
				ui.Info(fmt.Sprintf("Already using %s", network))
				return nil
			}

			ui.Header("Switch Network")
			ui.KV("From", oldNetwork)
			ui.KV("To", network)

			if network == "stellar-mainnet" && !*force {
				fmt.Println()
				ui.Warn("⚠️  You are switching to MAINNET with REAL assets")
				ui.Warn("Transactions will use real XLM/USDC with actual value")
				if !ui.Confirm("Proceed with mainnet?") {
					ui.Info("Cancelled - network unchanged")
					return nil
				}
			}

			// Update config
			cfg.Network = network
			if network == "stellar-mainnet" {
				cfg.Integrations.StellarHorizonURL = "https://horizon.stellar.org"
			} else {
				cfg.Integrations.StellarHorizonURL = "https://horizon-testnet.stellar.org"
			}

			if err := config.Save(cfg); err != nil {
				ui.Error("Failed to save config: " + err.Error())
				return err
			}

			fmt.Println()
			ui.Success(fmt.Sprintf("Network switched to %s", network))
			ui.KV("Horizon", cfg.Integrations.StellarHorizonURL)

			if network == "stellar-mainnet" {
				fmt.Println()
				ui.Info("Mainnet active - transactions will use real assets")
			} else {
				fmt.Println()
				ui.Info("Testnet active - use 'mozartpay wallet fund' for free XLM")
			}

			return nil
		},
	}
}

// ─── network switch ───────────────────────────

func newNetworkSwitchCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("switch", flag.ContinueOnError)
	return &Command{
		Name:  "switch",
		Short: "Interactive network switch",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Switch Network")
			fmt.Println()

			ui.SectionLabel("Select Network")
			current := cfg.Network
			if current == "stellar-mainnet" {
				fmt.Println("▸ [1] stellar-mainnet [CURRENT]")
				fmt.Println("  [2] stellar-testnet")
			} else {
				fmt.Println("  [1] stellar-mainnet")
				fmt.Println("▸ [2] stellar-testnet [CURRENT]")
			}

			fmt.Println()
			fmt.Print("Choose [1-2]: ")
			var choice string
			fmt.Scanln(&choice)

			var newNetwork string
			switch choice {
			case "1":
				newNetwork = "stellar-mainnet"
			case "2":
				newNetwork = "stellar-testnet"
			default:
				ui.Error("Invalid choice")
				return nil
			}

			if newNetwork == current {
				ui.Info("Already on " + current)
				return nil
			}

			// Warn if switching to mainnet
			if newNetwork == "stellar-mainnet" {
				fmt.Println()
				ui.Warn("⚠️  You are switching to MAINNET with REAL assets")
				fmt.Print("Proceed? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					ui.Info("Cancelled")
					return nil
				}
			}

			// Update config
			cfg.Network = newNetwork
			if newNetwork == "stellar-mainnet" {
				cfg.Integrations.StellarHorizonURL = "https://horizon.stellar.org"
			} else {
				cfg.Integrations.StellarHorizonURL = "https://horizon-testnet.stellar.org"
			}

			if err := config.Save(cfg); err != nil {
				ui.Error("Failed to save: " + err.Error())
				return err
			}

			ui.Success("Network switched to " + newNetwork)
			return nil
		},
	}
}
