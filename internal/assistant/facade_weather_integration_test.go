//go:build integration

// Spec 061 SCOPE-07 — weather facade source-assembly integration test.
//
// This test drives Facade.Handle end-to-end against:
//
//   - REAL PostgreSQL (test stack via DATABASE_URL).
//   - REAL weather.NewFacadeAssembler (no in-memory shortcut —
//     same closure shape cmd/core/wiring_assistant_facade.go installs
//     in production).
//   - REAL provenance.Enforce gate (BandHigh dispatch path).
//   - REAL contracts wiring.
//
// The executor is stubbed because driving the open-meteo provider HTTP
// boundary from a Go integration test is the wrong layer — the BS-006
// shell e2e test owns the end-to-end provider-stub path. Here we
// exercise the substrate the production capability layer integrates
// at: the SourceAssembler hook, the ExternalProviderRef invariant,
// and the provenance gate.
//
// Three cases:
//
//  1. BS-003 happy path — executor returns OutcomeOK with the
//     weather-query-v1 Final shape ({forecast_line, provider_name,
//     retrieved_at}). Assembler emits exactly one Source{Kind:
//     SourceExternalProvider, Ref: ExternalProviderRef{ProviderName,
//     RetrievedAt}}. RetrievedAt is the ORIGINAL upstream timestamp
//     (cache-hit invariant — design §5.2). Provenance gate passes
//     through.
//
//  2. BS-006 provider unavailable — executor returns OutcomeToolError
//     with empty Final (mirrors the tool's behavior when the upstream
//     provider returns 5xx / DNS / timeout — see design §5.2 failure
//     mapping). Assembler returns zero-value SourceAssembly. The
//     facade keeps its default-rendered body and the provenance gate
//     rewrites to canonical refusal + CaptureRoute=true (BS-006 path).
//
//  3. BS-007-equivalent fabrication refusal — executor returns
//     OutcomeOK BUT the Final is missing provider_name. Assembler
//     refuses to fabricate attribution and returns zero-value
//     SourceAssembly. Gate fires the same canonical refusal as the
//     provider-unavailable path, proving the anti-fabrication
//     invariant.
//
// The integration database is shared with other tests; we prefix the
// per-test user_id with a high-resolution timestamp so there are no
// row collisions on rapid re-runs. Weather does NOT write artifact
// rows (it's external-provider only) so no DB cleanup is needed.
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags integration \
//	    ./internal/assistant/ -run TestFacadeWeatherIntegration \
//	    -count=1 -v

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeWeatherIntegration_BS003_HappyPathEmitsExternalProviderSource(t *testing.T) {
	// Forces the test runner to instantiate the integration PG stack
	// even though the weather assembler does not consult it. Mirrors
	// the production wiring: the same FacadeConfig that wires weather
	// also wires retrieval against the PG-backed lookup, so a healthy
	// PG is a prerequisite. Skips on missing DATABASE_URL.
	_, _ = newIntegrationPostgres(t)

	prefix := "asm-int-weather-bs003-" + time.Now().UTC().Format("20060102150405.000000000")

	now := time.Date(2026, 5, 28, 14, 3, 0, 0, time.UTC)
	// Original upstream provider timestamp — the assembler MUST
	// preserve this verbatim (no time.Now substitution).
	upstreamRetrievedAt := time.Date(2026, 5, 28, 13, 58, 12, 0, time.UTC)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skills.weather.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	rawJSON := []byte(`{"forecast_line":"Seattle: 12\u00b0C, light rain.","provider_name":"open-meteo","retrieved_at":"` + upstreamRetrievedAt.Format(time.RFC3339) + `"}`)
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-bs003-int",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      rawJSON,
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.92,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.92}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"weather_query": weather.NewFacadeAssembler(cfg.SourcesMax),
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: prefix + "-u", Transport: "telegram",
		Text: "weather in Seattle today", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// BS-003 expectations:
	//   - body is the forecast_line verbatim (assembler body override).
	//   - exactly one Source with Kind=external_provider.
	//   - ExternalProviderRef.RetrievedAt is the ORIGINAL upstream
	//     timestamp, not the facade emittedAt or any later wall clock.
	//   - gate passes (CaptureRoute=false).
	const wantBody = "Seattle: 12°C, light rain."
	if resp.Body != wantBody {
		t.Errorf("Body mismatch:\n  got:  %q\n  want: %q", resp.Body, wantBody)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute=true; gate fired unexpectedly on BS-003 happy path")
	}
	if len(resp.Sources) != 1 {
		t.Fatalf("Sources len = %d; want exactly 1 external_provider source", len(resp.Sources))
	}
	src := resp.Sources[0]
	if src.Kind != contracts.SourceExternalProvider {
		t.Errorf("Sources[0].Kind = %q; want %q", src.Kind, contracts.SourceExternalProvider)
	}
	if src.ID != "open-meteo" || src.Title != "open-meteo" {
		t.Errorf("Sources[0] ID/Title mismatch: got id=%q title=%q; want both = open-meteo", src.ID, src.Title)
	}
	ref, ok := src.Ref.(contracts.ExternalProviderRef)
	if !ok {
		t.Fatalf("Sources[0].Ref type = %T; want contracts.ExternalProviderRef", src.Ref)
	}
	if ref.ProviderName != "open-meteo" {
		t.Errorf("ExternalProviderRef.ProviderName = %q; want open-meteo", ref.ProviderName)
	}
	if !ref.RetrievedAt.Equal(upstreamRetrievedAt) {
		t.Errorf("ExternalProviderRef.RetrievedAt = %s; want ORIGINAL upstream timestamp %s", ref.RetrievedAt, upstreamRetrievedAt)
	}
	if resp.SourcesOverflowCount != 0 {
		t.Errorf("SourcesOverflowCount = %d; want 0", resp.SourcesOverflowCount)
	}
	if executor.invocations != 1 {
		t.Errorf("executor.invocations = %d; want 1", executor.invocations)
	}
}

func TestFacadeWeatherIntegration_BS006_ProviderUnavailableTriggersRefusal(t *testing.T) {
	_, _ = newIntegrationPostgres(t)

	prefix := "asm-int-weather-bs006-" + time.Now().UTC().Format("20060102150405.000000000")

	now := time.Date(2026, 5, 28, 14, 3, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skills.weather.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	// Executor returns OutcomeProviderError (open-meteo 5xx / DNS /
	// timeout, per design §5.2 failure mapping). OutcomeToolError is
	// per-tool-call-only — the canonical top-level outcome the
	// executor surfaces for an unrecoverable upstream provider
	// failure is OutcomeProviderError (see internal/agent/executor.go
	// — `result.Outcome = OutcomeProviderError` at l443/l474). Final
	// is empty because the tool never returned a payload.
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-bs006-int",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeProviderError,
				OutcomeDetail: map[string]any{
					"tool":   "weather_lookup",
					"detail": "open-meteo: 503 service unavailable",
				},
				Final:     nil,
				StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.92,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.92}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"weather_query": weather.NewFacadeAssembler(cfg.SourcesMax),
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: prefix + "-u", Transport: "telegram",
		Text: "weather in Seattle today", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// BS-006 expectations: provider unavailable → translateFinalToBody
	// emits "provider unavailable." → assembler returns zero value
	// (Outcome != OK) → provenance gate sees requires_provenance=true
	// + Sources empty + Body non-empty → REWRITES to canonical refusal
	// (Status=StatusSavedAsIdea, Body=CanonicalRefusalBody,
	// CaptureRoute=true). ErrorCause is set BEFORE the gate runs
	// (translateOutcomeToErrorCause) and the gate preserves it; this
	// is the field the transport adapter uses to render the
	// `weather: unavailable` error line per spec BS-006.
	if got, want := resp.Status, contracts.StatusSavedAsIdea; got != want {
		t.Errorf("Status = %q; want %q (provenance gate rewrites provider-error to soft refusal)", got, want)
	}
	if got, want := resp.ErrorCause, contracts.ErrProviderUnavailable; got != want {
		t.Errorf("ErrorCause = %q; want %q (translateOutcomeToErrorCause MUST propagate provider failure for BS-006)", got, want)
	}
	if !resp.CaptureRoute {
		t.Errorf("CaptureRoute = false; want true (provenance gate must offer to capture per BS-006)")
	}
	if got, want := resp.Body, "I don't have a sourced answer for that."; got != want {
		t.Errorf("Body = %q; want %q (canonical refusal body)", got, want)
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources len = %d; want 0 (provider unavailable)", len(resp.Sources))
	}
}

func TestFacadeWeatherIntegration_AntiFabrication_MissingProviderTriggersRefusal(t *testing.T) {
	_, _ = newIntegrationPostgres(t)

	prefix := "asm-int-weather-antifab-" + time.Now().UTC().Format("20060102150405.000000000")

	now := time.Date(2026, 5, 28, 14, 3, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skills.weather.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	// Executor returns OutcomeOK but Final is MISSING provider_name —
	// the assembler MUST refuse to fabricate attribution and return
	// zero-value SourceAssembly. The provenance gate then refuses the
	// response with the canonical refusal body.
	rawJSON := []byte(`{"forecast_line":"Seattle: 12°C clear.","retrieved_at":"2026-05-28T13:58:12Z"}`)
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-antifab-int",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      rawJSON,
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.92,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.92}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"weather_query": weather.NewFacadeAssembler(cfg.SourcesMax),
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: prefix + "-u", Transport: "telegram",
		Text: "weather in Seattle today", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	const canonicalRefusal = "I don't have a sourced answer for that."
	if resp.Body != canonicalRefusal {
		t.Errorf("Body = %q; want canonical refusal %q (anti-fabrication: gate refuses missing-attribution Final)", resp.Body, canonicalRefusal)
	}
	if !resp.CaptureRoute {
		t.Errorf("CaptureRoute=false; gate did NOT fire on missing-attribution Final (anti-fabrication regression)")
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources len = %d; want 0 (assembler refused to fabricate)", len(resp.Sources))
	}
}
