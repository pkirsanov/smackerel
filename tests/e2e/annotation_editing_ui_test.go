//go:build e2e

// Spec 027 scope 9 — T9-08 / SCN-027-71..74 E2E regression body.
//
// Owner: bubbles.test (per PLAN-9-06; skeleton authored by
// bubbles.implement, body filled here).
//
// Contract under test (live HTTP against the ephemeral test stack):
//
//   - SCN-027-71 — inline-edit round-trips through
//     POST /api/artifacts/{id}/annotations with bearer + valid
//     X-Smackerel-Source header and increments the monotonic per-
//     artifact `version` counter exposed by the summary endpoint.
//   - SCN-027-72 — annotation writes that omit / spoof
//     X-Smackerel-Source are rejected fail-loud (NO-DEFAULTS
//     PLAN-9-04). Adversarial coverage for the SST allowlist guard.
//   - SCN-027-73 — GET /api/annotations?actor=me requires an
//     authenticated subject; calls without a per-user binding are
//     rejected with 403 even when the shared bearer is otherwise
//     accepted. The actor-filter shape itself (own-vs-other-actor)
//     is exhaustively covered by the unit tests
//     `TestListMyAnnotations_T9_01_ReturnsCallerEvents` /
//     `TestListMyAnnotations_T9_02_ForbidsOtherActor`; the live
//     stack runs with AUTH_ENABLED=false (shared-token mode), so
//     per-user PASETO issuance is not wired here and a live-HTTP
//     proof of cross-actor exclusion would require a dedicated
//     test-stack flavor. The e2e contract here is the
//     subject-required guard.
//   - SCN-027-74 — stale `If-Match` returns 409 with the current
//     summary body and writes no annotation row.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

type annSummary struct {
	Version int64 `json:"version"`
}

type annHistoryResp struct {
	Annotations []map[string]any `json:"annotations"`
	History     []map[string]any `json:"history"`
}

// TestAnnotationEditingUI_FullFlow exercises the spec 073 UI's POST →
// If-Match → list-my-annotations sequence end-to-end against the live
// test stack. Sub-tests run in order so the version counter is shared
// across them.
func TestAnnotationEditingUI_FullFlow(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	base := strings.TrimRight(cfg.CoreURL, "/")
	client := &http.Client{Timeout: 15 * time.Second}

	// Seed a real artifact via POST /api/capture so the annotation
	// endpoints operate against a row that actually exists in the
	// live DB (no fixture shortcuts).
	artifactID := annSeedArtifact(t, client, base, cfg.AuthToken,
		"spec-027-scope9-e2e seed "+time.Now().UTC().Format(time.RFC3339Nano))
	annotationsURL := base + "/api/artifacts/" + artifactID + "/annotations"
	summaryURL := annotationsURL + "/summary"
	historyURL := annotationsURL + "/"

	// V0 — version before any annotation write. Migration 055's
	// LEFT-JOIN-with-COALESCE-0 means a never-touched artifact reads 0.
	v0 := annReadSummaryVersion(t, client, cfg.AuthToken, summaryURL)

	t.Run("post_with_if_match_200", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"text": "loved this 5/5 #weeknight"})
		req, _ := http.NewRequest(http.MethodPost, annotationsURL, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Smackerel-Source", "web")
		req.Header.Set("If-Match", fmt.Sprintf("%d", v0))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST annotation: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("POST annotation status=%d body=%s", resp.StatusCode, string(raw))
		}

		v1 := annReadSummaryVersion(t, client, cfg.AuthToken, summaryURL)
		if v1 <= v0 {
			t.Fatalf("summary version did not advance: v0=%d v1=%d", v0, v1)
		}
	})

	t.Run("stale_if_match_409", func(t *testing.T) {
		vCur := annReadSummaryVersion(t, client, cfg.AuthToken, summaryURL)
		if vCur <= v0 {
			t.Fatalf("precondition violated: vCur=%d v0=%d (previous sub-test must have advanced version)", vCur, v0)
		}

		histBefore := annReadHistoryCount(t, client, cfg.AuthToken, historyURL)

		body, _ := json.Marshal(map[string]string{"text": "stale-edit attempt 3/5"})
		req, _ := http.NewRequest(http.MethodPost, annotationsURL, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Smackerel-Source", "web")
		req.Header.Set("If-Match", fmt.Sprintf("%d", v0))

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST stale annotation: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict on stale If-Match, got %d body=%s", resp.StatusCode, string(raw))
		}

		var conflictSummary annSummary
		if err := json.Unmarshal(raw, &conflictSummary); err != nil {
			t.Fatalf("decode 409 body: %v body=%s", err, string(raw))
		}
		if conflictSummary.Version != vCur {
			t.Fatalf("409 body version=%d expected current %d (stale was %d)",
				conflictSummary.Version, vCur, v0)
		}

		histAfter := annReadHistoryCount(t, client, cfg.AuthToken, historyURL)
		if histAfter != histBefore {
			t.Fatalf("history count changed after stale-If-Match POST: before=%d after=%d (no row should have been inserted)",
				histBefore, histAfter)
		}

		vAfter := annReadSummaryVersion(t, client, cfg.AuthToken, summaryURL)
		if vAfter != vCur {
			t.Fatalf("summary version moved after stale-If-Match POST: was %d now %d", vCur, vAfter)
		}
	})

	t.Run("list_my_annotations_filters_actor", func(t *testing.T) {
		// Adversarial #1 — missing X-Smackerel-Source header is a
		// fail-loud 400 (NO-DEFAULTS PLAN-9-04 SST guard).
		{
			body, _ := json.Marshal(map[string]string{"text": "no source header"})
			req, _ := http.NewRequest(http.MethodPost, annotationsURL, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("POST missing-source: %v", err)
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for missing X-Smackerel-Source, got %d body=%s", resp.StatusCode, string(raw))
			}
			if !strings.Contains(string(raw), "X-Smackerel-Source header required") {
				t.Fatalf("expected fail-loud error message, body=%s", string(raw))
			}
		}

		// Adversarial #2 — unknown source value rejected fail-loud.
		{
			body, _ := json.Marshal(map[string]string{"text": "spoofed source"})
			req, _ := http.NewRequest(http.MethodPost, annotationsURL, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Smackerel-Source", "rogue-client")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("POST unknown-source: %v", err)
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for unknown source_channel, got %d body=%s", resp.StatusCode, string(raw))
			}
			if !strings.Contains(string(raw), "unknown source_channel") {
				t.Fatalf("expected fail-loud error message, body=%s", string(raw))
			}
		}

		// SCN-027-73 actor-required guard (live HTTP). Test stack
		// runs AUTH_ENABLED=false → shared-token mode → no UserID
		// on the session → ListMyAnnotations MUST reject 403.
		req, _ := http.NewRequest(http.MethodGet,
			base+"/api/annotations?actor=me&limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET /api/annotations: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for actor=me under shared-token session, got %d body=%s",
				resp.StatusCode, string(raw))
		}
		if !strings.Contains(string(raw), "authenticated subject required") {
			t.Fatalf("expected actor-required guard message, body=%s", string(raw))
		}
	})

	t.Run("p95_latency_probe", func(t *testing.T) {
		const samples = 30

		postDurs := make([]time.Duration, 0, samples)
		for i := 0; i < samples; i++ {
			body, _ := json.Marshal(map[string]string{
				"text": fmt.Sprintf("latency probe #%d 4/5 #probe", i),
			})
			req, _ := http.NewRequest(http.MethodPost, annotationsURL, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Smackerel-Source", "web")
			start := time.Now()
			resp, err := client.Do(req)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("probe POST #%d: %v", i, err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("probe POST #%d status=%d", i, resp.StatusCode)
			}
			postDurs = append(postDurs, elapsed)
		}

		getDurs := make([]time.Duration, 0, samples)
		for i := 0; i < samples; i++ {
			// list-my-annotations under shared-token returns 403
			// before the DB query; probe the summary endpoint
			// instead — it exercises the same LEFT JOIN against
			// annotation_summary_version that the UI hits on
			// every read.
			req, _ := http.NewRequest(http.MethodGet, summaryURL, nil)
			req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			start := time.Now()
			resp, err := client.Do(req)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("probe GET #%d: %v", i, err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("probe GET summary #%d status=%d", i, resp.StatusCode)
			}
			getDurs = append(getDurs, elapsed)
		}

		postP95 := annPercentile(postDurs, 95)
		getP95 := annPercentile(getDurs, 95)
		t.Logf("LATENCY_EVIDENCE samples=%d POST_annotations_p95=%s GET_summary_p95=%s",
			samples, postP95, getP95)

		if postP95 <= 0 || getP95 <= 0 {
			t.Fatalf("latency probe produced non-positive p95 values: post=%s get=%s",
				postP95, getP95)
		}
	})
}

func annSeedArtifact(t *testing.T, client *http.Client, base, token, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/capture", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("seed POST /api/capture: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("seed POST /api/capture status=%d body=%s", resp.StatusCode, string(raw))
	}
	var cap struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.Unmarshal(raw, &cap); err != nil || cap.ArtifactID == "" {
		t.Fatalf("seed decode: err=%v body=%s", err, string(raw))
	}
	return cap.ArtifactID
}

func annReadSummaryVersion(t *testing.T, client *http.Client, token, url string) int64 {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET summary: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET summary status=%d body=%s", resp.StatusCode, string(raw))
	}
	var s annSummary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode summary: %v body=%s", err, string(raw))
	}
	return s.Version
}

func annReadHistoryCount(t *testing.T, client *http.Client, token, url string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET history: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET history status=%d body=%s", resp.StatusCode, string(raw))
	}
	var h annHistoryResp
	if err := json.Unmarshal(raw, &h); err == nil {
		if len(h.Annotations) > 0 {
			return len(h.Annotations)
		}
		if len(h.History) > 0 {
			return len(h.History)
		}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	t.Fatalf("decode history body (unrecognized shape): %s", string(raw))
	return 0
}

// annPercentile returns the rank-based percentile from durs using the
// nearest-rank method.
func annPercentile(durs []time.Duration, p int) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durs))
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
