package commands

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/ui"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Short       string
	Long        string
	Args        []string
	Flags       *flag.FlagSet
	SubCommands map[string]*Command
	Run         func(cmd *Command, args []string) error
	cfg         *config.Config
}

// RootCmd is the top-level command
type RootCmd struct {
	cfg      *config.Config
	commands map[string]*Command
}

func NewRootCmd(cfg *config.Config) *RootCmd {
	r := &RootCmd{
		cfg:      cfg,
		commands: make(map[string]*Command),
	}

	// Register all sub-commands
	r.register(newDIDCmd(cfg))
	r.register(newWalletCmd(cfg))
	r.register(newPayCmd(cfg))
	r.register(newSwapCmd(cfg))
	r.register(newPoolCmd(cfg))
	r.register(newTradeCmd(cfg))
	r.register(newAssetCmd(cfg))
	r.register(newClaimableCmd(cfg))
	r.register(newIntegrationsCmd(cfg))
	r.register(newReportCmd(cfg))
	r.register(newVersionCmd(cfg))
	r.register(newInitCmd(cfg))
	r.register(newStatusCmd(cfg))
	r.register(newFlowCmd(cfg))
	r.register(newNetworkCmd(cfg))
	r.register(newMcpCmd(cfg))
	r.register(newChatCmd(cfg))
	r.register(newTerminalCmd(cfg))
	r.register(newExchangeCmd(cfg))
	r.register(newContractCmd(cfg))
	r.register(newVCApiCmd(cfg))

	return r
}

func (r *RootCmd) register(cmd *Command) {
	r.commands[cmd.Name] = cmd
}

func (r *RootCmd) Execute() error {
	args := os.Args[1:]

	// Show banner + help if no args
	if len(args) == 0 {
		ui.PrintBanner()
		r.printHelp()
		return nil
	}

	// Global flags
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("mozartpay %s\n", config.Version)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		ui.PrintBanner()
		r.printHelp()
		return nil
	}

	name := args[0]
	cmd, ok := r.commands[name]
	if !ok {
		ui.Error(fmt.Sprintf("unknown command: %q", name))
		fmt.Println()
		r.printHelp()
		return fmt.Errorf("unknown command: %s", name)
	}

	return cmd.execute(args[1:])
}

func (r *RootCmd) printHelp() {
	fmt.Printf("  %s\n\n", ui.Dim_("Usage: mozartpay <command> [flags]"))

	fmt.Printf("  %s\n\n", ui.Bold_("Commands:"))

	groups := []struct {
		label    string
		commands []string
	}{
		{"Identity", []string{"did"}},
		{"Wallet", []string{"wallet"}},
		{"Transactions", []string{"pay", "swap", "pool", "trade", "asset", "claimable"}},
		{"Exchanges", []string{"exchange"}},
		{"Contracts", []string{"contract"}},
		{"Integrations", []string{"integrations"}},
		{"Reporting", []string{"report"}},
		{"AI", []string{"mcp", "chat"}},
		{"System", []string{"init", "status", "flow", "network", "version", "vc-api"}},
	}

	for _, g := range groups {
		fmt.Printf("  %s\n", ui.Dim_(g.label))
		for _, name := range g.commands {
			if cmd, ok := r.commands[name]; ok {
				fmt.Printf("    %-20s %s\n",
					ui.Teal_(cmd.Name),
					ui.Dim_(cmd.Short),
				)
			}
		}
		fmt.Println()
	}

	fmt.Printf("  %s\n", ui.Dim_("Run 'mozartpay <command> --help' for command-specific help."))
	fmt.Println()
}

// ─────────────────────────────────────────────
// Command execution
// ─────────────────────────────────────────────

func (c *Command) execute(args []string) error {
	// Check for sub-command
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if sub, ok := c.SubCommands[args[0]]; ok {
			return sub.execute(args[1:])
		}
	}

	// Check for help
	for _, a := range args {
		if a == "--help" || a == "-h" {
			c.printHelp()
			return nil
		}
	}

	// Parse flags
	if c.Flags != nil {
		c.Flags.Parse(args)
		c.Args = c.Flags.Args()
	} else {
		c.Args = args
	}

	if c.Run == nil {
		c.printHelp()
		return nil
	}

	return c.Run(c, c.Args)
}

func (c *Command) printHelp() {
	fmt.Println()
	fmt.Printf("  %s — %s\n", ui.Gold_(c.Name), c.Short)
	if c.Long != "" {
		fmt.Printf("\n  %s\n", ui.Dim_(c.Long))
	}

	if len(c.SubCommands) > 0 {
		fmt.Printf("\n  %s\n", ui.Bold_("Sub-commands:"))
		for name, sub := range c.SubCommands {
			fmt.Printf("    %-20s %s\n", ui.Teal_(name), ui.Dim_(sub.Short))
		}
	}

	if c.Flags != nil {
		fmt.Printf("\n  %s\n", ui.Bold_("Flags:"))
		c.Flags.SetOutput(os.Stdout)
		fmt.Print("  ")
		c.Flags.PrintDefaults()
	}
	fmt.Println()
}

// Helper to add sub-command
func (c *Command) addSub(sub *Command) {
	if c.SubCommands == nil {
		c.SubCommands = make(map[string]*Command)
	}
	c.SubCommands[sub.Name] = sub
}
