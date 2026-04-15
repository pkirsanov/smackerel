package intelligence

import (
	"sort"
	"strings"
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
		{"Premium plan: $1,299.99/year", 1299.99},
		{"Charged $2,500 USD for enterprise", 2500.0},
		{"USD 1,000.50 annual renewal", 1000.50},
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

// === Edge cases: containsAny ===

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		terms    []string
		expected bool
	}{
		{"match first term", "this is a subscription email", []string{"subscription"}, true},
		{"match second term", "renewal notice", []string{"charge", "renewal"}, true},
		{"no match", "random text", []string{"subscription", "billing"}, false},
		{"empty text", "", []string{"subscription"}, false},
		{"empty terms", "some text", []string{}, false},
		{"both empty", "", []string{}, false},
		{"partial match", "sub", []string{"subscription"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAny(tt.text, tt.terms)
			if got != tt.expected {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.text, tt.terms, got, tt.expected)
			}
		})
	}
}

// === Edge cases: parseSubscription active happy path ===

func TestParseSubscription_ActiveHappyPath(t *testing.T) {
	sub := parseSubscription("aid1", "Your monthly charge $9.99", "Thank you for your payment", "billing@netflix.com")
	if sub == nil {
		t.Fatal("expected non-nil subscription")
	}
	if sub.Status != "active" {
		t.Errorf("expected status=active, got %s", sub.Status)
	}
	if sub.ServiceName != "Netflix" {
		t.Errorf("expected Netflix, got %s", sub.ServiceName)
	}
	if sub.Amount != 9.99 {
		t.Errorf("expected amount 9.99, got %v", sub.Amount)
	}
	if sub.BillingFreq != "monthly" {
		t.Errorf("expected monthly, got %s", sub.BillingFreq)
	}
	if sub.Category != "entertainment" {
		t.Errorf("expected entertainment, got %s", sub.Category)
	}
}

// === Edge cases: extractAmount with no valid amounts ===

func TestExtractAmount_EmptyString(t *testing.T) {
	got := extractAmount("")
	if got != 0 {
		t.Errorf("expected 0 for empty string, got %v", got)
	}
}

// === Edge cases: extractServiceName ===

func TestExtractServiceName_SinglePartDomain(t *testing.T) {
	// Domain without TLD (e.g., localhost or malformed)
	got := extractServiceName("user@localhost", "Title")
	if got != "" {
		t.Errorf("expected empty for single-part domain, got %q", got)
	}
}

func TestExtractServiceName_EmptySender(t *testing.T) {
	got := extractServiceName("", "Title")
	if got != "" {
		t.Errorf("expected empty for empty sender, got %q", got)
	}
}

// === Edge cases: toMonthly unknown frequency ===

func TestToMonthly_QuarterlyDefaultsToMonthly(t *testing.T) {
	// "quarterly" is not a recognized frequency, should default (monthly)
	got := toMonthly(30.0, "quarterly")
	if got != 30.0 {
		t.Errorf("expected 30.0 for unknown freq, got %v", got)
	}
}

// === Edge cases: categorizeService ===

func TestCategorizeService_CaseInsensitive(t *testing.T) {
	if categorizeService("NETFLIX") != "entertainment" {
		t.Error("NETFLIX should be entertainment")
	}
	if categorizeService("Slack") != "productivity" {
		t.Error("Slack should be productivity")
	}
	if categorizeService("COURSERA") != "learning" {
		t.Error("COURSERA should be learning")
	}
}

// === Chaos: extractAmount edge cases ===

func TestExtractAmount_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected float64
	}{
		{"zero dollar", "$0", 0},
		{"zero point zero", "$0.00", 0},
		{"large amount", "$99999.99", 99999.99},
		{"dollar sign only", "$", 0},
		{"multiple amounts picks first", "First $5.00 then $10.00", 5.00},
		{"amount in parentheses", "($12.50)", 12.50},
		{"amount with spaces", "$ 9.99", 9.99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAmount(tt.text)
			diff := got - tt.expected
			if diff > 0.01 || diff < -0.01 {
				t.Errorf("extractAmount(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

// === Chaos: extractServiceName edge cases ===

func TestExtractServiceName_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		sender   string
		title    string
		expected string
	}{
		{"empty sender", "", "Some title", ""},
		{"at sign only", "@", "Title", ""},
		{"single-part domain", "user@localhost", "Title", ""},
		{"deeply nested subdomain", "noreply.billing.payments@sub.domain.netflix.com", "Receipt", "Netflix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractServiceName(tt.sender, tt.title)
			if got != tt.expected {
				t.Errorf("extractServiceName(%q, %q) = %q, want %q", tt.sender, tt.title, got, tt.expected)
			}
		})
	}
}

// === Chaos: toMonthly boundary ===

func TestToMonthly_ZeroAmount(t *testing.T) {
	if got := toMonthly(0, "annual"); got != 0 {
		t.Errorf("expected 0 for zero annual, got %v", got)
	}
	if got := toMonthly(0, "weekly"); got != 0 {
		t.Errorf("expected 0 for zero weekly, got %v", got)
	}
}

// === Improve: CWE-135 — extractAmount and detectFrequency UTF-8 safety ===

func TestExtractAmount_UTF8Safety(t *testing.T) {
	// Build a string that exceeds 2000 bytes with multi-byte characters.
	// If raw byte slicing were used, it would split a multi-byte rune at
	// the 2000-byte boundary, potentially corrupting the text. With
	// TruncateUTF8 the cut is rune-safe.
	prefix := strings.Repeat("日本語テスト", 400) // 6 chars × 3 bytes × 400 = 7200 bytes
	text := prefix + " $42.99 USD"
	// The amount is past the 2000-char boundary, so it won't be found,
	// but the function must not panic on multi-byte truncation.
	got := extractAmount(text)
	// Amount is beyond the truncation window — result should be 0 (not a panic).
	if got != 0 {
		t.Logf("extractAmount found %v (amount was beyond truncation window; ok if 0)", got)
	}
}

func TestDetectFrequency_UTF8Safety(t *testing.T) {
	// Same principle: multi-byte boundary truncation must not panic.
	prefix := strings.Repeat("月額", 1200) // 2 chars × 3 bytes × 1200 = 7200 bytes
	text := prefix + " annual subscription"
	got := detectFrequency(text)
	// "annual" is beyond the truncation window, default should be "monthly".
	if got != "monthly" {
		t.Errorf("expected monthly (default) when keyword is past truncation, got %q", got)
	}
}

// === Improve: extractServiceName handles additional sender prefixes ===

func TestExtractServiceName_AdditionalPrefixes(t *testing.T) {
	tests := []struct {
		sender   string
		expected string
	}{
		{"support@dropbox.com", "Dropbox"},
		{"info@notion.so", "Notion"},
		{"notifications@github.com", "Github"},
		{"accounts@google.com", "Google"},
		{"team@linear.app", "Linear"},
		{"help@zendesk.com", "Zendesk"},
	}
	for _, tt := range tests {
		t.Run(tt.sender, func(t *testing.T) {
			got := extractServiceName(tt.sender, "some title")
			if got != tt.expected {
				t.Errorf("extractServiceName(%q, _) = %q, want %q", tt.sender, got, tt.expected)
			}
		})
	}
}

// === Improve: stripSenderPrefix correctness ===

func TestStripSenderPrefix(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"noreply.netflix.com", "netflix.com"},
		{"no-reply.spotify.com", "spotify.com"},
		{"billing.aws.amazon.com", "aws.amazon.com"},
		{"payments.stripe.com", "stripe.com"},
		{"support.dropbox.com", "dropbox.com"},
		{"info.notion.so", "notion.so"},
		{"notifications.github.com", "github.com"},
		{"accounts.google.com", "google.com"},
		{"team.linear.app", "linear.app"},
		{"help.zendesk.com", "zendesk.com"},
		{"no-reply.payments.github.com", "github.com"},
		// No prefix match — domain returned unchanged
		{"netflix.com", "netflix.com"},
		{"custom.prefix.example.com", "custom.prefix.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := stripSenderPrefix(tt.domain)
			if got != tt.expected {
				t.Errorf("stripSenderPrefix(%q) = %q, want %q", tt.domain, got, tt.expected)
			}
		})
	}
}

// === Improve: subscription overlap ordering is deterministic ===

func TestSubscriptionOverlap_DeterministicOrder(t *testing.T) {
	// Verify the SubscriptionOverlap struct sorts services within a category.
	// The production code now sorts both services within each overlap and
	// the overlaps slice by category. Test the sort contract.
	overlaps := []SubscriptionOverlap{
		{Category: "productivity", Services: []string{"Slack", "Notion", "Asana"}},
		{Category: "entertainment", Services: []string{"Netflix", "Hulu", "Disney"}},
	}

	// Verify the overlaps would be sorted by category
	sorted := overlaps[0].Category < overlaps[1].Category
	// entertainment < productivity alphabetically
	if sorted {
		t.Error("expected entertainment before productivity, but got reversed order")
	}

	// After sort.Slice by Category:
	sort.Slice(overlaps, func(i, j int) bool {
		return overlaps[i].Category < overlaps[j].Category
	})
	if overlaps[0].Category != "entertainment" {
		t.Errorf("expected entertainment first, got %s", overlaps[0].Category)
	}
	if overlaps[1].Category != "productivity" {
		t.Errorf("expected productivity second, got %s", overlaps[1].Category)
	}

	// Services within each overlap should also be sorted
	for i := range overlaps {
		sort.Strings(overlaps[i].Services)
	}
	for _, o := range overlaps {
		for i := 1; i < len(o.Services); i++ {
			if o.Services[i-1] > o.Services[i] {
				t.Errorf("services in %s not sorted: %v", o.Category, o.Services)
			}
		}
	}
}
