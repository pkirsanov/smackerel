//go:build e2e

// Spec 038 Scope 1 — SCN-038-002 e2e-ui row.
//
// TestDriveConnectFlowShowsHealthyEmptyDriveConnector exercises the
// connect-and-render flow against the live test stack:
//
//  1. GET /pwa/connectors.html — assert HTML loads and contains the
//     drive provider list scaffold.
//  2. GET /pwa/connectors-add.html — assert the add-drive form has the
//     provider picker, access-mode radios, folder-scope textarea, and
//     submit button required by Screen 2.
//  3. POST /v1/connectors/drive/connect — assert the live stack's
//     handler returns 200 + {authURL, state} for a well-formed request.
//  4. Insert a healthy empty-drive connection directly via DATABASE_URL
//     (round 7's integration row covers FinalizeConnect against the
//     owned fixture; this e2e row simulates the post-OAuth state and
//     asserts the empty-healthy view renders end-to-end through the
//     live HTTP boundary).
//  5. GET /v1/connectors/drive/connection/{id} — assert JSON shape
//     reports status=healthy + indexed_count=0 + empty_drive=true.
//  6. GET /pwa/connector-detail.html — assert HTML carries the
//     accessible status banner scaffolding.
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriveConnectFlowShowsHealthyEmptyDriveConnector(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// 1. PWA connectors list page renders.
	resp, err := http.Get(cfg.CoreURL + "/pwa/connectors.html")
	if err != nil {
		t.Fatalf("GET /pwa/connectors.html: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read connectors.html: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("connectors.html status=%d body=%s", resp.StatusCode, string(body))
	}
	html := string(body)
	for _, want := range []string{
		`id="drive-connectors"`,
		`Cloud Drives`,
		`role="status"`,
		`/pwa/connectors.js`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("connectors.html missing %q", want)
		}
	}

	// 2. PWA add-drive form renders with required fields.
	resp, err = http.Get(cfg.CoreURL + "/pwa/connectors-add.html")
	if err != nil {
		t.Fatalf("GET /pwa/connectors-add.html: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read connectors-add.html: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("connectors-add.html status=%d", resp.StatusCode)
	}
	addHTML := string(body)
	for _, want := range []string{
		`id="drive-add-form"`,
		`id="provider-options"`,
		`name="access_mode"`,
		`value="read_only"`,
		`value="read_save"`,
		`id="folder-scope-input"`,
		`type="submit"`,
		`role="radiogroup"`,
		`aria-label="Drive provider"`,
		`aria-label="Access mode"`,
		`src="/pwa/connectors-add.js"`,
	} {
		if !strings.Contains(addHTML, want) {
			t.Errorf("connectors-add.html missing %q", want)
		}
	}

	// The provider radios are injected by connectors-add.js at load time;
	// verify the JS source actually wires name="provider_id" so the POST
	// handler will see the selected provider.
	resp, err = http.Get(cfg.CoreURL + "/pwa/connectors-add.js")
	if err != nil {
		t.Fatalf("GET /pwa/connectors-add.js: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read connectors-add.js: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("connectors-add.js status=%d", resp.StatusCode)
	}
	addJS := string(body)
	for _, want := range []string{
		`input.name = "provider_id"`,
		`input[name="provider_id"]:checked`,
		`/v1/connectors/drive/connect`,
	} {
		if !strings.Contains(addJS, want) {
			t.Errorf("connectors-add.js missing %q", want)
		}
	}

	// 3. POST /v1/connectors/drive/connect — assert {authURL, state}.
	owner := uuid.NewString()
	connectBody := map[string]any{
		"provider_id":   "google",
		"owner_user_id": owner,
		"access_mode":   "read_save",
		"scope": map[string]any{
			"folder_ids":     []string{"folder-acme"},
			"include_shared": false,
		},
	}
	bb, err := json.Marshal(connectBody)
	if err != nil {
		t.Fatalf("marshal connect body: %v", err)
	}
	connectURL := cfg.CoreURL + "/v1/connectors/drive/connect"
	req, err := http.NewRequest(http.MethodPost, connectURL, bytes.NewReader(bb))
	if err != nil {
		t.Fatalf("build connect request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err = (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST connect: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read connect body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("POST connect status=%d body=%s", resp.StatusCode, string(body))
	}
	var connectResp struct {
		AuthURL string `json:"authURL"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(body, &connectResp); err != nil {
		t.Fatalf("decode connect body: %v body=%s", err, string(body))
	}
	if connectResp.AuthURL == "" {
		t.Errorf("connect response missing authURL; body=%s", string(body))
	}
	if connectResp.State == "" {
		t.Errorf("connect response missing state; body=%s", string(body))
	}
	if !strings.Contains(connectResp.AuthURL, "state="+connectResp.State) {
		t.Errorf("authURL %q does not include the returned state token %q", connectResp.AuthURL, connectResp.State)
	}

	// 4. Insert a healthy empty-drive connection directly via DATABASE_URL.
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — cannot exercise empty-drive view")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	connID := uuid.New()
	expires := time.Now().Add(1 * time.Hour)
	scopeJSON := `{"folder_ids":["folder-acme"],"include_shared":false}`
	if _, err := pool.Exec(ctx,
		`INSERT INTO drive_connections
		 (id, provider_id, owner_user_id, account_label, access_mode, status,
		  scope, credentials_ref, expires_at)
		 VALUES ($1, 'google', $2, 'fixture-user@example.com', 'read_save', 'healthy',
		         $3::jsonb, 'bearer:e2e-fixture', $4)`,
		connID, owner, scopeJSON, expires,
	); err != nil {
		t.Fatalf("insert drive_connections: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, connID)
	})

	// 5. GET connection JSON — assert empty-drive contract.
	getURL := fmt.Sprintf("%s/v1/connectors/drive/connection/%s", cfg.CoreURL, connID.String())
	resp, err = http.Get(getURL)
	if err != nil {
		t.Fatalf("GET connection: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read connection body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET connection status=%d body=%s", resp.StatusCode, string(body))
	}
	var view struct {
		ID           string `json:"id"`
		ProviderID   string `json:"provider_id"`
		Status       string `json:"status"`
		AccessMode   string `json:"access_mode"`
		IndexedCount int64  `json:"indexed_count"`
		SkippedCount int64  `json:"skipped_count"`
		EmptyDrive   bool   `json:"empty_drive"`
		Scope        struct {
			FolderIDs []string `json:"folder_ids"`
		} `json:"scope"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode connection: %v body=%s", err, string(body))
	}
	if view.Status != "healthy" {
		t.Errorf("status=%q want healthy", view.Status)
	}
	if view.IndexedCount != 0 {
		t.Errorf("indexed_count=%d want 0", view.IndexedCount)
	}
	if view.SkippedCount != 0 {
		t.Errorf("skipped_count=%d want 0", view.SkippedCount)
	}
	if !view.EmptyDrive {
		t.Errorf("empty_drive=false want true")
	}
	if view.AccessMode != "read_save" {
		t.Errorf("access_mode=%q want read_save", view.AccessMode)
	}
	if view.ProviderID != "google" {
		t.Errorf("provider_id=%q want google", view.ProviderID)
	}
	if len(view.Scope.FolderIDs) != 1 || view.Scope.FolderIDs[0] != "folder-acme" {
		t.Errorf("scope folder_ids=%v want [folder-acme]", view.Scope.FolderIDs)
	}

	// 6. PWA connector-detail page renders with accessible scaffolding.
	resp, err = http.Get(cfg.CoreURL + "/pwa/connector-detail.html")
	if err != nil {
		t.Fatalf("GET connector-detail.html: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read connector-detail.html: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("connector-detail.html status=%d", resp.StatusCode)
	}
	detailHTML := string(body)
	for _, want := range []string{
		`id="connector-detail"`,
		`Drive connector`,
		`role="status"`,
		`aria-busy="true"`,
		`Indexed files`,
		`/pwa/connector-detail.js`,
	} {
		if !strings.Contains(detailHTML, want) {
			t.Errorf("connector-detail.html missing %q", want)
		}
	}
}
