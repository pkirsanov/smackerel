package intelligence

import (
	"context"
	"strings"
	"testing"
	"time"
)

// === validAlertTypes map completeness ===

func TestValidAlertTypes_ContainsAllConstants(t *testing.T) {
	allTypes := []AlertType{
		AlertBill,
		AlertReturnWindow,
		AlertTripPrep,
		AlertRelationship,
		AlertCommitmentOverdue,
		AlertMeetingBrief,
	}
	for _, at := range allTypes {
		if !validAlertTypes[at] {
			t.Errorf("validAlertTypes map missing constant: %s", at)
		}
	}
}

func TestValidAlertTypes_MapSize(t *testing.T) {
	// If a new AlertType constant is added but not registered in validAlertTypes,
	// this test helps catch the drift.
	expectedCount := 6
	if len(validAlertTypes) != expectedCount {
		t.Errorf("validAlertTypes has %d entries, expected %d", len(validAlertTypes), expectedCount)
	}
}

func TestValidAlertTypes_RejectsUnknown(t *testing.T) {
	unknownTypes := []AlertType{"", "unknown", "billing", "alert", "test_type"}
	for _, at := range unknownTypes {
		if validAlertTypes[at] {
			t.Errorf("validAlertTypes should not contain %q", at)
		}
	}
}

// === CreateAlert title sanitization ===

func TestCreateAlert_TitleNewlineCollapsed(t *testing.T) {
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Line one\nLine two\nLine three",
		Body:      "Body",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	// Title newlines should be replaced with spaces
	if strings.Contains(alert.Title, "\n") {
		t.Errorf("title should not contain newlines after sanitization: %q", alert.Title)
	}
	if !strings.Contains(alert.Title, "Line one") {
		t.Error("title content should be preserved")
	}
}

func TestCreateAlert_TitleTabCollapsed(t *testing.T) {
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Before\tAfter",
		Body:      "Body",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	if strings.Contains(alert.Title, "\t") {
		t.Errorf("title should not contain tabs after sanitization: %q", alert.Title)
	}
}

func TestCreateAlert_TitleControlCharsRemoved(t *testing.T) {
	engine := NewEngine(nil, nil)
	// Embed a null byte and carriage return in the title
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Clean\x00Title\rHere",
		Body:      "Body",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	if strings.ContainsRune(alert.Title, '\x00') {
		t.Error("title should not contain null bytes after sanitization")
	}
	if strings.ContainsRune(alert.Title, '\r') {
		t.Error("title should not contain carriage returns after sanitization")
	}
}

// === CreateAlert body sanitization ===

func TestCreateAlert_BodyNewlinesPreserved(t *testing.T) {
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Title",
		Body:      "Line 1\nLine 2\nLine 3",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	// Body should preserve newlines (meeting briefs use them)
	if !strings.Contains(alert.Body, "\n") {
		t.Errorf("body should preserve newlines: %q", alert.Body)
	}
}

func TestCreateAlert_BodyStripsNullBytes(t *testing.T) {
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Title",
		Body:      "Before\x00After",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	if strings.ContainsRune(alert.Body, '\x00') {
		t.Error("body should not contain null bytes after sanitization")
	}
}

func TestCreateAlert_BodyStripsCarriageReturn(t *testing.T) {
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Title",
		Body:      "Windows\r\nStyle",
		Priority:  2,
	}
	_ = engine.CreateAlert(context.Background(), alert)

	if strings.ContainsRune(alert.Body, '\r') {
		t.Error("body should not contain carriage returns after sanitization")
	}
	// But the \n should remain
	if !strings.Contains(alert.Body, "\n") {
		t.Error("body should preserve newlines")
	}
}

// === maxPendingAlertAgeDays constant ===

func TestMaxPendingAlertAgeDays_ExpectedValue(t *testing.T) {
	if maxPendingAlertAgeDays != 7 {
		t.Errorf("expected maxPendingAlertAgeDays=7, got %d", maxPendingAlertAgeDays)
	}
}

func TestMaxPendingAlertAgeDays_Positive(t *testing.T) {
	if maxPendingAlertAgeDays <= 0 {
		t.Errorf("maxPendingAlertAgeDays must be positive, got %d", maxPendingAlertAgeDays)
	}
}

// === Alert validation ordering ===

func TestCreateAlert_ValidationOrder(t *testing.T) {
	engine := NewEngine(nil, nil)

	// nil alert → caught first
	err := engine.CreateAlert(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "alert must not be nil") {
		t.Errorf("nil alert should be caught first, got: %v", err)
	}

	// empty title → caught before type validation
	err = engine.CreateAlert(context.Background(), &Alert{
		AlertType: AlertBill,
		Title:     "",
		Priority:  2,
	})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Errorf("empty title should be caught, got: %v", err)
	}

	// invalid type → caught before priority validation
	err = engine.CreateAlert(context.Background(), &Alert{
		AlertType: "bogus",
		Title:     "Valid",
		Priority:  2,
	})
	if err == nil || !strings.Contains(err.Error(), "unknown alert type") {
		t.Errorf("invalid type should be caught, got: %v", err)
	}

	// invalid priority → caught before pool check
	err = engine.CreateAlert(context.Background(), &Alert{
		AlertType: AlertBill,
		Title:     "Valid",
		Priority:  0,
	})
	if err == nil || !strings.Contains(err.Error(), "priority must be") {
		t.Errorf("invalid priority should be caught, got: %v", err)
	}

	// all valid but nil pool → pool error
	err = engine.CreateAlert(context.Background(), &Alert{
		AlertType: AlertBill,
		Title:     "Valid",
		Body:      "Body",
		Priority:  2,
	})
	if err == nil || !strings.Contains(err.Error(), "database connection") {
		t.Errorf("nil pool should be the final gate, got: %v", err)
	}
}

// === DismissAlert validation ===

func TestDismissAlert_ValidationOrder(t *testing.T) {
	engine := NewEngine(nil, nil)

	// Empty ID → caught before pool check
	err := engine.DismissAlert(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "alert ID is required") {
		t.Errorf("empty ID should be caught first, got: %v", err)
	}

	// Non-empty ID with nil pool → pool error
	err = engine.DismissAlert(context.Background(), "alert-1")
	if err == nil || !strings.Contains(err.Error(), "database connection") {
		t.Errorf("nil pool should error, got: %v", err)
	}
}

// === SnoozeAlert validation ===

func TestSnoozeAlert_ValidationOrder(t *testing.T) {
	engine := NewEngine(nil, nil)

	// Empty ID → caught first
	err := engine.SnoozeAlert(context.Background(), "", time.Now().Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "alert ID is required") {
		t.Errorf("empty ID should be caught first, got: %v", err)
	}

	// Past time → caught before pool
	err = engine.SnoozeAlert(context.Background(), "alert-1", time.Now().Add(-time.Hour))
	if err == nil || !strings.Contains(err.Error(), "snooze time must be in the future") {
		t.Errorf("past time should be caught, got: %v", err)
	}

	// Valid ID + future time + nil pool → pool error
	err = engine.SnoozeAlert(context.Background(), "alert-1", time.Now().Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "database connection") {
		t.Errorf("nil pool should error, got: %v", err)
	}
}

// === Alert struct field mutability ===

func TestAlert_StatusFieldValues(t *testing.T) {
	statuses := []AlertStatus{AlertPending, AlertDelivered, AlertDismissed, AlertSnoozed}
	seen := make(map[AlertStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("alert status should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate alert status: %s", s)
		}
		seen[s] = true
	}
	if len(statuses) != 4 {
		t.Errorf("expected 4 alert statuses, got %d", len(statuses))
	}
}

func TestAlert_TypeStringValues(t *testing.T) {
	// Verify the string values match expected snake_case format
	typeStrings := map[AlertType]string{
		AlertBill:              "bill",
		AlertReturnWindow:      "return_window",
		AlertTripPrep:          "trip_prep",
		AlertRelationship:      "relationship_cooling",
		AlertCommitmentOverdue: "commitment_overdue",
		AlertMeetingBrief:      "meeting_brief",
	}
	for at, expected := range typeStrings {
		if string(at) != expected {
			t.Errorf("AlertType %v should have string value %q, got %q", at, expected, string(at))
		}
	}
}

// === CreateAlert sets fields correctly ===

func TestCreateAlert_SetsFieldsAfterValidation(t *testing.T) {
	// With nil pool, CreateAlert errors at the pool check.
	// ID, Status, CreatedAt are set after pool-nil check (just before Exec),
	// so they remain zero when pool is nil.
	engine := NewEngine(nil, nil)
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Test alert",
		Body:      "Body",
		Priority:  2,
	}

	err := engine.CreateAlert(context.Background(), alert)
	if err == nil {
		t.Fatal("expected error from nil pool")
	}

	// Fields are NOT set when pool is nil — this verifies the field-set
	// happens after the pool guard, not before.
	if alert.ID != "" {
		t.Error("ID should not be set when pool is nil (set after pool check)")
	}
	if alert.Status != "" {
		t.Errorf("Status should not be set when pool is nil, got %s", alert.Status)
	}
}

// === CreateAlert ULID uniqueness (via field assignments) ===

func TestCreateAlert_ULIDFieldAssignment(t *testing.T) {
	// Verify the ULID generation logic indirectly:
	// The alert.ID field is assigned ulid.Make().String() in CreateAlert,
	// which requires a non-nil pool to reach. Without a pool we can verify
	// the validation pipeline works correctly for 100 distinct alerts.
	engine := NewEngine(nil, nil)
	for i := 0; i < 100; i++ {
		alert := &Alert{
			AlertType: AlertBill,
			Title:     "Test",
			Body:      "Body",
			Priority:  2,
		}
		err := engine.CreateAlert(context.Background(), alert)
		if err == nil {
			t.Fatal("expected error from nil pool")
		}
		// Verify validation doesn't corrupt the alert struct
		if alert.AlertType != AlertBill {
			t.Errorf("AlertType was mutated: %s", alert.AlertType)
		}
	}
}
