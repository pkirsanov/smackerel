package connector

import (
	"context"
	"fmt"
	"sort"
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
// Close() is called outside the lock to prevent blocking concurrent reads.
// A panic in Close() is recovered so a buggy connector cannot crash the caller.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	c, exists := r.connectors[id]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("connector %q not registered", id)
	}
	delete(r.connectors, id)
	r.mu.Unlock()

	var closeErr error
	func() {
		defer func() {
			if p := recover(); p != nil {
				closeErr = fmt.Errorf("close connector %q panicked: %v", id, p)
			}
		}()
		if err := c.Close(); err != nil {
			closeErr = fmt.Errorf("close connector %q: %w", id, err)
		}
	}()
	return closeErr
}

// Get returns a registered connector by ID.
func (r *Registry) Get(id string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.connectors[id]
	return c, ok
}

// List returns all registered connector IDs in sorted order.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.connectors))
	for id := range r.connectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Count returns the number of registered connectors.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connectors)
}

// ListConnectorHealth returns a map of connector ID → health status string.
// Snapshots connectors under lock, then calls Health() concurrently outside
// the lock so latency is O(slowest) instead of O(sum). A slow or blocking
// Health() implementation cannot stall Register/Unregister.
func (r *Registry) ListConnectorHealth(ctx context.Context) map[string]string {
	r.mu.RLock()
	snapshot := make(map[string]Connector, len(r.connectors))
	for id, c := range r.connectors {
		snapshot[id] = c
	}
	r.mu.RUnlock()

	if len(snapshot) == 0 {
		return map[string]string{}
	}

	type healthResult struct {
		id     string
		status string
	}

	ch := make(chan healthResult, len(snapshot))
	for id, c := range snapshot {
		go func(id string, c Connector) {
			ch <- healthResult{id: id, status: string(c.Health(ctx))}
		}(id, c)
	}

	result := make(map[string]string, len(snapshot))
	for range snapshot {
		hr := <-ch
		result[hr.id] = hr.status
	}
	return result
}
