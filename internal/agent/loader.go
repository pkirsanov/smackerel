// Scenario loader for spec 037 Scope 3.
//
// The loader scans a directory for YAML files, parses any whose top-level
// `type:` is `scenario`, and applies every load-time validation rule from
// design §2.2. Each rule failure rejects only that file (BS-009) and the
// loader continues scanning so a single bad file cannot wipe the registry.
// Two scenarios sharing an `id` is fatal — Load returns an error and the
// process is expected to refuse to start (BS-011).
//
// Each registered scenario carries a `ContentHash` (sha256 hex of the
// canonical JSON projection of its YAML), which is recorded on every
// trace and consulted by the replay command (Scope 6) to detect that
// the scenario file changed since the trace was captured.

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AllowedTool is one entry in a scenario's allowed_tools list.
type AllowedTool struct {
	Name            string
	SideEffectClass SideEffectClass
}

// ScenarioLimits are per-invocation guardrails declared by each scenario.
type ScenarioLimits struct {
	MaxLoopIterations int
	TimeoutMs         int
	SchemaRetryBudget int
	PerToolTimeoutMs  int
}

// Scenario is the parsed, validated scenario record. Schema bytes are
// stored as JSON (the loader converts YAML schema blocks to JSON so the
// rest of the agent runtime works in one format).
type Scenario struct {
	ID              string
	Version         string
	Description     string
	IntentExamples  []string
	SystemPrompt    string
	AllowedTools    []AllowedTool
	InputSchema     json.RawMessage
	OutputSchema    json.RawMessage
	Limits          ScenarioLimits
	TokenBudget     int
	Temperature     float64
	ModelPreference string
	SideEffectClass SideEffectClass
	ContentHash     string // sha256 hex of canonical JSON projection
	SourcePath      string

	inputSchema  *CompiledSchema
	outputSchema *CompiledSchema
}

// CompiledInputSchema returns the compiled input schema. Nil if the
// scenario was constructed outside the loader.
func (s *Scenario) CompiledInputSchema() *CompiledSchema { return s.inputSchema }

// CompiledOutputSchema returns the compiled output schema.
func (s *Scenario) CompiledOutputSchema() *CompiledSchema { return s.outputSchema }

// LoadError describes one rejected scenario file.
type LoadError struct {
	Path    string
	Message string
}

// Error implements the error interface.
func (e LoadError) Error() string { return fmt.Sprintf("%s: %s", e.Path, e.Message) }

// Loader is the public interface used by the executor and the linter.
type Loader interface {
	Load(dir, glob string) (registered []*Scenario, rejected []LoadError, fatal error)
}

// DefaultLoader returns a loader that uses the registry side-effect-class
// metadata for cross-checks. The registry MUST already be populated (i.e.
// init() side effects have run) before Load is called.
func DefaultLoader() Loader { return &defaultLoader{} }

type defaultLoader struct{}

var (
	scenarioIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	scenarioVersionPattern = regexp.MustCompile(`^([a-z][a-z0-9_-]*)-v(\d+)$`)
)

// Load reads `dir`, applies `glob` (default "*.yaml" + "*.yml" if empty)
// against the file names, and returns the registered scenarios, rejected
// files, and a fatal error (currently only set on duplicate `id`).
//
// The order of `registered` is sorted by `SourcePath` for determinism;
// the order of `rejected` is also sorted by Path. This matters for the
// linter binary so its output is reproducible across machines.
func (l *defaultLoader) Load(dir, glob string) ([]*Scenario, []LoadError, error) {
	if dir == "" {
		return nil, nil, fmt.Errorf("scenario dir is empty")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read scenario dir %s: %w", dir, err)
	}

	var registered []*Scenario
	var rejected []LoadError
	// Track id → first SourcePath; on duplicate, return a fatal error.
	idFirstSeen := make(map[string]string)
	type dup struct{ id, first, second string }
	var duplicates []dup

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !matchScenarioGlob(name, glob) {
			continue
		}
		full := filepath.Join(dir, name)

		data, err := os.ReadFile(full)
		if err != nil {
			rejected = append(rejected, LoadError{Path: full, Message: fmt.Sprintf("read file: %v", err)})
			continue
		}

		// Parse YAML into a generic map so we can detect non-scenario
		// files (e.g. existing prompt contracts with type != "scenario")
		// and skip them silently — those are not the loader's concern.
		var top map[string]any
		if err := yaml.Unmarshal(data, &top); err != nil {
			rejected = append(rejected, LoadError{Path: full, Message: fmt.Sprintf("parse YAML: %v", err)})
			continue
		}
		if top == nil {
			continue // empty file — silent skip
		}
		typeVal, _ := top["type"].(string)
		if typeVal != "scenario" {
			continue // not a scenario; existing prompt contracts pass through here
		}

		scn, lerr := parseScenario(full, data, top)
		if lerr != nil {
			rejected = append(rejected, *lerr)
			continue
		}

		if first, ok := idFirstSeen[scn.ID]; ok {
			duplicates = append(duplicates, dup{id: scn.ID, first: first, second: full})
			// Do NOT register the duplicate; the first wins so that, if the
			// caller chooses to ignore the fatal (e.g. linter mode), the
			// non-duplicate scenarios are still usable.
			continue
		}
		idFirstSeen[scn.ID] = full
		registered = append(registered, scn)
	}

	sort.Slice(registered, func(i, j int) bool { return registered[i].SourcePath < registered[j].SourcePath })
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].Path < rejected[j].Path })

	if len(duplicates) > 0 {
		var msgs []string
		for _, d := range duplicates {
			msgs = append(msgs, fmt.Sprintf("scenario id %q declared in both %s and %s", d.id, d.first, d.second))
		}
		return registered, rejected, fmt.Errorf("duplicate scenario id(s) — process refuses to start: %s",
			strings.Join(msgs, "; "))
	}
	return registered, rejected, nil
}

func matchScenarioGlob(name, glob string) bool {
	if glob == "" {
		return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
	}
	matched, err := filepath.Match(glob, name)
	if err != nil {
		// Invalid glob — fall back to extension match.
		return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
	}
	return matched
}

// parseScenario applies every §2.2 rule. It returns either a fully
// populated Scenario or a LoadError describing the first failure that
// prevents registration. The function intentionally short-circuits on
// the first failure per file so the operator sees one root cause.
func parseScenario(path string, raw []byte, top map[string]any) (*Scenario, *LoadError) {
	reject := func(msg string) (*Scenario, *LoadError) {
		return nil, &LoadError{Path: path, Message: msg}
	}

	// --- Required top-level fields (BS-009) -------------------------------
	required := []string{
		"id", "version", "type", "system_prompt", "allowed_tools",
		"input_schema", "output_schema", "limits", "side_effect_class",
	}
	for _, key := range required {
		if _, ok := top[key]; !ok {
			return reject(fmt.Sprintf("missing required field %q", key))
		}
	}

	id, _ := top["id"].(string)
	version, _ := top["version"].(string)
	desc, _ := top["description"].(string)
	systemPrompt, _ := top["system_prompt"].(string)
	sideClass, _ := top["side_effect_class"].(string)
	modelPref, _ := top["model_preference"].(string)

	if id == "" || !scenarioIDPattern.MatchString(id) {
		return reject(fmt.Sprintf("id %q does not match ^[a-z][a-z0-9_]*$", id))
	}
	if systemPrompt == "" {
		return reject("system_prompt is empty")
	}

	// --- version contract (slug must equal id, modulo - vs _) -------------
	m := scenarioVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return reject(fmt.Sprintf("version %q does not match ^[a-z][a-z0-9_-]*-v<N>$", version))
	}
	versionSlug := strings.ReplaceAll(m[1], "-", "_")
	if versionSlug != id {
		return reject(fmt.Sprintf("version slug %q (from %q) must equal id %q after replacing dashes with underscores",
			versionSlug, version, id))
	}

	// --- side_effect_class (scenario-level) -------------------------------
	scenarioClass := SideEffectClass(sideClass)
	if !scenarioClass.Valid() {
		return reject(fmt.Sprintf("side_effect_class %q must be one of read|write|external", sideClass))
	}

	// --- intent_examples (allowed empty for system-only scenarios) --------
	var intentExamples []string
	if rawEx, ok := top["intent_examples"]; ok && rawEx != nil {
		list, ok := rawEx.([]any)
		if !ok {
			return reject("intent_examples must be a list of strings")
		}
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return reject(fmt.Sprintf("intent_examples[%d] is not a string", i))
			}
			if strings.TrimSpace(s) == "" {
				return reject(fmt.Sprintf("intent_examples[%d] is empty", i))
			}
			intentExamples = append(intentExamples, s)
		}
	}

	// --- allowed_tools (BS-010) -------------------------------------------
	rawTools, ok := top["allowed_tools"].([]any)
	if !ok {
		return reject("allowed_tools must be a non-empty list")
	}
	if len(rawTools) == 0 {
		return reject("allowed_tools must declare at least one tool")
	}
	var tools []AllowedTool
	maxToolRank := -1
	seenToolNames := make(map[string]bool)
	for i, item := range rawTools {
		entry, ok := item.(map[string]any)
		if !ok {
			return reject(fmt.Sprintf("allowed_tools[%d] is not a mapping", i))
		}
		name, _ := entry["name"].(string)
		cls, _ := entry["side_effect_class"].(string)
		if name == "" {
			return reject(fmt.Sprintf("allowed_tools[%d].name is required", i))
		}
		if seenToolNames[name] {
			return reject(fmt.Sprintf("allowed_tools[%d].name %q duplicated", i, name))
		}
		seenToolNames[name] = true
		toolClass := SideEffectClass(cls)
		if !toolClass.Valid() {
			return reject(fmt.Sprintf("allowed_tools[%d].side_effect_class %q must be one of read|write|external", i, cls))
		}
		// Tool MUST be registered (BS-010).
		registered, ok := ByName(name)
		if !ok {
			return reject(fmt.Sprintf("allowed_tools[%d].name %q is not in the tool registry — register the tool from its owning package init() before declaring it in a scenario", i, name))
		}
		// Declared class MUST equal registered class.
		if registered.SideEffectClass != toolClass {
			return reject(fmt.Sprintf("allowed_tools[%d] %q declares side_effect_class %q but the registry has %q",
				i, name, toolClass, registered.SideEffectClass))
		}
		if toolClass.Rank() > maxToolRank {
			maxToolRank = toolClass.Rank()
		}
		tools = append(tools, AllowedTool{Name: name, SideEffectClass: toolClass})
	}

	// --- scenario class >= max(tool classes) ------------------------------
	if scenarioClass.Rank() < maxToolRank {
		return reject(fmt.Sprintf("scenario side_effect_class %q is below the highest allowed_tools class (rank %d)",
			scenarioClass, maxToolRank))
	}

	// --- limits ranges ----------------------------------------------------
	limitsRaw, ok := top["limits"].(map[string]any)
	if !ok {
		return reject("limits must be a mapping")
	}
	limits, lerr := parseLimits(limitsRaw)
	if lerr != "" {
		return reject(lerr)
	}

	// --- token_budget / temperature (optional but type-checked if present)
	tokenBudget := 0
	if v, ok := top["token_budget"]; ok && v != nil {
		n, ok := toInt(v)
		if !ok || n < 1 {
			return reject(fmt.Sprintf("token_budget must be a positive integer, got %v", v))
		}
		tokenBudget = n
	}
	temperature := 0.0
	if v, ok := top["temperature"]; ok && v != nil {
		f, ok := toFloat(v)
		if !ok || f < 0 || f > 2 {
			return reject(fmt.Sprintf("temperature must be a number in [0, 2], got %v", v))
		}
		temperature = f
	}

	// --- input/output schema (BS-009 A4 + x-redact policy) ----------------
	inputSchemaRaw, lerr := schemaToJSON(top["input_schema"])
	if lerr != "" {
		return reject("input_schema: " + lerr)
	}
	outputSchemaRaw, lerr := schemaToJSON(top["output_schema"])
	if lerr != "" {
		return reject("output_schema: " + lerr)
	}
	inputSchema, err := CompileSchema(inputSchemaRaw)
	if err != nil {
		return reject(fmt.Sprintf("input_schema failed to compile: %v", err))
	}
	outputSchema, err := CompileSchema(outputSchemaRaw)
	if err != nil {
		return reject(fmt.Sprintf("output_schema failed to compile: %v", err))
	}
	// x-redact policy: no required field may carry x-redact: true.
	if msg := violatesRedactPolicy(top["output_schema"]); msg != "" {
		return reject("output_schema redaction policy: " + msg)
	}

	// --- content_hash (canonical JSON projection of the parsed YAML) ------
	canonical, err := canonicalJSON(top)
	if err != nil {
		return reject(fmt.Sprintf("compute content_hash: %v", err))
	}
	sum := sha256.Sum256(canonical)

	scn := &Scenario{
		ID:              id,
		Version:         version,
		Description:     desc,
		IntentExamples:  intentExamples,
		SystemPrompt:    systemPrompt,
		AllowedTools:    tools,
		InputSchema:     inputSchemaRaw,
		OutputSchema:    outputSchemaRaw,
		Limits:          limits,
		TokenBudget:     tokenBudget,
		Temperature:     temperature,
		ModelPreference: modelPref,
		SideEffectClass: scenarioClass,
		ContentHash:     hex.EncodeToString(sum[:]),
		SourcePath:      path,
		inputSchema:     inputSchema,
		outputSchema:    outputSchema,
	}
	_ = raw // raw bytes retained only for error context if needed
	return scn, nil
}

// parseLimits enforces the ranges from design §2.2:
//
//	max_loop_iterations ∈ [1, 32]; timeout_ms ∈ [1000, 120000];
//	schema_retry_budget ∈ [0, 5]; per_tool_timeout_ms ∈ [1, timeout_ms].
func parseLimits(raw map[string]any) (ScenarioLimits, string) {
	requiredKeys := []string{"max_loop_iterations", "timeout_ms", "schema_retry_budget", "per_tool_timeout_ms"}
	for _, k := range requiredKeys {
		if _, ok := raw[k]; !ok {
			return ScenarioLimits{}, fmt.Sprintf("limits missing required key %q", k)
		}
	}
	maxIter, ok := toInt(raw["max_loop_iterations"])
	if !ok || maxIter < 1 || maxIter > 32 {
		return ScenarioLimits{}, fmt.Sprintf("limits.max_loop_iterations must be an integer in [1, 32], got %v", raw["max_loop_iterations"])
	}
	timeoutMs, ok := toInt(raw["timeout_ms"])
	if !ok || timeoutMs < 1000 || timeoutMs > 120000 {
		return ScenarioLimits{}, fmt.Sprintf("limits.timeout_ms must be an integer in [1000, 120000], got %v", raw["timeout_ms"])
	}
	retryBudget, ok := toInt(raw["schema_retry_budget"])
	if !ok || retryBudget < 0 || retryBudget > 5 {
		return ScenarioLimits{}, fmt.Sprintf("limits.schema_retry_budget must be an integer in [0, 5], got %v", raw["schema_retry_budget"])
	}
	perTool, ok := toInt(raw["per_tool_timeout_ms"])
	if !ok || perTool < 1 || perTool > timeoutMs {
		return ScenarioLimits{}, fmt.Sprintf("limits.per_tool_timeout_ms must be an integer in [1, %d] (timeout_ms), got %v", timeoutMs, raw["per_tool_timeout_ms"])
	}
	return ScenarioLimits{
		MaxLoopIterations: maxIter,
		TimeoutMs:         timeoutMs,
		SchemaRetryBudget: retryBudget,
		PerToolTimeoutMs:  perTool,
	}, ""
}

// schemaToJSON normalizes a YAML-parsed schema block to JSON bytes so the
// compiler library can ingest it. Returns ("", "errMsg") on failure.
func schemaToJSON(v any) (json.RawMessage, string) {
	if v == nil {
		return nil, "is empty"
	}
	normalized := yamlNormalize(v)
	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Sprintf("encode as JSON: %v", err)
	}
	return b, ""
}

// violatesRedactPolicy returns "" if the output_schema obeys the rule "no
// required field may carry x-redact: true". Returns a human-readable
// message naming the offending field otherwise.
func violatesRedactPolicy(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	requiredAny, _ := m["required"].([]any)
	if len(requiredAny) == 0 {
		return ""
	}
	required := make(map[string]bool, len(requiredAny))
	for _, x := range requiredAny {
		if s, ok := x.(string); ok {
			required[s] = true
		}
	}
	props, _ := m["properties"].(map[string]any)
	if len(props) == 0 {
		return ""
	}
	for fieldName, propAny := range props {
		if !required[fieldName] {
			continue
		}
		prop, ok := propAny.(map[string]any)
		if !ok {
			continue
		}
		if redact, _ := prop["x-redact"].(bool); redact {
			return fmt.Sprintf("required field %q has x-redact: true (forbidden — required fields cannot be redacted)", fieldName)
		}
	}
	return ""
}

// yamlNormalize converts map[interface{}]interface{} → map[string]interface{}
// recursively. yaml.v3 typically already produces map[string]interface{},
// but defensive normalization keeps the JSON marshaller happy.
func yamlNormalize(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = yamlNormalize(val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = yamlNormalize(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = yamlNormalize(val)
		}
		return out
	default:
		return v
	}
}

// canonicalJSON returns a deterministic JSON encoding of the parsed YAML
// document. json.Marshal already sorts map keys, so this is sufficient
// for content-hash stability across whitespace and comment changes.
func canonicalJSON(top map[string]any) ([]byte, error) {
	return json.Marshal(yamlNormalize(top))
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		if float64(int(x)) != x {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}
