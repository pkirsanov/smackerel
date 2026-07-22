// BUG-061-008 — the cross-scenario invariant that prevents the recurring
// "saved as an idea" masking. Every prior fix (BUG-061-006, BUG-061-007)
// patched ONE scenario; the masking was produced by a shared mechanism (the
// provenance gate running on non-OK outcomes), so each requires_provenance
// scenario was a latent copy of the same defect. These tests assert the fix
// mechanically across ALL such scenarios at once, so the class cannot silently
// recur: reverting the P1 guard (running the gate on a non-OK outcome) makes
// every row of TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea fail.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
)

// requiresProvenanceScenarios is the closed set whose manifest sets
// requires_provenance=true (skills_manifest_test.go asserts this exact set).
// Every one is subject to the provenance gate and therefore to the masking
// defect if the gate ever runs on a non-OK outcome.
var requiresProvenanceScenarios = []string{"weather_query", "retrieval_qa", "recipe_search"}

// errorOutcomes are non-OK executor outcomes that represent an execution
// FAILURE (not a genuine no-answer). Each MUST surface honestly.
var errorOutcomes = []agent.Outcome{agent.OutcomeProviderError, agent.OutcomeTimeout}

// newExecErrHonestyFacade builds a Facade for scenarioID with a stub executor
// returning the given outcome. No source-assembler is registered: for a non-OK
// outcome the assembler is irrelevant (the gate is skipped), and for the
// OK-fabrication case the absence of sources means the gate MUST still fire.
func newExecErrHonestyFacade(t *testing.T, scenarioID string, outcome agent.Outcome, final []byte) *Facade {
	t.Helper()
	now := time.Date(2026, 7, 22, 21, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: scenarioID}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{scenarioID: scenario}}
	manifest := newTestManifest(map[string]manifestEntry{
		scenarioID: {
			UserFacingLabel:    scenarioID,
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skill." + scenarioID + ".enabled",
			Enabled:            true,
		},
	})
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID: "trace-execerr", ScenarioID: sc.ID, Outcome: outcome,
				Final: final, StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: scenarioID, TopScore: 0.9,
			Considered: []agent.CandidateScore{{ScenarioID: scenarioID, Score: 0.9}},
		},
		ok: true,
	}
	return mustFacade(cfg, router, executor, registry, manifest, newMemContextStore(), &recordingAudit{})
}

// TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea — SCN-061-008-01/02.
// The invariant, across EVERY requires_provenance scenario × EVERY execution
// error outcome: the response surfaces honestly and is NEVER the
// capture-as-fallback "saved as an idea". This is the mechanical regression
// gate that catches the recurring masking for all scenarios at once.
func TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea(t *testing.T) {
	t.Parallel()
	for _, scenarioID := range requiresProvenanceScenarios {
		for _, outcome := range errorOutcomes {
			name := scenarioID + "/" + string(outcome)
			t.Run(name, func(t *testing.T) {
				f := newExecErrHonestyFacade(t, scenarioID, outcome, nil)
				resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
					UserID: "u-" + name, Transport: "telegram",
					Text: "do the thing", Kind: contracts.KindText,
				})
				if err != nil {
					t.Fatalf("Handle err: %v", err)
				}
				if resp.Status == contracts.StatusSavedAsIdea {
					t.Errorf("Status = saved_as_idea; an execution error (%s) MUST surface honestly, never masked", outcome)
				}
				if resp.Status != contracts.StatusUnavailable {
					t.Errorf("Status = %q; want unavailable for execution error %s", resp.Status, outcome)
				}
				if resp.Body == captureFallbackAcknowledgement {
					t.Errorf("Body is the capture acknowledgement; %s must surface an honest error, never 'saved as an idea'", outcome)
				}
				if resp.CaptureRoute {
					t.Errorf("CaptureRoute = true; an execution error (%s) must not be captured as an idea", outcome)
				}
				if resp.ErrorCause == "" {
					t.Errorf("ErrorCause empty; an execution error (%s) must carry a cause so the transport can render it honestly", outcome)
				}
			})
		}
	}
}

// TestExecutionErrorHonesty_OKNoSourcesStillRefuses — SCN-061-008-03.
// Complementary guard proving the fix does NOT over-correct: an OK outcome
// that produced a body with no valid sources is genuine fabrication and MUST
// still refuse (capture-as-fallback). The provenance gate stays intact for its
// real purpose; only NON-OK outcomes are exempted.
func TestExecutionErrorHonesty_OKNoSourcesStillRefuses(t *testing.T) {
	t.Parallel()
	for _, scenarioID := range requiresProvenanceScenarios {
		t.Run(scenarioID, func(t *testing.T) {
			f := newExecErrHonestyFacade(t, scenarioID, agent.OutcomeOK, []byte(`"a synthesized answer with no citations"`))
			resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
				UserID: "u-fab-" + scenarioID, Transport: "telegram",
				Text: "do the thing", Kind: contracts.KindText,
			})
			if err != nil {
				t.Fatalf("Handle err: %v", err)
			}
			if resp.Status != contracts.StatusSavedAsIdea {
				t.Errorf("Status = %q; an OK outcome with no sources is fabrication and MUST still refuse (want saved_as_idea)", resp.Status)
			}
			if resp.Body != captureFallbackAcknowledgement {
				t.Errorf("Body = %q; want the capture acknowledgement (anti-fabrication guard preserved)", resp.Body)
			}
			if !resp.CaptureRoute {
				t.Errorf("CaptureRoute = false; the anti-fabrication guard must set it")
			}
		})
	}
}

// TestExecutionErrorHonesty_MetricIncrements — SCN-061-008-04 (P3). A surfaced
// non-OK outcome increments ExecutionErrorSurfacedTotal{scenario,outcome,transport}
// so execution failures are observable on a dashboard. Uses the "fake"
// transport label (the only non-telegram label after normalizeTransportLabel)
// so the delta is isolated from the parallel honesty table above, which uses
// "telegram".
func TestExecutionErrorHonesty_MetricIncrements(t *testing.T) {
	const scenarioID = "weather_query"
	outcome := agent.OutcomeProviderError
	labels := []string{scenarioID, string(outcome), assistantmetrics.TransportFake}
	before := testutil.ToFloat64(assistantmetrics.ExecutionErrorSurfacedTotal.WithLabelValues(labels...))

	f := newExecErrHonestyFacade(t, scenarioID, outcome, nil)
	if _, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-metric", Transport: assistantmetrics.TransportFake, Text: "do the thing", Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	after := testutil.ToFloat64(assistantmetrics.ExecutionErrorSurfacedTotal.WithLabelValues(labels...))
	if delta := after - before; delta != 1 {
		t.Errorf("ExecutionErrorSurfacedTotal{%v} delta = %.0f; want 1", labels, delta)
	}
}
