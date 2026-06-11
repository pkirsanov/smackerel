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
