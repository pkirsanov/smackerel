//go:build integration

// Spec 068 Scope 3 — shared helper to build a facade with a caller-
// supplied StubExecutor so assertions can inspect Invocations.

package assistant_integration

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/intent"
)

func buildWriteFacade(t *testing.T, compiler intent.Compiler, router *recordingRouter, registry *assistant.MapRegistry, executor *assistant.StubExecutor, enabled map[string]assistant.ManifestEntryForTest) *assistant.Facade {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := assistant.FacadeConfig{
		BorderlineFloor:      0.75,
		AgentConfidenceFloor: 0.50,
		SourcesMax:           5,
		BodyMaxChars:         1000,
		WindowTurns:          5,
		DisambigMaxChoices:   3,
		DisambigTimeout:      30 * time.Second,
		Now:                  func() time.Time { return now },
	}
	manifest, err := assistant.NewManifestForTest(enabled)
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	f, err := assistant.NewFacade(cfg, router, executor, registry,
		manifest, assistant.NewInMemoryContextStore(), assistant.NewRecordingAudit())
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	if compiler != nil {
		f.WithIntentCompiler(compiler)
	}
	return f
}
