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
func AssembleHospitalityContext(ctx context.Context, pool *pgxpool.Pool) (*HospitalityDigestContext, error) {
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

	gAlerts, err := queryGuestAlerts(ctx, pool)
	if err != nil {
		slog.Warn("hospitality digest: failed to query guest alerts", "error", err)
	} else {
		hCtx.GuestAlerts = gAlerts
	}

	pAlerts, err := queryPropertyAlerts(ctx, pool)
	if err != nil {
		slog.Warn("hospitality digest: failed to query property alerts", "error", err)
	} else {
		hCtx.PropertyAlerts = pAlerts
	}

	// Fill revenue check-in/out counts from arrivals/departures
	hCtx.Revenue.TodayCheckIns = len(hCtx.TodayArrivals)
	hCtx.Revenue.TodayCheckOuts = len(hCtx.TodayDepartures)

	return hCtx, nil
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

// queryGuestAlerts returns alerts for repeat guests and low-sentiment guests.
func queryGuestAlerts(ctx context.Context, pool *pgxpool.Pool) ([]GuestAlert, error) {
	rows, err := pool.Query(ctx, `
		SELECT name, email,
			CASE
				WHEN total_stays > 1 THEN 'repeat_guest'
				WHEN sentiment_score IS NOT NULL AND sentiment_score < 0.3 THEN 'low_sentiment'
				ELSE 'unknown'
			END AS alert_type,
			CASE
				WHEN total_stays > 1 THEN FORMAT('Repeat guest with %s stays, total spend $%s', total_stays, ROUND(total_spend::numeric, 2))
				WHEN sentiment_score IS NOT NULL AND sentiment_score < 0.3 THEN FORMAT('Low sentiment score: %s', ROUND(sentiment_score::numeric, 2))
				ELSE ''
			END AS description
		FROM guests
		WHERE (total_stays > 1) OR (sentiment_score IS NOT NULL AND sentiment_score < 0.3)
		ORDER BY total_stays DESC, sentiment_score ASC
		LIMIT 20
	`)
	if err != nil {
		return nil, fmt.Errorf("query guest alerts: %w", err)
	}
	defer rows.Close()

	var alerts []GuestAlert
	for rows.Next() {
		var a GuestAlert
		if err := rows.Scan(&a.GuestName, &a.GuestEmail, &a.AlertType, &a.Description); err != nil {
			slog.Warn("guest alerts scan failed", "error", err)
			continue
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return alerts, err
	}
	return alerts, nil
}

// queryPropertyAlerts returns alerts for properties with high issue counts or
// low average ratings.
func queryPropertyAlerts(ctx context.Context, pool *pgxpool.Pool) ([]PropertyAlert, error) {
	rows, err := pool.Query(ctx, `
		SELECT name,
			CASE
				WHEN issue_count >= 5 THEN 'high_issue_count'
				WHEN avg_rating IS NOT NULL AND avg_rating < 3.5 THEN 'low_rating'
				ELSE 'unknown'
			END AS alert_type,
			CASE
				WHEN issue_count >= 5 THEN FORMAT('Property has %s open issues', issue_count)
				WHEN avg_rating IS NOT NULL AND avg_rating < 3.5 THEN FORMAT('Average rating: %s', ROUND(avg_rating::numeric, 1))
				ELSE ''
			END AS description
		FROM properties
		WHERE (issue_count >= 5) OR (avg_rating IS NOT NULL AND avg_rating < 3.5)
		ORDER BY issue_count DESC, avg_rating ASC
		LIMIT 20
	`)
	if err != nil {
		return nil, fmt.Errorf("query property alerts: %w", err)
	}
	defer rows.Close()

	var alerts []PropertyAlert
	for rows.Next() {
		var a PropertyAlert
		if err := rows.Scan(&a.PropertyName, &a.AlertType, &a.Description); err != nil {
			slog.Warn("property alerts scan failed", "error", err)
			continue
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return alerts, err
	}
	return alerts, nil
}
