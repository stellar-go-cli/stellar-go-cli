package exchange

import (
	"fmt"
	"sync"
)

// Registry manages all available exchanges
type Registry struct {
	exchanges map[string]Exchange
	mu        sync.RWMutex
}

// NewRegistry creates a new exchange registry
func NewRegistry() *Registry {
	return &Registry{
		exchanges: make(map[string]Exchange),
	}
}

// Register adds an exchange to the registry
func (r *Registry) Register(exchange Exchange) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := exchange.Name()
	if _, exists := r.exchanges[name]; exists {
		return fmt.Errorf("exchange %s already registered", name)
	}

	r.exchanges[name] = exchange
	return nil
}

// Get retrieves an exchange by name
func (r *Registry) Get(name string) (Exchange, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exchange, exists := r.exchanges[name]
	if !exists {
		return nil, fmt.Errorf("exchange %s not found", name)
	}

	return exchange, nil
}

// List returns all registered exchange names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.exchanges))
	for name := range r.exchanges {
		names = append(names, name)
	}

	return names
}

// ListByType returns exchanges filtered by type
func (r *Registry) ListByType(exchangeType ExchangeType) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0)
	for name, exchange := range r.exchanges {
		if exchange.Type() == exchangeType {
			names = append(names, name)
		}
	}

	return names
}

// Unregister removes an exchange from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.exchanges[name]; !exists {
		return fmt.Errorf("exchange %s not found", name)
	}

	delete(r.exchanges, name)
	return nil
}

// Has checks if an exchange is registered
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.exchanges[name]
	return exists
}

// GetAll returns all registered exchanges
func (r *Registry) GetAll() []Exchange {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Exchange, 0, len(r.exchanges))
	for _, exchange := range r.exchanges {
		result = append(result, exchange)
	}

	return result
}

// globalRegistry is the singleton registry instance
var globalRegistry *Registry
var once sync.Once

// GetRegistry returns the global registry instance
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = NewRegistry()
	})
	return globalRegistry
}

// ResetRegistry resets the global registry (useful for testing)
func ResetRegistry() {
	globalRegistry = NewRegistry()
}
