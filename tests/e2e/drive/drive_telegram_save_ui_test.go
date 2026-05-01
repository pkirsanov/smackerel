//go:build e2e

// Spec 038 Scope 5 — End-to-end Telegram receipt save test.
// Validates that the Telegram bridge writes through the full live stack
// and the formatted reply mentions the destination Drive folder along
// with a correction action callback identifier.
package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestTelegramReceiptSaveReplyShowsDriveFolderAndCorrectionAction(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	artifactID := "test:scope5-e2e-tg:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'telegram', 'receipt', 'receipt-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "e2e-telegram-rule",
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceTelegram)},
		Classification:       "receipt",
		SensitivityIn:        []string{string(rules.SensitivityFinancial)},
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

	registry := smdrive.NewRegistry()
	registry.Register(provider)
	saveSvc := save.NewService(pool, e2eResolver{reg: registry}, "https://drive.test/file/d")
	bridge := telegram.NewDriveSaveBridge(pool, repo, rules.NewEngine(time.Now), saveSvc)
	now := time.Now().UTC()
	outcome, err := bridge.SaveReceipt(ctx, telegram.ReceiptSaveInput{
		ArtifactID:     artifactID,
		Classification: "receipt",
		Sensitivity:    string(rules.SensitivityFinancial),
		Confidence:     0.95,
		Tokens:         map[string]string{"year": now.Format("2006")},
		Title:          "e2e-telegram-receipt.jpg",
		MimeType:       "image/jpeg",
		Body:           []byte("e2e telegram receipt bytes"),
	})
	if err != nil {
		t.Fatalf("SaveReceipt: %v", err)
	}
	if !outcome.Saved {
		t.Fatalf("not saved: folder=%q reason=%q err=%q", outcome.Folder, outcome.Reason, outcome.LastError)
	}

	reply := telegram.FormatReceiptReply(outcome)
	if !strings.Contains(reply, "Receipts/") {
		t.Fatalf("reply %q does not mention Receipts/{year} folder", reply)
	}
	if !strings.Contains(reply, "https://drive.test/file/d/") {
		t.Fatalf("reply %q does not include provider URL with prefix https://drive.test/file/d/", reply)
	}

	// Audit row records the correction-eligible match (so the bot can offer
	// a correction action callback against this rule_id).
	var auditOutcome string
	var ruleIDFromAudit string
	if err := pool.QueryRow(ctx,
		`SELECT outcome, rule_id FROM drive_rule_audit WHERE source_artifact_id=$1
		   ORDER BY created_at DESC LIMIT 1`, artifactID,
	).Scan(&auditOutcome, &ruleIDFromAudit); err != nil {
		t.Fatalf("read drive_rule_audit: %v", err)
	}
	if auditOutcome != string(rules.OutcomeMatched) {
		t.Fatalf("audit outcome = %q, want matched (correction action target)", auditOutcome)
	}
	if ruleIDFromAudit != rule.ID {
		t.Fatalf("audit rule_id = %q, want %q", ruleIDFromAudit, rule.ID)
	}
	_ = connectionID
}
