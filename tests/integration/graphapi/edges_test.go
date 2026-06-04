//go:build integration

// Spec 080 SCOPE-080-04 — live-stack edges tier.
//   - TestGraphAPI_ListEdges_ArtifactToAllKinds — SCN-080-08.
//   - TestGraphAPI_ListEdges_UnknownKind — SCN-080-14 adversarial.
//   - TestGraphAPI_ListEdges_MissingSource — adversarial: missing
//     ?source= MUST be 400 missing_param.
//   - TestGraphAPI_ListEdges_MalformedSource — adversarial: malformed
//     "kind:id" MUST be 400 invalid_kind.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type edgesListBody struct {
	Items      []crossLink `json:"items"`
	NextCursor string      `json:"nextCursor"`
}

func TestGraphAPI_ListEdges_ArtifactToAllKinds(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	// Seed an artifact + one of each downstream kind, then 3 edges
	// FROM the artifact TO topic/person/place. The dst label is the
	// id (people.name = id, topics.name = id in seedTopics, places
	// fall back to dst_id when no places table row exists). The
	// SCN-080-08 contract requires every item to carry a non-empty
	// reason; we assert presence of all three target kinds.
	artIDs := seedArtifacts(t, conn, prefix, 1)
	topicIDs := seedTopics(t, conn, prefix, 1)
	peopleIDs := seedPeople(t, conn, prefix, 1)
	// place-side: there's no first-class places table in this
	// schema; the edges resolver falls back to dst_id when the
	// LEFT JOIN to places produces no row, which keeps the label
	// non-empty (so the reason resolver does not fail loud).
	placeID := prefix + "-place-0"
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "topic", topicIDs[0], "mentions", 3.0)
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "person", peopleIDs[0], "mentions", 2.0)
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "place", placeID, "mentions", 1.0)

	resp, body := doAuthedGET(t, cfg, "/api/graph/edges?source=artifact:"+artIDs[0]+"&limit=50")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s; want 200", resp.StatusCode, string(body))
	}
	var got edgesListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(body))
	}
	if len(got.Items) < 3 {
		t.Fatalf("items=%d; want >=3 (one per kind)", len(got.Items))
	}
	kinds := map[string]bool{}
	for _, it := range got.Items {
		if it.TargetKind == "" || it.TargetID == "" || it.TargetLabel == "" || it.Reason == "" {
			t.Fatalf("edge item missing field: %+v", it)
		}
		kinds[it.TargetKind] = true
	}
	for _, want := range []string{"topic", "person", "place"} {
		if !kinds[want] {
			t.Fatalf("no edge with targetKind=%q in response (have kinds=%v)", want, kinds)
		}
	}
}

func TestGraphAPI_ListEdges_UnknownKind(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)

	resp, body := doAuthedGET(t, cfg, "/api/graph/edges?source=unicorn:X1")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; want 400", resp.StatusCode, string(body))
	}
	env := decodeError(t, body)
	if env.Error.Code != "invalid_kind" {
		t.Fatalf("error.code=%q; want invalid_kind; body=%s",
			env.Error.Code, string(body))
	}
	// Message MUST list the allowed kinds verbatim per SCN-080-14.
	for _, k := range []string{"artifact", "topic", "person", "place"} {
		if !strings.Contains(env.Error.Message, k) {
			t.Fatalf("error.message=%q does not list allowed kind %q", env.Error.Message, k)
		}
	}
}

func TestGraphAPI_ListEdges_MissingSource(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)

	resp, body := doAuthedGET(t, cfg, "/api/graph/edges")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; want 400", resp.StatusCode, string(body))
	}
	env := decodeError(t, body)
	if env.Error.Code != "missing_param" {
		t.Fatalf("error.code=%q; want missing_param; body=%s",
			env.Error.Code, string(body))
	}
}

func TestGraphAPI_ListEdges_MalformedSource(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)

	// "no-colon" — no `kind:id` separator.
	resp, body := doAuthedGET(t, cfg, "/api/graph/edges?source=no-colon-here")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; want 400", resp.StatusCode, string(body))
	}
	env := decodeError(t, body)
	if env.Error.Code != "invalid_kind" {
		t.Fatalf("error.code=%q; want invalid_kind; body=%s",
			env.Error.Code, string(body))
	}
}

// keep time import used (helpers use time.Second indirectly).
var _ = time.Second
