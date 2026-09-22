package commands

import (
	"flag"
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/mcp"
)

func newMcpCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	transport := fs.String("transport", "stdio", "Transport type: stdio | sse")
	port := fs.Int("port", 3000, "Port for SSE transport (only used with --transport sse)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")

	return &Command{
		Name:  "mcp",
		Short: "Start MCP server for AI assistant integration",
		Long: `Start an MCP (Model Context Protocol) server that exposes MozartPay CLI functionality as tools.

This allows AI assistants to programmatically control the CLI via stdio or SSE transport.

Transport modes:
  - stdio: JSON-RPC over stdin/stdout (default, for direct AI integration)
  - sse:   Server-Sent Events over HTTP (for browser/HTTP clients)

Example usage:
  mozartpay mcp                          # Start with stdio transport
  mozartpay mcp --transport sse --port 3000  # Start HTTP server

The server exposes tools like:
  - wallet_list, wallet_balance          # Wallet operations
  - swap_quote, swap_execute             # Asset swapping
  - pay_send, pay_request                # Payments
  - asset_list, asset_trust              # Asset management
  - system_status, system_health          # System monitoring`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			// MCP protocol requires clean stdout for JSON-RPC only
			// ui.Header intentionally omitted

			if *verbose {
				fmt.Printf("Transport: %s\n", *transport)
				if *transport == "sse" {
					fmt.Printf("Port: %d\n", *port)
				}
			}

			server := mcp.NewServer(cfg)
			return server.Start(*transport, *port)
		},
	}
}
