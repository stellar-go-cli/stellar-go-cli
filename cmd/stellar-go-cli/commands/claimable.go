package commands

import (
	"flag"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/claimables"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

func newClaimableCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "claimable",
		Short: "Manage claimable balances on Stellar",
		Long:  "List, accept, or decline claimable balances for your Stellar account.",
		cfg:   cfg,
	}
	cmd.addSub(newClaimableListCmd(cfg))
	cmd.addSub(newClaimableAcceptCmd(cfg))
	cmd.addSub(newClaimableAcceptAllCmd(cfg))
	cmd.addSub(newClaimableDeclineCmd(cfg))
	cmd.addSub(newClaimableDeclineAllCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── claimable list ───────────────────────────

func newClaimableListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "list",
		Short: "List all pending claimable balances",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Claimable Balances")

			net := models.Network(*network)
			if net == "" {
				// Try to get from active wallet
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				}
			}

			// Load active wallet
			walletSvc := wallet.NewService()
			active, err := walletSvc.GetActiveWallet()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			svc := claimables.NewService(net)

			ui.PrintStep(1, "Fetching Claimable Balances")
			spin := ui.NewSpinner("Querying Horizon...")
			spin.Start()

			balances, err := svc.ListClaimableBalances(active.Address)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to list balances: %w", err)
			}
			spin.Stop(true, fmt.Sprintf("Found %d balance(s)", len(balances)))

			if *output == "json" {
				fmt.Println(prettyJSON(balances))
				return nil
			}

			if len(balances) == 0 {
				ui.Info("No claimable balances found for this account")
				return nil
			}

			ui.SectionLabel("Pending Claimable Balances")
			for i, b := range balances {
				asset := b.AssetCode
				if !b.IsNativeAsset {
					asset = fmt.Sprintf("%s:%s...", b.AssetCode, truncateStr(b.AssetIssuer, 8))
				}
				ui.Separator()
				ui.KV(fmt.Sprintf("#%d ID", i+1), truncateStr(b.ID, 16))
				ui.KV("  Amount", fmt.Sprintf("%s %s", b.Amount, asset))
				ui.KV("  Sponsor", truncateStr(b.Sponsor, 16))
				ui.KV("  Created", b.CreatedAt.Format(time.RFC3339))
			}
			ui.Separator()

			ui.Info("Run 'stellar-go-cli claimable accept --id <BALANCE_ID>' to claim a balance")
			return nil
		},
	}
}

// ─── claimable accept ─────────────────────────

func newClaimableAcceptCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("accept", flag.ContinueOnError)
	balanceID := fs.String("id", "", "Balance ID to claim (required)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "accept",
		Short: "Accept (claim) a specific claimable balance",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Accept Claimable Balance")

			if *balanceID == "" {
				return fmt.Errorf("--id flag is required (use 'claimable list' to see balance IDs)")
			}

			net := models.Network(*network)
			if net == "" {
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				}
			}

			svc := claimables.NewService(net)

			ui.PrintStep(1, "Claiming Balance")
			spin := ui.NewSpinner(fmt.Sprintf("Claiming balance %s...", truncateStr(*balanceID, 12)))
			spin.Start()

			txHash, err := svc.AcceptClaimableBalance(*balanceID)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to claim balance: %w", err)
			}
			spin.Stop(true, "Balance claimed")

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]string{
					"balance_id": *balanceID,
					"tx_hash":    txHash,
					"status":     "claimed",
				}))
				return nil
			}

			ui.Success("Claimable balance accepted")
			ui.KV("Balance ID", truncateStr(*balanceID, 16))
			ui.KV("TX Hash", txHash)
			ui.Info("The balance has been added to your account")

			return nil
		},
	}
}

// ─── claimable accept-all ─────────────────────

func newClaimableAcceptAllCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("accept-all", flag.ContinueOnError)
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "accept-all",
		Short: "Accept all pending claimable balances",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Accept All Claimable Balances")

			net := models.Network(*network)
			if net == "" {
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				}
			}

			// Load active wallet
			walletSvc := wallet.NewService()
			active, err := walletSvc.GetActiveWallet()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			svc := claimables.NewService(net)

			// First, get count
			ui.PrintStep(1, "Checking Balances")
			spin := ui.NewSpinner("Fetching claimable balances...")
			spin.Start()

			balances, err := svc.ListClaimableBalances(active.Address)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to list balances: %w", err)
			}
			spin.Stop(true, fmt.Sprintf("Found %d balance(s)", len(balances)))

			if len(balances) == 0 {
				ui.Info("No claimable balances to accept")
				return nil
			}

			if !ui.Confirm(fmt.Sprintf("Accept all %d claimable balances in one transaction?", len(balances))) {
				ui.Info("Cancelled")
				return nil
			}

			ui.PrintStep(2, "Accepting All Balances")
			spin = ui.NewSpinner("Building batch transaction...")
			spin.Start()

			txHashes, errs := svc.AcceptAllClaimableBalances(active.Address)
			if len(errs) > 0 {
				spin.Stop(false, errs[0].Error())
				return fmt.Errorf("failed to accept balances: %w", errs[0])
			}
			spin.Stop(true, fmt.Sprintf("Accepted %d balance(s)", len(balances)))

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]interface{}{
					"tx_hashes":      txHashes,
					"balances_count": len(balances),
					"status":         "claimed",
				}))
				return nil
			}

			ui.Success("All claimable balances accepted")
			ui.KV("Balances Claimed", fmt.Sprintf("%d", len(balances)))
			for i, hash := range txHashes {
				ui.KV(fmt.Sprintf("TX Hash %d", i+1), hash)
			}

			return nil
		},
	}
}

// ─── claimable decline ────────────────────────

func newClaimableDeclineCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("decline", flag.ContinueOnError)
	balanceID := fs.String("id", "", "Balance ID to decline (required)")
	assetCode := fs.String("asset-code", "", "Asset code (required)")
	assetIssuer := fs.String("asset-issuer", "", "Asset issuer (empty for XLM)")
	amount := fs.String("amount", "", "Amount to return (required)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "decline",
		Short: "Decline a claimable balance (claim and return to sponsor)",
		Long:  "Claims the balance and immediately sends it back to the sponsor",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Decline Claimable Balance")

			if *balanceID == "" {
				return fmt.Errorf("--id flag is required (use 'claimable list' to see balance IDs)")
			}
			if *assetCode == "" {
				return fmt.Errorf("--asset-code flag is required")
			}
			if *amount == "" {
				return fmt.Errorf("--amount flag is required")
			}

			net := models.Network(*network)
			if net == "" {
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				}
			}

			svc := claimables.NewService(net)

			ui.SectionLabel("Decline Details")
			ui.KV("Balance ID", truncateStr(*balanceID, 16))
			ui.KV("Amount", fmt.Sprintf("%s %s", *amount, *assetCode))
			ui.Warn("This will claim the balance and return it to the sponsor")

			if !ui.Confirm("Proceed with declining this balance?") {
				ui.Info("Cancelled")
				return nil
			}

			ui.PrintStep(1, "Declining Balance")
			spin := ui.NewSpinner("Claiming and returning to sponsor...")
			spin.Start()

			txHash, err := svc.DeclineClaimableBalance(*balanceID, *assetCode, *assetIssuer, *amount)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to decline balance: %w", err)
			}
			spin.Stop(true, "Balance declined")

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]string{
					"balance_id": *balanceID,
					"tx_hash":    txHash,
					"status":     "declined",
				}))
				return nil
			}

			ui.Success("Claimable balance declined")
			ui.KV("Balance ID", truncateStr(*balanceID, 16))
			ui.KV("TX Hash", txHash)
			ui.Info("The balance was claimed and returned to the sponsor")

			return nil
		},
	}
}

// ─── claimable decline-all ────────────────────

func newClaimableDeclineAllCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("decline-all", flag.ContinueOnError)
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	confirm := fs.Bool("confirm", false, "Skip confirmation prompt")
	output := fs.String("output", "pretty", "Output format: pretty | json")

	return &Command{
		Name:  "decline-all",
		Short: "Decline all pending claimable balances",
		Long:  "Claims all balances and returns them to their sponsors in a single transaction",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Decline All Claimable Balances")

			net := models.Network(*network)
			if net == "" {
				walletSvc := wallet.NewService()
				if acc, err := walletSvc.GetActiveWallet(); err == nil {
					net = acc.Network
				}
			}

			// Load active wallet
			walletSvc := wallet.NewService()
			active, err := walletSvc.GetActiveWallet()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			svc := claimables.NewService(net)

			// First, get count and list
			ui.PrintStep(1, "Checking Balances")
			spin := ui.NewSpinner("Fetching claimable balances...")
			spin.Start()

			balances, err := svc.ListClaimableBalances(active.Address)
			if err != nil {
				spin.Stop(false, err.Error())
				return fmt.Errorf("failed to list balances: %w", err)
			}
			spin.Stop(true, fmt.Sprintf("Found %d balance(s)", len(balances)))

			if len(balances) == 0 {
				ui.Info("No claimable balances to decline")
				return nil
			}

			ui.SectionLabel("Balances to Decline")
			for i, b := range balances {
				asset := b.AssetCode
				if !b.IsNativeAsset {
					asset = fmt.Sprintf("%s:%s...", b.AssetCode, truncateStr(b.AssetIssuer, 8))
				}
				ui.KV(fmt.Sprintf("  %d", i+1), fmt.Sprintf("%s %s → %s...", b.Amount, asset, truncateStr(b.Sponsor, 8)))
			}
			ui.KV("Total", fmt.Sprintf("%d balances", len(balances)))
			ui.Warn("This will claim all balances and return them to their sponsors")

			if !*confirm && !ui.Confirm(fmt.Sprintf("Decline all %d claimable balances?", len(balances))) {
				ui.Info("Cancelled")
				return nil
			}

			ui.PrintStep(2, "Declining All Balances")
			spin = ui.NewSpinner("Building batch transaction...")
			spin.Start()

			txHashes, skippedBalances, skipReasons, errs := svc.DeclineAllClaimableBalances(active.Address)
			if len(errs) > 0 {
				spin.Stop(false, errs[0].Error())
				return fmt.Errorf("failed to decline balances: %w", errs[0])
			}

			declinedCount := len(balances) - len(skippedBalances)
			spin.Stop(true, fmt.Sprintf("Declined %d balance(s)", declinedCount))

			if *output == "json" {
				fmt.Println(prettyJSON(map[string]interface{}{
					"tx_hashes":        txHashes,
					"declined_count":   declinedCount,
					"skipped_count":    len(skippedBalances),
					"skipped_balances": skippedBalances,
					"skip_reasons":     skipReasons,
					"status":           "declined",
				}))
				return nil
			}

			ui.Success(fmt.Sprintf("%d claimable balance(s) declined", declinedCount))
			ui.KV("Balances Declined", fmt.Sprintf("%d", declinedCount))
			for i, hash := range txHashes {
				ui.KV(fmt.Sprintf("TX Hash %d", i+1), hash)
			}

			// Show skipped balances with reasons
			if len(skippedBalances) > 0 {
				ui.SectionLabel("Skipped Balances (Need Trustlines)")
				for i, b := range skippedBalances {
					asset := b.AssetCode
					if !b.IsNativeAsset {
						asset = fmt.Sprintf("%s:%s...", b.AssetCode, truncateStr(b.AssetIssuer, 8))
					}
					ui.KV(fmt.Sprintf("  %d", i+1), fmt.Sprintf("%s %s - %s", b.Amount, asset, skipReasons[i]))
				}
				ui.Info("To decline these, first establish trustlines: stellar-go-cli asset trust --code <CODE> --issuer <ISSUER>")
			}

			return nil
		},
	}
}
