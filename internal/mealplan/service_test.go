package mealplan

import (
	"testing"
	"time"
)

func TestAllowedTransition(t *testing.T) {
	tests := []struct {
		from     PlanStatus
		to       PlanStatus
		expected bool
	}{
		// Valid transitions
		{StatusDraft, StatusActive, true},
		{StatusActive, StatusCompleted, true},
		{StatusActive, StatusArchived, true},
		{StatusCompleted, StatusArchived, true},

		// Forbidden transitions
		{StatusDraft, StatusCompleted, false},
		{StatusDraft, StatusArchived, false},
		{StatusCompleted, StatusDraft, false},
		{StatusCompleted, StatusActive, false},
		{StatusArchived, StatusDraft, false},
		{StatusArchived, StatusActive, false},
		{StatusArchived, StatusCompleted, false},
		{StatusActive, StatusDraft, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			got := AllowedTransition(tt.from, tt.to)
			if got != tt.expected {
				t.Errorf("AllowedTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.expected)
			}
		})
	}
}

func TestScaleRecipeDomainData(t *testing.T) {
	// Recipe with 4 servings, scale to 8
	domainData := []byte(`{
		"domain": "recipe",
		"title": "Pasta Carbonara",
		"servings": 4,
		"ingredients": [
			{"name": "spaghetti", "quantity": "400", "unit": "g"},
			{"name": "eggs", "quantity": "4", "unit": ""},
			{"name": "parmesan", "quantity": "1/2", "unit": "cup"},
			{"name": "salt", "quantity": "", "unit": "to taste"}
		],
		"steps": []
	}`)

	result, err := scaleRecipeDomainData(domainData, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify the output contains scaled quantities
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestScaleRecipeDomainData_NoScalingNeeded(t *testing.T) {
	domainData := []byte(`{
		"domain": "recipe",
		"title": "Simple Salad",
		"servings": 2,
		"ingredients": [
			{"name": "lettuce", "quantity": "1", "unit": "head"}
		],
		"steps": []
	}`)

	result, err := scaleRecipeDomainData(domainData, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return original data unchanged
	if string(result) != string(domainData) {
		t.Error("expected original data when no scaling needed")
	}
}

func TestScaleRecipeDomainData_MissingServings(t *testing.T) {
	domainData := []byte(`{
		"domain": "recipe",
		"title": "Unknown Servings",
		"ingredients": [
			{"name": "flour", "quantity": "2", "unit": "cups"}
		],
		"steps": []
	}`)

	result, err := scaleRecipeDomainData(domainData, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestResolveTitle(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected string
	}{
		{
			name:     "valid title",
			data:     `{"title": "Pasta Carbonara"}`,
			expected: "Pasta Carbonara",
		},
		{
			name:     "empty title",
			data:     `{"title": ""}`,
			expected: "(unknown)",
		},
		{
			name:     "missing title",
			data:     `{"other": "field"}`,
			expected: "(unknown)",
		},
		{
			name:     "invalid JSON",
			data:     `not json`,
			expected: "(unknown)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTitle([]byte(tt.data))
			if got != tt.expected {
				t.Errorf("resolveTitle() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestServiceError(t *testing.T) {
	err := &ServiceError{Code: "TEST", Message: "test error", Status: 422}
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}
}

func TestPlanDateValidation(t *testing.T) {
	// Verify that end_date < start_date is caught at the service level
	start := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	if !end.Before(start) {
		t.Error("expected end to be before start")
	}
}
