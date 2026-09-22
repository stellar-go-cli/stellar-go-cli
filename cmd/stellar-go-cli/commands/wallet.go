package commands

import (
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
		Short: "Manage wallets, passkeys, and account connections",
		Long:  "Connect wwWallet via passkey, create or import a Stellar keypair, or register external EOA.",
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
	provider := fs.String("provider", "", "Wallet provider: wwwallet | stellar | external (interactive if omitted)")
	network := fs.String("network", "stellar-testnet", "Network: stellar-testnet | stellar-mainnet | evm-sepolia | evm-mainnet")
	address := fs.String("address", "", "External address (for --provider external)")
	passkey := fs.Bool("passkey", true, "Authenticate via WebAuthn passkey (wwwallet)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "connect",
		Short: "Connect a wallet (wwWallet, Stellar, or external EOA)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Connect Wallet")

			net := models.Network(*network)
			svc := wallet.NewService()

			var acc *models.Account
			var passkeyCred *models.PasskeyCredential
			var err error

			// Interactive provider selection if not specified
			selectedProvider := *provider
			if selectedProvider == "" {
				ui.SectionLabel("Select Wallet Type")
				fmt.Println("1) wwWallet (WebAuthn passkeys)")
				fmt.Println("2) Stellar (Native Stellar keypair)")
				fmt.Println("3) External EOA")
				fmt.Print("Choose option [1-3]: ")

				var choice string
				fmt.Scanln(&choice) //nolint:errcheck // empty input falls through to the default
				switch choice {
				case "1":
					selectedProvider = "wwwallet"
				case "2":
					selectedProvider = "stellar"
				case "3":
					selectedProvider = "external"
				default:
					ui.Error("Invalid choice. Please select 1, 2, or 3.")
					return nil
				}
			}

			switch selectedProvider {
			case "wwwallet":
				spin := ui.NewSpinner("Initiating WebAuthn passkey ceremony...")
				spin.Start()
				acc, passkeyCred, err = svc.ConnectWWWallet(net)
				if err != nil {
					spin.Stop(false, "Connection failed")
					return err
				}
				spin.Stop(true, "wwWallet connected via passkey")

			case "stellar":
				spin := ui.NewSpinner("Creating native Stellar keypair...")
				spin.Start()
				acc, passkeyCred, err = svc.ConnectStellarWallet(net)
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
			if passkeyCred != nil && *passkey {
				ui.SectionLabel("Passkey Credential")
				ui.KV("Credential ID", safeTruncW(passkeyCred.CredentialID, 24)+"...")
				ui.KV("Algorithm", passkeyCred.Algorithm)
				ui.KV("Origin", passkeyCred.Origin)
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

			// DEBUG
			fmt.Fprintf(os.Stderr, "\nDEBUG: Funded=%v, Network='%s', Testnet='%s', Mainnet='%s'\n", acc.Funded, acc.Network, models.NetworkStellarTestnet, models.NetworkStellarMainnet)
			fmt.Fprintf(os.Stderr, "DEBUG: Is testnet=%v, Is mainnet=%v\n", acc.Network == models.NetworkStellarTestnet, acc.Network == models.NetworkStellarMainnet)

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
			if *showPrivate && acc.PrivateKey != "" {
				ui.SectionLabel("⚠️  PRIVATE KEY - KEEP SECURE")
				ui.KV("Private Key", acc.PrivateKey)
				ui.Warn("Store this key securely and never share it!")
			} else if *showPrivate {
				ui.Warn("Private key not available for this wallet type")
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

// ─── wallet passkey ───────────────────────────

func newWalletPasskeyCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "passkey",
		Short: "Manage wwWallet passkeys",
		Long:  "Create, verify, list, and remove WebAuthn passkeys for wwWallet authentication.",
		cfg:   cfg,
	}
	cmd.addSub(newWalletPasskeyCreateCmd(cfg))
	cmd.addSub(newWalletPasskeyVerifyCmd(cfg))
	cmd.addSub(newWalletPasskeyListCmd(cfg))
	cmd.addSub(newWalletPasskeyRemoveCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── wallet passkey create ─────────────────────

func newWalletPasskeyCreateCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address (uses active wallet if omitted)")

	return &Command{
		Name:  "create",
		Short: "Create a new passkey for wallet",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create Passkey")

			svc := wallet.NewService()
			var targetAddress string

			if *address != "" {
				targetAddress = *address
			} else {
				// Use active wallet
				acc, err := svc.GetActiveWallet()
				if err != nil {
					ui.Error("No active wallet found. Use --address or set an active wallet first.")
					return nil
				}
				targetAddress = acc.Address

				if acc.Type != models.WalletWWWallet {
					ui.Warn("Active wallet is not a wwWallet. Creating passkey will convert it to wwWallet.")
				}
			}

			spin := ui.NewSpinner("Creating new passkey...")
			spin.Start()

			passkey, err := svc.CreatePasskey(targetAddress)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Passkey created successfully")

			ui.SectionLabel("New Passkey")
			ui.KV("Credential ID", safeTruncW(passkey.CredentialID, 24)+"...")
			ui.KV("Public Key", safeTruncW(passkey.PublicKey, 20)+"...")
			ui.KV("Algorithm", passkey.Algorithm)
			ui.KV("Origin", passkey.Origin)
			ui.KV("Created", passkey.CreatedAt.Format(time.RFC3339))

			ui.Success("Passkey created and linked to wallet")
			ui.Info("Use 'stellar-go-cli wallet passkey verify' to test the passkey")

			return nil
		},
	}
}

// ─── wallet passkey verify ─────────────────────

func newWalletPasskeyVerifyCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address (uses active wallet if omitted)")

	return &Command{
		Name:  "verify",
		Short: "Verify passkey functionality",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Verify Passkey")

			svc := wallet.NewService()
			var targetAddress string

			if *address != "" {
				targetAddress = *address
			} else {
				// Use active wallet
				acc, err := svc.GetActiveWallet()
				if err != nil {
					ui.Error("No active wallet found. Use --address or set an active wallet first.")
					return nil
				}
				targetAddress = acc.Address
			}

			spin := ui.NewSpinner("Verifying passkey...")
			spin.Start()

			valid, err := svc.VerifyPasskey(targetAddress)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			if valid {
				spin.Stop(true, "Passkey verification successful")
				ui.Success("Passkey is working correctly")
				ui.KV("Wallet Address", targetAddress)
				ui.KV("Status", "Valid")
			} else {
				spin.Stop(false, "Passkey verification failed")
				ui.Error("Passkey verification failed")
				ui.Info("The wallet may not have a passkey or the passkey is invalid")
			}

			return nil
		},
	}
}

// ─── wallet passkey list ───────────────────────

func newWalletPasskeyListCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "list",
		Short: "List all passkey credentials",
		Run: func(c *Command, args []string) error {
			ui.Header("Passkey Credentials")

			svc := wallet.NewService()
			passkeys, err := svc.ListPasskeys()
			if err != nil {
				ui.Error("Failed to list passkeys: " + err.Error())
				return err
			}

			if len(passkeys) == 0 {
				ui.Info("No passkey credentials found")
				ui.Info("Create a passkey with: stellar-go-cli wallet passkey create")
				return nil
			}

			ui.SectionLabel(fmt.Sprintf("Found %d passkey(s)", len(passkeys)))

			for i, pk := range passkeys {
				ui.KV(fmt.Sprintf("[%d] Address", i+1), safeTruncW(pk.Address, 30)+"...")
				ui.KV("    Network", pk.Network)
				ui.KV("    Algorithm", pk.Algorithm)
				ui.KV("    Origin", pk.Origin)
				ui.KV("    Created", pk.Created.Format(time.RFC3339))
				fmt.Println()
			}

			ui.Success("Passkey list complete")
			return nil
		},
	}
}

// ─── wallet passkey remove ─────────────────────

func newWalletPasskeyRemoveCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	address := fs.String("address", "", "Wallet address (uses active wallet if omitted)")
	confirm := fs.Bool("confirm", false, "Skip confirmation prompt")

	return &Command{
		Name:  "remove",
		Short: "Remove passkey from wallet",
		Long:  "Removes the passkey from a wwWallet, converting it to an external wallet. This action cannot be undone.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Remove Passkey")

			svc := wallet.NewService()
			var targetAddress string

			if *address != "" {
				targetAddress = *address
			} else {
				// Use active wallet
				acc, err := svc.GetActiveWallet()
				if err != nil {
					ui.Error("No active wallet found. Use --address or set an active wallet first.")
					return nil
				}
				targetAddress = acc.Address
			}

			// Get wallet info for confirmation
			acc, err := svc.GetWalletByAddress(targetAddress)
			if err != nil {
				ui.Error("Wallet not found: " + err.Error())
				return err
			}

			if acc.Type != models.WalletWWWallet {
				ui.Error("This wallet is not a wwWallet and has no passkey to remove")
				return nil
			}

			// Show wallet info
			ui.SectionLabel("Wallet to Remove Passkey From")
			ui.KV("Address", safeTruncW(targetAddress, 30)+"...")
			ui.KV("Type", string(acc.Type))
			ui.KV("Network", string(acc.Network))

			// Confirmation
			if !*confirm {
				fmt.Println()
				ui.Warn("⚠️  This will remove the passkey and convert the wallet to an external wallet.")
				ui.Warn("   This action cannot be undone.")
				fmt.Print("Type 'remove' to confirm: ")
				var confirmation string
				fmt.Scanln(&confirmation) //nolint:errcheck // empty input falls through to the default

				if confirmation != "remove" {
					ui.Info("Passkey removal cancelled")
					return nil
				}
			}

			spin := ui.NewSpinner("Removing passkey...")
			spin.Start()

			err = svc.RemovePasskey(targetAddress)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}

			spin.Stop(true, "Passkey removed successfully")

			ui.Success("Passkey removed from wallet")
			ui.KV("Wallet Address", safeTruncW(targetAddress, 30)+"...")
			ui.KV("New Type", "external")
			ui.Info("The wallet is now an external wallet and can be managed with private keys")

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
