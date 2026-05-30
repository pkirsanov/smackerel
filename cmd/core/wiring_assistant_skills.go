// Spec 061 SCOPE-06 — per-skill runtime services wiring for the
// agent tool registry. This file is the only place production code
// is allowed to call retrieval.SetServices(...) — keeping all
// skill-services wiring colocated lets a reviewer audit "who injects
// what into the Spec 037 tool registry" in one read.
//
// The wiring runs once at startup AFTER wireAgentBridge has populated
// the tool registry via blank-import side effects and BEFORE
// wireAssistantFacade is invoked (the facade ultimately drives
// executor.Run, which dispatches retrieval_search through the
// registry; if SetServices has not happened by then the handler
// returns the canonical retrieval_tools_not_configured envelope and
// the response surfaces as a failed tool call to the operator).
//
// Per skill, the SST gate is also applied here: when the
// per-skill *Enabled bool is false the SetServices call is skipped,
// which means the tool handler will return its
// retrieval_tools_not_configured envelope instead of attempting a
// search. Operators who flip the skill off via SST get the same
// loud failure as forgetting to wire the skill — there is no silent
// "disabled but registered" mode.
//
// SCOPE-07 and SCOPE-08 will append weather.SetServices and
// notifications.SetServices calls here when those skills land.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/smackerel/smackerel/internal/agent/tools/notification"
	"github.com/smackerel/smackerel/internal/agent/tools/recipesearch"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/config"
)

// wireAssistantSkillServices injects production dependencies into
// every assistant skill registered in the agent tool registry.
// Returns nil with a single INFO log when the assistant capability
// is disabled by SST so other startup paths short-circuit cleanly.
func wireAssistantSkillServices(cfg *config.Config, svc *coreServices) error {
	if cfg == nil {
		return errors.New("wireAssistantSkillServices: nil config")
	}
	if !cfg.Assistant.Enabled {
		slog.Info("assistant disabled by SST (assistant.enabled=false); skipping skill-services wiring")
		return nil
	}
	if svc == nil {
		return errors.New("wireAssistantSkillServices: nil coreServices")
	}

	if err := wireRetrievalSkillServices(cfg, svc); err != nil {
		return fmt.Errorf("retrieval skill services: %w", err)
	}
	if err := wireWeatherSkillServices(cfg, svc); err != nil {
		return fmt.Errorf("weather skill services: %w", err)
	}
	if err := wireNotificationSkillServices(cfg, svc); err != nil {
		return fmt.Errorf("notification skill services: %w", err)
	}
	if err := wireRecipeSearchSkillServices(cfg, svc); err != nil {
		return fmt.Errorf("recipe_search skill services: %w", err)
	}
	return nil
}

// wireRetrievalSkillServices wires *api.SearchEngine + the SST-derived
// MaxTopK cap into the retrieval_search tool handler.
//
// Fail-loud per SST: when assistant.skills.retrieval.enabled is true
// AND the search engine has not been constructed OR the top_k cap is
// not positive, return an error so startup aborts. Silently disabling
// retrieval would defeat the BS-007 gate (the facade would never
// observe an empty Sources case because the tool would never run).
func wireRetrievalSkillServices(cfg *config.Config, svc *coreServices) error {
	if !cfg.Assistant.RetrievalEnabled {
		slog.Info("assistant.skills.retrieval.enabled=false; retrieval tool handler will return retrieval_tools_not_configured at call time")
		return nil
	}
	if svc.searchEngine == nil {
		return errors.New("assistant.skills.retrieval.enabled=true but coreServices.searchEngine is nil — buildCoreServices must run before skill wiring")
	}
	if cfg.Assistant.RetrievalTopK < 1 {
		return fmt.Errorf("ASSISTANT_SKILLS_RETRIEVAL_TOP_K must be >= 1, got %d", cfg.Assistant.RetrievalTopK)
	}
	retrieval.SetServices(&retrieval.Services{
		Engine:  svc.searchEngine,
		MaxTopK: cfg.Assistant.RetrievalTopK,
	})
	slog.Info("retrieval skill services wired",
		"max_top_k", cfg.Assistant.RetrievalTopK,
	)
	return nil
}

// wireWeatherSkillServices wires the weather Provider + LRU Cache
// into the weather_lookup tool handler.
//
// SST gate (per smackerel-no-defaults): when
// assistant.skills.weather.enabled is true AND
//   - the configured provider is not one we know how to construct, OR
//   - the cache TTL is not positive,
//
// return an error so startup aborts. There is NO silent default
// provider and NO silent cache-TTL fallback.
//
// Provider selection: v1 ships open-meteo, which is a key-less
// public endpoint. WeatherAPIKeyRef is read from SST but only
// consulted when a provider that requires it is selected (a future
// SCOPE-07 follow-on); selecting open-meteo with a non-empty
// api_key_ref is allowed (the value is just ignored) so operators can
// pre-stage a future provider's Infisical reference without
// reconfiguring.
//
// HTTP client per-call timeout matches the tool's PerCallTimeoutMs
// budget (2s, see internal/agent/tools/weather/tool.go init()).
func wireWeatherSkillServices(cfg *config.Config, svc *coreServices) error {
	if !cfg.Assistant.WeatherEnabled {
		slog.Info("assistant.skills.weather.enabled=false; weather tool handler will return weather_tools_not_configured at call time")
		return nil
	}
	if cfg.Assistant.WeatherCacheTTL <= 0 {
		return fmt.Errorf("ASSISTANT_SKILLS_WEATHER_CACHE_TTL must be > 0, got %s", cfg.Assistant.WeatherCacheTTL)
	}
	var provider weather.Provider
	switch cfg.Assistant.WeatherProvider {
	case "open-meteo":
		provider = weather.NewOpenMeteoProvider(
			&http.Client{Timeout: 2 * time.Second},
			cfg.Assistant.WeatherGeocodeURL,
			cfg.Assistant.WeatherForecastURL,
		)
	default:
		return fmt.Errorf("ASSISTANT_SKILLS_WEATHER_PROVIDER %q is not a recognized provider (v1 supports: open-meteo)", cfg.Assistant.WeatherProvider)
	}
	// Cache capacity is a fixed per-process upper bound. 128 covers
	// the v1 small-deployment expectation and is well within memory
	// budget (one entry ≈ a Forecast struct + key string). If a
	// future deployment needs a larger cache this becomes a new SST
	// key — for v1 we intentionally do not expose it (smackerel-no-
	// defaults: every SST key must be REQUIRED, so capacity is
	// hard-coded here rather than introduced with a silent default).
	const cacheCapacity = 128
	weather.SetServices(&weather.Services{
		Provider: provider,
		Cache:    weather.NewCache(cfg.Assistant.WeatherCacheTTL, cacheCapacity),
	})
	slog.Info("weather skill services wired",
		"provider", cfg.Assistant.WeatherProvider,
		"cache_ttl", cfg.Assistant.WeatherCacheTTL,
		"cache_capacity", cacheCapacity,
	)
	return nil
}

// wireNotificationSkillServices wires the notification tool's
// ConfirmStore + Scheduler dependencies.
//
// SST gate: when assistant.skills.notifications.enabled is true AND
//   - confirm_timeout is not positive, OR
//   - the PG pool is unavailable,
//
// return an error so startup aborts. There is NO silent disable.
//
// Scheduler binding: spec 054's scheduler is the production target,
// but the binding contract is not yet finalized (tracked by cross-
// spec packet 054 — additive Job.Source/Originator fields). Until
// that packet lands, we wire a guarded stub scheduler that fails
// loud on every Schedule call so a misconfigured deployment surfaces
// the gap in the first reminder attempt instead of silently dropping
// it. Operators who do not intend to use notifications yet should
// set assistant.skills.notifications.enabled=false at SST.
//
// The PG-backed ConfirmStore IS production-ready (migration 043
// ships the underlying table); only the Scheduler binding is pending
// the cross-spec packet.
func wireNotificationSkillServices(cfg *config.Config, svc *coreServices) error {
	if !cfg.Assistant.NotificationsEnabled {
		slog.Info("assistant.skills.notifications.enabled=false; notification tool handlers will return notification_tools_not_configured at call time")
		return nil
	}
	if cfg.Assistant.NotificationsConfirmTimeout <= 0 {
		return fmt.Errorf("ASSISTANT_SKILLS_NOTIFICATIONS_CONFIRM_TIMEOUT must be > 0, got %s", cfg.Assistant.NotificationsConfirmTimeout)
	}
	if svc.pg == nil || svc.pg.Pool == nil {
		return errors.New("assistant.skills.notifications.enabled=true but coreServices.pg.Pool is nil — buildCoreServices must run before skill wiring")
	}
	notification.SetServices(&notification.Services{
		Confirm:        notification.NewPgConfirmStore(svc.pg.Pool),
		Scheduler:      &notificationSchedulerStub{},
		ConfirmTimeout: cfg.Assistant.NotificationsConfirmTimeout,
	})
	slog.Info("notification skill services wired (scheduler=stub, pending cross-spec packet 054)",
		"confirm_timeout", cfg.Assistant.NotificationsConfirmTimeout,
	)
	return nil
}

// notificationSchedulerStub is the temporary Scheduler binding that
// fails loud on every Schedule call. Replaced by the spec 054
// adapter once cross-spec packet 054 (additive Job.Source +
// Job.Originator fields) is accepted by the spec 054 owner.
type notificationSchedulerStub struct{}

// errNotificationSchedulerUnbound is returned by the stub scheduler
// on every Schedule call until the spec 054 binding lands. The error
// text intentionally names the cross-spec packet so a trace reader
// can route the issue without grepping for the ID.
var errNotificationSchedulerUnbound = errors.New("notification.Scheduler: stub binding (cross-spec packet 054 pending — additive Job.Source/Originator on spec 054 scheduler API)")

func (notificationSchedulerStub) Schedule(_ context.Context, _ time.Time, _ string, _ string, _ string) (string, error) {
	return "", errNotificationSchedulerUnbound
}

// wireRecipeSearchSkillServices wires *api.SearchEngine into the
// recipe_search tool handler. BUG-061-003 — fail-loud per SST.
func wireRecipeSearchSkillServices(cfg *config.Config, svc *coreServices) error {
	if !cfg.Assistant.RecipeSearchEnabled {
		slog.Info("assistant.skills.recipe_search.enabled=false; recipe_search tool handler will return recipe_search_tools_not_configured at call time")
		return nil
	}
	if svc.searchEngine == nil {
		return errors.New("assistant.skills.recipe_search.enabled=true but coreServices.searchEngine is nil — buildCoreServices must run before skill wiring")
	}
	if cfg.Assistant.RecipeSearchTopK < 1 {
		return fmt.Errorf("ASSISTANT_SKILLS_RECIPE_SEARCH_TOP_K must be >= 1, got %d", cfg.Assistant.RecipeSearchTopK)
	}
	recipesearch.SetServices(&recipesearch.Services{
		Engine:  svc.searchEngine,
		MaxTopK: cfg.Assistant.RecipeSearchTopK,
	})
	slog.Info("recipe_search skill services wired",
		"max_top_k", cfg.Assistant.RecipeSearchTopK,
	)
	return nil
}
