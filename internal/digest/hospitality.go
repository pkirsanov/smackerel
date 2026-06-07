package digest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HospitalityDigestContext holds hospitality-specific digest data.
type HospitalityDigestContext struct {
	TodayArrivals   []GuestStay       `json:"todayArrivals"`
	TodayDepartures []GuestStay       `json:"todayDepartures"`
	PendingTasks    []HospitalityTask `json:"pendingTasks"`
	Revenue         RevenueSnapshot   `json:"revenue"`
	GuestAlerts     []GuestAlert      `json:"guestAlerts"`
	PropertyAlerts  []PropertyAlert   `json:"propertyAlerts"`
}

// GuestStay represents a guest arrival or departure for the digest.
type GuestStay struct {
	GuestName    string  `json:"guestName"`
	GuestEmail   string  `json:"guestEmail"`
	PropertyName string  `json:"propertyName"`
	CheckIn      string  `json:"checkIn"`
	CheckOut     string  `json:"checkOut"`
	Source       string  `json:"source"`
	TotalPrice   float64 `json:"totalPrice"`
}

// HospitalityTask represents a pending hospitality task.
type HospitalityTask struct {
	PropertyName string `json:"propertyName"`
	Title        string `json:"title"`
	Category     string `json:"category"`
	Status       string `json:"status"`
}

// RevenueSnapshot summarises check-in/out counts and revenue for the digest.
type RevenueSnapshot struct {
	TodayCheckIns  int                `json:"todayCheckIns"`
	TodayCheckOuts int                `json:"todayCheckOuts"`
	DayRevenue     float64            `json:"dayRevenue"`
	WeekRevenue    float64            `json:"weekRevenue"`
	MonthRevenue   float64            `json:"monthRevenue"`
	ByChannel      map[string]float64 `json:"byChannel,omitempty"`
	ByProperty     map[string]float64 `json:"byProperty,omitempty"`
}

// GuestAlert flags notable guest conditions.
type GuestAlert struct {
	GuestName   string `json:"guestName"`
	GuestEmail  string `json:"guestEmail"`
	AlertType   string `json:"alertType"`
	Description string `json:"description"`
}

// PropertyAlert flags notable property conditions.
type PropertyAlert struct {
	PropertyName string `json:"propertyName"`
	AlertType    string `json:"alertType"`
	Description  string `json:"description"`
}

// IsEmpty returns true when there is no hospitality data worth including.
func (h *HospitalityDigestContext) IsEmpty() bool {
	return len(h.TodayArrivals) == 0 &&
		len(h.TodayDepartures) == 0 &&
		len(h.PendingTasks) == 0 &&
		len(h.GuestAlerts) == 0 &&
		len(h.PropertyAlerts) == 0 &&
		h.Revenue.TodayCheckIns == 0 &&
		h.Revenue.TodayCheckOuts == 0
}

// AssembleHospitalityContext queries the database for hospitality-specific
// digest data. The pool must be connected to the Smackerel database that
// contains artifacts, guests, and properties tables.
//
// Guest/property concern alerts are LLM-judged (BUG-021-010): the eval gathers
// candidate signals within the operational bounds and the
// hospitality_concern_evaluate scenario decides which warrant a host alert.
// A nil eval ⇒ no concern alerts (there is NO hardcoded sentiment/rating/
// issue-count threshold fallback); the rest of the digest is unaffected.
func AssembleHospitalityContext(ctx context.Context, pool *pgxpool.Pool, eval HospitalityEvaluator, bounds HospitalityBounds) (*HospitalityDigestContext, error) {
	now := time.Now()
	today := now.Format("2006-01-02")

	hCtx := &HospitalityDigestContext{}

	arrivals, err := queryGuestStaysByDate(ctx, pool, "checkin_date", "arrivals", today)
	if err != nil {
		slog.Warn("hospitality digest: failed to query arrivals", "error", err)
	} else {
		hCtx.TodayArrivals = arrivals
	}

	departures, err := queryGuestStaysByDate(ctx, pool, "checkout_date", "departures", today)
	if err != nil {
		slog.Warn("hospitality digest: failed to query departures", "error", err)
	} else {
		hCtx.TodayDepartures = departures
	}

	tasks, err := queryPendingTasks(ctx, pool)
	if err != nil {
		slog.Warn("hospitality digest: failed to query pending tasks", "error", err)
	} else {
		hCtx.PendingTasks = tasks
	}

	revenue, err := queryRevenueSnapshot(ctx, pool, now)
	if err != nil {
		slog.Warn("hospitality digest: failed to query revenue", "error", err)
	} else {
		hCtx.Revenue = revenue
	}

	// LLM-judged guest/property concern alerts (BUG-021-010). No hardcoded
	// sentiment/rating/issue thresholds; a nil evaluator ⇒ no concern alerts.
	gAlerts, pAlerts := assembleConcernAlerts(ctx, pool, eval, bounds)
	hCtx.GuestAlerts = gAlerts
	hCtx.PropertyAlerts = pAlerts

	// Fill revenue check-in/out counts from arrivals/departures
	hCtx.Revenue.TodayCheckIns = len(hCtx.TodayArrivals)
	hCtx.Revenue.TodayCheckOuts = len(hCtx.TodayDepartures)

	return hCtx, nil
}

// assembleConcernAlerts gathers guest/property candidate signals and asks the
// LLM which warrant a host alert. Returns empty slices (no alerts) when the
// evaluator is not wired or the judgment is unavailable — never a hardcoded
// threshold fallback.
func assembleConcernAlerts(ctx context.Context, pool *pgxpool.Pool, eval HospitalityEvaluator, bounds HospitalityBounds) ([]GuestAlert, []PropertyAlert) {
	if eval == nil {
		slog.Warn("hospitality digest: concern alerts skipped — LLM evaluator not wired (no hardcoded threshold fallback)")
		return nil, nil
	}

	guests, err := gatherGuestSignals(ctx, pool, bounds.GuestCandidateLimit)
	if err != nil {
		slog.Warn("hospitality digest: failed to gather guest signals", "error", err)
	}
	properties, err := gatherPropertySignals(ctx, pool, bounds.PropertyCandidateLimit)
	if err != nil {
		slog.Warn("hospitality digest: failed to gather property signals", "error", err)
	}
	if len(guests) == 0 && len(properties) == 0 {
		return nil, nil
	}

	decision, err := eval.EvaluateConcerns(ctx, guests, properties)
	if err != nil {
		slog.Warn("hospitality digest: concern evaluation failed", "error", err)
		return nil, nil
	}

	var gAlerts []GuestAlert
	for _, j := range decision.GuestAlerts {
		if j.Ref < 0 || j.Ref >= len(guests) {
			slog.Warn("hospitality digest: guest alert ref out of range", "ref", j.Ref, "guests", len(guests))
			continue
		}
		g := guests[j.Ref]
		gAlerts = append(gAlerts, GuestAlert{
			GuestName:   g.Name,
			GuestEmail:  g.Email,
			AlertType:   j.AlertType,
			Description: j.Description,
		})
	}

	var pAlerts []PropertyAlert
	for _, j := range decision.PropertyAlerts {
		if j.Ref < 0 || j.Ref >= len(properties) {
			slog.Warn("hospitality digest: property alert ref out of range", "ref", j.Ref, "properties", len(properties))
			continue
		}
		p := properties[j.Ref]
		pAlerts = append(pAlerts, PropertyAlert{
			PropertyName: p.Name,
			AlertType:    j.AlertType,
			Description:  j.Description,
		})
	}
	return gAlerts, pAlerts
}

// queryGuestStaysByDate returns bookings matching a given date field (checkin_date or checkout_date).
// The dateField parameter is validated to prevent SQL injection.
func queryGuestStaysByDate(ctx context.Context, pool *pgxpool.Pool, dateField, label, today string) ([]GuestStay, error) {
	if dateField != "checkin_date" && dateField != "checkout_date" {
		return nil, fmt.Errorf("unsupported hospitality date field: %s", dateField)
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(a.metadata->>'guest_name', ''),
			COALESCE(a.metadata->>'guest_email', ''),
			COALESCE(a.metadata->>'property_name', ''),
			COALESCE(a.metadata->>'checkin_date', ''),
			COALESCE(a.metadata->>'checkout_date', ''),
			a.source_id,
			COALESCE((a.metadata->>'total_price')::numeric, 0)
		FROM artifacts a
		WHERE a.source_id = 'guesthost'
		  AND a.artifact_type = 'booking'
		  AND a.metadata->>'%s' = $1
		ORDER BY a.created_at
		LIMIT 50
	`, dateField), today)
	if err != nil {
		return nil, fmt.Errorf("query today %s: %w", label, err)
	}
	defer rows.Close()

	var stays []GuestStay
	for rows.Next() {
		var s GuestStay
		if err := rows.Scan(&s.GuestName, &s.GuestEmail, &s.PropertyName,
			&s.CheckIn, &s.CheckOut, &s.Source, &s.TotalPrice); err != nil {
			slog.Warn(label+" scan failed", "error", err)
			continue
		}
		stays = append(stays, s)
	}
	if err := rows.Err(); err != nil {
		return stays, err
	}
	return stays, nil
}

// queryPendingTasks returns incomplete hospitality tasks.
func queryPendingTasks(ctx context.Context, pool *pgxpool.Pool) ([]HospitalityTask, error) {
	rows, err := pool.Query(ctx, `
		SELECT
			COALESCE(a.metadata->>'property_name', ''),
			a.title,
			COALESCE(a.metadata->>'category', ''),
			COALESCE(a.metadata->>'status', 'pending')
		FROM artifacts a
		WHERE a.source_id = 'guesthost'
		  AND a.artifact_type = 'task'
		  AND COALESCE(a.metadata->>'status', 'pending') != 'completed'
		ORDER BY a.created_at
		LIMIT 20
	`)
	if err != nil {
		return nil, fmt.Errorf("query pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []HospitalityTask
	for rows.Next() {
		var t HospitalityTask
		if err := rows.Scan(&t.PropertyName, &t.Title, &t.Category, &t.Status); err != nil {
			slog.Warn("pending tasks scan failed", "error", err)
			continue
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return tasks, err
	}
	return tasks, nil
}

// queryRevenueSnapshot computes 24h, week, and month revenue from booking
// artifacts, including per-channel and per-property breakdowns.
func queryRevenueSnapshot(ctx context.Context, pool *pgxpool.Pool, now time.Time) (RevenueSnapshot, error) {
	var snap RevenueSnapshot

	dayStart := now.Add(-24 * time.Hour)
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN a.created_at >= $1 THEN (a.metadata->>'total_price')::numeric ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.created_at >= $2 THEN (a.metadata->>'total_price')::numeric ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN a.created_at >= $3 THEN (a.metadata->>'total_price')::numeric ELSE 0 END), 0)
		FROM artifacts a
		WHERE a.source_id = 'guesthost'
		  AND a.artifact_type = 'booking'
		  AND a.created_at >= $3
	`, dayStart, weekStart, monthStart).Scan(&snap.DayRevenue, &snap.WeekRevenue, &snap.MonthRevenue)
	if err != nil {
		return snap, fmt.Errorf("query revenue snapshot: %w", err)
	}

	// Per-channel (booking_source) breakdown for the month
	snap.ByChannel = make(map[string]float64)
	chRows, err := pool.Query(ctx, `
		SELECT COALESCE(a.metadata->>'booking_source', 'unknown'),
		       COALESCE(SUM((a.metadata->>'total_price')::numeric), 0)
		FROM artifacts a
		WHERE a.source_id = 'guesthost'
		  AND a.artifact_type = 'booking'
		  AND a.created_at >= $1
		GROUP BY a.metadata->>'booking_source'
	`, monthStart)
	if err == nil {
		for chRows.Next() {
			var channel string
			var amount float64
			if err := chRows.Scan(&channel, &amount); err == nil && amount > 0 {
				snap.ByChannel[channel] = amount
			}
		}
		chRows.Close()
	}

	// Per-property breakdown for the month
	snap.ByProperty = make(map[string]float64)
	pRows, err := pool.Query(ctx, `
		SELECT COALESCE(a.metadata->>'property_name', 'unknown'),
		       COALESCE(SUM((a.metadata->>'total_price')::numeric), 0)
		FROM artifacts a
		WHERE a.source_id = 'guesthost'
		  AND a.artifact_type = 'booking'
		  AND a.created_at >= $1
		GROUP BY a.metadata->>'property_name'
	`, monthStart)
	if err == nil {
		for pRows.Next() {
			var propName string
			var amount float64
			if err := pRows.Scan(&propName, &amount); err == nil && amount > 0 {
				snap.ByProperty[propName] = amount
			}
		}
		pRows.Close()
	}

	return snap, nil
}

// gatherGuestSignals retrieves candidate guests and their deterministic signals
// for LLM concern judgment (BUG-021-010). Candidate selection is OPERATIONAL
// (guests we have a sentiment score for, or with stay history) — NOT the
// concern decision, which the LLM makes. No sentiment threshold is applied here.
func gatherGuestSignals(ctx context.Context, pool *pgxpool.Pool, limit int) ([]GuestSignal, error) {
	rows, err := pool.Query(ctx, `
		SELECT name, email, total_stays, sentiment_score, total_spend
		FROM guests
		WHERE sentiment_score IS NOT NULL OR total_stays > 1
		ORDER BY total_stays DESC, sentiment_score ASC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("gather guest signals: %w", err)
	}
	defer rows.Close()

	var signals []GuestSignal
	for rows.Next() {
		var s GuestSignal
		if err := rows.Scan(&s.Name, &s.Email, &s.TotalStays, &s.Sentiment, &s.TotalSpend); err != nil {
			slog.Warn("guest signal scan failed", "error", err)
			continue
		}
		s.Ref = len(signals)
		signals = append(signals, s)
	}
	if err := rows.Err(); err != nil {
		return signals, err
	}
	return signals, nil
}

// gatherPropertySignals retrieves candidate properties and their deterministic
// signals for LLM concern judgment (BUG-021-010). Candidate selection is
// OPERATIONAL (properties with a rating or any open issues) — NOT the concern
// decision. No rating/issue threshold is applied here.
func gatherPropertySignals(ctx context.Context, pool *pgxpool.Pool, limit int) ([]PropertySignal, error) {
	rows, err := pool.Query(ctx, `
		SELECT name, issue_count, avg_rating
		FROM properties
		WHERE avg_rating IS NOT NULL OR issue_count > 0
		ORDER BY issue_count DESC, avg_rating ASC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("gather property signals: %w", err)
	}
	defer rows.Close()

	var signals []PropertySignal
	for rows.Next() {
		var s PropertySignal
		if err := rows.Scan(&s.Name, &s.IssueCount, &s.AvgRating); err != nil {
			slog.Warn("property signal scan failed", "error", err)
			continue
		}
		s.Ref = len(signals)
		signals = append(signals, s)
	}
	if err := rows.Err(); err != nil {
		return signals, err
	}
	return signals, nil
}
