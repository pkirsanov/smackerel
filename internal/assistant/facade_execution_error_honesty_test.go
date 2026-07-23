// BUG-061-008 / BUG-061-009 — the cross-scenario invariant that prevents the
// recurring "saved as an idea" masking. Every per-scenario fix (BUG-061-006,
// -007) patched ONE path; the masking was produced by a shared mechanism, so
// each requires_provenance scenario was a latent copy of the same defect.
// BUG-061-008 (P1) closed the NON-OK execution-error path. BUG-061-009 closes
// the last path — an OK outcome that produced no valid sources — and enforces
// the general invariant INV-HB-REFUSAL: a band-high turn NEVER renders the
// capture acknowledgement. TestHighBandNeverMaskedAsSavedAsIdea asserts this
// mechanically across every requires_provenance scenario × every high-band
// no-source outcome, so the class cannot silently recur: reverting any layer of
// the fix fails it.

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

// TestHighBandNeverMaskedAsSavedAsIdea — SCN-061-008-01/02 + SCN-061-009-01/02.
// The invariant INV-HB-REFUSAL, across EVERY requires_provenance scenario ×
// EVERY high-band no-source outcome (provider error, timeout, AND an OK outcome
// that produced no valid sources): the response surfaces honestly
// (StatusUnavailable + a non-empty ErrorCause) and is NEVER the capture-as-
// fallback "saved as an idea". This is the mechanical regression gate that
// catches the recurring masking for the whole class at once — reverting the
// gate's honest-refusal shape, the OK-outcome guard, OR the band-low
// canonicalize scoping fails a row here.
func TestHighBandNeverMaskedAsSavedAsIdea(t *testing.T) {
	t.Parallel()
	type hbCase struct {
		name    string
		outcome agent.Outcome
		final   []byte
	}
	var cases []hbCase
	for _, o := range errorOutcomes {
		cases = append(cases, hbCase{string(o), o, nil})
	}
	// The OK-but-uncited case: the agent succeeded (produced a body) but with
	// no valid sources — the exact /ask path BUG-061-009 fixes.
	cases = append(cases, hbCase{"ok_uncited", agent.OutcomeOK, []byte(`"a synthesized answer with no citations"`)})

	for _, scenarioID := range requiresProvenanceScenarios {
		for _, tc := range cases {
			name := scenarioID + "/" + tc.name
			t.Run(name, func(t *testing.T) {
				f := newExecErrHonestyFacade(t, scenarioID, tc.outcome, tc.final)
				resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
					UserID: "u-" + name, Transport: "telegram",
					Text: "do the thing", Kind: contracts.KindText,
				})
				if err != nil {
					t.Fatalf("Handle err: %v", err)
				}
				if resp.Status == contracts.StatusSavedAsIdea {
					t.Errorf("Status = saved_as_idea; a high-band no-source outcome (%s) MUST surface honestly, never masked", tc.name)
				}
				if resp.Status != contracts.StatusUnavailable {
					t.Errorf("Status = %q; want unavailable for high-band no-source outcome %s", resp.Status, tc.name)
				}
				if resp.Body == captureFallbackAcknowledgement {
					t.Errorf("Body is the capture acknowledgement; %s must surface an honest message, never 'saved as an idea'", tc.name)
				}
				if resp.CaptureRoute {
					t.Errorf("CaptureRoute = true; a high-band %s must not be captured as an idea", tc.name)
				}
				if resp.ErrorCause == "" {
					t.Errorf("ErrorCause empty; a high-band %s must carry a cause so the transport can render it honestly", tc.name)
				}
			})
		}
	}
}

// TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly — SCN-061-009-01.
// BUG-061-009 supersedes BUG-061-008 SCN-061-008-03: an OK outcome that
// produced a body with no valid sources is a high-band REFUSAL, not a capture.
// The anti-fabrication guard still fires (the uncited body is never shown), but
// it refuses HONESTLY — StatusUnavailable + ErrNoGroundedAnswer + the canonical
// refusal body — never "saved as an idea".
func TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly(t *testing.T) {
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
			if resp.Status == contracts.StatusSavedAsIdea {
				t.Errorf("Status = saved_as_idea; an OK-but-uncited high-band answer MUST refuse honestly, never be masked as a capture")
			}
			if resp.Status != contracts.StatusUnavailable {
				t.Errorf("Status = %q; want unavailable (honest refusal)", resp.Status)
			}
			if resp.ErrorCause != contracts.ErrNoGroundedAnswer {
				t.Errorf("ErrorCause = %q; want %q", resp.ErrorCause, contracts.ErrNoGroundedAnswer)
			}
			if resp.Body == captureFallbackAcknowledgement {
				t.Errorf("Body is the capture acknowledgement; an OK-but-uncited refusal must be honest, never 'saved as an idea'")
			}
			if resp.Body != contracts.CanonicalRefusalBodyFor(contracts.RefusalDefault) {
				t.Errorf("Body = %q; want the honest canonical refusal (the uncited answer must not leak)", resp.Body)
			}
			if resp.CaptureRoute {
				t.Errorf("CaptureRoute = true; a high-band refusal is not a capture")
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
