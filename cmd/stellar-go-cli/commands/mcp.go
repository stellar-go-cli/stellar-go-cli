package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/mcp"
)

func newMcpCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	transport := fs.String("transport", "stdio", "Transport type: stdio | sse")
	port := fs.Int("port", 3000, "Port for SSE transport (only used with --transport sse)")
	bind := fs.String("bind", "", "Bind address for SSE transport (default 127.0.0.1, or MCP_BIND env)")
	token := fs.String("token", "", "Bearer token for SSE transport auth (or MCP_TOKEN env); required for non-loopback bind")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")

	return &Command{
		Name:  "mcp",
		Short: "Start MCP server for AI assistant integration",
		Long: `Start an MCP (Model Context Protocol) server that exposes Stellar Go CLI functionality as tools.

This allows AI assistants to programmatically control the CLI via stdio or SSE transport.

Transport modes:
  - stdio: JSON-RPC over stdin/stdout (default, for direct AI integration)
  - sse:   Server-Sent Events over HTTP (for browser/HTTP clients)

The SSE server binds to loopback by default. Binding a non-loopback address
requires --token (or MCP_TOKEN) — the server refuses to expose unauthenticated
signing tools on the network.

Example usage:
  stellar-go-cli mcp                          # Start with stdio transport
  stellar-go-cli mcp --transport sse --port 3000  # Start HTTP server (loopback)
  stellar-go-cli mcp --transport sse --bind 0.0.0.0 --token "$MCP_TOKEN"  # Network exposure

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

			bindAddr := *bind
			if bindAddr == "" {
				bindAddr = os.Getenv("MCP_BIND")
			}
			authToken := *token
			if authToken == "" {
				authToken = os.Getenv("MCP_TOKEN")
			}

			server := mcp.NewServer(cfg)
			return server.Start(*transport, *port, bindAddr, authToken)
		},
	}
}
