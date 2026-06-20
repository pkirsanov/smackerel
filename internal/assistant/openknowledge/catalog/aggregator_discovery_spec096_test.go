// Spec 096 §13 — catalog discovery observability tests: the per-provider
// reachability counter (state=ok|timeout|unreachable) + latency observation the
// aggregator emits, and the `model.discovery` → per-provider `provider.discover`
// span tree. ADVERSARIAL: each asserts the aggregator EMITS the §13
// observability and FAILS before build() is instrumented (RED-before).
//
// UNIT tests: the REACHABLE provider is the real Ollama adapter driven by an
// injected fake HTTPDoer (no daemon); the UNREACHABLE provider is an injected
// stub returning a typed *DiscoveryError. No live network, no interception —
// they reuse fakeDoer / tagsBody / stubAdapter from aggregator_test.go.
//
// Discovery touches NO credential, so the secret-safety guard pins that the
// free-form adapter Detail NEVER leaks into a span attribute — only the closed
// `state` token may carry the outcome.
package catalog

import (
	"context"
	"strings"
	"sync"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
)

// discoveryInc / discoveryLatency capture one Recorder call each.
type discoveryInc struct{ provider, state string }

type discoveryLatency struct {
	provider string
	seconds  float64
}

// discoverySpy records the §13 discovery Recorder calls the aggregator makes. It
// embeds okmetrics.Nop so every other Recorder method is a no-op (and the spy
// survives future Recorder additions). build() probes adapters in PARALLEL, so
// the slices are mutex-guarded.
type discoverySpy struct {
	okmetrics.Nop
	mu        sync.Mutex
	incs      []discoveryInc
	latencies []discoveryLatency
}

func (s *discoverySpy) IncProviderDiscovery(provider, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incs = append(s.incs, discoveryInc{provider: provider, state: state})
}

func (s *discoverySpy) ObserveProviderDiscoveryLatency(provider string, seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies = append(s.latencies, discoveryLatency{provider: provider, seconds: seconds})
}

var _ okmetrics.Recorder = (*discoverySpy)(nil)

func countInc(incs []discoveryInc, provider, state string) int {
	n := 0
	for _, i := range incs {
		if i.provider == provider && i.state == state {
			n++
		}
	}
	return n
}

// newInMemoryCatalogTracer returns a Tracer backed by tracetest.InMemoryExporter
// + a synchronous span processor, so GetSpans() returns the complete set the
// moment GetCatalog returns. Mirrors the agent dispatch-observability helper.
func newInMemoryCatalogTracer(t *testing.T) (*tracing.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return tracing.NewTracerFromProvider(provider, "smackerel-core-test"), exp
}

// detailCanary is a free-form adapter Detail the secret-safety guard asserts
// NEVER appears in a span attribute (only the closed `state` token may).
const detailCanary = "anthropic unreachable at https://api.anthropic.test (CANARY-detail-must-not-be-a-span-attr)"

// twoProviderAggregator builds an aggregator over one REACHABLE provider (the
// real Ollama adapter, one installed model via an injected fake /api/tags) and
// one UNREACHABLE provider (an injected stub returning a typed StateUnreachable
// *DiscoveryError whose Detail is the canary). Declaration order: ollama first.
func twoProviderAggregator(t *testing.T) *CatalogAggregator {
	t.Helper()
	ollama := NewOllamaAdapter(
		"local-ollama",
		"http://ollama:11434",
		&fakeDoer{resp: tagsBody("gemma3:4b")},
		nil,
	)
	down := &stubAdapter{
		connID: "anthropic-main",
		kind:   "anthropic",
		err:    &DiscoveryError{State: StateUnreachable, Detail: detailCanary},
	}
	agg, err := NewCatalogAggregator([]DiscoveryAdapter{ollama, down}, 60000, 2000, "ollama/gemma3:4b")
	if err != nil {
		t.Fatalf("NewCatalogAggregator: %v", err)
	}
	return agg
}

// spanAttrs flattens a span's attributes into a name→emitted-value map.
func spanAttrs(s sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	if s == nil {
		return out
	}
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

// ── #1 — reachable + unreachable provider emit the per-provider reachability
//
//	counter (state=ok / state=unreachable) and a latency observation each ──
func TestCatalogAggregator_EmitsDiscoveryMetrics_Spec096(t *testing.T) {
	spy := &discoverySpy{}
	agg := twoProviderAggregator(t).WithObservability(spy, nil) // metric path only; nil tracer

	cat, statuses := agg.GetCatalog(context.Background())

	// Sanity: aggregation + graceful degradation unchanged (the reachable
	// provider's model is served; the unreachable one contributes none but a
	// typed status — never a silent drop).
	if got := cat.IDs(); len(got) != 1 || got[0] != "ollama/gemma3:4b" {
		t.Fatalf("catalog ids = %v, want [ollama/gemma3:4b] (reachable subset only)", got)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses = %d, want 2 (one per adapter, never silently dropped)", len(statuses))
	}

	spy.mu.Lock()
	defer spy.mu.Unlock()

	if got := countInc(spy.incs, "ollama", "ok"); got != 1 {
		t.Errorf("discovery counter {ollama,ok} = %d, want 1", got)
	}
	if got := countInc(spy.incs, "anthropic", "unreachable"); got != 1 {
		t.Errorf("discovery counter {anthropic,unreachable} = %d, want 1", got)
	}
	if len(spy.incs) != 2 {
		t.Errorf("total discovery increments = %d (%v), want exactly 2 (one per provider)", len(spy.incs), spy.incs)
	}

	// Latency observed exactly once per provider (wall-clock value ≥ 0).
	gotLatency := map[string]int{}
	for _, l := range spy.latencies {
		gotLatency[l.provider]++
		if l.seconds < 0 {
			t.Errorf("provider %q latency = %v, want ≥ 0", l.provider, l.seconds)
		}
	}
	if gotLatency["ollama"] != 1 || gotLatency["anthropic"] != 1 {
		t.Fatalf("latency observations = %v, want exactly one per provider", spy.latencies)
	}
}

// ── #2 — a model.discovery root span with per-provider provider.discover
//
//	children carrying the right state/kind; NO free-form Detail leaks into
//	any span attr ──
func TestCatalogAggregator_EmitsDiscoverySpanTree_Spec096(t *testing.T) {
	tr, exp := newInMemoryCatalogTracer(t)
	agg := twoProviderAggregator(t).WithObservability(nil, tr) // nil recorder ⇒ Nop; spans only

	agg.GetCatalog(context.Background())

	spans := exp.GetSpans().Snapshots()

	// Exactly one model.discovery root.
	var root sdktrace.ReadOnlySpan
	rootCount := 0
	for _, s := range spans {
		if s.Name() == "model.discovery" {
			root = s
			rootCount++
		}
	}
	if rootCount != 1 {
		t.Fatalf("model.discovery spans = %d, want exactly 1", rootCount)
	}

	// Two provider.discover children, each parented to the root.
	children := map[string]sdktrace.ReadOnlySpan{} // keyed by the kind attr
	for _, s := range spans {
		if s.Name() != "provider.discover" {
			continue
		}
		if s.Parent().SpanID() != root.SpanContext().SpanID() {
			t.Errorf("provider.discover child is not parented to the model.discovery root")
		}
		children[spanAttrs(s)["kind"]] = s
	}
	if len(children) != 2 {
		t.Fatalf("provider.discover children = %d, want 2 (one per provider)", len(children))
	}

	// Reachable Ollama child: kind=ollama, state=ok, model_count=1, conn id set.
	okChild := spanAttrs(children["ollama"])
	if okChild["kind"] != "ollama" || okChild["state"] != "ok" {
		t.Errorf("ollama child kind/state = %q/%q, want ollama/ok", okChild["kind"], okChild["state"])
	}
	if okChild["connection_id"] != "local-ollama" {
		t.Errorf("ollama child connection_id = %q, want local-ollama", okChild["connection_id"])
	}
	if okChild["model_count"] != "1" {
		t.Errorf("ollama child model_count = %q, want 1", okChild["model_count"])
	}
	if _, has := okChild["latency_ms"]; !has {
		t.Errorf("ollama child missing latency_ms attr (have %v)", okChild)
	}

	// Unreachable Anthropic child: kind=anthropic, state=unreachable, model_count=0.
	downChild := spanAttrs(children["anthropic"])
	if downChild["kind"] != "anthropic" || downChild["state"] != "unreachable" {
		t.Errorf("anthropic child kind/state = %q/%q, want anthropic/unreachable", downChild["kind"], downChild["state"])
	}
	if downChild["model_count"] != "0" {
		t.Errorf("anthropic child model_count = %q, want 0", downChild["model_count"])
	}

	// §13 secret-safety: the free-form adapter Detail (a secret-class diagnostic)
	// MUST NOT appear in ANY attribute value of ANY emitted span — only the
	// closed `state` token may carry the outcome.
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if strings.Contains(kv.Value.Emit(), "CANARY-detail-must-not-be-a-span-attr") {
				t.Fatalf("free-form Detail leaked into span %q attr %q", s.Name(), kv.Key)
			}
		}
	}
}
