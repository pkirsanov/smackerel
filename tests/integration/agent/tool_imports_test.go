//go:build integration

package agent_integration

// Blank imports register the production tool set into the agent tool
// registry so integration tests that load real config/prompt_contracts
// scenarios pass scenario-loader validation. Mirrors cmd/core wiring
// (wiring_agent.go); without these the loader rejects scenarios that
// reference unregistered tools.
import (
	_ "github.com/smackerel/smackerel/internal/agent/tools/microtools"
	_ "github.com/smackerel/smackerel/internal/agent/tools/notification"
	_ "github.com/smackerel/smackerel/internal/agent/tools/recipesearch"
	_ "github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	_ "github.com/smackerel/smackerel/internal/agent/tools/weather"
	_ "github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	_ "github.com/smackerel/smackerel/internal/recommendation/tools"
)
