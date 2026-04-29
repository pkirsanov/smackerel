package digest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Freshness windows for weather artifacts consumed by the digest.
// Current conditions are considered stale after 6h; forecasts after 24h.
// Mirrors the connector's typical sync cadence.
const (
	weatherCurrentTTL       = 6 * time.Hour
	weatherForecastTTL      = 24 * time.Hour
	weatherForecastMaxItems = 3
)

// WeatherDigestContext holds weather data assembled for the daily digest.
// It is populated only when a home location is configured AND fresh
// weather/current or weather/forecast artifacts exist within the TTL window.
type WeatherDigestContext struct {
	Location string                 `json:"location,omitempty"`
	Current  *WeatherCurrentSummary `json:"current,omitempty"`
	Forecast []WeatherForecastDay   `json:"forecast,omitempty"`
}

// WeatherCurrentSummary mirrors the persisted weather/current artifact.
// content_raw / summary capture the human-readable conditions; metadata is
// not persisted by the pipeline so structured numeric fields are not
// available here.
type WeatherCurrentSummary struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Summary     string    `json:"summary,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
}

// WeatherForecastDay mirrors a single weather/forecast artifact row.
type WeatherForecastDay struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CapturedAt  time.Time `json:"captured_at"`
}

// IsEmpty returns true when the context has nothing worth rendering.
func (w *WeatherDigestContext) IsEmpty() bool {
	if w == nil {
		return true
	}
	return w.Current == nil && len(w.Forecast) == 0
}

// AssembleWeatherContext queries fresh weather artifacts for the configured
// home location. It mirrors the existing DigestContext sub-context pattern
// (direct DB query against g.Pool) used by getOvernightArtifacts /
// AssembleHospitalityContext.
//
// Returns nil when:
//   - homeLocation is empty (no configured home location)
//   - pool is nil
//   - no fresh weather/current and no fresh weather/forecast artifacts exist
//   - the query fails (a slog.Warn with key "weather" is emitted in that case)
//
// The function never returns an error: the digest must render gracefully
// without a weather section when weather data is unavailable.
func AssembleWeatherContext(ctx context.Context, pool *pgxpool.Pool, homeLocation string, now time.Time) *WeatherDigestContext {
	homeLocation = strings.TrimSpace(homeLocation)
	if homeLocation == "" {
		return nil
	}
	if pool == nil {
		return nil
	}

	w := &WeatherDigestContext{Location: homeLocation}

	currentThreshold := now.Add(-weatherCurrentTTL)
	forecastThreshold := now.Add(-weatherForecastTTL)

	// Most recent weather/current within the TTL window whose title
	// references the home location.
	var (
		cTitle, cRaw, cSummary string
		cAt                    time.Time
	)
	row := pool.QueryRow(ctx, `
		SELECT title, COALESCE(content_raw, ''), COALESCE(summary, ''), created_at
		FROM artifacts
		WHERE artifact_type = 'weather/current'
		  AND source_id = 'weather'
		  AND title ILIKE '%' || $1 || '%'
		  AND created_at > $2
		ORDER BY created_at DESC
		LIMIT 1
	`, homeLocation, currentThreshold)
	if err := row.Scan(&cTitle, &cRaw, &cSummary, &cAt); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("failed to assemble weather digest context",
				"weather", "current",
				"error", err,
			)
		}
	} else {
		w.Current = &WeatherCurrentSummary{
			Title:       cTitle,
			Description: cRaw,
			Summary:     cSummary,
			CapturedAt:  cAt,
		}
	}

	// Up to N most recent weather/forecast artifacts within the TTL window.
	rows, err := pool.Query(ctx, `
		SELECT title, COALESCE(content_raw, ''), created_at
		FROM artifacts
		WHERE artifact_type = 'weather/forecast'
		  AND source_id = 'weather'
		  AND title ILIKE '%' || $1 || '%'
		  AND created_at > $2
		ORDER BY created_at DESC
		LIMIT $3
	`, homeLocation, forecastThreshold, weatherForecastMaxItems)
	if err != nil {
		slog.Warn("failed to assemble weather digest context",
			"weather", "forecast",
			"error", err,
		)
	} else {
		defer rows.Close()
		for rows.Next() {
			var f WeatherForecastDay
			if scanErr := rows.Scan(&f.Title, &f.Description, &f.CapturedAt); scanErr != nil {
				slog.Warn("failed to assemble weather digest context",
					"weather", "forecast",
					"error", scanErr,
				)
				continue
			}
			w.Forecast = append(w.Forecast, f)
		}
		if rerr := rows.Err(); rerr != nil {
			slog.Warn("failed to assemble weather digest context",
				"weather", "forecast",
				"error", rerr,
			)
		}
	}

	if w.IsEmpty() {
		return nil
	}
	return w
}

// formatWeatherFallback produces the plain-text weather section for the
// fallback digest used when the ML sidecar is unreachable. Mirrors the
// formatHospitalityFallback / formatKnowledgeHealthFallback pattern.
func formatWeatherFallback(w *WeatherDigestContext) string {
	if w == nil || w.IsEmpty() {
		return ""
	}
	var lines []string
	if w.Location != "" {
		lines = append(lines, fmt.Sprintf("🌤️ Weather (%s)", w.Location))
	} else {
		lines = append(lines, "🌤️ Weather")
	}
	if w.Current != nil {
		text := w.Current.Summary
		if text == "" {
			text = w.Current.Description
		}
		if text == "" {
			text = w.Current.Title
		}
		text = firstLine(text)
		if text != "" {
			lines = append(lines, fmt.Sprintf("  • %s", text))
		}
	}
	if len(w.Forecast) > 0 {
		lines = append(lines, "  Forecast:")
		for _, f := range w.Forecast {
			text := f.Description
			if text == "" {
				text = f.Title
			}
			text = firstLine(text)
			if text != "" {
				lines = append(lines, fmt.Sprintf("    - %s", text))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// firstLine returns the first non-empty trimmed line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			return t
		}
	}
	return ""
}
