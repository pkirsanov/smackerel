//go:build e2e

// BUG-004-004 SCOPE-04 — T004-05-API / T004-07-08-API / T004-09-AUTH.
//
// These exercise the HTTP surface against the live stack. The integration tests
// prove the read MODEL; only these prove the handler is actually wired to it and
// that the auth gate is real. A model can be perfect while the route returns a
// hardcoded shape, and nothing below the transport would notice.

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type synthesisLatestBody struct {
	State  string `json:"state"`
	Output *struct {
		OutputID       string `json:"outputId"`
		Kind           string `json:"kind"`
		InsightCount   int    `json:"insightCount"`
		CitationCount  int    `json:"citationCount"`
		LifecycleState string `json:"lifecycleState"`
		CreatedAt      string `json:"createdAt"`
	} `json:"output"`
}

func synthesisGet(t *testing.T, cfg e2eConfig, path string, authorized bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	if authorized {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp.StatusCode, body
}

// SCN-004-004-05. Never-run must be an explicit state, not an empty success.
// The pre-fix failure mode was a green health field standing in for a system
// that had produced nothing, so an absent or blank state here is the exact
// regression being guarded.
func TestSynthesisAPI_LatestReportsAnExplicitState(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/latest", true)
	if status != http.StatusOK {
		t.Fatalf("GET latest returned %d, want 200; body=%s", status, string(body))
	}

	var parsed synthesisLatestBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode latest: %v; body=%s", err, string(body))
	}
	if parsed.State == "" {
		t.Fatalf("latest returned no state; a caller must never infer never-run from an absent field. body=%s", string(body))
	}

	switch parsed.State {
	case "never-run":
		if parsed.Output != nil {
			t.Fatalf("never-run carried an output; that is a contradiction. body=%s", string(body))
		}
	case "full", "quiet", "partial":
		if parsed.Output == nil {
			t.Fatalf("state %q carried no output. body=%s", parsed.State, string(body))
		}
		if parsed.Output.OutputID == "" {
			t.Fatalf("state %q carried an output with no id. body=%s", parsed.State, string(body))
		}
		if parsed.State == "quiet" && parsed.Output.InsightCount != 0 {
			t.Fatalf("quiet output reported %d insights. body=%s", parsed.Output.InsightCount, string(body))
		}
	default:
		t.Fatalf("latest returned unknown state %q; the vocabulary is closed. body=%s", parsed.State, string(body))
	}
}

// SCN-004-004-07/08. The history listing must serialise as a list, and every
// entry must carry a known kind. A null here would read as a missing field to a
// client rather than as "no runs".
func TestSynthesisAPI_RunsListIsWellFormed(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=5", true)
	if status != http.StatusOK {
		t.Fatalf("GET runs returned %d, want 200; body=%s", status, string(body))
	}

	var parsed struct {
		Runs []struct {
			OutputID string `json:"outputId"`
			Kind     string `json:"kind"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode runs: %v; body=%s", err, string(body))
	}
	if parsed.Runs == nil {
		t.Fatalf("runs serialised as null rather than []; that reads as a missing field. body=%s", string(body))
	}
	if len(parsed.Runs) > 5 {
		t.Fatalf("limit=5 returned %d runs; the bound must be enforced server-side", len(parsed.Runs))
	}
	for i, run := range parsed.Runs {
		if run.OutputID == "" {
			t.Fatalf("run %d has no output id. body=%s", i, string(body))
		}
		switch run.Kind {
		case "full", "quiet", "partial":
		default:
			t.Fatalf("run %d has unknown kind %q; the vocabulary is closed. body=%s", i, run.Kind, string(body))
		}
	}
}

// An invalid limit must be refused rather than silently coerced. Coercion would
// hide a client bug and make the bound untestable from outside.
func TestSynthesisAPI_RejectsInvalidLimit(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=abc", true)
	if status != http.StatusBadRequest {
		t.Fatalf("limit=abc returned %d, want 400; body=%s", status, string(body))
	}
}

// SCN-004-004-09. The routes sit behind the bearer gate. This is the assertion
// that would fail if they were mounted outside it -- which no amount of
// integration testing against the read model could detect.
func TestSynthesisAPI_DeniesUnauthenticatedCallers(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	for _, path := range []string{
		"/api/synthesis/latest",
		"/api/synthesis/runs",
		"/api/synthesis/runs/does-not-exist",
	} {
		status, body := synthesisGet(t, cfg, path, false)
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatalf("unauthenticated GET %s returned %d, want 401 or 403; body=%s", path, status, string(body))
		}
		// The denial must not describe what exists behind it.
		for _, leak := range []string{"outputId", "insightCount", "throughLine"} {
			if len(body) > 0 && containsE2E(string(body), leak) {
				t.Fatalf("unauthenticated denial for %s leaked %q: %s", path, leak, string(body))
			}
		}
	}
}

// An unknown output id must not be answered with a confident success.
func TestSynthesisAPI_UnknownRunIsNotFound(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs/01JZZZZZZZZZZZZZZZZZZZZZZZ", true)
	if status != http.StatusNotFound {
		t.Fatalf("unknown output id returned %d, want 404; body=%s", status, string(body))
	}
	// A bare 404 proves nothing: an unmounted route returns one too, and this
	// test passed against exactly that before the prefix was corrected. The
	// structured code is what distinguishes the handler answering from the
	// router shrugging.
	if !containsE2E(string(body), "synthesis_output_not_found") {
		t.Fatalf("404 body did not carry the handler's error code; the route may not be mounted at all: %s", string(body))
	}
}

func containsE2E(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func synthesisPost(t *testing.T, cfg e2eConfig, path, body string, authorized bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authorized {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp.StatusCode, out
}

// The retry route reports what HAPPENED, not that the request was accepted.
// A 202 would be the same shape of claim the original defect made: true about
// the request, silent about whether anything was stored.
func TestSynthesisAPI_RetryReportsAPersistedOutcome(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, true)
	if status == http.StatusAccepted {
		t.Fatalf("retry returned 202; the route must report the outcome, not merely that it was asked. body=%s", string(body))
	}
	if status != http.StatusOK && status != http.StatusConflict {
		t.Fatalf("retry returned %d, want 200 or 409; body=%s", status, string(body))
	}

	var parsed struct {
		Outcome string `json:"outcome"`
		Output  *struct {
			OutputID string `json:"outputId"`
			Kind     string `json:"kind"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode retry: %v; body=%s", err, string(body))
	}

	switch parsed.Outcome {
	case "persisted":
		if parsed.Output == nil || parsed.Output.OutputID == "" {
			t.Fatalf("outcome persisted carried no output id; that is the claim the defect made. body=%s", string(body))
		}
		// The id must resolve. An id that reads back as absent would mean the
		// route reported a commit that is not there.
		detailStatus, detailBody := synthesisGet(t, cfg, "/api/synthesis/runs/"+parsed.Output.OutputID, true)
		if detailStatus != http.StatusOK {
			t.Fatalf("retry reported output %s but reading it returned %d; a reported commit must be readable. body=%s",
				parsed.Output.OutputID, detailStatus, string(detailBody))
		}
	case "claimed-elsewhere":
		if parsed.Output != nil {
			t.Fatalf("claimed-elsewhere carried an output; this process did not produce one. body=%s", string(body))
		}
	default:
		t.Fatalf("retry returned unknown outcome %q; the vocabulary is closed. body=%s", parsed.Outcome, string(body))
	}
}

// An unrecognised cadence must be refused, never defaulted. Silently running
// daily would produce a real output answering a question nobody asked.
func TestSynthesisAPI_RetryRefusesUnknownCadence(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	for _, body := range []string{`{"cadence":"hourly"}`, `{"cadence":""}`, `{}`, `not json`} {
		status, out := synthesisPost(t, cfg, "/api/synthesis/retry", body, true)
		if status != http.StatusBadRequest {
			t.Fatalf("retry with %s returned %d, want 400; body=%s", body, status, string(out))
		}
	}
}

func TestSynthesisAPI_RetryDeniesUnauthenticatedCallers(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, false)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("unauthenticated retry returned %d, want 401 or 403; body=%s", status, string(body))
	}
}
