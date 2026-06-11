//go:build e2e

// Spec 083 Card Rewards Companion (Scope 02) — T-02-04 / SCN-083-B08.
// End-to-end CRUD round-trip for the card-rewards API against the live stack:
// POST a (custom) card, GET it, PUT an edit, DELETE it, and confirm the final
// GET reflects deletion (404). No mocks — exercises the real chi router, bearer
// auth, service, and PostgreSQL store. Uses a custom card so the round-trip is
// self-contained (no catalog seeding required).
//
// Run via: ./smackerel.sh test e2e --go-run CardRewardsAPI

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// cardRewardsAPISend performs an authenticated request with an optional JSON
// body against the live stack (used for PUT/DELETE not covered by the shared
// apiGet/apiPostJSON helpers).
func cardRewardsAPISend(t *testing.T, cfg e2eConfig, method, path string, payload any) *http.Response {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, cfg.CoreURL+path, body)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func TestCardRewardsAPICRUDRoundTrip_B08(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	unique := time.Now().UTC().Format("20060102150405.000000000")

	// --- POST: create a custom card -------------------------------------
	createBody := map[string]any{
		"custom": map[string]any{
			"name":             "E2E Round-Trip Card " + unique,
			"issuer":           "E2E Bank",
			"card_type":        "fixed",
			"annual_fee_cents": 9500,
		},
	}
	resp, err := apiPostJSON(cfg, "/api/cards", createBody)
	if err != nil {
		t.Fatalf("POST /api/cards: %v", err)
	}
	body, _ := readBody(resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/cards = %d, want 201; body=%s", resp.StatusCode, string(body))
	}
	var created struct {
		ID            string `json:"id"`
		CardCatalogID string `json:"card_catalog_id"`
		Active        bool   `json:"active"`
		CatalogName   string `json:"catalog_name"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse create response: %v; body=%s", err, string(body))
	}
	if created.ID == "" {
		t.Fatalf("created card missing id; body=%s", string(body))
	}
	if !created.Active {
		t.Fatalf("created card not active; body=%s", string(body))
	}
	if created.CatalogName != "E2E Round-Trip Card "+unique {
		t.Fatalf("created catalog_name = %q, want the custom name; body=%s", created.CatalogName, string(body))
	}

	cardPath := "/api/cards/" + created.ID

	// --- GET: read it back ----------------------------------------------
	getResp, err := apiGet(cfg, cardPath)
	if err != nil {
		t.Fatalf("GET %s: %v", cardPath, err)
	}
	getBody, _ := readBody(getResp)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200; body=%s", cardPath, getResp.StatusCode, string(getBody))
	}
	var fetched struct {
		ID       string  `json:"id"`
		Nickname *string `json:"nickname"`
	}
	if err := json.Unmarshal(getBody, &fetched); err != nil {
		t.Fatalf("parse get response: %v; body=%s", err, string(getBody))
	}
	if fetched.ID != created.ID {
		t.Fatalf("GET returned id %q, want %q", fetched.ID, created.ID)
	}

	// --- PUT: edit nickname/note ----------------------------------------
	putResp := cardRewardsAPISend(t, cfg, http.MethodPut, cardPath, map[string]any{
		"nickname": "Edited Nickname",
		"note":     "edited via e2e",
		"active":   false,
	})
	putBody, _ := readBody(putResp)
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %s = %d, want 200; body=%s", cardPath, putResp.StatusCode, string(putBody))
	}
	var updated struct {
		Nickname *string `json:"nickname"`
		Active   bool    `json:"active"`
	}
	if err := json.Unmarshal(putBody, &updated); err != nil {
		t.Fatalf("parse put response: %v; body=%s", err, string(putBody))
	}
	if updated.Nickname == nil || *updated.Nickname != "Edited Nickname" {
		t.Fatalf("PUT nickname = %v, want 'Edited Nickname'; body=%s", updated.Nickname, string(putBody))
	}
	if updated.Active {
		t.Fatalf("PUT active = true, want false (edit applied); body=%s", string(putBody))
	}

	// --- DELETE ----------------------------------------------------------
	delResp := cardRewardsAPISend(t, cfg, http.MethodDelete, cardPath, nil)
	delBody, _ := readBody(delResp)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE %s = %d, want 204; body=%s", cardPath, delResp.StatusCode, string(delBody))
	}

	// --- GET again: must reflect deletion (404) -------------------------
	gone, err := apiGet(cfg, cardPath)
	if err != nil {
		t.Fatalf("GET (post-delete) %s: %v", cardPath, err)
	}
	goneBody, _ := readBody(gone)
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("post-delete GET %s = %d, want 404; body=%s", cardPath, gone.StatusCode, string(goneBody))
	}
	var errEnv struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(goneBody, &errEnv); err != nil {
		t.Fatalf("parse post-delete error envelope: %v; body=%s", err, string(goneBody))
	}
	if errEnv.Error.Code != "CARD_NOT_FOUND" {
		t.Fatalf("post-delete error code = %q, want CARD_NOT_FOUND; body=%s", errEnv.Error.Code, string(goneBody))
	}
}

// TestCardRewardsRecommendationsE2E_G08 — Scope 07 / SCN-083-G08. Drives the
// recommendation + optimization-report endpoints end-to-end against the live
// stack: declare a tracked category, add a card with a 5% offer on it, generate
// recommendations for a unique period, then GET both endpoints and confirm they
// return the current period's data. No mocks — real chi router, bearer auth,
// optimizer, recommender, and PostgreSQL.
func TestCardRewardsRecommendationsE2E_G08(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	unique := time.Now().UTC().Format("20060102150405.000000000")
	category := "E2E Dining " + unique // unique tracked category isolates this run
	period := "e2e-" + unique          // unique period isolates recommendation rows

	// --- declare the tracked category -----------------------------------
	aliasResp, err := apiPostJSON(cfg, "/api/card-category-aliases", map[string]any{
		"canonical_category": category,
		"equivalents":        []string{"e2e eating out " + unique},
		"starred":            true,
	})
	if err != nil {
		t.Fatalf("POST /api/card-category-aliases: %v", err)
	}
	aliasBody, _ := readBody(aliasResp)
	if aliasResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/card-category-aliases = %d, want 201; body=%s", aliasResp.StatusCode, string(aliasBody))
	}

	// --- add a card with a 5% offer on the tracked category -------------
	cardResp, err := apiPostJSON(cfg, "/api/cards", map[string]any{
		"custom": map[string]any{
			"name":             "E2E Rec Card " + unique,
			"issuer":           "E2E Bank",
			"card_type":        "fixed",
			"annual_fee_cents": 0,
		},
	})
	if err != nil {
		t.Fatalf("POST /api/cards: %v", err)
	}
	cardBody, _ := readBody(cardResp)
	if cardResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/cards = %d, want 201; body=%s", cardResp.StatusCode, string(cardBody))
	}
	var card struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(cardBody, &card); err != nil || card.ID == "" {
		t.Fatalf("parse created card: %v; body=%s", err, string(cardBody))
	}

	offerResp := cardRewardsAPISend(t, cfg, http.MethodPost, "/api/cards/"+card.ID+"/offers", map[string]any{
		"title":               "5% Dining offer",
		"category":            category,
		"rate":                5,
		"rate_type":           "percent",
		"activation_required": false,
	})
	offerBody, _ := readBody(offerResp)
	if offerResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST offer = %d, want 201; body=%s", offerResp.StatusCode, string(offerBody))
	}

	// --- generate recommendations for the period ------------------------
	genResp := cardRewardsAPISend(t, cfg, http.MethodPost, "/api/card-recommendations/generate", map[string]any{
		"period": period,
	})
	genBody, _ := readBody(genResp)
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("POST generate = %d, want 200; body=%s", genResp.StatusCode, string(genBody))
	}
	var gen struct {
		Period    string `json:"period"`
		Generated int    `json:"generated"`
	}
	if err := json.Unmarshal(genBody, &gen); err != nil {
		t.Fatalf("parse generate report: %v; body=%s", err, string(genBody))
	}
	if gen.Period != period || gen.Generated < 1 {
		t.Fatalf("generate report = %+v, want period=%s generated>=1; body=%s", gen, period, string(genBody))
	}

	// --- GET recommendations: must return the current period's data -----
	recResp, err := apiGet(cfg, "/api/card-recommendations?period="+period)
	if err != nil {
		t.Fatalf("GET /api/card-recommendations: %v", err)
	}
	recBody, _ := readBody(recResp)
	if recResp.StatusCode != http.StatusOK {
		t.Fatalf("GET recommendations = %d, want 200; body=%s", recResp.StatusCode, string(recBody))
	}
	var recEnv struct {
		Period          string `json:"period"`
		Recommendations []struct {
			Category              string  `json:"category"`
			RecommendedUserCardID *string `json:"recommended_user_card_id"`
			Rate                  float64 `json:"rate"`
			Reason                string  `json:"reason"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(recBody, &recEnv); err != nil {
		t.Fatalf("parse recommendations: %v; body=%s", err, string(recBody))
	}
	if recEnv.Period != period {
		t.Fatalf("recommendations period = %q, want %q", recEnv.Period, period)
	}
	var found bool
	for _, r := range recEnv.Recommendations {
		if r.Category != category {
			continue
		}
		found = true
		if r.RecommendedUserCardID == nil || *r.RecommendedUserCardID != card.ID {
			t.Fatalf("recommended card = %v, want %s; body=%s", r.RecommendedUserCardID, card.ID, string(recBody))
		}
		if r.Rate != 5 {
			t.Fatalf("recommended rate = %v, want 5; body=%s", r.Rate, string(recBody))
		}
		if r.Reason == "" {
			t.Fatalf("recommendation reason is empty (Principle 8); body=%s", string(recBody))
		}
	}
	if !found {
		t.Fatalf("no recommendation for category %q in period %q; body=%s", category, period, string(recBody))
	}

	// --- GET optimization report: must return the optimizer breakdown ---
	repResp, err := apiGet(cfg, "/api/card-optimization-report?period="+period)
	if err != nil {
		t.Fatalf("GET /api/card-optimization-report: %v", err)
	}
	repBody, _ := readBody(repResp)
	if repResp.StatusCode != http.StatusOK {
		t.Fatalf("GET optimization-report = %d, want 200; body=%s", repResp.StatusCode, string(repBody))
	}
	var repEnv struct {
		Period     string `json:"period"`
		Categories []struct {
			Category string  `json:"category"`
			Rate     float64 `json:"rate"`
			Source   string  `json:"source"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(repBody, &repEnv); err != nil {
		t.Fatalf("parse optimization report: %v; body=%s", err, string(repBody))
	}
	if repEnv.Period != period {
		t.Fatalf("report period = %q, want %q", repEnv.Period, period)
	}
	var reported bool
	for _, c := range repEnv.Categories {
		if c.Category != category {
			continue
		}
		reported = true
		if c.Rate != 5 || c.Source != "offer" {
			t.Fatalf("report for %q = rate %v source %q, want 5/offer; body=%s", category, c.Rate, c.Source, string(repBody))
		}
	}
	if !reported {
		t.Fatalf("optimization report missing category %q; body=%s", category, string(repBody))
	}
}
