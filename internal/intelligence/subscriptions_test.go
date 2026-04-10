package intelligence

import (
	"testing"
)

func TestSubscription_ParseServiceName(t *testing.T) {
	tests := []struct {
		sender   string
		title    string
		expected string
	}{
		{"billing@netflix.com", "Your monthly charge", "Netflix"},
		{"noreply@spotify.com", "Receipt for your subscription", "Spotify"},
		{"no-reply.payments@github.com", "GitHub charge", "Github"},
		{"random@unknown.org", "Some email", "Unknown"},
		{"nosender", "Title", ""},
	}

	for _, tt := range tests {
		t.Run(tt.sender, func(t *testing.T) {
			got := extractServiceName(tt.sender, tt.title)
			if got != tt.expected {
				t.Errorf("extractServiceName(%q, %q) = %q, want %q", tt.sender, tt.title, got, tt.expected)
			}
		})
	}
}

func TestExtractAmount(t *testing.T) {
	tests := []struct {
		text     string
		expected float64
	}{
		{"Your charge: $9.99", 9.99},
		{"Payment of 14.99 USD received", 14.99},
		{"USD 29.00 was charged", 29.00},
		{"No amount here", 0},
		{"$0.99 per month", 0.99},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := extractAmount(tt.text)
			if diff := got - tt.expected; diff > 0.01 || diff < -0.01 {
				t.Errorf("extractAmount(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestDetectFrequency(t *testing.T) {
	tests := []struct {
		text     string
		expected string
	}{
		{"Annual subscription renewal", "annual"},
		{"Your yearly plan", "annual"},
		{"Monthly charge", "monthly"},
		{"Weekly delivery", "weekly"},
		{"Your subscription", "monthly"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := detectFrequency(tt.text)
			if got != tt.expected {
				t.Errorf("detectFrequency(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestCategorizeService(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"Netflix", "entertainment"},
		{"Spotify", "entertainment"},
		{"Slack", "productivity"},
		{"GitHub", "productivity"},
		{"Coursera", "learning"},
		{"RandomService", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categorizeService(tt.name)
			if got != tt.expected {
				t.Errorf("categorizeService(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestToMonthly(t *testing.T) {
	tests := []struct {
		amount   float64
		freq     string
		expected float64
	}{
		{120.0, "annual", 10.0},
		{10.0, "weekly", 43.3},
		{9.99, "monthly", 9.99},
		{15.0, "", 15.0}, // unknown defaults to monthly
	}

	for _, tt := range tests {
		t.Run(tt.freq, func(t *testing.T) {
			got := toMonthly(tt.amount, tt.freq)
			if diff := got - tt.expected; diff > 0.1 || diff < -0.1 {
				t.Errorf("toMonthly(%v, %q) = %v, want %v", tt.amount, tt.freq, got, tt.expected)
			}
		})
	}
}

func TestParseSubscription_Nil(t *testing.T) {
	// No sender domain means no service name
	sub := parseSubscription("aid1", "Test", "content", "nosender")
	if sub != nil {
		t.Error("expected nil for sender without domain")
	}
}

func TestParseSubscription_Cancelled(t *testing.T) {
	sub := parseSubscription("aid1", "Your subscription has been cancelled", "content", "billing@netflix.com")
	if sub == nil {
		t.Fatal("expected non-nil subscription")
	}
	if sub.Status != "cancelled" {
		t.Errorf("expected status=cancelled, got %s", sub.Status)
	}
}

func TestParseSubscription_Trial(t *testing.T) {
	sub := parseSubscription("aid1", "Your free trial ends soon", "content", "billing@spotify.com")
	if sub == nil {
		t.Fatal("expected non-nil subscription")
	}
	if sub.Status != "trial" {
		t.Errorf("expected status=trial, got %s", sub.Status)
	}
}

func TestDetectSubscriptions_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.DetectSubscriptions(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestGetSubscriptionSummary_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GetSubscriptionSummary(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}
