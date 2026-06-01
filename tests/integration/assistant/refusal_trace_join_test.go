//go:build integration

// Spec 071 SCOPE-04 — Refusal counter ⇄ IntentTrace.refusal_cause join (SCN-071-A07).
//
// Live-Postgres proof that the spec 064 openknowledge_refusal_total
// counter's `cause` label and the spec 071 IntentTrace.refusal_cause
// column share one closed vocabulary and join cleanly by cause:
//
//   1. Every value in contracts.AllRefusalCauses is a legal
//      IntentTrace.refusal_cause AND a legal openknowledge metric
//      `cause` label. A vocabulary drift on either side breaks the
//      dashboard join in spec 071 design §'Assistant Intents
//      Dashboard' panel 'Refusal causes'.
//
//   2. Recording an IntentTrace with refusal_cause=X and
//      incrementing openknowledge_refusal_total{cause=X} yields a
//      pair of observations that match exactly by cause label.
//      Adversarial: a regression that mapped one vocabulary onto a
//      different string (e.g. underscore→hyphen normalisation) would
//      surface as a non-empty symmetric-difference set here.

package assistant_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
)

func counterValueForCause(t *testing.T, m *okmetrics.Metrics, reg *prometheus.Registry, cause string) float64 {
	t.Helper()
	// Collect from the registry rather than touching the unexported
	// counter — exercises the same code path the /metrics endpoint
	// uses.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("registry.Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != okmetrics.NameRefusal {
			continue
		}
		for _, met := range mf.GetMetric() {
			if labelValue(met, "cause") == cause {
				return met.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// TestRefusalCauseVocabularyMatchesIntentTraceColumn — SCN-071-A07 vocabulary
// invariant. Every refusal cause the spec 064 counter accepts MUST be
// a value the IntentTrace recorder can persist into refusal_cause,
// and vice versa. The recorder column is an open string, so the test
// drives each canonical cause through a real persistence round-trip
// and asserts the value is preserved verbatim.
func TestRefusalCauseVocabularyMatchesIntentTraceColumn(t *testing.T) {
	if len(contracts.AllRefusalCauses) == 0 {
		t.Fatal("contracts.AllRefusalCauses is empty; refusal vocabulary disappeared")
	}

	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	ns := fmt.Sprintf("spec071-a07-vocab-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	// Fresh registry so the openknowledge counter is isolated from
	// any other test running in the same process.
	reg := prometheus.NewRegistry()
	m := okmetrics.New(nil)
	if err := m.Register(reg); err != nil {
		t.Fatalf("openknowledge metrics Register: %v", err)
	}

	for _, cause := range contracts.AllRefusalCauses {
		causeStr := string(cause)

		// Counter side: increment with the canonical cause label.
		m.IncRefusal(causeStr)
		if got := counterValueForCause(t, m, reg, causeStr); got != 1 {
			t.Fatalf("openknowledge_refusal_total{cause=%q} = %v, want 1 — cause label was dropped by the counter allow-set", causeStr, got)
		}

		// Trace side: persist a refused-status IntentTrace with the
		// same canonical cause.
		traceID := fmt.Sprintf("%s-%s", ns, causeStr)
		turnID := traceID + "-turn"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := recorder.Record(ctx, intenttrace.TurnTraceInput{
			TraceID:             traceID,
			TurnID:              turnID,
			UserIDHash:          "deadbeefdeadbeef",
			Transport:           intenttrace.TransportTelegram,
			TransportMessageID:  "tg-vocab",
			CompilerInvoked:     true,
			Sampled:             true,
			ActionClass:         "refused",
			SideEffectClass:     "none",
			RouteDecision:       "refusal",
			ToolCalls:           []intenttrace.ToolCallSummary{},
			FinalResponseStatus: intenttrace.StatusRefused,
			RefusalCause:        causeStr,
			SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
				RawText: "absent",
			},
			EmittedAt: time.Now().UTC(),
		})
		cancel()
		if err != nil {
			t.Fatalf("Record(cause=%q): %v", causeStr, err)
		}

		// Read back and assert the persisted refusal_cause is the
		// exact same string the counter was incremented with.
		qctx, qcancel := context.WithTimeout(context.Background(), 5*time.Second)
		row, err := store.Get(qctx, traceID)
		qcancel()
		if err != nil {
			t.Fatalf("Get(%s): %v", traceID, err)
		}
		if row.RefusalCause != causeStr {
			t.Errorf("IntentTrace.refusal_cause = %q, want %q (vocabulary drift: trace side normalised the value)", row.RefusalCause, causeStr)
		}
	}
}

// TestRefusalCounterAndIntentTraceJoinByCauseLabel — SCN-071-A07 join
// proof. Drives one refusal end-to-end and asserts the dashboard
// join key (`cause` label on the counter vs `refusal_cause` column on
// the trace row) has equal non-zero values on both sides.
func TestRefusalCounterAndIntentTraceJoinByCauseLabel(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	ns := fmt.Sprintf("spec071-a07-join-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	reg := prometheus.NewRegistry()
	m := okmetrics.New(nil)
	if err := m.Register(reg); err != nil {
		t.Fatalf("openknowledge metrics Register: %v", err)
	}

	cause := string(contracts.RefusalBudgetExhausted)

	// Drive three matching refusals on both sides.
	const n = 3
	for i := 0; i < n; i++ {
		m.IncRefusal(cause)

		traceID := fmt.Sprintf("%s-%d", ns, i)
		turnID := traceID + "-turn"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := recorder.Record(ctx, intenttrace.TurnTraceInput{
			TraceID:             traceID,
			TurnID:              turnID,
			UserIDHash:          "deadbeefdeadbeef",
			Transport:           intenttrace.TransportTelegram,
			TransportMessageID:  fmt.Sprintf("tg-join-%d", i),
			CompilerInvoked:     true,
			Sampled:             true,
			ActionClass:         "refused",
			SideEffectClass:     "none",
			RouteDecision:       "refusal",
			ToolCalls:           []intenttrace.ToolCallSummary{},
			FinalResponseStatus: intenttrace.StatusRefused,
			RefusalCause:        cause,
			SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
				RawText: "absent",
			},
			EmittedAt: time.Now().UTC(),
		})
		cancel()
		if err != nil {
			t.Fatalf("Record(%d): %v", i, err)
		}
	}

	// Join key: same `cause` string on both sides.
	counterValue := counterValueForCause(t, m, reg, cause)
	if counterValue != float64(n) {
		t.Fatalf("openknowledge_refusal_total{cause=%q} = %v, want %d", cause, counterValue, n)
	}

	qctx, qcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer qcancel()
	var traceRows int
	if err := pool.QueryRow(qctx,
		`SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1 AND refusal_cause = $2`,
		ns+"%", cause,
	).Scan(&traceRows); err != nil {
		t.Fatalf("trace row count: %v", err)
	}
	if traceRows != n {
		t.Fatalf("IntentTrace rows with refusal_cause=%q = %d, want %d", cause, traceRows, n)
	}

	// Adversarial: the join MUST be exact-equal on the cause label.
	// If the trace side were normalising (e.g. lower-casing or
	// stripping underscores), the counter+trace would each be 3 but
	// queryable only via a different join key. Prove the trace row
	// stores the exact byte sequence by reading one back.
	var stored string
	if err := pool.QueryRow(qctx,
		`SELECT refusal_cause FROM assistant_intent_traces WHERE trace_id = $1`,
		fmt.Sprintf("%s-0", ns),
	).Scan(&stored); err != nil {
		t.Fatalf("read stored cause: %v", err)
	}
	if stored != cause {
		t.Fatalf("stored refusal_cause = %q, want %q (join would silently miss matching rows)", stored, cause)
	}
}
