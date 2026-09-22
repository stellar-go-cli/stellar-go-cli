package commands

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/soroban"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/xdr"
)

func newContractCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "contract",
		Short: "Interact with Soroban smart contracts on Stellar",
		Long:  "Deploy and manage smart contracts on Stellar using pure Go Soroban RPC.",
		cfg:   cfg,
	}
	cmd.addSub(newContractDeployCmd(cfg))
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

// ─── contract deploy ───────────────────────────

func newContractDeployCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	wasmPath := fs.String("wasm", "", "Path to the .wasm file to deploy (required)")
	network := fs.String("network", "", "Stellar network: stellar-testnet or stellar-mainnet (defaults to config)")
	constructorArgsStr := fs.String("constructor-args", "", "Constructor args as comma-separated XDR ScVal base64 strings (optional)")
	ownerAddr := fs.String("owner", "", "Owner address for contracts with __constructor(owner: Address). Defaults to deployer address")

	return &Command{
		Name:  "deploy",
		Short: "Deploy any .wasm contract to Soroban (pure Go, no stellar CLI)",
		Long:  "Uploads WASM bytecode and creates a contract instance on the Soroban network.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *wasmPath == "" {
				ui.Error("WASM file path required (--wasm)")
				return fmt.Errorf("missing --wasm flag")
			}

			wasmBytes, err := os.ReadFile(*wasmPath)
			if err != nil {
				ui.Error(fmt.Sprintf("Failed to read WASM file: %v", err))
				return fmt.Errorf("failed to read wasm: %w", err)
			}

			net := *network
			if net == "" {
				net = cfg.Network
			}
			if net == "" {
				net = "stellar-testnet"
			}

			svc := wallet.NewService()
			account, err := svc.GetActiveAccount()
			if err != nil {
				return fmt.Errorf("no active wallet: %w", err)
			}

			kp, err := keypair.ParseFull(account.PrivateKey)
			if err != nil {
				return fmt.Errorf("invalid private key in wallet: %w", err)
			}

			ui.Header("Deploy Soroban Contract")
			ui.Info(fmt.Sprintf("WASM file: %s (%d bytes)", *wasmPath, len(wasmBytes)))
			ui.Info(fmt.Sprintf("Network: %s", net))
			ui.Info(fmt.Sprintf("Deployer: %s", account.Address))

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			ui.SectionLabel("Uploading WASM bytecode...")

			var constructorArgs []xdr.ScVal
			if *constructorArgsStr != "" {
				for _, argB64 := range splitComma(*constructorArgsStr) {
					var scv xdr.ScVal
					if err := xdr.SafeUnmarshalBase64(argB64, &scv); err != nil {
						return fmt.Errorf("invalid constructor arg %q: %w", argB64, err)
					}
					constructorArgs = append(constructorArgs, scv)
				}
			} else if *ownerAddr != "" {
				ownerScAddr, err := soroban.AccountToScAddress(*ownerAddr)
				if err != nil {
					return fmt.Errorf("invalid owner address: %w", err)
				}
				constructorArgs = []xdr.ScVal{soroban.ScvAddress(ownerScAddr)}
			} else {
				deployerScAddr, err := soroban.AccountToScAddress(account.Address)
				if err != nil {
					return fmt.Errorf("failed to convert deployer address: %w", err)
				}
				constructorArgs = []xdr.ScVal{soroban.ScvAddress(deployerScAddr)}
			}

			result, err := client.DeployWithConstructorArgs(ctx, kp, wasmBytes, constructorArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Deployment failed: %v", err))
				return fmt.Errorf("deploy failed: %w", err)
			}

			ui.Success("Contract deployed successfully!")
			ui.Info(fmt.Sprintf("Contract ID: %s", result.ContractID))
			ui.Info(fmt.Sprintf("WASM Hash:   %s", result.WasmHash))
			ui.Info(fmt.Sprintf("Upload TX:   %s", result.UploadTxHash))
			ui.Info(fmt.Sprintf("Create TX:   %s", result.CreateTxHash))

			explorerURL := "https://stellar.expert/explorer/testnet/contract/" + result.ContractID
			if net == "stellar-mainnet" {
				explorerURL = "https://stellar.expert/explorer/public/contract/" + result.ContractID
			}
			ui.Info(fmt.Sprintf("Explorer: %s", explorerURL))

			cfg.ContractID = result.ContractID
			if err := config.Save(cfg); err != nil {
				ui.Warn("Failed to save contract ID to config")
			} else {
				ui.SectionLabel("Contract ID saved to config")
			}

			return nil
		},
	}
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

			kp, net, account, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Create Orchestrated Agreement")
			ui.Info(fmt.Sprintf("Initiator: %s", account.Address))
			ui.Info(fmt.Sprintf("Contract: %s", cfg.ContractID))

			initiatorAddr, err := soroban.AccountToScAddress(account.Address)
			if err != nil {
				return fmt.Errorf("failed to convert address: %w", err)
			}

			invArgs := []xdr.ScVal{
				soroban.ScvAddress(initiatorAddr),
				soroban.ScvU64(uint64(*disputeWindow)),
			}
			if *counterparty != "" {
				cpAddr, err := soroban.AccountToScAddress(*counterparty)
				if err != nil {
					return fmt.Errorf("invalid counterparty address: %w", err)
				}
				invArgs = append(invArgs, soroban.ScvAddress(cpAddr))
			}
			if *expiresAt > 0 {
				invArgs = append(invArgs, soroban.ScvU64(*expiresAt))
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "create_agreement", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success(fmt.Sprintf("Agreement created. TX: %s", result.TxHash))
			ui.Info(fmt.Sprintf("Result: %s", result.ResultXDR))
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

			_, net, _, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Agreement Details")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.SimulateOnly(ctx, cfg.ContractID, "get_agreement", []xdr.ScVal{
				soroban.ScvString(id),
			})
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("simulate failed: %w", err)
			}

			fmt.Println(result)
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

			_, net, account, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("My Agreements")

			initiatorAddr, err := soroban.AccountToScAddress(account.Address)
			if err != nil {
				return fmt.Errorf("failed to convert address: %w", err)
			}

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.SimulateOnly(ctx, cfg.ContractID, "get_initiator_agreements", []xdr.ScVal{
				soroban.ScvAddress(initiatorAddr),
			})
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("simulate failed: %w", err)
			}

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
			fmt.Println(result)
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

			kp, net, _, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Attest Identity")

			attestationHash := generateHash(id + *did + time.Now().String())

			invArgs := []xdr.ScVal{
				soroban.ScvString(id),
				soroban.ScvString(*did),
				soroban.ScvString(*method),
				soroban.ScvString(*vcType),
				soroban.ScvBytes([]byte(attestationHash)),
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "attest_identity", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Identity attested successfully")
			ui.Info(fmt.Sprintf("TX: %s", result.TxHash))
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

			kp, net, account, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Connect Wallet")

			stellarAddr, err := soroban.AccountToScAddress(account.Address)
			if err != nil {
				return fmt.Errorf("failed to convert address: %w", err)
			}

			invArgs := []xdr.ScVal{
				soroban.ScvString(id),
				soroban.ScvAddress(stellarAddr),
				soroban.ScvString(*walletType),
				soroban.ScvBool(*passkey),
				soroban.ScvU32(1),
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "connect_wallet", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Wallet connected successfully")
			ui.Info(fmt.Sprintf("TX: %s", result.TxHash))
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

			kp, net, _, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Fund Asset")

			invArgs := []xdr.ScVal{
				soroban.ScvString(id),
				soroban.ScvString(*assetCode),
				soroban.ScvString(*amount),
				soroban.ScvString(*locked),
				soroban.ScvString("fungible"),
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "fund_and_set_asset", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Asset funded successfully")
			ui.Info(fmt.Sprintf("TX: %s", result.TxHash))
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

			kp, net, _, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Execute Agreement")

			invArgs := []xdr.ScVal{
				soroban.ScvString(id),
				soroban.ScvBytes([]byte(finalTxHash)),
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "execute_agreement", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Agreement executed successfully")
			ui.Info(fmt.Sprintf("TX: %s", result.TxHash))
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

			kp, net, _, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}

			ui.Header("Settle Agreement")

			invArgs := []xdr.ScVal{
				soroban.ScvString(id),
			}

			ui.SectionLabel("Submitting transaction...")

			client := soroban.NewClientForNetwork(net)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := client.Invoke(ctx, kp, cfg.ContractID, "settle_agreement", invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success("Agreement settled successfully")
			ui.Info(fmt.Sprintf("TX: %s", result.TxHash))
			return nil
		},
	}
}

// ─── Helpers ───────────────────────────

func getKeypairAndNetwork(cfg *config.Config) (*keypair.Full, string, *models.Account, error) {
	svc := wallet.NewService()
	account, err := svc.GetActiveAccount()
	if err != nil {
		return nil, "", nil, fmt.Errorf("no active wallet: %w", err)
	}

	kp, err := keypair.ParseFull(account.PrivateKey)
	if err != nil {
		return nil, "", nil, fmt.Errorf("invalid private key in wallet: %w", err)
	}

	net := cfg.Network
	if net == "" {
		net = "stellar-testnet"
	}

	return kp, net, account, nil
}

func generateHash(input string) string {
	h := sha256.New()
	h.Write([]byte(input))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
