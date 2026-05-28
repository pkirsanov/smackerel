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
	"errors"
	"fmt"
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
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
	// SCOPE-07 (weather) and SCOPE-08 (notifications) will append
	// their SetServices wiring blocks here.
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
