// Spec 096 SCOPE-04 — the catalog aggregator (design §6.3, §2.3).
//
// CatalogAggregator runs every effective-enabled connection's discovery
// adapter in parallel — each independently bounded by the SST
// `per_provider_timeout_ms` — and merges the reachable subset into ONE
// provider-qualified ModelCatalog. It ALWAYS emits a typed
// ProviderDiscoveryStatus per adapter (reachable or not), so a slow / down /
// auth-failed provider degrades gracefully (its models absent) and is NEVER
// silently dropped. A last-good catalog is cached for `cache_ttl_ms` so a
// picker render never re-probes every provider on each call.
//
// Both bounds come from SST (SCOPE-01 ModelDiscoveryConfig); the constructor is
// fail-loud `> 0` (G028) and NO hardcoded TTL/timeout default lives in this
// file.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
)

// CatalogAggregator aggregates per-kind discovery adapters into a unified
// provider-qualified catalog, bounded by SST discovery config. It is safe for
// concurrent GetCatalog calls.
type CatalogAggregator struct {
	adapters           []DiscoveryAdapter
	cacheTTL           time.Duration
	perProviderTimeout time.Duration
	systemDefault      string
	now                func() time.Time

	// Spec 096 §13 — discovery observability. recorder is NEVER nil (the
	// constructor defaults it to okmetrics.Nop{}); tracer is nil until
	// WithObservability wires the boot tracer, and a nil tracer makes the
	// discovery spans no-ops. Both are wired by cmd/core's SCOPE-07 activation.
	// Discovery touches NO credential, so nothing emitted here carries a secret.
	recorder okmetrics.Recorder
	tracer   *tracing.Tracer

	mu     sync.Mutex
	cached *cachedCatalog
}

type cachedCatalog struct {
	catalog  ModelCatalog
	statuses []ProviderDiscoveryStatus
	builtAt  time.Time
}

// NewCatalogAggregator builds the aggregator. cacheTTLms and
// perProviderTimeoutMs come from the SST `llm.discovery` block (SCOPE-01) and
// MUST be `> 0` — a non-positive bound is a fail-loud construction error
// (G028), NEVER a substituted default. systemDefault is the no-override
// synthesis model (provider-qualified). adapters is one adapter per
// effective-enabled connection.
func NewCatalogAggregator(adapters []DiscoveryAdapter, cacheTTLms, perProviderTimeoutMs int, systemDefault string) (*CatalogAggregator, error) {
	var errs []string
	if cacheTTLms <= 0 {
		errs = append(errs, fmt.Sprintf("cache_ttl_ms must be > 0 (got %d)", cacheTTLms))
	}
	if perProviderTimeoutMs <= 0 {
		errs = append(errs, fmt.Sprintf("per_provider_timeout_ms must be > 0 (got %d)", perProviderTimeoutMs))
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("catalog: invalid SST discovery bounds: %s", strings.Join(errs, "; "))
	}
	return &CatalogAggregator{
		adapters:           adapters,
		cacheTTL:           time.Duration(cacheTTLms) * time.Millisecond,
		perProviderTimeout: time.Duration(perProviderTimeoutMs) * time.Millisecond,
		systemDefault:      systemDefault,
		now:                time.Now,
		recorder:           okmetrics.Nop{},
	}, nil
}

// WithNow overrides the clock (tests assert TTL behaviour at a fixed instant).
// Returns the receiver for chaining.
func (a *CatalogAggregator) WithNow(now func() time.Time) *CatalogAggregator {
	a.now = now
	return a
}

// WithObservability wires the spec 096 §13 discovery observability surface: the
// openknowledge metrics Recorder (per-provider reachability counter + latency
// histogram) and the assistant Tracer (model.discovery → provider.discover
// spans). A nil recorder falls back to the no-op okmetrics.Nop{}; a nil tracer
// disables span emission while the metric path still runs. Returns the receiver
// for chaining. Discovery touches NO credential, so nothing this surface emits
// can carry a secret (the span attrs are connection_id/kind/state/model_count/
// latency_ms and the metric labels are the closed provider+state vocab).
func (a *CatalogAggregator) WithObservability(recorder okmetrics.Recorder, tracer *tracing.Tracer) *CatalogAggregator {
	if recorder == nil {
		recorder = okmetrics.Nop{}
	}
	a.recorder = recorder
	a.tracer = tracer
	return a
}

// GetCatalog returns the aggregated provider-qualified catalog plus one typed
// ProviderDiscoveryStatus per adapter. A fresh last-good cache (age <
// cache_ttl_ms) is served without re-probing; otherwise the catalog is rebuilt
// with every adapter run in parallel, each bounded by per_provider_timeout_ms.
// The reachable subset is ALWAYS served — one slow / down provider never blocks
// the others and is never silently dropped.
func (a *CatalogAggregator) GetCatalog(ctx context.Context) (ModelCatalog, []ProviderDiscoveryStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cached != nil && a.now().Sub(a.cached.builtAt) < a.cacheTTL {
		return a.cached.catalog, a.cached.statuses
	}
	catalog, statuses := a.build(ctx)
	a.cached = &cachedCatalog{catalog: catalog, statuses: statuses, builtAt: a.now()}
	return catalog, statuses
}

// Spec 096 §13 — the discovery span names. model.discovery is the per-build root
// span; provider.discover is one child per adapter. The child attrs are
// connection_id/kind/state/model_count/latency_ms — all secret-free (discovery
// never touches a credential), and the free-form Detail is DELIBERATELY excluded.
const (
	discoveryRootSpanName  = "model.discovery"
	discoveryChildSpanName = "provider.discover"
)

// build runs every adapter in parallel, each bounded by the per-provider
// timeout, and assembles the catalog in adapter (declaration) order. A failing
// adapter contributes NO models but ALWAYS a typed status — graceful
// degradation, never a silent drop. It ALSO emits the spec 096 §13 discovery
// observability per provider (reachability counter + latency histogram + a
// provider.discover span under one model.discovery root); a nil tracer / the Nop
// recorder make that emission a no-op, so the aggregation behaviour is unchanged.
func (a *CatalogAggregator) build(ctx context.Context) (ModelCatalog, []ProviderDiscoveryStatus) {
	// Open the model.discovery root span (nil tracer ⇒ no spans; rootCtx == ctx).
	// Ended after every provider.discover child completes (post wg.Wait()).
	rootCtx := ctx
	var rootSpan trace.Span
	if a.tracer != nil {
		rootCtx, rootSpan = a.tracer.StartSpan(ctx, discoveryRootSpanName, "", "", "", "", "")
	}

	type slot struct {
		models []ModelDescriptor
		status ProviderDiscoveryStatus
	}
	slots := make([]slot, len(a.adapters))

	var wg sync.WaitGroup
	for i, ad := range a.adapters {
		wg.Add(1)
		go func(i int, ad DiscoveryAdapter) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(rootCtx, a.perProviderTimeout)
			defer cancel()

			st := ProviderDiscoveryStatus{ConnectionID: ad.ConnectionID(), Kind: ad.Kind()}
			start := time.Now()
			models, err := ad.Discover(cctx)
			latency := time.Since(start)
			if err != nil {
				st.State, st.Detail = classifyDiscoveryError(cctx, err)
				st.ModelCount = 0
				slots[i] = slot{status: st} // models absent — but status ALWAYS recorded
			} else {
				st.State = StateOK
				st.ModelCount = len(models)
				slots[i] = slot{models: models, status: st}
			}
			a.emitDiscoveryObservability(rootCtx, st, latency)
		}(i, ad)
	}
	wg.Wait()

	if rootSpan != nil {
		tracing.EndSpan(rootSpan, "ok", "")
	}

	catalog := ModelCatalog{Default: a.systemDefault}
	statuses := make([]ProviderDiscoveryStatus, 0, len(a.adapters))
	for _, s := range slots {
		catalog.Models = append(catalog.Models, s.models...)
		statuses = append(statuses, s.status)
	}
	return catalog, statuses
}

// emitDiscoveryObservability records the spec 096 §13 per-provider discovery
// reachability counter + latency histogram and, when a tracer is wired, opens a
// `provider.discover` child span (under the model.discovery root carried by
// rootCtx) with ONLY secret-free attrs: connection_id, kind, state, model_count,
// latency_ms. Discovery touches NO credential, so neither a metric label nor a
// span attr can carry a secret — and the free-form Detail is DELIBERATELY
// excluded (only the closed `state` token is emitted, via the span attr and the
// EndSpan error_cause). Safe to call concurrently from the per-adapter
// goroutines: the Recorder and the OTel span API are goroutine-safe.
func (a *CatalogAggregator) emitDiscoveryObservability(rootCtx context.Context, st ProviderDiscoveryStatus, latency time.Duration) {
	a.recorder.IncProviderDiscovery(st.Kind, string(st.State))
	a.recorder.ObserveProviderDiscoveryLatency(st.Kind, latency.Seconds())
	if a.tracer == nil {
		return
	}
	_, span := a.tracer.StartSpan(rootCtx, discoveryChildSpanName, "", "", "", "", "",
		attribute.String("connection_id", st.ConnectionID),
		attribute.String("kind", st.Kind),
		attribute.String("state", string(st.State)),
		attribute.Int("model_count", st.ModelCount),
		attribute.Int64("latency_ms", latency.Milliseconds()),
	)
	status, errorCause := "ok", ""
	if st.State != StateOK {
		status, errorCause = "error", string(st.State)
	}
	tracing.EndSpan(span, status, errorCause)
}

// classifyDiscoveryError maps an adapter error onto the closed DiscoveryState
// set. A typed *DiscoveryError carries its own state; a context-deadline error
// is StateTimeout; everything else is StateUnreachable. The detail is
// secret-free (adapters never place a credential in an error).
func classifyDiscoveryError(ctx context.Context, err error) (DiscoveryState, string) {
	var de *DiscoveryError
	if errors.As(err, &de) {
		detail := de.Detail
		if detail == "" {
			detail = string(de.State)
		}
		return de.State, detail
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
		return StateTimeout, "discovery timed out"
	}
	return StateUnreachable, "discovery failed: provider unreachable"
}
