package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/xdr"
)

func newContractCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "contract",
		Short: "Interact with Soroban smart contracts on Stellar",
		Long:  "Deploy and invoke smart contracts on Stellar using pure Go Soroban RPC.",
		cfg:   cfg,
	}
	cmd.addSub(newContractDeployCmd(cfg))
	cmd.addSub(newContractInvokeCmd(cfg))
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
			defer client.Close() //nolint:errcheck // RPC client cleanup

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

// ─── contract invoke ───────────────────────────

func newContractInvokeCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("invoke", flag.ContinueOnError)
	contractID := fs.String("id", "", "Contract ID (defaults to the last deployed contract)")
	fnName := fs.String("fn", "", "Contract function name to invoke (required)")
	argsStr := fs.String("args", "", "Arguments as comma-separated XDR ScVal base64 strings (optional)")
	simulate := fs.Bool("simulate", false, "Simulate only — do not submit a transaction")
	network := fs.String("network", "", "Stellar network: stellar-testnet or stellar-mainnet (defaults to config)")

	return &Command{
		Name:  "invoke",
		Short: "Invoke a contract method (submit or simulate)",
		Long:  "Invokes a Soroban contract method. With --simulate the call is read-only and no transaction is submitted.",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *fnName == "" {
				ui.Error("Function name required (--fn)")
				return fmt.Errorf("missing --fn flag")
			}

			id := *contractID
			if id == "" {
				id = cfg.ContractID
			}
			if id == "" {
				ui.Error("No contract ID. Pass --id or run: stellar-go-cli contract deploy")
				return fmt.Errorf("contract ID not set")
			}

			var invArgs []xdr.ScVal
			if *argsStr != "" {
				for _, argB64 := range splitComma(*argsStr) {
					var scv xdr.ScVal
					if err := xdr.SafeUnmarshalBase64(argB64, &scv); err != nil {
						return fmt.Errorf("invalid arg %q: %w", argB64, err)
					}
					invArgs = append(invArgs, scv)
				}
			}

			net := *network
			if net == "" {
				net = cfg.Network
			}
			if net == "" {
				net = "stellar-testnet"
			}

			client := soroban.NewClientForNetwork(net)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ui.Header("Invoke Contract")
			ui.Info(fmt.Sprintf("Contract: %s", id))
			ui.Info(fmt.Sprintf("Function: %s", *fnName))
			ui.Info(fmt.Sprintf("Network:  %s", net))

			if *simulate {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				result, err := client.SimulateOnly(ctx, id, *fnName, invArgs)
				if err != nil {
					ui.Error(fmt.Sprintf("Simulation failed: %v", err))
					return fmt.Errorf("simulate failed: %w", err)
				}
				ui.SectionLabel("Simulation Result")
				fmt.Println(result)
				return nil
			}

			kp, _, account, err := getKeypairAndNetwork(cfg)
			if err != nil {
				return err
			}
			ui.Info(fmt.Sprintf("Invoker: %s", account.Address))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			ui.SectionLabel("Submitting transaction...")

			result, err := client.Invoke(ctx, kp, id, *fnName, invArgs)
			if err != nil {
				ui.Error(fmt.Sprintf("Transaction failed: %v", err))
				return fmt.Errorf("invoke failed: %w", err)
			}

			ui.Success(fmt.Sprintf("Invocation succeeded. TX: %s", result.TxHash))
			ui.Info(fmt.Sprintf("Result: %s", result.ResultXDR))
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

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
