package maps

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/smackerel/smackerel/internal/connector"
)

// PatternDetector detects commute patterns, trip events, and creates temporal-spatial links.
type PatternDetector struct {
	pool   *pgxpool.Pool
	config MapsConfig
}

// CommutePattern represents a detected repeated commute route.
type CommutePattern struct {
	StartClusterID       string
	EndClusterID         string
	StartLat             float64
	StartLng             float64
	EndLat               float64
	EndLng               float64
	Frequency            int
	TypicalDepartureHour int
	AvgDurationMin       float64
	AvgDistanceKm        float64
}

// TripEvent represents a detected trip away from home.
type TripEvent struct {
	DestinationLat    float64
	DestinationLng    float64
	StartDate         time.Time
	EndDate           time.Time
	DistanceFromHome  float64
	ActivityBreakdown map[string]int
	TotalActivities   int
}

// LocationCluster represents a row from the location_clusters table.
type LocationCluster struct {
	ID              string
	SourceRef       string
	StartClusterLat float64
	StartClusterLng float64
	EndClusterLat   float64
	EndClusterLng   float64
	ActivityType    string
	ActivityDate    time.Time
	DayOfWeek       int
	DepartureHour   int
	DistanceKm      float64
	DurationMin     float64
}

// NewPatternDetector creates a new PatternDetector.
func NewPatternDetector(pool *pgxpool.Pool, config MapsConfig) *PatternDetector {
	return &PatternDetector{
		pool:   pool,
		config: config,
	}
}

// DetectCommutes queries location_clusters and classifies commute patterns.
func (pd *PatternDetector) DetectCommutes(ctx context.Context) ([]connector.RawArtifact, error) {
	clusters, err := pd.queryRecentClusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("query recent clusters: %w", err)
	}

	patterns := classifyCommutes(clusters, pd.config)
	return normalizeCommutePatterns(patterns), nil
}

// DetectTrips queries location_clusters and classifies trip events.
func (pd *PatternDetector) DetectTrips(ctx context.Context) ([]connector.RawArtifact, error) {
	home, err := pd.InferHome(ctx)
	if err != nil {
		return nil, fmt.Errorf("infer home: %w", err)
	}
	if home == nil {
		slog.Info("no home location inferred, skipping trip detection")
		return nil, nil
	}

	clusters, err := pd.queryRecentClusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("query recent clusters: %w", err)
	}

	trips := classifyTrips(clusters, *home, pd.config)
	return normalizeTripEvents(trips), nil
}

// LinkTemporalSpatial creates CAPTURED_DURING edges between synced activities and existing artifacts.
func (pd *PatternDetector) LinkTemporalSpatial(ctx context.Context, activities []TakeoutActivity) (int, error) {
	if pd.pool == nil {
		return 0, nil
	}

	linkedCount := 0
	extend := time.Duration(pd.config.LinkTimeExtendMin) * time.Minute
	proximityKm := pd.config.LinkProximityRadiusM / 1000.0

	for _, activity := range activities {
		// Check for context cancellation between activities.
		if err := ctx.Err(); err != nil {
			return linkedCount, fmt.Errorf("linking cancelled after %d links: %w", linkedCount, err)
		}

		windowStart := activity.StartTime.Add(-extend)
		windowEnd := activity.EndTime.Add(extend)

		// Query artifacts that overlap the time window.
		rows, err := pd.pool.Query(ctx, `
			SELECT id, captured_at,
				COALESCE((metadata->>'lat')::double precision, 0) AS lat,
				COALESCE((metadata->>'lng')::double precision, 0) AS lng
			FROM artifacts
			WHERE source_id != 'google-maps-timeline'
				AND captured_at >= $1
				AND captured_at <= $2`,
			windowStart, windowEnd,
		)
		if err != nil {
			slog.Warn("temporal-spatial query failed", "activity_start", activity.StartTime, "error", err)
			continue
		}

		// Collect matching artifacts before closing rows to avoid holding
		// a connection while inserting edges (pool exhaustion risk).
		type artifactMatch struct {
			id  string
			lat float64
			lng float64
		}
		var matches []artifactMatch
		for rows.Next() {
			var m artifactMatch
			var capturedAt time.Time
			if err := rows.Scan(&m.id, &capturedAt, &m.lat, &m.lng); err != nil {
				slog.Warn("scan artifact row failed", "error", err)
				continue
			}
			matches = append(matches, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			slog.Warn("rows iteration error", "error", err)
			continue
		}

		activityRef := computeDedupHash(activity)

		for _, m := range matches {
			linkType := determineLinkType(activity, m.lat, m.lng, proximityKm)

			edgeID := ulid.Make().String()
			metaJSON, _ := json.Marshal(map[string]string{"link_type": linkType})

			_, err := pd.pool.Exec(ctx, `
				INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
				VALUES ($1, 'artifact', $2, 'artifact', $3, 'CAPTURED_DURING', 1.0, $4)
				ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING`,
				edgeID, m.id, activityRef, metaJSON,
			)
			if err != nil {
				slog.Warn("insert edge failed", "source", m.id, "target", activityRef, "error", err)
				continue
			}
			linkedCount++
		}
	}

	return linkedCount, nil
}

// InferHome finds the most frequent weekday morning (Mon-Fri, 6-10am) start cluster.
func (pd *PatternDetector) InferHome(ctx context.Context) (*LatLng, error) {
	if pd.pool == nil {
		return nil, nil
	}

	row := pd.pool.QueryRow(ctx, `
		SELECT start_cluster_lat, start_cluster_lng, COUNT(*) as freq
		FROM location_clusters
		WHERE day_of_week BETWEEN 1 AND 5
			AND departure_hour BETWEEN 6 AND 9
		GROUP BY start_cluster_lat, start_cluster_lng
		ORDER BY freq DESC
		LIMIT 1`,
	)

	var lat, lng float64
	var freq int
	if err := row.Scan(&lat, &lng, &freq); err != nil {
		return nil, nil // no data yet
	}

	return &LatLng{Lat: lat, Lng: lng}, nil
}

// queryRecentClusters retrieves location_clusters within the configured window.
func (pd *PatternDetector) queryRecentClusters(ctx context.Context) ([]LocationCluster, error) {
	cutoff := time.Now().AddDate(0, 0, -pd.config.CommuteWindowDays)

	rows, err := pd.pool.Query(ctx, `
		SELECT id, source_ref,
			start_cluster_lat, start_cluster_lng,
			end_cluster_lat, end_cluster_lng,
			activity_type, activity_date,
			day_of_week, departure_hour,
			distance_km, duration_min
		FROM location_clusters
		WHERE activity_date >= $1
		ORDER BY activity_date`,
		cutoff.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("query location_clusters: %w", err)
	}
	defer rows.Close()

	var clusters []LocationCluster
	for rows.Next() {
		var c LocationCluster
		if err := rows.Scan(
			&c.ID, &c.SourceRef,
			&c.StartClusterLat, &c.StartClusterLng,
			&c.EndClusterLat, &c.EndClusterLng,
			&c.ActivityType, &c.ActivityDate,
			&c.DayOfWeek, &c.DepartureHour,
			&c.DistanceKm, &c.DurationMin,
		); err != nil {
			return nil, fmt.Errorf("scan location_cluster row: %w", err)
		}
		clusters = append(clusters, c)
	}
	return clusters, rows.Err()
}

// --- Pure Logic Functions (unit-testable without DB) ---

// routeKey produces a deterministic key for a start→end cluster pair.
func routeKey(startLat, startLng, endLat, endLng float64) string {
	return fmt.Sprintf("%.3f,%.3f→%.3f,%.3f", startLat, startLng, endLat, endLng)
}

// classifyCommutes detects commute patterns from location cluster data.
// Pure logic — does not query the database.
func classifyCommutes(clusters []LocationCluster, config MapsConfig) []CommutePattern {
	type routeStats struct {
		startLat       float64
		startLng       float64
		endLat         float64
		endLng         float64
		count          int
		totalDuration  float64
		totalDistance  float64
		departureHours []int
	}

	routes := make(map[string]*routeStats)

	for _, c := range clusters {
		// Filter weekends if configured.
		if config.CommuteWeekdaysOnly && (c.DayOfWeek == 0 || c.DayOfWeek == 6) {
			continue
		}

		key := routeKey(c.StartClusterLat, c.StartClusterLng, c.EndClusterLat, c.EndClusterLng)
		rs, ok := routes[key]
		if !ok {
			rs = &routeStats{
				startLat: c.StartClusterLat,
				startLng: c.StartClusterLng,
				endLat:   c.EndClusterLat,
				endLng:   c.EndClusterLng,
			}
			routes[key] = rs
		}
		rs.count++
		rs.totalDuration += c.DurationMin
		rs.totalDistance += c.DistanceKm
		rs.departureHours = append(rs.departureHours, c.DepartureHour)
	}

	var patterns []CommutePattern
	for _, rs := range routes {
		if rs.count < config.CommuteMinOccurrences {
			continue
		}

		patterns = append(patterns, CommutePattern{
			StartClusterID:       fmt.Sprintf("%.3f,%.3f", rs.startLat, rs.startLng),
			EndClusterID:         fmt.Sprintf("%.3f,%.3f", rs.endLat, rs.endLng),
			StartLat:             rs.startLat,
			StartLng:             rs.startLng,
			EndLat:               rs.endLat,
			EndLng:               rs.endLng,
			Frequency:            rs.count,
			TypicalDepartureHour: typicalHour(rs.departureHours),
			AvgDurationMin:       rs.totalDuration / float64(rs.count),
			AvgDistanceKm:        rs.totalDistance / float64(rs.count),
		})
	}

	// Sort for deterministic output.
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Frequency > patterns[j].Frequency
	})

	return patterns
}

// classifyTrips detects trip events by finding clusters far from home with overnight stays.
// Pure logic — does not query the database.
func classifyTrips(clusters []LocationCluster, home LatLng, config MapsConfig) []TripEvent {
	// Sort clusters by date.
	sorted := make([]LocationCluster, len(clusters))
	copy(sorted, clusters)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ActivityDate.Before(sorted[j].ActivityDate)
	})

	// Find remote activity runs (clusters far from home).
	type dayEntry struct {
		date       time.Time
		destLat    float64
		destLng    float64
		activities map[string]int
		total      int
	}

	var remoteDays []dayEntry

	for _, c := range sorted {
		// Check if either start or end cluster is far from home.
		startPt := LatLng{Lat: c.StartClusterLat, Lng: c.StartClusterLng}
		endPt := LatLng{Lat: c.EndClusterLat, Lng: c.EndClusterLng}
		startDist := Haversine(home, startPt)
		endDist := Haversine(home, endPt)
		if startDist < config.TripMinDistanceKm && endDist < config.TripMinDistanceKm {
			continue
		}

		// Use the remote end as destination.
		destLat, destLng := endPt.Lat, endPt.Lng
		if endDist < startDist {
			destLat, destLng = startPt.Lat, startPt.Lng
		}

		if len(remoteDays) > 0 {
			last := &remoteDays[len(remoteDays)-1]
			if sameDate(last.date, c.ActivityDate) {
				last.activities[c.ActivityType]++
				last.total++
				continue
			}
		}

		remoteDays = append(remoteDays, dayEntry{
			date:       c.ActivityDate,
			destLat:    destLat,
			destLng:    destLng,
			activities: map[string]int{c.ActivityType: 1},
			total:      1,
		})
	}

	// Group consecutive remote days into trips.
	var trips []TripEvent
	if len(remoteDays) == 0 {
		return trips
	}

	tripStart := 0
	for i := 1; i <= len(remoteDays); i++ {
		consecutive := i < len(remoteDays) && daysDiff(remoteDays[i-1].date, remoteDays[i].date) <= 1
		if consecutive {
			continue
		}

		// End of a consecutive run.
		startDay := remoteDays[tripStart]
		endDay := remoteDays[i-1]

		// Check minimum overnight hours: the span from first day start to last day end.
		spanHours := endDay.date.Sub(startDay.date).Hours() + 24 // add 24h for the last day itself
		if spanHours < config.TripMinOvernightHours {
			tripStart = i
			continue
		}

		// Merge activity breakdowns.
		breakdown := make(map[string]int)
		totalActivities := 0
		for j := tripStart; j < i; j++ {
			for actType, count := range remoteDays[j].activities {
				breakdown[actType] += count
			}
			totalActivities += remoteDays[j].total
		}

		// Use first remote day's destination as the trip destination.
		trip := TripEvent{
			DestinationLat:    startDay.destLat,
			DestinationLng:    startDay.destLng,
			StartDate:         startDay.date,
			EndDate:           endDay.date,
			DistanceFromHome:  Haversine(home, LatLng{Lat: startDay.destLat, Lng: startDay.destLng}),
			ActivityBreakdown: breakdown,
			TotalActivities:   totalActivities,
		}
		trips = append(trips, trip)

		tripStart = i
	}

	return trips
}

// determineLinkType decides the link type based on location proximity.
func determineLinkType(activity TakeoutActivity, artifactLat, artifactLng, proximityKm float64) string {
	if artifactLat == 0 && artifactLng == 0 {
		return "temporal-only"
	}

	// Check if artifact location is near any point on the activity route.
	artifactPt := LatLng{Lat: artifactLat, Lng: artifactLng}
	for _, routePt := range activity.Route {
		if Haversine(artifactPt, routePt) <= proximityKm {
			return "temporal-spatial"
		}
	}

	return "temporal-only"
}

// normalizeCommutePatterns converts detected patterns to RawArtifacts.
func normalizeCommutePatterns(patterns []CommutePattern) []connector.RawArtifact {
	var artifacts []connector.RawArtifact
	for _, p := range patterns {
		artifacts = append(artifacts, normalizeCommutePattern(p))
	}
	return artifacts
}

// normalizeCommutePattern converts a single CommutePattern to a RawArtifact.
func normalizeCommutePattern(p CommutePattern) connector.RawArtifact {
	sourceRef := commuteSourceRef(p)
	return connector.RawArtifact{
		SourceID:    "google-maps-timeline",
		SourceRef:   sourceRef,
		ContentType: "pattern/commute",
		Title:       fmt.Sprintf("Commute: %s→%s (%d trips)", p.StartClusterID, p.EndClusterID, p.Frequency),
		RawContent: fmt.Sprintf("Commute pattern: %s to %s, %d trips, typical departure %d:00, avg %.0fmin, avg %.1fkm",
			p.StartClusterID, p.EndClusterID, p.Frequency, p.TypicalDepartureHour, p.AvgDurationMin, p.AvgDistanceKm),
		Metadata: map[string]interface{}{
			"frequency":              p.Frequency,
			"typical_departure_hour": p.TypicalDepartureHour,
			"avg_duration_min":       p.AvgDurationMin,
			"avg_distance_km":        p.AvgDistanceKm,
			"start_lat":              p.StartLat,
			"start_lng":              p.StartLng,
			"end_lat":                p.EndLat,
			"end_lng":                p.EndLng,
			"processing_tier":        "light",
		},
		CapturedAt: time.Now(),
	}
}

// normalizeTripEvents converts detected trips to RawArtifacts.
func normalizeTripEvents(trips []TripEvent) []connector.RawArtifact {
	var artifacts []connector.RawArtifact
	for _, t := range trips {
		artifacts = append(artifacts, normalizeTripEvent(t))
	}
	return artifacts
}

// normalizeTripEvent converts a single TripEvent to a RawArtifact.
func normalizeTripEvent(t TripEvent) connector.RawArtifact {
	sourceRef := tripSourceRef(t)
	startStr := t.StartDate.Format("2006-01-02")
	endStr := t.EndDate.Format("2006-01-02")
	return connector.RawArtifact{
		SourceID:    "google-maps-timeline",
		SourceRef:   sourceRef,
		ContentType: "event/trip",
		Title:       fmt.Sprintf("Trip to (%.2f,%.2f) — %s–%s", t.DestinationLat, t.DestinationLng, startStr, endStr),
		RawContent: fmt.Sprintf("Trip to (%.2f,%.2f) from %s to %s, %.0fkm from home, %d activities",
			t.DestinationLat, t.DestinationLng, startStr, endStr, t.DistanceFromHome, t.TotalActivities),
		Metadata: map[string]interface{}{
			"destination_lat":    t.DestinationLat,
			"destination_lng":    t.DestinationLng,
			"start_date":         startStr,
			"end_date":           endStr,
			"distance_from_home": t.DistanceFromHome,
			"activity_breakdown": t.ActivityBreakdown,
			"total_activities":   t.TotalActivities,
			"processing_tier":    "full",
		},
		CapturedAt: t.StartDate,
	}
}

// commuteSourceRef produces a deterministic sourceRef for dedup of commute patterns.
func commuteSourceRef(p CommutePattern) string {
	input := fmt.Sprintf("commute:%.3f,%.3f:%.3f,%.3f", p.StartLat, p.StartLng, p.EndLat, p.EndLng)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("commute-%x", hash[:8])
}

// tripSourceRef produces a deterministic sourceRef for dedup of trip events.
func tripSourceRef(t TripEvent) string {
	input := fmt.Sprintf("trip:%.3f,%.3f:%s:%s",
		t.DestinationLat, t.DestinationLng,
		t.StartDate.Format("2006-01-02"), t.EndDate.Format("2006-01-02"))
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("trip-%x", hash[:8])
}

// typicalHour finds the most frequent hour among departure hours.
func typicalHour(hours []int) int {
	if len(hours) == 0 {
		return 0
	}
	freq := make(map[int]int)
	for _, h := range hours {
		freq[h]++
	}
	bestHour := hours[0]
	bestCount := 0
	for h, c := range freq {
		if c > bestCount {
			bestHour = h
			bestCount = c
		}
	}
	return bestHour
}

// sameDate checks if two times fall on the same calendar date (UTC).
func sameDate(a, b time.Time) bool {
	y1, m1, d1 := a.UTC().Date()
	y2, m2, d2 := b.UTC().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// daysDiff returns the number of calendar days between two dates.
func daysDiff(a, b time.Time) int {
	y1, m1, d1 := a.UTC().Date()
	y2, m2, d2 := b.UTC().Date()
	t1 := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	diff := t2.Sub(t1).Hours() / 24
	return int(math.Abs(diff))
}

// withProcessingTier returns a shallow copy of metadata with the processing_tier set to tier.
func withProcessingTier(metadata map[string]interface{}, tier string) map[string]interface{} {
	updated := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		updated[k] = v
	}
	updated["processing_tier"] = tier
	return updated
}
