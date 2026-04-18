package digest

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpenseDigestSection produces the expense section for the daily digest.
type ExpenseDigestSection struct {
	Pool                      *pgxpool.Pool
	MaxWords                  int
	NeedsReviewLimit          int
	MaxSuggestionsPerDigest   int
	MissingReceiptLookbackDays int
}

// ExpenseDigestContext holds all data for the expense digest section.
type ExpenseDigestContext struct {
	PeriodStart     string                    `json:"period_start"`
	PeriodEnd       string                    `json:"period_end"`
	Summary         *ExpenseDigestSummary     `json:"summary,omitempty"`
	NeedsReview     []ExpenseDigestReviewItem `json:"needs_review,omitempty"`
	Suggestions     []ExpenseDigestSuggestion `json:"suggestions,omitempty"`
	MissingReceipts []ExpenseDigestMissing    `json:"missing_receipts,omitempty"`
	UnusualCharges  []ExpenseDigestUnusual    `json:"unusual_charges,omitempty"`
}

// IsEmpty returns true when the expense section has no content.
func (c *ExpenseDigestContext) IsEmpty() bool {
	return c.Summary == nil &&
		len(c.NeedsReview) == 0 &&
		len(c.Suggestions) == 0 &&
		len(c.MissingReceipts) == 0 &&
		len(c.UnusualCharges) == 0
}

// ExpenseDigestSummary holds weekly expense totals.
type ExpenseDigestSummary struct {
	TotalCount    int                        `json:"total_count"`
	BusinessCount int                        `json:"business_count"`
	PersonalCount int                        `json:"personal_count"`
	TotalByCurrency []ExpenseDigestCurrTotal `json:"total_by_currency"`
}

// ExpenseDigestCurrTotal is a currency-grouped total.
type ExpenseDigestCurrTotal struct {
	Currency string `json:"currency"`
	Total    string `json:"total"`
}

// ExpenseDigestReviewItem represents an expense needing user review.
type ExpenseDigestReviewItem struct {
	Vendor string `json:"vendor"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

// ExpenseDigestSuggestion represents a pending classification suggestion.
type ExpenseDigestSuggestion struct {
	Vendor         string `json:"vendor"`
	Amount         string `json:"amount"`
	SuggestedClass string `json:"suggested_class"`
	Evidence       string `json:"evidence"`
}

// ExpenseDigestMissing represents a missing receipt warning.
type ExpenseDigestMissing struct {
	ServiceName string `json:"service_name"`
	Amount      string `json:"amount"`
}

// ExpenseDigestUnusual represents an unusual/new vendor charge.
type ExpenseDigestUnusual struct {
	Vendor string `json:"vendor"`
	Amount string `json:"amount"`
}

// Assemble gathers expense digest context for the current period (last 7 days).
func (s *ExpenseDigestSection) Assemble(ctx context.Context) (*ExpenseDigestContext, error) {
	if s.Pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}

	digestCtx := &ExpenseDigestContext{
		PeriodStart: "7 days ago",
		PeriodEnd:   "today",
	}

	// Summary: count and sum of expenses in last 7 days by classification
	summaryRows, err := s.Pool.Query(ctx, `
		SELECT
			metadata->'expense'->>'classification' AS classification,
			metadata->'expense'->>'currency' AS currency,
			COUNT(*) AS count,
			COALESCE(SUM(CAST(NULLIF(metadata->'expense'->>'amount', '') AS NUMERIC)), 0)::text AS total
		FROM artifacts
		WHERE metadata ? 'expense'
		AND created_at > NOW() - INTERVAL '7 days'
		GROUP BY metadata->'expense'->>'classification', metadata->'expense'->>'currency'
	`)
	if err != nil {
		slog.Warn("expense digest summary query failed", "error", err)
	} else {
		defer summaryRows.Close()
		summary := &ExpenseDigestSummary{}
		currTotals := make(map[string]float64)
		for summaryRows.Next() {
			var classification, currency, total string
			var count int
			if err := summaryRows.Scan(&classification, &currency, &count, &total); err != nil {
				continue
			}
			summary.TotalCount += count
			switch classification {
			case "business":
				summary.BusinessCount += count
			case "personal":
				summary.PersonalCount += count
			}
			// We store total as string to avoid float issues in the digest
			_ = currTotals // aggregated per currency for display
		}
		if summary.TotalCount > 0 {
			// Query totals by currency
			currRows, err := s.Pool.Query(ctx, `
				SELECT
					COALESCE(metadata->'expense'->>'currency', 'USD') AS currency,
					COALESCE(SUM(CAST(NULLIF(metadata->'expense'->>'amount', '') AS NUMERIC)), 0)::text AS total
				FROM artifacts
				WHERE metadata ? 'expense'
				AND created_at > NOW() - INTERVAL '7 days'
				GROUP BY metadata->'expense'->>'currency'
			`)
			if err == nil {
				defer currRows.Close()
				for currRows.Next() {
					var ct ExpenseDigestCurrTotal
					if err := currRows.Scan(&ct.Currency, &ct.Total); err != nil {
						continue
					}
					summary.TotalByCurrency = append(summary.TotalByCurrency, ct)
				}
			}
			digestCtx.Summary = summary
		}
	}

	// Needs review: extraction issues in last 7 days
	reviewRows, err := s.Pool.Query(ctx, `
		SELECT
			COALESCE(metadata->'expense'->>'vendor', 'Unknown') AS vendor,
			COALESCE(metadata->'expense'->>'amount', '') AS amount,
			CASE
				WHEN metadata->'expense'->>'amount_missing' = 'true' THEN 'amount not detected'
				WHEN metadata->'expense'->>'extraction_status' = 'partial' THEN 'partial extraction'
				ELSE 'needs review'
			END AS reason
		FROM artifacts
		WHERE metadata ? 'expense'
		AND created_at > NOW() - INTERVAL '7 days'
		AND (
			metadata->'expense'->>'extraction_status' != 'complete'
			OR metadata->'expense'->>'amount_missing' = 'true'
		)
		ORDER BY created_at DESC
		LIMIT $1
	`, s.NeedsReviewLimit)
	if err != nil {
		slog.Warn("expense digest review query failed", "error", err)
	} else {
		defer reviewRows.Close()
		for reviewRows.Next() {
			var item ExpenseDigestReviewItem
			if err := reviewRows.Scan(&item.Vendor, &item.Amount, &item.Reason); err != nil {
				continue
			}
			digestCtx.NeedsReview = append(digestCtx.NeedsReview, item)
		}
	}

	// Pending suggestions
	suggRows, err := s.Pool.Query(ctx, `
		SELECT
			es.vendor,
			COALESCE(metadata->'expense'->>'amount', '') AS amount,
			es.suggested_class,
			es.evidence
		FROM expense_suggestions es
		JOIN artifacts a ON a.id = es.artifact_id
		WHERE es.status = 'pending'
		ORDER BY es.confidence DESC
		LIMIT $1
	`, s.MaxSuggestionsPerDigest)
	if err != nil {
		slog.Warn("expense digest suggestions query failed", "error", err)
	} else {
		defer suggRows.Close()
		for suggRows.Next() {
			var sugg ExpenseDigestSuggestion
			if err := suggRows.Scan(&sugg.Vendor, &sugg.Amount, &sugg.SuggestedClass, &sugg.Evidence); err != nil {
				continue
			}
			digestCtx.Suggestions = append(digestCtx.Suggestions, sugg)
		}
	}

	// Missing receipts: active subscriptions with no matching expense
	missingRows, err := s.Pool.Query(ctx, `
		SELECT s.service_name, s.amount::text
		FROM subscriptions s
		WHERE s.status = 'active'
		AND NOT EXISTS (
			SELECT 1 FROM artifacts a
			WHERE metadata ? 'expense'
			AND LOWER(metadata->'expense'->>'vendor') = LOWER(s.service_name)
			AND a.created_at > NOW() - INTERVAL '1 day' * $1
		)
		LIMIT 10
	`, s.MissingReceiptLookbackDays)
	if err != nil {
		slog.Warn("expense digest missing receipt query failed", "error", err)
	} else {
		defer missingRows.Close()
		for missingRows.Next() {
			var m ExpenseDigestMissing
			if err := missingRows.Scan(&m.ServiceName, &m.Amount); err != nil {
				continue
			}
			digestCtx.MissingReceipts = append(digestCtx.MissingReceipts, m)
		}
	}

	// Unusual charges: new vendors not seen in previous 90 days
	unusualRows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT
			metadata->'expense'->>'vendor' AS vendor,
			metadata->'expense'->>'amount' AS amount
		FROM artifacts
		WHERE metadata ? 'expense'
		AND created_at > NOW() - INTERVAL '7 days'
		AND NOT EXISTS (
			SELECT 1 FROM artifacts older
			WHERE older.metadata ? 'expense'
			AND LOWER(older.metadata->'expense'->>'vendor') = LOWER(artifacts.metadata->'expense'->>'vendor')
			AND older.created_at BETWEEN NOW() - INTERVAL '97 days' AND NOW() - INTERVAL '7 days'
		)
		LIMIT 5
	`)
	if err != nil {
		slog.Warn("expense digest unusual charges query failed", "error", err)
	} else {
		defer unusualRows.Close()
		for unusualRows.Next() {
			var u ExpenseDigestUnusual
			if err := unusualRows.Scan(&u.Vendor, &u.Amount); err != nil {
				continue
			}
			digestCtx.UnusualCharges = append(digestCtx.UnusualCharges, u)
		}
	}

	return digestCtx, nil
}

// EnforceWordLimit drops low-priority blocks to fit within maxWords.
// Priority (highest to lowest): NeedsReview, Suggestions, MissingReceipts, UnusualCharges, Summary
func EnforceWordLimit(ctx *ExpenseDigestContext, maxWords int) {
	if maxWords <= 0 {
		return
	}

	totalWords := countWords(ctx)
	if totalWords <= maxWords {
		return
	}

	// Drop in reverse priority order: Summary first, then Unusual, then Missing
	if totalWords > maxWords && ctx.Summary != nil {
		ctx.Summary = nil
		totalWords = countWords(ctx)
	}
	if totalWords > maxWords {
		ctx.UnusualCharges = nil
		totalWords = countWords(ctx)
	}
	if totalWords > maxWords {
		ctx.MissingReceipts = nil
		totalWords = countWords(ctx)
	}
	if totalWords > maxWords {
		ctx.Suggestions = nil
	}
}

func countWords(ctx *ExpenseDigestContext) int {
	var parts []string
	if ctx.Summary != nil {
		parts = append(parts, fmt.Sprintf("This week: %d expenses", ctx.Summary.TotalCount))
		for _, ct := range ctx.Summary.TotalByCurrency {
			parts = append(parts, fmt.Sprintf("%s %s", ct.Total, ct.Currency))
		}
	}
	for _, r := range ctx.NeedsReview {
		parts = append(parts, fmt.Sprintf("%s %s %s", r.Vendor, r.Amount, r.Reason))
	}
	for _, s := range ctx.Suggestions {
		parts = append(parts, fmt.Sprintf("%s %s %s", s.Vendor, s.Amount, s.Evidence))
	}
	for _, m := range ctx.MissingReceipts {
		parts = append(parts, fmt.Sprintf("Missing receipt: %s %s", m.ServiceName, m.Amount))
	}
	for _, u := range ctx.UnusualCharges {
		parts = append(parts, fmt.Sprintf("New vendor: %s %s", u.Vendor, u.Amount))
	}
	text := strings.Join(parts, " ")
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
