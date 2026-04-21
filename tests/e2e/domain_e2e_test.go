//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Scenario: Domain extraction E2E
// Given a recipe URL is captured
// When processing and domain extraction complete
// Then the artifact has domain_data with ingredients and steps
// And searching "recipes with [ingredient]" returns the artifact
func TestE2E_DomainExtraction(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Step 1: Capture a text artifact with recipe-like content
	uniqueMarker := fmt.Sprintf("e2e-domain-%d", time.Now().UnixNano())
	captureBody := map[string]string{
		"text": fmt.Sprintf(
			"Classic Margherita Pizza Recipe. Ingredients: 500g pizza dough, "+
				"200g San Marzano tomatoes, 200g fresh mozzarella, fresh basil leaves, "+
				"2 tbsp extra virgin olive oil, salt to taste. Instructions: "+
				"1. Preheat oven to 250C. 2. Roll out dough into a circle. "+
				"3. Spread crushed tomatoes evenly. 4. Tear mozzarella and distribute. "+
				"5. Bake for 10-12 minutes. 6. Top with fresh basil and olive oil. "+
				"Unique marker: %s", uniqueMarker),
		"context": "e2e domain extraction test — recipe content",
	}
	bodyBytes, _ := json.Marshal(captureBody)

	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/capture", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("create capture request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("capture request failed: %v", err)
	}
	captureRespBody, err := readBody(resp)
	if err != nil {
		t.Fatalf("read capture response: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 200/201 from capture, got %d: %s", resp.StatusCode, string(captureRespBody))
	}

	var captureResp struct {
		ArtifactID   string `json:"artifact_id"`
		Title        string `json:"title"`
		ArtifactType string `json:"artifact_type"`
	}
	if err := json.Unmarshal(captureRespBody, &captureResp); err != nil {
		t.Fatalf("parse capture response: %v", err)
	}
	if captureResp.ArtifactID == "" {
		t.Fatal("capture response missing artifact_id")
	}
	t.Logf("captured recipe artifact: id=%s title=%q", captureResp.ArtifactID, captureResp.Title)

	// Step 2: Wait for processing AND domain extraction to complete
	deadline := time.Now().Add(90 * time.Second) // domain extraction takes longer
	var domainStatus string
	processed := false

	for time.Now().Before(deadline) {
		detailResp, err := apiGet(cfg, "/api/artifact/"+captureResp.ArtifactID)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		detailBody, err := readBody(detailResp)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if detailResp.StatusCode != http.StatusOK {
			time.Sleep(2 * time.Second)
			continue
		}

		var detail struct {
			ProcessingStatus       string          `json:"processing_status"`
			DomainExtractionStatus string          `json:"domain_extraction_status"`
			DomainData             json.RawMessage `json:"domain_data"`
		}
		if err := json.Unmarshal(detailBody, &detail); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		domainStatus = detail.DomainExtractionStatus

		// Check if both processing and domain extraction are complete
		isProcessed := detail.ProcessingStatus == "processed" || detail.ProcessingStatus == "completed"
		isDomainDone := detail.DomainExtractionStatus == "completed"

		if isProcessed && isDomainDone && len(detail.DomainData) > 2 {
			processed = true
			t.Logf("artifact processed with domain_data: processing=%s domain=%s",
				detail.ProcessingStatus, detail.DomainExtractionStatus)
			break
		}
		t.Logf("waiting for domain extraction... processing=%s domain=%s",
			detail.ProcessingStatus, detail.DomainExtractionStatus)
		time.Sleep(3 * time.Second)
	}

	if !processed {
		t.Fatalf("domain extraction not completed within 90s timeout — last domain_status=%s (pipeline or ML sidecar may not support domain extraction)", domainStatus)
	}

	// Step 3: Verify domain_data has expected structure
	detailResp, err := apiGet(cfg, "/api/artifact/"+captureResp.ArtifactID)
	if err != nil {
		t.Fatalf("fetch final artifact detail: %v", err)
	}
	detailBody, err := readBody(detailResp)
	if err != nil {
		t.Fatalf("read final detail: %v", err)
	}

	var finalDetail struct {
		DomainData json.RawMessage `json:"domain_data"`
	}
	if err := json.Unmarshal(detailBody, &finalDetail); err != nil {
		t.Fatalf("parse final detail: %v", err)
	}

	var domainData map[string]interface{}
	if err := json.Unmarshal(finalDetail.DomainData, &domainData); err != nil {
		t.Fatalf("parse domain_data: %v", err)
	}

	// Verify domain_data has ingredients or steps
	hasIngredients := domainData["ingredients"] != nil
	hasSteps := domainData["steps"] != nil
	if !hasIngredients && !hasSteps {
		t.Errorf("domain_data missing both ingredients and steps: %v", domainData)
	}
	t.Logf("domain_data keys: %v", keysOf(domainData))

	// Step 4: Search for the recipe by ingredient-related query
	searchBody := map[string]interface{}{
		"query": "pizza recipe with mozzarella",
		"limit": 10,
	}
	searchBytes, _ := json.Marshal(searchBody)
	searchReq, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/search", bytes.NewReader(searchBytes))
	if err != nil {
		t.Fatalf("create search request: %v", err)
	}
	searchReq.Header.Set("Content-Type", "application/json")
	searchReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	searchResp, err := client.Do(searchReq)
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	searchRespBody, err := readBody(searchResp)
	if err != nil {
		t.Fatalf("read search response: %v", err)
	}

	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from search, got %d: %s", searchResp.StatusCode, string(searchRespBody))
	}

	var searchResult struct {
		Results []struct {
			ArtifactID string `json:"artifact_id"`
			Title      string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal(searchRespBody, &searchResult); err != nil {
		t.Fatalf("parse search response: %v", err)
	}

	found := false
	for _, r := range searchResult.Results {
		if r.ArtifactID == captureResp.ArtifactID {
			found = true
			t.Logf("found domain-extracted artifact in search results: %s", r.Title)
			break
		}
	}
	if !found {
		t.Error("domain-extracted recipe artifact not found in search results")
	}

	t.Logf("e2e domain extraction test completed, artifact_id=%s", captureResp.ArtifactID)
}

// keysOf returns the top-level keys of a map.
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
