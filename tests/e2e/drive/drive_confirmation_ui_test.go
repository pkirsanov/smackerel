//go:build e2e

// Spec 038 Scope 6 — Confirmation HTTP endpoint E2E test.
//
// Anchor: SCN-038-016 — low-confidence routing requires user
// confirmation before any provider write.
//
// The test inserts a pending confirmation through the live confirm.Store
// (same code path the runtime uses), then drives the live HTTP endpoint
// at /api/v1/drive/confirmations/{id} for GET, POST resolve, and the
// adversarial double-resolve. Asserts:
//
//   - GET returns the pending row as JSON
//   - POST resolves with outcome=commit, returns 200, status=committed
//   - second POST returns 409 Conflict (idempotent — exactly-once)
//   - the persisted row matches the FIRST resolution
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/drive/confirm"
)

func TestLowConfidenceConfirmationPausesRoutingUntilUserChoosesOutcome(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed an artifact for the FK.
	artifactID := "test:scope6-e2e-confirm:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw,
		                       content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'drive_file', 'low-confidence.pdf', '<bytes>',
		        $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE id=$1`, artifactID)
	})

	// Create a pending confirmation through the same store the runtime
	// uses — proves the HTTP route reads what the production writer
	// wrote.
	store := confirm.NewStore(pool, 1*time.Hour)
	pending, err := store.Create(ctx, confirm.CreateInput{
		Kind:             confirm.KindClassification,
		SourceArtifactID: artifactID,
		Payload: confirm.Payload{
			Classification: "personal",
			Sensitivity:    "none",
			Confidence:     0.42,
			RenderedPath:   "Personal/2025/notes.pdf",
			Title:          "low-confidence.pdf",
			ProviderID:     "google",
		},
	})
	if err != nil {
		t.Fatalf("seed pending confirmation: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM drive_confirmations WHERE id=$1`, pending.ID)
	})

	client := &http.Client{Timeout: 10 * time.Second}
	endpoint := cfg.CoreURL + "/v1/drive/confirmations/" + pending.ID

	// GET the pending row.
	getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	getReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatalf("GET confirmation: %v", err)
	}
	getBody, _ := readBody(getResp)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", getResp.StatusCode, string(getBody))
	}
	var getView struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Kind   string `json:"kind"`
	}
	if err := json.Unmarshal(getBody, &getView); err != nil {
		t.Fatalf("decode GET body: %v body=%s", err, string(getBody))
	}
	if getView.ID != pending.ID {
		t.Fatalf("GET id = %q, want %q", getView.ID, pending.ID)
	}
	if getView.Status != string(confirm.StatusPending) {
		t.Fatalf("GET status = %q, want pending", getView.Status)
	}
	if getView.Kind != string(confirm.KindClassification) {
		t.Fatalf("GET kind = %q, want classification", getView.Kind)
	}

	// POST resolve with outcome=commit.
	resolveBody := map[string]any{
		"channel": "web",
		"choice": map[string]any{
			"outcome": "commit",
		},
	}
	resolvePayload, _ := json.Marshal(resolveBody)
	postReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(resolvePayload))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	postResp, err := client.Do(postReq)
	if err != nil {
		t.Fatalf("POST resolve: %v", err)
	}
	postRespBody, _ := readBody(postResp)
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, body=%s", postResp.StatusCode, string(postRespBody))
	}
	var postView struct {
		Status  string `json:"status"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(postRespBody, &postView); err != nil {
		t.Fatalf("decode POST body: %v body=%s", err, string(postRespBody))
	}
	if postView.Status != string(confirm.StatusCommitted) {
		t.Fatalf("POST status = %q, want committed", postView.Status)
	}
	if postView.Channel != string(confirm.ChannelWeb) {
		t.Fatalf("POST channel = %q, want web", postView.Channel)
	}

	// Adversarial: second POST must return 409 Conflict — exactly-once
	// proven over the live HTTP boundary.
	post2Req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(resolvePayload))
	post2Req.Header.Set("Content-Type", "application/json")
	post2Req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	post2Resp, err := client.Do(post2Req)
	if err != nil {
		t.Fatalf("POST resolve (second): %v", err)
	}
	post2Body, _ := readBody(post2Resp)
	if post2Resp.StatusCode != http.StatusConflict {
		t.Fatalf("second POST status = %d, want 409 Conflict; body=%s",
			post2Resp.StatusCode, string(post2Body))
	}

	// Verify persisted row matches first resolution.
	var status, channel string
	if err := pool.QueryRow(ctx, `
		SELECT status, channel FROM drive_confirmations WHERE id=$1`,
		pending.ID).Scan(&status, &channel); err != nil {
		t.Fatalf("re-fetch persisted: %v", err)
	}
	if status != string(confirm.StatusCommitted) {
		t.Fatalf("persisted status = %q, want committed", status)
	}
	if channel != string(confirm.ChannelWeb) {
		t.Fatalf("persisted channel = %q, want web", channel)
	}
}
