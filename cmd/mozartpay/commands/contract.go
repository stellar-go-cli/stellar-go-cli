package commands

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/ui"
	"github.com/ogtechnologies/mozartpay/internal/wallet"
)

func newContractCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "contract",
		Short: "Interact with Orchestrated Agreement smart contract",
		Long:  "Create and manage orchestrated agreements on Stellar.",
		cfg:   cfg,
	}
	cmd.addSub(newContractCreateAgreementCmd(cfg))
	cmd.addSub(newContractShowCmd(cfg))
	cmd.addSub(newContractListCmd(cfg))
	cmd.addSub(newContractAttestIdentityCmd(cfg))
	cmd.addSub(newContractConnectWalletCmd(cfg))
	cmd.addSub(newContractFundAssetCmd(cfg))
	cmd.addSub(newContractExecuteCmd(cfg))
	cmd.addSub(newContractSettleCmd(cfg))
	cmd.addSub(newContractSetCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── contract set ───────────────────────────

func newContractSetCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	contractID := fs.String("id", "", "Contract ID to store")

	return &Command{
		Name:  "set",
		Short: "Store the deployed contract ID",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *contractID == "" {
				ui.Error("Contract ID required (--id)")
				return fmt.Errorf("missing contract ID")
			}

			cfg.ContractID = *contractID
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			ui.Success(fmt.Sprintf("Contract ID stored: %s", *contractID))
			return nil
		},
	}
}

// ─── contract create-agreement ───────────────────────────

func newContractCreateAgreementCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("create-agreement", flag.ContinueOnError)
	counterparty := fs.String("counterparty", "", "Optional counterparty address")
	expiresAt := fs.Uint64("expires-at", 0, "Optional expiration timestamp (Unix)")
	disputeWindow := fs.Uint("dispute-window", 86400, "Dispute window in seconds")

	return &Command{
		Name:  "create-agreement",
		Short: "Create a new orchestrated agreement",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured. Run: mozartpay contract set --id <CONTRACT_ID>")
				return fmt.Errorf("contract ID not set")
			}

			// Get active wallet
			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Create Orchestrated Agreement")
			ui.Info(fmt.Sprintf("Initiator: %s", account.Address))
			ui.Info(fmt.Sprintf("Contract: %s", cfg.ContractID))

			// Build stellar contract invoke command
			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"create_agreement",
				"--initiator", account.Address,
				"--dispute_window", fmt.Sprintf("%d", *disputeWindow),
			}

			if *counterparty != "" {
				cmdArgs = append(cmdArgs, "--counterparty", *counterparty)
			}
			if *expiresAt > 0 {
				cmdArgs = append(cmdArgs, "--expires_at", fmt.Sprintf("%d", *expiresAt))
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			// Parse agreement ID from output
			outputStr := string(output)
			var agreementID string
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) == 64 && isHexString(line) {
					agreementID = line
					break
				}
			}

			if agreementID == "" {
				ui.Warn("Could not parse agreement ID from output")
				fmt.Println(outputStr)
				return nil
			}

			// Store agreement ID
			cfg.AgreementIDs = append(cfg.AgreementIDs, agreementID)
			cfg.LastAgreementID = agreementID
			if err := config.Save(cfg); err != nil {
				ui.Warn("Failed to save agreement ID to config")
			}

			ui.Success(fmt.Sprintf("Agreement created: %s", agreementID))
			ui.Info(fmt.Sprintf("Explorer: https://stellar.expert/explorer/testnet/contract/%s", cfg.ContractID))

			return nil
		},
	}
}

// ─── contract show ───────────────────────────

func newContractShowCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID (uses last created if omitted)")

	return &Command{
		Name:  "show",
		Short: "Display agreement details",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided and no recent agreement found")
				return fmt.Errorf("agreement ID required")
			}

			// Get active wallet
			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Agreement Details")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"get_agreement",
				"--agreement_id", id,
			}

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			fmt.Println(string(output))
			return nil
		},
	}
}

// ─── contract list ───────────────────────────

func newContractListCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "list",
		Short: "List agreements by initiator",
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("My Agreements")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"get_initiator_agreements",
				"--initiator", account.Address,
			}

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			// Display stored agreements
			if len(cfg.AgreementIDs) > 0 {
				ui.SectionLabel("Stored Agreement IDs:")
				for i, id := range cfg.AgreementIDs {
					marker := ""
					if id == cfg.LastAgreementID {
						marker = " (latest)"
					}
					fmt.Printf("  %d. %s%s\n", i+1, id, marker)
				}
			}

			ui.SectionLabel("On-chain Result:")
			fmt.Println(string(output))
			return nil
		},
	}
}

// ─── contract attest-identity ───────────────────────────

func newContractAttestIdentityCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("attest-identity", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID")
	did := fs.String("did", "", "Decentralized Identifier (e.g., did:web:example.com)")
	method := fs.String("method", "web", "DID method: web | key | ethr | ebsi")
	vcType := fs.String("vc-type", "national_id", "Type of verifiable credential")

	return &Command{
		Name:  "attest-identity",
		Short: "Attest DID and VC for an agreement (Layer 1)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided")
				return fmt.Errorf("agreement ID required")
			}

			if *did == "" {
				ui.Error("DID required (--did)")
				return fmt.Errorf("DID required")
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Attest Identity")

			// Generate attestation hash (simplified - in production this would be a real hash)
			attestationHash := generateHash(id + *did + time.Now().String())

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"attest_identity",
				"--agreement_id", id,
				"--did", *did,
				"--did_method", *method,
				"--vc_type", *vcType,
				"--attestation_hash", attestationHash,
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Identity attested successfully")
			return nil
		},
	}
}

// ─── contract connect-wallet ───────────────────────────

func newContractConnectWalletCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("connect-wallet", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID")
	walletType := fs.String("type", "wwwallet", "Wallet type: wwwallet | external | testnet")
	passkey := fs.Bool("passkey", true, "Enable passkey authentication")

	return &Command{
		Name:  "connect-wallet",
		Short: "Connect wallet to an agreement (Layer 2)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided")
				return fmt.Errorf("agreement ID required")
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Connect Wallet")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"connect_wallet",
				"--agreement_id", id,
				"--stellar_address", account.Address,
				"--wallet_type", *walletType,
				"--passkey_enabled", fmt.Sprintf("%t", *passkey),
				"--required_signers", "1",
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Wallet connected successfully")
			return nil
		},
	}
}

// ─── contract fund-asset ───────────────────────────

func newContractFundAssetCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("fund-asset", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID")
	assetCode := fs.String("asset", "USDC", "Asset code")
	amount := fs.String("amount", "100", "Total amount")
	locked := fs.String("locked", "0", "Amount to lock as collateral")

	return &Command{
		Name:  "fund-asset",
		Short: "Fund and set asset for an agreement (Layer 3)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided")
				return fmt.Errorf("agreement ID required")
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Fund Asset")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"fund_and_set_asset",
				"--agreement_id", id,
				"--asset_code", *assetCode,
				"--amount", *amount,
				"--locked_amount", *locked,
				"--asset_type", "fungible",
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Asset funded successfully")
			return nil
		},
	}
}

// ─── contract execute ───────────────────────────

func newContractExecuteCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID")
	txHash := fs.String("tx-hash", "", "Transaction hash for settlement")

	return &Command{
		Name:  "execute",
		Short: "Execute an agreement (Layer 5)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided")
				return fmt.Errorf("agreement ID required")
			}

			finalTxHash := *txHash
			if finalTxHash == "" {
				finalTxHash = generateHash(id + time.Now().String())
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Execute Agreement")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"execute_agreement",
				"--agreement_id", id,
				"--final_tx_hash", finalTxHash,
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Agreement executed successfully")
			return nil
		},
	}
}

// ─── contract settle ───────────────────────────

func newContractSettleCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("settle", flag.ContinueOnError)
	agreementID := fs.String("id", "", "Agreement ID")

	return &Command{
		Name:  "settle",
		Short: "Settle an agreement",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if cfg.ContractID == "" {
				ui.Error("No contract ID configured")
				return fmt.Errorf("contract ID not set")
			}

			id := *agreementID
			if id == "" {
				id = cfg.LastAgreementID
			}
			if id == "" {
				ui.Error("No agreement ID provided")
				return fmt.Errorf("agreement ID required")
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			ui.Header("Settle Agreement")

			cmdArgs := []string{
				"contract", "invoke",
				"--id", cfg.ContractID,
				"--source", account.Address,
				"--rpc-url", "https://soroban-testnet.stellar.org",
				"--network-passphrase", "Test SDF Network ; September 2015",
				"--",
				"settle_agreement",
				"--agreement_id", id,
			}

			ui.SectionLabel("Submitting transaction...")

			cmd := exec.Command("stellar", cmdArgs...)
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/opt/homebrew/etc/ca-certificates/cert.pem")
			output, err := cmd.CombinedOutput()
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %s", string(output)))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Agreement settled successfully")
			return nil
		},
	}
}

// ─── Helpers ───────────────────────────

func isHexString(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func generateHash(input string) string {
	// Simplified hash generation - in production use proper SHA256
	h := sha256.New()
	h.Write([]byte(input))
	return fmt.Sprintf("%x", h.Sum(nil))
}
