//go:build integration

// Spec 038 Scope 5 — Meal plan write-back integration test.
package drive

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/internal/mealplan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestMealPlanSaveBackCreatesDriveFileAndDigestLink(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Insert a real artifact for the FK.
	artifactID := "test:scope5-mealplan:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'meal_plan', 'plan', 'plan-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

	// Insert a meal plan row with no slots.
	planID := "mp-scope5-" + uuid.NewString()
	startDate := time.Now().UTC().Truncate(24 * time.Hour)
	endDate := startDate.Add(7 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO meal_plans (id, title, start_date, end_date, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'draft', NOW(), NOW())`,
		planID, "Scope-5 plan", startDate, endDate); err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM meal_plans WHERE id=$1`, planID) })

	// Configure a meal-plan save rule.
	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "meal-plan-rule",
		Enabled:              true,
		SourceKinds:          []string{string(rules.SourceMealPlan)},
		Classification:       "meal_plan",
		SensitivityIn:        []string{string(rules.SensitivityNone)},
		ConfidenceMin:        0.5,
		ProviderID:           "google",
		TargetFolderTemplate: "Plans/{year}",
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

	store := mealplan.NewStore(pool)
	saveBack := mealplan.NewDriveSaveBack(pool, repo, rules.NewEngine(time.Now), saveSvc, store)
	outcome, err := saveBack.SavePlan(ctx, planID, artifactID)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if !outcome.Saved {
		t.Fatalf("outcome.Saved = false (folder=%q reason=%q err=%q)", outcome.Folder, outcome.Reason, outcome.LastError)
	}
	if outcome.ProviderURL == "" {
		t.Fatalf("outcome.ProviderURL is empty (no digest link)")
	}

	// Fixture upload count.
	uploads := fixtureServer.Uploads()
	if len(uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(uploads))
	}
	if uploads[0].Title != "Scope-5 plan.md" {
		t.Fatalf("upload title = %q, want Scope-5 plan.md", uploads[0].Title)
	}

	// drive_save_requests row.
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

	// meal_plans.provider_url populated.
	var providerURL string
	if err := pool.QueryRow(ctx,
		`SELECT provider_url FROM meal_plans WHERE id=$1`, planID,
	).Scan(&providerURL); err != nil {
		t.Fatalf("read meal_plan provider_url: %v", err)
	}
	if providerURL == "" {
		t.Fatalf("meal_plans.provider_url is empty after save (digest link unavailable)")
	}
	if providerURL != outcome.ProviderURL {
		t.Fatalf("meal_plans.provider_url = %q, want %q (matches outcome)", providerURL, outcome.ProviderURL)
	}

	_ = connectionID
}
