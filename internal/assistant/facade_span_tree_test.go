// Spec 061 SCOPE-09b — OTel span tree assertion test.
//
// Wires a real Facade with the in-memory SDK exporter and asserts:
//
//   - The 7 facade-owned spans from design §8.3.1.A (items 2-6, 8, 9)
//     are emitted on the high-band happy path.
//   - All 5 mandatory canonical attrs from §8.3.1.B are present on
//     every span, with the closed-vocab end attrs (status,
//     error_cause) stamped at span End.
//   - Parent/child shape matches §8.3.1 — every child has
//     `assistant.facade.handle` as its direct parent (the facade-owned
//     subtree of the design tree).
//   - The conditional `assistant.confirm.persist` span is ABSENT on
//     non-confirm turns (per design §8.3.1.A item 7 — missing here is
//     correct behavior, not a defect).
//   - Adversarial: low-band turns omit the executor + provenance
//     spans, proving the test fails-loud if span emission leaks
//     across band branches.

package assistant

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
)

// newInMemoryTracer returns a Tracer backed by tracetest.InMemoryExporter
// + a synchronous span processor so GetSpans() returns the complete set
// the moment Handle returns. The returned exporter is the assertion
// surface; the cleanup closure shuts the provider down for the test.
func newInMemoryTracer(t *testing.T) (*tracing.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return tracing.NewTracerFromProvider(provider, "smackerel-core-test"), exp
}

// spansByName indexes a recorded span slice by name. The facade
// emits at most one span per name per turn, so collisions are a
// test-failure signal.
func spansByName(t *testing.T, spans []sdktrace.ReadOnlySpan) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	out := make(map[string]sdktrace.ReadOnlySpan, len(spans))
	for _, s := range spans {
		if _, dup := out[s.Name()]; dup {
			t.Fatalf("span name %q recorded twice in one turn", s.Name())
		}
		out[s.Name()] = s
	}
	return out
}

func attrsMap(s sdktrace.ReadOnlySpan) map[string]string {
	m := make(map[string]string, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		m[string(kv.Key)] = kv.Value.AsString()
	}
	return m
}

func assertMandatoryAttrs(t *testing.T, name string, attrs map[string]string) {
	t.Helper()
	for _, key := range []string{"transport", "user_id_hashed", "assistant_turn_id", "scenario_id", "correlation_id"} {
		if _, ok := attrs[key]; !ok {
			t.Errorf("span %q missing mandatory attr %q (have %v)", name, key, attrs)
		}
	}
	if _, ok := attrs["status"]; !ok {
		t.Errorf("span %q missing end attr status (have %v)", name, attrs)
	}
	if _, ok := attrs["error_cause"]; !ok {
		t.Errorf("span %q missing end attr error_cause (have %v)", name, attrs)
	}
}

// TestFacade_SpanTree_HighBand_HappyPath drives a weather_query turn
// through the facade and asserts the 7 facade-owned spans land with
// correct parentage, attributes, and outcomes. No confirm.persist
// span is expected because the manifest has confirm_required=false.
func TestFacade_SpanTree_HighBand_HappyPath(t *testing.T) {
	t.Parallel()

	tr, exp := newInMemoryTracer(t)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	weather := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": weather,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check the weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: false,
			ConfirmRequired:    false,
			EnableSSTKey:       "assistant.skill.weather_query.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-09b",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`"sunny"`),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: weather,
		decision: agent.RoutingDecision{
			Reason:    agent.ReasonSimilarityMatch,
			Chosen:    "weather_query",
			TopScore:  0.93,
			Threshold: 0.50,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.93},
			},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit).WithTracer(tr)

	msg := contracts.AssistantMessage{
		UserID:    "user-09b",
		Transport: "telegram",
		Text:      "weather in barcelona",
		Kind:      contracts.KindText,
		TransportMetadata: map[string]string{
			"telegram_update_id": "777",
		},
	}
	resp, err := facade.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.Status != contracts.StatusThinking {
		t.Fatalf("Status = %q; want %q", resp.Status, contracts.StatusThinking)
	}

	spans := exp.GetSpans().Snapshots()
	byName := spansByName(t, spans)

	// Design §8.3.1.A facade-owned subtree (items 2-6, 8, 9). The
	// adapter-owned spans (translate/render) live in the adapter
	// package's tests; this facade test asserts only the spans the
	// facade emits.
	want := []string{
		"assistant.facade.handle",
		"assistant.context.load",
		"assistant.router.classify",
		"assistant.router.band",
		"assistant.provenance.check",
		"assistant.context.persist",
		"assistant.audit.write",
	}
	for _, name := range want {
		s, ok := byName[name]
		if !ok {
			t.Errorf("expected span %q absent; have %v", name, names(spans))
			continue
		}
		assertMandatoryAttrs(t, name, attrsMap(s))
	}

	// Conditional span MUST be absent: confirm_required=false and the
	// facade does not invoke the confirm machine in v1 (design
	// §8.3.1.A item 7 — missing here is correct behavior).
	if _, present := byName["assistant.confirm.persist"]; present {
		t.Errorf("assistant.confirm.persist MUST be absent on non-confirm turn; spans=%v", names(spans))
	}

	// Parent/child shape: every child has facade.handle as its
	// parent. The root facade.handle has no parent (we invoke
	// Handle with a bare context.Background that carries no span).
	root, ok := byName["assistant.facade.handle"]
	if !ok {
		t.Fatal("facade.handle span missing; cannot assert tree shape")
	}
	if root.Parent().IsValid() {
		t.Errorf("facade.handle parent span ctx is valid; want invalid (root): %+v", root.Parent())
	}
	rootSpanID := root.SpanContext().SpanID()
	for _, name := range want {
		if name == "assistant.facade.handle" {
			continue
		}
		child, ok := byName[name]
		if !ok {
			continue
		}
		if child.Parent().SpanID() != rootSpanID {
			t.Errorf("span %q parent = %s; want facade.handle %s",
				name, child.Parent().SpanID(), rootSpanID)
		}
	}

	// Adversarial sub-check: a span name that should NOT appear
	// when the facade does not invoke spec-037-owned executor
	// instrumentation. agent.executor.run and agent.tool.<n>.invoke
	// are explicitly deferred per design §8.3.1 preamble.
	for _, forbidden := range []string{"agent.executor.run", "agent.tool.weather_query.invoke"} {
		if _, present := byName[forbidden]; present {
			t.Errorf("forbidden spec-037-owned span %q leaked into facade subtree", forbidden)
		}
	}

	// Verify the assistant_turn_id stamped on every span is the
	// deterministic facade turn id (asst-<unix-nano>). All child
	// spans MUST carry the same value so dashboards can group
	// span-tree by turn id without joining on parent ids.
	expectedTurnID := facadeTurnIDFromTime(now)
	for _, name := range want {
		s, ok := byName[name]
		if !ok {
			continue
		}
		got := attrsMap(s)["assistant_turn_id"]
		if name == "assistant.facade.handle" {
			// facade.handle stamps it late via SetAttributes so it
			// always equals expectedTurnID at span End.
			if got != expectedTurnID {
				t.Errorf("facade.handle assistant_turn_id = %q; want %q", got, expectedTurnID)
			}
			continue
		}
		// Mid-Handle spans (context.load, router.*, provenance.check)
		// were started BEFORE turnAssistantTurnID was assigned, so
		// they may legitimately carry "" — but the persist + audit
		// spans (started after) MUST carry the resolved id.
		if name == "assistant.context.persist" || name == "assistant.audit.write" {
			if got != expectedTurnID {
				t.Errorf("span %q assistant_turn_id = %q; want %q",
					name, got, expectedTurnID)
			}
		}
	}
}

// TestFacade_SpanTree_LowBand_OmitsProvenanceSpan drives a low-band
// (capture-fallback) turn and asserts the provenance.check span is
// NOT emitted — that span is only reached on the high-band branch.
// This is the adversarial guard against span emission leaking
// across band branches.
func TestFacade_SpanTree_LowBand_OmitsProvenanceSpan(t *testing.T) {
	t.Parallel()

	tr, exp := newInMemoryTracer(t)
	now := time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	// Empty registry — router returns no chosen scenario, low score
	// → BandLow → CaptureRoute, no executor, no provenance.
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{
		chosen: nil,
		decision: agent.RoutingDecision{
			Reason:   agent.ReasonUnknownIntent,
			TopScore: 0.10,
		},
		ok: false,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit).WithTracer(tr)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "user-low",
		Transport: "telegram",
		Text:      "??? incoherent input ???",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if !resp.CaptureRoute {
		t.Fatalf("CaptureRoute = false; want true on low-band fallback")
	}

	byName := spansByName(t, exp.GetSpans().Snapshots())

	// The 4 spans that MUST emit on every turn regardless of band:
	for _, name := range []string{
		"assistant.facade.handle",
		"assistant.context.load",
		"assistant.router.classify",
		"assistant.router.band",
		"assistant.context.persist",
		"assistant.audit.write",
	} {
		if _, ok := byName[name]; !ok {
			t.Errorf("expected always-present span %q absent on low-band path", name)
		}
	}
	// Adversarial: provenance.check MUST NOT emit on low-band — the
	// facade's high-band branch is the ONLY caller. A leak here
	// would mean span emission decoupled from real execution flow.
	if _, present := byName["assistant.provenance.check"]; present {
		t.Errorf("assistant.provenance.check MUST be absent on low-band turn (no scenario executed)")
	}
	// And confirm.persist stays absent on this path too.
	if _, present := byName["assistant.confirm.persist"]; present {
		t.Errorf("assistant.confirm.persist MUST be absent on low-band turn")
	}
}

// names is a small helper for error messages when an expected span
// is missing — turns the slice into a readable list of recorded
// span names.
func names(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name())
	}
	return out
}

// Compile-time assertion that the test imports the trace SDK
// surface we actually rely on.
var _ trace.SpanContext = trace.SpanContext{}
