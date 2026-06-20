package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/smackerel/smackerel/internal/stringutil"
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
	// IMP-006-R14-001: Extended to detect USD ($), EUR (€), and GBP (£) patterns.
	// Match patterns like $9.99, €9.99, £9.99, 9.99 USD, USD 9.99, 9.99 EUR, etc.
	amountPatternUSD = regexp.MustCompile(`\$\s*(\d+\.?\d*)|(\d+\.?\d*)\s*USD|USD\s*(\d+\.?\d*)`)
	amountPatternEUR = regexp.MustCompile(`€\s*(\d+\.?\d*)|(\d+\.?\d*)\s*EUR|EUR\s*(\d+\.?\d*)`)
	amountPatternGBP = regexp.MustCompile(`£\s*(\d+\.?\d*)|(\d+\.?\d*)\s*GBP|GBP\s*(\d+\.?\d*)`)
)

// cancellationEventPhrases denote an actual cancellation EVENT in a billing
// email. CHA-006-R17-001/002: the previous detector used the bare substrings
// "cancel" and "unsubscribe", which produced pervasive false cancellations —
// "cancel anytime" appears in a large share of legitimate active-subscription
// receipts, and an "Unsubscribe" footer link appears in virtually every billing
// email. Because DetectSubscriptions routes any "cancelled" parse into
// UPDATE ... SET status='cancelled' WHERE service_name=$1 AND status='active',
// either false positive would silently cancel a live subscription on its next
// receipt. We now require an explicit cancellation-event phrase, covering both
// UK ("cancelled") and US ("canceled") spellings. Future-tense phrasing
// ("will be cancelled") is intentionally excluded: a scheduled cancellation
// leaves the subscription active until the period ends.
var cancellationEventPhrases = []string{
	"has been cancelled", "has been canceled",
	"was cancelled", "was canceled",
	"subscription cancelled", "subscription canceled",
	"membership cancelled", "membership canceled",
	"plan cancelled", "plan canceled",
	"successfully cancelled", "successfully canceled",
	"you have cancelled", "you have canceled",
	"you've cancelled", "you've canceled",
	"we have cancelled", "we have canceled",
	"we've cancelled", "we've canceled",
	"order cancelled", "order canceled",
	"cancellation confirmation", "cancellation confirmed",
	"your cancellation",
}

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

		// IMP-006-R14-002: When a cancellation email arrives for an existing
		// active subscription, update the status instead of creating a duplicate.
		if sub.Status == "cancelled" {
			updated, updateErr := e.Pool.Exec(ctx, `
				UPDATE subscriptions
				SET status = 'cancelled', cancelled_at = NOW()
				WHERE service_name = $1 AND status = 'active'
			`, sub.ServiceName)
			if updateErr != nil {
				slog.Warn("update subscription status failed", "service", sub.ServiceName, "error", updateErr)
			} else if updated.RowsAffected() > 0 {
				slog.Info("subscription cancelled", "service", sub.ServiceName)
				detected = append(detected, *sub)
				continue
			}
			// If no existing active subscription found, fall through to insert
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
			sort.Strings(services)
			summary.Overlaps = append(summary.Overlaps, SubscriptionOverlap{
				Category:     cat,
				Services:     services,
				CombinedCost: categoryCost[cat],
			})
		}
	}

	// Sort overlaps by category for deterministic API output.
	sort.Slice(summary.Overlaps, func(i, j int) bool {
		return summary.Overlaps[i].Category < summary.Overlaps[j].Category
	})

	return summary, rows.Err()
}

// parseSubscription extracts subscription data from an email artifact.
func parseSubscription(artifactID, title, content, sender string) *Subscription {
	serviceName := extractServiceName(sender, title)
	if serviceName == "" {
		return nil
	}

	combinedText := title + " " + content
	amount, currency := extractAmountWithCurrency(combinedText)
	freq := detectFrequency(combinedText)
	category := categorizeService(serviceName)
	status := "active"
	lowerText := strings.ToLower(combinedText)
	if containsAny(lowerText, cancellationEventPhrases) {
		status = "cancelled"
	} else if containsAny(lowerText, []string{"free trial", "trial ends", "trial exp"}) {
		status = "trial"
	}

	return &Subscription{
		ID:           ulid.Make().String(),
		ServiceName:  serviceName,
		Amount:       amount,
		Currency:     currency,
		BillingFreq:  freq,
		Category:     category,
		Status:       status,
		DetectedFrom: artifactID,
		FirstSeen:    time.Now(),
		CreatedAt:    time.Now(),
	}
}

// senderPrefixes is the list of common automated-email subdomain prefixes
// that should be stripped to reveal the actual service domain.
// IMP-006-SQS-001: Ordered longest-first so compound prefixes like
// "no-reply.payments." match before shorter "no-reply." prefix.
var senderPrefixes = []string{
	"no-reply.payments.", "noreply.", "no-reply.", "billing.", "payments.",
	"support.", "info.", "notifications.", "accounts.",
	"team.", "help.",
}

// stripSenderPrefix removes known automated-email subdomain prefixes from a domain.
func stripSenderPrefix(domain string) string {
	for _, prefix := range senderPrefixes {
		if strings.HasPrefix(domain, prefix) {
			return domain[len(prefix):]
		}
	}
	return domain
}

func extractServiceName(sender, title string) string {
	// Try to get service name from sender domain
	if at := strings.LastIndex(sender, "@"); at >= 0 {
		domain := sender[at+1:]
		// Strip common email prefixes
		domain = stripSenderPrefix(domain)
		// Use domain without TLD as service name
		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			return cases.Title(language.English).String(parts[len(parts)-2])
		}
	}
	return ""
}

// extractAmountWithCurrency extracts the amount and currency from text.
// IMP-006-R14-001: Supports USD ($), EUR (€), and GBP (£) detection.
// Returns (amount, currency) where currency defaults to "USD" if none detected.
func extractAmountWithCurrency(text string) (float64, string) {
	// Limit input to first 2000 chars to bound regex cost on large emails.
	// Use TruncateUTF8 to avoid splitting multi-byte runes (CWE-135).
	const maxExtractLen = 2000
	if len(text) > maxExtractLen {
		text = stringutil.TruncateUTF8(text, maxExtractLen)
	}
	// Strip commas from amounts like "$1,299.99" before matching.
	normalized := strings.ReplaceAll(text, ",", "")

	// Try EUR first (€ symbol is distinctive), then GBP, then USD.
	if amount := extractAmountFromPattern(normalized, amountPatternEUR); amount > 0 {
		return amount, "EUR"
	}
	if amount := extractAmountFromPattern(normalized, amountPatternGBP); amount > 0 {
		return amount, "GBP"
	}
	if amount := extractAmountFromPattern(normalized, amountPatternUSD); amount > 0 {
		return amount, "USD"
	}
	return 0, "USD" // default to USD if no amount found
}

// extractAmountFromPattern extracts the first matching amount from the pattern.
func extractAmountFromPattern(text string, pattern *regexp.Regexp) float64 {
	matches := pattern.FindStringSubmatch(text)
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

// extractAmount extracts the amount from text (USD assumed for backward compatibility).
func extractAmount(text string) float64 {
	amount, _ := extractAmountWithCurrency(text)
	return amount
}

func detectFrequency(text string) string {
	// Limit input to first 2000 chars to bound scan cost on large emails.
	// Use TruncateUTF8 to avoid splitting multi-byte runes (CWE-135).
	const maxScanLen = 2000
	if len(text) > maxScanLen {
		text = stringutil.TruncateUTF8(text, maxScanLen)
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
