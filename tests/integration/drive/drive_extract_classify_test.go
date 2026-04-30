//go:build integration

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveExtractClassifyPersistsSearchableDomainMetadata(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope3-recipe",
			Name:       "Dinner plan.txt",
			MimeType:   "text/plain",
			SizeBytes:  128,
			FolderPath: []string{"Meal Plans", "April"},
			RevisionID: "scope3-recipe-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-recipe",
			Content:    []byte("Dinner plan: chickpeas, parsley, lemon, tahini. Action: buy chickpeas."),
		},
		{
			ID:         "scope3-expense",
			Name:       "Travel receipt.txt",
			MimeType:   "text/plain",
			SizeBytes:  96,
			FolderPath: []string{"Receipts", "Travel"},
			RevisionID: "scope3-expense-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-expense",
			Content:    []byte("Receipt total 42.13 for airport lunch paid with card."),
		},
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := smscan.NewPostgresStore(pool)
	if _, err := smscan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}

	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker())
	result, err := processor.ProcessPending(ctx, connectionID)
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if result.ProcessedCount != 2 || result.SkippedCount != 0 || result.BlockedCount != 0 {
		t.Fatalf("ProcessPending result = %+v, want 2 processed and no skipped/blocked", result)
	}

	var contentRaw string
	var metadataBytes []byte
	var domainBytes []byte
	var extractionState string
	var sensitivity string
	if err := pool.QueryRow(ctx, `
		SELECT a.content_raw, a.metadata, a.domain_data, f.extraction_state, f.sensitivity
		FROM artifacts a
		JOIN drive_files f ON f.artifact_id=a.id
		WHERE f.connection_id=$1 AND f.provider_file_id='scope3-recipe'`, connectionID).Scan(
		&contentRaw, &metadataBytes, &domainBytes, &extractionState, &sensitivity,
	); err != nil {
		t.Fatalf("read recipe artifact: %v", err)
	}
	if !strings.Contains(contentRaw, "chickpeas") || extractionState != "complete" || sensitivity != "none" {
		t.Fatalf("recipe extraction state/content/sensitivity = content=%q state=%s sensitivity=%s", contentRaw, extractionState, sensitivity)
	}

	var metadata map[string]any
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	driveMetadata := metadata["drive"].(map[string]any)
	classification := driveMetadata["classification"].(map[string]any)
	if classification["classification"] != "recipe" || classification["topic"] == "" || classification["audience"] == "" {
		t.Fatalf("classification metadata = %+v, want recipe with topic/audience", classification)
	}
	if confidence, ok := classification["confidence"].(float64); !ok || confidence <= 0.5 {
		t.Fatalf("classification confidence = %v, want > 0.5", classification["confidence"])
	}
	if evidence, ok := classification["evidence"].([]any); !ok || len(evidence) == 0 {
		t.Fatalf("classification evidence = %v, want non-empty evidence", classification["evidence"])
	}

	var domainData map[string]any
	if err := json.Unmarshal(domainBytes, &domainData); err != nil {
		t.Fatalf("domain_data JSON: %v", err)
	}
	if strings.Contains(string(domainBytes), "google") {
		t.Fatalf("domain metadata must be provider-neutral, got %s", string(domainBytes))
	}
	routes := domainData["domain_routes"].([]any)
	if !containsAny(routes, "recipes") || !containsAny(routes, "meal_plan") || !containsAny(routes, "lists") || !containsAny(routes, "digest") {
		t.Fatalf("domain_routes = %v, want recipes, meal_plan, lists, digest", routes)
	}

	var folderSummary []byte
	if err := pool.QueryRow(ctx, `
		SELECT folder_summary FROM drive_folders WHERE connection_id=$1 AND array_to_string(folder_path, '/')='Meal Plans/April'`, connectionID).Scan(&folderSummary); err != nil {
		t.Fatalf("folder summary missing: %v", err)
	}
	if !strings.Contains(string(folderSummary), "Meal Plans") || !strings.Contains(string(folderSummary), "recipe") {
		t.Fatalf("folder summary = %s, want Meal Plans recipe context", string(folderSummary))
	}
}

func containsAny(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
