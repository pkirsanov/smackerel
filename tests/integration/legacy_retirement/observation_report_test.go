//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-08 / SCN-075-A08.
//
// Live-stack integration proof: the SQL observation report blocks
// final retired-handler deletion when any retired-handler invocation
// is observed during the post-window interval. The EligibleForFinalDeletion
// gate must return false unless the snapshot has zero invocations
// AND spans at least the configured minimum duration.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

type fakeInvocationSource struct{ count int }

func (f *fakeInvocationSource) CountInvocations(_ context.Context, _, _ time.Time) (int, error) {
	return f.count, nil
}

// TestRetirement_ZeroInvocationGateBlocksDeletion covers
// SCN-075-A08 against the live test-stack Postgres. Two snapshots
// are generated for distinct window ids: one with one stray
// invocation (deletion MUST be blocked) and one with zero
// invocations spanning the minimum duration (deletion MUST be
// permitted).
func TestRetirement_ZeroInvocationGateBlocksDeletion(t *testing.T) {
	pool := openPool(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	const observationDuration = 14 * 24 * time.Hour
	const minDuration = 14 * 24 * time.Hour

	blockedWindow := fmt.Sprintf("tp-076-06-08-blocked-%d", time.Now().UnixNano())
	clearWindow := fmt.Sprintf("tp-076-06-08-clear-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_observations WHERE window_id IN ($1, $2)`, blockedWindow, clearWindow)
	})
	ctx := context.Background()

	// Case 1: one stray retired-handler invocation → deletion blocked.
	blockedReport, err := legacyretirement.NewSQLObservationReport(legacyretirement.SQLObservationReportConfig{
		Pool:   pool,
		Source: &fakeInvocationSource{count: 1},
		Clock:  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSQLObservationReport blocked: %v", err)
	}
	blockedSnap, err := blockedReport.Generate(ctx, blockedWindow, observationDuration)
	if err != nil {
		t.Fatalf("Generate blocked: %v", err)
	}
	if blockedSnap.RetiredHandlerInvocations != 1 {
		t.Errorf("blocked snapshot invocations=%d, want 1", blockedSnap.RetiredHandlerInvocations)
	}
	if blockedSnap.EligibleForFinalDeletion(minDuration) {
		t.Fatal("EligibleForFinalDeletion=true with non-zero invocations — gate broken (deletion would proceed despite stray retired-handler hit)")
	}

	// Case 2: zero invocations + sufficient duration → eligible.
	clearReport, err := legacyretirement.NewSQLObservationReport(legacyretirement.SQLObservationReportConfig{
		Pool:   pool,
		Source: &fakeInvocationSource{count: 0},
		Clock:  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSQLObservationReport clear: %v", err)
	}
	clearSnap, err := clearReport.Generate(ctx, clearWindow, observationDuration)
	if err != nil {
		t.Fatalf("Generate clear: %v", err)
	}
	if !clearSnap.EligibleForFinalDeletion(minDuration) {
		t.Fatal("EligibleForFinalDeletion=false with zero invocations and sufficient duration — gate too strict (legitimate deletion blocked)")
	}

	// Adversarial: a zero-invocation snapshot whose duration is one
	// hour MUST NOT be eligible (minimum observation window not met).
	short, err := clearReport.Generate(ctx, clearWindow, time.Hour)
	if err != nil {
		t.Fatalf("Generate short: %v", err)
	}
	if short.EligibleForFinalDeletion(minDuration) {
		t.Fatal("EligibleForFinalDeletion=true for one-hour observation — duration gate not enforced")
	}

	// The persisted rows MUST exist and round-trip through LatestSnapshot.
	latest, ok, err := clearReport.LatestSnapshot(ctx, clearWindow)
	if err != nil || !ok {
		t.Fatalf("LatestSnapshot clear: ok=%v err=%v", ok, err)
	}
	if latest.RetiredHandlerInvocations != 0 {
		t.Errorf("latest clear invocations=%d, want 0", latest.RetiredHandlerInvocations)
	}
}
