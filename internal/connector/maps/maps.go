package maps

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// maxActivities caps the number of activities parsed from a single Takeout
// import to prevent memory exhaustion from multi-year exports.
const maxActivities = 50000

// ActivityType represents the type of map activity.
type ActivityType string

const (
	ActivityWalk    ActivityType = "walk"
	ActivityCycle   ActivityType = "cycle"
	ActivityDrive   ActivityType = "drive"
	ActivityTransit ActivityType = "transit"
	ActivityHike    ActivityType = "hike"
	ActivityRun     ActivityType = "run"
)

// TakeoutActivity represents a parsed Google Takeout activity.
type TakeoutActivity struct {
	Type          ActivityType `json:"activity_type"`
	StartTime     time.Time    `json:"start_time"`
	EndTime       time.Time    `json:"end_time"`
	Route         []LatLng     `json:"route"`
	StartLocation LatLng       `json:"start_location"`
	EndLocation   LatLng       `json:"end_location"`
	DistanceKm    float64      `json:"distance_km"`
	DurationMin   float64      `json:"duration_min"`
	ElevationM    float64      `json:"elevation_m"`
}

// LatLng represents a geographic coordinate.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// ParseTakeoutJSON parses Google Takeout location history JSON.
func ParseTakeoutJSON(data []byte) ([]TakeoutActivity, error) {
	var raw struct {
		TimelineObjects []struct {
			ActivitySegment *struct {
				StartLocation struct {
					LatitudeE7  int `json:"latitudeE7"`
					LongitudeE7 int `json:"longitudeE7"`
				} `json:"startLocation"`
				EndLocation struct {
					LatitudeE7  int `json:"latitudeE7"`
					LongitudeE7 int `json:"longitudeE7"`
				} `json:"endLocation"`
				Duration struct {
					StartTimestamp string `json:"startTimestamp"`
					EndTimestamp   string `json:"endTimestamp"`
				} `json:"duration"`
				Distance     int    `json:"distance"`
				ActivityType string `json:"activityType"`
				WaypointPath struct {
					Waypoints []struct {
						LatE7 int `json:"latE7"`
						LngE7 int `json:"lngE7"`
					} `json:"waypoints"`
				} `json:"waypointPath"`
			} `json:"activitySegment"`
		} `json:"timelineObjects"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse takeout JSON: %w", err)
	}

	var activities []TakeoutActivity
	for _, obj := range raw.TimelineObjects {
		seg := obj.ActivitySegment
		if seg == nil {
			continue
		}

		startTime, err := time.Parse(time.RFC3339, seg.Duration.StartTimestamp)
		if err != nil {
			slog.Warn("skipping activity with unparseable start timestamp",
				"timestamp", seg.Duration.StartTimestamp, "error", err)
			continue
		}
		endTime, err := time.Parse(time.RFC3339, seg.Duration.EndTimestamp)
		if err != nil {
			slog.Warn("skipping activity with unparseable end timestamp",
				"timestamp", seg.Duration.EndTimestamp, "error", err)
			continue
		}

		// Validate before classifying: skip structurally invalid entries early.
		if seg.Distance < 0 {
			slog.Warn("skipping activity with negative distance",
				"distance", seg.Distance)
			continue
		}
		if endTime.Before(startTime) {
			slog.Warn("skipping activity with end time before start time",
				"start", seg.Duration.StartTimestamp, "end", seg.Duration.EndTimestamp)
			continue
		}

		// Validate start/end location coordinates (same bounds as waypoints).
		startLat := float64(seg.StartLocation.LatitudeE7) / 1e7
		startLng := float64(seg.StartLocation.LongitudeE7) / 1e7
		endLat := float64(seg.EndLocation.LatitudeE7) / 1e7
		endLng := float64(seg.EndLocation.LongitudeE7) / 1e7
		if !validCoord(startLat, startLng) {
			slog.Warn("skipping activity with out-of-range start location",
				"lat", startLat, "lng", startLng)
			continue
		}
		if !validCoord(endLat, endLng) {
			slog.Warn("skipping activity with out-of-range end location",
				"lat", endLat, "lng", endLng)
			continue
		}

		actType := ClassifyActivity(seg.ActivityType, float64(seg.Distance)/1000.0)

		var route []LatLng
		for _, wp := range seg.WaypointPath.Waypoints {
			lat := float64(wp.LatE7) / 1e7
			lng := float64(wp.LngE7) / 1e7
			// Skip waypoints with out-of-range coordinates.
			if !validCoord(lat, lng) {
				slog.Warn("skipping waypoint with out-of-range coordinates",
					"lat", lat, "lng", lng)
				continue
			}
			route = append(route, LatLng{
				Lat: lat,
				Lng: lng,
			})
		}

		activities = append(activities, TakeoutActivity{
			Type:      actType,
			StartTime: startTime,
			EndTime:   endTime,
			Route:     route,
			StartLocation: LatLng{
				Lat: startLat,
				Lng: startLng,
			},
			EndLocation: LatLng{
				Lat: endLat,
				Lng: endLng,
			},
			DistanceKm:  float64(seg.Distance) / 1000.0,
			DurationMin: endTime.Sub(startTime).Minutes(),
		})

		if len(activities) >= maxActivities {
			slog.Warn("activities cap reached, truncating import",
				"cap", maxActivities)
			break
		}
	}

	return activities, nil
}

// ClassifyActivity determines the activity type from Google's type and distance.
func ClassifyActivity(googleType string, distanceKm float64) ActivityType {
	switch googleType {
	case "WALKING":
		if distanceKm > 5.0 {
			return ActivityHike
		}
		return ActivityWalk
	case "CYCLING":
		return ActivityCycle
	case "IN_VEHICLE", "DRIVING":
		return ActivityDrive
	case "IN_BUS", "IN_SUBWAY", "IN_TRAIN", "IN_TRAM":
		return ActivityTransit
	case "RUNNING":
		return ActivityRun
	default:
		return ActivityWalk
	}
}

// IsTrailQualified checks if an activity qualifies as a trail.
// Per R-404: walking/hiking/running >=2km OR >=30min, cycling >=5km.
func IsTrailQualified(activity TakeoutActivity) bool {
	switch activity.Type {
	case ActivityWalk, ActivityHike, ActivityRun:
		return activity.DistanceKm >= 2.0 || activity.DurationMin >= 30.0
	case ActivityCycle:
		return activity.DistanceKm >= 5.0
	default:
		return false
	}
}

// ToGeoJSON converts a route to GeoJSON format.
// Returns a LineString for routes with ≥2 points (per RFC 7946 §3.1.4),
// a Point for single-point routes, or nil for empty routes.
func ToGeoJSON(route []LatLng) map[string]interface{} {
	switch {
	case len(route) == 0:
		return nil
	case len(route) == 1:
		return map[string]interface{}{
			"type":        "Point",
			"coordinates": []float64{route[0].Lng, route[0].Lat},
		}
	default:
		coords := make([][]float64, len(route))
		for i, p := range route {
			coords[i] = []float64{p.Lng, p.Lat}
		}
		return map[string]interface{}{
			"type":        "LineString",
			"coordinates": coords,
		}
	}
}

// validCoord returns true when lat ∈ [-90, 90] and lng ∈ [-180, 180].
func validCoord(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// Haversine calculates distance between two LatLng points in km.
func Haversine(a, b LatLng) float64 {
	const R = 6371.0 // Earth radius in km
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLng/2)*math.Sin(dLng/2)*math.Cos(lat1)*math.Cos(lat2)
	return 2 * R * math.Asin(math.Sqrt(h))
}
