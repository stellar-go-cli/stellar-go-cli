package commands

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"golang.org/x/term"
)

func newWalletCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "wallet",
		Short: "Manage wallets and account connections",
		Long:  "Create or import a Stellar keypair, or register an external EOA.",
		cfg:   cfg,
	}
	cmd.addSub(newWalletConnectCmd(cfg))
	cmd.addSub(newWalletImportCmd(cfg))
	cmd.addSub(newWalletFundCmd(cfg))
	cmd.addSub(newWalletBalanceCmd(cfg))
	cmd.addSub(newWalletAssetsCmd(cfg))
	cmd.addSub(newWalletShowCmd(cfg))
	cmd.addSub(newWalletListCmd(cfg))
	cmd.addSub(newWalletSwitchCmd(cfg))
	cmd.addSub(newWalletRenameCmd(cfg))
	cmd.addSub(newWalletRemoveCmd(cfg))
	cmd.addSub(newWalletExportCmd(cfg))
	cmd.addSub(newWalletPasskeyCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── wallet connect ───────────────────────────

func newWalletConnectCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	provider := fs.String("provider", "", "Wallet provider: stellar | external (interactive if omitted)")
	network := fs.String("network", "stellar-testnet", "Network: stellar-testnet | stellar-mainnet | evm-sepolia | evm-mainnet")
	address := fs.String("address", "", "External address (for --provider external)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "connect",
		Short: "Connect a wallet (Stellar or external EOA)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Connect Wallet")

			net := models.Network(*network)
			svc := wallet.NewService()

			var acc *models.Account
			var err error

			// Interactive provider selection if not specified
			selectedProvider := *provider
			if selectedProvider == "" {
				ui.SectionLabel("Select Wallet Type")
				fmt.Println("1) Stellar (Native Stellar keypair)")
				fmt.Println("2) External EOA")
				fmt.Print("Choose option [1-2]: ")

				var choice string
				fmt.Scanln(&choice) //nolint:errcheck // empty input falls through to the default
				switch choice {
				case "1":
					selectedProvider = "stellar"
				case "2":
					selectedProvider = "external"
				default:
					ui.Error("Invalid choice. Please select 1 or 2.")
					return nil
				}
			}

			switch selectedProvider {
			case "stellar":
				spin := ui.NewSpinner("Creating native Stellar keypair...")
				spin.Start()
				acc, err = svc.ConnectStellarWallet(net)
				if err != nil {
					spin.Stop(false, "Failed to create Stellar wallet")
					return err
				}
				spin.Stop(true, "Stellar wallet created")

			case "external":
				spin := ui.NewSpinner("Registering external EOA...")
				spin.Start()
				acc, err = svc.ConnectExternalEOA(*address, net)
				if err != nil {
					spin.Stop(false, "Failed")
					return err
				}
				spin.Stop(true, "External EOA registered")

			default:
				return fmt.Errorf("unknown provider: %s", selectedProvider)
			}

			// Link active DID if available (from config or state)
			linkedDID := cfg.ActiveDID
			if linkedDID == "" {
				for _, m := range []string{"ebsi", "key", "web", "ethr"} {
					var doc map[string]interface{}
					if err := config.LoadState("did_"+m, &doc); err == nil {
						if id, ok := doc["id"].(string); ok {
							linkedDID = id
							break
						}
					}
				}
			}
			if linkedDID != "" {
				acc = svc.LinkDID(acc, linkedDID)
				ui.Success("DID linked: " + truncateStr(linkedDID, 40) + "...")
			}

			if *output == "json" {
				fmt.Println(prettyJSON(acc))
				return nil
			}

			ui.SectionLabel("Account Details")
			ui.KV("Address", acc.Address)
			ui.KV("Network", wallet.NetworkDisplayName(net))
			ui.KV("Type", string(acc.Type))
			ui.KV("Balance", acc.Balance)
			ui.KV("Funded", fmt.Sprintf("%v", acc.Funded))
			if acc.DID != "" {
				ui.KV("DID", safeTruncW(acc.DID, 40)+"...")
			}
			// Migrate from legacy if needed
			svc.MigrateFromLegacy() //nolint:errcheck // best-effort migration

			// Check if wallet already exists
			existingWallets, activeWallet, _ := svc.ListWallets() //nolint:errcheck // partial results acceptable for existence checks
			walletExists := false
			for _, w := range existingWallets {
				if w.Address == acc.Address {
					walletExists = true
					break
				}
			}

			// If this is a new wallet (not just a reconnect), ask if user wants to make it active
			makeActive := true
			if !walletExists && len(existingWallets) > 0 {
				fmt.Println()
				fmt.Printf("You have %d other wallet(s). Make this your active wallet? [Y/n]: ", len(existingWallets))
				var response string
				fmt.Scanln(&response) //nolint:errcheck // empty input falls through to the default
				if response != "" && response != "Y" && response != "y" {
					makeActive = false
				}
			}

			// Save to new registry system
			if err := svc.AddWalletToRegistry(acc, makeActive); err != nil {
				return fmt.Errorf("failed to save wallet to registry: %w", err)
			}

			if makeActive {
				cfg.ActiveAddress = acc.Address
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				ui.Success("Wallet created and set as active")
			} else {
				ui.Success("Wallet created (not active)")
				ui.Info(fmt.Sprintf("Active wallet remains: %s", safeTruncW(activeWallet, 30)))
				ui.Info("Use 'stellar-go-cli wallet switch' to activate this wallet")
			}
			ui.KV("Address", acc.Address)

			if !acc.Funded {
				fmt.Println()
				ui.Warn("Account not funded. Run: stellar-go-cli wallet fund")
				ui.Info("Faucet: " + wallet.FaucetURL(net, acc.Address))
				if net == models.NetworkStellarTestnet {
					ui.Info("Explorer: https://stellar.expert/explorer/testnet/account/" + acc.Address)
					if acc.Type == models.WalletStellar {
						ui.Info("Stellar Lab: https://lab.stellar.org/account/create")
					}
				}
			}

			return nil
		},
	}
}

// ─── wallet import ───────────────────────────

func newWalletImportCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	priv := fs.String("private-key", "", "Stellar secret seed (S...). Prefer --private-key-file or hidden prompt to avoid shell history.")
	keyFile := fs.String("private-key-file", "", "File containing the secret key (whitespace trimmed)")
	network := fs.String("network", "stellar-mainnet", "Network: stellar-testnet | stellar-mainnet")
	out := fs.String("output", "pretty", "Output format: pretty | json")
	noActive := fs.Bool("no-active", false, "Save wallet but do not set it as active")

	return &Command{
		Name:  "import",
		Short: "Import a Stellar wallet from secret key",
		Long:  "Adds an existing Stellar account to the local wallet store using an S... strkey. The key file is written with mode 0600; treat it like a password.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Import Stellar wallet")
			ui.Warn("Anyone with this secret key can spend funds from the account.")

			secret, err := readStellarImportSecret(*priv, *keyFile)
			if err != nil {
				return fmt.Errorf("read secret key: %w", err)
			}

			net := models.Network(*network)
			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				return fmt.Errorf("import only supports stellar-testnet and stellar-mainnet")
			}

			svc := wallet.NewService()
			_ = svc.MigrateFromLegacy() //nolint:errcheck // best-effort migration

			spin := ui.NewSpinner("Validating key and fetching account from Horizon...")
			spin.Start()
			acc, err := svc.ImportStellarFromSecret(secret, net)
			spin.Stop(err == nil, "Done")
			if err != nil {
				ui.Error(err.Error())
				return nil
			}

			if old, err := svc.GetWalletByAddress(acc.Address); err == nil {
				acc.CreatedAt = old.CreatedAt
			}

			existingWallets, activeWallet, _ := svc.ListWallets() //nolint:errcheck // partial results acceptable for existence checks
			walletExists := false
			for _, w := range existingWallets {
				if w.Address == acc.Address {
					walletExists = true
					break
				}
			}

			makeActive := !*noActive
			if makeActive && !walletExists && len(existingWallets) > 0 {
				fmt.Println()
				fmt.Printf("You have %d other wallet(s). Make this your active wallet? [Y/n]: ", len(existingWallets))
				var response string
				fmt.Scanln(&response) //nolint:errcheck // empty input falls through to the default
				if response != "" && response != "Y" && response != "y" {
					makeActive = false
				}
			}

			if err := svc.AddWalletToRegistry(acc, makeActive); err != nil {
				return fmt.Errorf("save wallet: %w", err)
			}
			if makeActive {
				cfg.ActiveAddress = acc.Address
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}

			if *out == "json" {
				redacted := *acc
				redacted.PrivateKey = ""
				fmt.Println(prettyJSON(&redacted))
				return nil
			}

			ui.SectionLabel("Imported account")
			ui.KV("Address", acc.Address)
			ui.KV("Network", wallet.NetworkDisplayName(net))
			ui.KVColor("Balance", acc.Balance, ui.BrightGreen)
			ui.KV("Funded", fmt.Sprintf("%v", acc.Funded))
			if makeActive {
				ui.Success("Wallet imported and set as active")
			} else {
				ui.Success("Wallet imported")
				if activeWallet != "" {
					ui.Info(fmt.Sprintf("Active wallet remains: %s", safeTruncW(activeWallet, 30)))
				}
				ui.Info("Use 'stellar-go-cli wallet switch' to activate this wallet")
			}
			if !acc.Funded && net == models.NetworkStellarTestnet {
				ui.Info("Fund testnet: stellar-go-cli wallet fund")
			}
			return nil
		},
	}
}

func readStellarImportSecret(fromFlag, fromFile string) (string, error) {
	if s := strings.TrimSpace(fromFlag); s != "" {
		return s, nil
	}
	if fromFile != "" {
		b, err := os.ReadFile(fromFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Stellar secret key (S... ; input hidden): ")
		secret, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(secret)), nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// ─── wallet fund ─────────────────────────────

func newWalletFundCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("fund", flag.ContinueOnError)
	fs.Bool("eoa", false, "Select External Owner Account mode")
	network := fs.String("network", "stellar-testnet", "Testnet to fund: stellar-testnet | evm-sepolia")

	return &Command{
		Name:  "fund",
		Short: "Fund testnet account via faucet",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Fund Testnet Account")

			svc := wallet.NewService()

			// Try to get active wallet from registry first
			acc, err := svc.GetActiveWallet()
			if err != nil {
				// Fall back to legacy state
				var legacyAcc models.Account
				if err := config.LoadState("account", &legacyAcc); err != nil {
					ui.Error("No active wallet found. Run 'stellar-go-cli wallet connect' first.")
					return nil
				}
				acc = &legacyAcc
			}

			// Override network if specified
			if *network != "" {
				acc.Network = models.Network(*network)
			}

			spin := ui.NewSpinner(fmt.Sprintf("Requesting funds from %s faucet...", acc.Network))
			spin.Start()

			funded, err := svc.FundTestnetAccount(acc)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Account funded successfully")

			ui.SectionLabel("Funded Account")
			ui.KV("Address", funded.Address)
			ui.KV("Network", wallet.NetworkDisplayName(funded.Network))
			ui.KVColor("Balance", funded.Balance, ui.BrightGreen)
			ui.KV("Faucet URL", wallet.FaucetURL(funded.Network, funded.Address))
			if funded.Network == models.NetworkStellarTestnet {
				ui.KV("Explorer", "https://stellar.expert/explorer/testnet/account/"+funded.Address)
			}

			config.SaveState("account", funded) //nolint:errcheck // best-effort state cache
			cfg.ActiveAddress = funded.Address
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			return nil
		},
	}
}

// ─── wallet balance ───────────────────────────

func newWalletBalanceCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("balance", flag.ContinueOnError)
	fs.String("address", "", "Address to query (uses saved account if omitted)")

	return &Command{
		Name:  "balance",
		Short: "Query account balance",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Account Balance")

			svc := wallet.NewService()

			// Get current network from config
			currentNetwork := models.Network(cfg.Network)
			if currentNetwork == "" {
				currentNetwork = models.NetworkStellarTestnet
			}

			var acc *models.Account
			var err error

			// Try to get active wallet first
			acc, err = svc.GetActiveWallet()
			if err != nil {
				// Fall back to loading from state
				var stateAcc models.Account
				if loadErr := config.LoadState("account", &stateAcc); loadErr != nil {
					ui.Warn("No account found. Run 'stellar-go-cli wallet connect' first.")
					return nil
				}
				acc = &stateAcc
			}

			// Check if wallet network matches current config
			if acc.Network != currentNetwork {
				ui.Warn(fmt.Sprintf("Active wallet is on %s but current network is %s", acc.Network, currentNetwork))
				ui.Info("Searching for wallet on current network...")

				// Try to find a wallet on the current network
				wallets, _, listErr := svc.ListWallets()
				if listErr == nil {
					for _, w := range wallets {
						if w.Network == currentNetwork {
							// Found a wallet on the correct network, use it
							if updatedAcc, getErr := svc.GetWalletByAddress(w.Address); getErr == nil {
								acc = updatedAcc
								ui.Success(fmt.Sprintf("Using wallet on %s: %s", currentNetwork, w.Address[:12]))
								break
							}
						}
					}
				}

				// If still mismatched, show warning
				if acc.Network != currentNetwork {
					ui.Warn(fmt.Sprintf("No wallet found on %s. Use 'stellar-go-cli wallet switch' to select a wallet on this network.", currentNetwork))
				}
			}

			// Refresh balance from network
			spin := ui.NewSpinner("Fetching balance from network...")
			spin.Start()
			updatedAcc, err := svc.UpdateWalletBalance(acc.Address)
			if err == nil && updatedAcc != nil {
				acc = updatedAcc
			}
			spin.Stop(err == nil, "Balance fetched")

			ui.SectionLabel("Balance")
			ui.KV("Address", acc.Address)
			ui.KV("Network", wallet.NetworkDisplayName(acc.Network))
			ui.KVColor("Balance", acc.Balance, ui.BrightGreen)
			ui.KV("Funded", fmt.Sprintf("%v", acc.Funded))

			return nil
		},
	}
}

// ─── wallet assets ───────────────────────────

func newWalletAssetsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("assets", flag.ContinueOnError)
	addr := fs.String("address", "", "Stellar account address (uses active wallet if omitted)")
	netStr := fs.String("network", "", "stellar-testnet | stellar-mainnet (inferred if address is in your registry)")
	out := fs.String("output", "pretty", "Output: pretty | json")

	return &Command{
		Name:  "assets",
		Short: "List balances and trustlines for a Stellar address",
		Long:  "Fetches Horizon account balances: native XLM plus all credited assets the account trusts (trustlines).",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			svc := wallet.NewService()
			_ = svc.MigrateFromLegacy() //nolint:errcheck // best-effort migration

			var target string
			if *addr != "" {
				target = *addr
			} else {
				acc, err := svc.GetActiveWallet()
				if err != nil {
					ui.Error("No address provided and no active wallet. Use --address or 'stellar-go-cli wallet switch'.")
					return nil
				}
				target = acc.Address
			}

			var net models.Network
			if *netStr != "" {
				net = models.Network(*netStr)
			} else if reg, err := svc.GetWalletByAddress(target); err == nil {
				net = reg.Network
			} else {
				ui.Error("Unknown wallet in registry. Pass --network stellar-testnet or stellar-mainnet.")
				return nil
			}

			if net != models.NetworkStellarTestnet && net != models.NetworkStellarMainnet {
				ui.Error("wallet assets only supports stellar-testnet and stellar-mainnet.")
				return nil
			}

			spin := ui.NewSpinner("Fetching account assets from Horizon...")
			spin.Start()
			assets, err := svc.GetAccountAssets(target, net)
			spin.Stop(err == nil, "Done")
			if err != nil {
				ui.Error("Failed to fetch assets: " + err.Error())
				return nil
			}

			if *out == "json" {
				fmt.Println(prettyJSON(assets))
				return nil
			}

			ui.Header("Trusted assets & balances")
			ui.KV("Address", target)
			ui.KV("Network", wallet.NetworkDisplayName(net))

			if len(assets) == 0 {
				ui.Warn("No account on this network (unfunded or missing), or Horizon returned no balances.")
				return nil
			}

			ui.SectionLabel(fmt.Sprintf("%d balance line(s)", len(assets)))
			for _, a := range assets {
				if a.Code == "XLM" {
					ui.KV("  XLM (native)", a.Balance)
					continue
				}
				ui.KV(fmt.Sprintf("  %s", a.Code), a.Balance)
				ui.KV("    issuer", a.Issuer)
			}

			return nil
		},
	}
}

// ─── wallet show ─────────────────────────────

func newWalletShowCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "show",
		Short: "Show connected wallet details",
		Run: func(c *Command, args []string) error {
			ui.Header("Wallet Info")

			svc := wallet.NewService()

			// Migrate from legacy if needed
			if err := svc.MigrateFromLegacy(); err != nil {
				ui.Warn("Failed to migrate legacy wallet: " + err.Error())
			}

			// Get current network from config
			currentNetwork := models.Network(cfg.Network)
			if currentNetwork == "" {
				currentNetwork = models.NetworkStellarTestnet
			}

			// Get active wallet
			acc, err := svc.GetActiveWallet()
			if err != nil {
				ui.Warn("No active wallet found. Run 'stellar-go-cli wallet connect' or 'stellar-go-cli wallet switch'.")
				return nil
			}

			// Check if active wallet network matches current config
			if acc.Network != currentNetwork {
				ui.Warn(fmt.Sprintf("Active wallet is on %s but current network is %s", acc.Network, currentNetwork))
				ui.Info("Searching for wallet on current network...")

				// Try to find a wallet on the current network
				wallets, _, err := svc.ListWallets()
				if err == nil {
					for _, w := range wallets {
						if w.Network == currentNetwork {
							// Found a wallet on the correct network, switch to it
							if err := svc.SetActiveWallet(w.Address); err == nil {
								acc, _ = svc.GetActiveWallet() //nolint:errcheck // refreshed best-effort; only used for display
								ui.Success(fmt.Sprintf("Switched to wallet on %s: %s", currentNetwork, w.Address[:12]))
								break
							}
						}
					}
				}

				// If still mismatched, show warning
				if acc.Network != currentNetwork {
					ui.Warn(fmt.Sprintf("No wallet found on %s. Use 'stellar-go-cli wallet switch' to select a wallet on this network.", currentNetwork))
					ui.Info("Showing active wallet (network mismatch):")
				}
			}

			// Refresh balance from network before displaying
			spin := ui.NewSpinner("Fetching live balance from network...")
			spin.Start()
			updatedAcc, err := svc.UpdateWalletBalance(acc.Address)
			if err == nil && updatedAcc != nil {
				acc = updatedAcc
			}
			spin.Stop(err == nil, "Balance updated")

			ui.SectionLabel("Connected Account")
			ui.KV("Address", acc.Address)
			ui.KV("Network", wallet.WalletNetworkDisplayName(acc.Network, acc.Type))
			if acc.Network == models.NetworkStellarTestnet || acc.Network == models.NetworkStellarMainnet {
				ui.KV("URL", stellarExplorerURL(acc.Address, acc.Network))
			}
			ui.KV("Type", string(acc.Type))
			ui.KVColor("Balance", acc.Balance, ui.BrightGreen)
			ui.KV("Funded", fmt.Sprintf("%v", acc.Funded))
			if acc.DID != "" {
				ui.KV("Linked DID", acc.DID)
			}
			ui.KV("Created", acc.CreatedAt.Format(time.RFC3339))

			// Display assets/trustlines - filter for important assets only
			if acc.Funded && (acc.Network == models.NetworkStellarTestnet || acc.Network == models.NetworkStellarMainnet) {
				assets, _ := svc.GetAccountAssets(acc.Address, acc.Network) //nolint:errcheck // empty list on error
				if len(assets) > 0 {
					fmt.Println()
					ui.SectionLabel("Assets (Trustlines)")
					// Important assets to display (whitelist only)
					importantAssets := map[string]bool{
						"XLM": true, "USDC": true, "USDT": true, "yXLM": true,
						"AQUA": true, "SHX": true, "XRP": true, "BTC": true, "ETH": true,
						"EURT": true, "GBP": true, "NGNC": true, "XRF": true,
					}
					for _, asset := range assets {
						// Only show whitelisted important assets
						if importantAssets[asset.Code] {
							ui.KV(fmt.Sprintf("  %s", asset.Code), asset.Balance)
						}
					}
				}
			}

			return nil
		},
	}
}

// ─── wallet switch ───────────────────────────

func newWalletSwitchCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("switch", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to switch to (interactive if omitted)")

	return &Command{
		Name:  "switch",
		Short: "Switch between stored wallets",
		Long:  "Select which wallet to use for transactions from your stored wallets.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Switch Wallet")

			svc := wallet.NewService()

			// Migrate from legacy if needed
			if err := svc.MigrateFromLegacy(); err != nil {
				ui.Warn("Failed to migrate legacy wallet: " + err.Error())
			}

			wallets, active, err := svc.ListWallets()
			if err != nil {
				ui.Error("Failed to load wallets: " + err.Error())
				return err
			}

			if len(wallets) == 0 {
				ui.Info("No wallets found. Create one with: stellar-go-cli wallet connect")
				return nil
			}

			// If address provided via flag, use it
			selectedAddress := *address
			if selectedAddress == "" {
				// Interactive selection
				ui.SectionLabel("Select Wallet")
				for i, w := range wallets {
					prefix := "  "
					if w.Address == active {
						prefix = "▸ "
					}

					displayName := w.Name
					if displayName == "" {
						displayName = "Unnamed"
					}

					fmt.Printf("%s[%d] %s (%s)", prefix, i+1, displayName, w.Address)
					if w.Address == active {
						fmt.Print(" [ACTIVE]")
					}
					fmt.Println()
				}

				fmt.Print("\nSelect wallet [1-" + fmt.Sprintf("%d", len(wallets)) + "]: ")
				var choice string
				fmt.Scanln(&choice) //nolint:errcheck // empty input falls through to the default

				// Parse selection
				var index int
				if _, err := fmt.Sscanf(choice, "%d", &index); err != nil || index < 1 || index > len(wallets) {
					ui.Error("Invalid selection")
					return fmt.Errorf("invalid wallet selection")
				}

				selectedAddress = wallets[index-1].Address
			}

			// Switch to selected wallet
			if err := svc.SetActiveWallet(selectedAddress); err != nil {
				ui.Error("Failed to switch wallet: " + err.Error())
				return err
			}

			// Update config
			acc, err := svc.GetActiveWallet()
			if err != nil {
				ui.Error("Failed to load active wallet: " + err.Error())
				return err
			}

			cfg.ActiveAddress = acc.Address
			if err := config.Save(cfg); err != nil {
				ui.Warn("Failed to save config: " + err.Error())
			}

			ui.Success("Switched to wallet")
			ui.KV("Address", acc.Address)
			ui.KV("Type", string(acc.Type))
			ui.KV("Network", wallet.NetworkDisplayName(acc.Network))

			return nil
		},
	}
}

// ─── wallet export ───────────────────────────

func newWalletExportCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to export (uses active wallet if omitted)")
	showPrivate := fs.Bool("private", false, "Show private key (use with caution)")

	return &Command{
		Name:  "export",
		Short: "Export wallet keys for backup",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Export Wallet Keys")

			svc := wallet.NewService()

			// Migrate from legacy if needed
			svc.MigrateFromLegacy() //nolint:errcheck // best-effort migration

			var acc *models.Account
			var err error

			if *address != "" {
				// Load specific wallet by address
				acc, err = svc.GetWalletByAddress(*address)
				if err != nil {
					ui.Error("Wallet not found: " + err.Error())
					return nil
				}
			} else {
				// Use active wallet
				acc, err = svc.GetActiveWallet()
				if err != nil {
					ui.Error("No active wallet. Use --address or switch to a wallet first.")
					return nil
				}
			}

			ui.SectionLabel("Wallet Export")
			ui.KV("Type", string(acc.Type))
			ui.KV("Address", acc.Address)
			ui.KV("Network", wallet.NetworkDisplayName(acc.Network))
			ui.KV("Public Key", acc.PublicKey)

			// Show private key if requested and available
			if *showPrivate {
				seed, serr := wallet.ResolveSeed(acc)
				if serr != nil {
					ui.Warn("Private key not available: " + serr.Error())
				} else {
					ui.SectionLabel("⚠️  PRIVATE KEY - KEEP SECURE")
					ui.KV("Private Key", seed)
					ui.Warn("Store this key securely and never share it!")
				}
			} else {
				ui.Info("Use --private flag to show private key")
			}

			return nil
		},
	}
}

// ─── wallet list ────────────────────────────

func newWalletListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	return &Command{
		Name:  "list",
		Short: "List all stored wallets",
		Long:  "Show all wallets in your wallet registry with their details and active status.",
		Flags: fs,
		cfg:   cfg,
		Run: func(c *Command, args []string) error {
			ui.Header("Stored Wallets")

			svc := wallet.NewService()

			// Migrate from legacy if needed
			if err := svc.MigrateFromLegacy(); err != nil {
				ui.Warn("Failed to migrate legacy wallet: " + err.Error())
			}

			wallets, active, err := svc.ListWallets()
			if err != nil {
				ui.Error("Failed to load wallets: " + err.Error())
				return err
			}

			if len(wallets) == 0 {
				ui.Info("No wallets found. Create one with: stellar-go-cli wallet connect")
				return nil
			}

			ui.SectionLabel(fmt.Sprintf("Found %d wallet(s)", len(wallets)))

			// Refresh balances from network and fetch assets
			for i := range wallets {
				updated, err := svc.UpdateWalletBalance(wallets[i].Address)
				if err == nil && updated != nil {
					wallets[i].Balance = updated.Balance
					wallets[i].Funded = updated.Funded
				}
			}

			for i, w := range wallets {
				prefix := "  "
				if w.Address == active {
					prefix = "▸ "
				}

				displayName := w.Name
				if displayName == "" {
					displayName = "Unnamed"
				}

				ui.KV(fmt.Sprintf("%s[%d] %s", prefix, i+1, displayName), "")
				ui.KV("    Address", w.Address)
				ui.KV("    Type", string(w.Type))
				ui.KV("    Network", string(w.Network))
				ui.KV("    Balance", w.Balance)
				if w.Funded {
					ui.KVColor("    Status", "Funded", ui.BrightGreen)
					// Fetch and display assets
					assets, err := svc.GetAccountAssets(w.Address, w.Network)
					if err == nil && len(assets) > 0 {
						ui.SectionLabel("    Assets (Trustlines)")
						for _, asset := range assets {
							if asset.Code == "XLM" {
								ui.KV("      • XLM", asset.Balance)
							} else {
								issuerShort := safeTruncW(asset.Issuer, 10) + "..."
								ui.KV(fmt.Sprintf("      • %s (%s)", asset.Code, issuerShort), asset.Balance)
							}
						}
					}
				} else {
					ui.KVColor("    Status", "Unfunded", ui.BrightYellow)
				}
				fmt.Println()
			}

			if active != "" {
				ui.Success(fmt.Sprintf("Active wallet: %s", safeTruncW(active, 30)))
			}

			ui.Info("Use 'stellar-go-cli wallet switch' to change active wallet")
			ui.Info("Use 'stellar-go-cli wallet rename <address> <name>' to assign names")

			return nil
		},
	}
}

// ─── wallet rename ──────────────────────────

func newWalletRenameCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	return &Command{
		Name:  "rename",
		Short: "Assign a friendly name to a wallet",
		Long:  "Give a wallet a memorable name like 'personal', 'business', or 'savings'.",
		Flags: fs,
		cfg:   cfg,
		Run: func(c *Command, args []string) error {
			if len(args) < 2 {
				ui.Error("Usage: stellar-go-cli wallet rename <address> <name>")
				return fmt.Errorf("missing arguments")
			}

			address := args[0]
			name := args[1]

			ui.Header("Rename Wallet")

			svc := wallet.NewService()

			if err := svc.RenameWallet(address, name); err != nil {
				ui.Error("Failed to rename wallet: " + err.Error())
				return err
			}

			ui.Success(fmt.Sprintf("Wallet renamed to '%s'", name))
			ui.KV("Address", address)

			return nil
		},
	}
}

// ─── wallet remove ───────────────────────────

func newWalletRemoveCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to remove (interactive if omitted)")
	confirm := fs.Bool("confirm", false, "Skip confirmation prompt")

	return &Command{
		Name:  "remove",
		Short: "Remove a wallet from storage",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Remove Wallet")

			svc := wallet.NewService()
			targetAddress := *address

			// Get address if not provided
			if targetAddress == "" {
				// List wallets for selection
				wallets, _, err := svc.ListWallets()
				if err != nil {
					return fmt.Errorf("failed to list wallets: %w", err)
				}

				if len(wallets) == 0 {
					ui.Info("No wallets found")
					return nil
				}

				fmt.Println("\nAvailable wallets:")
				for i, w := range wallets {
					acc, err := svc.GetWalletByAddress(w.Address)
					if err != nil {
						continue
					}
					status := "Unfunded"
					if acc.Funded {
						status = "Funded"
					}
					fmt.Printf("  [%d] %s (%s) - %s\n", i+1, w.Address[:10]+"...", acc.Type, status)
				}

				fmt.Print("\nSelect wallet to remove [1]: ")
				var selection int
				fmt.Scanln(&selection) //nolint:errcheck // empty input falls through to the default
				if selection < 1 || selection > len(wallets) {
					selection = 1
				}

				targetAddress = wallets[selection-1].Address
			}

			// Get wallet details
			acc, err := svc.GetWalletByAddress(targetAddress)
			if err != nil {
				return fmt.Errorf("wallet not found: %w", err)
			}

			// Show wallet info
			ui.SectionLabel("Wallet to Remove")
			ui.KV("Address", targetAddress)
			ui.KV("Type", string(acc.Type))
			ui.KV("Network", string(acc.Network))
			ui.KV("Balance", acc.Balance)
			ui.KV("Status", func() string {
				if acc.Funded {
					return "Funded"
				}
				return "Unfunded"
			}())

			// Warning if funded
			if acc.Funded && acc.Balance != "0" {
				ui.Warn("⚠️  This wallet has a non-zero balance!")
				ui.Warn("   Make sure you've backed up any funds before removing.")
			}

			// Confirmation
			if !*confirm {
				fmt.Println()
				ui.Warn("⚠️  This will permanently remove the wallet from local storage.")
				ui.Warn("   This action cannot be undone.")
				fmt.Print("Type 'remove' to confirm: ")
				var confirmation string
				fmt.Scanln(&confirmation) //nolint:errcheck // empty input falls through to the default

				if confirmation != "remove" {
					ui.Info("Wallet removal cancelled")
					return nil
				}
			}

			// Check if it's the active wallet
			active, err := svc.GetActiveWallet()
			if err == nil && active.Address == targetAddress {
				ui.Warn("This is the active wallet. It will be deactivated.")
			}

			spin := ui.NewSpinner("Removing wallet...")
			spin.Start()

			err = svc.RemoveWallet(targetAddress)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			spin.Stop(true, "Wallet removed successfully")

			ui.Success("Wallet removed from storage")
			ui.KV("Removed Address", targetAddress[:30]+"...")

			return nil
		},
	}
}

// ─── wallet passkey ───────────────────────────

func newWalletPasskeyCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "passkey",
		Short: "Passkey (WebAuthn) protection for wallet keys",
		Long: "Protect a wallet's seed behind a real passkey: the seed is encrypted at rest " +
			"and a WebAuthn assertion is required to unlock it for signing.",
	}
	cmd.addSub(newWalletPasskeyRegisterCmd(cfg))
	cmd.addSub(newWalletPasskeyUnlockCmd(cfg))
	cmd.addSub(newWalletPasskeyLockCmd(cfg))
	cmd.addSub(newWalletPasskeyStatusCmd(cfg))
	cmd.addSub(newWalletPasskeyListCmd(cfg))
	cmd.addSub(newWalletPasskeyDisableCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// resolvePasskeyTarget resolves a wallet address from a flag or the active wallet.
func resolvePasskeyTarget(svc *wallet.Service, address string) (string, error) {
	if address != "" {
		return address, nil
	}
	acc, err := svc.GetActiveWallet()
	if err != nil {
		return "", fmt.Errorf("no address given and no active wallet: %w", err)
	}
	return acc.Address, nil
}

func newWalletPasskeyRegisterCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey register", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to protect (default: active wallet)")

	return &Command{
		Name:  "register",
		Short: "Register a passkey and enable seed protection",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Register Passkey")

			svc := wallet.NewService()
			target, err := resolvePasskeyTarget(svc, *address)
			if err != nil {
				return err
			}

			spin := ui.NewSpinner("Running WebAuthn registration ceremony...")
			spin.Start()
			cred, err := svc.RegisterPasskey(target)
			if err != nil {
				spin.Stop(false, "Passkey registration failed")
				return err
			}
			spin.Stop(true, "Passkey registered")

			ui.SectionLabel("Credential")
			ui.KV("Wallet", target)
			ui.KV("Credential ID", safeTruncW(base64.RawURLEncoding.EncodeToString(cred.ID), 24)+"...")
			ui.Info("The wallet seed is now encrypted at rest.")
			ui.Info("Run 'stellar-go-cli wallet passkey unlock' before signing operations.")
			return nil
		},
	}
}

func newWalletPasskeyUnlockCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey unlock", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to unlock (default: active wallet)")
	ttl := fs.Duration("ttl", 0, "How long the wallet stays unlocked (default 15m)")

	return &Command{
		Name:  "unlock",
		Short: "Authenticate with passkey to unlock the wallet seed",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Unlock Wallet")

			svc := wallet.NewService()
			target, err := resolvePasskeyTarget(svc, *address)
			if err != nil {
				return err
			}

			spin := ui.NewSpinner("Waiting for passkey assertion...")
			spin.Start()
			if err := svc.Unlock(target, *ttl); err != nil {
				spin.Stop(false, "Unlock failed")
				return err
			}

			st, _ := svc.GetPasskeyStatus(target) //nolint:errcheck // display-only best effort
			spin.Stop(true, "Wallet unlocked")
			if st != nil && st.UnlockedUntil != nil {
				ui.KV("Valid until", st.UnlockedUntil.Format(time.RFC3339))
			}
			ui.Info("Lock early with 'stellar-go-cli wallet passkey lock'")
			return nil
		},
	}
}

func newWalletPasskeyLockCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey lock", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address to lock (default: active wallet)")

	return &Command{
		Name:  "lock",
		Short: "Immediately lock a passkey-protected wallet",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			svc := wallet.NewService()
			target, err := resolvePasskeyTarget(svc, *address)
			if err != nil {
				return err
			}
			if err := svc.Lock(target); err != nil {
				return err
			}
			ui.Success("Wallet locked: " + target)
			return nil
		},
	}
}

func newWalletPasskeyStatusCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey status", flag.ContinueOnError)

	return &Command{
		Name:  "status",
		Short: "Show passkey protection state for all wallets",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Passkey Status")

			svc := wallet.NewService()
			wallets, _, err := svc.ListWallets()
			if err != nil {
				return fmt.Errorf("failed to list wallets: %w", err)
			}

			any := false
			for _, w := range wallets {
				st, err := svc.GetPasskeyStatus(w.Address)
				if err != nil || !st.Protected {
					continue
				}
				any = true
				state := "locked"
				if st.Unlocked && st.UnlockedUntil != nil {
					state = "unlocked until " + st.UnlockedUntil.Format("15:04:05")
				}
				ui.KV(w.Address[:12]+"...", fmt.Sprintf("%s (%d passkey(s))", state, len(st.Credentials)))
			}
			if !any {
				ui.Info("No passkey-protected wallets.")
				ui.Info("Enable with 'stellar-go-cli wallet passkey register'")
			}
			return nil
		},
	}
}

func newWalletPasskeyListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey list", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address (default: active wallet)")

	return &Command{
		Name:  "list",
		Short: "List passkey credentials for a wallet",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			svc := wallet.NewService()
			target, err := resolvePasskeyTarget(svc, *address)
			if err != nil {
				return err
			}

			st, err := svc.GetPasskeyStatus(target)
			if err != nil {
				return err
			}
			if len(st.Credentials) == 0 {
				ui.Info("No passkeys registered for " + target)
				return nil
			}
			ui.SectionLabel("Passkeys for " + target)
			for _, c := range st.Credentials {
				ui.KV("Credential", c.CredentialID[:16]+"...")
				ui.KV("  Sign count", fmt.Sprintf("%d", c.SignCount))
				if len(c.Transports) > 0 {
					ui.KV("  Transports", fmt.Sprintf("%v", c.Transports))
				}
			}
			return nil
		},
	}
}

func newWalletPasskeyDisableCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("passkey disable", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address (default: active wallet)")

	return &Command{
		Name:  "disable",
		Short: "Remove passkey protection and restore the plaintext seed",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Disable Passkey Protection")

			svc := wallet.NewService()
			target, err := resolvePasskeyTarget(svc, *address)
			if err != nil {
				return err
			}

			ui.Warn("This restores the plaintext seed in the wallet file.")
			spin := ui.NewSpinner("Waiting for passkey assertion...")
			spin.Start()
			if err := svc.DisablePasskey(target); err != nil {
				spin.Stop(false, "Disable failed")
				return err
			}
			spin.Stop(true, "Passkey protection removed")
			return nil
		},
	}
}

func safeTruncW(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func stellarExplorerURL(address string, network models.Network) string {
	switch network {
	case models.NetworkStellarTestnet:
		return fmt.Sprintf("https://stellar.expert/explorer/testnet/account/%s", address)
	case models.NetworkStellarMainnet:
		return fmt.Sprintf("https://stellar.expert/explorer/public/account/%s", address)
	default:
		return fmt.Sprintf("https://stellar.expert/explorer/testnet/account/%s", address)
	}
}
