//go:build integration

// Spec 080 SCOPE-080-03 — live-stack time tier.
//   - TestGraphAPI_Time_GroupsByDay — SCN-080-07.
//   - TestGraphAPI_Time_WindowTooLarge — SCN-080-12.
//   - TestGraphAPI_Time_MissingTo — SCN-080-13.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type timeArtifact struct {
	ArtifactID string    `json:"artifactId"`
	Title      string    `json:"title"`
	CapturedAt time.Time `json:"capturedAt"`
}
type timeDay struct {
	Date      string         `json:"date"`
	Artifacts []timeArtifact `json:"artifacts"`
}
type timeBody struct {
	Days []timeDay `json:"days"`
}

func TestGraphAPI_Time_GroupsByDay(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)
	conn := connectDB(t, cfg)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := fixturePrefix(t)
	t.Cleanup(func() { cleanupFixtures(t, conn, prefix) })

	// Seed 2 artifacts inside a tight 2-day window with deterministic
	// timestamps so the response contains identifiable rows.
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	mkArtifact := func(id string, ts time.Time) {
		_, err := conn.Exec(context.Background(),
			`INSERT INTO artifacts (id, artifact_type, title, content_hash,
			    source_id, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			id, "note", id+"-title", id+"-hash", "graphapi-it-seed", ts)
		if err != nil {
			t.Fatalf("seed artifact %s: %v", id, err)
		}
	}
	mkArtifact(prefix+"-time-a0", from.Add(2*time.Hour))
	mkArtifact(prefix+"-time-a1", from.Add(28*time.Hour))

	path := "/api/time?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	resp, body := doAuthedGET(t, cfg, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	var got timeBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(body))
	}
	// Adversarial: every artifact's capturedAt MUST be within the window.
	for _, d := range got.Days {
		for _, a := range d.Artifacts {
			if a.CapturedAt.Before(from) || !a.CapturedAt.Before(to) {
				t.Fatalf("artifact %s capturedAt=%s outside [from=%s, to=%s)",
					a.ArtifactID, a.CapturedAt, from, to)
			}
		}
	}
	// Look for at least the two seeded ids.
	want := map[string]bool{prefix + "-time-a0": false, prefix + "-time-a1": false}
	for _, d := range got.Days {
		for _, a := range d.Artifacts {
			if _, ok := want[a.ArtifactID]; ok {
				want[a.ArtifactID] = true
			}
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("seeded artifact %s missing from window response", id)
		}
	}
}

func TestGraphAPI_Time_WindowTooLarge(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)

	// > 365 days; SCN-080-12 requires invalid_window 400.
	path := "/api/time?from=2024-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"
	resp, body := doAuthedGET(t, cfg, path)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; want 400", resp.StatusCode, string(body))
	}
	env := decodeError(t, body)
	if env.Error.Code != "invalid_window" {
		t.Fatalf("error.code=%q; want invalid_window; body=%s",
			env.Error.Code, string(body))
	}
}

func TestGraphAPI_Time_MissingTo(t *testing.T) {
	cfg := loadLive(t)
	waitHealthy(t, cfg, 30*time.Second)

	resp, body := doAuthedGET(t, cfg, "/api/time?from=2026-05-01T00:00:00Z")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; want 400", resp.StatusCode, string(body))
	}
	env := decodeError(t, body)
	if env.Error.Code != "missing_param" {
		t.Fatalf("error.code=%q; want missing_param; body=%s",
			env.Error.Code, string(body))
	}
	if env.Error.Field != "to" {
		t.Fatalf("error.field=%q; want 'to'; body=%s",
			env.Error.Field, string(body))
	}
}
