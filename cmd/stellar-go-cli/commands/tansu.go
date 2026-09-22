package commands

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/integrations"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

func newIntTansuCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "tansu",
		Short: "Query Tansu project governance & versioning on Stellar",
		Long:  "Read-only queries against the Tansu Soroban contract: list projects, get proposals, check attestations, view members and badges.",
		cfg:   cfg,
	}
	cmd.addSub(newIntTansuListCmd(cfg))
	cmd.addSub(newIntTansuShowCmd(cfg))
	cmd.addSub(newIntTansuCommitCmd(cfg))
	cmd.addSub(newIntTansuEvidenceCmd(cfg))
	cmd.addSub(newIntTansuSubProjectsCmd(cfg))
	cmd.addSub(newIntTansuThresholdCmd(cfg))
	cmd.addSub(newIntTansuFinalityCmd(cfg))
	cmd.addSub(newIntTansuAttestationsCmd(cfg))
	cmd.addSub(newIntTansuProposalsCmd(cfg))
	cmd.addSub(newIntTansuProposalCmd(cfg))
	cmd.addSub(newIntTansuCoiCmd(cfg))
	cmd.addSub(newIntTansuMemberCmd(cfg))
	cmd.addSub(newIntTansuBadgesCmd(cfg))
	cmd.addSub(newIntTansuWeightCmd(cfg))
	cmd.addSub(newIntTansuAnonConfigCmd(cfg))
	cmd.addSub(newIntTansuAdminsCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// tansuContractID resolves the Tansu contract ID from config or testnet default.
func tansuContractID(cfg *config.Config) string {
	if cfg.Integrations.TansuContractID != "" {
		return cfg.Integrations.TansuContractID
	}
	return integrations.TansuTestnetContractID
}

// tansuNetwork resolves the network from config or testnet default.
func tansuNetwork(cfg *config.Config) string {
	if cfg.Network != "" {
		return cfg.Network
	}
	return "stellar-testnet"
}

// newTansuClient creates a TansuClient from config.
func newTansuClient(cfg *config.Config) *integrations.TansuClient {
	return integrations.NewTansuClientForNetwork(tansuNetwork(cfg), tansuContractID(cfg))
}

// resolveKey resolves a project key from --key (hex) or --name.
func resolveKey(keyFlag, nameFlag string) ([]byte, error) {
	return integrations.ResolveProjectKey(keyFlag, nameFlag)
}

// ─── integrations tansu list ────────────────────

func newIntTansuListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-list", flag.ContinueOnError)
	page := fs.Uint("page", 0, "Page number (0-indexed)")

	return &Command{
		Name:  "list",
		Short: "List registered Tansu projects (paginated)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Tansu Projects")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetProjects(ctx, uint32(*page))
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel(fmt.Sprintf("Page %d — Raw XDR:", *page))
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu show ────────────────────

func newIntTansuShowCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-show", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "show",
		Short: "Show project details",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Project")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetProject(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu commit ──────────────────

func newIntTansuCommitCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-commit", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "commit",
		Short: "Get latest commit hash for a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Commit")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetCommit(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu evidence ────────────────

func newIntTansuEvidenceCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-evidence", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	commit := fs.String("commit", "", "Commit hash (required)")
	kind := fs.String("kind", "Sbom", "Evidence kind: Sbom | Cve | Attestation")

	return &Command{
		Name:  "evidence",
		Short: "Get evidence history for a project commit",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *commit == "" {
				ui.Error("--commit is required")
				return fmt.Errorf("--commit required")
			}

			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Evidence")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetEvidence(ctx, projectKey, *commit, *kind)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu sub-projects ────────────

func newIntTansuSubProjectsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-sub-projects", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "sub-projects",
		Short: "List sub-projects for a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Sub-Projects")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetSubProjects(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu threshold ───────────────

func newIntTansuThresholdCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-threshold", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "threshold",
		Short: "Get attestation finality threshold for a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Attestation Threshold")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetAttestationThreshold(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu finality ────────────────

func newIntTansuFinalityCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-finality", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	commit := fs.String("commit", "", "Commit hash (required)")
	target := fs.String("target", "Commit", "Attestation target: Commit")

	return &Command{
		Name:  "finality",
		Short: "Get attestation finality status for a project commit",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *commit == "" {
				ui.Error("--commit is required")
				return fmt.Errorf("--commit required")
			}

			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Attestation Finality")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetAttestationFinality(ctx, projectKey, *commit, *target)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu attestations ────────────

func newIntTansuAttestationsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-attestations", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	commit := fs.String("commit", "", "Commit hash (required)")
	target := fs.String("target", "Commit", "Attestation target: Commit")

	return &Command{
		Name:  "attestations",
		Short: "List attestations for a project commit",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *commit == "" {
				ui.Error("--commit is required")
				return fmt.Errorf("--commit required")
			}

			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Attestations")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetAttestations(ctx, projectKey, *commit, *target)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu proposals ───────────────

func newIntTansuProposalsCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-proposals", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	page := fs.Uint("page", 0, "Page number (0-indexed)")

	return &Command{
		Name:  "proposals",
		Short: "List proposals for a project (paginated)",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Proposals")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetDao(ctx, projectKey, uint32(*page))
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel(fmt.Sprintf("Page %d — Raw XDR:", *page))
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu proposal ────────────────

func newIntTansuProposalCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-proposal", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	id := fs.Uint("id", 0, "Proposal ID")

	return &Command{
		Name:  "proposal",
		Short: "Get a single proposal by ID",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Proposal")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetProposal(ctx, projectKey, uint32(*id))
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu coi ─────────────────────

func newIntTansuCoiCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-coi", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	proposal := fs.Uint("proposal", 0, "Proposal ID")

	return &Command{
		Name:  "coi",
		Short: "Get conflict-of-interest addresses for a proposal",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Conflict of Interest")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetConflictOfInterest(ctx, projectKey, uint32(*proposal))
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu member ──────────────────

func newIntTansuMemberCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-member", flag.ContinueOnError)
	address := fs.String("address", "", "Member Stellar address (G...)")

	return &Command{
		Name:  "member",
		Short: "Get member information by address",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *address == "" {
				ui.Error("--address is required")
				return fmt.Errorf("--address required")
			}

			ui.Header("Tansu Member")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetMember(ctx, *address)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu badges ──────────────────

func newIntTansuBadgesCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-badges", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "badges",
		Short: "Get badge holders for a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Badges")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetBadges(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu weight ──────────────────

func newIntTansuWeightCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-weight", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")
	address := fs.String("address", "", "Member Stellar address (G...)")

	return &Command{
		Name:  "weight",
		Short: "Get voting weight for a member in a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *address == "" {
				ui.Error("--address is required")
				return fmt.Errorf("--address required")
			}

			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Voting Weight")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetMaxWeight(ctx, projectKey, *address)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu anon-config ─────────────

func newIntTansuAnonConfigCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("tansu-anon-config", flag.ContinueOnError)
	key := fs.String("key", "", "Project key (hex)")
	name := fs.String("name", "", "Project name (computes keccak256 key)")

	return &Command{
		Name:  "anon-config",
		Short: "Get anonymous voting config for a project",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			projectKey, err := resolveKey(*key, *name)
			if err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Header("Tansu Anonymous Voting Config")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetAnonymousVotingConfig(ctx, projectKey)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}

// ─── integrations tansu admins ──────────────────

func newIntTansuAdminsCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "admins",
		Short: "Get Tansu admin configuration",
		Run: func(c *Command, args []string) error {
			ui.Header("Tansu Admins Config")

			client := newTansuClient(cfg)
			defer client.Close() //nolint:errcheck // RPC client cleanup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.GetAdminsConfig(ctx)
			if err != nil {
				ui.Error(fmt.Sprintf("Query failed: %v", err))
				return fmt.Errorf("tansu query failed: %w", err)
			}

			ui.SectionLabel("Raw XDR:")
			fmt.Println(result)
			return nil
		},
	}
}
