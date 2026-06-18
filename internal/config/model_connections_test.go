package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Spec 096 SCOPE-01 — Provider-connection registry + config SST schema.
//
// These tests pin the closed-set fail-loud loader/validator
// (internal/config/model_connections.go) against scenarios SCN-096-A01,
// SCN-096-A04, SCN-096-G02. They are specification-driven: every
// adversarial case carries a CONTROL assertion (the un-mutated fixture
// passes) so a neutralised validator (always-pass) OR an over-zealous one
// (always-fail) would trip the test — none is tautological.

// mkConn builds one connection in the exact generator WIRE shape: per-kind
// params and the curated models list are compact inline-JSON STRINGS (the
// hand-rolled yaml_get_json generator parses a list of flat objects only),
// re-parsed by parseModelConnections into the typed Params map / Models.
func mkConn(id, kind string, enabled bool, params map[string]any, mode, envKey string, models map[string]any) map[string]any {
	paramsStr := "{}"
	if params != nil {
		b, _ := json.Marshal(params)
		paramsStr = string(b)
	}
	modelsStr := ""
	if models != nil {
		b, _ := json.Marshal(models)
		modelsStr = string(b)
	}
	return map[string]any{
		"id":                 id,
		"kind":               kind,
		"enabled":            enabled,
		"params":             paramsStr,
		"secret_ref_mode":    mode,
		"secret_ref_env_key": envKey,
		"models":             modelsStr,
	}
}

func connsJSON(conns ...map[string]any) string {
	b, _ := json.Marshal(conns)
	return string(b)
}

func costsJSON(costs ...ModelCost) string {
	arr := make([]map[string]any, 0, len(costs))
	for _, c := range costs {
		arr = append(arr, map[string]any{
			"model":             c.Model,
			"input_usd_per_1k":  c.InputUSDPer1k,
			"output_usd_per_1k": c.OutputUSDPer1k,
		})
	}
	b, _ := json.Marshal(arr)
	return string(b)
}

// spec096TestSecretKeys is a synthetic managed-secret manifest used by the
// env-mode validation tests. DECLARED_PROVIDER_KEY stands in for a real
// managed secret key declared in infrastructure.secret_keys.
func spec096TestSecretKeys() []string {
	return []string{"LLM_PROVIDER_SECRET_MASTER_KEY", "DECLARED_PROVIDER_KEY"}
}

// curatedModels builds an inline models object for a hosted connection.
func curatedModels(id string) map[string]any {
	return map[string]any{
		"strategy": "curated",
		"list": []map[string]any{
			{"id": id, "tool_capable": true, "vision": true, "context_window": 200000},
		},
	}
}

// SCN-096-A01 — The registry represents N independent operator-global
// provider connections; each is an independent slot (id, kind, params)
// with NO actor_user_id (single shared graph), loaded closed-set.
func TestModelConnections_MultipleOperatorGlobalConnections_Spec096(t *testing.T) {
	conns := connsJSON(
		mkConn("local-ollama", ModelConnectionKindOllama, true,
			map[string]any{"base_url": "http://ollama:11434"}, "", "",
			map[string]any{"strategy": "live"}),
		mkConn("anthropic-primary", ModelConnectionKindAnthropic, true,
			nil, "db", "", curatedModels("claude-3-5-sonnet")),
		mkConn("openai-primary", ModelConnectionKindOpenAI, false,
			map[string]any{"org": "acme"}, "db", "", curatedModels("gpt-4o")),
		mkConn("azure-primary", ModelConnectionKindAzureFoundry, false,
			map[string]any{"endpoint": "https://acme.openai.azure.com", "api_version": "2024-06-01", "deployment": "gpt-4o"},
			"db", "", curatedModels("gpt-4o")),
		mkConn("google-primary", ModelConnectionKindGoogle, false,
			map[string]any{"project": "acme-proj", "location": "us-central1"}, "db", "", curatedModels("gemini-1.5-pro")),
		mkConn("bedrock-primary", ModelConnectionKindBedrock, false,
			map[string]any{"region": "us-east-1"}, "db", "", curatedModels("anthropic.claude-3-5-sonnet-20241022-v2:0")),
	)
	costs := costsJSON(ModelCost{Model: "anthropic/claude-3-5-sonnet", InputUSDPer1k: 0.003, OutputUSDPer1k: 0.015})

	cfg, err := parseModelConnections(conns, costs, 60000, 4000)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(cfg.Connections) != 6 {
		t.Fatalf("expected 6 operator-global connections, got %d", len(cfg.Connections))
	}
	// Distinct ids + kinds, generic params map, no per-user dimension.
	kinds := map[string]string{}
	for _, c := range cfg.Connections {
		if _, dup := kinds[c.ID]; dup {
			t.Fatalf("duplicate connection id %q", c.ID)
		}
		kinds[c.ID] = c.Kind
	}
	if kinds["azure-primary"] != ModelConnectionKindAzureFoundry {
		t.Fatalf("azure-primary kind mismatch: %q", kinds["azure-primary"])
	}
	// Params are carried generically (a map), not a typed field per kind.
	ollama := cfg.Connections[0]
	if ollama.Params["base_url"] != "http://ollama:11434" {
		t.Fatalf("ollama base_url not loaded into generic params map: %+v", ollama.Params)
	}
	// Operator-global: the struct has no actor_user_id field. Assert that
	// a JSON round-trip of a connection exposes no user dimension key.
	b, _ := json.Marshal(cfg.Connections[1])
	if strings.Contains(strings.ToLower(string(b)), "user_id") || strings.Contains(strings.ToLower(string(b)), "actoruser") {
		t.Fatalf("connection unexpectedly carries a per-user dimension: %s", b)
	}
	if err := cfg.Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("expected the multi-connection registry to validate, got: %v", err)
	}

	// Full env-pipeline path: LoadModelConnections parses the same wire
	// shape from env and validates against the real SecretKeys().
	t.Setenv("LLM_CONNECTIONS_JSON", connsJSON(
		mkConn("local-ollama", ModelConnectionKindOllama, true,
			map[string]any{"base_url": "http://ollama:11434"}, "", "",
			map[string]any{"strategy": "live"}),
	))
	t.Setenv("LLM_MODEL_COSTS_JSON", "[]")
	t.Setenv("LLM_DISCOVERY_CACHE_TTL_MS", "60000")
	t.Setenv("LLM_DISCOVERY_PER_PROVIDER_TIMEOUT_MS", "4000")
	loaded, err := LoadModelConnections()
	if err != nil {
		t.Fatalf("LoadModelConnections failed on a valid ollama-only registry: %v", err)
	}
	if len(loaded.Connections) != 1 || loaded.Connections[0].Kind != ModelConnectionKindOllama {
		t.Fatalf("LoadModelConnections parsed unexpected registry: %+v", loaded.Connections)
	}
}

// SCN-096-A01 (ADVERSARIAL) — an out-of-vocabulary kind aborts the registry
// build with a named error; passes only if the kind is in the closed set.
func TestModelConnections_UnknownKindRejectedFailLoud_Spec096(t *testing.T) {
	build := func(kind string) *ModelConnectionsConfig {
		cfg, err := parseModelConnections(
			connsJSON(mkConn("rogue", kind, false, map[string]any{"region": "us-east-1"}, "db", "", nil)),
			"[]", 60000, 4000)
		if err != nil {
			t.Fatalf("parse failed for kind %q: %v", kind, err)
		}
		return &cfg
	}

	// CONTROL: a valid kind passes — proves the rejection below is the
	// kind check, not an unrelated failure.
	if err := build(ModelConnectionKindBedrock).Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("control: a valid bedrock kind must pass, got: %v", err)
	}

	// ADVERSARIAL: an unknown kind aborts fail-loud, naming the slot + kind.
	err := build("grok").Validate(spec096TestSecretKeys())
	if err == nil {
		t.Fatal("expected an unknown kind to abort the registry build")
	}
	for _, want := range []string{"F096-SST-INVALID", "rogue", "unknown kind", "grok"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name %q, got: %v", want, err)
		}
	}
}

// SCN-096-A04 — the richest per-kind param set (azure-foundry
// endpoint+api_version+deployment) loads via the generic params map; a kind
// records its own params, not another kind's.
func TestModelConnections_PerKindParams_AzureFoundryRichest_Spec096(t *testing.T) {
	cfg, err := parseModelConnections(
		connsJSON(
			mkConn("azure-rich", ModelConnectionKindAzureFoundry, false,
				map[string]any{"endpoint": "https://acme.openai.azure.com", "api_version": "2024-06-01", "deployment": "gpt-4o"},
				"db", "", curatedModels("gpt-4o")),
			mkConn("anthropic-plain", ModelConnectionKindAnthropic, false,
				nil, "db", "", curatedModels("claude-3-5-sonnet")),
		),
		"[]", 60000, 4000)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	azure := cfg.Connections[0]
	for _, p := range []string{"endpoint", "api_version", "deployment"} {
		if !hasNonEmptyParam(azure.Params, p) {
			t.Fatalf("azure connection missing per-kind param %q in generic params map: %+v", p, azure.Params)
		}
	}
	// A kind records ITS OWN params, not another kind's: anthropic carries
	// none of azure's params.
	anthropic := cfg.Connections[1]
	if hasNonEmptyParam(anthropic.Params, "endpoint") || hasNonEmptyParam(anthropic.Params, "deployment") {
		t.Fatalf("anthropic connection unexpectedly carries azure params: %+v", anthropic.Params)
	}
	if err := cfg.Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("expected the azure-richest registry to validate, got: %v", err)
	}
}

// SCN-096-A04 (ADVERSARIAL) — a connection missing a required per-kind param
// fails loud NAMING the offending connection + param; passes with it present.
func TestModelConnections_MissingRequiredPerKindParam_FailsLoud_Spec096(t *testing.T) {
	build := func(params map[string]any) *ModelConnectionsConfig {
		cfg, err := parseModelConnections(
			connsJSON(mkConn("azure-x", ModelConnectionKindAzureFoundry, false, params, "db", "", nil)),
			"[]", 60000, 4000)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		return &cfg
	}

	// CONTROL: all three azure params present → passes.
	if err := build(map[string]any{"endpoint": "https://x", "api_version": "2024-06-01", "deployment": "gpt-4o"}).Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("control: complete azure params must pass, got: %v", err)
	}

	// ADVERSARIAL: drop "deployment" → fails loud naming the connection + param.
	err := build(map[string]any{"endpoint": "https://x", "api_version": "2024-06-01"}).Validate(spec096TestSecretKeys())
	if err == nil {
		t.Fatal("expected a missing required azure param to abort")
	}
	for _, want := range []string{"azure-x", "deployment", "missing required param"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name %q, got: %v", want, err)
		}
	}
}

// SCN-096-G02 (ADVERSARIAL) — a discovery cache_ttl_ms or
// per_provider_timeout_ms <= 0 aborts with a NAMED error and NO default.
func TestModelConnections_DiscoveryTtlNonPositive_AbortsNamed_Spec096(t *testing.T) {
	build := func(ttl, timeout int) *ModelConnectionsConfig {
		cfg, err := parseModelConnections(
			connsJSON(mkConn("local-ollama", ModelConnectionKindOllama, true,
				map[string]any{"base_url": "http://ollama:11434"}, "", "", map[string]any{"strategy": "live"})),
			"[]", ttl, timeout)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		return &cfg
	}

	// CONTROL: positive bounds pass.
	if err := build(60000, 4000).Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("control: positive discovery bounds must pass, got: %v", err)
	}

	// ADVERSARIAL: zero cache_ttl_ms aborts named.
	err := build(0, 4000).Validate(spec096TestSecretKeys())
	if err == nil || !strings.Contains(err.Error(), "cache_ttl_ms") {
		t.Fatalf("expected a named cache_ttl_ms abort, got: %v", err)
	}
	// ADVERSARIAL: negative per_provider_timeout_ms aborts named.
	err = build(60000, -1).Validate(spec096TestSecretKeys())
	if err == nil || !strings.Contains(err.Error(), "per_provider_timeout_ms") {
		t.Fatalf("expected a named per_provider_timeout_ms abort, got: %v", err)
	}
}

// SCN-096-G02 (ADVERSARIAL) — an env-mode secret_ref.env_key absent from
// infrastructure.secret_keys aborts naming the missing key; present passes.
func TestModelConnections_EnvModeSecretNotInSecretKeys_AbortsNamed_Spec096(t *testing.T) {
	build := func(envKey string) *ModelConnectionsConfig {
		cfg, err := parseModelConnections(
			connsJSON(mkConn("openai-env", ModelConnectionKindOpenAI, false, nil, "env", envKey, nil)),
			"[]", 60000, 4000)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		return &cfg
	}

	// CONTROL: an env_key declared in the manifest passes.
	if err := build("DECLARED_PROVIDER_KEY").Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("control: a declared env-mode secret key must pass, got: %v", err)
	}

	// ADVERSARIAL: an undeclared env_key aborts naming the key.
	err := build("UNDECLARED_PROVIDER_KEY").Validate(spec096TestSecretKeys())
	if err == nil {
		t.Fatal("expected an undeclared env-mode secret key to abort")
	}
	for _, want := range []string{"openai-env", "UNDECLARED_PROVIDER_KEY", "infrastructure.secret_keys"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name %q, got: %v", want, err)
		}
	}
}

// SCN-096-G02 (ADVERSARIAL) — an enabled non-ollama model with no
// llm.model_costs entry aborts fail-loud (never silent $0); and a missing
// required env var is a fail-loud load error. No default is ever substituted.
func TestModelConnections_NoDefaultsFailLoud_Spec096(t *testing.T) {
	// CONTROL: an enabled hosted model WITH a model_costs entry passes.
	withCost, err := parseModelConnections(
		connsJSON(mkConn("anthropic-paid", ModelConnectionKindAnthropic, true, nil, "db", "", curatedModels("claude-3-5-sonnet"))),
		costsJSON(ModelCost{Model: "anthropic/claude-3-5-sonnet", InputUSDPer1k: 0.003, OutputUSDPer1k: 0.015}),
		60000, 4000)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := withCost.Validate(spec096TestSecretKeys()); err != nil {
		t.Fatalf("control: an enabled paid model WITH a rate must pass, got: %v", err)
	}

	// ADVERSARIAL: the same enabled paid model with NO rate aborts named —
	// the registry never substitutes a silent $0 default.
	noCost, err := parseModelConnections(
		connsJSON(mkConn("anthropic-paid", ModelConnectionKindAnthropic, true, nil, "db", "", curatedModels("claude-3-5-sonnet"))),
		"[]", 60000, 4000)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	verr := noCost.Validate(spec096TestSecretKeys())
	if verr == nil {
		t.Fatal("expected an enabled paid model with no rate to abort fail-loud")
	}
	for _, want := range []string{"anthropic-paid", "model_costs", "anthropic/claude-3-5-sonnet"} {
		if !strings.Contains(verr.Error(), want) {
			t.Fatalf("error must name %q, got: %v", want, verr)
		}
	}

	// A missing required env var is a fail-loud [F096-SST-MISSING] load
	// error — no default is substituted for an absent registry value.
	t.Setenv("LLM_CONNECTIONS_JSON", "[]")
	t.Setenv("LLM_MODEL_COSTS_JSON", "[]")
	t.Setenv("LLM_DISCOVERY_CACHE_TTL_MS", "60000")
	t.Setenv("LLM_DISCOVERY_PER_PROVIDER_TIMEOUT_MS", "4000")
	if err := os.Unsetenv("LLM_DISCOVERY_CACHE_TTL_MS"); err != nil {
		t.Fatalf("unset failed: %v", err)
	}
	_, lerr := LoadModelConnections()
	if lerr == nil {
		t.Fatal("expected a missing LLM_DISCOVERY_CACHE_TTL_MS to fail loud")
	}
	for _, want := range []string{"F096-SST-MISSING", "LLM_DISCOVERY_CACHE_TTL_MS"} {
		if !strings.Contains(lerr.Error(), want) {
			t.Fatalf("missing-env error must name %q, got: %v", want, lerr)
		}
	}
}
