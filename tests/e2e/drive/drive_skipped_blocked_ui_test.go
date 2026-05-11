//go:build e2e

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

func TestSkippedAndBlockedFilesAreGroupedByConcreteReasonWithActions(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope3-e2e-too-large",
			Name:       "Too large.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  2048,
			FolderPath: []string{"Contracts"},
			RevisionID: "scope3-e2e-too-large-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-e2e-too-large",
			Content:    []byte("too large"),
		},
		{
			ID:         "scope3-e2e-zip",
			Name:       "Encrypted.zip",
			MimeType:   "application/zip",
			SizeBytes:  64,
			FolderPath: []string{"Sensitive"},
			RevisionID: "scope3-e2e-zip-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-e2e-zip",
			Content:    []byte("PK encrypted"),
		},
	})
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker(), driveextract.WithMaxFileSizeBytes(1024)).ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	responseText := getText(t, liveConfig, liveConfig.CoreURL+"/v1/connectors/drive/connection/"+connectionID+"/skipped")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(responseText), &decoded); err != nil {
		t.Fatalf("decode skipped view: %v body=%s", err, responseText)
	}
	if int(decoded["total"].(float64)) != 2 {
		t.Fatalf("skipped total = %v, want 2; body=%s", decoded["total"], responseText)
	}
	if !strings.Contains(responseText, "file_too_large") || !strings.Contains(responseText, "unsupported_binary") || !strings.Contains(responseText, "recommended_action") {
		t.Fatalf("skipped/blocked response missing reasons/actions: %s", responseText)
	}
	detailHTML := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/connector-detail.html")
	for _, expected := range []string{"id=\"skipped-review\"", "Skipped and blocked files"} {
		if !strings.Contains(detailHTML, expected) {
			t.Fatalf("connector detail HTML missing %q", expected)
		}
	}
}
