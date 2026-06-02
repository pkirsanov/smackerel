package openknowledge

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Sentinel errors returned by Registry. Callers MUST compare with
// errors.Is rather than string-matching the messages.
var (
	// ErrUnknownTool is returned by Lookup when the requested name has
	// never been Registered.
	ErrUnknownTool = errors.New("openknowledge: unknown tool")
	// ErrDuplicateTool is returned by Register when a tool with the
	// same Name() has already been registered.
	ErrDuplicateTool = errors.New("openknowledge: duplicate tool registration")
	// ErrToolNotAllowed is returned by Lookup when the tool is
	// registered but excluded by the operator allowlist. A nil or
	// empty allowlist means deny-all.
	ErrToolNotAllowed = errors.New("openknowledge: tool not in allowlist")

	// ErrToolNotRegistered is the spec 076 SCOPE-2b name for the
	// registry "name was never Registered" sentinel. Alias of
	// ErrUnknownTool so callers using either name with errors.Is
	// keep working.
	ErrToolNotRegistered = ErrUnknownTool
	// ErrToolDisabled is the spec 076 SCOPE-2b name for the registry
	// "registered but operator-disabled (allowlist excludes it)"
	// sentinel. Alias of ErrToolNotAllowed.
	ErrToolDisabled = ErrToolNotAllowed
)

// Registry is the in-memory store of Tool implementations available
// to the agent loop. The allowlist is set at construction time from
// the SST config block assistant.open_knowledge.tool_allowlist; a
// nil or empty allowlist denies every tool. There is no implicit
// "allow all" mode by design (smackerel NO-DEFAULTS).
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	allowlist map[string]struct{}
}

// NewRegistry returns an empty Registry whose allowlist is the set of
// names supplied by the caller. A nil or empty slice is preserved
// verbatim, which means Lookup will refuse every tool until the
// operator widens the allowlist.
func NewRegistry(allowlist []string) *Registry {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = struct{}{}
	}
	return &Registry{
		tools:     make(map[string]Tool),
		allowlist: allowed,
	}
}

// Register adds a Tool to the registry. It rejects nil tools, tools
// whose Name() is empty, and duplicate names. Registration is
// independent of the allowlist: a tool may be Registered but excluded
// from Enabled()/Lookup() because the operator has not allowlisted it.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("%w: nil tool", ErrDuplicateTool)
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("%w: empty Name()", ErrDuplicateTool)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateTool, name)
	}
	r.tools[name] = t
	return nil
}

// Lookup returns the Tool registered under name, gated by the
// allowlist. Callers receive ErrUnknownTool for names that were never
// Registered and ErrToolNotAllowed for names that are Registered but
// not in the allowlist. Both errors are distinct so the agent loop
// can attribute refusal causes accurately.
func (r *Registry) Lookup(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}
	if _, allowed := r.allowlist[name]; !allowed {
		return nil, fmt.Errorf("%w: %q", ErrToolNotAllowed, name)
	}
	return t, nil
}

// Enabled returns the subset of Registered tools that the operator
// allowlist permits, sorted by Name() for deterministic output. The
// planner system prompt is built from this slice; deterministic
// ordering keeps prompts cache-stable across process restarts.
func (r *Registry) Enabled() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for name, t := range r.tools {
		if _, allowed := r.allowlist[name]; allowed {
			out = append(out, t)
		}
		_ = name
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
