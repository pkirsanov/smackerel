package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// === FormatAlertMessage edge cases ===

func TestFormatAlertMessage_EmptyTitleAndBody(t *testing.T) {
	msg := FormatAlertMessage("bill", "", "")
	// Should produce "💰 \n" — icon, space, empty title, newline, empty body
	if !strings.HasPrefix(msg, "💰") {
		t.Errorf("expected bill icon prefix, got: %q", msg)
	}
	if !strings.Contains(msg, "\n") {
		t.Errorf("expected newline separator even with empty title/body, got: %q", msg)
	}
}

func TestFormatAlertMessage_EmptyTitleNonEmptyBody(t *testing.T) {
	msg := FormatAlertMessage("trip_prep", "", "Pack bags")
	expected := "✈️ \nPack bags"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestFormatAlertMessage_NonEmptyTitleEmptyBody(t *testing.T) {
	msg := FormatAlertMessage("return_window", "Return Deadline", "")
	expected := "📦 Return Deadline\n"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestFormatAlertMessage_UnicodeInTitleAndBody(t *testing.T) {
	msg := FormatAlertMessage("bill", "日本語タイトル", "Körper über alles — «цена»")
	if !strings.Contains(msg, "日本語タイトル") {
		t.Errorf("unicode title not preserved: %q", msg)
	}
	if !strings.Contains(msg, "Körper über alles — «цена»") {
		t.Errorf("unicode body not preserved: %q", msg)
	}
}

func TestFormatAlertMessage_MultilineBody(t *testing.T) {
	body := "Line 1\nLine 2\nLine 3"
	msg := FormatAlertMessage("commitment_overdue", "Overdue", body)
	// The format is: icon + " " + title + "\n" + body
	// Body already contains newlines — they should be preserved.
	lines := strings.Split(msg, "\n")
	// First line: "⏰ Overdue", then body lines: "Line 1", "Line 2", "Line 3"
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (title + 3 body lines), got %d: %q", len(lines), msg)
	}
}

func TestFormatAlertMessage_TitleWithNewline(t *testing.T) {
	// Title containing a newline — format should pass it through as-is.
	msg := FormatAlertMessage("bill", "Title\nExtra", "Body")
	if !strings.Contains(msg, "Title\nExtra") {
		t.Errorf("newline in title should be preserved: %q", msg)
	}
}

func TestFormatAlertMessage_WhitespaceOnly(t *testing.T) {
	msg := FormatAlertMessage("bill", "   ", "  \t  ")
	if !strings.Contains(msg, "💰") {
		t.Errorf("expected bill icon in whitespace-only message: %q", msg)
	}
	// Verify format structure is still icon + space + title + newline + body
	parts := strings.SplitN(msg, "\n", 2)
	if len(parts) != 2 {
		t.Fatalf("expected format with newline separator, got: %q", msg)
	}
}

func TestFormatAlertMessage_EveryKnownType_ExactFormat(t *testing.T) {
	// Verify exact format: "{icon} {title}\n{body}" for each known type
	tests := []struct {
		alertType string
		icon      string
	}{
		{"bill", "💰"},
		{"return_window", "📦"},
		{"trip_prep", "✈️"},
		{"relationship_cooling", "👋"},
		{"commitment_overdue", "⏰"},
		{"meeting_brief", "📋"},
	}
	for _, tt := range tests {
		t.Run(tt.alertType, func(t *testing.T) {
			msg := FormatAlertMessage(tt.alertType, "T", "B")
			expected := tt.icon + " T\nB"
			if msg != expected {
				t.Errorf("expected %q, got %q", expected, msg)
			}
		})
	}
}

func TestFormatAlertMessage_FallbackIcon_ExactFormat(t *testing.T) {
	msg := FormatAlertMessage("nonexistent", "T", "B")
	expected := "🔔 T\nB"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestFormatAlertMessage_SpecialCharactersInTitle(t *testing.T) {
	// Telegram markdown special chars should pass through unescaped
	title := `*bold* _italic_ [link](url) <html>`
	msg := FormatAlertMessage("bill", title, "body")
	if !strings.Contains(msg, title) {
		t.Errorf("special characters in title should be preserved: %q", msg)
	}
}

func TestFormatAlertMessage_SpecialCharactersInBody(t *testing.T) {
	body := "`code` ```block``` ~strike~ ||spoiler||"
	msg := FormatAlertMessage("bill", "Title", body)
	if !strings.Contains(msg, body) {
		t.Errorf("special characters in body should be preserved: %q", msg)
	}
}

func TestFormatAlertMessage_OutputIsValidUTF8(t *testing.T) {
	tests := []struct {
		name      string
		alertType string
		title     string
		body      string
	}{
		{"known_type", "bill", "Title", "Body"},
		{"unknown_type", "x", "Title", "Body"},
		{"empty_type", "", "Title", "Body"},
		{"unicode", "trip_prep", "✈️ Flug", "Über München"},
		{"all_empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := FormatAlertMessage(tt.alertType, tt.title, tt.body)
			if !utf8.ValidString(msg) {
				t.Errorf("output is not valid UTF-8: %q", msg)
			}
		})
	}
}

func TestFormatAlertMessage_OutputContainsExactlyOneNewline_MinimalInput(t *testing.T) {
	// With no embedded newlines, the output should have exactly one newline
	// (the separator between title and body).
	msg := FormatAlertMessage("bill", "Title", "Body")
	count := strings.Count(msg, "\n")
	if count != 1 {
		t.Errorf("expected exactly 1 newline in minimal message, got %d: %q", count, msg)
	}
}

// === AlertTypeIcons map tests ===

func TestAlertTypeIcons_NoDuplicateValues(t *testing.T) {
	seen := make(map[string]string) // icon → alertType
	for alertType, icon := range AlertTypeIcons {
		if prev, exists := seen[icon]; exists {
			t.Errorf("duplicate icon %s: used by both %q and %q", icon, prev, alertType)
		}
		seen[icon] = alertType
	}
}

func TestAlertTypeIcons_AllKeysNonEmpty(t *testing.T) {
	for alertType, icon := range AlertTypeIcons {
		if alertType == "" {
			t.Error("AlertTypeIcons has empty string key")
		}
		if icon == "" {
			t.Errorf("AlertTypeIcons[%q] has empty icon", alertType)
		}
	}
}

func TestAlertTypeIcons_AllValuesValidUTF8(t *testing.T) {
	for alertType, icon := range AlertTypeIcons {
		if !utf8.ValidString(icon) {
			t.Errorf("icon for %q is not valid UTF-8: %q", alertType, icon)
		}
	}
}

func TestAlertTypeIcons_FallbackNotInMap(t *testing.T) {
	// The fallback icon "🔔" should not be used as a value in the map
	// to ensure it's unambiguously the "unknown type" indicator.
	for alertType, icon := range AlertTypeIcons {
		if icon == "🔔" {
			t.Errorf("alert type %q uses the fallback bell icon — this creates ambiguity with unknown types", alertType)
		}
	}
}

// === deliverPendingAlerts additional edge cases ===

func TestDeliverPendingAlerts_CancelledContext_NilEngine(t *testing.T) {
	// Pre-cancelled context with nil engine — should exit at engine-nil guard.
	s := New(nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.deliverPendingAlerts(ctx)
	// No panic = success
}

// === SCN-021-001: deliverAlertBatch happy path — alerts sent and marked delivered ===

func TestDeliverAlertBatch_HappyPath(t *testing.T) {
	alerts := []intelligence.Alert{
		{ID: "a1", AlertType: intelligence.AlertCommitmentOverdue, Title: "Overdue", Body: "Task overdue", Priority: 1},
		{ID: "a2", AlertType: intelligence.AlertBill, Title: "Bill Due", Body: "AWS invoice", Priority: 2},
	}

	var sentMessages []string
	var markedIDs []string

	sendFn := func(msg string) error {
		sentMessages = append(sentMessages, msg)
		return nil
	}
	markFn := func(_ context.Context, id string) error {
		markedIDs = append(markedIDs, id)
		return nil
	}

	delivered, failed := deliverAlertBatch(context.Background(), alerts, sendFn, markFn)

	if delivered != 2 {
		t.Errorf("expected 2 delivered, got %d", delivered)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
	if len(sentMessages) != 2 {
		t.Fatalf("expected 2 sent messages, got %d", len(sentMessages))
	}
	if !strings.Contains(sentMessages[0], "Overdue") {
		t.Errorf("first message should contain 'Overdue', got %q", sentMessages[0])
	}
	if !strings.Contains(sentMessages[1], "Bill Due") {
		t.Errorf("second message should contain 'Bill Due', got %q", sentMessages[1])
	}
	if len(markedIDs) != 2 {
		t.Fatalf("expected 2 marked IDs, got %d", len(markedIDs))
	}
	if markedIDs[0] != "a1" || markedIDs[1] != "a2" {
		t.Errorf("unexpected marked IDs: %v", markedIDs)
	}
}

// === SCN-021-014: deliverAlertBatch — Telegram send failure leaves alert pending for retry ===

func TestDeliverAlertBatch_SendFailure_AlertStaysPending(t *testing.T) {
	alerts := []intelligence.Alert{
		{ID: "a1", AlertType: intelligence.AlertBill, Title: "Bill", Body: "Due soon", Priority: 2},
		{ID: "a2", AlertType: intelligence.AlertTripPrep, Title: "Trip", Body: "Pack bags", Priority: 2},
	}

	var markedIDs []string

	sendFn := func(msg string) error {
		// First alert fails, second succeeds
		if strings.Contains(msg, "Bill") {
			return fmt.Errorf("telegram send failed")
		}
		return nil
	}
	markFn := func(_ context.Context, id string) error {
		markedIDs = append(markedIDs, id)
		return nil
	}

	delivered, failed := deliverAlertBatch(context.Background(), alerts, sendFn, markFn)

	if delivered != 1 {
		t.Errorf("expected 1 delivered, got %d", delivered)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
	// Only a2 should be marked delivered — a1 failed to send
	if len(markedIDs) != 1 || markedIDs[0] != "a2" {
		t.Errorf("expected only a2 marked delivered, got %v", markedIDs)
	}
}

// === SCN-021-015: deliverAlertBatch — empty alert list is a no-op ===

func TestDeliverAlertBatch_EmptyList_NoOp(t *testing.T) {
	sendCalled := false
	markCalled := false

	sendFn := func(_ string) error {
		sendCalled = true
		return nil
	}
	markFn := func(_ context.Context, _ string) error {
		markCalled = true
		return nil
	}

	delivered, failed := deliverAlertBatch(context.Background(), nil, sendFn, markFn)

	if delivered != 0 || failed != 0 {
		t.Errorf("expected 0/0, got delivered=%d failed=%d", delivered, failed)
	}
	if sendCalled {
		t.Error("sendFn should not be called for empty alert list")
	}
	if markCalled {
		t.Error("markFn should not be called for empty alert list")
	}
}

// === SCN-021-002: deliverAlertBatch — cap enforced by caller returning empty list ===

func TestDeliverAlertBatch_CapEnforced_EmptyFromGetPendingAlerts(t *testing.T) {
	// GetPendingAlerts enforces the 2/day cap by returning an empty list.
	// When deliverAlertBatch receives an empty list, it must be a no-op.
	// This test documents that the cap boundary is upstream (GetPendingAlerts SQL),
	// and the batch function correctly handles the empty result.
	delivered, failed := deliverAlertBatch(context.Background(), []intelligence.Alert{}, func(_ string) error {
		t.Error("sendFn should not be called when cap is reached (empty list)")
		return nil
	}, func(_ context.Context, _ string) error {
		t.Error("markFn should not be called when cap is reached (empty list)")
		return nil
	})

	if delivered != 0 || failed != 0 {
		t.Errorf("expected 0/0 for cap-enforced empty list, got delivered=%d failed=%d", delivered, failed)
	}
}

// === SCN-021-014: deliverAlertBatch — mark-delivered failure counts as failed ===

func TestDeliverAlertBatch_MarkFailure(t *testing.T) {
	alerts := []intelligence.Alert{
		{ID: "a1", AlertType: intelligence.AlertReturnWindow, Title: "Return", Body: "Deadline", Priority: 1},
	}

	sendFn := func(_ string) error { return nil }
	markFn := func(_ context.Context, _ string) error {
		return fmt.Errorf("db connection lost")
	}

	delivered, failed := deliverAlertBatch(context.Background(), alerts, sendFn, markFn)

	if delivered != 0 {
		t.Errorf("expected 0 delivered when mark fails, got %d", delivered)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}

// === Job overlap guard: each run*Job returns immediately on TryLock failure ===

func TestRunDigestJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muDigest.Lock()
	// Should return immediately without panic (TryLock fails)
	s.runDigestJob()
	s.muDigest.Unlock()
}

func TestRunTopicMomentumJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muHourly.Lock()
	s.runTopicMomentumJob()
	s.muHourly.Unlock()
}

func TestRunSynthesisJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muDaily.Lock()
	s.runSynthesisJob()
	s.muDaily.Unlock()
}

func TestRunResurfacingJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muResurface.Lock()
	s.runResurfacingJob()
	s.muResurface.Unlock()
}

func TestRunPreMeetingBriefsJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muBriefs.Lock()
	s.runPreMeetingBriefsJob()
	s.muBriefs.Unlock()
}

func TestRunWeeklySynthesisJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muWeekly.Lock()
	s.runWeeklySynthesisJob()
	s.muWeekly.Unlock()
}

func TestRunMonthlyReportJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muMonthly.Lock()
	s.runMonthlyReportJob()
	s.muMonthly.Unlock()
}

func TestRunSubscriptionDetectionJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muSubs.Lock()
	s.runSubscriptionDetectionJob()
	s.muSubs.Unlock()
}

func TestRunFrequentLookupsJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muLookups.Lock()
	s.runFrequentLookupsJob()
	s.muLookups.Unlock()
}

func TestRunAlertDeliveryJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muAlerts.Lock()
	s.runAlertDeliveryJob()
	s.muAlerts.Unlock()
}

func TestRunAlertProductionJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muAlertProd.Lock()
	s.runAlertProductionJob()
	s.muAlertProd.Unlock()
}

func TestRunRelationshipCoolingJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muRelCool.Lock()
	s.runRelationshipCoolingJob()
	s.muRelCool.Unlock()
}

func TestRunKnowledgeLintJob_OverlapGuard(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.muKnowledgeLint.Lock()
	s.runKnowledgeLintJob()
	s.muKnowledgeLint.Unlock()
}

// === runDigestJob nil-guard tests ===

func TestRunDigestJob_NilDigestGen(t *testing.T) {
	// When digestGen is nil, runDigestJob should log a warning and return.
	s := New(nil, nil, nil, nil)
	// Must not panic
	s.runDigestJob()
}
