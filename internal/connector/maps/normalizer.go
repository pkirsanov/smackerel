package maps

import (
	"crypto/sha256"
	"fmt"
	"math"

	"github.com/smackerel/smackerel/internal/connector"
)

// NormalizeActivity converts a TakeoutActivity into a connector.RawArtifact.
func NormalizeActivity(activity TakeoutActivity, sourceFile string) connector.RawArtifact {
	actType := validatedActivityType(activity.Type)
	activity.Type = actType
	return connector.RawArtifact{
		SourceID:    "google-maps-timeline",
		SourceRef:   computeDedupHash(activity),
		ContentType: "activity/" + string(actType),
		Title:       buildTitle(activity),
		RawContent:  buildContent(activity),
		Metadata:    buildMetadata(activity, sourceFile),
		CapturedAt:  activity.StartTime,
	}
}

// validatedActivityType returns the activity type if known, or ActivityWalk as a safe default.
func validatedActivityType(t ActivityType) ActivityType {
	switch t {
	case ActivityHike, ActivityWalk, ActivityCycle, ActivityDrive, ActivityTransit, ActivityRun:
		return t
	default:
		return ActivityWalk
	}
}

// buildTitle generates a human-readable title: "Hike — 8.3km, 142min".
func buildTitle(activity TakeoutActivity) string {
	typeName := activityDisplayName(activity.Type)
	return fmt.Sprintf("%s — %.1fkm, %.0fmin", typeName, activity.DistanceKm, activity.DurationMin)
}

// buildContent assembles a human-readable activity summary.
func buildContent(activity TakeoutActivity) string {
	typeName := activityDisplayName(activity.Type)
	date := activity.StartTime.Format("2006-01-02")
	startTime := activity.StartTime.Format("15:04")
	endTime := activity.EndTime.Format("15:04")

	content := fmt.Sprintf("%s on %s from %s to %s.\nDistance: %.1fkm. Duration: %.0f minutes.",
		typeName, date, startTime, endTime, activity.DistanceKm, activity.DurationMin)

	if len(activity.Route) > 0 {
		start := activity.Route[0]
		end := activity.Route[len(activity.Route)-1]
		content += fmt.Sprintf("\nStart: [%.3f, %.3f]. End: [%.3f, %.3f].",
			start.Lat, start.Lng, end.Lat, end.Lng)
		content += fmt.Sprintf("\nRoute: %d waypoints.", len(activity.Route))
	}

	return content
}

// buildMetadata creates the full metadata map per R-007.
func buildMetadata(activity TakeoutActivity, sourceFile string) map[string]interface{} {
	meta := map[string]interface{}{
		"activity_type":   string(activity.Type),
		"start_time":      activity.StartTime.Format("2006-01-02T15:04:05Z07:00"),
		"end_time":        activity.EndTime.Format("2006-01-02T15:04:05Z07:00"),
		"distance_km":     activity.DistanceKm,
		"duration_min":    activity.DurationMin,
		"elevation_m":     activity.ElevationM,
		"waypoint_count":  len(activity.Route),
		"trail_qualified": IsTrailQualified(activity),
		"source_file":     sourceFile,
		"dedup_hash":      computeDedupHash(activity),
		"processing_tier": assignTier(activity),
	}

	if len(activity.Route) > 0 {
		meta["start_lat"] = activity.Route[0].Lat
		meta["start_lng"] = activity.Route[0].Lng
		meta["end_lat"] = activity.Route[len(activity.Route)-1].Lat
		meta["end_lng"] = activity.Route[len(activity.Route)-1].Lng
		meta["route_geojson"] = ToGeoJSON(activity.Route)
	} else if activity.StartLocation.Lat != 0 || activity.StartLocation.Lng != 0 ||
		activity.EndLocation.Lat != 0 || activity.EndLocation.Lng != 0 {
		meta["start_lat"] = activity.StartLocation.Lat
		meta["start_lng"] = activity.StartLocation.Lng
		meta["end_lat"] = activity.EndLocation.Lat
		meta["end_lng"] = activity.EndLocation.Lng
		meta["route_geojson"] = nil
	} else {
		meta["start_lat"] = 0.0
		meta["start_lng"] = 0.0
		meta["end_lat"] = 0.0
		meta["end_lng"] = 0.0
		meta["route_geojson"] = nil
	}

	return meta
}

// activityGridCoords returns the start and end route coordinates snapped to a ~500m grid.
// Falls back to StartLocation/EndLocation when route is empty.
// Returns zeroes when neither route nor start/end locations are available.
func activityGridCoords(activity TakeoutActivity) (startLat, startLng, endLat, endLng float64) {
	if len(activity.Route) > 0 {
		startLat = roundToGrid(activity.Route[0].Lat)
		startLng = roundToGrid(activity.Route[0].Lng)
		endLat = roundToGrid(activity.Route[len(activity.Route)-1].Lat)
		endLng = roundToGrid(activity.Route[len(activity.Route)-1].Lng)
	} else if activity.StartLocation.Lat != 0 || activity.StartLocation.Lng != 0 ||
		activity.EndLocation.Lat != 0 || activity.EndLocation.Lng != 0 {
		startLat = roundToGrid(activity.StartLocation.Lat)
		startLng = roundToGrid(activity.StartLocation.Lng)
		endLat = roundToGrid(activity.EndLocation.Lat)
		endLng = roundToGrid(activity.EndLocation.Lng)
	}
	return
}

// computeDedupHash generates a dedup key from date + activity type + start hour + rounded coords.
// Including type and hour prevents hash collision between distinct activities at the same
// grid location on the same day (e.g., morning jog vs. evening walk, or two routeless drives).
func computeDedupHash(activity TakeoutActivity) string {
	date := activity.StartTime.Format("2006-01-02")
	hour := activity.StartTime.Hour()
	startLat, startLng, endLat, endLng := activityGridCoords(activity)

	input := fmt.Sprintf("%s:%s:%d:%.3f,%.3f:%.3f,%.3f",
		date, string(activity.Type), hour, startLat, startLng, endLat, endLng)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:8]) // first 16 hex chars
}

// assignTier determines the processing tier for an activity.
func assignTier(activity TakeoutActivity) string {
	if IsTrailQualified(activity) {
		return "full"
	}
	return "standard"
}

// roundToGrid rounds a coordinate component to a ~500m grid.
func roundToGrid(v float64) float64 {
	return math.Floor(v*200) / 200
}

// activityDisplayName returns a human-readable display name for an activity type.
func activityDisplayName(t ActivityType) string {
	switch t {
	case ActivityHike:
		return "Hike"
	case ActivityWalk:
		return "Walk"
	case ActivityCycle:
		return "Cycle"
	case ActivityDrive:
		return "Drive"
	case ActivityTransit:
		return "Transit"
	case ActivityRun:
		return "Run"
	default:
		return "Activity"
	}
}
