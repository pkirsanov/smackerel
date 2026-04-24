// Tool registry for spec 037 Scope 2.
//
// Decentralized: tools register themselves from package init() in the
// package that owns the data they touch. There is NO central registration
// table. RegisterTool panics on duplicate name, missing fields, invalid
// side-effect class, or schemas that fail to compile — the binary refuses
// to start so misconfiguration cannot reach production.
//
// The registry is package-private; callers outside this package access it
// only through Has, ByName, and All. Schema bytes are defensively copied
// at registration so the source buffer cannot be mutated to alter
// validation behavior (BS-005).

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"sync"
)

// SideEffectClass classifies what a tool does to system state.
//
//	read     pure read of local state (Postgres, in-memory caches)
//	write    mutates local state
//	external calls a remote service or non-deterministic source
//
// A scenario's declared side_effect_class MUST be >= the highest class
// across its allowed_tools (read < write < external). The loader enforces
// this in Scope 3.
type SideEffectClass string

const (
	SideEffectRead     SideEffectClass = "read"
	SideEffectWrite    SideEffectClass = "write"
	SideEffectExternal SideEffectClass = "external"
)

// Rank returns 0 for read, 1 for write, 2 for external, -1 for unknown.
// Comparisons use Rank(); the order is fixed by spec 037 §3.2.
func (c SideEffectClass) Rank() int {
	switch c {
	case SideEffectRead:
		return 0
	case SideEffectWrite:
		return 1
	case SideEffectExternal:
		return 2
	default:
		return -1
	}
}

// Valid reports whether the value is one of the three known classes.
func (c SideEffectClass) Valid() bool { return c.Rank() >= 0 }

// AllSideEffectClasses returns the canonical ordered list of side-effect
// classes. Used by the exhaustive-switch test and by linters.
func AllSideEffectClasses() []SideEffectClass {
	return []SideEffectClass{SideEffectRead, SideEffectWrite, SideEffectExternal}
}

// ToolHandler executes a tool call. It receives JSON-encoded args (already
// validated against the tool's input schema) and must return JSON-encoded
// output (validated against the output schema by the executor).
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Tool describes a callable tool. Every field except PerCallTimeoutMs is
// required; RegisterTool panics on missing values.
type Tool struct {
	Name             string          // snake_case, globally unique
	Description      string          // one-line, used by LLM for tool selection
	InputSchema      json.RawMessage // JSON Schema (Draft 2020-12)
	OutputSchema     json.RawMessage // JSON Schema (Draft 2020-12)
	SideEffectClass  SideEffectClass
	OwningPackage    string // for trace + ops attribution
	PerCallTimeoutMs int    // 0 = use scenario default
	Handler          ToolHandler
}

// registeredTool holds the validated, immutable runtime view of a Tool.
type registeredTool struct {
	tool         Tool
	inputSchema  *CompiledSchema
	outputSchema *CompiledSchema
	callSite     string
}

var (
	regMu    sync.RWMutex
	registry = make(map[string]*registeredTool)
)

// RegisterTool registers a Tool from a package init() function. It panics
// (so the process refuses to start) on:
//
//   - duplicate Name (the panic message names both registration call sites)
//   - empty Name, Description, OwningPackage, or Handler
//   - invalid SideEffectClass
//   - empty or malformed InputSchema / OutputSchema
//
// Callers outside init() are technically allowed but discouraged; the
// scenario linter (Scope 10) will reject any RegisterTool call sites
// outside init().
func RegisterTool(t Tool) {
	callSite := callerFrame(2)

	switch {
	case t.Name == "":
		panic(fmt.Sprintf("agent.RegisterTool: empty tool name at %s", callSite))
	case t.Description == "":
		panic(fmt.Sprintf("agent.RegisterTool: tool %q missing description at %s", t.Name, callSite))
	case t.Handler == nil:
		panic(fmt.Sprintf("agent.RegisterTool: tool %q missing handler at %s", t.Name, callSite))
	case !t.SideEffectClass.Valid():
		panic(fmt.Sprintf("agent.RegisterTool: tool %q invalid side_effect_class %q at %s (must be one of read|write|external)",
			t.Name, t.SideEffectClass, callSite))
	case t.OwningPackage == "":
		panic(fmt.Sprintf("agent.RegisterTool: tool %q missing owning_package at %s", t.Name, callSite))
	case len(t.InputSchema) == 0:
		panic(fmt.Sprintf("agent.RegisterTool: tool %q missing input schema at %s", t.Name, callSite))
	case len(t.OutputSchema) == 0:
		panic(fmt.Sprintf("agent.RegisterTool: tool %q missing output schema at %s", t.Name, callSite))
	case t.PerCallTimeoutMs < 0:
		panic(fmt.Sprintf("agent.RegisterTool: tool %q negative per_call_timeout_ms %d at %s",
			t.Name, t.PerCallTimeoutMs, callSite))
	}

	inSch, err := CompileSchema(t.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("agent.RegisterTool: tool %q input_schema failed to compile: %v (at %s)",
			t.Name, err, callSite))
	}
	outSch, err := CompileSchema(t.OutputSchema)
	if err != nil {
		panic(fmt.Sprintf("agent.RegisterTool: tool %q output_schema failed to compile: %v (at %s)",
			t.Name, err, callSite))
	}

	regMu.Lock()
	defer regMu.Unlock()

	if existing, ok := registry[t.Name]; ok {
		panic(fmt.Sprintf("agent.RegisterTool: duplicate tool name %q registered at %s; first registered at %s",
			t.Name, callSite, existing.callSite))
	}

	// Defensive copy of schema bytes — caller cannot mutate post-registration.
	tCopy := t
	tCopy.InputSchema = append(json.RawMessage(nil), t.InputSchema...)
	tCopy.OutputSchema = append(json.RawMessage(nil), t.OutputSchema...)

	registry[t.Name] = &registeredTool{
		tool:         tCopy,
		inputSchema:  inSch,
		outputSchema: outSch,
		callSite:     callSite,
	}
}

// Has reports whether a tool with the given name is registered.
func Has(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	_, ok := registry[name]
	return ok
}

// ByName returns a copy of the registered Tool. The schema byte slices are
// shared with the registry; do not mutate them.
func ByName(name string) (Tool, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	rt, ok := registry[name]
	if !ok {
		return Tool{}, false
	}
	return rt.tool, true
}

// All returns every registered tool sorted by Name.
func All() []Tool {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Tool, 0, len(registry))
	for _, rt := range registry {
		out = append(out, rt.tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SchemasFor returns the compiled input/output schemas for a tool. Used
// by the executor (Scope 5) to validate args before dispatch and results
// after dispatch.
func SchemasFor(name string) (input, output *CompiledSchema, ok bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	rt, ok := registry[name]
	if !ok {
		return nil, nil, false
	}
	return rt.inputSchema, rt.outputSchema, true
}

// resetRegistryForTest clears the registry. Test-only; not exported.
func resetRegistryForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	registry = make(map[string]*registeredTool)
}

// callerFrame returns "file:line" for the caller `skip` frames above the
// caller of callerFrame itself. Used for duplicate-tool error messages.
func callerFrame(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown:0"
	}
	return fmt.Sprintf("%s:%d", file, line)
}
