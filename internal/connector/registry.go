package connector

import (
	"fmt"
	"sync"
)

// Registry manages connector lifecycle.
type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
}

// NewRegistry creates a new connector registry.
func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]Connector),
	}
}

// Register adds a connector to the registry.
func (r *Registry) Register(c Connector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := c.ID()
	if _, exists := r.connectors[id]; exists {
		return fmt.Errorf("connector %q already registered", id)
	}

	r.connectors[id] = c
	return nil
}

// Unregister removes a connector from the registry.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, exists := r.connectors[id]
	if !exists {
		return fmt.Errorf("connector %q not registered", id)
	}

	if err := c.Close(); err != nil {
		return fmt.Errorf("close connector %q: %w", id, err)
	}

	delete(r.connectors, id)
	return nil
}

// Get returns a registered connector by ID.
func (r *Registry) Get(id string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.connectors[id]
	return c, ok
}

// List returns all registered connector IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.connectors))
	for id := range r.connectors {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of registered connectors.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connectors)
}
