//go:build e2e

// Spec 038 Scope 5 — End-to-end save tests against the live stack.
//
// These tests exercise the full save path: HTTP API → save service → fixture
// provider → drive_save_requests + drive_folder_resolutions + edges + meal
// plan provider_url + digest exposure.
package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/internal/mealplan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

// TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable proves that an
// end-to-end meal-plan save round-trips into the live database and surfaces
// a provider URL on the meal_plans row that the digest layer can read.
func TestDriveSaveE2E_MealPlanSavedBackAndDigestLinkAvailable(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	artifactID := "test:scope5-e2e-mp:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'meal_plan', 'plan', 'plan-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

	planID := "mp-e2e-scope5-" + uuid.NewString()
	startDate := time.Now().UTC().Truncate(24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO meal_plans (id, title, start_date, end_date, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'draft', NOW(), NOW())`,
		planID, "E2E meal plan", startDate, startDate.Add(7*24*time.Hour)); err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM meal_plans WHERE id=$1`, planID) })

	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "e2e-mp-rule",
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
	saveSvc := save.NewService(pool, e2eResolver{reg: registry}, "https://drive.test/file/d")
	store := mealplan.NewStore(pool)
	saveBack := mealplan.NewDriveSaveBack(repo, rules.NewEngine(time.Now), saveSvc, store)
	outcome, err := saveBack.SavePlan(ctx, planID, artifactID)
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if !outcome.Saved {
		t.Fatalf("not saved: folder=%q reason=%q err=%q", outcome.Folder, outcome.Reason, outcome.LastError)
	}
	if outcome.ProviderURL == "" {
		t.Fatalf("provider URL is empty (digest cannot link)")
	}
	var providerURL string
	if err := pool.QueryRow(ctx,
		`SELECT provider_url FROM meal_plans WHERE id=$1`, planID,
	).Scan(&providerURL); err != nil {
		t.Fatalf("read meal_plan provider_url: %v", err)
	}
	if providerURL != outcome.ProviderURL {
		t.Fatalf("meal_plans.provider_url = %q, want %q", providerURL, outcome.ProviderURL)
	}

	// Now hit the HTTP API and prove /v1/drive/save/requests surfaces this row.
	// loadE2EConfig already fails loud if SMACKEREL_AUTH_TOKEN is unset
	// (spec 044 Scope 02 — bearerAuthMiddleware now wraps /v1/drive/*).
	authToken := cfg.AuthToken
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.CoreURL+"/v1/drive/save/requests?limit=10", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call /v1/drive/save/requests: %v", err)
	}
	body, _ := readBody(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/drive/save/requests status %d: %s", resp.StatusCode, string(body))
	}
	var listing struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	found := false
	for _, row := range listing.Requests {
		if id, ok := row["source_artifact_id"].(string); ok && id == artifactID {
			found = true
			if status, _ := row["status"].(string); status != string(save.StatusWritten) {
				t.Fatalf("/v1/drive/save/requests row status = %s, want written", status)
			}
			if url, _ := row["provider_url"].(string); !strings.HasPrefix(url, "https://drive.test/file/d/") {
				t.Fatalf("provider_url = %q, want prefix https://drive.test/file/d/", url)
			}
		}
	}
	if !found {
		t.Fatalf("/v1/drive/save/requests did not include row for artifact %s", artifactID)
	}
	_ = connectionID
}

// TestDriveSaveE2E_ConcurrentMissingFolderCreatesExactlyOneFolder spawns N
// goroutines firing parallel save requests for the same target folder and
// asserts the fixture observes exactly one folder creation across them.
func TestDriveSaveE2E_ConcurrentMissingFolderCreatesExactlyOneFolder(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.SetFolderCreateDelay(50 * time.Millisecond)
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "e2e-concurrent-rule",
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

	// One artifact per caller (different idempotency keys per save request)
	// so the load actually races on folder creation, not save dedup.
	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	errs := make([]error, callers)
	now := time.Now().UTC()
	for i := 0; i < callers; i++ {
		idx := i
		artifactID := fmt.Sprintf("test:scope5-e2e-conc:%d:%s", idx, uuid.NewString())
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
			VALUES ($1, 'telegram', 'receipt', 'concurrent-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
			t.Fatalf("insert artifact %d: %v", idx, err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

		go func() {
			defer wg.Done()
			path, err := rules.RenderTargetPath(rule.TargetFolderTemplate, map[string]string{"year": now.Format("2006")})
			if err != nil {
				errs[idx] = err
				return
			}
			_, err = saveSvc.Save(ctx, save.Request{
				Rule:             rule,
				SourceArtifactID: artifactID,
				ConnectionID:     connectionID,
				RenderedPath:     path,
				Bytes: save.Bytes{
					Title:    fmt.Sprintf("concurrent-receipt-%d.txt", idx),
					MimeType: "text/plain",
					Body:     []byte(fmt.Sprintf("payload %d", idx)),
				},
			})
			errs[idx] = err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Save call %d: %v", i, err)
		}
	}
	folderPath, err := rules.RenderTargetPath(rule.TargetFolderTemplate, map[string]string{"year": now.Format("2006")})
	if err != nil {
		t.Fatalf("RenderTargetPath: %v", err)
	}
	if got := fixtureServer.FolderCreateCount(folderPath); got != 1 {
		t.Fatalf("fixture FolderCreateCount(%q) = %d, want exactly 1 (concurrent collapse)", folderPath, got)
	}
	var resCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM drive_folder_resolutions
		 WHERE connection_id=$1 AND folder_path=$2`,
		connectionID, folderPath,
	).Scan(&resCount); err != nil {
		t.Fatalf("count drive_folder_resolutions: %v", err)
	}
	if resCount != 1 {
		t.Fatalf("drive_folder_resolutions count = %d, want exactly 1", resCount)
	}
}

type e2eResolver struct {
	reg *smdrive.Registry
}

func (r e2eResolver) Get(id string) (smdrive.Provider, bool) {
	return r.reg.Get(id)
}
