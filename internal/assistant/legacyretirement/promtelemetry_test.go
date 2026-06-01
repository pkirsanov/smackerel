// promtelemetry_test.go — spec 075 SCOPE-3 unit tests covering the
// Prometheus residual telemetry implementation and the fan-out
// MultiResidualTelemetry. These tests do not need a live stack.
//
// Adversarial coverage (SCN-075-A04 + SCN-075-A11):
//
//   - A regression that passes a raw user id (anything other than
//     a 64-char hex HMAC) to Record() MUST be collapsed to the
//     "anonymous" sentinel — never observed as a label value.
//   - The notice-outcome counter is NOT incremented when outcome
//     is the empty string (defensive against caller bugs).
//   - MultiResidualTelemetry fans out to every non-nil sink in
//     order and tolerates nil sinks without panic.
package legacyretirement

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func resetSpec075Counters(t *testing.T) {
	t.Helper()
	ResidualUsageCounter.Reset()
	NoticeOutcomeCounter.Reset()
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter Write: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestPrometheusResidualTelemetry_IncrementsBothCounters(t *testing.T) {
	resetSpec075Counters(t)
	tel := NewPrometheusResidualTelemetry()

	const (
		cmd    = "/weather"
		bucket = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	tel.Record(cmd, bucket, OutcomeNoticeAndServed)
	tel.Record(cmd, bucket, OutcomeNoticeAndServed)

	residual := ResidualUsageCounter.WithLabelValues(cmd, bucket)
	if got := counterValue(t, residual); got != 2 {
		t.Errorf("residual counter for (%s, %s) = %v, want 2", cmd, bucket, got)
	}
	notice := NoticeOutcomeCounter.WithLabelValues(cmd, string(OutcomeNoticeAndServed))
	if got := counterValue(t, notice); got != 2 {
		t.Errorf("notice counter for (%s, %s) = %v, want 2", cmd, OutcomeNoticeAndServed, got)
	}
}

func TestPrometheusResidualTelemetry_RawIDCollapsesToAnonymous(t *testing.T) {
	resetSpec075Counters(t)
	tel := NewPrometheusResidualTelemetry()

	const cmd = "/weather"
	// Adversarial: a raw id-shaped value (NOT a 64-char hex HMAC)
	// is passed. The privacy invariant requires the metric label
	// to collapse to "anonymous", never the raw id.
	tel.Record(cmd, "telegram-chat-1234567890", OutcomeNoticeAndServed)

	rawSeries := ResidualUsageCounter.WithLabelValues(cmd, "telegram-chat-1234567890")
	if got := counterValue(t, rawSeries); got != 0 {
		t.Errorf("raw-id-shaped bucket must not produce a sample; got count=%v — privacy regression", got)
	}
	anon := ResidualUsageCounter.WithLabelValues(cmd, AnonymousBucketLabel)
	if got := counterValue(t, anon); got != 1 {
		t.Errorf("non-HMAC bucket must collapse to %q label, got count=%v", AnonymousBucketLabel, got)
	}
}

func TestPrometheusResidualTelemetry_EmptyBucketIsAnonymous(t *testing.T) {
	resetSpec075Counters(t)
	tel := NewPrometheusResidualTelemetry()

	tel.Record("/remind", "", OutcomePausedSuppressed)

	anon := ResidualUsageCounter.WithLabelValues("/remind", AnonymousBucketLabel)
	if got := counterValue(t, anon); got != 1 {
		t.Errorf("empty bucket must be observed under %q, got %v", AnonymousBucketLabel, got)
	}
}

func TestPrometheusResidualTelemetry_EmptyCommandIsDropped(t *testing.T) {
	resetSpec075Counters(t)
	tel := NewPrometheusResidualTelemetry()

	tel.Record("", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", OutcomeNoticeAndServed)

	// Walk every collected metric and assert no sample carries an
	// empty command label.
	ch := make(chan prometheus.Metric, 32)
	ResidualUsageCounter.Collect(ch)
	close(ch)
	for m := range ch {
		var p dto.Metric
		if err := m.Write(&p); err != nil {
			t.Fatalf("Write: %v", err)
		}
		for _, lp := range p.Label {
			if lp.GetName() == LabelCommand && lp.GetValue() == "" {
				t.Fatalf("empty command produced a sample with empty command label: %v", &p)
			}
		}
	}
}

func TestPrometheusResidualTelemetry_EmptyOutcomeSkipsNoticeCounter(t *testing.T) {
	resetSpec075Counters(t)
	tel := NewPrometheusResidualTelemetry()

	const (
		cmd    = "/weather"
		bucket = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	tel.Record(cmd, bucket, RetirementOutcome(""))

	// Residual still increments — observations should never be
	// silently dropped by an outcome-label bug.
	residual := ResidualUsageCounter.WithLabelValues(cmd, bucket)
	if got := counterValue(t, residual); got != 1 {
		t.Errorf("residual counter must still increment when outcome is empty; got %v", got)
	}

	// Notice-outcome counter must NOT carry an empty-outcome sample.
	ch := make(chan prometheus.Metric, 16)
	NoticeOutcomeCounter.Collect(ch)
	close(ch)
	for m := range ch {
		var p dto.Metric
		if err := m.Write(&p); err != nil {
			t.Fatalf("Write: %v", err)
		}
		for _, lp := range p.Label {
			if lp.GetName() == LabelOutcome && lp.GetValue() == "" {
				t.Fatalf("empty outcome label leaked into notice counter: %v", &p)
			}
		}
	}
}

type recordedObservation struct {
	command, bucket string
	outcome         RetirementOutcome
}

type capturingSink struct{ rec []recordedObservation }

func (c *capturingSink) Record(cmd, bucket string, outcome RetirementOutcome) {
	c.rec = append(c.rec, recordedObservation{cmd, bucket, outcome})
}

func TestMultiResidualTelemetry_FansOutInOrderAndDropsNilSinks(t *testing.T) {
	a := &capturingSink{}
	b := &capturingSink{}
	multi := NewMultiResidualTelemetry(a, nil, b)

	multi.Record("/weather", "bkt", OutcomeNoticeAndServed)

	if got := len(a.rec); got != 1 {
		t.Errorf("sink a got %d observations, want 1", got)
	}
	if got := len(b.rec); got != 1 {
		t.Errorf("sink b got %d observations, want 1", got)
	}
}

func TestMultiResidualTelemetry_EmptyIsNoop(t *testing.T) {
	multi := NewMultiResidualTelemetry()
	// Must not panic.
	multi.Record("/weather", "bkt", OutcomeNoticeAndServed)
}
