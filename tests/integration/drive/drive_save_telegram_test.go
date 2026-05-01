//go:build integration

// Spec 038 Scope 5 — Telegram receipt save integration test.
// Proves the Telegram save bridge wires through the rule engine, the save
// service, and the fixture-backed Drive provider end to end.
package drive

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestTelegramReceiptSaveWritesProviderFileAndArtifactLocation(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Insert a real artifact so the FK on drive_save_requests holds.
	artifactID := "test:scope5-telegram:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'telegram', 'receipt', 'receipt-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "telegram-receipt-rule",
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceTelegram)},
		Classification:       "receipt",
		SensitivityIn:        []string{string(rules.SensitivityFinancial)},
		ConfidenceMin:        0.7,
		ProviderID:           "google",
		TargetFolderTemplate: "Receipts/{year}",
		OnMissingFolder:      rules.OnMissingCreate,
		OnExistingFile:       rules.OnExistingVersion,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), rule.ID) })

	registry := smdrive.NewRegistry()
	registry.Register(provider)
	saveSvc := save.NewService(pool, saveProviderResolver{reg: registry}, "https://drive.test/file/d")
	bridge := telegram.NewDriveSaveBridge(pool, repo, rules.NewEngine(time.Now), saveSvc)

	// Override the connection auto-pick by passing it through manually:
	// the bridge does not expose ConnectionID, so we'll exercise the
	// auto-pick path. The connection just created is the most recent
	// healthy one for "google", so that path picks it.
	now := time.Now().UTC()
	receiptBytes := []byte("Telegram receipt photo bytes — fixture stand-in")
	outcome, err := bridge.SaveReceipt(ctx, telegram.ReceiptSaveInput{
		ArtifactID:     artifactID,
		Classification: "receipt",
		Sensitivity:    string(rules.SensitivityFinancial),
		Confidence:     0.95,
		Tokens:         map[string]string{"year": now.Format("2006")},
		Title:          "telegram-receipt.jpg",
		MimeType:       "image/jpeg",
		Body:           receiptBytes,
	})
	if err != nil {
		t.Fatalf("SaveReceipt: %v", err)
	}
	if !outcome.Saved {
		t.Fatalf("outcome.Saved = false (folder=%q reason=%q err=%q)", outcome.Folder, outcome.Reason, outcome.LastError)
	}

	// Reply formatting carries the saved location.
	reply := telegram.FormatReceiptReply(outcome)
	if reply == "" {
		t.Fatalf("FormatReceiptReply: empty")
	}
	if got := reply; !contains(got, outcome.Folder) {
		t.Fatalf("reply %q does not mention folder %q", got, outcome.Folder)
	}

	// Fixture should have recorded exactly one upload (idempotent).
	uploads := fixtureServer.Uploads()
	if len(uploads) != 1 {
		t.Fatalf("fixture uploads = %d, want 1", len(uploads))
	}
	if uploads[0].Title != "telegram-receipt.jpg" {
		t.Fatalf("upload title = %q, want telegram-receipt.jpg", uploads[0].Title)
	}

	// drive_save_requests row written.
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM drive_save_requests WHERE source_artifact_id=$1 AND rule_id=$2`,
		artifactID, rule.ID,
	).Scan(&status); err != nil {
		t.Fatalf("read save request: %v", err)
	}
	if status != string(save.StatusWritten) {
		t.Fatalf("status = %s, want written", status)
	}

	// Edge row exists.
	var edgeCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM edges WHERE src_id=$1 AND edge_type='drive_save'`, artifactID,
	).Scan(&edgeCount); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("edge count = %d, want 1", edgeCount)
	}

	_ = connectionID // reachable via auto-pick path
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
