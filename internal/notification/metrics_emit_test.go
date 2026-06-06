package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// Spec 054 Scope 8 / SCN-054-024 contract: the notification handler MUST expose
// source-qualified pipeline-stage metrics (ingest → normalize → dedupe →
// decide/action → deliver, plus per-stage duration) using BOUNDED label values
// only — never raw payload/title/body content. These tests are adversarial:
// each increment assertion fails if the corresponding `.Inc()` emit site is
// removed, and the redaction/label-allowlist test fails if any payload-derived
// or unbounded label is introduced.

// notificationMetricLabelNames is the exact bounded label-name allowlist for
// each spec 054 notification metric family. The registration test rejects any
// metric that carries a label outside this set (cardinality/redaction guard).
var notificationMetricLabelNames = map[string][]string{
	"smackerel_notification_ingest_total":               {"source_type", "source_form", "status"},
	"smackerel_notification_normalization_errors_total": {"source_type", "error_kind"},
	"smackerel_notification_dedupe_total":               {"source_type", "suppression_kind"},
	"smackerel_notification_action_attempts_total":      {"action_class", "status"},
	"smackerel_notification_delivery_attempts_total":    {"channel", "status"},
	"smackerel_notification_processing_duration_ms":     {"stage"},
}

func notifLabelIn(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// notificationMetricTestEngine builds a minimal valid DecisionEngine for the
// metric emit tests.
func notificationMetricTestEngine(t *testing.T) DecisionEngine {
	t.Helper()
	engine, err := NewDecisionEngine(DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: SeverityHigh, LowConfidenceThreshold: 0.55, OutputChannels: []string{"dashboard"}, MaxRetries: 2})
	if err != nil {
		t.Fatalf("decision engine: %v", err)
	}
	return engine
}

// exerciseAllNotificationMetrics drives every spec 054 notification metric emit
// site through its REAL wiring so each family appears in the gatherer with at
// least one child series.
func exerciseAllNotificationMetrics(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()

	// ingest_total{rejected} + processing_duration_ms{ingest,total} via the real
	// Process() reject path (nil pool → CreateRawEvent returns a clean error).
	svc := NewService(NewStore(nil), notificationMetricTestEngine(t))
	if _, err := svc.SubmitSourceEvent(context.Background(), SourceEventEnvelope{SourceType: "metrics_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, SourceEventID: "evt-exercise", ObservedAt: now, RawPayloadKind: RawPayloadKindText, RawPayload: []byte("body")}); err == nil {
		t.Fatal("expected nil-pool ingest to be rejected")
	}

	// normalization_errors_total via the real Normalize() error path.
	if _, err := NewNormalizer().Normalize(RawEventRecord{ID: "r-exercise", SourceType: "metrics_fixture", SourceInstanceID: "src-a"}, SourceEventEnvelope{}); err == nil {
		t.Fatal("expected normalization to fail")
	}

	// dedupe_total{reaction_loop} via the real LoopGuard.Evaluate().
	origin := LoopOrigin{DecisionID: "d-exercise", OutputChannel: "dashboard", PayloadHash: "p-exercise", EmittedAt: now}
	if NewLoopGuard(10*time.Minute).Evaluate(SourceEventEnvelope{SourceType: "metrics_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, ObservedAt: now, LoopMetadata: map[string]string{"loop_guard_key": origin.Key()}}, []LoopOrigin{origin}) == nil {
		t.Fatal("expected loop suppression")
	}

	// action_attempts_total via the real Decide() suppressed path.
	notificationMetricTestEngine(t).Decide(NormalizedNotification{ID: "n-exercise"}, Classification{}, Incident{ID: "i-exercise"}, nil, []Suppression{{Kind: SuppressionDedupe}})

	// delivery_attempts_total via the real Dispatch().
	dispatcher := NewOutputDispatcher([]OutputChannel{&recordingOutputChannel{id: "dashboard"}})
	if _, err := dispatcher.Dispatch(context.Background(), DeliveryRequest{Channel: "dashboard", DestinationRef: "operator", SourceType: "webhook_fixture", SourceInstanceID: "src-a", Title: "t", Body: "b"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func TestNotificationMetricFamiliesRegisteredWithBoundedLabels(t *testing.T) {
	exerciseAllNotificationMetrics(t)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := map[string][]string{}
	for _, mf := range families {
		name := mf.GetName()
		if _, ok := notificationMetricLabelNames[name]; !ok {
			continue
		}
		seen := map[string]bool{}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				seen[lp.GetName()] = true
			}
		}
		names := make([]string, 0, len(seen))
		for n := range seen {
			names = append(names, n)
		}
		got[name] = names
	}

	for name, wantLabels := range notificationMetricLabelNames {
		gotLabels, ok := got[name]
		if !ok {
			t.Errorf("metric family %q not registered/emitted via DefaultGatherer", name)
			continue
		}
		for _, wl := range wantLabels {
			if !notifLabelIn(gotLabels, wl) {
				t.Errorf("metric %q missing expected label %q (got %v)", name, wl, gotLabels)
			}
		}
		// Adversarial cardinality/redaction guard: any label outside the
		// bounded allowlist indicates an unbounded or payload-derived label.
		for _, gl := range gotLabels {
			if !notifLabelIn(wantLabels, gl) {
				t.Errorf("metric %q carries unexpected label %q (possible unbounded/leaky label)", name, gl)
			}
		}
	}
}

func TestNotificationIngestTotalIncrementsOnRejectedRawEvent(t *testing.T) {
	svc := NewService(NewStore(nil), notificationMetricTestEngine(t))
	envelope := SourceEventEnvelope{SourceType: "ingest_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, SourceEventID: "evt-ingest", ObservedAt: time.Now().UTC(), RawPayloadKind: RawPayloadKindText, RawPayload: []byte("body")}

	rejected := metrics.NotificationIngestTotal.WithLabelValues("ingest_fixture", "webhook", "rejected")
	before := testutil.ToFloat64(rejected)

	if _, err := svc.SubmitSourceEvent(context.Background(), envelope); err == nil {
		t.Fatal("expected nil-pool ingest to fail (rejected raw event)")
	}

	after := testutil.ToFloat64(rejected)
	if after != before+1 {
		t.Fatalf("smackerel_notification_ingest_total{source_type=ingest_fixture,source_form=webhook,status=rejected} = %v, want %v", after, before+1)
	}
}

func TestNotificationDeliveryAttemptsIncrementsOnDispatch(t *testing.T) {
	dispatcher := NewOutputDispatcher([]OutputChannel{&recordingOutputChannel{id: "dashboard"}})

	success := metrics.NotificationDeliveryAttempts.WithLabelValues("dashboard", "success")
	beforeSuccess := testutil.ToFloat64(success)
	if _, err := dispatcher.Dispatch(context.Background(), DeliveryRequest{DecisionID: "d-metrics", IncidentID: "i-metrics", Channel: "dashboard", DestinationRef: "operator", SourceType: "webhook_fixture", SourceInstanceID: "src-a", Title: "needs attention", Body: "body"}); err != nil {
		t.Fatalf("dispatch (success): %v", err)
	}
	afterSuccess := testutil.ToFloat64(success)
	if afterSuccess != beforeSuccess+1 {
		t.Fatalf("smackerel_notification_delivery_attempts_total{channel=dashboard,status=success} = %v, want %v", afterSuccess, beforeSuccess+1)
	}

	// Adversarial failure path: an unconfigured channel must count a failure.
	failure := metrics.NotificationDeliveryAttempts.WithLabelValues("unconfigured", "failure")
	beforeFailure := testutil.ToFloat64(failure)
	if _, err := dispatcher.Dispatch(context.Background(), DeliveryRequest{DecisionID: "d-metrics-2", IncidentID: "i-metrics-2", Channel: "unconfigured", DestinationRef: "operator", SourceType: "webhook_fixture", SourceInstanceID: "src-a"}); err == nil {
		t.Fatal("expected unconfigured channel dispatch to fail")
	}
	afterFailure := testutil.ToFloat64(failure)
	if afterFailure != beforeFailure+1 {
		t.Fatalf("smackerel_notification_delivery_attempts_total{channel=unconfigured,status=failure} = %v, want %v", afterFailure, beforeFailure+1)
	}
}

func TestNotificationActionAttemptsIncrementsOnDecide(t *testing.T) {
	engine := notificationMetricTestEngine(t)

	suppressed := metrics.NotificationActionAttempts.WithLabelValues("no_action", "suppressed")
	before := testutil.ToFloat64(suppressed)

	engine.Decide(NormalizedNotification{ID: "n-action"}, Classification{}, Incident{ID: "i-action"}, nil, []Suppression{{Kind: SuppressionDedupe}})

	after := testutil.ToFloat64(suppressed)
	if after != before+1 {
		t.Fatalf("smackerel_notification_action_attempts_total{action_class=no_action,status=suppressed} = %v, want %v", after, before+1)
	}
}

func TestNotificationNormalizationErrorsIncrementsOnInvalidRawEvent(t *testing.T) {
	normalizer := NewNormalizer()

	errKind := metrics.NotificationNormalizationErrors.WithLabelValues("normalize_fixture", "missing_observed_at")
	before := testutil.ToFloat64(errKind)

	// ID + source identity present, ObservedAt zero → "observed_at is required".
	if _, err := normalizer.Normalize(RawEventRecord{ID: "r-normalize", SourceType: "normalize_fixture", SourceInstanceID: "src-a"}, SourceEventEnvelope{}); err == nil {
		t.Fatal("expected normalization to fail on missing observed_at")
	}

	after := testutil.ToFloat64(errKind)
	if after != before+1 {
		t.Fatalf("smackerel_notification_normalization_errors_total{source_type=normalize_fixture,error_kind=missing_observed_at} = %v, want %v", after, before+1)
	}
}

func TestNotificationDedupeTotalIncrementsOnLoopSuppression(t *testing.T) {
	now := time.Now().UTC()
	origin := LoopOrigin{DecisionID: "d-dedupe", OutputChannel: "dashboard", PayloadHash: "p-dedupe", EmittedAt: now}
	envelope := SourceEventEnvelope{SourceType: "dedupe_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, ObservedAt: now, LoopMetadata: map[string]string{"loop_guard_key": origin.Key()}}

	loop := metrics.NotificationDedupeTotal.WithLabelValues("dedupe_fixture", "reaction_loop")
	before := testutil.ToFloat64(loop)

	if NewLoopGuard(10*time.Minute).Evaluate(envelope, []LoopOrigin{origin}) == nil {
		t.Fatal("expected loop suppression")
	}

	after := testutil.ToFloat64(loop)
	if after != before+1 {
		t.Fatalf("smackerel_notification_dedupe_total{source_type=dedupe_fixture,suppression_kind=reaction_loop} = %v, want %v", after, before+1)
	}
}

func TestNotificationMetricsDoNotLeakPayloadInLabels(t *testing.T) {
	const marker = "SUPERSECRET-PAYLOAD-MARKER"

	// Drive payload-bearing free text through delivery, normalization, and ingest.
	dispatcher := NewOutputDispatcher([]OutputChannel{&recordingOutputChannel{id: "dashboard"}})
	if _, err := dispatcher.Dispatch(context.Background(), DeliveryRequest{Channel: "dashboard", DestinationRef: "operator", SourceType: "webhook_fixture", SourceInstanceID: "src-a", Title: marker, Body: "bearer " + marker}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if _, err := NewNormalizer().Normalize(RawEventRecord{ID: "r-leak", SourceType: "leak_fixture", SourceInstanceID: "src-a"}, SourceEventEnvelope{MappingHints: map[string]string{"title": marker, "body": marker}}); err == nil {
		t.Fatal("expected normalization to fail")
	}
	svc := NewService(NewStore(nil), notificationMetricTestEngine(t))
	if _, err := svc.SubmitSourceEvent(context.Background(), SourceEventEnvelope{SourceType: "leak_fixture", SourceInstanceID: "src-a", SourceForm: SourceFormWebhook, ObservedAt: time.Now().UTC(), RawPayloadKind: RawPayloadKindText, RawPayload: []byte(marker), MappingHints: map[string]string{"title": marker, "body": marker}}); err == nil {
		t.Fatal("expected nil-pool ingest to be rejected")
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		if !strings.HasPrefix(mf.GetName(), "smackerel_notification_") {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if strings.Contains(lp.GetValue(), marker) {
					t.Errorf("metric %s label %s leaked payload content: %q", mf.GetName(), lp.GetName(), lp.GetValue())
				}
			}
		}
	}
}
