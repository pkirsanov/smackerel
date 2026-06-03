//go:build e2e

// Spec 076 SCOPE-4b — TP-076-04b-04.
//
// Regression E2E for the SCOPE-4b dual-write shadow comparator.
//
// Walk against the live test stack (same fixture as the SCOPE-4a
// sweep at nl_facade_routing_e2e_test.go):
//
//  1. Snapshot `smackerel_annotation_classifier_shadow_calls_total`
//     for `channel="api"` via the live `/metrics` endpoint.
//  2. Seed one deterministic text artifact via `POST /api/capture`.
//  3. POST an annotation with text "made it" against
//     `/api/artifacts/{id}/annotations` and assert HTTP 201 (the
//     PRIMARY inline interactionMap path remains unchanged).
//  4. Re-scrape `/metrics` and assert the counter for `channel="api"`
//     advanced — proving the SCOPE-4b dual-write comparator fired
//     in-line with the annotation request. The outcome label may be
//     "match", "divergence", "shadow_below_floor", or "shadow_error"
//     depending on the live LLM's view; the regression contract is
//     that the comparator EMITS, not that the LLM agrees.
package assistant_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAnnotationClassifierWithShadowComparator(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	waitAssistantFacadeReady(t, stack, 90*time.Second)

	client := &http.Client{Timeout: 60 * time.Second}

	beforeTotal := scrape076ShadowCallsTotal(t, stack, client, "api")

	marker := "e2e-076-04b-shadow-" + time.Now().UTC().Format("20060102T150405.000")
	capBody, _ := json.Marshal(map[string]string{"text": "smackerel-076-04b shadow comparator e2e fixture " + marker})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	capReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/capture", bytes.NewReader(capBody))
	capReq.Header.Set("Content-Type", "application/json")
	capReq.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	capResp, err := client.Do(capReq)
	if err != nil {
		t.Fatalf("POST /api/capture: %v", err)
	}
	capRaw, _ := io.ReadAll(capResp.Body)
	_ = capResp.Body.Close()
	if capResp.StatusCode != http.StatusOK && capResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/capture status = %d body=%s", capResp.StatusCode, string(capRaw))
	}
	var captureResp struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.Unmarshal(capRaw, &captureResp); err != nil || captureResp.ArtifactID == "" {
		t.Fatalf("decode capture response: err=%v body=%s", err, string(capRaw))
	}

	annotateBody, _ := json.Marshal(map[string]string{"text": "made it"})
	annReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		stack.BaseURL+"/api/artifacts/"+captureResp.ArtifactID+"/annotations",
		bytes.NewReader(annotateBody))
	annReq.Header.Set("Content-Type", "application/json")
	annReq.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	annResp, err := client.Do(annReq)
	if err != nil {
		t.Fatalf("POST annotation: %v", err)
	}
	annRaw, _ := io.ReadAll(annResp.Body)
	_ = annResp.Body.Close()
	if annResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST annotation status = %d, body=%s", annResp.StatusCode, string(annRaw))
	}

	afterTotal := scrape076ShadowCallsTotal(t, stack, client, "api")
	if afterTotal <= beforeTotal {
		t.Fatalf("shadow_calls_total for channel=api did not advance; before=%v after=%v — SCOPE-4b dual-write comparator did not fire", beforeTotal, afterTotal)
	}
}

func scrape076ShadowCallsTotal(t *testing.T, stack httpTurnLiveStack, client *http.Client, channel string) float64 {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, stack.BaseURL+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d", resp.StatusCode)
	}
	prefix := "smackerel_annotation_classifier_shadow_calls_total{"
	wantChannel := fmt.Sprintf(`channel="%s"`, channel)
	var total float64
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, prefix) || !strings.Contains(line, wantChannel) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		total += v
	}
	return total
}
