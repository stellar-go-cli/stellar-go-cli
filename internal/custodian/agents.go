// agents.go — the §11.1 agent lifecycle registry. Every agent DID known to
// the custodian carries a state: enrolled → active → suspended → retired.
// Suspension refuses further requests without deleting history; retirement
// additionally revokes every mandate naming the agent. The registry is
// custodian-owned — the gatekeeper's did:key identity itself is managed by
// internal/mcp/agent_identity.go.
package custodian

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/signsock"
)

// Agent lifecycle states (§11.1).
const (
	agentEnrolled  = "enrolled"
	agentActive    = "active"
	agentSuspended = "suspended"
	agentRetired   = "retired"
)

// validAgentTransition reports whether from→to is a legal lifecycle move.
func validAgentTransition(from, to string) bool {
	switch from {
	case agentEnrolled:
		return to == agentActive || to == agentRetired
	case agentActive:
		return to == agentSuspended || to == agentRetired
	case agentSuspended:
		return to == agentActive || to == agentRetired
	}
	return false // retired is terminal; unknown states reject everything
}

type agentRegistry struct {
	mu     sync.Mutex
	path   string
	agents map[string]*signsock.AgentRecord
}

func agentsPath(stateDir string) string { return filepath.Join(stateDir, "agents.json") }

// loadAgentRegistry reads state/agents.json; a missing file starts empty.
func loadAgentRegistry(stateDir string) (*agentRegistry, error) {
	r := &agentRegistry{path: agentsPath(stateDir), agents: map[string]*signsock.AgentRecord{}}
	data, err := os.ReadFile(r.path) // #nosec G304 -- fixed name inside state dir
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("read agents: %w", err)
	}
	var list []*signsock.AgentRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse agents: %w", err)
	}
	for _, a := range list {
		r.agents[a.DID] = a
	}
	return r, nil
}

func (r *agentRegistry) save() error {
	list := make([]*signsock.AgentRecord, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, a)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// enroll registers a DID in state enrolled, or promotes an existing
// enrolled/suspended record when activate is set.
func (r *agentRegistry) enroll(did, profile, note string, activate bool) (*signsock.AgentRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	a, ok := r.agents[did]
	if !ok {
		a = &signsock.AgentRecord{DID: did, State: agentEnrolled, EnrolledAt: now}
		r.agents[did] = a
	}
	if a.State == agentRetired {
		return nil, fmt.Errorf("agent %s is retired — enroll a new identity instead", did)
	}
	if profile != "" {
		a.Profile = profile
	}
	if note != "" {
		a.Note = note
	}
	if activate && a.State != agentActive {
		a.State = agentActive
	}
	a.UpdatedAt = now
	if err := r.save(); err != nil {
		return nil, err
	}
	return a, nil
}

// stateOf returns the lifecycle state for a DID, or "" when unknown.
func (r *agentRegistry) stateOf(did string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[did]; ok {
		return a.State
	}
	return ""
}

// setStatus drives a transition; retire cascades to the agent's mandates.
func (d *Daemon) setAgentStatus(did, to string) error {
	d.agents.mu.Lock()
	a, ok := d.agents.agents[did]
	if !ok {
		d.agents.mu.Unlock()
		return fmt.Errorf("unknown agent %s — enroll it first", did)
	}
	from := a.State
	if from == to {
		d.agents.mu.Unlock()
		return nil // idempotent
	}
	if !validAgentTransition(from, to) {
		d.agents.mu.Unlock()
		return fmt.Errorf("invalid transition %s → %s", from, to)
	}
	a.State = to
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	err := d.agents.save()
	d.agents.mu.Unlock()
	if err != nil {
		return err
	}
	if to == agentRetired {
		// §11.1: retirement revokes credentials and dependent mandates.
		for _, m := range d.mandates.list("") {
			id, _ := m["id"].(string)
			agent, _ := m["agent"].(string)
			if agent == did && id != "" && m["status"] != mandateRevoked {
				d.applyMandateStatus(id, mandateRevoked, "")
			}
		}
	}
	return nil
}

func (r *agentRegistry) list() []*signsock.AgentRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*signsock.AgentRecord, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}
