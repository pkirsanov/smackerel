package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Subscription represents a detected recurring service charge.
type Subscription struct {
	ID           string     `json:"id"`
	ServiceName  string     `json:"service_name"`
	Amount       float64    `json:"amount"`
	Currency     string     `json:"currency"`
	BillingFreq  string     `json:"billing_freq"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	DetectedFrom string     `json:"detected_from"`
	FirstSeen    time.Time  `json:"first_seen"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SubscriptionOverlap represents services that overlap in functionality.
type SubscriptionOverlap struct {
	Category     string   `json:"category"`
	Services     []string `json:"services"`
	CombinedCost float64  `json:"combined_monthly_cost"`
}

// SubscriptionSummary provides the current subscription state.
type SubscriptionSummary struct {
	Active       []Subscription        `json:"active"`
	MonthlyTotal float64               `json:"monthly_total"`
	Overlaps     []SubscriptionOverlap `json:"overlaps"`
	GeneratedAt  time.Time             `json:"generated_at"`
}

var (
	billingKeywords = []string{
		"charge", "receipt", "billing", "subscription", "monthly",
		"annual", "renewal", "trial", "payment", "invoice",
	}
	// Match patterns like $9.99, 9.99 USD, USD 9.99
	amountPattern = regexp.MustCompile(`\$\s*(\d+\.?\d*)|(\d+\.?\d*)\s*USD|USD\s*(\d+\.?\d*)`)
)

// DetectSubscriptions scans email artifacts for recurring billing patterns per R-504.
func (e *Engine) DetectSubscriptions(ctx context.Context) ([]Subscription, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("subscription detection requires a database connection")
	}

	// Find email artifacts containing billing keywords that aren't already tracked
	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.raw_content, a.source_id,
		       COALESCE(a.metadata->>'sender', '') as sender
		FROM artifacts a
		WHERE a.source_id IN ('gmail', 'imap', 'outlook')
		AND (
			LOWER(a.title) SIMILAR TO '%(' || $1 || ')%'
			OR LOWER(a.raw_content) SIMILAR TO '%(' || $1 || ')%'
		)
		AND NOT EXISTS (
			SELECT 1 FROM subscriptions s WHERE s.detected_from = a.id
		)
		ORDER BY a.created_at DESC
		LIMIT 100
	`, strings.Join(billingKeywords, "|"))
	if err != nil {
		return nil, fmt.Errorf("query billing emails: %w", err)
	}
	defer rows.Close()

	var detected []Subscription
	for rows.Next() {
		var artifactID, title, content, sourceID, sender string
		if err := rows.Scan(&artifactID, &title, &content, &sourceID, &sender); err != nil {
			slog.Warn("subscription scan failed", "error", err)
			continue
		}

		sub := parseSubscription(artifactID, title, content, sender)
		if sub == nil {
			continue
		}

		// Insert into database
		_, err := e.Pool.Exec(ctx, `
			INSERT INTO subscriptions (id, service_name, amount, currency, billing_freq, category, status, detected_from, first_seen, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (detected_from) DO NOTHING
		`, sub.ID, sub.ServiceName, sub.Amount, sub.Currency, sub.BillingFreq,
			sub.Category, sub.Status, sub.DetectedFrom, sub.FirstSeen, sub.CreatedAt)
		if err != nil {
			slog.Warn("insert subscription failed", "service", sub.ServiceName, "error", err)
			continue
		}

		detected = append(detected, *sub)
	}

	return detected, rows.Err()
}

// GetSubscriptionSummary returns the current subscription state with overlap analysis.
func (e *Engine) GetSubscriptionSummary(ctx context.Context) (*SubscriptionSummary, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("subscription summary requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, service_name, amount, currency, billing_freq, category, status,
		       detected_from, first_seen, cancelled_at, created_at
		FROM subscriptions
		WHERE status = 'active'
		ORDER BY amount DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer rows.Close()

	summary := &SubscriptionSummary{GeneratedAt: time.Now()}
	categoryMap := make(map[string][]string)
	categoryCost := make(map[string]float64)

	for rows.Next() {
		var s Subscription
		if err := rows.Scan(
			&s.ID, &s.ServiceName, &s.Amount, &s.Currency, &s.BillingFreq,
			&s.Category, &s.Status, &s.DetectedFrom, &s.FirstSeen,
			&s.CancelledAt, &s.CreatedAt,
		); err != nil {
			slog.Warn("subscription summary scan failed", "error", err)
			continue
		}

		monthly := toMonthly(s.Amount, s.BillingFreq)
		summary.MonthlyTotal += monthly
		summary.Active = append(summary.Active, s)

		if s.Category != "" {
			categoryMap[s.Category] = append(categoryMap[s.Category], s.ServiceName)
			categoryCost[s.Category] += monthly
		}
	}

	// Overlap detection: 2+ services in same category
	for cat, services := range categoryMap {
		if len(services) >= 2 {
			summary.Overlaps = append(summary.Overlaps, SubscriptionOverlap{
				Category:     cat,
				Services:     services,
				CombinedCost: categoryCost[cat],
			})
		}
	}

	return summary, rows.Err()
}

// parseSubscription extracts subscription data from an email artifact.
func parseSubscription(artifactID, title, content, sender string) *Subscription {
	serviceName := extractServiceName(sender, title)
	if serviceName == "" {
		return nil
	}

	amount := extractAmount(title + " " + content)
	freq := detectFrequency(title + " " + content)
	category := categorizeService(serviceName)
	status := "active"
	if containsAny(strings.ToLower(title+" "+content), []string{"cancel", "cancelled", "unsubscribe"}) {
		status = "cancelled"
	} else if containsAny(strings.ToLower(title+" "+content), []string{"free trial", "trial ends", "trial exp"}) {
		status = "trial"
	}

	return &Subscription{
		ID:           ulid.Make().String(),
		ServiceName:  serviceName,
		Amount:       amount,
		Currency:     "USD",
		BillingFreq:  freq,
		Category:     category,
		Status:       status,
		DetectedFrom: artifactID,
		FirstSeen:    time.Now(),
		CreatedAt:    time.Now(),
	}
}

func extractServiceName(sender, title string) string {
	// Try to get service name from sender domain
	if at := strings.LastIndex(sender, "@"); at >= 0 {
		domain := sender[at+1:]
		// Strip common email prefixes
		domain = strings.TrimPrefix(domain, "noreply.")
		domain = strings.TrimPrefix(domain, "no-reply.")
		domain = strings.TrimPrefix(domain, "billing.")
		domain = strings.TrimPrefix(domain, "payments.")
		// Use domain without TLD as service name
		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			return cases.Title(language.English).String(parts[len(parts)-2])
		}
	}
	return ""
}

func extractAmount(text string) float64 {
	// Limit input to first 2000 chars to bound regex cost on large emails.
	const maxExtractLen = 2000
	if len(text) > maxExtractLen {
		text = text[:maxExtractLen]
	}
	// Strip commas from amounts like "$1,299.99" before matching.
	normalized := strings.ReplaceAll(text, ",", "")
	matches := amountPattern.FindStringSubmatch(normalized)
	if len(matches) == 0 {
		return 0
	}
	for _, m := range matches[1:] {
		if m != "" {
			var amount float64
			fmt.Sscanf(m, "%f", &amount)
			return amount
		}
	}
	return 0
}

func detectFrequency(text string) string {
	// Limit input to first 2000 chars to bound scan cost on large emails.
	const maxScanLen = 2000
	if len(text) > maxScanLen {
		text = text[:maxScanLen]
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "annual") || strings.Contains(lower, "yearly"):
		return "annual"
	case strings.Contains(lower, "weekly"):
		return "weekly"
	default:
		return "monthly"
	}
}

func categorizeService(name string) string {
	lower := strings.ToLower(name)
	productivityServices := []string{"slack", "notion", "linear", "figma", "github", "gitlab", "jira", "asana", "trello", "zoom", "grammarly", "languagetool"}
	entertainmentServices := []string{"netflix", "spotify", "hulu", "disney", "hbo", "apple", "youtube", "twitch", "audible"}
	learningServices := []string{"coursera", "udemy", "skillshare", "masterclass", "pluralsight", "linkedin"}

	for _, s := range productivityServices {
		if strings.Contains(lower, s) {
			return "productivity"
		}
	}
	for _, s := range entertainmentServices {
		if strings.Contains(lower, s) {
			return "entertainment"
		}
	}
	for _, s := range learningServices {
		if strings.Contains(lower, s) {
			return "learning"
		}
	}
	return "other"
}

func toMonthly(amount float64, freq string) float64 {
	switch freq {
	case "annual":
		return amount / 12
	case "weekly":
		return amount * 4.33
	default:
		return amount
	}
}

func containsAny(text string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}
