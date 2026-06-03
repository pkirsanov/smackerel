// Spec 076 SCOPE-4b — TP-076-04b-03.
//
// Dual-write shadow comparator: every annotation call routed through
// `ShadowComparator.Compare` MUST emit the right
// `smackerel_annotation_classifier_shadow_calls_total` outcome label
// AND, on divergence, also increment
// `smackerel_annotation_classifier_divergence_total` with the
// `primary_type` / `shadow_type` pair.
//
// This test exercises the comparator with a scripted shadow Classifier
// that lets us drive every outcome class:
//
//   - match               — shadow returns same InteractionType as primary
//   - divergence          — shadow returns a different InteractionType
//   - shadow_below_floor  — shadow returns ErrBelowConfidenceFloor
//   - shadow_error        — shadow returns a generic error
//
// No live LLM / bridge is exercised; TP-076-04b-04 covers the live
// stack end-to-end path. Metrics are observed via the global registry
// so the assertions match the wiring layer's emission contract
// verbatim (the production binary increments these same global
// counters).
package annotation_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/metrics"
)

// scriptedClassifier is a Classifier whose result is fully scripted
// by the test. Used as the SHADOW only (the inline interactionMap is
// the primary in production, but the comparator only receives the
// primary's already-computed InteractionType, so the test can pass
// any value as `primary`).
type scriptedClassifier struct {
	it   annotation.InteractionType
	conf float64
	err  error
}

func (s scriptedClassifier) Classify(_ context.Context, _ string, _ annotation.SourceChannel) (annotation.InteractionType, float64, error) {
	return s.it, s.conf, s.err
}

func TestDualWriteShadowComparator_EmitsDivergenceTelemetry(t *testing.T) {
	silentLog := slog.New(slog.NewTextHandler(io.Discard, nil))

	type tc struct {
		name           string
		shadow         scriptedClassifier
		primary        annotation.InteractionType
		wantOutcome    string
		wantDivergence bool // also expects divergence counter increment
	}

	cases := []tc{
		{
			name:        "match",
			shadow:      scriptedClassifier{it: annotation.InteractionMadeIt, conf: 0.95},
			primary:     annotation.InteractionMadeIt,
			wantOutcome: "match",
		},
		{
			name:           "divergence",
			shadow:         scriptedClassifier{it: annotation.InteractionTriedIt, conf: 0.9},
			primary:        annotation.InteractionMadeIt,
			wantOutcome:    "divergence",
			wantDivergence: true,
		},
		{
			name:        "shadow_below_floor",
			shadow:      scriptedClassifier{it: "", conf: 0.2, err: annotation.ErrBelowConfidenceFloor},
			primary:     annotation.InteractionBoughtIt,
			wantOutcome: "shadow_below_floor",
		},
		{
			name:        "shadow_error",
			shadow:      scriptedClassifier{err: errors.New("bridge unavailable")},
			primary:     annotation.InteractionVisited,
			wantOutcome: "shadow_error",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := annotation.NewShadowComparator(c.shadow, silentLog, 50*time.Millisecond)
			if cmp == nil {
				t.Fatalf("NewShadowComparator returned nil for non-nil shadow")
			}

			channel := annotation.ChannelAPI

			beforeOutcome := testutil.ToFloat64(
				metrics.AnnotationClassifierShadowCalls.WithLabelValues(string(channel), c.wantOutcome))
			var beforeDiv float64
			if c.wantDivergence {
				beforeDiv = testutil.ToFloat64(
					metrics.AnnotationClassifierDivergence.WithLabelValues(
						string(channel), string(c.primary), string(c.shadow.it)))
			}

			cmp.Compare(context.Background(), "made it last night", channel, c.primary)

			afterOutcome := testutil.ToFloat64(
				metrics.AnnotationClassifierShadowCalls.WithLabelValues(string(channel), c.wantOutcome))
			if afterOutcome-beforeOutcome != 1 {
				t.Fatalf("shadow_calls_total{channel=%q,outcome=%q} delta = %v, want 1",
					channel, c.wantOutcome, afterOutcome-beforeOutcome)
			}

			if c.wantDivergence {
				afterDiv := testutil.ToFloat64(
					metrics.AnnotationClassifierDivergence.WithLabelValues(
						string(channel), string(c.primary), string(c.shadow.it)))
				if afterDiv-beforeDiv != 1 {
					t.Fatalf("divergence_total{channel=%q,primary=%q,shadow=%q} delta = %v, want 1",
						channel, c.primary, c.shadow.it, afterDiv-beforeDiv)
				}
			}
		})
	}

	// Nil comparator is a safe no-op for Compare (call-site guard
	// contract); calling it must not panic and must not increment any
	// metric.
	t.Run("nil_comparator_is_noop", func(t *testing.T) {
		var nilCmp *annotation.ShadowComparator
		nilCmp.Compare(context.Background(), "anything", annotation.ChannelAPI, annotation.InteractionMadeIt)
	})
}
