//go:build integration

// Spec 080 / BUG-080-001 SCOPE-02 — T080-03-PG (SCN-080-001-03).
//
// Authorized FIVE-family read journey against the LIVE disposable
// stack. Real PostgreSQL rows are seeded under a unique fixture prefix
// and read back through the real authorized HTTP capability
// (`Authorization: Bearer <SMACKEREL_AUTH_TOKEN>` against
// CORE_EXTERNAL_URL) for every graph family — /api/topics,
// /api/people, /api/places, /api/time, /api/graph/edges — and each
// response is asserted to be contract-valid AND to contain the seeded
// rows. No mocks, no request interception, no in-process stub: the
// bytes asserted here were produced by the live handlers reading the
// live database.
//
// The DoD additionally requires the journey to be provably READ-ONLY:
// "graph-table write counts are unchanged before and after the
// journey". Row counts for every graph write table are captured AFTER
// seeding / BEFORE the first read, and again AFTER the last read; any
// delta fails the test. A regression that made a read path INSERT —
// lazily materializing a place row, backfilling a missing edge, or
// upserting a topic during cross-link enrichment — is therefore caught
// rather than silently tolerated.

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

// graphWriteTables is the closed set of tables the knowledge-graph read
// paths project over. There is deliberately no `places` entry: this
// schema has no first-class places table — place rows are DERIVED from
// location_clusters (maps source) and artifacts.location_geo (artifact
// source), and both of those ARE counted here.
var graphWriteTables = []string{"artifacts", "people", "topics", "edges", "location_clusters"}

// graphTableCounts snapshots the authoritative row count of every graph
// write table. Counts (not timestamps) are the read-only proof metric
// required by SCN-080-001-03.
func graphTableCounts(t *testing.T, conn *pgx.Conn) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(graphWriteTables))
	for _, table := range graphWriteTables {
		var n int64
		// Table names come from the package-local closed list above,
		// never from request input, so the concatenation is safe.
		if err := conn.QueryRow(context.Background(),
			`SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count rows in %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// assertGraphCountsUnchanged fails loud on ANY graph-table row-count
// delta observed across the read journey.
func assertGraphCountsUnchanged(t *testing.T, before, after map[string]int64) {
	t.Helper()
	for _, table := range graphWriteTables {
		if before[table] != after[table] {
			t.Fatalf("read-only violation: %s row count changed across the authorized read journey: before=%d after=%d delta=%+d",
				table, before[table], after[table], after[table]-before[table])
		}
	}
}

// findSeededPlace walks /api/places (following the opaque cursor) until
// the seeded artifact-derived place surfaces. Place ids are
// server-computed, so the seeded row is located by its unique
// displayName. Fails loud when the place never appears.
func findSeededPlace(t *testing.T, cfg liveCfg, wantDisplayName string) placeRow {
	t.Helper()
	cursor := ""
	seenIDs := map[string]bool{}
	for page := 0; page < 10; page++ {
		path := "/api/places?limit=200"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		resp, body := doAuthedGET(t, cfg, path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s; want 200", path, resp.StatusCode, string(body))
		}
		var got placesListBody
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode places page %d: %v body=%s", page, err, string(body))
		}
		for _, it := range got.Items {
			if it.ID == "" || it.DisplayName == "" {
				t.Fatalf("places item missing id/displayName: %+v", it)
			}
			if seenIDs[it.ID] {
				t.Fatalf("duplicate place id %q across /api/places pages", it.ID)
			}
			seenIDs[it.ID] = true
			if it.DisplayName == wantDisplayName {
				return it
			}
		}
		if got.NextCursor == "" {
			break
		}
		cursor = got.NextCursor
	}
	t.Fatalf("seeded place %q missing from /api/places after scanning %d place ids", wantDisplayName, len(seenIDs))
	return placeRow{}
}

// TestGraphFamiliesReadSeededPostgresThroughAuthorizedCapability is
// T080-03-PG: all five graph families read seeded PostgreSQL rows
// through the authorized production HTTP path, and the journey writes
// nothing.
func TestGraphFamiliesReadSeededPostgresThroughAuthorizedCapability(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	defer cleanupFixtures(t, conn, prefix)

	// ---- Disposable fixtures, all tagged with the unique prefix ----
	topicIDs := seedTopics(t, conn, prefix, 3)
	peopleIDs := seedPeople(t, conn, prefix, 2)
	// seedArtifacts staggers created_at by 24h ascending, ending one
	// day before now, so the /api/time window below contains all of
	// them.
	artIDs := seedArtifacts(t, conn, prefix, 3)
	placeName := prefix + "-family-place"
	placeArtifactID := seedArtifactWithLocation(t, conn, prefix, placeName)

	// Edges FROM the first seeded artifact TO one of every downstream
	// kind, so /api/graph/edges has a populated source to project.
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "topic", topicIDs[0], "mentions", 3.0)
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "person", peopleIDs[0], "mentions", 2.0)
	seedEdge(t, conn, prefix, "artifact", artIDs[0], "place", prefix+"-edge-place-0", "mentions", 1.0)
	// Topic/person cross-links so the list projections carry real
	// linked-artifact counts rather than zeros.
	for i, tid := range topicIDs {
		seedEdge(t, conn, prefix, "topic", tid, "artifact", artIDs[i], "mentions", 1.0)
	}
	for i, pid := range peopleIDs {
		seedEdge(t, conn, prefix, "artifact", artIDs[i], "person", pid, "mentions", 1.0)
	}

	// ---- Read-only baseline: AFTER seeding, BEFORE the first read ----
	countsBefore := graphTableCounts(t, conn)

	// ---- Family 1/5: /api/topics ----
	topicsResp, topicsBody := doAuthedGET(t, cfg, "/api/topics?limit=200")
	if topicsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/topics status=%d body=%s; want 200", topicsResp.StatusCode, string(topicsBody))
	}
	var topics topicsListBody
	if err := json.Unmarshal(topicsBody, &topics); err != nil {
		t.Fatalf("decode /api/topics: %v body=%s", err, string(topicsBody))
	}
	seenTopics := map[string]bool{}
	for _, it := range topics.Items {
		if it.ID == "" || it.Label == "" {
			t.Fatalf("/api/topics item missing id/label: %+v", it)
		}
		seenTopics[it.ID] = true
	}
	for _, want := range topicIDs {
		if !seenTopics[want] {
			t.Fatalf("seeded topic %s missing from /api/topics (%d items returned)", want, len(topics.Items))
		}
	}

	// ---- Family 2/5: /api/people ----
	peopleResp, peopleBody := doAuthedGET(t, cfg, "/api/people?limit=200")
	if peopleResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/people status=%d body=%s; want 200", peopleResp.StatusCode, string(peopleBody))
	}
	var people personListBody
	if err := json.Unmarshal(peopleBody, &people); err != nil {
		t.Fatalf("decode /api/people: %v body=%s", err, string(peopleBody))
	}
	seenPeople := map[string]bool{}
	for _, it := range people.Items {
		if it.ID == "" || it.DisplayName == "" {
			t.Fatalf("/api/people item missing id/displayName: %+v", it)
		}
		seenPeople[it.ID] = true
	}
	for _, want := range peopleIDs {
		if !seenPeople[want] {
			t.Fatalf("seeded person %s missing from /api/people (%d items returned)", want, len(people.Items))
		}
	}

	// ---- Family 3/5: /api/places ----
	place := findSeededPlace(t, cfg, placeName)
	// The artifact-derived source MUST be labelled 'ar:' — a regression
	// that dropped the artifacts.location_geo source (or mislabelled
	// it) fails here instead of silently returning a maps-only list.
	if !strings.HasPrefix(place.ID, "ar:") {
		t.Fatalf("/api/places returned seeded place %q with id=%q; want an artifact-derived 'ar:' id (artifact %s)",
			placeName, place.ID, placeArtifactID)
	}
	if place.Source == "" {
		t.Fatalf("/api/places item %+v has empty source; contract requires the merge source", place)
	}

	// ---- Family 4/5: /api/time ----
	// Window covers every seeded artifact (oldest is now-72h, the
	// location artifact is NOW) and is far inside the configured
	// maximum window.
	from := time.Now().UTC().Add(-96 * time.Hour).Truncate(time.Second)
	to := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second)
	timePath := "/api/time?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	timeResp, timeRespBody := doAuthedGET(t, cfg, timePath)
	if timeResp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s; want 200", timePath, timeResp.StatusCode, string(timeRespBody))
	}
	var days timeBody
	if err := json.Unmarshal(timeRespBody, &days); err != nil {
		t.Fatalf("decode /api/time: %v body=%s", err, string(timeRespBody))
	}
	wantArtifacts := map[string]bool{placeArtifactID: false}
	for _, id := range artIDs {
		wantArtifacts[id] = false
	}
	for _, d := range days.Days {
		if d.Date == "" {
			t.Fatalf("/api/time day bucket missing date: %+v", d)
		}
		for _, a := range d.Artifacts {
			if a.ArtifactID == "" || a.Title == "" {
				t.Fatalf("/api/time artifact missing artifactId/title: %+v", a)
			}
			// Window contract is inclusive-start, exclusive-end.
			if a.CapturedAt.Before(from) || !a.CapturedAt.Before(to) {
				t.Fatalf("/api/time artifact %s capturedAt=%s outside [from=%s, to=%s)",
					a.ArtifactID, a.CapturedAt, from, to)
			}
			if _, ok := wantArtifacts[a.ArtifactID]; ok {
				wantArtifacts[a.ArtifactID] = true
			}
		}
	}
	for id, found := range wantArtifacts {
		if !found {
			t.Fatalf("seeded artifact %s missing from /api/time window [%s, %s)", id, from, to)
		}
	}

	// ---- Family 5/5: /api/graph/edges ----
	edgesPath := "/api/graph/edges?source=artifact:" + artIDs[0] + "&limit=200"
	edgesResp, edgesRespBody := doAuthedGET(t, cfg, edgesPath)
	if edgesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s; want 200", edgesPath, edgesResp.StatusCode, string(edgesRespBody))
	}
	var edges edgesListBody
	if err := json.Unmarshal(edgesRespBody, &edges); err != nil {
		t.Fatalf("decode /api/graph/edges: %v body=%s", err, string(edgesRespBody))
	}
	if len(edges.Items) < 3 {
		t.Fatalf("/api/graph/edges returned %d items for artifact:%s; want >=3 (one per downstream kind)",
			len(edges.Items), artIDs[0])
	}
	edgeKinds := map[string]bool{}
	for _, it := range edges.Items {
		if it.TargetKind == "" || it.TargetID == "" || it.TargetLabel == "" || it.Reason == "" {
			t.Fatalf("/api/graph/edges item missing cross-link field: %+v", it)
		}
		edgeKinds[it.TargetKind] = true
	}
	for _, want := range []string{"topic", "person", "place"} {
		if !edgeKinds[want] {
			t.Fatalf("/api/graph/edges missing targetKind=%q for artifact:%s (kinds seen: %v)",
				want, artIDs[0], edgeKinds)
		}
	}

	// ---- Read-only proof: nothing was written by the journey ----
	countsAfter := graphTableCounts(t, conn)
	assertGraphCountsUnchanged(t, countsBefore, countsAfter)
}
