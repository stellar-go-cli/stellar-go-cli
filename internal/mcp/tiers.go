package mcp

import "fmt"

// Tool risk tiers per docs/audit/agent-policy-engine.md §7.4. The tier map —
// not tool annotations or descriptions — is the authoritative classification.
type toolTier int

const (
	tierT0 toolTier = iota // read-only query
	tierT1                 // low-impact mutation, no keys
	tierT2                 // secret/key/security lifecycle
	tierT3                 // value transfer / on-chain signing
)

// toolTiers classifies every registrable tool by its highest reachable tier —
// a tool whose tier depends on arguments (e.g. system_network read vs switch)
// is classified at the most privileged form. A tool absent from this map is
// unclassified and fails closed: applyProfile refuses to register it.
var toolTiers = map[string]toolTier{
	// T0 — read-only queries.
	"wallet_list":                    tierT0,
	"wallet_show":                    tierT0,
	"wallet_balance":                 tierT0,
	"wallet_assets":                  tierT0,
	"swap_quote":                     tierT0,
	"swap_assets":                    tierT0,
	"swap_scan":                      tierT0,
	"swap_monitor":                   tierT0,
	"swap_arbitrage_scan":            tierT0,
	"swap_arbitrage_all":             tierT0,
	"asset_list":                     tierT0,
	"asset_info":                     tierT0,
	"asset_score":                    tierT0,
	"asset_show":                     tierT0,
	"pay_request":                    tierT0,
	"pay_history":                    tierT0,
	"pay_quote":                      tierT0,
	"pay_rails":                      tierT0,
	"system_status":                  tierT0,
	"system_health":                  tierT0,
	"memory_get":                     tierT0,
	"memory_list":                    tierT0,
	"memory_search":                  tierT0,
	"tansu_list_projects":            tierT0,
	"tansu_get_project":              tierT0,
	"tansu_get_proposals":            tierT0,
	"tansu_get_proposal":             tierT0,
	"tansu_get_member":               tierT0,
	"tansu_get_commit":               tierT0,
	"tansu_get_evidence":             tierT0,
	"tansu_get_attestation_finality": tierT0,

	// T1 — low-impact mutations without key access. system_network is T1
	// because it can persist a network switch; its read form is collateral.
	"wallet_rename":  tierT1,
	"wallet_fund":    tierT1,
	"memory_save":    tierT1,
	"memory_delete":  tierT1,
	"system_network": tierT1,

	// T2 — secret/key/security lifecycle. wallet_import/wallet_export are
	// deliberately unlisted: they are operator CLI operations and must not
	// be agent-reachable under any profile.
	"wallet_connect": tierT2,
	"wallet_switch":  tierT2,
	"wallet_remove":  tierT2,
	"wallet_passkey": tierT2,

	// T3 — value transfer / on-chain signing. Classified at the executing
	// form even where a dry-run variant exists.
	"pay_send":         tierT3,
	"pay_x402":         tierT3,
	"pay_zk":           tierT3,
	"swap_execute":     tierT3,
	"swap_triangular":  tierT3,
	"asset_trust":      tierT3,
	"asset_create_ft":  tierT3,
	"asset_create_nfa": tierT3,
	"asset_carbon":     tierT3,
}

// profileTiers maps a --profile name to the tiers it exposes, per spec §9.4.
var profileTiers = map[string]map[toolTier]bool{
	"readonly": {tierT0: true},
	"transact": {tierT0: true, tierT1: true, tierT3: true},
	"admin":    {tierT0: true, tierT1: true, tierT2: true, tierT3: true},
}

// defaultProfile preserves the pre-profile tool surface (all tiers).
const defaultProfile = "admin"

// OperatorOptions carries startup-time, operator-controlled policy switches —
// values an agent may never influence through tool arguments (spec §9.4/§9.5).
type OperatorOptions struct {
	// Profile selects the tool exposure profile: readonly | transact | admin.
	Profile string
	// X402RPCURL pins the Soroban RPC endpoint used by pay_x402. Empty uses
	// the default endpoint for the selected network.
	X402RPCURL string
	// X402MaxAtomicAmount is the operator hard spend cap for pay_x402 in
	// token atomic units. The effective cap is min(request, operator).
	X402MaxAtomicAmount string
	// AllowPrivateResources permits pay_x402 to fetch resource URLs resolving
	// to private/local addresses. Defaults to deny (SSRF guard active).
	AllowPrivateResources bool
	// AllowedOrigins lists extra Origin/Host values the HTTP transport
	// accepts beyond loopback and the bind host — DNS-rebinding defense.
	AllowedOrigins []string
	// SignerSocket is the custodian daemon's socket path. The gatekeeper
	// holds no key material — all signing and wallet state go through it.
	SignerSocket string
}

// ApplyOperatorOptions applies the operator's startup policy switches.
func (s *Server) ApplyOperatorOptions(opts OperatorOptions) error {
	if err := s.SetProfile(opts.Profile); err != nil {
		return err
	}
	s.x402RPCURL = opts.X402RPCURL
	s.x402MaxAtomicAmount = opts.X402MaxAtomicAmount
	s.allowPrivateResources = opts.AllowPrivateResources
	s.allowedOrigins = opts.AllowedOrigins
	s.custodianSock = opts.SignerSocket
	return nil
}

// SetProfile selects the least-privilege tool profile for this server — an
// operator choice at startup, never an agent request.
func (s *Server) SetProfile(name string) error {
	if name == "" {
		name = defaultProfile
	}
	if _, ok := profileTiers[name]; !ok {
		return fmt.Errorf("unknown tool profile %q (valid: readonly, transact, admin)", name)
	}
	s.profile = name
	return nil
}

// applyProfile drops tools whose tier the profile does not expose and —
// fail-closed — any tool missing from the tier map entirely.
func (s *Server) applyProfile() {
	profile := s.profile
	if profile == "" {
		profile = defaultProfile
	}
	allowed := profileTiers[profile]
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.tools {
		tier, classified := toolTiers[name]
		if !classified {
			s.logger.Printf("tool %q has no risk-tier classification — refusing to register it\n", name)
			delete(s.tools, name)
			delete(s.handlers, name)
			continue
		}
		if !allowed[tier] {
			delete(s.tools, name)
			delete(s.handlers, name)
		}
	}
	s.logger.Printf("Tool profile %q: %d tools exposed\n", profile, len(s.tools))
}
