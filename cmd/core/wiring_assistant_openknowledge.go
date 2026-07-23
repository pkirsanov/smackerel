// Spec 064 SCOPE-12 — live wiring for the open-knowledge subsystem.
//
// This file is the single startup glue that constructs and installs the
// open-knowledge agent loop behind the substrate `open_knowledge_invoke`
// tool. After wireOpenKnowledge returns, the spec 037 substrate
// executor can invoke the open_knowledge scenario (selected as the
// fallback per `agent.routing.fallback_scenario_id` in
// config/smackerel.yaml) and the substrate Handler will delegate to
// the real *okagent.Agent installed via agenttool.SetAgent.
//
// Construction order (rationale):
//
//  1. Read cfg.Assistant.OpenKnowledge. Enabled=false → no-op return.
//     The substrate Handler keeps its "agent not wired" refusal path
//     and the scenario YAML is filtered out of the router's
//     candidate set by the skills-manifest enable resolver.
//
//  2. LLM bridge client. Reuses cfg.MLSidecarURL + cfg.AuthToken
//     (same sidecar that hosts /embed) at endpoint /llm/chat with
//     the dedicated assistant.open_knowledge.llm_timeout_ms per-call
//     timeout. There is no separate open-knowledge LLM endpoint
//     today; this is the single sidecar that owns both /embed (used
//     by the assistant router) and /llm/chat (tool-use protocol).
//
//  3. Web search provider. Concrete impl is chosen by
//     cfg.Provider:
//     - "searxng" → web.NewSearxNG(endpoint, http.Client) — only
//     provider with a real implementation today.
//     - "brave" / "tavily" → ErrProviderNotConfigured stubs.
//     Future providers register here.
//
//  4. GraphSearcher. We re-use the pgx-backed text-similarity
//     adapter shipped in SCOPE-06 (tools.NewPgxGraphSearcher). A
//     pgvector/embedding-backed adapter that re-uses the spec-061
//     retrieval skill is deferred (see scopes.md SCOPE-06 finding);
//     the text-similarity adapter is sufficient to unblock the
//     gate-routing proof at SCOPE-12. cmd/core already holds the
//     live pgx pool via *db.Postgres.Pool.
//
//  5. Registry + tool registration. NewRegistry honors the SST
//     allowlist; RegisterAll wires the four production tools
//     (internal_retrieval, web_search, unit_convert, calculator).
//     A tool registered but not allowlisted is silently filtered
//     from Registry.Enabled() — the planner only sees what the
//     operator opted in.
//
//  6. Cite-back verifier — citeback.Verify is the pure function the
//     agent loop invokes per turn; no constructor needed.
//
//  7. Agent system prompt — loaded from
//     config/prompt_contracts/open_knowledge.yaml's
//     `agent_system_prompt` top-level field. The scenario loader
//     (internal/agent/loader.go::parseScenario) does NOT enforce
//     unknown-key rejection on top-level scenario fields, so the
//     field coexists with the substrate planner's `system_prompt`
//     block. NO-DEFAULTS (G028): an empty or missing prompt fails
//     loud at wiring time and the agent's New() also refuses an
//     empty SystemPrompt.
//
//  8. CostFn. Spec 096 SCOPE-05 makes the cost seam MODEL-AWARE over
//     the SST `llm.model_costs[]` rate table: an ollama/local model
//     costs $0 (the budget is not consumed), a paid provider-qualified
//     model is priced from its SST rate, and a billable model with NO
//     declared rate is refused fail-loud at dispatch (NEVER a silent
//     $0 — G028). With the SCOPE-05 spend ledger (migration 062) this
//     makes the per-query / monthly / per-user USD budgets load-bearing
//     for paid providers while keeping Ollama free.
//
//  9. agenttool.SetAgent installs the *okagent.Agent into the
//     package-level atomic pointer the substrate Handler reads on
//     every invocation.
//
// NO-DEFAULTS (Gate G028): every URL, key, model, budget, timeout,
// and allowlist value originates in cfg. There is no `||` fallback,
// no os.Getenv, no hidden default.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/smackerel/smackerel/internal/config"

	"github.com/smackerel/smackerel/internal/api"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/usageledger"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
	"github.com/smackerel/smackerel/internal/assistant/selfknowledge"

	"github.com/prometheus/client_golang/prometheus"
)

// compactionThresholdRatio is the in-code constant the agent loop
// uses to signal context-compaction. Not exposed through SST because
// it is a tuning constant for the loop itself, not an operator
// policy. Documented in agent.go::Config.CompactionThresholdRatio.
const compactionThresholdRatio = 0.85

// agentPromptYAMLField is the open_knowledge.yaml top-level key that
// carries the open-knowledge AGENT system prompt (the <CITATIONS>
// protocol prompt that drives the LLM ↔ tools loop). Distinct from
// the substrate planner's `system_prompt` block already consumed by
// the scenario loader.
const agentPromptYAMLField = "agent_system_prompt"

// agentPromptFileName is the scenario YAML basename inside the
// agent.scenario_dir directory.
const agentPromptFileName = "open_knowledge.yaml"

// wireOpenKnowledge builds the open-knowledge subsystem and installs
// the live agent behind agenttool.SetAgent. See the package doc
// comment for the staged contract.
//
// Returns nil when assistant.open_knowledge.enabled=false (no-op);
// returns a wrapped error when any required dep is missing or any
// construction step fails. Callers (cmd/core/main.go) MUST fail loud
// on a non-nil error per SST.
func wireOpenKnowledge(cfg *config.Config, svc *coreServices, agentScenarioDir string) error {
	if cfg == nil {
		return errors.New("wireOpenKnowledge: nil config")
	}
	okCfg := cfg.Assistant.OpenKnowledge
	if !okCfg.Enabled {
		slog.Info("open-knowledge subsystem disabled by SST (assistant.open_knowledge.enabled=false); leaving agenttool unbound")
		return nil
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		return errors.New("wireOpenKnowledge: postgres pool is required when open_knowledge is enabled")
	}
	if strings.TrimSpace(cfg.MLSidecarURL) == "" {
		return errors.New("wireOpenKnowledge: ML_SIDECAR_URL is required when assistant.open_knowledge.enabled=true (open-knowledge agent posts to <ML_SIDECAR_URL>/llm/chat)")
	}
	if agentScenarioDir == "" {
		return errors.New("wireOpenKnowledge: agentScenarioDir is empty; SCOPE-03 validator should have failed first")
	}

	// 2. LLM bridge client.
	llmTimeout := time.Duration(okCfg.LLMTimeoutMs) * time.Millisecond
	llmClient, err := llm.NewClient(llm.Config{
		EndpointURL: cfg.MLSidecarURL,
		AuthToken:   cfg.AuthToken,
		Timeout:     llmTimeout,
	})
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build LLM client: %w", err)
	}

	// 2b. Metrics — SCOPE-14 + SCOPE-15. Constructed early so we can
	//     pass the *Metrics into the SearxNG provider as a
	//     SuspiciousSnippetRecorder (SCOPE-15). The allowed-tool set
	//     comes from the SST allowlist directly; registry.Enabled()
	//     would give the same set (intersection of registered ∩
	//     allowlisted) once the four production tools register
	//     unconditionally. Register() runs after all collectors are
	//     in place, below.
	//
	//     Spec 104 SCOPE-04 — self_knowledge is ALWAYS-ON (FR-1): the
	//     operator cannot disable answering about smackerel itself.
	//     We force it into the effective allowlist (dedup-safe) and
	//     use that same slice for BOTH the metrics collector AND the
	//     registry below, preserving the "allowlist == registry.Enabled()"
	//     invariant this comment relies on.
	effectiveAllowlist := make([]string, 0, len(okCfg.ToolAllowlist)+1)
	selfKnowledgeAllowed := false
	for _, name := range okCfg.ToolAllowlist {
		effectiveAllowlist = append(effectiveAllowlist, name)
		if name == tools.SelfKnowledgeToolName {
			selfKnowledgeAllowed = true
		}
	}
	if !selfKnowledgeAllowed {
		effectiveAllowlist = append(effectiveAllowlist, tools.SelfKnowledgeToolName)
	}
	okMetrics := okmetrics.New(effectiveAllowlist, okCfg.Provider)

	// 3. Web search provider.
	webProvider, err := buildOpenKnowledgeWebProvider(okCfg, llmTimeout, okMetrics)
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build web provider: %w", err)
	}

	// 3b. SCOPE-16 — wrap the provider in a circuit breaker before
	//     it reaches the tool registry. The breaker emits state +
	//     trip metrics through okMetrics (CircuitStateRecorder
	//     interface satisfied via duck-typing). Config bounds come
	//     from assistant.open_knowledge.circuit_breaker.*; all three
	//     are required positive integers — the SST validator already
	//     rejected zero/negative values before we reach this point.
	cbCfg := web.CircuitConfig{
		FailureThreshold: okCfg.CircuitBreaker.FailureThreshold,
		OpenWindow:       time.Duration(okCfg.CircuitBreaker.OpenWindowSeconds) * time.Second,
		HalfOpenAfter:    time.Duration(okCfg.CircuitBreaker.HalfOpenAfterSeconds) * time.Second,
	}
	webProvider, err = web.NewCircuitBreaker(webProvider, cbCfg, web.WithCircuitStateRecorder(okMetrics))
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build circuit breaker: %w", err)
	}

	// 4. GraphSearcher — pgx text-similarity adapter (see SCOPE-06
	//    finding for the embedding-backed follow-up).
	graphSearcher := tools.NewPgxGraphSearcher(svc.pg.Pool)

	// 4b. Spec 104 SCOPE-04 — self_knowledge semantic searcher. Reuses
	//     the SAME ML-sidecar embedder the assistant router uses
	//     (buildAssistantEmbedder, embedder_mode SST) over the live pgx
	//     pool, scoped at query time to the smackerel_self namespace by
	//     the tool. On embedder failure the searcher returns a typed
	//     error (no keyword fallback) so a product meta-answer never
	//     degrades into an ungrounded guess.
	selfKnowledgeEmbedder, err := buildAssistantEmbedder(cfg)
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build embedder for self_knowledge: %w", err)
	}
	selfKnowledgeSearcher := tools.NewPgxSemanticSearcher(svc.pg.Pool, selfKnowledgeEmbedder)

	// 5. Registry + tool registration.
	registry := ok.NewRegistry(effectiveAllowlist)
	if err := tools.RegisterAll(registry, tools.Deps{
		GraphSearcher:     graphSearcher,
		WebSearchProvider: webProvider,
	}); err != nil {
		return fmt.Errorf("wireOpenKnowledge: register tools: %w", err)
	}

	// 5b. Spec 104 SCOPE-04 — register the always-on self_knowledge tool.
	//     It is registered SEPARATELY from RegisterAll (which owns the
	//     four generic tools) because it carries a distinct dependency
	//     (the namespace-scoped semantic searcher) and a fixed namespace
	//     binding. It is already present in effectiveAllowlist above.
	if err := registry.Register(tools.NewSelfKnowledge(selfKnowledgeSearcher, selfknowledge.SelfKnowledgeNamespace)); err != nil {
		return fmt.Errorf("wireOpenKnowledge: register self_knowledge tool: %w", err)
	}

	// 7. Agent system prompt.
	systemPrompt, err := loadOpenKnowledgeAgentPrompt(filepath.Join(agentScenarioDir, agentPromptFileName))
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: load agent system prompt: %w", err)
	}

	// 8. CostFn — spec 096 SCOPE-05 model-aware cost seam over the SST
	//    llm.model_costs[] rate table. ollama/local → $0 (budget not
	//    consumed); a paid provider-qualified model → its SST rate; a
	//    billable model with NO declared rate → a fail-loud refusal at
	//    dispatch (NEVER a silent $0 — G028). This makes the per-query /
	//    monthly / per-user USD budgets load-bearing for paid providers.
	modelRates := make(map[string]okagent.ModelRate, len(cfg.ModelConnections.ModelCosts))
	for _, mc := range cfg.ModelConnections.ModelCosts {
		modelRates[mc.Model] = okagent.ModelRate{
			InputUSDPer1k:  mc.InputUSDPer1k,
			OutputUSDPer1k: mc.OutputUSDPer1k,
		}
	}
	costFn := okagent.NewModelAwareCostFn(modelRates)

	// 8a. Spend ledger — spec 096 SCOPE-05 month-to-date USD accounting
	//     (migration 062). Makes the budget pre-flight load-bearing: a
	//     paid turn is refused before any provider call when the caller's
	//     month-to-date spend would breach a ceiling, and the realized
	//     cost is appended after a successful billable turn. The
	//     ollama/free path never reads or writes it (NFR-2). The
	//     claim-bound actor comes from the request context (auth), never
	//     a request body.
	spendLedger := usageledger.New(svc.pg.Pool)

	// 8b. Metrics registration — SCOPE-14. okMetrics was constructed
	//     above (step 2b) so SearxNG could be wired with it as a
	//     SuspiciousSnippetRecorder; register the collectors here
	//     against the application Registerer.
	if err := okMetrics.Register(prometheus.DefaultRegisterer); err != nil {
		return fmt.Errorf("wireOpenKnowledge: register metrics: %w", err)
	}

	// 9. Agent.
	agent, err := okagent.New(llmClient, registry, citeback.Verify, okagent.Config{
		SystemPrompt:               systemPrompt,
		Model:                      okCfg.LLMModelID,
		SynthesisModel:             okCfg.SynthesisModelID,
		SynthesisRetryBudget:       okCfg.SynthesisRetryBudget,
		MaxIterations:              okCfg.MaxIterations,
		PerQueryTokenBudget:        okCfg.PerQueryTokenBudget,
		PerQueryUSDBudget:          okCfg.PerQueryUSDBudget,
		MonthlyBudgetUSDRemaining:  okCfg.MonthlyBudgetUSD,
		PerUserMonthlyUSDRemaining: okCfg.PerUserMonthlyBudgetUSD,
		CompactionThresholdRatio:   compactionThresholdRatio,
		CostFn:                     costFn,
		SpendLedger:                spendLedger,
		Recorder:                   okMetrics,
		Tracer:                     svc.assistantTracer,
		Logger:                     slog.Default(),
		EnforcementMode:            okCfg.CitebackEnforcementMode,
		// BUG-064-002 DEFECT 3b — cap salvaged sources to the SST
		// assistant.sources_max (same cap the facade assembler applies
		// to the user-visible list).
		SourcesMax: cfg.Assistant.SourcesMax,
	})
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build agent: %w", err)
	}

	agenttool.SetAgent(agent)

	// Spec 088 — build + install the runtime switchable-model allowlist from
	// the SAME SST already loaded. Gated on open_knowledge.enabled (this whole
	// function early-returns when disabled), so agenttool.SwitchableModels() is
	// non-nil exactly when CurrentAgent() is. NewAllowlist fails loud on an
	// empty/un-profiled/envelope-busting set; the OllamaMemoryLimitMiB envelope
	// is 0 on dev (no daemon) so the co-residence check is skipped there
	// (config-generation already fails loud on an envelope-busting list).
	//
	// Spec 102 SCOPE-102-03 — the modelswitch allowlist operates on resident
	// MiB ints, so convert the KV-aware profiles to their real resident
	// footprint (weights + KV @ num_ctx × num_parallel) here, and gate the
	// co-residence check on the SST max_loaded_models posture: under on-demand
	// swap (==1) models are NEVER co-resident, so pass envelope 0 to skip the
	// co-resident sum (config generation already enforced per-model fit). Only
	// under co-resident posture (>1) is the real envelope passed.
	residentProfiles := make(map[string]int, len(cfg.MLModelMemoryProfiles))
	for name, p := range cfg.MLModelMemoryProfiles {
		residentProfiles[name] = p.ResidentMiB(cfg.OllamaNumParallel)
	}
	allowlistEnvelopeMiB := cfg.OllamaMemoryLimitMiB
	if cfg.OllamaMaxLoadedModels <= 1 {
		allowlistEnvelopeMiB = 0
	}
	allow, err := modelswitch.NewAllowlist(
		okCfg.SwitchableModels,
		residentProfiles,              // spec 102 — resident MiB (weights + KV)
		allowlistEnvelopeMiB,          // env ollama envelope (0 ⇒ co-residence check skipped)
		okCfg.LLMModelID,              // gather model (co-resident on the synthesis turn)
		okCfg.SynthesisModelID,        // baseline synthesis = the "default" in rejections
		okCfg.ToolCapableGatherModels, // spec 089 — tool-capable gather set (--gather-model= validates against this; baseline llm_model_id must be a member)
	)
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: build switchable allowlist: %w", err)
	}
	agenttool.SetSwitchableModels(allow)

	// Spec 089 — construct + install the per-user sticky preference store over
	// the same pgx pool. Both fast-paths' sticky read AND the /model CRUD
	// surfaces reach it via agenttool.ModelPref(); it is claim-bound — the
	// store keys ONLY on the actor id the surfaces thread (Telegram
	// resolveActorUserID / HTTP PASETO subject), never a request-body field.
	agenttool.SetModelPref(modelpref.NewPostgresStore(svc.pg.Pool))

	// Spec 096 SCOPE-07 — activate the deferred multi-provider discovery
	// aggregator + month-to-date USD budget + provider-aware dispatch resolver and
	// install them behind the late-bound agenttool singletons the unified
	// selection surfaces read. A no-op (089 fallback preserved) when no connection
	// is declared or there is no Postgres pool. spendLedger + the system-default
	// synthesis model are already in scope above. The /ask dispatch injection that
	// CONSUMES the resolver is the deferred follow-up — nothing reads the resolver
	// singleton yet.
	if err := wireSpec096DiscoveryAndDispatch(cfg, svc, spendLedger, okCfg.SynthesisModelID, okMetrics); err != nil {
		return fmt.Errorf("wireOpenKnowledge: spec096 discovery/dispatch activation: %w", err)
	}

	slog.Info("open-knowledge subsystem wired",
		openKnowledgeBootLogAttrs(okCfg, len(registry.Enabled()), "wired")...)
	return nil
}

// openKnowledgeBootLogAttrs builds the structured boot-log attributes that name
// the resolved open-knowledge model-selection surface — spec 089: the standing
// synthesis_model, the switchable + tool_capable_gather_models sets, and
// whether the sticky-preference store is wired. This is the operator's
// hot-swap verification hook: the runbook greps this line after a core recreate
// to confirm the new default is live. Extracted so the SCN-089-A13 test can
// assert the named fields without standing up a live subsystem.
func openKnowledgeBootLogAttrs(okCfg config.OpenKnowledgeConfig, toolCount int, stickyPrefStore string) []any {
	return []any{
		"provider", okCfg.Provider,
		"model", okCfg.LLMModelID,
		"synthesis_model", okCfg.SynthesisModelID,
		"switchable_models", okCfg.SwitchableModels,
		"tool_capable_gather_models", okCfg.ToolCapableGatherModels,
		"sticky_pref_store", stickyPrefStore,
		"synthesis_retry_budget", okCfg.SynthesisRetryBudget,
		"max_iterations", okCfg.MaxIterations,
		"per_query_token_budget", okCfg.PerQueryTokenBudget,
		"per_query_usd_budget", okCfg.PerQueryUSDBudget,
		"tool_count", toolCount,
	}
}

// buildOpenKnowledgeWebProvider selects the WebSearchProvider impl
// based on cfg.Provider. All three branches return a typed
// implementation; the stub providers surface ErrProviderNotConfigured
// from Search() so the agent's tool trace records the cause cleanly.
//
// SCOPE-15: the http.Client for SearxNG is wrapped in an
// EgressAllowlistTransport whose effective allowlist is the union of
// (a) the provider_endpoint host (always implicitly allowed — without
// it the provider could not be reached) and (b) cfg.AllowedEgressHosts
// (additional operator-sanctioned hosts; defaults to empty list, in
// which case only the provider host is reachable). The recorder is
// installed on the SearxNG adapter so the suspicious-snippet metric
// can fire on prompt-injection trigger patterns in returned content.
func buildOpenKnowledgeWebProvider(cfg config.OpenKnowledgeConfig, timeout time.Duration, recorder web.SuspiciousSnippetRecorder) (web.WebSearchProvider, error) {
	switch cfg.Provider {
	case config.OpenKnowledgeProviderSearxng:
		if strings.TrimSpace(cfg.ProviderEndpoint) == "" {
			return nil, errors.New("provider=searxng requires non-empty provider_endpoint")
		}
		// Effective allowlist = provider_endpoint host ∪ AllowedEgressHosts.
		endpointURL, err := neturl.Parse(cfg.ProviderEndpoint)
		if err != nil {
			return nil, fmt.Errorf("parse provider_endpoint: %w", err)
		}
		host := endpointURL.Hostname()
		if host == "" {
			return nil, errors.New("provider_endpoint must include a host")
		}
		effective := make([]string, 0, 1+len(cfg.AllowedEgressHosts))
		effective = append(effective, host)
		for _, h := range cfg.AllowedEgressHosts {
			effective = append(effective, h)
		}
		egress, err := web.NewEgressAllowlistTransport(effective, http.DefaultTransport)
		if err != nil {
			return nil, fmt.Errorf("build egress allowlist transport: %w", err)
		}
		client := &http.Client{Timeout: timeout, Transport: egress}
		return web.NewSearxNG(cfg.ProviderEndpoint, client, web.WithSuspiciousSnippetRecorder(recorder))
	case config.OpenKnowledgeProviderBrave:
		return web.NewBrave(), nil
	case config.OpenKnowledgeProviderTavily:
		return web.NewTavily(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (Validate() should have rejected this)", cfg.Provider)
	}
}

// loadOpenKnowledgeAgentPrompt reads the scenario YAML at path and
// returns the value of the `agent_system_prompt` top-level field.
// Empty / missing → typed error (G028 — no silent default; the agent
// New() also refuses empty SystemPrompt).
func loadOpenKnowledgeAgentPrompt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	raw, ok := top[agentPromptYAMLField]
	if !ok {
		return "", fmt.Errorf("%s: top-level field %q missing (REQUIRED for open-knowledge agent loop)", path, agentPromptYAMLField)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: field %q must be a string, got %T", path, agentPromptYAMLField, raw)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s: field %q is empty (REQUIRED non-empty per G028)", path, agentPromptYAMLField)
	}
	return s, nil
}

// modelProviderMasterKeyEpoch is the initial master-key epoch the connection
// vault encrypts under (secret_key_version). It is the rotation epoch, not a
// runtime SST value; a future rotation driver (SCOPE-02 §11.3) bumps it. The
// SCOPE-02 persistence round-trip is pinned to epoch 1.
const modelProviderMasterKeyEpoch = 1

// buildModelConnectionsAdmin constructs the Spec 096 SCOPE-06 runtime-plane
// wiring: the DB-backed connstore.Store (the SCOPE-03 CredentialSource AND the
// single effective-enabled predicate SCOPE-04 discovery consults), the operator
// gate (R1), and the operator-gated admin handler.
//
// It is fail-loud (G028): when a db-mode connection is declared (the
// credential-mutating /v1/admin/model-connections* surface is reachable) and
// infrastructure.operator_user_ids is empty in production, it refuses to start
// an open-by-default operator surface. In dev/test the surface runs fail-closed
// (the operator gate locks everyone out) with a warning.
//
// The connstore.Store is stashed on svc.modelConnStore so the (deferred) live
// dispatch-resolver / catalog-aggregator wiring reads the SAME seam the admin
// surface writes — the single effective-enabled gate across dispatch +
// discovery. Returns (nil, gate, nil) when there is no Postgres pool
// (config-validate mode) or no db-mode slot: the admin routes are then simply
// not mounted, but the gate is still validated.
func buildModelConnectionsAdmin(cfg *config.Config, svc *coreServices) (*api.ModelConnectionsAdminHandler, *api.OperatorGate, error) {
	if cfg == nil {
		return nil, nil, errors.New("buildModelConnectionsAdmin: nil config")
	}
	conns := cfg.ModelConnections.Connections
	dbModeDeclared := false
	for _, c := range conns {
		if c.SecretRef.Mode == config.ModelConnectionSecretModeDB {
			dbModeDeclared = true
			break
		}
	}
	// R1 / G028 fail-loud operator-gate guard: no open-by-default operator
	// surface. surfaceReachable == "a db-mode slot is declared".
	if err := api.ValidateOperatorGate(cfg.OperatorUserIDs, dbModeDeclared, cfg.Environment); err != nil {
		return nil, nil, err
	}
	gate := api.NewOperatorGate(cfg.OperatorUserIDs)

	if svc == nil || svc.pg == nil || svc.pg.Pool == nil || !dbModeDeclared {
		// config-validate mode / no DB, or no db-mode slot to administer: the
		// gate is validated but no live admin surface is mounted.
		return nil, gate, nil
	}

	// The master key is required iff a db-mode connection is declared
	// (connvault.LoadVault enforces the predicate); an Ollama-only deployment
	// needs no vault and no new secret.
	vault, err := connvault.LoadVault(os.Getenv("LLM_PROVIDER_SECRET_MASTER_KEY"), modelProviderMasterKeyEpoch, conns)
	if err != nil {
		return nil, nil, fmt.Errorf("model-connections admin: load credential vault: %w", err)
	}
	// SCOPE-07 — stash the vault so wireSpec096DiscoveryAndDispatch's dispatch
	// resolver decrypts hosted credentials through the SAME vault this surface
	// writes (this line is reached only when a db-mode connection is declared, so
	// a non-nil vault here implies hosted dispatch is possible).
	svc.modelConnVault = vault

	store := connstore.NewStore(svc.pg.Pool, conns)
	svc.modelConnStore = store // SCOPE-03 CredentialSource + SCOPE-04 effective-enabled seam

	timeout := time.Duration(cfg.ModelConnections.Discovery.PerProviderTimeoutMs) * time.Millisecond
	probe := api.NewHTTPConnectionProbe(&http.Client{Timeout: timeout}, timeout)
	handler := api.NewModelConnectionsAdminHandler(store, vault, probe)
	// Spec 096 §13 — stash the handler so wireSpec096DiscoveryAndDispatch can
	// late-bind its connection-test observability with the SAME okMetrics Recorder
	// the agent/catalog use (okMetrics is constructed later in wireOpenKnowledge,
	// after this runs) plus the boot tracer.
	svc.modelConnAdmin = handler

	slog.Info("spec096 scope06: model-connections admin surface wired",
		"operator_gate_configured", gate.Configured(),
		"note", "DB-backed CredentialSource + effective-enabled predicate ready for SCOPE-03 dispatch + SCOPE-04 discovery (live resolver/aggregator wiring deferred to SCOPE-07)")
	return handler, gate, nil
}

// wireSpec096DiscoveryAndDispatch is the Spec 096 SCOPE-07 live-wiring
// activation: it constructs the multi-provider discovery aggregator, the
// month-to-date USD budget source, and the provider-aware dispatch resolver, and
// installs all three behind the late-bound agenttool singletons the unified
// selection surfaces (and the deferred /ask dispatch) read.
//
// It mirrors buildModelConnectionsAdmin's guards. When there is no Postgres pool
// (config-validate mode) or no llm.connections[] are declared, it installs
// NOTHING: the agenttool catalog/budget/resolver singletons stay nil and BOTH
// surfaces keep the byte-for-byte spec-089 Ollama flat-list fallback. That nil
// fallback is the load-bearing 089 contract — it MUST be preserved.
//
// NO-DEFAULTS (G028): every discovery bound (cache_ttl_ms,
// per_provider_timeout_ms) and every provider base_url originates in cfg; there
// is no hardcoded TTL, timeout, or URL here.
//
// Adapter set vs resolver set (a deliberate asymmetry):
//   - Discovery adapters are built ONLY for EFFECTIVE-ENABLED connections — the
//     aggregator's documented precondition ("one adapter per effective-enabled
//     connection"). A disabled connection gets no adapter; a HostedAdapter
//     ALWAYS returns its curated models with StateOK, so including a disabled
//     connection would wrongly surface its models as reachable in the picker.
//   - The dispatch resolver is built from the FULL connection set so it can
//     reject a disabled / credential-less target fail-loud
//     (RejectConnectionDisabled / RejectCredentialMissing) — NEVER a silent
//     Ollama fallback (FR-X1). The fuller store-backed effective-enabled
//     predicate (db-mode credential presence) + live hosted reachability probes
//     are the deferred /ask-dispatch follow-up.
//
// Ordering: buildModelConnectionsAdmin (inside buildAPIDeps) runs BEFORE
// wireOpenKnowledge, so svc.modelConnVault / svc.modelConnStore are already
// stashed for a db-mode config by the time this runs; both are nil for the
// Ollama-only default (the resolver tolerates a nil vault and a nil credential
// source).
func wireSpec096DiscoveryAndDispatch(cfg *config.Config, svc *coreServices, spendLedger *usageledger.PostgresLedger, systemDefaultModel string, recorder okmetrics.Recorder) error {
	if cfg == nil {
		return errors.New("wireSpec096DiscoveryAndDispatch: nil config")
	}
	// Guard: config-validate mode (no DB pool) ⇒ install nothing, leave the 089
	// fallback (mirrors buildModelConnectionsAdmin).
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		slog.Info("spec096 scope07: discovery/dispatch activation skipped (no Postgres pool — config-validate mode); leaving byte-for-byte 089 fallback")
		return nil
	}
	conns := cfg.ModelConnections.Connections
	// Guard: no declared connection ⇒ install nothing, leave the 089 fallback.
	if len(conns) == 0 {
		slog.Info("spec096 scope07: discovery/dispatch activation skipped (no llm.connections declared); leaving byte-for-byte 089 fallback")
		return nil
	}

	// Build one discovery adapter per EFFECTIVE-ENABLED connection. The Ollama
	// (live) kind probes <base_url>/api/tags bounded by the SST per-provider
	// timeout; every hosted kind serves its SST-curated models[].
	discoveryTimeout := time.Duration(cfg.ModelConnections.Discovery.PerProviderTimeoutMs) * time.Millisecond
	adapters := make([]catalog.DiscoveryAdapter, 0, len(conns))
	for _, conn := range conns {
		if !conn.Enabled {
			continue
		}
		if conn.Kind == config.ModelConnectionKindOllama {
			// DISCOVERY probe target — resolve from the env-wired OLLAMA_URL
			// seam (cfg.OllamaURL), NOT the 096 connection registry's base_url
			// param. See ollamaDiscoveryBaseURL for the BUG-096-001 rationale
			// (the registry param is a dev-compose-DNS literal baked into the
			// build-once bundle; the deploy adapter re-points the host Ollama
			// daemon via OLLAMA_URL, the SAME seam /health and synthesis use).
			baseURL, err := ollamaDiscoveryBaseURL(cfg)
			if err != nil {
				return fmt.Errorf("wireSpec096DiscoveryAndDispatch: connection %q: %w", conn.ID, err)
			}
			adapters = append(adapters, catalog.NewOllamaAdapter(
				conn.ID,
				baseURL,
				&http.Client{Timeout: discoveryTimeout},
				ollamaCapabilityHints(conn),
			))
			continue
		}
		adapters = append(adapters, catalog.NewHostedAdapter(conn))
	}

	// Aggregator over the SST discovery bounds (fail-loud > 0 — no default).
	agg, err := catalog.NewCatalogAggregator(
		adapters,
		cfg.ModelConnections.Discovery.CacheTTLms,
		cfg.ModelConnections.Discovery.PerProviderTimeoutMs,
		systemDefaultModel,
	)
	if err != nil {
		return fmt.Errorf("wireSpec096DiscoveryAndDispatch: build catalog aggregator: %w", err)
	}
	// Spec 096 §13 — wire the discovery observability surface onto the aggregator:
	// the SAME openknowledge metrics Recorder the agent uses (per-provider
	// reachability counter + latency histogram) and the boot tracer
	// (model.discovery → provider.discover spans). nil-safe: a no-op tracer (otel
	// disabled) keeps the spans no-ops, and discovery touches NO credential, so
	// nothing emitted here can carry a secret.
	agg.WithObservability(recorder, svc.assistantTracer)
	// Spec 096 §13 — late-bind the operator connection-test observability onto the
	// admin handler (built earlier in buildModelConnectionsAdmin, before okMetrics
	// existed) with the SAME recorder + boot tracer the aggregator/agent use.
	// nil-safe: the handler is non-nil exactly when a db-mode connection is
	// declared. SECRET-SAFETY: the seam only carries connection_id/kind/outcome.
	if svc.modelConnAdmin != nil {
		svc.modelConnAdmin.WithObservability(recorder, svc.assistantTracer)
	}
	agenttool.SetModelCatalogProvider(agg)

	// Month-to-date USD budget source (SCOPE-05 spend ledger). The picker
	// surfaces a budget line ONLY when a paid model is in the catalog.
	agenttool.SetBudgetProvider(spendLedger)

	// Provider-aware dispatch resolver over the FULL connection set. A nil
	// credential source (no db-mode connection ⇒ svc.modelConnStore is a typed
	// nil) MUST be passed as a true nil interface, not a typed-nil *connstore.Store,
	// so the resolver's nil-source path reports "no credential" instead of
	// dereferencing a nil receiver. The vault is a concrete pointer the resolver
	// nil-checks directly, so a nil vault is safe to pass as-is.
	var creds llm.CredentialSource
	if svc.modelConnStore != nil {
		creds = svc.modelConnStore
	}
	resolver, err := llm.NewDispatchResolver(conns, svc.modelConnVault, creds)
	if err != nil {
		return fmt.Errorf("wireSpec096DiscoveryAndDispatch: build dispatch resolver: %w", err)
	}
	agenttool.SetDispatchResolver(resolver)

	slog.Info("spec096 scope07: discovery/dispatch activated",
		"discovery_adapters", len(adapters),
		"connections_declared", len(conns),
		"catalog_installed", true,
		"budget_installed", true,
		"resolver_installed", true,
		"note", "combined catalog + budget + provider-aware dispatch resolver wired; the /ask dispatch loop consumes the resolver (hosted models dispatch to their provider; bare/ollama stay on the 089 path)")
	return nil
}

// ollamaDiscoveryBaseURL resolves the base URL the Ollama DISCOVERY probe
// (GET <base>/api/tags, SCOPE-04) targets. It comes from the env-wired
// OLLAMA_URL seam (cfg.OllamaURL) — the SAME host-Ollama URL the /health probe
// and the ML-sidecar synthesis path consume — NOT the 096 connection registry's
// base_url param.
//
// BUG-096-001: the registry base_url param is a fixed dev compose-service DNS
// name (host `ollama`, not a host-routable URL) carried verbatim in the
// build-once bundle; it is NOT re-pointed per target. On the single-host
// self-hosted topology the local Ollama daemon is a HOST singleton (no in-stack
// `ollama` compose service), so
// the deploy adapter re-points OLLAMA_URL / OLLAMA_BASE_URL to the host tailnet
// IP. Discovery MUST follow that same env seam or it probes compose DNS
// (NXDOMAIN) and falsely reports local-ollama "unreachable" while synthesis —
// which routes through the sidecar's OLLAMA_URL — works.
//
// Fail loud on an empty seam — NEVER a substituted compose-DNS default
// (G028 / smackerel-no-defaults). scripts/commands/config.sh always emits
// OLLAMA_URL from the REQUIRED llm.ollama_url SST value, so an empty value here
// is a genuine mis-provision, not an expected state.
func ollamaDiscoveryBaseURL(cfg *config.Config) (string, error) {
	s := strings.TrimSpace(cfg.OllamaURL)
	if s == "" {
		return "", errors.New("OLLAMA_URL is required to probe the local Ollama daemon for model discovery (env-wired seam; no compose-DNS default)")
	}
	return s, nil
}

// ollamaCapabilityHints maps an Ollama connection's OPTIONAL operator-curated
// models[] into the per-bare-name capability hints the OllamaAdapter stamps onto
// live-discovered models (the /api/tags payload does not report capabilities).
// The default live-strategy ollama connection declares no list ⇒ nil ⇒ the zero
// capability triplet, identical to passing nil. This consumes an existing
// OPTIONAL SST field; it invents no param (G028).
func ollamaCapabilityHints(conn config.ModelConnection) map[string]catalog.ModelCapabilities {
	if len(conn.Models.List) == 0 {
		return nil
	}
	hints := make(map[string]catalog.ModelCapabilities, len(conn.Models.List))
	for _, m := range conn.Models.List {
		hints[m.ID] = catalog.ModelCapabilities{
			ToolCapable:   m.ToolCapable,
			Vision:        m.Vision,
			ContextWindow: m.ContextWindow,
		}
	}
	return hints
}
