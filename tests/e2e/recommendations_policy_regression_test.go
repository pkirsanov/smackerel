//go:build e2e

// SCN-039-040 / BS-023 regression: an adversarial sponsored candidate carries
// a higher provider_score than the organic candidates in the same fixture
// bundle, so the ONLY way it could outrank them is if the engine applied a
// sponsored boost. With promotions disabled at SST and no per-request opt-in,
// the live stack MUST refuse the boost: every organic candidate MUST outrank
// the sponsored row, and the sponsored row MUST be labeled (not allowed) in
// its persisted policy_decisions list.
//
// The test runs against the disposable e2e stack and uses the FixtureProvider
// "sponsored regression" branch so no external provider traffic is required.
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSponsoredRegression_BS023_NoRankBoost(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "sponsored regression vegetarian quiet near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     5,
	})
	if err != nil {
		t.Fatalf("POST /api/recommendations/requests failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		RequestID       string `json:"request_id"`
		Status          string `json:"status"`
		Recommendations []struct {
			ID              string           `json:"id"`
			Title           string           `json:"title"`
			Rank            int              `json:"rank"`
			PolicyDecisions []map[string]any `json:"policy_decisions"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse response body: %v; body=%s", err, string(body))
	}
	if parsed.Status != "delivered" {
		t.Fatalf("BS-023 regression: expected status=delivered, got %q; body=%s", parsed.Status, string(body))
	}

	var sponsoredRank, organicARank, organicBRank int
	var sponsoredFound, organicAFound, organicBFound bool
	var sponsoredDecisions []map[string]any
	for _, rec := range parsed.Recommendations {
		switch {
		case strings.Contains(rec.Title, "SponsoredRegressionAlpha"):
			organicARank = rec.Rank
			organicAFound = true
		case strings.Contains(rec.Title, "SponsoredRegressionBravo"):
			organicBRank = rec.Rank
			organicBFound = true
		case strings.Contains(rec.Title, "SponsoredRegressionCharlie"):
			sponsoredRank = rec.Rank
			sponsoredFound = true
			sponsoredDecisions = rec.PolicyDecisions
		}
	}

	if !organicAFound || !organicBFound {
		t.Fatalf("BS-023 regression fixture incomplete: organic A found=%v, organic B found=%v; titles=%s", organicAFound, organicBFound, deliveredTitlesJSON(parsed.Recommendations))
	}
	if !sponsoredFound {
		t.Fatalf("BS-023 regression: sponsored candidate was not delivered (must be labeled, not withheld); titles=%s", deliveredTitlesJSON(parsed.Recommendations))
	}

	// Adversarial guard: with promotions disabled, BOTH organic candidates
	// MUST outrank the sponsored candidate even though the sponsored row
	// carries the highest provider_score in the fixture bundle.
	if organicARank >= sponsoredRank {
		t.Fatalf("BS-023 violation: organicA rank=%d should be ahead of sponsored rank=%d", organicARank, sponsoredRank)
	}
	if organicBRank >= sponsoredRank {
		t.Fatalf("BS-023 violation: organicB rank=%d should be ahead of sponsored rank=%d", organicBRank, sponsoredRank)
	}

	// Adversarial guard: persisted policy_decisions for the sponsored
	// candidate MUST contain a sponsored label decision AND MUST NOT contain
	// a sponsored:allow decision (which would indicate boost was authorised).
	var sawSponsoredLabel, sawSponsoredBoostBlock, sawSponsoredAllow bool
	for _, decision := range sponsoredDecisions {
		kind, _ := decision["kind"].(string)
		outcomeStr, _ := decision["outcome"].(string)
		switch {
		case kind == "sponsored" && outcomeStr == "label":
			sawSponsoredLabel = true
		case kind == "sponsored" && outcomeStr == "allow":
			sawSponsoredAllow = true
		case kind == "sponsored_boost" && outcomeStr == "deny":
			sawSponsoredBoostBlock = true
		}
	}
	if !sawSponsoredLabel {
		t.Fatalf("BS-023 violation: sponsored candidate missing sponsored:label decision; decisions=%v", sponsoredDecisions)
	}
	if sawSponsoredAllow {
		t.Fatalf("BS-023 violation: sponsored candidate carries unauthorised sponsored:allow decision (promotions disabled); decisions=%v", sponsoredDecisions)
	}
	if !sawSponsoredBoostBlock {
		t.Fatalf("BS-023 violation: missing sponsored_boost:deny audit decision (promotions disabled); decisions=%v", sponsoredDecisions)
	}
}

// deliveredTitlesJSON serialises the delivered titles for use in failure
// messages so adversarial assertion failures show the actual rank order.
func deliveredTitlesJSON(recs []struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Rank            int              `json:"rank"`
	PolicyDecisions []map[string]any `json:"policy_decisions"`
}) string {
	out := make([]map[string]any, 0, len(recs))
	for _, rec := range recs {
		out = append(out, map[string]any{"rank": rec.Rank, "title": rec.Title})
	}
	encoded, _ := json.Marshal(out)
	return string(encoded)
}
