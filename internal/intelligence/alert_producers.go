package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
)

// ProduceBillAlerts surfaces subscriptions whose upcoming charge the LLM judges
// worth a reminder NOW (BUG-021-006). The Go core estimates the next billing
// date and retrieves candidates; the alert_timing_evaluate scenario decides
// whether now is a good time given the charge's size/cadence. There is NO
// hardcoded alert-timing window — when the evaluator is not wired, production
// is skipped.
func (e *Engine) ProduceBillAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("bill alert production requires a database connection")
	}
	if e.alertTiming == nil || e.alertTiming.Evaluator == nil {
		slog.Warn("bill alert production skipped: LLM alert-timing evaluator not wired (no hardcoded fallback)")
		return nil
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
		      AND created_at > NOW() - make_interval(days => $1)
		  )
		LIMIT $2
	`, e.alertTiming.LookaheadDays, e.alertTiming.MaxCandidates)
	if err != nil {
		return fmt.Errorf("query subscriptions for billing: %w", err)
	}

	type billCand struct {
		cand   AlertTimingCandidate
		amount float64
	}
	var bills []billCand
	for rows.Next() {
		var id, serviceName, currency, billingFreq string
		var amount float64
		var firstSeen time.Time
		if err := rows.Scan(&id, &serviceName, &amount, &currency, &billingFreq, &firstSeen); err != nil {
			slog.Warn("bill alert scan failed", "error", err)
			continue
		}

		// Estimate next billing date using proper date arithmetic.
		now := time.Now()
		billingDay := firstSeen.Day()
		localToday := localMidnight(now)

		var nextBilling time.Time
		if billingFreq == "annual" {
			nextBilling = clampDay(now.Year(), firstSeen.Month(), billingDay)
			if nextBilling.Before(localToday) {
				nextBilling = clampDay(now.Year()+1, firstSeen.Month(), billingDay)
			}
		} else {
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
		if daysUntilBilling < 0 || daysUntilBilling > e.alertTiming.LookaheadDays {
			continue
		}

		detail := fmt.Sprintf("%s subscription", billingFreq)
		if amount > 0 {
			detail = fmt.Sprintf("%s %s %.2f subscription", billingFreq, currency, amount)
		}
		bills = append(bills, billCand{
			cand: AlertTimingCandidate{
				ArtifactID:     id,
				AlertType:      AlertBill,
				Priority:       2,
				AlertKind:      AlertKindBill,
				Subject:        serviceName,
				DaysUntilEvent: daysUntilBilling,
				Detail:         detail,
			},
			amount: amount,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("bill alert row iteration: %w", err)
	}
	rows.Close()

	var created int
	for _, b := range bills {
		if ctx.Err() != nil {
			slog.Warn("bill alert production context cancelled, remaining candidates skipped", "created_so_far", created)
			break
		}
		title := fmt.Sprintf("Upcoming charge: %s", b.cand.Subject)
		if b.amount > 0 {
			title = fmt.Sprintf("Upcoming charge: %s (%.2f)", b.cand.Subject, b.amount)
		}
		fallback := fmt.Sprintf("%s charges in %d days.", b.cand.Subject, b.cand.DaysUntilEvent)
		if e.evaluateAndCreateTimedAlert(ctx, b.cand, title, fallback) {
			created++
		}
	}

	slog.Info("bill alert production complete", "candidates", len(bills), "created", created)
	return nil
}

// ProduceTripPrepAlerts surfaces upcoming trips the LLM judges worth a prep
// reminder NOW (BUG-021-006). The Go core retrieves trips within the
// operational lookahead horizon; the alert_timing_evaluate scenario decides
// whether now is a good time given the trip's nature.
func (e *Engine) ProduceTripPrepAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("trip prep alert production requires a database connection")
	}
	if e.alertTiming == nil || e.alertTiming.Evaluator == nil {
		slog.Warn("trip prep alert production skipped: LLM alert-timing evaluator not wired (no hardcoded fallback)")
		return nil
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, destination, start_date
		FROM trips
		WHERE status = 'upcoming'
		  AND start_date BETWEEN CURRENT_DATE AND CURRENT_DATE + make_interval(days => $1)
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'trip_prep'
		      AND artifact_id = trips.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT $2
	`, e.alertTiming.LookaheadDays, e.alertTiming.MaxCandidates)
	if err != nil {
		return fmt.Errorf("query upcoming trips: %w", err)
	}

	var trips []AlertTimingCandidate
	for rows.Next() {
		var id, destination string
		var startDate time.Time
		if err := rows.Scan(&id, &destination, &startDate); err != nil {
			slog.Warn("trip alert scan failed", "error", err)
			continue
		}
		now := time.Now()
		localToday := localMidnight(now)
		daysUntil := calendarDaysBetween(localToday, startDate)
		if daysUntil < 0 {
			daysUntil = 0
		}
		trips = append(trips, AlertTimingCandidate{
			ArtifactID:     id,
			AlertType:      AlertTripPrep,
			Priority:       2,
			AlertKind:      AlertKindTripPrep,
			Subject:        destination,
			DaysUntilEvent: daysUntil,
			Detail:         fmt.Sprintf("trip to %s", destination),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("trip alert row iteration: %w", err)
	}
	rows.Close()

	var created int
	for _, c := range trips {
		if ctx.Err() != nil {
			slog.Warn("trip prep alert production context cancelled, remaining candidates skipped", "created_so_far", created)
			break
		}
		title := fmt.Sprintf("Trip prep: %s in %d days", c.Subject, c.DaysUntilEvent)
		fallback := fmt.Sprintf("Your trip to %s departs in %d days. Check bookings and packing.", c.Subject, c.DaysUntilEvent)
		if e.evaluateAndCreateTimedAlert(ctx, c, title, fallback) {
			created++
		}
	}

	slog.Info("trip prep alert production complete", "candidates", len(trips), "created", created)
	return nil
}

// ProduceReturnWindowAlerts surfaces artifacts with an approaching return
// deadline that the LLM judges worth a reminder NOW (BUG-021-006). The Go core
// retrieves deadlines within the operational lookahead horizon; the
// alert_timing_evaluate scenario decides whether now is a good time.
func (e *Engine) ProduceReturnWindowAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("return window alert production requires a database connection")
	}
	if e.alertTiming == nil || e.alertTiming.Evaluator == nil {
		slog.Warn("return window alert production skipped: LLM alert-timing evaluator not wired (no hardcoded fallback)")
		return nil
	}

	// Use a safe date cast so a single artifact with a malformed return_deadline
	// doesn't cause the entire query to fail. The regex validates month (01-12)
	// and day (01-31) ranges before the ::date cast.
	rows, err := e.Pool.Query(ctx, `
		SELECT id, title, metadata->>'return_deadline' AS return_deadline
		FROM artifacts
		WHERE metadata->>'return_deadline' IS NOT NULL
		  AND metadata->>'return_deadline' ~ '^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$'
		  AND (metadata->>'return_deadline')::date BETWEEN CURRENT_DATE AND CURRENT_DATE + make_interval(days => $1)
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'return_window'
		      AND artifact_id = artifacts.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT $2
	`, e.alertTiming.LookaheadDays, e.alertTiming.MaxCandidates)
	if err != nil {
		return fmt.Errorf("query return windows: %w", err)
	}

	type returnCand struct {
		cand        AlertTimingCandidate
		deadlineStr string
	}
	var returns []returnCand
	for rows.Next() {
		var id, title, deadlineStr string
		if err := rows.Scan(&id, &title, &deadlineStr); err != nil {
			slog.Warn("return window scan failed", "error", err)
			continue
		}
		daysUntil := 0
		if d, perr := time.Parse("2006-01-02", deadlineStr); perr == nil {
			now := time.Now()
			localToday := localMidnight(now)
			deadlineLocal := localMidnight(d)
			daysUntil = calendarDaysBetween(localToday, deadlineLocal)
			if daysUntil < 0 {
				daysUntil = 0
			}
		}
		returns = append(returns, returnCand{
			cand: AlertTimingCandidate{
				ArtifactID:     id,
				AlertType:      AlertReturnWindow,
				Priority:       1,
				AlertKind:      AlertKindReturnWindow,
				Subject:        title,
				DaysUntilEvent: daysUntil,
				Detail:         fmt.Sprintf("return deadline for %q", title),
			},
			deadlineStr: deadlineStr,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("return window row iteration: %w", err)
	}
	rows.Close()

	var created int
	for _, r := range returns {
		if ctx.Err() != nil {
			slog.Warn("return window alert production context cancelled, remaining candidates skipped", "created_so_far", created)
			break
		}
		title := fmt.Sprintf("Return window closing: %s", r.cand.Subject)
		fallback := fmt.Sprintf("Return deadline for %q is %s. Act soon.", r.cand.Subject, r.deadlineStr)
		if e.evaluateAndCreateTimedAlert(ctx, r.cand, title, fallback) {
			created++
		}
	}

	slog.Info("return window alert production complete", "candidates", len(returns), "created", created)
	return nil
}

// candidateCoolingQuery retrieves cooling CANDIDATES and their interaction
// signals. This is pure data retrieval — it applies NO business threshold for
// "cooling" (that judgment is the LLM evaluator's). The only numeric inputs are
// OPERATIONAL bounds: $1 = dedup window days (don't re-alert a person whose
// cooling nudge is still pending/delivered within the window) and $2 = the
// per-run candidate cap (throughput). People are ordered most-dormant-first so
// the cap keeps the highest-signal candidates. typical_gap_days is the person's
// average cadence (span / (interactions-1)) — pure arithmetic, no threshold —
// so the LLM can compare the current silence against THIS person's own rhythm.
const candidateCoolingQuery = `
	SELECT p.id, p.name,
	       EXTRACT(DAY FROM NOW() - MAX(a.created_at))::int AS days_since_last,
	       COUNT(DISTINCT a.id)::int AS total_interactions,
	       EXTRACT(DAY FROM MAX(a.created_at) - MIN(a.created_at))::int AS span_days
	FROM people p
	JOIN edges e ON e.dst_id = p.id AND e.dst_type = 'person'
	JOIN artifacts a ON a.id = e.src_id
	WHERE NOT EXISTS (
	  SELECT 1 FROM alerts
	  WHERE alert_type = 'relationship_cooling'
	    AND artifact_id = p.id
	    AND status IN ('pending', 'delivered')
	    AND created_at > NOW() - make_interval(days => $1)
	)
	GROUP BY p.id, p.name
	HAVING COUNT(DISTINCT a.id) >= 1
	ORDER BY MAX(a.created_at) ASC
	LIMIT $2
`

// ProduceRelationshipCoolingAlerts surfaces contacts whose relationship the
// LLM judges to be cooling (BUG-021-005). The Go core retrieves candidates and
// their interaction signals; the `relationship_cooling_evaluate` scenario
// decides, per candidate, whether the relationship is genuinely cooling given
// that person's own historical cadence. There is NO hardcoded threshold for
// "cooling" — when the evaluator is not wired, cooling production is skipped
// (the framework does not fall back to magic numbers).
func (e *Engine) ProduceRelationshipCoolingAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("relationship cooling alert production requires a database connection")
	}
	if e.cooling == nil || e.cooling.Evaluator == nil {
		slog.Warn("relationship cooling alert production skipped: LLM evaluator not wired (no hardcoded fallback)")
		return nil
	}

	rows, err := e.Pool.Query(ctx, candidateCoolingQuery, e.cooling.DedupWindowDays, e.cooling.MaxCandidates)
	if err != nil {
		return fmt.Errorf("query cooling candidates: %w", err)
	}

	// Collect candidates first so the evaluator's LLM calls do not run while a
	// DB row cursor is held open.
	var candidates []CoolingCandidate
	for rows.Next() {
		var c CoolingCandidate
		var spanDays int
		if err := rows.Scan(&c.PersonID, &c.Name, &c.DaysSinceLastInteraction, &c.TotalInteractions, &spanDays); err != nil {
			slog.Warn("relationship cooling candidate scan failed", "error", err)
			continue
		}
		c.RelationshipSpanDays = spanDays
		c.TypicalGapDays = coolingTypicalGapDays(spanDays, c.TotalInteractions)
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("relationship cooling candidate iteration: %w", err)
	}
	rows.Close()

	var created int
	for _, c := range candidates {
		if ctx.Err() != nil {
			slog.Warn("relationship cooling alert production context cancelled, remaining candidates skipped", "created_so_far", created)
			break
		}

		decision, err := e.cooling.Evaluator.EvaluateCooling(ctx, c)
		if err != nil {
			slog.Warn("relationship cooling evaluation failed", "person", c.Name, "error", err)
			metrics.AlertProducerFailures.WithLabelValues(string(AlertRelationship)).Inc()
			continue
		}
		// The LLM judges cooling; the confidence floor is an OPERATIONAL safety
		// gate (withhold the nudge when the model is not confident — Product
		// Principle 6, invisible by default).
		if !coolingShouldSurface(decision, e.cooling.ConfidenceFloor) {
			continue
		}

		body := decision.Rationale
		if body == "" {
			body = fmt.Sprintf("It's been %d days since your last interaction with %s.", c.DaysSinceLastInteraction, c.Name)
		}
		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertRelationship,
			Title:      fmt.Sprintf("Reconnect with %s? Last contact %d days ago", c.Name, c.DaysSinceLastInteraction),
			Body:       body,
			Priority:   3,
			ArtifactID: c.PersonID,
		}); err != nil {
			slog.Warn("failed to create relationship cooling alert", "person", c.Name, "error", err)
			metrics.AlertProducerFailures.WithLabelValues(string(AlertRelationship)).Inc()
		} else {
			metrics.AlertsProduced.WithLabelValues(string(AlertRelationship)).Inc()
			created++
		}
	}

	slog.Info("relationship cooling alert production complete", "candidates", len(candidates), "created", created)
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

// localMidnight returns midnight (local time) of the calendar day containing t.
// The alert producers compare and difference calendar days in the user's local
// timezone, so this truncates the time-of-day while preserving the local date.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}
