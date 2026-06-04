//go:build integration

// Spec 080 SCOPE-080-02 — live-stack topics tier.
//   - TestGraphAPI_ListTopics — SCN-080-01 (items + cursor shape).
//   - TestGraphAPI_GetTopic — SCN-080-02 (cross-link envelope + reason).
//   - TestGraphAPI_ListTopics_Pagination — adversarial: opaque cursor
//     round-trip across two pages MUST NOT duplicate or skip ids.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type topicRow struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	LinkedArtifactCount int    `json:"linkedArtifactCount"`
	PeopleCount         int    `json:"peopleCount"`
	PlaceCount          int    `json:"placeCount"`
}

type topicsListBody struct {
	Items      []topicRow `json:"items"`
	NextCursor string     `json:"nextCursor"`
}

type crossLink struct {
	TargetKind  string `json:"targetKind"`
	TargetID    string `json:"targetId"`
	TargetLabel string `json:"targetLabel"`
	Reason      string `json:"reason"`
}

type topicDetailBody struct {
	ID              string      `json:"id"`
	Label           string      `json:"label"`
	LinkedArtifacts []crossLink `json:"linkedArtifacts"`
	RelatedPeople   []crossLink `json:"relatedPeople"`
	RelatedPlaces   []crossLink `json:"relatedPlaces"`
}

func TestGraphAPI_ListTopics(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	// SCN-080-01 requires "at least 3 topics with linked artifacts".
	topicIDs := seedTopics(t, conn, prefix, 3)
	artIDs := seedArtifacts(t, conn, prefix, 3)
	for i, tid := range topicIDs {
		seedEdge(t, conn, prefix, "topic", tid, "artifact", artIDs[i], "mentions", 1.0)
	}

	resp, body := doAuthedGET(t, cfg, "/api/topics?limit=50")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got topicsListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode list body: %v body=%s", err, string(body))
	}

	// nextCursor field MUST be present (may be empty); items MUST
	// be non-nil; every seeded topic id MUST appear at least once.
	seenIDs := map[string]bool{}
	for _, it := range got.Items {
		if it.ID == "" {
			t.Fatalf("item with empty id: %+v", it)
		}
		if it.Label == "" {
			t.Fatalf("item with empty label: %+v", it)
		}
		seenIDs[it.ID] = true
	}
	for _, want := range topicIDs {
		if !seenIDs[want] {
			t.Fatalf("seeded topic %s missing from response (have %d items)", want, len(got.Items))
		}
	}
}

func TestGraphAPI_GetTopic(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	topicIDs := seedTopics(t, conn, prefix, 1)
	artIDs := seedArtifacts(t, conn, prefix, 2)
	peopleIDs := seedPeople(t, conn, prefix, 1)
	for _, aid := range artIDs {
		seedEdge(t, conn, prefix, "topic", topicIDs[0], "artifact", aid, "mentions", 1.0)
	}
	seedEdge(t, conn, prefix, "topic", topicIDs[0], "person", peopleIDs[0], "co-occurs", 1.0)

	resp, body := doAuthedGET(t, cfg, "/api/topics/"+topicIDs[0])
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got topicDetailBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode detail body: %v body=%s", err, string(body))
	}
	if got.ID != topicIDs[0] {
		t.Fatalf("id=%q; want %q", got.ID, topicIDs[0])
	}
	if len(got.LinkedArtifacts) < 2 {
		t.Fatalf("linkedArtifacts=%d; want >=2", len(got.LinkedArtifacts))
	}
	for _, cl := range got.LinkedArtifacts {
		if cl.TargetKind == "" || cl.TargetID == "" || cl.TargetLabel == "" || cl.Reason == "" {
			t.Fatalf("linkedArtifact has empty cross-link field: %+v", cl)
		}
	}
	if len(got.RelatedPeople) < 1 {
		t.Fatalf("relatedPeople=%d; want >=1", len(got.RelatedPeople))
	}
	for _, cl := range got.RelatedPeople {
		if cl.Reason == "" {
			t.Fatalf("relatedPerson has empty reason: %+v", cl)
		}
	}
}

func TestGraphAPI_ListTopics_Pagination(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	// Seed 5 topics; ask limit=2, then follow the cursor to page 2.
	// Adversarial: row counts across page1 ∪ page2 MUST have no
	// duplicate ids — if cursor were row-offset and a concurrent
	// insert shifted the window we'd see dupes; opaque cursor
	// keeps the window stable.
	seedTopics(t, conn, prefix, 5)

	resp1, body1 := doAuthedGET(t, cfg, "/api/topics?limit=2")
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("page1 status=%d body=%s", resp1.StatusCode, string(body1))
	}
	var page1 topicsListBody
	if err := json.Unmarshal(body1, &page1); err != nil {
		t.Fatalf("decode page1: %v body=%s", err, string(body1))
	}
	if page1.NextCursor == "" {
		t.Fatalf("page1.nextCursor empty; total topics >> 2 so cursor MUST be present")
	}

	resp2, body2 := doAuthedGET(t, cfg, "/api/topics?limit=2&cursor="+page1.NextCursor)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("page2 status=%d body=%s", resp2.StatusCode, string(body2))
	}
	var page2 topicsListBody
	if err := json.Unmarshal(body2, &page2); err != nil {
		t.Fatalf("decode page2: %v body=%s", err, string(body2))
	}

	seen := map[string]bool{}
	for _, it := range page1.Items {
		seen[it.ID] = true
	}
	for _, it := range page2.Items {
		if seen[it.ID] {
			t.Fatalf("duplicate id %q across page1+page2", it.ID)
		}
	}
}
