package notification

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type RegisteredSource struct {
	Key     SourceKey
	Adapter SourceAdapter
}

type SourceRegistry struct {
	mu         sync.RWMutex
	byInstance map[string]RegisteredSource
	byKey      map[SourceKey]RegisteredSource
}

func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{byInstance: make(map[string]RegisteredSource), byKey: make(map[SourceKey]RegisteredSource)}
}

func (r *SourceRegistry) Register(adapter SourceAdapter) error {
	if r == nil {
		return fmt.Errorf("notification source registry: registry is required")
	}
	if adapter == nil {
		return fmt.Errorf("notification source registry: adapter is required")
	}
	key := SourceKey{
		SourceType:       strings.TrimSpace(adapter.SourceType()),
		SourceInstanceID: strings.TrimSpace(adapter.InstanceID()),
		SourceForm:       adapter.SourceForm(),
	}
	if key.SourceType == "" {
		return fmt.Errorf("notification source registry: source type is required")
	}
	if key.SourceInstanceID == "" {
		return fmt.Errorf("notification source registry: source instance id is required")
	}
	if !key.SourceForm.Valid() {
		return fmt.Errorf("notification source registry: source form %q is invalid", key.SourceForm)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byInstance[key.SourceInstanceID]; ok {
		return fmt.Errorf("notification source registry: source instance id %q already registered for %s/%s", key.SourceInstanceID, existing.Key.SourceType, existing.Key.SourceForm)
	}
	if existing, ok := r.byKey[key]; ok {
		return fmt.Errorf("notification source registry: source key %s/%s/%s already registered as %T", key.SourceType, key.SourceInstanceID, key.SourceForm, existing.Adapter)
	}
	record := RegisteredSource{Key: key, Adapter: adapter}
	r.byInstance[key.SourceInstanceID] = record
	r.byKey[key] = record
	return nil
}

func (r *SourceRegistry) List() []RegisteredSource {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]RegisteredSource, 0, len(r.byKey))
	for _, item := range r.byKey {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].Key.SourceInstanceID + "\x00" + items[i].Key.SourceType + "\x00" + string(items[i].Key.SourceForm)
		right := items[j].Key.SourceInstanceID + "\x00" + items[j].Key.SourceType + "\x00" + string(items[j].Key.SourceForm)
		return left < right
	})
	return items
}

func (r *SourceRegistry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byKey)
}
