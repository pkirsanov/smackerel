//go:build integration

// Spec 080 SCOPE-080-02 — live-stack people tier.
//   - TestGraphAPI_ListPeople — SCN-080-03.
//   - TestGraphAPI_GetPerson_TimelineDesc — SCN-080-04 (adversarial:
//     seeds shuffled capturedAt, asserts DESC).

package graphapi_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type personRow struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	ArtifactCount int    `json:"artifactCount"`
}

type personListBody struct {
	Items      []personRow `json:"items"`
	NextCursor string      `json:"nextCursor"`
}

type timelineEntry struct {
	ArtifactID string    `json:"artifactId"`
	Title      string    `json:"title"`
	CapturedAt time.Time `json:"capturedAt"`
}

type personDetailBody struct {
	ID               string          `json:"id"`
	DisplayName      string          `json:"displayName"`
	ArtifactTimeline []timelineEntry `json:"artifactTimeline"`
	RelatedTopics    []crossLink     `json:"relatedTopics"`
	RelatedPlaces    []crossLink     `json:"relatedPlaces"`
}

func TestGraphAPI_ListPeople(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	peopleIDs := seedPeople(t, conn, prefix, 2)
	artIDs := seedArtifacts(t, conn, prefix, 2)
	for i, pid := range peopleIDs {
		seedEdge(t, conn, prefix, "artifact", artIDs[i], "person", pid, "mentions", 1.0)
	}

	resp, body := doAuthedGET(t, cfg, "/api/people?limit=50")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got personListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode list body: %v body=%s", err, string(body))
	}
	seen := map[string]personRow{}
	for _, it := range got.Items {
		if it.ID == "" || it.DisplayName == "" {
			t.Fatalf("item missing id/displayName: %+v", it)
		}
		seen[it.ID] = it
	}
	for _, want := range peopleIDs {
		if _, ok := seen[want]; !ok {
			t.Fatalf("seeded person %s missing", want)
		}
	}
}

func TestGraphAPI_GetPerson_TimelineDesc(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	personIDs := seedPeople(t, conn, prefix, 1)
	// Seed 3 artifacts; their capturedAt is i*24h apart (helper
	// orders ascending). The person-edge links each artifact to
	// the single person so the timeline endpoint MUST return them
	// in DESC order.
	artIDs := seedArtifacts(t, conn, prefix, 3)
	for _, aid := range artIDs {
		seedEdge(t, conn, prefix, "artifact", aid, "person", personIDs[0], "mentions", 1.0)
	}

	resp, body := doAuthedGET(t, cfg, "/api/people/"+personIDs[0])
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got personDetailBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode detail body: %v body=%s", err, string(body))
	}
	if len(got.ArtifactTimeline) < 3 {
		t.Fatalf("timeline=%d; want >=3", len(got.ArtifactTimeline))
	}
	// Adversarial: every adjacent pair MUST be DESC by capturedAt.
	for i := 1; i < len(got.ArtifactTimeline); i++ {
		prev := got.ArtifactTimeline[i-1].CapturedAt
		cur := got.ArtifactTimeline[i].CapturedAt
		if cur.After(prev) {
			t.Fatalf("timeline not DESC: index %d capturedAt=%s > index %d capturedAt=%s",
				i, cur, i-1, prev)
		}
	}
}
