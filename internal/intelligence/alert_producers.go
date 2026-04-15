package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ProduceBillAlerts creates alerts for subscriptions with upcoming billing dates.
func (e *Engine) ProduceBillAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("bill alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, service_name, amount, currency, billing_freq, first_seen
		FROM subscriptions
		WHERE status = 'active'
		  AND billing_freq IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'bill'
		      AND artifact_id = subscriptions.id
		      AND status IN ('pending', 'delivered')
		      AND created_at > NOW() - INTERVAL '30 days'
		  )
		LIMIT 20
	`)
	if err != nil {
		return fmt.Errorf("query subscriptions for billing: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("bill alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, serviceName, currency, billingFreq string
		var amount float64
		var firstSeen time.Time
		if err := rows.Scan(&id, &serviceName, &amount, &currency, &billingFreq, &firstSeen); err != nil {
			slog.Warn("bill alert scan failed", "error", err)
			continue
		}

		// Estimate next billing date using proper date arithmetic.
		// For monthly: same day-of-month in the current month (clamped to month length).
		// For annual: same month and day as first_seen.
		now := time.Now()
		billingDay := firstSeen.Day()
		// Use local midnight (not now.Truncate which aligns to UTC boundaries)
		// to ensure consistent comparison with clampDay's time.Local dates.
		localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

		var nextBilling time.Time
		if billingFreq == "annual" {
			// Try this year first, then next year
			nextBilling = clampDay(now.Year(), firstSeen.Month(), billingDay)
			if nextBilling.Before(localToday) {
				nextBilling = clampDay(now.Year()+1, firstSeen.Month(), billingDay)
			}
		} else {
			// Monthly: try current month, then next month
			nextBilling = clampDay(now.Year(), now.Month(), billingDay)
			if nextBilling.Before(localToday) {
				nextMonth := now.Month() + 1
				nextYear := now.Year()
				if nextMonth > 12 {
					nextMonth = 1
					nextYear++
				}
				nextBilling = clampDay(nextYear, nextMonth, billingDay)
			}
		}

		daysUntilBilling := calendarDaysBetween(localToday, nextBilling)
		if daysUntilBilling < 0 || daysUntilBilling > 3 {
			continue
		}

		title := fmt.Sprintf("Upcoming charge: %s", serviceName)
		if amount > 0 {
			title = fmt.Sprintf("Upcoming charge: %s (%.2f %s)", serviceName, amount, currency)
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertBill,
			Title:      title,
			Body:       fmt.Sprintf("%s billing expected in ~%d days", serviceName, daysUntilBilling),
			Priority:   2,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create bill alert", "subscription", serviceName, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("bill alert row iteration: %w", err)
	}

	slog.Info("bill alert production complete", "created", created)
	return nil
}

// ProduceTripPrepAlerts creates alerts for upcoming trips with departure within 5 days.
func (e *Engine) ProduceTripPrepAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("trip prep alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, destination, start_date
		FROM trips
		WHERE status = 'upcoming'
		  AND start_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'trip_prep'
		      AND artifact_id = trips.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query upcoming trips: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("trip prep alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, destination string
		var startDate time.Time
		if err := rows.Scan(&id, &destination, &startDate); err != nil {
			slog.Warn("trip alert scan failed", "error", err)
			continue
		}

		// Use calendarDaysBetween for DST-safe, time-of-day-independent day counting
		// consistent with ProduceBillAlerts and CheckOverdueCommitments.
		now := time.Now()
		localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		daysUntil := calendarDaysBetween(localToday, startDate)
		if daysUntil < 0 {
			daysUntil = 0
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertTripPrep,
			Title:      fmt.Sprintf("Trip prep: %s in %d days", destination, daysUntil),
			Body:       fmt.Sprintf("Your trip to %s departs in %d days. Check bookings and packing.", destination, daysUntil),
			Priority:   2,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create trip prep alert", "trip", destination, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("trip alert row iteration: %w", err)
	}

	slog.Info("trip prep alert production complete", "created", created)
	return nil
}

// ProduceReturnWindowAlerts creates alerts for artifacts with return deadlines expiring within 5 days.
func (e *Engine) ProduceReturnWindowAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("return window alert production requires a database connection")
	}

	// Use a safe date cast so a single artifact with a malformed return_deadline
	// doesn't cause the entire query to fail (killing all return window detection).
	// The regex validates month (01-12) and day (01-31) ranges to prevent
	// dates like "2026-13-45" from reaching the ::date cast and crashing the query.
	rows, err := e.Pool.Query(ctx, `
		SELECT id, title, metadata->>'return_deadline' AS return_deadline
		FROM artifacts
		WHERE metadata->>'return_deadline' IS NOT NULL
		  AND metadata->>'return_deadline' ~ '^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$'
		  AND (metadata->>'return_deadline')::date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'return_window'
		      AND artifact_id = artifacts.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query return windows: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("return window alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, title, deadlineStr string
		if err := rows.Scan(&id, &title, &deadlineStr); err != nil {
			slog.Warn("return window scan failed", "error", err)
			continue
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertReturnWindow,
			Title:      fmt.Sprintf("Return window closing: %s", title),
			Body:       fmt.Sprintf("Return deadline for \"%s\" is %s. Act soon.", title, deadlineStr),
			Priority:   1,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create return window alert", "artifact", id, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("return window row iteration: %w", err)
	}

	slog.Info("return window alert production complete", "created", created)
	return nil
}

// ProduceRelationshipCoolingAlerts creates alerts for contacts with fading communication.
func (e *Engine) ProduceRelationshipCoolingAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("relationship cooling alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT p.id, p.name,
		       EXTRACT(DAY FROM NOW() - MAX(a.created_at))::int AS days_since
		FROM people p
		JOIN edges e ON e.dst_id = p.id AND e.dst_type = 'person'
		JOIN artifacts a ON a.id = e.src_id
		GROUP BY p.id, p.name
		HAVING EXTRACT(DAY FROM NOW() - MAX(a.created_at)) > 30
		   AND COUNT(DISTINCT a.id) FILTER (WHERE a.created_at BETWEEN NOW() - INTERVAL '180 days' AND NOW() - INTERVAL '90 days') >= 4
		   AND NOT EXISTS (
		     SELECT 1 FROM alerts
		     WHERE alert_type = 'relationship_cooling'
		       AND artifact_id = p.id
		       AND status IN ('pending', 'delivered')
		       AND created_at > NOW() - INTERVAL '30 days'
		   )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query cooling relationships: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("relationship cooling alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, name string
		var daysSince int
		if err := rows.Scan(&id, &name, &daysSince); err != nil {
			slog.Warn("relationship cooling scan failed", "error", err)
			continue
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertRelationship,
			Title:      fmt.Sprintf("Reconnect with %s? Last contact %d days ago", name, daysSince),
			Body:       fmt.Sprintf("You used to communicate regularly with %s, but it's been %d days since your last interaction.", name, daysSince),
			Priority:   3,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create relationship cooling alert", "person", name, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("relationship cooling row iteration: %w", err)
	}

	slog.Info("relationship cooling alert production complete", "created", created)
	return nil
}

// clampDay returns the date for (year, month, day) clamped to the last day
// of the given month. E.g. clampDay(2026, time.February, 31) → Feb 28.
func clampDay(year int, month time.Month, day int) time.Time {
	// time.Date normalises overflow: Feb 31 → Mar 3, so instead compute
	// the last day of the month and clamp.
	if day < 1 {
		day = 1
	}
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

// calendarDaysBetween counts whole calendar days from 'from' to 'to',
// ignoring time-of-day. Uses UTC normalisation so DST transitions
// (23-hour or 25-hour days) don't cause off-by-one errors.
func calendarDaysBetween(from, to time.Time) int {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC).Hours() / 24)
}
