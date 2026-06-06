package notification

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// concurrencySafeOutputChannel is a goroutine-safe OutputChannel stub for the
// concurrent emit-site race regression test. The package's recordingOutputChannel
// appends to an unsynchronized slice and is therefore unsafe to share across
// goroutines; this stub records nothing and simply reports a sent delivery so the
// only shared surface under test is the prometheus metric-vec emit site inside
// OutputDispatcher.Dispatch's defer.
type concurrencySafeOutputChannel struct{ channelID string }

func (c concurrencySafeOutputChannel) ID() string { return c.channelID }

func (c concurrencySafeOutputChannel) Deliver(context.Context, DeliveryRequest) (DeliveryResult, error) {
	return DeliveryResult{Status: DeliverySent}, nil
}

// TestNotificationMetricEmitSitesAreConcurrencySafe is the concurrency/race
// regression guard for the spec 054 notification pipeline metric emit sites
// wired in BUG-054-002. Those six families are incremented/observed inside
// `defer` blocks on the pipeline hot path (Service.Process, OutputDispatcher.
// Dispatch, DecisionEngine.Decide, Normalizer.Normalize, LoopGuard.Evaluate),
// and the handler ingests from multiple sources concurrently in production. The
// pre-existing metrics_emit_test.go drives every emit site SEQUENTIALLY, so a
// plain `go test -race` run cannot observe the concurrent-dispatch behavior.
//
// This test drives all six emit sites from many parallel goroutines. Run under
// `go test -race` it proves the CounterVec/HistogramVec .Inc()/.Observe() emit
// sites are free of data races, and the exact per-series delta assertions prove
// no increment is lost to a race (a lost-update race would fail the delta even
// if the race detector were disabled). It is adversarial: it fails with a race
// report or a wrong delta if any emit site is moved to a non-thread-safe counter
// or shared mutable state under concurrency.
func TestNotificationMetricEmitSitesAreConcurrencySafe(t *testing.T) {
	const goroutines = 16
	const iterations = 32
	const expected = goroutines * iterations

	now := time.Now().UTC()

	// Build the shared decision engine on the test goroutine so any construction
	// failure surfaces via t.Fatalf here (never from a worker goroutine). The
	// DecisionEngine, Normalizer, and LoopGuard are immutable value types and are
	// safe to share across goroutines.
	engine := notificationMetricTestEngine(t)

	// Stable, bounded child series per emit site so the deltas are exact. The
	// ingest/normalization/dedupe series use a fixture-unique source_type so no
	// other (sequential) test in the package can perturb their counts.
	ingestRejected := metrics.NotificationIngestTotal.WithLabelValues("concurrency_fixture", "webhook", "rejected")
	normErr := metrics.NotificationNormalizationErrors.WithLabelValues("concurrency_fixture", "missing_observed_at")
	dedupeLoop := metrics.NotificationDedupeTotal.WithLabelValues("concurrency_fixture", SuppressionReactionLoop)
	actionSuppressed := metrics.NotificationActionAttempts.WithLabelValues(string(DecisionNoAction), "suppressed")
	deliverySuccess := metrics.NotificationDeliveryAttempts.WithLabelValues("dashboard", "success")

	beforeIngest := testutil.ToFloat64(ingestRejected)
	beforeNorm := testutil.ToFloat64(normErr)
	beforeDedupe := testutil.ToFloat64(dedupeLoop)
	beforeAction := testutil.ToFloat64(actionSuppressed)
	beforeDelivery := testutil.ToFloat64(deliverySuccess)

	origin := LoopOrigin{DecisionID: "d-concurrency", OutputChannel: "dashboard", PayloadHash: "p-concurrency", EmittedAt: now}
	loopEnvelope := SourceEventEnvelope{SourceType: "concurrency_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, ObservedAt: now, LoopMetadata: map[string]string{"loop_guard_key": origin.Key()}}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			// Each goroutine owns its own service/dispatcher (own nil-pool Store);
			// the only shared surface is the global prometheus metric vecs.
			svc := NewService(NewStore(nil), engine)
			dispatcher := NewOutputDispatcher([]OutputChannel{concurrencySafeOutputChannel{channelID: "dashboard"}})
			for i := 0; i < iterations; i++ {
				// ingest reject path → ingest_total{rejected} + processing_duration_ms{ingest,total}
				_, _ = svc.SubmitSourceEvent(context.Background(), SourceEventEnvelope{SourceType: "concurrency_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, ObservedAt: now, RawPayloadKind: RawPayloadKindText, RawPayload: []byte("body")})
				// normalization error path → normalization_errors_total{missing_observed_at}
				_, _ = NewNormalizer().Normalize(RawEventRecord{ID: "r-concurrency", SourceType: "concurrency_fixture", SourceInstanceID: "src-a"}, SourceEventEnvelope{})
				// loop suppression path → dedupe_total{reaction_loop}
				_ = NewLoopGuard(10*time.Minute).Evaluate(loopEnvelope, []LoopOrigin{origin})
				// decide suppressed path → action_attempts_total{no_action,suppressed}
				engine.Decide(NormalizedNotification{ID: "n-concurrency"}, Classification{}, Incident{ID: "i-concurrency"}, nil, []Suppression{{Kind: SuppressionDedupe}})
				// dispatch success path → delivery_attempts_total{dashboard,success}
				_, _ = dispatcher.Dispatch(context.Background(), DeliveryRequest{Channel: "dashboard", DestinationRef: "operator", SourceType: "concurrency_fixture", SourceInstanceID: "src-a", Title: "t", Body: "b"})
			}
		}()
	}
	wg.Wait()

	// Exact-delta assertions: a lost increment (a race) makes these fail even if
	// the race detector itself observed nothing on a given run.
	assertDelta := func(name string, after, before float64) {
		t.Helper()
		if got := after - before; got != float64(expected) {
			t.Fatalf("%s delta under %d concurrent goroutines = %v, want %d (a missing increment indicates a data race or lost update at the emit site)", name, goroutines, got, expected)
		}
	}
	assertDelta("smackerel_notification_ingest_total{status=rejected}", testutil.ToFloat64(ingestRejected), beforeIngest)
	assertDelta("smackerel_notification_normalization_errors_total{error_kind=missing_observed_at}", testutil.ToFloat64(normErr), beforeNorm)
	assertDelta("smackerel_notification_dedupe_total{suppression_kind=reaction_loop}", testutil.ToFloat64(dedupeLoop), beforeDedupe)
	assertDelta("smackerel_notification_action_attempts_total{action_class=no_action,status=suppressed}", testutil.ToFloat64(actionSuppressed), beforeAction)
	assertDelta("smackerel_notification_delivery_attempts_total{channel=dashboard,status=success}", testutil.ToFloat64(deliverySuccess), beforeDelivery)
}
