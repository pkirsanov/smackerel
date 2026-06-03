//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-03 / SCN-075-A03.
//
// Live-stack integration proof: dedup is keyed per-(user, command,
// window). A MarkShown for /weather MUST NOT mark /remind as
// notified for the same user, so the user sees the notice exactly
// once per distinct retired command.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// TestRetirement_DifferentCommandProducesOwnNotice covers
// SCN-075-A03 against the live SQL ledger.
func TestRetirement_DifferentCommandProducesOwnNotice(t *testing.T) {
	pool := openPool(t)
	const windowID = "tp-076-06-03-window"
	userID := fmt.Sprintf("tp-076-06-03-user-%d", time.Now().UnixNano())
	seedConversation(t, pool, userID, "telegram", windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	if err := ledger.MarkShown(ctx, userID, "/weather", windowID, now); err != nil {
		t.Fatalf("MarkShown /weather: %v", err)
	}

	// /remind dedup MUST still report not-notified.
	if ok, err := ledger.HasNotified(ctx, userID, "/remind", windowID); err != nil {
		t.Fatalf("HasNotified /remind: %v", err)
	} else if ok {
		t.Fatal("/remind dedup contaminated by /weather entry — per-command keying broken")
	}
	// /weather entry MUST still exist after the cross-command check.
	if ok, err := ledger.HasNotified(ctx, userID, "/weather", windowID); err != nil || !ok {
		t.Fatalf("HasNotified /weather post-cross-check: ok=%v err=%v — entry leaked", ok, err)
	}

	// Mark /remind and assert both entries coexist independently.
	if err := ledger.MarkShown(ctx, userID, "/remind", windowID, now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkShown /remind: %v", err)
	}
	weather, ok, err := ledger.Get(ctx, userID, "/weather", windowID)
	if err != nil || !ok {
		t.Fatalf("Get /weather: ok=%v err=%v", ok, err)
	}
	remind, ok, err := ledger.Get(ctx, userID, "/remind", windowID)
	if err != nil || !ok {
		t.Fatalf("Get /remind: ok=%v err=%v", ok, err)
	}
	if weather.NoticeCount != 1 || remind.NoticeCount != 1 {
		t.Errorf("per-command counts diverged: /weather=%d /remind=%d (each must be 1)", weather.NoticeCount, remind.NoticeCount)
	}
}
