//go:build integration

package integration

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-04 — T004-09-TELEMETRY.
//
// The privacy risk here is specific: an insight's through-line is user content
// derived from their corpus. If it reaches a failure message, an audit row, or
// a metric label, it has left the boundary the API is careful about and landed
// somewhere with no access control at all.

const synthesisSecretThroughLine = "a highly distinctive private through line about a named person"

// A rejected candidate's audit trail must record the CLASS of problem, never
// the content that caused it.
func TestSynthesisTelemetry_RejectionAuditCarriesNoSynthesisText(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	coord, err := intelligence.NewSynthesisCoordinator(persistence,
		intelligence.SynthesisRetryPolicy{
			MaxAttempts:  2,
			InitialDelay: time.Millisecond,
			MaxDelay:     2 * time.Millisecond,
			LeaseTTL:     time.Minute,
		}, "telemetry-holder")
	if err != nil {
		t.Fatalf("construct coordinator: %v", err)
	}

	ctx := context.Background()
	runKey := synthesisClaimKey("telemetry")
	if err := coord.ClaimWindow(ctx, runKey, time.Now().UTC()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// A candidate whose content is the thing that must not leak, failing for a
	// reason derived from that content.
	cand := synthesisCompleteCandidate("operator-telemetry")
	cand.Insights[0].ThroughLine = synthesisSecretThroughLine
	cand.Insights[0].SourceArtifactIDs = nil

	_ = coord.RunWithRetry(ctx, runKey.LogicalKey(), func(ctx context.Context) error {
		_, commitErr := persistence.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
		return commitErr
	})

	rows, err := pool.Query(ctx, `
		SELECT outcome, COALESCE(failure_class, ''), COALESCE(failure_message, '')
		FROM synthesis_run_attempts`)
	if err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	defer rows.Close()

	var recorded int
	for rows.Next() {
		var outcome, class, message string
		if err := rows.Scan(&outcome, &class, &message); err != nil {
			t.Fatalf("scan attempt: %v", err)
		}
		recorded++
		for _, field := range []string{outcome, class, message} {
			if strings.Contains(field, synthesisSecretThroughLine) {
				t.Fatalf("an attempt row carried the synthesis through line: %q", field)
			}
			// Even a fragment is a leak; the corpus is the user's.
			if strings.Contains(field, "named person") {
				t.Fatalf("an attempt row carried a fragment of synthesis content: %q", field)
			}
		}
		if outcome == "failed" && class == "" {
			t.Fatal("a failed attempt recorded no failure class; the class is what makes the audit useful without the content")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate attempts: %v", err)
	}
	if recorded == 0 {
		t.Fatal("no attempts recorded; this test proves nothing unless a failure was actually audited")
	}
}

// The content-free read types must STAY content-free. A future field named
// ThroughLine or Title on either would silently start shipping user content to
// every listing, log line and metric label that renders them.
func TestSynthesisTelemetry_ReadTypesExposeNoContentFields(t *testing.T) {
	contentish := []string{"throughline", "title", "summary", "text", "body", "content", "tension", "action"}

	for _, target := range []any{
		intelligence.SynthesisLatest{},
		intelligence.SynthesisHistoryEntry{},
	} {
		typ := reflect.TypeOf(target)
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, banned := range contentish {
				if strings.Contains(name, banned) {
					t.Fatalf("%s.%s looks like a content field; latest and history are rendered in places with no access control",
						typ.Name(), typ.Field(i).Name)
				}
			}
		}
	}

	// The control: the DETAIL aggregate is where content legitimately lives, so
	// it MUST carry insights. Without this the check above would also pass on a
	// codebase that had lost the ability to return content at all.
	aggType := reflect.TypeOf(intelligence.SynthesisAggregate{})
	if _, ok := aggType.FieldByName("Insights"); !ok {
		t.Fatal("SynthesisAggregate has no Insights field; the detail path is where content belongs")
	}
}
