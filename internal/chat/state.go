// Package chat provides an interactive chat interface for Stellar Go CLI
package chat

import (
	"sync"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/triangular"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// State manages the chat session state including pending operations,
// wallet cache, and current network context
type State struct {
	mu sync.RWMutex

	// Current network context
	CurrentNetwork string

	// Pending operations
	CurrentOperation               *Operation
	PendingSwap                    *SwapQuote
	PendingTriangularOpportunities []triangular.TriangularResult
	SelectedTriangularIndex        int // -1 if none selected

	// Wallet cache
	cachedWallets  []models.WalletEntry
	cacheTimestamp time.Time
	cacheValid     bool
	cacheDuration  time.Duration

	// Memory storage for user preferences and context
	memories map[string]string

	// Running state
	Running bool
}

// Operation represents a pending operation requiring user input
type Operation struct {
	ToolName       string
	MissingParams  []ParamInfo
	ProvidedParams map[string]interface{}
	Timestamp      time.Time
}

// ParamInfo describes a parameter that needs to be collected
type ParamInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Examples    []string `json:"examples,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// SwapQuote stores the exact swap quote data for execution
type SwapQuote struct {
	From        string            `json:"from"`
	To          string            `json:"to"`
	Amount      string            `json:"amount"`
	Type        string            `json:"type"`
	Rate        string            `json:"rate,omitempty"`
	Estimated   string            `json:"estimated,omitempty"`
	MinReceived string            `json:"min_received,omitempty"`
	PriceImpact float64           `json:"price_impact,omitempty"`
	Paths       []models.SwapPath `json:"paths,omitempty"`
}

// NewState creates a new chat state with default values
func NewState(cfg *config.Config) *State {
	network := cfg.Network
	if network == "" {
		network = "stellar-testnet" // fallback if config is empty
	}
	return &State{
		CurrentNetwork: network,
		Running:        true,
		cacheDuration:  30 * time.Second,
		cachedWallets:  make([]models.WalletEntry, 0),
		memories:       make(map[string]string),
	}
}

// SetNetwork changes the current network and invalidates wallet cache
func (s *State) SetNetwork(network string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentNetwork = network
	s.invalidateCache()
}

// GetNetwork returns the current network
func (s *State) GetNetwork() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentNetwork
}

// SetPendingOperation stores an operation awaiting parameters
func (s *State) SetPendingOperation(op *Operation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentOperation = op
}

// GetPendingOperation retrieves the current pending operation
func (s *State) GetPendingOperation() *Operation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentOperation
}

// ClearPendingOperation removes the pending operation
func (s *State) ClearPendingOperation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentOperation = nil
}

// SetPendingSwap stores swap quote data for execution
func (s *State) SetPendingSwap(quote *SwapQuote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingSwap = quote
}

// GetPendingSwap retrieves the pending swap quote
func (s *State) GetPendingSwap() *SwapQuote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PendingSwap
}

// ClearPendingSwap removes the pending swap
func (s *State) ClearPendingSwap() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingSwap = nil
}

// HasPendingSwap returns true if there's a swap awaiting execution
func (s *State) HasPendingSwap() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PendingSwap != nil
}

// UpdateWalletCache stores the wallet list with timestamp
func (s *State) UpdateWalletCache(wallets []models.WalletEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedWallets = wallets
	s.cacheTimestamp = time.Now()
	s.cacheValid = true
}

// GetCachedWallets returns cached wallets if still valid
func (s *State) GetCachedWallets() ([]models.WalletEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.cacheValid {
		return nil, false
	}

	if time.Since(s.cacheTimestamp) > s.cacheDuration {
		s.cacheValid = false
		return nil, false
	}

	return s.cachedWallets, true
}

// InvalidateWalletCache clears the wallet cache
func (s *State) InvalidateWalletCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateCache()
}

func (s *State) invalidateCache() {
	s.cacheValid = false
	s.cachedWallets = make([]models.WalletEntry, 0)
}

// GetActiveWallet returns the active wallet for the current network
func (s *State) GetActiveWallet() *models.WalletEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	network := models.Network(s.CurrentNetwork)

	// Return first wallet on this network (as a copy to prevent slice modification)
	for i := range s.cachedWallets {
		if s.cachedWallets[i].Network == network {
			// Return a copy, not a pointer to the slice element
			copy := s.cachedWallets[i]
			return &copy
		}
	}

	return nil
}

// IsRunning returns the running state
func (s *State) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Running
}

// Stop sets running to false
func (s *State) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = false
}

// SetPendingTriangularOpportunities stores scan results and resets selection
func (s *State) SetPendingTriangularOpportunities(results []triangular.TriangularResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingTriangularOpportunities = results
	s.SelectedTriangularIndex = -1
}

// GetPendingTriangularOpportunities returns stored opportunities
func (s *State) GetPendingTriangularOpportunities() []triangular.TriangularResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.PendingTriangularOpportunities
}

// HasPendingTriangularOpportunities checks if there are stored opportunities
func (s *State) HasPendingTriangularOpportunities() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.PendingTriangularOpportunities) > 0
}

// SelectTriangularOpportunity sets the selected opportunity index
func (s *State) SelectTriangularOpportunity(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.PendingTriangularOpportunities) {
		return false
	}
	s.SelectedTriangularIndex = index
	return true
}

// GetSelectedTriangularOpportunity returns the selected opportunity
func (s *State) GetSelectedTriangularOpportunity() *triangular.TriangularResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.SelectedTriangularIndex < 0 || s.SelectedTriangularIndex >= len(s.PendingTriangularOpportunities) {
		return nil
	}
	result := s.PendingTriangularOpportunities[s.SelectedTriangularIndex]
	return &result
}

// ClearPendingTriangularOpportunities clears stored opportunities
func (s *State) ClearPendingTriangularOpportunities() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingTriangularOpportunities = nil
	s.SelectedTriangularIndex = -1
}

// SaveMemory stores a key-value pair in memory
func (s *State) SaveMemory(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memories[key] = value
}

// GetMemory retrieves a value from memory by key
func (s *State) GetMemory(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.memories[key]
	return val, ok
}

// ListMemories returns all stored memory keys
func (s *State) ListMemories() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.memories))
	for k := range s.memories {
		keys = append(keys, k)
	}
	return keys
}

// DeleteMemory removes a key from memory
func (s *State) DeleteMemory(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.memories, key)
}
