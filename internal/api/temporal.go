package api

import (
	"strings"
	"time"
)

// temporalFilter represents a parsed temporal expression from a search query.
type temporalFilter struct {
	DateFrom string // RFC3339 date string
	DateTo   string // RFC3339 date string
	Cleaned  string // query with temporal expression removed
}

// parseTemporalIntent extracts temporal expressions from a search query
// and converts them to date range filters.
// Examples: "pricing video from last week" → DateFrom=7 days ago
//
//	"notes yesterday" → DateFrom=yesterday, DateTo=today
//	"articles this month" → DateFrom=first of month
func parseTemporalIntent(query string) *temporalFilter {
	now := time.Now()
	lower := strings.ToLower(query)

	// Try each pattern — most specific first
	patterns := []struct {
		phrases  []string
		dateFrom func() time.Time
		dateTo   func() time.Time
	}{
		{
			phrases:  []string{"yesterday"},
			dateFrom: func() time.Time { return now.AddDate(0, 0, -1).Truncate(24 * time.Hour) },
			dateTo:   func() time.Time { return now.Truncate(24 * time.Hour) },
		},
		{
			phrases:  []string{"today"},
			dateFrom: func() time.Time { return now.Truncate(24 * time.Hour) },
			dateTo:   func() time.Time { return now.Add(24 * time.Hour).Truncate(24 * time.Hour) },
		},
		{
			phrases:  []string{"last week", "past week", "this past week"},
			dateFrom: func() time.Time { return now.AddDate(0, 0, -7) },
			dateTo:   func() time.Time { return now },
		},
		{
			phrases:  []string{"last month", "past month"},
			dateFrom: func() time.Time { return now.AddDate(0, -1, 0) },
			dateTo:   func() time.Time { return now },
		},
		{
			phrases:  []string{"this month"},
			dateFrom: func() time.Time { return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()) },
			dateTo:   func() time.Time { return now },
		},
		{
			phrases:  []string{"this week"},
			dateFrom: func() time.Time { return now.AddDate(0, 0, -int(now.Weekday())) },
			dateTo:   func() time.Time { return now },
		},
		{
			phrases:  []string{"last few days", "past few days", "recent days", "recently"},
			dateFrom: func() time.Time { return now.AddDate(0, 0, -3) },
			dateTo:   func() time.Time { return now },
		},
		{
			phrases:  []string{"last year", "past year"},
			dateFrom: func() time.Time { return now.AddDate(-1, 0, 0) },
			dateTo:   func() time.Time { return now },
		},
	}

	for _, p := range patterns {
		for _, phrase := range p.phrases {
			// Check for phrase with optional "from " prefix
			variants := []string{
				"from " + phrase,
				phrase,
			}
			for _, variant := range variants {
				if idx := strings.Index(lower, variant); idx >= 0 {
					// Remove the temporal expression from the query
					cleaned := query[:idx] + query[idx+len(variant):]
					cleaned = strings.TrimSpace(cleaned)
					// Remove dangling prepositions
					cleaned = strings.TrimSuffix(cleaned, " from")
					cleaned = strings.TrimSuffix(cleaned, " since")
					cleaned = strings.TrimSuffix(cleaned, " in")
					cleaned = strings.TrimSpace(cleaned)

					return &temporalFilter{
						DateFrom: p.dateFrom().Format(time.RFC3339),
						DateTo:   p.dateTo().Format(time.RFC3339),
						Cleaned:  cleaned,
					}
				}
			}
		}
	}

	return nil
}
