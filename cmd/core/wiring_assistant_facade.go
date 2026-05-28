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

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/assistant"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
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
	router, err := agent.NewRouter(ctx, agentRT.Config.Routing, scenarios, agent.NoopEmbedder{})
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

	// 5. FacadeConfig from the assistant SST envelope (SCOPE-01).
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
//   - retrieval_qa is the only scenario whose Final shape is
//     {answer, cited_artifact_ids} (config/prompt_contracts/
//     retrieval-qa-v1.yaml) and therefore the only scenario that
//     needs an explicit assembler today.
//
//   - Future scenarios with their own source-bearing output (e.g.
//     digest_recap citing recent artifacts, recommendation_explain
//     citing decision packets) will register their own assemblers
//     here. The hook itself in facade.go is generic.
//
//   - The artifact lookup is backed by *db.Postgres.GetArtifact so
//     the (title, capturedAt) tuple comes from the same authoritative
//     row the rest of the system reads. Test environments override
//     this with a fake lookup directly in the facade tests.
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
		"retrieval_qa": retrieval.NewFacadeAssembler(lookup, sourcesMax),
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
