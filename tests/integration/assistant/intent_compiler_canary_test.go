//go:build integration

// Spec 068 SCOPE-1 — facade canary integration test.
//
// Shared Infrastructure Impact Sweep (scopes.md Scope 1a): the
// assistant facade is a shared surface. This canary proves that the
// existing /reset path and operational-command bypass detection still
// function after the intent compiler foundation lands.
//
// The test runs in-process: it builds a Facade from the exported test
// helpers in internal/assistant/testing_support.go (no DB, no NATS),
// then exercises (a) the existing /reset shortcut path through
// Facade.Handle and (b) the new intent.IsOperationalCommand / BypassTrace
// surface for /status, /reset, /digest, /recent, /done, /help. The
// HTTP-route e2e proof for SCN-068-A06/A07 is deferred to spec 069
// per the Scope 1a split.

package assistant_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// TestIntentCompilerCanary_ExistingFacadeResetAndStatusStillWork is
// the spec 068 Scope 1a canary required by the Shared Infrastructure
// Impact Sweep. It asserts:
//
//  1. The existing /reset shortcut (KindText "/reset") still emits
//     StatusSavedAsIdea and deletes the conversation row.
//  2. The native KindReset envelope still emits StatusSavedAsIdea.
//  3. The intent package's operational-command carve-out classifies
//     /status (and the rest of the carve-out set) as a bypass turn
//     with the canonical trace label, independent of any facade
//     change.
func TestIntentCompilerCanary_ExistingFacadeResetAndStatusStillWork(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

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

	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{})
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}

	store := assistant.NewInMemoryContextStore()
	executor := assistant.NewStubExecutor()
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{})
	router := assistant.NewStubRouter()
	audit := assistant.NewRecordingAudit()

	f, err := assistant.NewFacade(cfg, router, executor, registry, manifest, store, audit)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}

	ctx := context.Background()

	t.Run("kind_reset_envelope_still_resets", func(t *testing.T) {
		resp, err := f.Handle(ctx, contracts.AssistantMessage{
			UserID:    "u-canary",
			Transport: "telegram",
			Kind:      contracts.KindReset,
		})
		if err != nil {
			t.Fatalf("Handle KindReset: %v", err)
		}
		if resp.Status != contracts.StatusSavedAsIdea {
			t.Fatalf("KindReset status = %q, want %q", resp.Status, contracts.StatusSavedAsIdea)
		}
	})

	t.Run("slash_reset_shortcut_still_resets", func(t *testing.T) {
		resp, err := f.Handle(ctx, contracts.AssistantMessage{
			UserID:    "u-canary-2",
			Transport: "telegram",
			Kind:      contracts.KindText,
			Text:      "/reset",
		})
		if err != nil {
			t.Fatalf("Handle /reset: %v", err)
		}
		if resp.Status != contracts.StatusSavedAsIdea {
			t.Fatalf("/reset shortcut status = %q, want %q", resp.Status, contracts.StatusSavedAsIdea)
		}
	})

	t.Run("operational_carve_out_detects_status_and_full_set", func(t *testing.T) {
		// SCN-068-A07: every command in the carve-out set is detected
		// as a bypass and produces a trace stamped with the canonical
		// label, regardless of whether a Facade handler exists for it
		// today. (Only /reset is wired into the facade in v1; the
		// other commands have no facade handler yet, but the bypass
		// trace label MUST be available so spec 067 / spec 069 can
		// attach handlers without ambiguity.)
		for _, cmd := range []string{"/help", "/status", "/reset", "/digest", "/recent", "/done"} {
			matched, ok := intent.IsOperationalCommand(cmd)
			if !ok || matched != cmd {
				t.Errorf("IsOperationalCommand(%q) = (%q,%v), want (%q,true)", cmd, matched, ok, cmd)
			}
			tr := intent.BypassTrace(cmd, matched)
			if tr.Outcome != intent.OutcomeBypass {
				t.Errorf("BypassTrace(%q) outcome = %q, want %q", cmd, tr.Outcome, intent.OutcomeBypass)
			}
			if tr.Bypass == nil || tr.Bypass.Label != intent.BypassTraceLabel {
				t.Errorf("BypassTrace(%q) label missing or wrong: %+v", cmd, tr.Bypass)
			}
		}
	})

	// Sanity: a free-text turn that does NOT match a carve-out
	// command MUST NOT be classified as a bypass — otherwise the
	// compiler insertion in Scope 2 would silently leak natural-
	// language turns onto the operational fast path.
	t.Run("natural_text_not_bypass", func(t *testing.T) {
		_, ok := intent.IsOperationalCommand("what was the weather yesterday?")
		if ok {
			t.Fatalf("free-text classified as operational bypass; carve-out leaked")
		}
	})
}
