//go:build integration

// Spec 076 SCOPE-4b — TP-076-04b-03.
//
// Dual-write shadow comparator emits divergence telemetry. This
// integration-tagged sibling of the unit test in
// `internal/annotation/classifier_shadow_test.go` proves that the
// production wiring contract (Prometheus counter + structured log)
// holds when the comparator is consumed across package boundaries —
// i.e. from a test that lives outside the `annotation` package and
// reads the metrics through the same global registry the running
// binary uses.
//
// The shadow Classifier is scripted (no live LLM); production
// behaviour against the live LLM stack is covered by the e2e file
// `tests/e2e/assistant/annotation_classifier_e2e_test.go`.
package annotation_integration

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

type fixedShadow struct {
	it   annotation.InteractionType
	conf float64
	err  error
}

func (f fixedShadow) Classify(_ context.Context, _ string, _ annotation.SourceChannel) (annotation.InteractionType, float64, error) {
	return f.it, f.conf, f.err
}

func TestDualWriteShadowComparator_EmitsDivergenceTelemetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	type row struct {
		name        string
		shadow      annotation.Classifier
		primary     annotation.InteractionType
		wantOutcome string
		expectDiv   bool
	}

	rows := []row{
		{
			name:        "match",
			shadow:      fixedShadow{it: annotation.InteractionReadIt, conf: 0.92},
			primary:     annotation.InteractionReadIt,
			wantOutcome: "match",
		},
		{
			name:        "divergence_records_pair",
			shadow:      fixedShadow{it: annotation.InteractionUsedIt, conf: 0.9},
			primary:     annotation.InteractionBoughtIt,
			wantOutcome: "divergence",
			expectDiv:   true,
		},
		{
			name:        "shadow_below_floor_is_not_divergence",
			shadow:      fixedShadow{it: "", conf: 0.2, err: annotation.ErrBelowConfidenceFloor},
			primary:     annotation.InteractionMadeIt,
			wantOutcome: "shadow_below_floor",
		},
		{
			name:        "shadow_error_is_not_divergence",
			shadow:      fixedShadow{err: errors.New("bridge transport closed")},
			primary:     annotation.InteractionVisited,
			wantOutcome: "shadow_error",
		},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			cmp := annotation.NewShadowComparator(r.shadow, logger, 100*time.Millisecond)
			if cmp == nil {
				t.Fatalf("NewShadowComparator returned nil for non-nil shadow")
			}

			channel := annotation.ChannelTelegram
			outcomeBefore := testutil.ToFloat64(metrics.AnnotationClassifierShadowCalls.WithLabelValues(string(channel), r.wantOutcome))
			var divBefore float64
			if r.expectDiv {
				divBefore = testutil.ToFloat64(metrics.AnnotationClassifierDivergence.WithLabelValues(string(channel), string(r.primary), string(r.shadow.(fixedShadow).it)))
			}

			cmp.Compare(context.Background(), "some annotation text", channel, r.primary)

			outcomeAfter := testutil.ToFloat64(metrics.AnnotationClassifierShadowCalls.WithLabelValues(string(channel), r.wantOutcome))
			if outcomeAfter-outcomeBefore != 1 {
				t.Fatalf("outcome=%q delta=%v want 1", r.wantOutcome, outcomeAfter-outcomeBefore)
			}
			if r.expectDiv {
				divAfter := testutil.ToFloat64(metrics.AnnotationClassifierDivergence.WithLabelValues(string(channel), string(r.primary), string(r.shadow.(fixedShadow).it)))
				if divAfter-divBefore != 1 {
					t.Fatalf("divergence{primary=%q,shadow=%q} delta=%v want 1", r.primary, r.shadow.(fixedShadow).it, divAfter-divBefore)
				}
			}
		})
	}
}
