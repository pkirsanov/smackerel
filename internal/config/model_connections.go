// Package config — Spec 096 SCOPE-01: multi-provider model-connection registry.
//
// ModelConnectionsConfig governs the `llm.connections[]`, `llm.discovery`,
// and `llm.model_costs[]` SST blocks. Every value originates in
// config/smackerel.yaml and flows through scripts/commands/config.sh into
// the generated env file as LLM_CONNECTIONS_JSON / LLM_DISCOVERY_* /
// LLM_MODEL_COSTS_JSON. There are NO in-source defaults (Gate G028,
// smackerel-no-defaults): every env var MUST be present at load time, and
// the closed-set Validate() aborts fail-loud (with a NAMED error and zero
// substituted default) on an unknown kind, a missing required per-kind
// param, a non-positive discovery bound, an env-mode secret absent from
// infrastructure.secret_keys, or an enabled non-ollama model with no
// model_costs entry.
//
// The registry is operator-global: a ModelConnection carries NO
// actor_user_id (single shared graph, consistent with operator-global
// connectors). Secret material is NEVER an inline plaintext value — a
// db-mode secret rides the SCOPE-02 vault, an env-mode secret rides the
// managed-secret infrastructure.secret_keys path; the struct has no field
// that could hold a cleartext credential.
package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Closed-set provider-kind vocabulary (Spec 096 SCOPE-01, design §4). A
// kind outside this set aborts the registry build fail-loud — there is no
// silent acceptance and no default kind.
const (
	ModelConnectionKindOllama       = "ollama"
	ModelConnectionKindAnthropic    = "anthropic"
	ModelConnectionKindOpenAI       = "openai"
	ModelConnectionKindAzureFoundry = "azure-foundry"
	ModelConnectionKindGoogle       = "google"
	ModelConnectionKindBedrock      = "bedrock"
)

// modelConnectionKinds is the closed admissible kind set. Order is the
// declaration order above; used only for the error message vocabulary.
var modelConnectionKinds = []string{
	ModelConnectionKindOllama,
	ModelConnectionKindAnthropic,
	ModelConnectionKindOpenAI,
	ModelConnectionKindAzureFoundry,
	ModelConnectionKindGoogle,
	ModelConnectionKindBedrock,
}

// Secret-provisioning modes (design §4 variation axis 3). "" = no secret
// (ollama, local); "db" = web-UI-entered, encrypted vault (SCOPE-02);
// "env" = managed-secret env var via the infrastructure.secret_keys path.
const (
	ModelConnectionSecretModeNone = ""
	ModelConnectionSecretModeDB   = "db"
	ModelConnectionSecretModeEnv  = "env"
)

// ModelConnectionSecretRef points at WHERE a connection's secret lives.
// It NEVER holds the secret value itself — only the provisioning mode and,
// for env-mode, the name of the managed-secret env var.
type ModelConnectionSecretRef struct {
	Mode   string // "" | "db" | "env"
	EnvKey string // managed-secret env var NAME (env-mode only)
}

// ModelDescriptor is one provider-offered model in a curated connection's
// models list. The capability flags are carried by the SST registry now
// (additive) and consumed by the SCOPE-04 catalog; SCOPE-01 only needs the
// id (to cost-check enabled non-ollama models against llm.model_costs).
type ModelDescriptor struct {
	ID            string
	ToolCapable   bool
	Vision        bool
	ContextWindow int
}

// ModelConnectionModels is a connection's discovery strategy plus, for the
// curated (hosted) strategy, the operator-curated model list. The ollama
// (live) strategy carries no list — its models are discovered at runtime
// (SCOPE-04) and are always $0.
type ModelConnectionModels struct {
	Strategy string // "live" (ollama) | "curated" (hosted)
	List     []ModelDescriptor
}

// ModelConnection is one operator-global provider-connection slot. Per-kind
// non-secret connection parameters are carried generically by Params so the
// contract never grows a typed field per kind (design §4 variation axis 2).
type ModelConnection struct {
	ID        string
	Kind      string
	Enabled   bool
	Params    map[string]any
	SecretRef ModelConnectionSecretRef
	Models    ModelConnectionModels
}

// ModelDiscoveryConfig bounds discovery aggregation (design §6.3). Both
// fields are REQUIRED > 0 whenever the registry declares ≥1 connection —
// there is no default (G028).
type ModelDiscoveryConfig struct {
	CacheTTLms           int
	PerProviderTimeoutMs int
}

// ModelCost is one provider-qualified (<kind>/<backend-id>) USD rate. A
// billable (enabled, non-ollama) model with no matching entry aborts
// fail-loud — never a silent $0 (design §12; SCN-096-G02/G03).
type ModelCost struct {
	Model          string
	InputUSDPer1k  float64
	OutputUSDPer1k float64
}

// ModelConnectionsConfig is the loaded llm.connections[]/discovery/
// model_costs[] SST surface for spec 096 SCOPE-01.
type ModelConnectionsConfig struct {
	Connections []ModelConnection
	Discovery   ModelDiscoveryConfig
	ModelCosts  []ModelCost
}

// modelConnectionRequiredParams maps each provider kind to the non-secret
// params it REQUIRES (design §4). A kind absent here (anthropic, openai,
// google) has no unconditionally-required param; google additionally has a
// Vertex-coherence rule enforced in Validate (project and location travel
// together).
var modelConnectionRequiredParams = map[string][]string{
	ModelConnectionKindOllama:       {"base_url"},
	ModelConnectionKindAzureFoundry: {"endpoint", "api_version", "deployment"},
	ModelConnectionKindBedrock:      {"region"},
}

// rawModelConnection is the wire shape emitted by the config generator into
// LLM_CONNECTIONS_JSON. The generic per-kind params and the curated models
// list are carried as compact inline-JSON STRINGS (the hand-rolled
// yaml_get_json generator parses a list of flat objects only); they are
// re-parsed here into the typed Params map / Models struct.
type rawModelConnection struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Enabled         bool   `json:"enabled"`
	Params          string `json:"params"`          // inner JSON object
	SecretRefMode   string `json:"secret_ref_mode"` // "" | "db" | "env"
	SecretRefEnvKey string `json:"secret_ref_env_key"`
	Models          string `json:"models"` // inner JSON object {strategy,list}
}

// rawModels is the inner JSON shape of a connection's models field.
type rawModels struct {
	Strategy string `json:"strategy"`
	List     []struct {
		ID            string `json:"id"`
		ToolCapable   bool   `json:"tool_capable"`
		Vision        bool   `json:"vision"`
		ContextWindow int    `json:"context_window"`
	} `json:"list"`
}

// LoadModelConnections reads the LLM_CONNECTIONS_JSON / LLM_DISCOVERY_* /
// LLM_MODEL_COSTS_JSON env vars, parses them into the typed registry, and
// runs the closed-set fail-loud Validate() against the canonical managed
// secret-key manifest (SecretKeys()). A missing env var (LookupEnv == false)
// is always a fail-loud [F096-SST-MISSING] error.
func LoadModelConnections() (ModelConnectionsConfig, error) {
	var errs []string

	connectionsJSON, errs := lookupString("LLM_CONNECTIONS_JSON", errs)
	modelCostsJSON, errs := lookupString("LLM_MODEL_COSTS_JSON", errs)
	cacheTTLms, errs := lookupInt("LLM_DISCOVERY_CACHE_TTL_MS", errs)
	perProviderTimeoutMs, errs := lookupInt("LLM_DISCOVERY_PER_PROVIDER_TIMEOUT_MS", errs)

	if len(errs) > 0 {
		return ModelConnectionsConfig{}, fmt.Errorf("[F096-SST-MISSING] missing or invalid required llm model-connection configuration: %s", strings.Join(errs, ", "))
	}

	cfg, err := parseModelConnections(connectionsJSON, modelCostsJSON, cacheTTLms, perProviderTimeoutMs)
	if err != nil {
		return ModelConnectionsConfig{}, err
	}
	if err := cfg.Validate(SecretKeys()); err != nil {
		return ModelConnectionsConfig{}, err
	}
	return cfg, nil
}

// parseModelConnections builds the typed registry from the generator wire
// shape. It returns a [F096-SST-INVALID] error ONLY for malformed JSON /
// unparseable inner objects; the closed-set semantic rules live in
// Validate so a unit test can exercise them against a synthetic secret-key
// list without the full env pipeline.
func parseModelConnections(connectionsJSON, modelCostsJSON string, cacheTTLms, perProviderTimeoutMs int) (ModelConnectionsConfig, error) {
	cfg := ModelConnectionsConfig{
		Discovery: ModelDiscoveryConfig{CacheTTLms: cacheTTLms, PerProviderTimeoutMs: perProviderTimeoutMs},
	}

	var raws []rawModelConnection
	if strings.TrimSpace(connectionsJSON) != "" {
		if err := json.Unmarshal([]byte(connectionsJSON), &raws); err != nil {
			return ModelConnectionsConfig{}, fmt.Errorf("[F096-SST-INVALID] LLM_CONNECTIONS_JSON is not a valid JSON array: %v", err)
		}
	}
	for i, r := range raws {
		conn := ModelConnection{
			ID:      r.ID,
			Kind:    r.Kind,
			Enabled: r.Enabled,
			SecretRef: ModelConnectionSecretRef{
				Mode:   r.SecretRefMode,
				EnvKey: r.SecretRefEnvKey,
			},
			Params: map[string]any{},
		}
		// Re-parse the inner params object (compact JSON string). Empty is
		// an empty param set (e.g. anthropic, gemini).
		if strings.TrimSpace(r.Params) != "" {
			if err := json.Unmarshal([]byte(r.Params), &conn.Params); err != nil {
				return ModelConnectionsConfig{}, fmt.Errorf("[F096-SST-INVALID] llm.connections[%d] (id=%q) params is not a valid JSON object: %v", i, r.ID, err)
			}
		}
		// Re-parse the inner models object (compact JSON string).
		if strings.TrimSpace(r.Models) != "" {
			var m rawModels
			if err := json.Unmarshal([]byte(r.Models), &m); err != nil {
				return ModelConnectionsConfig{}, fmt.Errorf("[F096-SST-INVALID] llm.connections[%d] (id=%q) models is not a valid JSON object: %v", i, r.ID, err)
			}
			conn.Models.Strategy = m.Strategy
			for _, md := range m.List {
				conn.Models.List = append(conn.Models.List, ModelDescriptor{
					ID:            md.ID,
					ToolCapable:   md.ToolCapable,
					Vision:        md.Vision,
					ContextWindow: md.ContextWindow,
				})
			}
		}
		cfg.Connections = append(cfg.Connections, conn)
	}

	if strings.TrimSpace(modelCostsJSON) != "" {
		var costs []struct {
			Model          string  `json:"model"`
			InputUSDPer1k  float64 `json:"input_usd_per_1k"`
			OutputUSDPer1k float64 `json:"output_usd_per_1k"`
		}
		if err := json.Unmarshal([]byte(modelCostsJSON), &costs); err != nil {
			return ModelConnectionsConfig{}, fmt.Errorf("[F096-SST-INVALID] LLM_MODEL_COSTS_JSON is not a valid JSON array: %v", err)
		}
		for _, c := range costs {
			cfg.ModelCosts = append(cfg.ModelCosts, ModelCost{
				Model:          c.Model,
				InputUSDPer1k:  c.InputUSDPer1k,
				OutputUSDPer1k: c.OutputUSDPer1k,
			})
		}
	}

	return cfg, nil
}

// QualifiedModelID returns the canonical provider-qualified id
// (<kind>/<backend-id>) used to look a model up in the model_costs table
// (design §8 identifier grammar). SCOPE-04 owns the live litellm-routing
// canonicalization; SCOPE-01 only needs the stable cost key.
func (c ModelConnection) QualifiedModelID(backendID string) string {
	return c.Kind + "/" + backendID
}

// Validate enforces the spec 096 SCOPE-01 closed-set fail-loud rules. Each
// violation is a NAMED error (it names the offending connection id and the
// offending param/kind/key) with zero substituted default. secretKeys is
// the canonical managed secret-key manifest (config.SecretKeys() at load
// time; a synthetic list in unit tests).
func (c *ModelConnectionsConfig) Validate(secretKeys []string) error {
	secretKeySet := make(map[string]struct{}, len(secretKeys))
	for _, k := range secretKeys {
		secretKeySet[k] = struct{}{}
	}
	costSet := make(map[string]struct{}, len(c.ModelCosts))
	for _, mc := range c.ModelCosts {
		costSet[mc.Model] = struct{}{}
	}

	var errs []string
	seenIDs := make(map[string]struct{}, len(c.Connections))
	for _, conn := range c.Connections {
		label := conn.ID
		if strings.TrimSpace(label) == "" {
			errs = append(errs, "llm.connections[] (a connection has an empty id)")
			continue
		}
		if _, dup := seenIDs[conn.ID]; dup {
			errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (duplicate connection id)", conn.ID))
		}
		seenIDs[conn.ID] = struct{}{}

		// Rule 1 — closed-set kind vocabulary.
		if !isKnownModelConnectionKind(conn.Kind) {
			errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (unknown kind %q; must be one of %s)", conn.ID, conn.Kind, strings.Join(modelConnectionKinds, "|")))
			// An unknown kind cannot have its per-kind params checked;
			// skip the remaining per-connection rules for this slot.
			continue
		}

		// Rule 2 — per-kind required non-secret params, carried generically
		// by Params. A missing/empty required param fails loud naming the
		// connection + the param.
		for _, req := range modelConnectionRequiredParams[conn.Kind] {
			if !hasNonEmptyParam(conn.Params, req) {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] kind %q (missing required param %q)", conn.ID, conn.Kind, req))
			}
		}
		// Google Vertex coherence: project and location travel together
		// (vertex mode); neither present = gemini mode (no required param).
		if conn.Kind == ModelConnectionKindGoogle {
			hasProject := hasNonEmptyParam(conn.Params, "project")
			hasLocation := hasNonEmptyParam(conn.Params, "location")
			if hasProject != hasLocation {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] kind \"google\" (Vertex requires BOTH project and location; got project=%t location=%t)", conn.ID, hasProject, hasLocation))
			}
		}

		// Rule 4 — secret_ref closed-set + env-mode key-in-manifest. The
		// struct carries no plaintext secret field, so "no inline plaintext
		// secret" is structural; here we enforce the reference shape.
		switch conn.SecretRef.Mode {
		case ModelConnectionSecretModeNone:
			if strings.TrimSpace(conn.SecretRef.EnvKey) != "" {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (secret_ref.env_key set but mode is empty; env_key is only valid with mode \"env\")", conn.ID))
			}
		case ModelConnectionSecretModeDB:
			if strings.TrimSpace(conn.SecretRef.EnvKey) != "" {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (secret_ref.env_key set but mode is \"db\"; db-mode secrets ride the vault, not an env key)", conn.ID))
			}
		case ModelConnectionSecretModeEnv:
			key := strings.TrimSpace(conn.SecretRef.EnvKey)
			if key == "" {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (secret_ref.mode \"env\" requires a non-empty env_key)", conn.ID))
			} else if _, ok := secretKeySet[key]; !ok {
				errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (secret_ref.env_key %q is not declared in infrastructure.secret_keys)", conn.ID, key))
			}
		default:
			errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (unknown secret_ref.mode %q; must be one of \"\"|\"db\"|\"env\")", conn.ID, conn.SecretRef.Mode))
		}
		// Ollama is local and carries no credential.
		if conn.Kind == ModelConnectionKindOllama && conn.SecretRef.Mode != ModelConnectionSecretModeNone {
			errs = append(errs, fmt.Sprintf("llm.connections[id=%q] (ollama is local and carries no secret_ref; got mode %q)", conn.ID, conn.SecretRef.Mode))
		}

		// Rule 5 — an enabled non-ollama model MUST have a model_costs
		// entry (provider-qualified). A missing rate is a fail-loud named
		// abort, NEVER a silent $0.
		if conn.Enabled && conn.Kind != ModelConnectionKindOllama {
			for _, md := range conn.Models.List {
				qualified := conn.QualifiedModelID(md.ID)
				if _, ok := costSet[qualified]; !ok {
					errs = append(errs, fmt.Sprintf("llm.connections[id=%q] enabled %s model %q has no llm.model_costs entry (expected key %q; a billable model with no rate is refused, never $0)", conn.ID, conn.Kind, md.ID, qualified))
				}
			}
		}
	}

	// Rule 3 — discovery bounds REQUIRED > 0 whenever ≥1 connection is
	// declared (no default).
	if len(c.Connections) >= 1 {
		if c.Discovery.CacheTTLms <= 0 {
			errs = append(errs, fmt.Sprintf("llm.discovery.cache_ttl_ms (must be > 0 when ≥1 connection is declared, got %d; no default is substituted)", c.Discovery.CacheTTLms))
		}
		if c.Discovery.PerProviderTimeoutMs <= 0 {
			errs = append(errs, fmt.Sprintf("llm.discovery.per_provider_timeout_ms (must be > 0 when ≥1 connection is declared, got %d; no default is substituted)", c.Discovery.PerProviderTimeoutMs))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("[F096-SST-INVALID] invalid llm model-connection configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// isKnownModelConnectionKind reports whether kind is in the closed
// admissible vocabulary.
func isKnownModelConnectionKind(kind string) bool {
	switch kind {
	case ModelConnectionKindOllama, ModelConnectionKindAnthropic, ModelConnectionKindOpenAI,
		ModelConnectionKindAzureFoundry, ModelConnectionKindGoogle, ModelConnectionKindBedrock:
		return true
	default:
		return false
	}
}

// hasNonEmptyParam reports whether params[key] is present and, for string
// values, non-empty after trimming. Non-string values (numbers, bools) are
// treated as present.
func hasNonEmptyParam(params map[string]any, key string) bool {
	v, ok := params[key]
	if !ok {
		return false
	}
	if s, isStr := v.(string); isStr {
		return strings.TrimSpace(s) != ""
	}
	return v != nil
}
