package intelligence

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Freshness window and cap for weather/forecast artifacts consumed by the
// trip dossier assembler. Mirrors the digest-side TTL used in
// internal/digest/weather.go to keep one consistent forecast-consumption
// pattern across consumers (BUG-016-W1 / BUG-016-W2).
const (
	dossierForecastTTL      = 24 * time.Hour
	dossierForecastMaxItems = 3
)

// DossierForecast carries forecast data assembled for a single trip
// dossier. Populated only when a non-empty destination matches one or
// more fresh weather/forecast artifacts within the TTL window.
type DossierForecast struct {
	Destination string               `json:"destination,omitempty"`
	Days        []DossierForecastDay `json:"days,omitempty"`
	AssembledAt time.Time            `json:"assembled_at"`
}

// DossierForecastDay mirrors a single weather/forecast artifact row
// scoped to the dossier's destination.
type DossierForecastDay struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CapturedAt  time.Time `json:"captured_at"`
}

// IsEmpty returns true when the forecast has nothing worth rendering.
func (f *DossierForecast) IsEmpty() bool {
	if f == nil {
		return true
	}
	return len(f.Days) == 0
}

// assembleDestinationForecast queries fresh weather/forecast artifacts
// whose title references the supplied destination. Mirrors the existing
// internal/intelligence/ pattern of direct DB queries against e.Pool.
//
// Returns nil when:
//   - destination is empty (no usable lookup key)
//   - e.Pool is nil (no database available)
//   - no fresh weather/forecast artifacts match the destination
//   - the query fails (a slog.Warn with key "forecast" is emitted in
//     that case)
//
// The function never returns an error: trip dossiers MUST render
// gracefully when forecast data is unavailable.
func (e *Engine) assembleDestinationForecast(ctx context.Context, destination string, now time.Time) *DossierForecast {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return nil
	}
	if e == nil || e.Pool == nil {
		return nil
	}

	threshold := now.Add(-dossierForecastTTL)

	rows, err := e.Pool.Query(ctx, `
		SELECT title, COALESCE(content_raw, ''), created_at
		FROM artifacts
		WHERE artifact_type = 'weather/forecast'
		  AND source_id = 'weather'
		  AND title ILIKE '%' || $1 || '%'
		  AND created_at > $2
		ORDER BY created_at DESC
		LIMIT $3
	`, destination, threshold, dossierForecastMaxItems)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("failed to assemble dossier forecast",
				"forecast", "query",
				"destination", destination,
				"error", err,
			)
		}
		return nil
	}
	defer rows.Close()

	fc := &DossierForecast{
		Destination: destination,
		AssembledAt: now,
	}
	for rows.Next() {
		var d DossierForecastDay
		if scanErr := rows.Scan(&d.Title, &d.Description, &d.CapturedAt); scanErr != nil {
			slog.Warn("failed to assemble dossier forecast",
				"forecast", "scan",
				"destination", destination,
				"error", scanErr,
			)
			continue
		}
		fc.Days = append(fc.Days, d)
	}
	if rerr := rows.Err(); rerr != nil {
		slog.Warn("failed to assemble dossier forecast",
			"forecast", "rows",
			"destination", destination,
			"error", rerr,
		)
	}

	if fc.IsEmpty() {
		return nil
	}
	return fc
}

// formatDossierForecastLine produces the single-line forecast section
// rendered by assembleDossierText. Returns "" when the forecast is nil
// or empty so the caller can skip the section gracefully.
func formatDossierForecastLine(f *DossierForecast) string {
	if f.IsEmpty() {
		return ""
	}
	max := dossierForecastMaxItems
	if len(f.Days) < max {
		max = len(f.Days)
	}
	parts := make([]string, 0, max)
	for i := 0; i < max; i++ {
		text := firstNonEmptyLine(f.Days[i].Description)
		if text == "" {
			text = firstNonEmptyLine(f.Days[i].Title)
		}
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return ""
	}
	dest := f.Destination
	if dest == "" {
		return fmt.Sprintf("🌤️ Forecast: %s", strings.Join(parts, " / "))
	}
	return fmt.Sprintf("🌤️ Forecast: %s — %s", dest, strings.Join(parts, " / "))
}

// firstNonEmptyLine returns the first non-empty trimmed line of s.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			return t
		}
	}
	return ""
}
