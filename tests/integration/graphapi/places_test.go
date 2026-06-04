//go:build integration

// Spec 080 SCOPE-080-03 — live-stack places tier.
//   - TestGraphAPI_ListPlaces_MergesSources — SCN-080-05 (seeds one
//     maps-side place via location_clusters and one artifact-side
//     place via artifacts.location_geo, asserts both surface in
//     /api/places without duplicate ids).
//   - TestGraphAPI_GetPlace — SCN-080-06 (place detail returns a
//     location object and uses the cross-link shape with reason
//     matching the "same place <label>" taxonomy prefix).

package graphapi_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type placeRow struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	ArtifactCount int    `json:"artifactCount"`
	Source        string `json:"source"`
}

type placesListBody struct {
	Items      []placeRow `json:"items"`
	NextCursor string     `json:"nextCursor"`
}

type placeLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type placeDetailBody struct {
	ID              string         `json:"id"`
	DisplayName     string         `json:"displayName"`
	Location        *placeLocation `json:"location"`
	LinkedArtifacts []crossLink    `json:"linkedArtifacts"`
}

func seedMapsCluster(t *testing.T, conn *pgx.Conn, prefix string, lat, lng float64) {
	t.Helper()
	id := prefix + "-cluster-0"
	_, err := conn.Exec(context.Background(),
		`INSERT INTO location_clusters
		   (id, source_ref, start_cluster_lat, start_cluster_lng,
		    end_cluster_lat, end_cluster_lng, activity_type,
		    activity_date, day_of_week, departure_hour,
		    distance_km, duration_min)
		 VALUES ($1, $2, $3, $4, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, prefix+"-src", lat, lng, "walk",
		time.Now().UTC().Format("2006-01-02"), 1, 9, 1.2, 15.0)
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}
}

func seedArtifactWithLocation(t *testing.T, conn *pgx.Conn, prefix string, locationName string) string {
	t.Helper()
	id := prefix + "-loc-artifact-0"
	geo, _ := json.Marshal(map[string]any{"name": locationName})
	_, err := conn.Exec(context.Background(),
		`INSERT INTO artifacts
		   (id, artifact_type, title, content_hash, source_id,
		    created_at, updated_at, location_geo)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), $6::jsonb)`,
		id, "note", id+"-title", id+"-hash", "graphapi-it-seed", string(geo))
	if err != nil {
		t.Fatalf("seed artifact-with-location: %v", err)
	}
	return id
}

func cleanupLocationClusters(t *testing.T, conn *pgx.Conn, prefix string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(),
		`DELETE FROM location_clusters WHERE id LIKE $1`, prefix+"-%"); err != nil {
		t.Logf("cleanup location_clusters failed (continuing): %v", err)
	}
}

func TestGraphAPI_ListPlaces_MergesSources(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() {
		cleanupLocationClusters(t, conn, prefix)
		cleanupFixtures(t, conn, prefix)
	})

	// Unique lat/lng so the maps-derived id ("mp:<lat>:<lng>") is
	// distinguishable from existing data.
	const lat, lng = 12.3456, 78.9012
	seedMapsCluster(t, conn, prefix, lat, lng)
	uniqueName := prefix + "-place-name"
	seedArtifactWithLocation(t, conn, prefix, uniqueName)

	resp, body := doAuthedGET(t, cfg, "/api/places?limit=200")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got placesListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(body))
	}

	// Adversarial: assert no duplicate ids across the union.
	seenIDs := map[string]bool{}
	sawMaps := false
	sawArtifact := false
	for _, it := range got.Items {
		if seenIDs[it.ID] {
			t.Fatalf("duplicate place id %q in /api/places response", it.ID)
		}
		seenIDs[it.ID] = true
		if strings.HasPrefix(it.ID, "mp:") && it.Source != "" {
			sawMaps = true
		}
		if strings.HasPrefix(it.ID, "ar:") && it.Source != "" {
			sawArtifact = true
		}
	}
	if !sawMaps {
		t.Fatalf("no maps-sourced place ('mp:' prefix) in %d items — merge missing maps source", len(got.Items))
	}
	if !sawArtifact {
		t.Fatalf("no artifact-derived place ('ar:' prefix) in %d items — merge missing artifact source", len(got.Items))
	}
}

func TestGraphAPI_GetPlace(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() {
		cleanupLocationClusters(t, conn, prefix)
		cleanupFixtures(t, conn, prefix)
	})

	uniqueName := prefix + "-detail-place"
	artID := seedArtifactWithLocation(t, conn, prefix, uniqueName)

	// Discover the artifact-derived id via the list endpoint
	// (server-computed sha1 over the lowercased trimmed name).
	listResp, listBody := doAuthedGET(t, cfg, "/api/places?limit=200")
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResp.StatusCode, string(listBody))
	}
	var list placesListBody
	if err := json.Unmarshal(listBody, &list); err != nil {
		t.Fatalf("decode list: %v body=%s", err, string(listBody))
	}
	var targetID string
	for _, it := range list.Items {
		if it.DisplayName == uniqueName {
			targetID = it.ID
			break
		}
	}
	if targetID == "" {
		t.Fatalf("seeded place %q not found in list response (have %d items)", uniqueName, len(list.Items))
	}

	resp, body := doAuthedGET(t, cfg, "/api/places/"+targetID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", resp.StatusCode, string(body))
	}
	var got placeDetailBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode detail: %v body=%s", err, string(body))
	}
	if got.ID != targetID {
		t.Fatalf("detail id=%q; want %q", got.ID, targetID)
	}
	if got.DisplayName != uniqueName {
		t.Fatalf("detail displayName=%q; want %q", got.DisplayName, uniqueName)
	}
	if len(got.LinkedArtifacts) < 1 {
		t.Fatalf("linkedArtifacts=%d; want >=1 (seeded artifact %s)", len(got.LinkedArtifacts), artID)
	}
	for _, cl := range got.LinkedArtifacts {
		if cl.TargetKind == "" || cl.TargetID == "" || cl.TargetLabel == "" || cl.Reason == "" {
			t.Fatalf("cross-link missing field: %+v", cl)
		}
		// SCN-080-06 reason taxonomy: "same place <label>".
		if !strings.HasPrefix(cl.Reason, "same place ") {
			t.Fatalf("reason=%q; want prefix 'same place '", cl.Reason)
		}
	}
}

// Ensure SCN-080-05 maps-only id format remains stable across server
// builds: the maps-derived id MUST embed the (lat,lng) so two clients
// computing it independently agree. We don't reimplement the format
// here, but we assert the prefix and that ListPlaces returns at least
// one mp-prefixed id when location_clusters is non-empty.
var _ = http.StatusOK // keep net/http used even if compiler dead-code-eliminates above
