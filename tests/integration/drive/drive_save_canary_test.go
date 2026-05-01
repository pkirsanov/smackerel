//go:build integration

// Spec 038 Scope 5 Canary — Idempotent folder resolution + graph linking.
//
// This is the "Canary" row in scopes.md Scope 5 Test Plan. It runs against the
// live test stack and proves three coupled invariants of the Save Service:
//
//  1. Concurrent callers asking for the same (connection_id, folder_path)
//     collapse onto exactly one drive_folder_resolutions row, even when the
//     provider EnsureFolder call itself is called multiple times.
//  2. The Save Service writes exactly one drive_save_requests row per
//     (rule_id, source_artifact_id, target_path) triple — second callers
//     with the same idempotency key short-circuit and return the already-
//     written outcome without re-uploading.
//  3. Successful saves emit an `edges` row linking the source artifact to
//     the destination drive_save_request (graph hook for downstream
//     surfaces like the artifact detail page and the digest layer).
//
// If any of those drift, the canary fails before the broader Scope 5
// integration tests run, giving a fast signal that the foundation is broken.
package drive

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveSaveCanary_IdempotentFolderResolutionAndGraphLinks(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Insert a source artifact so the FK on drive_save_requests holds.
	artifactID := "test:scope5-canary:" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'telegram', 'canary', 'canary-content', $1, $1, NOW(), NOW())`, artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id=$1`, artifactID) })

	// Configure a single matching rule via repository.
	repo := rules.NewRepository(pool)
	rule, err := repo.Create(ctx, rules.Rule{
		Name:                 "canary-rule",
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

	// Build a save service backed by the fixture-targeted provider.
	registry := smdrive.NewRegistry()
	registry.Register(provider)
	resolver := saveProviderResolver{reg: registry}
	saveSvc := save.NewService(pool, resolver, "https://drive.test/file/d")

	// Force every concurrent caller to use the same connection.
	bytes := []byte("canary-receipt-bytes")
	req := save.Request{
		Rule:             rule,
		SourceArtifactID: artifactID,
		ConnectionID:     connectionID,
		RenderedPath:     renderRulePath(t, rule, time.Now().UTC()),
		Bytes: save.Bytes{
			Title:    "canary-receipt.txt",
			MimeType: "text/plain",
			Body:     bytes,
		},
	}

	const callers = 16
	results := make([]save.Result, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		idx := i
		go func() {
			defer wg.Done()
			res, err := saveSvc.Save(ctx, req)
			results[idx] = res
			errs[idx] = err
		}()
	}
	wg.Wait()
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("Save call %d failed: %v", i, errs[i])
		}
		if results[i].Status != save.StatusWritten {
			t.Fatalf("Save call %d status = %s, want written", i, results[i].Status)
		}
	}

	// Invariant 1 — exactly one folder mapping for this (connection, path).
	var resolutionCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM drive_folder_resolutions
		 WHERE connection_id=$1 AND folder_path=$2`,
		connectionID, req.RenderedPath,
	).Scan(&resolutionCount); err != nil {
		t.Fatalf("count folder resolutions: %v", err)
	}
	if resolutionCount != 1 {
		t.Fatalf("drive_folder_resolutions count = %d, want exactly 1 (idempotent collapse)", resolutionCount)
	}

	// Invariant 2 — exactly one save request row.
	var requestCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM drive_save_requests
		 WHERE source_artifact_id=$1 AND rule_id=$2`,
		artifactID, rule.ID,
	).Scan(&requestCount); err != nil {
		t.Fatalf("count save requests: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("drive_save_requests count = %d, want exactly 1 (idempotent collapse)", requestCount)
	}

	// Invariant 3 — graph edge from artifact to save request.
	var edgeCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM edges
		 WHERE src_type='artifact' AND src_id=$1 AND edge_type='drive_save'`,
		artifactID,
	).Scan(&edgeCount); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("edges count = %d, want exactly 1 (graph link)", edgeCount)
	}
}

// saveProviderResolver adapts a smdrive.Registry into the
// save.ProviderResolver interface.
type saveProviderResolver struct {
	reg *smdrive.Registry
}

func (r saveProviderResolver) Get(id string) (smdrive.Provider, bool) {
	return r.reg.Get(id)
}

func renderRulePath(t *testing.T, rule rules.Rule, when time.Time) string {
	t.Helper()
	tokens := map[string]string{
		"year":  when.Format("2006"),
		"month": when.Format("01"),
	}
	out, err := rules.RenderTargetPath(rule.TargetFolderTemplate, tokens)
	if err != nil {
		t.Fatalf("renderRulePath: %v", err)
	}
	return out
}
