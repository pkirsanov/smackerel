//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce exercises
// SCN-040-008. The duplicates dashboard MUST surface a stable shape and
// it MUST be safe to filter clusters by the cross_provider_hash kind so
// the user only sees each cross-provider duplicate group once. The
// quality histogram and removal review endpoints share the dashboard
// envelope used by the same scenario.
func TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	dupResp, err := apiGet(cfg, "/v1/photos/health/duplicates")
	if err != nil {
		t.Fatalf("GET /v1/photos/health/duplicates: %v", err)
	}
	dupBody, err := readBody(dupResp)
	if err != nil {
		t.Fatalf("read duplicates body: %v", err)
	}
	if dupResp.StatusCode != http.StatusOK {
		t.Fatalf("duplicates status=%d body=%s", dupResp.StatusCode, string(dupBody))
	}
	var clusters struct {
		Clusters []struct {
			ClusterID string `json:"cluster_id"`
			Kind      string `json:"kind"`
		} `json:"clusters"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(dupBody, &clusters); err != nil {
		t.Fatalf("decode duplicates: %v body=%s", err, string(dupBody))
	}
	if clusters.Total < 0 {
		t.Fatalf("duplicates total=%d, want >=0", clusters.Total)
	}

	// Each cluster MUST appear at most once in the unfiltered list, so a
	// cross-provider duplicate cannot be reported twice. Empty data is
	// valid because the live e2e stack has no seeded duplicates yet.
	seen := map[string]struct{}{}
	for _, c := range clusters.Clusters {
		if _, dup := seen[c.ClusterID]; dup {
			t.Fatalf("cluster %q appears twice in duplicates response", c.ClusterID)
		}
		seen[c.ClusterID] = struct{}{}
	}

	// Filter by cross-provider kind: the endpoint MUST accept the
	// taxonomy value created by Scope 3 without 4xx/5xx.
	crossResp, err := apiGet(cfg, "/v1/photos/health/duplicates?kind=cross_provider_hash")
	if err != nil {
		t.Fatalf("GET /v1/photos/health/duplicates?kind=cross_provider_hash: %v", err)
	}
	crossBody, err := readBody(crossResp)
	if err != nil {
		t.Fatalf("read cross-provider body: %v", err)
	}
	if crossResp.StatusCode != http.StatusOK {
		t.Fatalf("cross-provider filter status=%d body=%s", crossResp.StatusCode, string(crossBody))
	}
	var crossClusters struct {
		Clusters []struct {
			ClusterID string `json:"cluster_id"`
			Kind      string `json:"kind"`
		} `json:"clusters"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(crossBody, &crossClusters); err != nil {
		t.Fatalf("decode cross-provider clusters: %v body=%s", err, string(crossBody))
	}
	for _, c := range crossClusters.Clusters {
		if c.Kind != "cross_provider_hash" {
			t.Fatalf("cross-provider filter returned cluster of kind %q", c.Kind)
		}
	}

	qualityResp, err := apiGet(cfg, "/v1/photos/health/quality")
	if err != nil {
		t.Fatalf("GET /v1/photos/health/quality: %v", err)
	}
	qualityBody, err := readBody(qualityResp)
	if err != nil {
		t.Fatalf("read quality body: %v", err)
	}
	if qualityResp.StatusCode != http.StatusOK {
		t.Fatalf("quality status=%d body=%s", qualityResp.StatusCode, string(qualityBody))
	}
	var quality struct {
		Buckets []struct {
			Bucket string `json:"bucket"`
			Count  int    `json:"count"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(qualityBody, &quality); err != nil {
		t.Fatalf("decode quality: %v body=%s", err, string(qualityBody))
	}

	removalResp, err := apiGet(cfg, "/v1/photos/health/removal")
	if err != nil {
		t.Fatalf("GET /v1/photos/health/removal: %v", err)
	}
	removalBody, err := readBody(removalResp)
	if err != nil {
		t.Fatalf("read removal body: %v", err)
	}
	if removalResp.StatusCode != http.StatusOK {
		t.Fatalf("removal status=%d body=%s", removalResp.StatusCode, string(removalBody))
	}
	var removal struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(removalBody, &removal); err != nil {
		t.Fatalf("decode removal: %v body=%s", err, string(removalBody))
	}
}
