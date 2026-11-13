package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/custodian"
	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
)

func newSignerCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("signer", flag.ContinueOnError)
	socket := fs.String("socket", "", "Custodian socket path (default: ~/.stellar-go-cli/state/signer.sock)")
	policy := fs.String("policy", "", "Policy file path (default: ~/.stellar-go-cli/state/policy.json)")

	cmd := &Command{
		Name:  "signer",
		Short: "Run the signer custodian daemon",
		Long: `Run the signer custodian — the local daemon that owns key custody for
agent-facing flows.

The custodian holds passkey unlock grants in memory and enforces the operator
policy (hard limits, network allowlist, denylist, escalation thresholds)
independently on every signing request. The MCP server and CLI sign through
it over a Unix socket; no seed material ever leaves this process.

Architecture: docs/audit/mcp-signer-architecture.md

Example usage:
  stellar-go-cli signer                          # default socket + policy
  stellar-go-cli signer --socket /tmp/s.sock     # custom socket
  stellar-go-cli mcp                             # in another terminal — mcp talks to the socket`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			sockPath := *socket
			if sockPath == "" {
				var err error
				sockPath, err = signsock.DefaultSocketPath()
				if err != nil {
					return err
				}
			}
			policyPath := *policy
			if policyPath == "" {
				dir, err := config.StateDir()
				if err != nil {
					return err
				}
				policyPath = dir + "/policy.json"
			}
			logger := log.New(os.Stderr, "[signer] ", log.LstdFlags)
			fmt.Fprintf(os.Stderr, "Starting custodian daemon (socket: %s, policy: %s)\n", sockPath, policyPath)
			return custodian.Run(sockPath, policyPath, logger)
		},
	}
	cmd.addSub(newSignerPolicyCmd(cfg))
	cmd.addSub(newSignerAgentCmd(cfg))
	return cmd
}

// ─── signer agent — §11.1 lifecycle registry ───

func newSignerAgentCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "agent",
		Short: "Agent lifecycle registry (spec §11.1): enrolled → active → suspended → retired",
		Long: `Manage the custodian's agent lifecycle registry. Enrolled agents hold no
authority until a mandate names them; suspending an agent refuses further
requests without deleting history; retiring additionally revokes every
mandate naming the agent. The custodian daemon must be running.`,
	}
	cmd.addSub(newSignerAgentEnrollCmd(cfg))
	cmd.addSub(newSignerAgentListCmd(cfg))
	cmd.addSub(newSignerAgentSetCmd(cfg, "suspend", "Refuse further requests from the agent (history kept)"))
	cmd.addSub(newSignerAgentSetCmd(cfg, "reactivate", "Return a suspended agent to active"))
	cmd.addSub(newSignerAgentSetCmd(cfg, "retire", "Retire the agent and revoke all its mandates (terminal)"))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

func newSignerAgentEnrollCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("signer agent enroll", flag.ContinueOnError)
	did := fs.String("did", "", "Agent did:key to register (required)")
	profile := fs.String("profile", "", "Tool profile bound to this agent (informational)")
	note := fs.String("note", "", "Operator note")
	return &Command{
		Name:  "enroll",
		Short: "Register an agent identity (state: enrolled)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if strings.TrimSpace(*did) == "" || !strings.HasPrefix(*did, "did:") {
				return fmt.Errorf("--did must be a did:key identifier")
			}
			rw, err := custodianRemote()
			if err != nil {
				return err
			}
			rec, err := rw.AgentEnroll(*did, *profile, *note)
			if err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Agent enrolled: %s", rec.DID))
			ui.KV("State", rec.State)
			return nil
		},
	}
}

func newSignerAgentListCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "list",
		Short: "List registered agents and their lifecycle state",
		Run: func(c *Command, args []string) error {
			rw, err := custodianRemote()
			if err != nil {
				return err
			}
			agents, err := rw.AgentList()
			if err != nil {
				return err
			}
			if len(agents) == 0 {
				ui.Info("No agents enrolled")
				return nil
			}
			for _, a := range agents {
				extra := ""
				if a.Profile != "" {
					extra = " profile:" + a.Profile
				}
				ui.KV(a.State, fmt.Sprintf("%s%s (enrolled %s)", a.DID, extra, a.EnrolledAt))
			}
			return nil
		},
	}
}

func newSignerAgentSetCmd(cfg *config.Config, name, desc string) *Command {
	fs := flag.NewFlagSet("signer agent "+name, flag.ContinueOnError)
	account := fs.String("account", "", "Wallet whose passkey authorizes activation (default: active wallet)")

	return &Command{
		Name:  name,
		Short: desc,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if len(args) < 1 || !strings.HasPrefix(args[0], "did:") {
				return fmt.Errorf("usage: signer agent %s <did:key>", name)
			}
			rw, err := custodianRemote()
			if err != nil {
				return err
			}
			acct := ""
			if name == "reactivate" {
				// Reactivation GRANTS authority — bound passkey ceremony on
				// the operator's wallet (specula-lite F5).
				acct, err = resolvePasskeyTarget(wallet.NewService(), *account)
				if err != nil {
					return err
				}
			}
			if err := rw.AgentSetStatus(args[0], name, acct, wallet.OpenCeremony); err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Agent %s → %s", args[0], name))
			if name == "retire" {
				ui.Info("All mandates naming this agent were revoked.")
			}
			return nil
		},
	}
}

func newSignerPolicyCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "policy",
		Short: "Custodian policy approvals (spec §11.3)",
	}
	cmd.addSub(newSignerPolicyApproveCmd(cfg))
	cmd.addSub(newSignerPolicyStatusCmd(cfg))
	return cmd
}

func newSignerPolicyApproveCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("signer policy approve", flag.ContinueOnError)
	policy := fs.String("policy", "", "Policy file path (default: state/policy.json)")

	return &Command{
		Name:  "approve",
		Short: "Record operator approval for the current policy",
		Long: `Approve the custodian policy as it stands on disk. The daemon refuses
to start when policy.json widens authority without a matching approval —
approving records the version hash and updates the approved snapshot.`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			policyPath := *policy
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			if policyPath == "" {
				policyPath = dir + "/policy.json"
			}
			approver := os.Getenv("USER")
			ap, err := custodian.ApprovePolicy(policyPath, dir, approver)
			if err != nil {
				return err
			}
			ui.Success(fmt.Sprintf("Policy %s approved (hash %s)", ap.Version, ap.Hash))
			return nil
		},
	}
}

func newSignerPolicyStatusCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "status",
		Short: "Show current vs approved policy state",
		Run: func(c *Command, args []string) error {
			dir, err := config.StateDir()
			if err != nil {
				return err
			}
			cur, err := os.ReadFile(dir + "/policy.json")
			if err != nil {
				return fmt.Errorf("no policy.json — daemon writes one on first run")
			}
			var curP custodian.Policy
			if err := json.Unmarshal(cur, &curP); err != nil {
				return err
			}
			approved, last := custodian.ApprovedPolicy(dir)
			ui.KV("Policy version", curP.Version)
			if approved == nil {
				ui.Warn("No approved policy snapshot — daemon will self-approve on first run")
				return nil
			}
			ui.KV("Approved version", approved.Version)
			if last != nil {
				ui.KV("Approved by", fmt.Sprintf("%s at %s", last.Approver, last.ApprovedAt))
			}
			if diffs := custodian.PolicyWidening(approved, &curP); len(diffs) > 0 {
				ui.Warn("Unapproved widening changes:")
				for _, d := range diffs {
					fmt.Fprintf(os.Stderr, "  - %s\n", d)
				}
				ui.Info("Run `stellar-go-cli signer policy approve` to approve")
			} else {
				ui.Success("No unapproved widening changes")
			}
			return nil
		},
	}
}
