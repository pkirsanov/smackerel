//go:build e2e

// Spec 038 Scope 7 — End-to-end drive agent tools test.
//
// TestDriveAgentToolsE2E_SearchGetSaveListRulesRespectPolicy proves
// the four drive tools registered with the spec 037 agent registry
// route through the same runtime services the HTTP API and Telegram
// bot use, and inherit the same policy contract:
//
//   - drive_search returns provider-neutral candidates for a seeded
//     drive_file artifact.
//   - drive_get_file with a SENSITIVE selected_artifact_id MUST
//     downgrade to secure_link and return zero bytes (BS-025), even
//     though the LLM is the caller.
//   - drive_save_file refuses sensitive content via pre-flight policy
//     evaluation before the Save Service runs (defense in depth — the
//     Save Rules also gate, but the agent surface adds an explicit
//     refusal so a misconfigured rule cannot leak).
//   - drive_list_rules returns the configured rules so the LLM can
//     plan saves without inventing folders.
//
// This is the live-stack canary that pairs with the unit-level
// internal/drive/tools/tools_test.go and the integration-level
// drive_tools_canary_test.go.
package drive

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	drivepolicy "github.com/smackerel/smackerel/internal/drive/policy"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	drivetools "github.com/smackerel/smackerel/internal/drive/tools"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveAgentToolsE2E_SearchGetSaveListRulesRespectPolicy(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope7-tool-medical",
			Name:       "Insurance card scan.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  3_584,
			FolderPath: []string{"Health", "Insurance"},
			RevisionID: "scope7-tool-medical-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-tool-medical",
			Content:    []byte("Insurance card scan; sensitive."),
			Shared:     false,
		},
	})

	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).
		ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE drive_files SET sensitivity='medical', size_bytes=3584
		 WHERE provider_file_id='scope7-tool-medical'`); err != nil {
		t.Fatalf("normalize sensitivity: %v", err)
	}

	// Wire ToolServices for the duration of the test.
	registry := smdrive.NewRegistry()
	registry.Register(provider)
	resolver := e2eResolver{reg: registry}
	saveSvc := save.NewService(pool, resolver, "https://drive.test/file/d")
	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "scope7-tool-rule",
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceMobile)},
		Classification:       "receipt",
		SensitivityIn:        []string{string(rules.SensitivityNone)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Receipts/{year}",
		OnMissingFolder:      rules.OnMissingCreate,
		OnExistingFile:       rules.OnExistingVersion,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), rule.ID) })

	searcher := retrieve.NewPostgresSearcher(pool)
	fetchCount := 0
	fetcher := retrieve.NewProviderBytesFetcher(pool, func(ctx context.Context, providerID, conn, fileID string) (io.ReadCloser, string, error) {
		fetchCount++
		p, ok := registry.Get(providerID)
		if !ok {
			t.Fatalf("provider %q not registered", providerID)
		}
		body, err := p.GetFile(ctx, conn, fileID)
		if err != nil {
			return nil, "", err
		}
		return body.Reader, body.MimeType, nil
	})
	pol := drivepolicy.NewEngine()
	retriever := retrieve.NewService(searcher, fetcher, pol, 5*1024*1024, retrieve.DefaultReasonTable())

	drivetools.SetToolServices(&drivetools.ToolServices{
		Retriever:   retriever,
		SaveService: saveSvc,
		RulesRepo:   repo,
		RulesEngine: rules.NewEngine(time.Now),
		Policy:      pol,
	})
	t.Cleanup(drivetools.ResetForTest)

	// 1) drive_search — finds the seeded artifact under the fixture title.
	searchTool, ok := agent.ByName("drive_search")
	if !ok {
		t.Fatalf("drive_search not registered")
	}
	out, err := searchTool.Handler(ctx, json.RawMessage(`{"query":"Insurance card"}`))
	if err != nil {
		t.Fatalf("drive_search handler: %v", err)
	}
	var searchPayload struct {
		OK         bool             `json:"ok"`
		Candidates []map[string]any `json:"candidates"`
	}
	if err := json.Unmarshal(out, &searchPayload); err != nil {
		t.Fatalf("unmarshal search payload: %v", err)
	}
	if !searchPayload.OK || len(searchPayload.Candidates) == 0 {
		t.Fatalf("search payload empty: %s", string(out))
	}
	var insuranceArtifactID string
	for _, c := range searchPayload.Candidates {
		if title, _ := c["title"].(string); strings.Contains(title, "Insurance card") {
			insuranceArtifactID, _ = c["artifact_id"].(string)
			if got, _ := c["sensitivity"].(string); got != "medical" {
				t.Fatalf("candidate sensitivity = %q, want medical (BS-025 label)", got)
			}
			if got, _ := c["provider"].(string); got != "google" {
				t.Fatalf("candidate provider = %q, want google", got)
			}
		}
	}
	if insuranceArtifactID == "" {
		t.Fatalf("insurance artifact missing from search payload: %s", string(out))
	}

	// 2) drive_get_file with the sensitive artifact MUST downgrade.
	getTool, _ := agent.ByName("drive_get_file")
	getOut, err := getTool.Handler(ctx, json.RawMessage(`{"query":"Insurance card","selected_artifact_id":"`+insuranceArtifactID+`"}`))
	if err != nil {
		t.Fatalf("drive_get_file handler: %v", err)
	}
	var getPayload map[string]any
	if err := json.Unmarshal(getOut, &getPayload); err != nil {
		t.Fatalf("unmarshal get payload: %v", err)
	}
	if mode, _ := getPayload["mode"].(string); mode != string(retrieve.ModeSecureLink) {
		t.Fatalf("BS-025 VIOLATION: mode = %q, want secure_link; payload=%s", mode, string(getOut))
	}
	if _, present := getPayload["bytes_base64"]; present {
		t.Fatalf("BS-025 VIOLATION: bytes_base64 present for medical: %s", string(getOut))
	}
	if fetchCount != 0 {
		t.Fatalf("BS-025 VIOLATION: fetcher.calls = %d for sensitive get_file; must be 0", fetchCount)
	}

	// 3) drive_save_file with sensitive metadata MUST refuse via
	// pre-flight policy (no Save Service call).
	saveTool, _ := agent.ByName("drive_save_file")
	saveBody := base64.StdEncoding.EncodeToString([]byte("agent-supplied bytes"))
	saveOut, err := saveTool.Handler(ctx, json.RawMessage(`{"artifact_id":"agent:test","title":"agent.pdf","classification":"receipt","sensitivity":"medical","confidence":0.9,"content_base64":"`+saveBody+`"}`))
	if err != nil {
		t.Fatalf("drive_save_file handler: %v", err)
	}
	var savePayload map[string]any
	if err := json.Unmarshal(saveOut, &savePayload); err != nil {
		t.Fatalf("unmarshal save payload: %v", err)
	}
	if reason, _ := savePayload["reason"].(string); reason != "policy_refuse" {
		t.Fatalf("BS-025 VIOLATION: save reason = %q, want policy_refuse; payload=%s", reason, string(saveOut))
	}
	if saved, _ := savePayload["saved"].(bool); saved {
		t.Fatalf("BS-025 VIOLATION: save saved=true for sensitive content")
	}
	if policyReason, _ := savePayload["policy_reason"].(string); policyReason == "" {
		t.Fatalf("save payload missing policy_reason: %s", string(saveOut))
	}

	// 4) drive_list_rules returns the registered rule.
	listTool, _ := agent.ByName("drive_list_rules")
	listOut, err := listTool.Handler(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("drive_list_rules handler: %v", err)
	}
	var listPayload struct {
		OK    bool             `json:"ok"`
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(listOut, &listPayload); err != nil {
		t.Fatalf("unmarshal list payload: %v", err)
	}
	if !listPayload.OK || len(listPayload.Rules) == 0 {
		t.Fatalf("list payload empty: %s", string(listOut))
	}
	found := false
	for _, r := range listPayload.Rules {
		if id, _ := r["id"].(string); id == rule.ID {
			found = true
			if got, _ := r["target_folder_template"].(string); got != "Receipts/{year}" {
				t.Fatalf("rule target_folder_template = %q, want Receipts/{year}", got)
			}
		}
	}
	if !found {
		t.Fatalf("seeded rule %s missing from list payload: %s", rule.ID, string(listOut))
	}
	_ = connectionID
}
