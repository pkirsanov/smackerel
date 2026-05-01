//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm exercises
// SCN-040-009 against the live stack. The /v1/photos/actions/plan
// endpoint must mint a token that requires a follow-up confirmation
// before any provider mutation. Mismatched scope, missing text, or
// expired tokens MUST be rejected at the confirm step.
func TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	syntheticPhoto := "00000000-0000-0000-0000-000000040007"
	otherPhoto := "00000000-0000-0000-0000-000000040008"

	// Plan: archive a synthetic photo. We don't expect the photo to
	// exist on the live stack, but the planning endpoint MUST mint a
	// token without performing any mutation.
	planResp, err := apiPostJSON(cfg, "/v1/photos/actions/plan", map[string]any{
		"action": "archive",
		"scope":  map[string]any{"photo_ids": []string{syntheticPhoto}},
	})
	if err != nil {
		t.Fatalf("POST /v1/photos/actions/plan: %v", err)
	}
	planBody, err := readBody(planResp)
	if err != nil {
		t.Fatalf("read plan body: %v", err)
	}
	if planResp.StatusCode != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", planResp.StatusCode, string(planBody))
	}
	var plan struct {
		ActionToken  string `json:"action_token"`
		Action       string `json:"action"`
		PhotoCount   int    `json:"photo_count"`
		RequiresText bool   `json:"requires_text"`
	}
	if err := json.Unmarshal(planBody, &plan); err != nil {
		t.Fatalf("decode plan: %v body=%s", err, string(planBody))
	}
	if plan.ActionToken == "" {
		t.Fatalf("plan response missing action_token: %s", string(planBody))
	}
	if plan.Action != "archive" {
		t.Fatalf("plan action=%q, want archive", plan.Action)
	}
	if plan.PhotoCount != 1 {
		t.Fatalf("plan photo_count=%d, want 1", plan.PhotoCount)
	}
	if plan.RequiresText {
		t.Fatalf("archive plan should not require text confirmation")
	}

	// Adversarial 1: confirm with a DIFFERENT photo id MUST fail with
	// scope-drift response.
	driftResp, err := apiPostJSON(cfg, "/v1/photos/actions/confirm", map[string]any{
		"action_token": plan.ActionToken,
		"scope":        map[string]any{"photo_ids": []string{otherPhoto}},
	})
	if err != nil {
		t.Fatalf("POST /v1/photos/actions/confirm (drift): %v", err)
	}
	driftBody, err := readBody(driftResp)
	if err != nil {
		t.Fatalf("read drift body: %v", err)
	}
	if driftResp.StatusCode == http.StatusOK {
		t.Fatalf("scope drift expected non-200 response, got 200 body=%s", string(driftBody))
	}
	if !strings.Contains(string(driftBody), "scope") {
		t.Fatalf("scope drift response should reference scope; got %s", string(driftBody))
	}

	// Adversarial 2: delete plans MUST require text confirmation.
	deletePlanResp, err := apiPostJSON(cfg, "/v1/photos/actions/plan", map[string]any{
		"action": "delete",
		"scope":  map[string]any{"photo_ids": []string{syntheticPhoto}},
	})
	if err != nil {
		t.Fatalf("POST /v1/photos/actions/plan (delete): %v", err)
	}
	deletePlanBody, err := readBody(deletePlanResp)
	if err != nil {
		t.Fatalf("read delete plan body: %v", err)
	}
	if deletePlanResp.StatusCode != http.StatusOK {
		t.Fatalf("delete plan status=%d body=%s", deletePlanResp.StatusCode, string(deletePlanBody))
	}
	var deletePlan struct {
		ActionToken  string `json:"action_token"`
		RequiresText bool   `json:"requires_text"`
	}
	if err := json.Unmarshal(deletePlanBody, &deletePlan); err != nil {
		t.Fatalf("decode delete plan: %v body=%s", err, string(deletePlanBody))
	}
	if !deletePlan.RequiresText {
		t.Fatalf("delete plan must require text confirmation")
	}

	// Adversarial 3: confirm delete WITHOUT text confirmation MUST fail.
	deleteWithoutText, err := apiPostJSON(cfg, "/v1/photos/actions/confirm", map[string]any{
		"action_token": deletePlan.ActionToken,
		"scope":        map[string]any{"photo_ids": []string{syntheticPhoto}},
	})
	if err != nil {
		t.Fatalf("POST /v1/photos/actions/confirm (no text): %v", err)
	}
	deleteWithoutTextBody, err := readBody(deleteWithoutText)
	if err != nil {
		t.Fatalf("read delete-without-text body: %v", err)
	}
	if deleteWithoutText.StatusCode == http.StatusOK {
		t.Fatalf("delete-without-text should fail, got 200 body=%s", string(deleteWithoutTextBody))
	}
	if !strings.Contains(strings.ToLower(string(deleteWithoutTextBody)), "text") {
		t.Fatalf("delete-without-text response should mention text confirmation; got %s", string(deleteWithoutTextBody))
	}
}
