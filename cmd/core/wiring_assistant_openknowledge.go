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
//  8. CostFn. Spec 064 SCOPE-09 contracts a USD/token rate from
//     SST. We do not yet have a provider-priced rate table on
//     OpenKnowledgeConfig; the only operator-meaningful caps are
//     the per-query/monthly USD budgets. We therefore install a
//     zero-cost CostFn at wiring time (every call charges $0 USD),
//     which keeps the per-query USD budget effectively unconsumed
//     by LLM round-trips. Token caps + iteration caps still bind.
//     Adding a per-provider rate is owned by a follow-up scope and
//     tracked as a finding in report.md; that future work supplies
//     a CostFn that multiplies tokens by the operator's rate.
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

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"

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
	okMetrics := okmetrics.New(okCfg.ToolAllowlist, okCfg.Provider)

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

	// 5. Registry + tool registration.
	registry := ok.NewRegistry(okCfg.ToolAllowlist)
	if err := tools.RegisterAll(registry, tools.Deps{
		GraphSearcher:     graphSearcher,
		WebSearchProvider: webProvider,
	}); err != nil {
		return fmt.Errorf("wireOpenKnowledge: register tools: %w", err)
	}

	// 7. Agent system prompt.
	systemPrompt, err := loadOpenKnowledgeAgentPrompt(filepath.Join(agentScenarioDir, agentPromptFileName))
	if err != nil {
		return fmt.Errorf("wireOpenKnowledge: load agent system prompt: %w", err)
	}

	// 8. CostFn — zero-cost stub. Token + iteration caps still bind.
	//    The per-query USD budget is therefore not exercised by LLM
	//    calls under this CostFn; that limitation is recorded in
	//    report.md as a SCOPE-12 follow-up finding.
	costFn := okagent.CostFn(func(int) float64 { return 0 })

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
		MaxIterations:              okCfg.MaxIterations,
		PerQueryTokenBudget:        okCfg.PerQueryTokenBudget,
		PerQueryUSDBudget:          okCfg.PerQueryUSDBudget,
		MonthlyBudgetUSDRemaining:  okCfg.MonthlyBudgetUSD,
		PerUserMonthlyUSDRemaining: okCfg.PerUserMonthlyBudgetUSD,
		CompactionThresholdRatio:   compactionThresholdRatio,
		CostFn:                     costFn,
		Recorder:                   okMetrics,
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
	slog.Info("open-knowledge subsystem wired",
		"provider", okCfg.Provider,
		"model", okCfg.LLMModelID,
		"max_iterations", okCfg.MaxIterations,
		"per_query_token_budget", okCfg.PerQueryTokenBudget,
		"per_query_usd_budget", okCfg.PerQueryUSDBudget,
		"tool_count", len(registry.Enabled()),
	)
	return nil
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
