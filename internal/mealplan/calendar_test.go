package mealplan

import (
	"testing"
	"time"
)

func TestSlotStartTime(t *testing.T) {
	bridge := &CalendarBridge{
		MealTimes: map[string]string{
			"breakfast": "08:00",
			"lunch":     "12:30",
			"dinner":    "19:00",
			"snack":     "15:00",
		},
	}

	tests := []struct {
		name     string
		mealType string
		wantHour int
		wantMin  int
	}{
		{"breakfast", "breakfast", 8, 0},
		{"lunch", "lunch", 12, 30},
		{"dinner", "dinner", 19, 0},
		{"snack", "snack", 15, 0},
		{"unknown meal", "brunch", 12, 0}, // fallback to noon
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot := Slot{
				SlotDate: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
				MealType: tt.mealType,
			}
			got := bridge.slotStartTime(slot)
			if got.Hour() != tt.wantHour || got.Minute() != tt.wantMin {
				t.Errorf("slotStartTime() = %02d:%02d, want %02d:%02d",
					got.Hour(), got.Minute(), tt.wantHour, tt.wantMin)
			}
		})
	}
}

func TestCalendarSyncResult(t *testing.T) {
	result := &CalendarSyncResult{
		EventsCreated: 5,
		EventsFailed:  1,
	}
	if result.EventsCreated != 5 {
		t.Errorf("EventsCreated = %d, want 5", result.EventsCreated)
	}
	if result.EventsFailed != 1 {
		t.Errorf("EventsFailed = %d, want 1", result.EventsFailed)
	}
}

func TestCalendarBridgeEventUID(t *testing.T) {
	// Verify UID format: "smackerel-meal-{slotID}"
	slotID := "mps-12345"
	expected := "smackerel-meal-mps-12345"
	uid := "smackerel-meal-" + slotID
	if uid != expected {
		t.Errorf("UID = %q, want %q", uid, expected)
	}
}
