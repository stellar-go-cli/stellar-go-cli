package commands

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/mcp"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
)

func newMcpCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	transport := fs.String("transport", "stdio", "Transport type: stdio | sse")
	port := fs.Int("port", 3000, "Port for SSE transport (only used with --transport sse)")
	bind := fs.String("bind", "", "Bind address for SSE transport (default 127.0.0.1, or MCP_BIND env)")
	token := fs.String("token", "", "Bearer token for SSE transport auth (or MCP_TOKEN env); required for non-loopback bind")
	profile := fs.String("profile", "admin", "Tool exposure profile: readonly | transact | admin (or MCP_PROFILE env)")
	x402RPCURL := fs.String("x402-rpc-url", "", "Operator-pinned Soroban RPC endpoint for pay_x402 (or MCP_X402_RPC_URL env)")
	x402MaxAmount := fs.String("x402-max-amount", "", "Operator hard spend cap for pay_x402 in atomic units (or MCP_X402_MAX_AMOUNT env)")
	allowPrivate := fs.Bool("allow-private-resources", false, "Allow pay_x402 fetches to private/local addresses (SSRF guard bypass — dangerous)")
	allowedOrigins := fs.String("allowed-origins", "", "Comma-separated extra Origin/Host names for the HTTP transport (or MCP_ALLOWED_ORIGINS env)")
	signerSocket := fs.String("signer-socket", "", "Custodian signer daemon socket path (default: state dir signer.sock, or MCP_SIGNER_SOCKET env)")
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
			profileName := *profile
			if env := os.Getenv("MCP_PROFILE"); env != "" && profileName == "admin" {
				profileName = env
			}
			rpcURL := *x402RPCURL
			if rpcURL == "" {
				rpcURL = os.Getenv("MCP_X402_RPC_URL")
			}
			maxAmount := *x402MaxAmount
			if maxAmount == "" {
				maxAmount = os.Getenv("MCP_X402_MAX_AMOUNT")
			}
			originsRaw := *allowedOrigins
			if originsRaw == "" {
				originsRaw = os.Getenv("MCP_ALLOWED_ORIGINS")
			}
			signerSock := *signerSocket
			if signerSock == "" {
				signerSock = os.Getenv("MCP_SIGNER_SOCKET")
			}
			if signerSock == "" {
				if def, derr := signsock.DefaultSocketPath(); derr == nil {
					signerSock = def
				}
			}
			var origins []string
			for _, o := range strings.Split(originsRaw, ",") {
				if o = strings.TrimSpace(o); o != "" {
					origins = append(origins, o)
				}
			}

			server := mcp.NewServer(cfg)
			if err := server.ApplyOperatorOptions(mcp.OperatorOptions{
				Profile:               profileName,
				X402RPCURL:            rpcURL,
				X402MaxAtomicAmount:   maxAmount,
				AllowPrivateResources: *allowPrivate,
				AllowedOrigins:        origins,
				SignerSocket:          signerSock,
			}); err != nil {
				return err
			}
			return server.Start(*transport, *port, bindAddr, authToken)
		},
	}
}
