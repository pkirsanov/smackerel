// Spec 061 SCOPE-05 — capability-layer facade + Telegram reference
// adapter wiring.
//
// This file is the single startup glue that:
//
//  1. Builds an assistant.Facade from the assistant SST envelope
//     (SCOPE-01) plus the loaded agent scenarios (Spec 037 substrate
//     reused via the agentRuntime bundle returned from
//     wireAgentBridge).
//  2. Loads the spec 061 skills manifest from
//     config/assistant/scenarios.yaml (SUBSTRATE-TOUCHPOINT-1
//     Option (b)) using the same EnableResolver that SCOPE-03's
//     scenarios validator consumes.
//  3. Constructs the PostgreSQL-backed context store + idle-sweep
//     ticker (SCOPE-04 substrate).
//  4. Builds the Telegram reference adapter (SCOPE-05) and binds it
//     to the already-running *telegram.Bot via SetAssistantAdapter
//     + Adapter.Start.
//
// Fail-loud per SST: every required dependency is checked at
// construction time. When the assistant block is SST-disabled
// (cfg.Assistant.Enabled == false) the function short-circuits as a
// no-op so legacy installs continue through the BS-001 fallthrough.
// When the bot is not configured (telegram disabled) we still build
// the facade (so other transports can attach in v2) but skip the
// adapter step.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/embedder/sidecar"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/assistant"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
	"github.com/smackerel/smackerel/internal/assistant/skills/recipesearch"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// assistantDisambigMaxChoices is the design-fixed cap (design.md §3.2)
// on the number of non-save_as_note Choices the facade emits per
// DisambiguationPrompt. It is NOT exposed through SST because it is
// a UX constant — three is the maximum that fits one Telegram inline
// keyboard row without horizontal scroll on the smallest device.
const assistantDisambigMaxChoices = 3

// wireAssistantFacade builds the spec 061 capability layer + adapter
// and binds it to the supplied *telegram.Bot. See the package doc
// comment for the staged contract.
func wireAssistantFacade(
	ctx context.Context,
	cfg *config.Config,
	svc *coreServices,
	agentRT *agentRuntime,
	tgBot *telegram.Bot,
	agentScenarioDir string,
) error {
	if cfg == nil {
		return errors.New("wireAssistantFacade: nil config")
	}
	if !cfg.Assistant.Enabled {
		slog.Info("assistant disabled by SST (assistant.enabled=false); skipping facade wiring")
		return nil
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		return errors.New("wireAssistantFacade: postgres pool is required")
	}
	if agentRT == nil || agentRT.Bridge == nil || agentRT.Executor == nil || agentRT.Config == nil {
		return errors.New("wireAssistantFacade: agentRuntime is required (agent bridge wiring must run first)")
	}
	if agentScenarioDir == "" {
		return errors.New("wireAssistantFacade: agentScenarioDir is empty; SCOPE-03 validator must have failed first")
	}

	// 1. Load skills manifest (sibling lookup file per
	//    SUBSTRATE-TOUCHPOINT-1 Option (b)).
	manifestPath := filepath.Join(filepath.Dir(agentScenarioDir), assistantManifestRelPath)
	manifest, err := assistant.LoadSkillsManifest(manifestPath, assistantEnableResolver(cfg))
	if err != nil {
		return fmt.Errorf("load skills manifest %s: %w", manifestPath, err)
	}

	// 2. Build a parallel Router over the bridge's loaded scenarios.
	//    The Bridge holds a Router internally; we build a second one
	//    here so the facade has a stable handle. Hot reload (Spec
	//    037 BS-019) is owned by the Bridge for its own Invoke path;
	//    v1 capability layer reads the snapshot at startup. SCOPE-09+
	//    will wire reload propagation when scenario hot-add becomes
	//    a user-visible requirement.
	scenarios := agentRT.Bridge.Scenarios()
	embedder, err := buildAssistantEmbedder(cfg)
	if err != nil {
		return fmt.Errorf("build assistant embedder: %w", err)
	}
	router, err := agent.NewRouter(ctx, agentRT.Config.Routing, scenarios, embedder)
	if err != nil {
		return fmt.Errorf("build assistant router: %w", err)
	}

	// 3. Build scenario registry for the facade's executor lookup.
	registry := newAssistantRegistry(scenarios)

	// 4. PostgreSQL context store + idle-sweep ticker.
	contextStore := assistantctx.NewPgStore(svc.pg.Pool)
	ticker, err := assistantctx.NewIdleSweepTicker(
		contextStore,
		cfg.Assistant.ContextIdleTimeout,
		cfg.Assistant.ContextIdleSweepInterval,
		slog.Default(),
	)
	if err != nil {
		return fmt.Errorf("idle-sweep ticker: %w", err)
	}
	go ticker.Run(ctx)

	// 4b. Active-threads gauge refresher (spec 061 SCOPE-09). Samples
	//     CountActiveByTransport on the same cadence as the idle-sweep
	//     ticker and pushes per-transport counts into
	//     assistantmetrics.ActiveThreadsGauge. Zero-fills the closed
	//     transport vocabulary so dashboards reflect empty transports
	//     as 0 rather than freezing at a stale Set().
	refresher, err := assistantctx.NewActiveThreadsRefresher(
		contextStore,
		assistantmetrics.AllTransports,
		cfg.Assistant.ContextIdleSweepInterval,
		func(transport string, count float64) {
			assistantmetrics.ActiveThreadsGauge.WithLabelValues(transport).Set(count)
		},
		slog.Default(),
	)
	if err != nil {
		return fmt.Errorf("active-threads refresher: %w", err)
	}
	go refresher.Run(ctx)

	// 5. FacadeConfig from the assistant SST envelope (SCOPE-01).
	legacyPolicy, err := buildLegacyRetirementPolicy(cfg, svc.pg.Pool, time.Now)
	if err != nil {
		return fmt.Errorf("build legacy retirement policy: %w", err)
	}
	facadeCfg := assistant.FacadeConfig{
		BorderlineFloor:      cfg.Assistant.BorderlineFloor,
		AgentConfidenceFloor: agentRT.Config.Routing.ConfidenceFloor,
		SourcesMax:           cfg.Assistant.SourcesMax,
		BodyMaxChars:         cfg.Assistant.BodyMaxChars,
		WindowTurns:          cfg.Assistant.ContextWindowTurns,
		DisambigMaxChoices:   assistantDisambigMaxChoices,
		DisambigTimeout:      cfg.Assistant.DisambiguateTimeout,
		Now:                  time.Now,
		SourceAssemblers:     buildAssistantSourceAssemblers(svc, cfg.Assistant.SourcesMax),
		Policy:               legacyPolicy,
	}
	audit := assistant.NewNoopAuditWriter() // SCOPE-08 swaps in PG/NATS-backed writer
	facade, err := assistant.NewFacade(
		facadeCfg,
		router,
		agentRT.Executor,
		registry,
		manifest,
		contextStore,
		audit,
	)
	if err != nil {
		return fmt.Errorf("build assistant facade: %w", err)
	}
	// Spec 061 SCOPE-09b — thread the OTel tracer (initialized in
	// services.go::wireAssistantTracing) into the facade so it can
	// emit `assistant.facade.handle` and its 6 mandatory child spans
	// per design §8.3.1.A. The no-op default installed by NewFacade
	// stays in place if svc.assistantTracer is nil (defensive — every
	// production wiring path sets it).
	facade.WithTracer(svc.assistantTracer)
	slog.Info("assistant facade wired",
		"scenarios", len(scenarios),
		"borderline_floor", facadeCfg.BorderlineFloor,
		"agent_confidence_floor", facadeCfg.AgentConfidenceFloor,
	)

	// 6. Telegram reference adapter — only when the bot is configured
	//    AND the assistant SST opts the telegram transport in.
	if tgBot == nil {
		slog.Info("telegram bot not configured; assistant facade ready but no transport bound")
		return nil
	}
	if !cfg.Assistant.TelegramEnabled {
		slog.Info("assistant.transports.telegram.enabled=false; facade built but telegram adapter not bound")
		return nil
	}
	mode, ok := parseAssistantMarkdownMode(cfg.Assistant.TelegramMarkdownMode)
	if !ok {
		return fmt.Errorf("assistant.transports.telegram.markdown_mode %q is not one of MarkdownV2/plain/HTML",
			cfg.Assistant.TelegramMarkdownMode)
	}
	adapter, err := assistant_adapter.NewAdapter(assistant_adapter.Options{
		Sender:          telegram.NewBotSender(tgBot),
		Capture:         telegram.NewBotCaptureFn(tgBot),
		ResolveUser:     telegram.NewBotChatResolver(tgBot),
		MarkdownMode:    mode,
		MaxMessageChars: cfg.Assistant.TelegramMaxMessageChars,
		// Spec 061 SCOPE-09b — thread the OTel tracer into the
		// adapter so HandleUpdate emits the canonical root
		// `assistant.adapter.translate` span and the sibling
		// `assistant.adapter.render` child span per design §8.3.1.A.
		Tracer: svc.assistantTracer,
	})
	if err != nil {
		return fmt.Errorf("build telegram assistant adapter: %w", err)
	}
	if err := adapter.Start(ctx, facade); err != nil {
		return fmt.Errorf("bind facade to telegram adapter: %w", err)
	}
	tgBot.SetAssistantAdapter(adapter)
	slog.Info("assistant Telegram adapter wired and bound to bot",
		"markdown_mode", string(mode),
		"max_message_chars", cfg.Assistant.TelegramMaxMessageChars,
	)
	return nil
}

// parseAssistantMarkdownMode maps the SST string to the closed
// vocabulary the adapter accepts. The empty case is rejected by SST
// validation upstream; this function is defensive.
func parseAssistantMarkdownMode(s string) (assistant_adapter.MarkdownMode, bool) {
	switch s {
	case string(assistant_adapter.MarkdownV2):
		return assistant_adapter.MarkdownV2, true
	case string(assistant_adapter.PlainText):
		return assistant_adapter.PlainText, true
	case string(assistant_adapter.HTML):
		return assistant_adapter.HTML, true
	default:
		return "", false
	}
}

// assistantRegistry is the facade's ScenarioRegistry implementation,
// keyed by scenario ID over the loaded substrate scenarios.
type assistantRegistry struct {
	byID map[string]*agent.Scenario
}

func newAssistantRegistry(scenarios []*agent.Scenario) *assistantRegistry {
	r := &assistantRegistry{byID: make(map[string]*agent.Scenario, len(scenarios))}
	for _, sc := range scenarios {
		if sc == nil {
			continue
		}
		r.byID[sc.ID] = sc
	}
	return r
}

// Scenario implements assistant.ScenarioRegistry.
func (r *assistantRegistry) Scenario(id string) (*agent.Scenario, bool) {
	sc, ok := r.byID[id]
	return sc, ok
}

// buildAssistantSourceAssemblers builds the v1 per-scenario
// source-assembly registry the Facade consults between executor.Run
// and provenance.Enforce (spec 061 SCOPE-04 facade source-assembly
// hook).
//
// Wiring rationale:
//
//   - retrieval_qa emits {answer, cited_artifact_ids} (config/
//     prompt_contracts/retrieval-qa-v1.yaml) — assembled via a
//     PostgreSQL-backed artifact lookup.
//
//   - weather_query emits {forecast_line, provider_name,
//     retrieved_at} (config/prompt_contracts/weather-query-v1.yaml)
//     — assembled into exactly one Source{Kind:
//     SourceExternalProvider, Ref: ExternalProviderRef{...}}
//     (design §5.2). No artifact lookup is needed; the assembler is
//     pure parse + map.
//
//   - notification_schedule (SCOPE-08) is requires_provenance:false
//     because the scheduler record IS the provenance (design §5.3
//     YAML header). The Facade therefore wires NO assembler for it
//     — the gate skips notification responses entirely and the
//     facade renders proposed_action directly.
//
//   - Future scenarios with their own source-bearing output will
//     register their own assemblers here. The hook itself in
//     facade.go is generic.
//
// The function returns nil when the postgres pool is not configured;
// the Facade interprets a nil map as "no assemblers wired" and
// produces empty Sources for every scenario, which is the correct
// behavior for that misconfigured state (provenance gate refuses
// every requires_provenance scenario, fail-loud).
func buildAssistantSourceAssemblers(svc *coreServices, sourcesMax int) map[string]contracts.SourceAssembler {
	if svc == nil || svc.pg == nil {
		return nil
	}
	if sourcesMax <= 0 {
		return nil
	}
	lookup := newPostgresArtifactLookup(svc)
	return map[string]contracts.SourceAssembler{
		"retrieval_qa":   retrieval.NewFacadeAssembler("retrieval_qa", lookup, sourcesMax),
		"weather_query":  weather.NewFacadeAssembler(sourcesMax),
		"recipe_search":  recipesearch.NewFacadeAssembler(lookup, sourcesMax),
		"open_knowledge": newOpenKnowledgeAssembler(sourcesMax),
	}
}

// buildAssistantEmbedder selects the agent.Embedder implementation
// from the assistant.routing.embedder_mode SST key.
//
// BUG-061-004 — production wiring; replaces the unconditional
// agent.NoopEmbedder{} that hid an alphabetical-tie-break bug for
// months. "sidecar" mode requires cfg.MLSidecarURL + cfg.AuthToken;
// "noop" mode is rejected in non-dev/test environments to prevent a
// silent prod regression.
func buildAssistantEmbedder(cfg *config.Config) (agent.Embedder, error) {
	switch cfg.Assistant.EmbedderMode {
	case "sidecar":
		if strings.TrimSpace(cfg.MLSidecarURL) == "" {
			return nil, errors.New("buildAssistantEmbedder: ML_SIDECAR_URL required when embedder_mode=sidecar")
		}
		if strings.TrimSpace(cfg.AuthToken) == "" {
			return nil, errors.New("buildAssistantEmbedder: SMACKEREL_AUTH_TOKEN required when embedder_mode=sidecar")
		}
		return sidecar.New(cfg.MLSidecarURL, cfg.AuthToken, cfg.Assistant.EmbedTimeout)
	case "noop":
		env := strings.ToLower(strings.TrimSpace(cfg.Environment))
		if env != "" && env != "development" && env != "dev" && env != "test" {
			return nil, fmt.Errorf("buildAssistantEmbedder: embedder_mode=noop forbidden in environment %q (BUG-061-004 D8)", cfg.Environment)
		}
		slog.Warn("assistant routing using NoopEmbedder — test/dev only; NL routing collapses to alphabetical tie-break")
		return agent.NoopEmbedder{}, nil
	default:
		return nil, fmt.Errorf("buildAssistantEmbedder: unknown embedder_mode %q (validator rejects this earlier)", cfg.Assistant.EmbedderMode)
	}
}

// newPostgresArtifactLookup returns an ArtifactLookupFn backed by
// *db.Postgres.GetArtifact. The lookup distinguishes "not found"
// (pgx.ErrNoRows wrapped by GetArtifact's %w error chain) from
// transient errors so the assembler can attribute drops correctly
// (missing_artifact vs lookup_error counters per design §5.1).
func newPostgresArtifactLookup(svc *coreServices) retrieval.ArtifactLookupFn {
	pg := svc.pg
	return func(ctx context.Context, artifactID string) (string, time.Time, bool, error) {
		a, err := pg.GetArtifact(ctx, artifactID)
		if err != nil {
			// GetArtifact wraps with %w so errors.Is sees the
			// underlying pgx.ErrNoRows. The wrapped message also
			// contains the pgx sentinel text "no rows in result
			// set" as a defense-in-depth fallback for older callers.
			if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
				return "", time.Time{}, false, nil
			}
			return "", time.Time{}, false, err
		}
		if a == nil {
			return "", time.Time{}, false, nil
		}
		return a.Title, a.CreatedAt, true, nil
	}
}

// buildLegacyRetirementPolicy assembles the spec 075 SCOPE-6.2
// legacy-retirement dispatcher from the LegacyRetirement SST block.
// It wires the SST catalog, the Postgres-backed dedup ledger and
// residual store, the SST window-state resolver (with a static
// not-paused reader until Scope 4 wires the threshold-driven writer),
// the HMAC user-bucket hasher, and a MultiResidualTelemetry that
// fans Record() out to the Prometheus counters and the SQL residual
// store. Fail-loud on every missing SST key / nil dependency
// (G028/G029, no-defaults).
func buildLegacyRetirementPolicy(cfg *config.Config, pool *pgxpool.Pool, now func() time.Time) (legacyretirement.Policy, error) {
	if cfg == nil {
		return nil, errors.New("buildLegacyRetirementPolicy: nil config")
	}
	if pool == nil {
		return nil, errors.New("buildLegacyRetirementPolicy: postgres pool is required")
	}
	if now == nil {
		return nil, errors.New("buildLegacyRetirementPolicy: clock is required")
	}
	lr := cfg.LegacyRetirement
	catalog, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand:          lr.NoticeCopyPerCommand,
		PostWindowUnknownResponseCopy: lr.PostWindowUnknownResponseCopy,
	})
	if err != nil {
		return nil, fmt.Errorf("legacy retirement catalog: %w", err)
	}
	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		return nil, fmt.Errorf("legacy retirement ledger: %w", err)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{WindowID: lr.WindowID, WindowState: lr.WindowState},
		legacyretirement.NewStaticPauseStateReader(false),
	)
	if err != nil {
		return nil, fmt.Errorf("legacy retirement state resolver: %w", err)
	}
	hasher, err := legacyretirement.NewUserBucketHasher(lr.UserBucketHMACKey)
	if err != nil {
		return nil, fmt.Errorf("legacy retirement bucket hasher: %w", err)
	}
	prom := legacyretirement.NewPrometheusResidualTelemetry()
	sql, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: lr.WindowID,
		Clock:    now,
	})
	if err != nil {
		return nil, fmt.Errorf("legacy retirement residual store: %w", err)
	}
	telemetry := legacyretirement.NewMultiResidualTelemetry(prom, sql)
	policy, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       catalog,
		Ledger:        ledger,
		StateResolver: resolver,
		Telemetry:     telemetry,
		BucketHasher:  hasher,
		WindowID:      lr.WindowID,
		Clock:         now,
	})
	if err != nil {
		return nil, fmt.Errorf("legacy retirement policy: %w", err)
	}
	return policy, nil
}
