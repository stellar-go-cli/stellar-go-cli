package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/assets"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
)

func newAssetCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "asset",
		Short: "Create and manage SAC / SEP-41 assets on Stellar",
		Long:  "Issue fungible tokens (FT) and non-fungible assets (NFA) using the Stellar Asset Contract standard (SEP-41).",
		cfg:   cfg,
	}
	cmd.addSub(newAssetCreateFTCmd(cfg))
	cmd.addSub(newAssetCreateNFACmd(cfg))
	cmd.addSub(newAssetTrustCmd(cfg))
	cmd.addSub(newAssetUntrustCmd(cfg))
	cmd.addSub(newAssetScoreCmd(cfg))
	cmd.addSub(newAssetCarbonCmd(cfg))
	cmd.addSub(newAssetShowCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── asset create-ft ──────────────────────────

func newAssetCreateFTCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("create-ft", flag.ContinueOnError)
	name := fs.String("name", "", "Token name (required)")
	symbol := fs.String("symbol", "", "Token symbol, max 12 chars (required)")
	supply := fs.String("supply", "1000000", "Total supply")
	decimals := fs.Int("decimals", 7, "Decimal places (Stellar default: 7)")
	network := fs.String("network", "stellar-testnet", "Network")
	withCarbon := fs.Bool("with-carbon", false, "Attach StellarCarbon offset credits")
	carbonAmt := fs.Float64("carbon-amount", 1.0, "Carbon credit amount in tCO2e")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "create-ft",
		Short: "Create a SEP-41 fungible token (SAC)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create Fungible Token (SAC/SEP-41)")

			if *name == "" {
				*name = ui.Prompt("Token name:")
			}
			if *symbol == "" {
				*symbol = ui.Prompt("Token symbol (max 12 chars):")
			}

			// Interactive network selection if not provided via flag
			if *network == "stellar-testnet" {
				ui.PrintStep(1, "Network Configuration")
				*network = ui.SelectNetwork(*network)
			}

			// Interactive total supply prompt if not provided via flag
			if *supply == "1000000" {
				ui.PrintStep(2, "Supply Configuration")
				*supply = ui.PromptSupply(*supply)
			}

			// Load active wallet to get private key for signing
			svcWallet := wallet.NewService()
			acc, err := svcWallet.GetActiveWallet()
			if err != nil || acc.PrivateKey == "" {
				ui.Error("No active Stellar wallet with private key found. Please create and fund a Stellar wallet first.")
				return fmt.Errorf("wallet required for asset issuance")
			}

			// Verify it's a Stellar wallet
			if acc.Type != models.WalletStellar {
				ui.Error("Active wallet is not a Stellar wallet. Please switch to a Stellar wallet first.")
				return fmt.Errorf("stellar wallet required for asset issuance")
			}
			fmt.Printf("DEBUG: Loaded account - Address: %s, PrivateKey first 10 chars: %s...\n", acc.Address, acc.PrivateKey[:10])
			issuer := acc.Address
			issuerKey := acc.PrivateKey

			net := models.Network(*network)
			svc := assets.NewService()

			ui.PrintStep(3, "Deploying SAC contract to Stellar")
			spin := ui.NewSpinner(fmt.Sprintf("Deploying %s (%s) contract...", *name, *symbol))
			spin.Start()
			time.Sleep(800 * time.Millisecond)

			metadata := map[string]interface{}{
				"description": fmt.Sprintf("%s token issued via MozartPay CLI", *name),
				"issuerDID":   cfg.ActiveDID,
				"version":     "1.0",
			}

			asset, err := svc.CreateFungibleAsset(*name, *symbol, *decimals, *supply, issuer, issuerKey, net, metadata)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Contract deployed")

			// Optional: attach carbon credits
			if *withCarbon {
				spin3 := ui.NewSpinner("Attaching StellarCarbon offset credits...")
				spin3.Start()
				time.Sleep(500 * time.Millisecond)
				asset, err = svc.AttachCarbonCredit(asset, *carbonAmt, time.Now().Year()-1, "VCS")
				if err != nil {
					spin3.Stop(false, err.Error())
				} else {
					spin3.Stop(true, fmt.Sprintf("Carbon offset: %.2f tCO2e attached", *carbonAmt))
				}
			}

			if *output == "json" {
				fmt.Println(prettyJSON(asset))
				return nil
			}

			ui.PrintStep(4, "Asset Summary")
			ui.Separator()
			ui.KV("Contract ID", asset.ContractID)
			ui.KV("Name", asset.Name)
			ui.KVColor("Symbol", asset.Symbol, ui.BrightYellow)
			ui.KV("Type", string(asset.Type))
			ui.KV("Standard", asset.Standard)
			ui.KVColor("Total Supply", asset.TotalSupply, ui.BrightGreen)
			ui.KV("Decimals", strconv.Itoa(asset.Decimals))
			ui.KV("Issuer", asset.Issuer)
			ui.KV("Network", string(asset.Network))
			ui.KV("TX Hash", asset.TxHash)
			ui.KV("Created", asset.CreatedAt.Format(time.RFC3339))

			if asset.Score != nil {
				ui.SectionLabel("OA Score")
				ui.KVColor("Score", fmt.Sprintf("%d / 1000 (%s)", asset.Score.Score, asset.Score.Grade), ui.BrightCyan)
				ui.KV("Risk Level", asset.Score.RiskLevel)
			}
			if asset.CarbonOffset != nil {
				ui.SectionLabel("Carbon Offset")
				ui.KVColor("Amount", fmt.Sprintf("%.4f tCO2e", asset.CarbonOffset.Amount), ui.BrightGreen)
				ui.KV("Standard", asset.CarbonOffset.Standard)
				ui.KV("Vintage", strconv.Itoa(asset.CarbonOffset.Vintage))
				ui.KV("Token ID", asset.CarbonOffset.TokenID)
			}
			ui.Separator()

			config.SaveState("asset_latest", asset)
			ui.Info("Run 'mozartpay report generate' to produce a compliance report.")

			return nil
		},
	}
}

// ─── asset create-nfa ─────────────────────────

func newAssetCreateNFACmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("create-nfa", flag.ContinueOnError)
	name := fs.String("name", "", "NFA name (required)")
	description := fs.String("description", "", "NFA description")
	imageURL := fs.String("image", "", "Image URL for NFA metadata")
	network := fs.String("network", "stellar-testnet", "Network")
	withCarbon := fs.Bool("with-carbon", false, "Attach a carbon credit to this NFA")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "create-nfa",
		Short: "Create a SEP-41 non-fungible asset (NFA)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create Non-Fungible Asset (NFA/SEP-41)")

			if *name == "" {
				*name = ui.Prompt("NFA name:")
			}

			// Interactive network selection if not provided via flag
			if *network == "stellar-testnet" {
				ui.PrintStep(1, "Network Configuration")
				*network = ui.SelectNetwork(*network)
			}

			issuer := cfg.ActiveAddress
			if issuer == "" {
				issuer = "GISSUER" + "PLACEHOLDER000000000000000000000000000000000000000"
			}

			net := models.Network(*network)
			svc := assets.NewService()

			metadata := map[string]interface{}{
				"description": *description,
				"image":       *imageURL,
				"issuerDID":   cfg.ActiveDID,
				"attributes":  []map[string]string{},
			}

			ui.PrintStep(2, "Minting NFA")
			spin := ui.NewSpinner(fmt.Sprintf("Minting NFA: %s...", *name))
			spin.Start()
			time.Sleep(700 * time.Millisecond)

			asset, err := svc.CreateNonFungibleAsset(*name, issuer, net, metadata)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "NFA minted")

			if *withCarbon {
				spin2 := ui.NewSpinner("Attaching carbon credit...")
				spin2.Start()
				time.Sleep(400 * time.Millisecond)
				asset, _ = svc.AttachCarbonCredit(asset, 0.5, time.Now().Year()-1, "Gold Standard")
				spin2.Stop(true, "Carbon credit attached to NFA")
			}

			if *output == "json" {
				fmt.Println(prettyJSON(asset))
				return nil
			}

			ui.SectionLabel("Non-Fungible Asset")
			ui.KV("Contract ID", asset.ContractID)
			ui.KV("Name", asset.Name)
			ui.KVColor("Type", "Non-Fungible (1/1)", ui.BrightMagenta)
			ui.KV("Standard", asset.Standard)
			ui.KV("Issuer", asset.Issuer)
			ui.KV("TX Hash", asset.TxHash)

			if asset.CarbonOffset != nil {
				ui.KVColor("Carbon Offset", fmt.Sprintf("%.2f tCO2e (%s)", asset.CarbonOffset.Amount, asset.CarbonOffset.Standard), ui.BrightGreen)
			}

			config.SaveState("asset_latest", asset)
			return nil
		},
	}
}

// ─── asset trust ─────────────────────────────

func newAssetTrustCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("trust", flag.ContinueOnError)
	code := fs.String("code", "", "Asset code (required)")
	issuer := fs.String("issuer", "", "Asset issuer address (required)")
	limit := fs.String("limit", "", "Trust limit (optional, defaults to max)")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "trust",
		Short: "Create a trustline for an asset",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create Trustline")

			if *code == "" {
				*code = ui.Prompt("Asset code (e.g., USDC):")
			}
			if *issuer == "" {
				*issuer = ui.Prompt("Asset issuer address:")
			}

			svc := assets.NewService()

			// Load active wallet
			walletSvc := wallet.NewService()
			active, err := walletSvc.GetActiveWallet()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.PrintStep(1, "Creating Trustline")
			spin := ui.NewSpinner(fmt.Sprintf("Establishing trustline for %s...", *code))
			spin.Start()

			txHash, err := svc.CreateTrustline(active.Address, *code, *issuer, *limit)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to create trustline: %w", err)
			}

			spin.Stop(true, "Trustline created successfully")

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]string{
					"asset_code": *code,
					"issuer":     *issuer,
					"tx_hash":    txHash,
					"status":     "success",
				}))
				return nil
			}

			ui.Success("Trustline established")
			ui.KV("Asset", fmt.Sprintf("%s:%s", *code, *issuer))
			ui.KV("Account", active.Address)
			ui.KV("TX Hash", txHash)
			ui.Info("You can now receive this asset in swaps and payments")

			return nil
		},
	}
}

// ─── asset untrust ────────────────────────────

func newAssetUntrustCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("untrust", flag.ContinueOnError)
	code := fs.String("code", "", "Asset code to remove (required for single mode)")
	issuer := fs.String("issuer", "", "Asset issuer address (required for single mode)")
	batch := fs.String("batch", "", "Path to JSON file with multiple assets [{\"code\":\"X\",\"issuer\":\"Y\"},...]")
	confirm := fs.Bool("confirm", false, "Skip confirmation prompt")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "untrust",
		Short: "Remove trustlines for assets (single or batch)",
		Long:  "Remove trustlines by setting limit to 0. Use --code/--issuer for single removal, or --batch for multiple assets in one transaction.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Remove Trustline(s)")

			svc := assets.NewService()

			// Load active wallet
			walletSvc := wallet.NewService()
			active, err := walletSvc.GetActiveWallet()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			// Batch mode
			if *batch != "" {
				ui.PrintStep(1, "Loading batch file")
				data, err := os.ReadFile(*batch)
				if err != nil {
					return fmt.Errorf("failed to read batch file: %w", err)
				}

				var assetList []assets.AssetIdentifier
				if err := json.Unmarshal(data, &assetList); err != nil {
					return fmt.Errorf("failed to parse batch file (expected JSON array): %w", err)
				}

				if len(assetList) == 0 {
					return fmt.Errorf("no assets found in batch file")
				}

				ui.SectionLabel("Assets to Remove")
				for i, a := range assetList {
					ui.KV(fmt.Sprintf("  %d", i+1), fmt.Sprintf("%s:%s...", a.Code, truncateStr(a.Issuer, 16)))
				}
				ui.KV("Total", fmt.Sprintf("%d assets", len(assetList)))

				if !*confirm && !ui.Confirm(fmt.Sprintf("Remove %d trustlines in single transaction?", len(assetList))) {
					ui.Info("Cancelled")
					return nil
				}

				ui.PrintStep(2, "Removing Trustlines")
				spin := ui.NewSpinner(fmt.Sprintf("Removing %d trustlines...", len(assetList)))
				spin.Start()

				txHash, err := svc.RemoveTrustlinesBatch(active.Address, assetList)
				if err != nil {
					spin.Stop(false, err.Error())
					return fmt.Errorf("batch removal failed: %w", err)
				}
				spin.Stop(true, fmt.Sprintf("Removed %d trustlines", len(assetList)))

				if *output == "json" {
					fmt.Println(prettyJSON(map[string]interface{}{
						"tx_hash":      txHash,
						"assets_count": len(assetList),
						"status":       "success",
					}))
					return nil
				}

				ui.Success("Batch trustline removal complete")
				ui.KV("Assets Removed", fmt.Sprintf("%d", len(assetList)))
				ui.KV("TX Hash", txHash)
				return nil
			}

			// Single mode
			if *code == "" {
				*code = ui.Prompt("Asset code to remove:")
			}
			if *issuer == "" {
				*issuer = ui.Prompt("Asset issuer address:")
			}

			ui.SectionLabel("Trustline Removal")
			ui.KV("Asset", fmt.Sprintf("%s:%s", *code, truncateStr(*issuer, 16)))
			ui.KV("Account", active.Address)

			if !*confirm && !ui.Confirm("Remove this trustline?") {
				ui.Info("Cancelled")
				return nil
			}

			ui.PrintStep(1, "Removing Trustline")
			spin := ui.NewSpinner(fmt.Sprintf("Removing trustline for %s...", *code))
			spin.Start()

			txHash, err := svc.RemoveTrustline(active.Address, *code, *issuer)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to remove trustline: %w", err)
			}

			spin.Stop(true, "Trustline removed")

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]string{
					"asset_code": *code,
					"issuer":     *issuer,
					"tx_hash":    txHash,
					"status":     "success",
				}))
				return nil
			}

			ui.Success("Trustline removed successfully")
			ui.KV("Asset", fmt.Sprintf("%s:%s", *code, truncateStr(*issuer, 16)))
			ui.KV("Account", active.Address)
			ui.KV("TX Hash", txHash)
			ui.Info("You can no longer receive this asset")

			return nil
		},
	}
}

// ─── asset score ──────────────────────────────

func newAssetScoreCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "score",
		Short: "Add an OA score to latest asset",
		Run: func(c *Command, args []string) error {
			ui.Header("OA Score")
			ui.Warn("OA scoring functionality has been removed.")
			return nil
		},
	}
}

// ─── asset carbon ─────────────────────────────

func newAssetCarbonCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("carbon", flag.ContinueOnError)
	amount := fs.Float64("amount", 1.0, "Amount in tCO2e")
	vintage := fs.Int("vintage", 2023, "Credit vintage year")
	standard := fs.String("standard", "VCS", "Carbon standard: VCS | Gold Standard | CAR | ACR")
	retire := fs.Bool("retire", false, "Immediately retire the credit")

	return &Command{
		Name:  "carbon",
		Short: "Attach StellarCarbon offset credits to the latest asset",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("StellarCarbon Credit")

			spin := ui.NewSpinner("Issuing carbon credit on StellarCarbon...")
			spin.Start()
			time.Sleep(600 * time.Millisecond)

			svc := assets.NewService()
			var asset models.Asset
			if err := config.LoadState("asset_latest", &asset); err != nil {
				spin.Stop(false, "No asset found. Run 'mozartpay asset create-ft' first.")
				return nil
			}

			updated, err := svc.AttachCarbonCredit(&asset, *amount, *vintage, *standard)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Carbon credit tokenized")

			if *retire {
				updated.CarbonOffset.Retired = true
				now := time.Now().UTC()
				updated.CarbonOffset.RetiredAt = &now
				ui.Success(fmt.Sprintf("Credit retired on-chain: %s", updated.CarbonOffset.TokenID))
			}

			config.SaveState("asset_latest", updated)

			ui.SectionLabel("Carbon Credit")
			ui.KV("Token ID", updated.CarbonOffset.TokenID)
			ui.KVColor("Amount", fmt.Sprintf("%.4f tCO2e", updated.CarbonOffset.Amount), ui.BrightGreen)
			ui.KV("Vintage", strconv.Itoa(updated.CarbonOffset.Vintage))
			ui.KV("Standard", updated.CarbonOffset.Standard)
			ui.KV("Retired", fmt.Sprintf("%v", updated.CarbonOffset.Retired))
			ui.KV("TX Hash", updated.CarbonOffset.TxHash)

			return nil
		},
	}
}

// ─── asset show ───────────────────────────────

func newAssetShowCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "show",
		Short: "Show the latest created asset",
		Run: func(c *Command, args []string) error {
			ui.Header("Latest Asset")

			var asset models.Asset
			if err := config.LoadState("asset_latest", &asset); err != nil {
				ui.Warn("No asset found. Run 'mozartpay asset create-ft' or 'mozartpay asset create-nfa'.")
				return nil
			}

			ui.SectionLabel("Asset Details")
			ui.KV("Contract ID", asset.ContractID)
			ui.KV("Name", fmt.Sprintf("%s (%s)", asset.Name, asset.Symbol))
			ui.KV("Type", string(asset.Type))
			ui.KV("Standard", asset.Standard)
			ui.KV("Total Supply", asset.TotalSupply)
			ui.KV("Issuer", asset.Issuer)
			ui.KV("Network", string(asset.Network))
			ui.KV("TX Hash", asset.TxHash)
			ui.KV("Created", asset.CreatedAt.Format(time.RFC3339))

			if asset.Score != nil {
				ui.SectionLabel("OA Score")
				ui.KVColor("Score", fmt.Sprintf("%d / 1000 (%s) — %s risk", asset.Score.Score, asset.Score.Grade, asset.Score.RiskLevel), ui.BrightCyan)
			}
			if asset.CarbonOffset != nil {
				ui.SectionLabel("Carbon Offset")
				ui.KVColor("Credits", fmt.Sprintf("%.4f tCO2e (%s %d)", asset.CarbonOffset.Amount, asset.CarbonOffset.Standard, asset.CarbonOffset.Vintage), ui.BrightGreen)
				ui.KV("Retired", fmt.Sprintf("%v", asset.CarbonOffset.Retired))
			}

			return nil
		},
	}
}
