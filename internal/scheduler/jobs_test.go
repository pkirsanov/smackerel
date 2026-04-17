package scheduler

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
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
