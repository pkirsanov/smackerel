// Spec 096 §13 — operator connection-TEST observability tests for the
// /v1/admin/model-connections/{id}/test handler: the
// openknowledge_model_connection_test_total{kind,outcome} counter and the
// `model.connection.test` span (attrs connection_id/kind/outcome). ADVERSARIAL +
// RED-before: each asserts the handler EMITS the §13 observability and FAILS
// before the emission point is wired into Test().
//
// SECRET-SAFETY is the load-bearing assertion: the Test handler decrypts the
// stored credential to probe, so these tests inject a capturingProbe that proves
// the live api_key was in handler scope during the probe (making the no-leak
// span canary NON-tautological), then pin that NEITHER the synthetic api_key NOR
// the probe's typed Detail ever appears as a span attribute — only the closed
// connection_id / kind / ok|failed outcome may.
//
// All secret values are SYNTHETIC and carry a gitleaks allow marker (repo
// convention) so the pre-commit scanner does not flag the fixtures.
package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"github.com/smackerel/smackerel/internal/config"
)

// --- fakes / helpers --------------------------------------------------------

// capturingProbe records the decrypted secret bundle it was handed so a test can
// prove the credential-decryption path actually ran (the secret was live in
// handler scope) before asserting that secret never leaked into a span attr.
type capturingProbe struct {
	result    ProbeResult
	mu        sync.Mutex
	gotSecret map[string]string
}

func (p *capturingProbe) Probe(_ context.Context, _ config.ModelConnection, secret map[string]string) ProbeResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gotSecret = secret
	return p.result
}

func (p *capturingProbe) received(field string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gotSecret[field]
}

// connTestInc captures one IncConnectionTest call.
type connTestInc struct{ kind, outcome string }

// connTestSpy records the §13 connection-test Recorder calls. It embeds
// okmetrics.Nop so every other Recorder method is a no-op (and the spy survives
// future Recorder additions).
type connTestSpy struct {
	okmetrics.Nop
	mu   sync.Mutex
	incs []connTestInc
}

func (s *connTestSpy) IncConnectionTest(kind, outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incs = append(s.incs, connTestInc{kind: kind, outcome: outcome})
}

var _ okmetrics.Recorder = (*connTestSpy)(nil)

func (s *connTestSpy) count(kind, outcome string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, i := range s.incs {
		if i.kind == kind && i.outcome == outcome {
			n++
		}
	}
	return n
}

func (s *connTestSpy) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.incs)
}

// newInMemoryAdminTracer returns a Tracer backed by tracetest.InMemoryExporter +
// a synchronous span processor, so GetSpans() returns the complete set the
// moment the handler returns.
func newInMemoryAdminTracer(t *testing.T) (*tracing.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return tracing.NewTracerFromProvider(provider, "smackerel-core-test"), exp
}

// requireSingleConnTestSpan returns the one model.connection.test span (fatal
// otherwise — a missing span is the RED state before the emission lands).
func requireSingleConnTestSpan(t *testing.T, exp *tracetest.InMemoryExporter) sdktrace.ReadOnlySpan {
	t.Helper()
	var found sdktrace.ReadOnlySpan
	n := 0
	for _, s := range exp.GetSpans().Snapshots() {
		if s.Name() == "model.connection.test" {
			found = s
			n++
		}
	}
	if n != 1 {
		t.Fatalf("model.connection.test spans = %d, want exactly 1 (span emission point missing)", n)
	}
	return found
}

// connTestSpanAttrs flattens a span's attributes into a name→emitted-value map.
func connTestSpanAttrs(s sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	if s == nil {
		return out
	}
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

// assertNoNeedleInSpanAttrs fails if any span attr value contains needle.
func assertNoNeedleInSpanAttrs(t *testing.T, attrs map[string]string, needle, label string) {
	t.Helper()
	for k, v := range attrs {
		if strings.Contains(v, needle) {
			t.Errorf("SECRET LEAK: span attr %q = %q contains the %s", k, v, label)
		}
	}
}

// --- tests ------------------------------------------------------------------

// TestAdminModelConnections_TestConnection_EmitsMetricAndSpan_Spec096
// (ADVERSARIAL) — a successful operator test emits
// openknowledge_model_connection_test_total{kind=anthropic,outcome=ok}==1 and a
// model.connection.test span carrying connection_id/kind/outcome with status=ok,
// and NO secret. RED before the emission point is wired into Test().
func TestAdminModelConnections_TestConnection_EmitsMetricAndSpan_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	const apiKey = "sk-ant-synthetic-canary-DEADbeef0000WXYZ" // gitleaks:allow
	probe := &capturingProbe{result: ProbeResult{Outcome: connstore.TestOutcomeOK}}
	spy := &connTestSpy{}
	tr, exp := newInMemoryAdminTracer(t)
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), probe).WithObservability(spy, tr))

	// Seed the write-only credential — the test probes the stored, DECRYPTED secret.
	if rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
		`{"secret_fields":{"api_key":"`+apiKey+`"}}`); rec.Code != http.StatusOK {
		t.Fatalf("seed credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST test status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// The decryption path actually ran: the live secret was in handler scope during
	// the probe, so the no-leak span canary below is NON-tautological.
	if got := probe.received("api_key"); got != apiKey {
		t.Fatalf("probe did not receive the decrypted api_key (got %q); the no-leak canary would be tautological", got)
	}

	// Metric: exactly one {anthropic, ok}, and nothing else.
	if got := spy.count(config.ModelConnectionKindAnthropic, okmetrics.ConnectionTestOutcomeOK); got != 1 {
		t.Fatalf("connection-test counter {anthropic,ok} = %d, want 1 (emission point missing)", got)
	}
	if got := spy.total(); got != 1 {
		t.Fatalf("total connection-test increments = %d, want exactly 1", got)
	}

	// Span: exactly one model.connection.test with the secret-free attrs + status=ok.
	attrs := connTestSpanAttrs(requireSingleConnTestSpan(t, exp))
	if attrs["connection_id"] != "anthropic-primary" {
		t.Errorf("span connection_id = %q, want anthropic-primary", attrs["connection_id"])
	}
	if attrs["kind"] != config.ModelConnectionKindAnthropic {
		t.Errorf("span kind = %q, want anthropic", attrs["kind"])
	}
	if attrs["outcome"] != okmetrics.ConnectionTestOutcomeOK {
		t.Errorf("span outcome = %q, want ok", attrs["outcome"])
	}
	if attrs["status"] != "ok" {
		t.Errorf("span status = %q, want ok", attrs["status"])
	}
	// SECRET-SAFETY canary: the synthetic api_key must NOT appear in any span attr.
	assertNoNeedleInSpanAttrs(t, attrs, apiKey, "synthetic api_key")
}

// TestAdminModelConnections_TestConnection_FailedOutcome_EmitsFailedMetricAndSpan_Spec096
// (ADVERSARIAL) — a truthful FAILED probe emits
// openknowledge_model_connection_test_total{kind=anthropic,outcome=failed}==1 and
// a model.connection.test span with outcome=failed / status=error, and leaks
// NEITHER the api_key NOR the probe's typed Detail (auth_failed) into any attr.
func TestAdminModelConnections_TestConnection_FailedOutcome_EmitsFailedMetricAndSpan_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	const apiKey = "sk-ant-synthetic-canary-FA11ed000000WXYZ" // gitleaks:allow
	// A truthful FAILED probe whose typed Detail must NEVER reach a span attr
	// (only the closed ok|failed outcome may).
	probe := &capturingProbe{result: ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailAuthFailed}}
	spy := &connTestSpy{}
	tr, exp := newInMemoryAdminTracer(t)
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), probe).WithObservability(spy, tr))

	if rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
		`{"secret_fields":{"api_key":"`+apiKey+`"}}`); rec.Code != http.StatusOK {
		t.Fatalf("seed credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST test status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	// Control: the truthful outcome really is failed (the emission is not asserting
	// against a fabricated ok).
	res := decodeJSON(t, rec)
	if res["outcome"] != connstore.TestOutcomeFailed {
		t.Fatalf("truthful outcome=%v, want failed", res["outcome"])
	}
	if got := probe.received("api_key"); got != apiKey {
		t.Fatalf("probe did not receive the decrypted api_key (got %q); the no-leak canary would be tautological", got)
	}

	// Metric: exactly one {anthropic, failed}.
	if got := spy.count(config.ModelConnectionKindAnthropic, okmetrics.ConnectionTestOutcomeFailed); got != 1 {
		t.Fatalf("connection-test counter {anthropic,failed} = %d, want 1 (emission point missing)", got)
	}

	// Span: outcome=failed, status=error, error_cause=failed (the closed outcome
	// token — NOT the typed Detail).
	attrs := connTestSpanAttrs(requireSingleConnTestSpan(t, exp))
	if attrs["outcome"] != okmetrics.ConnectionTestOutcomeFailed {
		t.Errorf("span outcome = %q, want failed", attrs["outcome"])
	}
	if attrs["status"] != "error" {
		t.Errorf("span status = %q, want error", attrs["status"])
	}
	if attrs["error_cause"] != okmetrics.ConnectionTestOutcomeFailed {
		t.Errorf("span error_cause = %q, want failed (closed outcome token, not the typed Detail)", attrs["error_cause"])
	}
	// SECRET-SAFETY: neither the api_key NOR the typed probe Detail may appear in a span attr.
	assertNoNeedleInSpanAttrs(t, attrs, apiKey, "synthetic api_key")
	for k, v := range attrs {
		if v == ProbeDetailAuthFailed {
			t.Errorf("span attr %q leaked the probe Detail %q (only the closed ok|failed outcome is allowed)", k, ProbeDetailAuthFailed)
		}
	}
}
