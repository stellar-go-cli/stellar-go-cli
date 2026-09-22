package integrations

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/soroban"
	"github.com/stellar/go/xdr"
	"golang.org/x/crypto/sha3"
)

// Tansu testnet contract ID
const TansuTestnetContractID = "CBXKUSLQPVF35FYURR5C42BPYA5UOVDXX2ELKIM2CAJMCI6HXG2BHGZA"

// TansuClient wraps soroban.Client for read-only Tansu contract queries.
type TansuClient struct {
	client     *soroban.Client
	contractID string
}

// NewTansuClient creates a new Tansu client.
func NewTansuClient(rpcURL, passphrase, contractID string) *TansuClient {
	return &TansuClient{
		client:     soroban.NewClient(rpcURL, passphrase),
		contractID: contractID,
	}
}

// NewTansuClientForNetwork creates a Tansu client for the given network.
func NewTansuClientForNetwork(network, contractID string) *TansuClient {
	return &TansuClient{
		client:     soroban.NewClientForNetwork(network),
		contractID: contractID,
	}
}

// Close closes the underlying soroban client.
func (t *TansuClient) Close() error {
	return t.client.Close()
}

// ─── VersioningTrait read-only functions ────────────────────────

// GetProjects returns a page of registered projects.
func (t *TansuClient) GetProjects(ctx context.Context, page uint32) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_projects", []xdr.ScVal{
		soroban.ScvU32(page),
	})
}

// GetProject returns project information for the given project key.
func (t *TansuClient) GetProject(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_project", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// GetCommit returns the latest commit hash for a project.
func (t *TansuClient) GetCommit(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_commit", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// GetEvidence returns evidence history for a specific project commit and kind.
func (t *TansuClient) GetEvidence(ctx context.Context, projectKey []byte, commitHash, kind string) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_evidence", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvString(commitHash),
		soroban.ScvSymbol(kind),
	})
}

// GetSubProjects returns sub-projects for a project.
func (t *TansuClient) GetSubProjects(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_sub_projects", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// GetAttestationThreshold returns the attestation finality threshold for a project.
func (t *TansuClient) GetAttestationThreshold(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_attestation_threshold", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// GetAttestationFinality returns the finality status for a project commit and target.
func (t *TansuClient) GetAttestationFinality(ctx context.Context, projectKey []byte, commitHash, target string) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_attestation_finality", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvString(commitHash),
		soroban.ScvSymbol(target),
	})
}

// GetAttestations returns attestations for a project commit and target.
func (t *TansuClient) GetAttestations(ctx context.Context, projectKey []byte, commitHash, target string) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_attestations", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvString(commitHash),
		soroban.ScvSymbol(target),
	})
}

// ─── MembershipTrait read-only functions ───────────────────────

// GetMember returns member information for the given address.
func (t *TansuClient) GetMember(ctx context.Context, address string) (string, error) {
	addr, err := soroban.AccountToScAddress(address)
	if err != nil {
		return "", fmt.Errorf("invalid address: %w", err)
	}
	return t.client.SimulateOnly(ctx, t.contractID, "get_member", []xdr.ScVal{
		soroban.ScvAddress(addr),
	})
}

// GetBadges returns badge holders for a project.
func (t *TansuClient) GetBadges(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_badges", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// GetMaxWeight returns the maximum voting weight for a member in a project.
func (t *TansuClient) GetMaxWeight(ctx context.Context, projectKey []byte, address string) (string, error) {
	addr, err := soroban.AccountToScAddress(address)
	if err != nil {
		return "", fmt.Errorf("invalid address: %w", err)
	}
	return t.client.SimulateOnly(ctx, t.contractID, "get_max_weight", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvAddress(addr),
	})
}

// ─── DaoTrait read-only functions ──────────────────────────────

// GetDao returns a page of proposals for a project.
func (t *TansuClient) GetDao(ctx context.Context, projectKey []byte, page uint32) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_dao", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvU32(page),
	})
}

// GetProposal returns a single proposal by ID.
func (t *TansuClient) GetProposal(ctx context.Context, projectKey []byte, proposalID uint32) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_proposal", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvU32(proposalID),
	})
}

// GetConflictOfInterest returns addresses barred from voting on a proposal.
func (t *TansuClient) GetConflictOfInterest(ctx context.Context, projectKey []byte, proposalID uint32) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_conflict_of_interest", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
		soroban.ScvU32(proposalID),
	})
}

// GetAnonymousVotingConfig returns the anonymous voting config for a project.
func (t *TansuClient) GetAnonymousVotingConfig(ctx context.Context, projectKey []byte) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_anonymous_voting_config", []xdr.ScVal{
		soroban.ScvBytes(projectKey),
	})
}

// ─── TansuTrait read-only functions ────────────────────────────

// GetAdminsConfig returns the admin configuration.
func (t *TansuClient) GetAdminsConfig(ctx context.Context) (string, error) {
	return t.client.SimulateOnly(ctx, t.contractID, "get_admins_config", nil)
}

// ─── Helpers ───────────────────────────────────────────────────

// HexToProjectKey decodes a hex string to a project key byte slice.
func HexToProjectKey(hexStr string) ([]byte, error) {
	key, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid project key hex: %w", err)
	}
	return key, nil
}

// ProjectNameToKey computes the keccak256 hash of a project name,
// matching the Tansu contract's project key derivation.
func ProjectNameToKey(name string) ([]byte, error) {
	if len(name) > 30 {
		return nil, fmt.Errorf("project name too long (max 30 chars)")
	}
	for _, b := range []byte(name) {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && (b < '0' || b > '9') {
			return nil, fmt.Errorf("project name contains invalid characters (only alphanumeric allowed)")
		}
	}
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(name))
	return h.Sum(nil), nil
}

// ResolveProjectKey resolves a project key from either a hex key or a name.
// If hexKey is non-empty, it decodes the hex. Otherwise, it computes keccak256(name).
func ResolveProjectKey(hexKey, name string) ([]byte, error) {
	if hexKey != "" {
		return HexToProjectKey(hexKey)
	}
	if name != "" {
		return ProjectNameToKey(name)
	}
	return nil, fmt.Errorf("either --key or --name must be provided")
}
