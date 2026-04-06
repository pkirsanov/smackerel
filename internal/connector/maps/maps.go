package maps

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

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
	Type        ActivityType `json:"activity_type"`
	StartTime   time.Time    `json:"start_time"`
	EndTime     time.Time    `json:"end_time"`
	Route       []LatLng     `json:"route"`
	DistanceKm  float64      `json:"distance_km"`
	DurationMin float64      `json:"duration_min"`
	ElevationM  float64      `json:"elevation_m"`
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

		startTime, _ := time.Parse(time.RFC3339, seg.Duration.StartTimestamp)
		endTime, _ := time.Parse(time.RFC3339, seg.Duration.EndTimestamp)

		actType := ClassifyActivity(seg.ActivityType, float64(seg.Distance)/1000.0)

		var route []LatLng
		for _, wp := range seg.WaypointPath.Waypoints {
			route = append(route, LatLng{
				Lat: float64(wp.LatE7) / 1e7,
				Lng: float64(wp.LngE7) / 1e7,
			})
		}

		activities = append(activities, TakeoutActivity{
			Type:        actType,
			StartTime:   startTime,
			EndTime:     endTime,
			Route:       route,
			DistanceKm:  float64(seg.Distance) / 1000.0,
			DurationMin: endTime.Sub(startTime).Minutes(),
		})
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

// IsTrailQualified checks if an activity qualifies as a trail (>=2km walking/hiking/running).
func IsTrailQualified(activity TakeoutActivity) bool {
	switch activity.Type {
	case ActivityWalk, ActivityHike, ActivityRun, ActivityCycle:
		return activity.DistanceKm >= 2.0
	default:
		return false
	}
}

// ToGeoJSON converts a route to GeoJSON LineString format.
func ToGeoJSON(route []LatLng) map[string]interface{} {
	coords := make([][]float64, len(route))
	for i, p := range route {
		coords[i] = []float64{p.Lng, p.Lat}
	}
	return map[string]interface{}{
		"type":        "LineString",
		"coordinates": coords,
	}
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
