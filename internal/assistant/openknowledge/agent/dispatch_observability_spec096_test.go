// Spec 096 §13 — the agent-side LIVE WIRING of the cost-bearing dispatch
// observability into the /ask loop. These tests pin four behaviors at the
// agent spine:
//
//  1. a HOSTED provider dispatch emits the per-provider dispatch counter once
//     plus the per-provider token + USD-cents observation (ADVERSARIAL —
//     FAILS before the agent.go emission wiring);
//  2. a HOSTED provider dispatch opens a `model.dispatch` span carrying ONLY
//     provider/model/turn/cost.usd, and the decrypted credential NEVER appears
//     in ANY span attribute value (ADVERSARIAL + the §13 secret-safety
//     invariant — FAILS before the wiring);
//  3. a BARE / "ollama/…" dispatch emits NO provider metric and NO
//     model.dispatch span — the byte-for-byte spec 089 path (GUARD — green
//     before AND after);
//  4. a hosted resolve DECRYPT failure increments the typed, secret-free
//     vault-decrypt-failure counter and dispatches nothing (ADVERSARIAL —
//     FAILS before the wiring).
//
// Reuses the shared harness (recordingLLM, spec087Cfg, toolUse, endTurn,
// webCiteEntry, citationsBlock, newRegistry, textPtr) plus the fake resolver
// scaffolding (fakeDispatchResolver, installFakeDispatchResolver) from
// dispatch_provider_spec096_test.go.
package agent

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
)

const (
	spec096ObsHostedModel = "anthropic/claude-3-5-sonnet"
	spec096ObsBareModel   = "gemma3:4b"
	// Synthetic credential; never a real secret. The §13 secret-safety test
	// asserts THIS string never appears in any span attribute value.
	spec096ObsAPIKey = "sk-ant-SYNTHETIC-OBS-CREDENTIAL-00000000" // gitleaks:allow — synthetic test credential
)

// providerObservation records one ObserveProviderDispatch call.
type providerObservation struct {
	provider string
	tokens   int
	usdCents float64
}

// spyRecorder records the spec 096 §13 emission calls (provider dispatch
// counter, per-provider token/USD observation, vault-decrypt failures) so a
// test can assert WHAT the agent emitted on the hosted vs bare vs decrypt-fail
// paths. Every other Recorder method is a no-op.
//
// Embedding okmetrics.Nop auto-satisfies the FULL Recorder interface (including
// IncConnectionTest and any future additions); the explicit capture methods
// below override Nop's no-ops for the calls these tests assert on.
type spyRecorder struct {
	okmetrics.Nop
	providerDispatches  []string
	providerObs         []providerObservation
	vaultDecryptReasons []string
}

func (s *spyRecorder) IncProviderDispatch(provider string) {
	s.providerDispatches = append(s.providerDispatches, provider)
}

func (s *spyRecorder) ObserveProviderDispatch(provider string, tokens int, usdCents float64) {
	s.providerObs = append(s.providerObs, providerObservation{provider: provider, tokens: tokens, usdCents: usdCents})
}

func (s *spyRecorder) IncVaultDecryptFailure(reason string) {
	s.vaultDecryptReasons = append(s.vaultDecryptReasons, reason)
}

// — unused Recorder surface (no-ops) —
func (*spyRecorder) RecordTurn(int, int, float64)       {}
func (*spyRecorder) IncToolCall(string, string)         {}
func (*spyRecorder) ObserveToolLatency(string, float64) {}
func (*spyRecorder) IncBudgetExhausted(string)          {}
func (*spyRecorder) IncFabricatedSource()               {}
func (*spyRecorder) IncRefusal(string)                  {}
func (*spyRecorder) IncCompactionSignaled()             {}

// Spec 096 §13 discovery surface — the agent never emits discovery metrics (the
// catalog aggregator does), so these are no-ops on the agent-side spy.
func (*spyRecorder) IncProviderDiscovery(string, string)             {}
func (*spyRecorder) ObserveProviderDiscoveryLatency(string, float64) {}

var _ okmetrics.Recorder = (*spyRecorder)(nil)

// newInMemoryDispatchTracer returns a Tracer backed by tracetest.InMemoryExporter
// + a synchronous span processor so GetSpans() returns the complete set the
// moment Run returns. Mirrors facade_span_tree_test.go's helper.
func newInMemoryDispatchTracer(t *testing.T) (*tracing.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return tracing.NewTracerFromProvider(provider, "smackerel-core-test"), exp
}

// installHostedSynthesisResolver wires a fake resolver that maps the hosted
// model to a populated anthropic dispatch carrying the synthetic credential, so
// a SYNTHESIS-turn override dispatches through it. The bare gather turn keeps
// the 089 path.
func installHostedSynthesisResolver(t *testing.T) {
	t.Helper()
	f := &fakeDispatchResolver{
		t: t,
		resolved: map[string]llm.ResolvedDispatch{
			spec096ObsHostedModel: {
				Request: llm.ChatRequest{
					Model:    "claude-3-5-sonnet", // BARE backend id
					Provider: "anthropic",
					APIKey:   textPtr(spec096ObsAPIKey),
				},
				Attribution: spec096ObsHostedModel,
			},
		},
	}
	installFakeDispatchResolver(t, f)
}

// hostedModelAwareCostFn prices the hosted synthesis model at $0.02/round (→ 2.0
// cents) and everything else at $0 (the 089 path consumes no budget).
func hostedModelAwareCostFn(model string, _ int) (float64, error) {
	if model == spec096ObsHostedModel {
		return 0.02, nil
	}
	return 0, nil
}

// ── #1 — hosted dispatch emits the per-provider counter + token/USD observation ──

func TestAgent_HostedDispatch_EmitsProviderMetrics_Spec096(t *testing.T) {
	installHostedSynthesisResolver(t)

	const maxIter = 2 // iter0 bare gather turn, iter1 hosted forced-final synthesis turn
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(verdict, 80),
	}}
	spy := &spyRecorder{}
	cfg := spec087Cfg(maxIter, 1)
	cfg.Recorder = spy
	cfg.CostFn = hostedModelAwareCostFn
	base, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The hosted model is the selected SYNTHESIS model (Fork B); the gather turn
	// keeps the bare SST baseline.
	a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec096ObsHostedModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
	}

	// Exactly ONE hosted provider dispatch (the synthesis turn); the bare gather
	// turn took the 089 path and emitted nothing.
	if len(spy.providerDispatches) != 1 || spy.providerDispatches[0] != "anthropic" {
		t.Fatalf("provider dispatches = %v, want exactly [anthropic] (the hosted synthesis turn only)", spy.providerDispatches)
	}
	if len(spy.providerObs) != 1 {
		t.Fatalf("provider observations = %v, want exactly 1", spy.providerObs)
	}
	obs := spy.providerObs[0]
	if obs.provider != "anthropic" {
		t.Errorf("observation provider = %q, want anthropic", obs.provider)
	}
	if obs.tokens != 80 {
		t.Errorf("observation tokens = %d, want 80 (the synthesis round TokensUsed)", obs.tokens)
	}
	if obs.usdCents != 2.0 {
		t.Errorf("observation usdCents = %v, want 2.0 ($0.02 × 100)", obs.usdCents)
	}
}

// ── #2 — hosted dispatch opens a model.dispatch span; NO secret in any attr ──

func TestAgent_HostedDispatch_EmitsModelDispatchSpanNoSecret_Spec096(t *testing.T) {
	installHostedSynthesisResolver(t)
	tr, exp := newInMemoryDispatchTracer(t)

	const maxIter = 2
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(verdict, 80),
	}}
	cfg := spec087Cfg(maxIter, 1)
	cfg.Tracer = tr
	cfg.CostFn = hostedModelAwareCostFn
	base, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec096ObsHostedModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
	}

	spans := exp.GetSpans().Snapshots()
	var dispatch sdktrace.ReadOnlySpan
	count := 0
	for _, s := range spans {
		if s.Name() == "model.dispatch" {
			dispatch = s
			count++
		}
	}
	if count != 1 {
		t.Fatalf("model.dispatch spans = %d, want exactly 1 (only the hosted synthesis turn)", count)
	}

	attrs := map[string]string{}
	for _, kv := range dispatch.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if attrs["provider"] != "anthropic" {
		t.Errorf("span provider attr = %q, want anthropic", attrs["provider"])
	}
	if attrs["model"] != "claude-3-5-sonnet" {
		t.Errorf("span model attr = %q, want the BARE backend id claude-3-5-sonnet", attrs["model"])
	}
	if attrs["turn"] != "synthesis" {
		t.Errorf("span turn attr = %q, want synthesis", attrs["turn"])
	}
	if _, ok := attrs["cost.usd"]; !ok {
		t.Errorf("span missing cost.usd attr (have %v)", attrs)
	}

	// §13 alarming invariant: the decrypted credential MUST NOT appear in ANY
	// attribute value of ANY emitted span.
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if strings.Contains(kv.Value.Emit(), spec096ObsAPIKey) {
				t.Fatalf("api_key leaked into span %q attr %q", s.Name(), kv.Key)
			}
		}
	}
}

// ── #3 — bare/ollama dispatch emits NO provider metric and NO span (089 path) ──

func TestAgent_BareDispatch_NoProviderMetricsOrSpan_ByteForByte089_Spec096(t *testing.T) {
	// A HOSTED resolver IS installed, but a bare/ollama selection must not reach
	// it and must emit NO provider metric and NO model.dispatch span — proving
	// the §13 emission is gated on the hosted-kind check, not resolver presence.
	f := &fakeDispatchResolver{
		t: t,
		resolved: map[string]llm.ResolvedDispatch{
			spec096ObsBareModel: {
				Request:     llm.ChatRequest{Model: "should-not-be-used", Provider: "anthropic", APIKey: textPtr("sk-MUST-NOT-EMIT")}, // gitleaks:allow — synthetic
				Attribution: spec096ObsBareModel,
			},
		},
	}
	installFakeDispatchResolver(t, f)
	tr, exp := newInMemoryDispatchTracer(t)

	const maxIter = 2
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(verdict, 80),
	}}
	spy := &spyRecorder{}
	cfg := spec087Cfg(maxIter, 1)
	cfg.Recorder = spy
	cfg.Tracer = tr
	base, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a := base.WithModelOverride(modelswitch.Override{GatherModel: spec096ObsBareModel, SynthesisModel: spec096ObsBareModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success (089 path)", got.Status, got.RefusalReason)
	}
	if len(spy.providerDispatches) != 0 {
		t.Fatalf("provider dispatches = %v, want none (the 089 path MUST NOT emit a provider metric)", spy.providerDispatches)
	}
	if len(spy.providerObs) != 0 {
		t.Fatalf("provider observations = %v, want none (089 path)", spy.providerObs)
	}
	if f.calls != 0 {
		t.Fatalf("resolver consulted %d times for a bare model, want 0", f.calls)
	}
	for _, s := range exp.GetSpans().Snapshots() {
		if s.Name() == "model.dispatch" {
			t.Fatalf("a model.dispatch span was emitted on the byte-for-byte 089 path")
		}
	}
}

// ── #4 — hosted decrypt failure increments the vault counter, dispatches nothing ──

func TestAgent_HostedDecryptFailure_IncrementsVaultMetric_Spec096(t *testing.T) {
	const connCanary = "conn-LEAK-CANARY-anthropic-OBS-001"
	f := &fakeDispatchResolver{t: t, errs: map[string]error{
		spec096ObsHostedModel: &llm.ResolveError{
			Reason: llm.RejectDecryptFailed,
			Model:  spec096ObsHostedModel,
			Kind:   "anthropic",
			ConnID: connCanary,
		},
	}}
	installFakeDispatchResolver(t, f)

	const maxIter = 2
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		endTurn(verdict, 80),
	}}
	spy := &spyRecorder{}
	cfg := spec087Cfg(maxIter, 1)
	cfg.Recorder = spy
	base, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Hosted GATHER model → the resolve error lands on turn 0, before any Chat.
	a := base.WithModelOverride(modelswitch.Override{GatherModel: spec096ObsHostedModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused || got.TerminationReason != TerminationDispatchRejected {
		t.Fatalf("status=%q termination=%q want refused/dispatch_rejected", got.Status, got.TerminationReason)
	}
	if len(spy.vaultDecryptReasons) != 1 || spy.vaultDecryptReasons[0] != "decrypt_failed" {
		t.Fatalf("vault-decrypt reasons = %v, want exactly [decrypt_failed]", spy.vaultDecryptReasons)
	}
	// The rejected dispatch never reached Chat → no provider dispatch metric.
	if len(spy.providerDispatches) != 0 {
		t.Fatalf("provider dispatches = %v, want none (the dispatch was rejected before Chat)", spy.providerDispatches)
	}
	// Secret discipline: only the typed reason token propagates — never the
	// connection identity.
	for _, reason := range spy.vaultDecryptReasons {
		if strings.Contains(reason, connCanary) {
			t.Fatalf("connection identity leaked into the vault-decrypt reason: %q", reason)
		}
	}
}
