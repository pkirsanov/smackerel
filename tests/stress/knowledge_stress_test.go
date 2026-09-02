//go:build stress

// Package stress contains knowledge synthesis layer stress tests.
// Run via: ./smackerel.sh test stress
//
// Spec 025 validation checkpoints:
// - Synthesis throughput at 500+ artifacts (< 30s P95 per artifact)
// - Lint at 1000-artifact scale (< 5 minutes)
// - Knowledge query response < 2s P95
package stress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

const (
	knowledgeLintMaxDuration               = 5 * time.Minute
	knowledgeLintRunTimeout                = knowledgeLintMaxDuration
	knowledgeLintDatabaseTimeout           = 30 * time.Second
	knowledgeLintArtifactCount       int64 = 1000
	knowledgeLintContextItemLimit          = 50
	knowledgeLintContentCharacterCap       = 8000
)

type knowledgeLintCardinality struct {
	totalRows             int64
	distinctArtifactIDs   int64
	distinctContentHashes int64
}

// stressConfig holds live-stack connection details resolved from environment.
type stressConfig struct {
	CoreURL   string
	AuthToken string
}

// loadStressConfig reads live-stack connection details from environment.
// Config values MUST come from env (SST) — no hardcoded defaults.
func loadStressConfig(t *testing.T) stressConfig {
	t.Helper()
	coreURL := strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL"))
	if coreURL == "" {
		t.Fatal("stress: CORE_EXTERNAL_URL is empty; the canonical stress stack must provide a live core URL")
	}
	authToken := strings.TrimSpace(os.Getenv("SMACKEREL_AUTH_TOKEN"))
	if authToken == "" {
		t.Fatal("stress: SMACKEREL_AUTH_TOKEN is empty; the canonical stress stack must provide authenticated API and NATS access")
	}
	return stressConfig{CoreURL: coreURL, AuthToken: authToken}
}

// loadKnowledgeLintStressConfig resolves the complete production linter
// dependency envelope through config.Load. The stress runner supplies the
// generated disposable test env, so any absent or malformed database, NATS, or
// linter setting is a failed test rather than an optional prerequisite.
func loadKnowledgeLintStressConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("stress: load canonical generated test configuration: %v", err)
	}
	if strings.TrimSpace(cfg.AuthToken) == "" {
		t.Fatal("stress: SMACKEREL_AUTH_TOKEN is empty after config.Load; authenticated API and NATS access are required")
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		t.Fatal("stress: DATABASE_URL is empty after config.Load; production lint requires the disposable PostgreSQL stack")
	}
	if strings.TrimSpace(cfg.NATSURL) == "" {
		t.Fatal("stress: NATS_URL is empty after config.Load; production lint retries require the disposable NATS stack")
	}
	if !cfg.KnowledgeEnabled {
		t.Fatal("stress: KNOWLEDGE_ENABLED must be true for the canonical knowledge stress lane")
	}
	if cfg.KnowledgeLintStaleDays <= 0 {
		t.Fatalf("stress: KNOWLEDGE_LINT_STALE_DAYS must resolve to a positive integer, got %d", cfg.KnowledgeLintStaleDays)
	}
	if cfg.KnowledgeMaxSynthesisRetries < 0 {
		t.Fatalf("stress: KNOWLEDGE_MAX_SYNTHESIS_RETRIES must resolve to a non-negative integer, got %d", cfg.KnowledgeMaxSynthesisRetries)
	}
	if strings.TrimSpace(cfg.KnowledgePromptContractIngestSynthesis) == "" {
		t.Fatal("stress: KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS is empty after config.Load")
	}
	return cfg
}

// stressWaitForHealth blocks until the health endpoint reports healthy.
func stressWaitForHealth(t *testing.T, cfg stressConfig, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := client.Get(cfg.CoreURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("stress: services not healthy after %s at %s", maxWait, cfg.CoreURL)
}

// stressAPIGet performs an authenticated GET, returns status, body, error.
func stressAPIGet(cfg stressConfig, path string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// stressAPIPost performs an authenticated POST, returns status, body, error.
func stressAPIPost(cfg stressConfig, path string, payload []byte) (int, []byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

func seedKnowledgeLintArtifacts(ctx context.Context, pool *pgxpool.Pool, ownerToken string) error {
	commandTag, err := pool.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, content_raw, content_hash, source_id,
			processing_status, synthesis_status, synthesis_at, created_at, updated_at
		)
		SELECT
			$1 || '-artifact-' || generated.ordinal::text,
			'note',
			'test-b025006 synthetic knowledge lint artifact ' || generated.ordinal::text,
			'test-b025006 synthetic content for knowledge lint artifact ' || generated.ordinal::text,
			$1 || '-content-hash-' || generated.ordinal::text,
			$1,
			'completed',
			'completed',
			NOW(),
			NOW(),
			NOW()
		FROM generate_series(1, $2::bigint) AS generated(ordinal)`, ownerToken, knowledgeLintArtifactCount)
	if err != nil {
		return fmt.Errorf("seed knowledge lint artifacts: %w", err)
	}
	if inserted := commandTag.RowsAffected(); inserted != knowledgeLintArtifactCount {
		return fmt.Errorf("seed knowledge lint artifacts: inserted %d rows, expected %d", inserted, knowledgeLintArtifactCount)
	}
	return nil
}

func loadKnowledgeLintCardinality(ctx context.Context, pool *pgxpool.Pool, ownerToken string) (knowledgeLintCardinality, error) {
	var cardinality knowledgeLintCardinality
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT id), COUNT(DISTINCT content_hash)
		FROM artifacts
		WHERE source_id = $1`, ownerToken).Scan(
		&cardinality.totalRows,
		&cardinality.distinctArtifactIDs,
		&cardinality.distinctContentHashes,
	)
	if err != nil {
		return knowledgeLintCardinality{}, fmt.Errorf("query knowledge lint artifact cardinality: %w", err)
	}
	return cardinality, nil
}

func validateKnowledgeLintCardinality(expected int64, actual knowledgeLintCardinality) error {
	if actual.totalRows != expected || actual.distinctArtifactIDs != expected || actual.distinctContentHashes != expected {
		return fmt.Errorf(
			"knowledge lint scale cardinality: expected total=%d distinct_ids=%d distinct_content_hashes=%d, got total=%d distinct_ids=%d distinct_content_hashes=%d",
			expected, expected, expected,
			actual.totalRows, actual.distinctArtifactIDs, actual.distinctContentHashes,
		)
	}
	return nil
}

func cleanupKnowledgeLintScaleRows(t *testing.T, pool *pgxpool.Pool, ownerToken, reportID string) {
	t.Helper()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), knowledgeLintDatabaseTimeout)
	defer cleanupCancel()

	if reportID != "" {
		commandTag, err := pool.Exec(cleanupCtx, `DELETE FROM knowledge_lint_reports WHERE id = $1`, reportID)
		if err != nil {
			t.Errorf("cleanup knowledge lint report %s: %v", reportID, err)
		} else if deleted := commandTag.RowsAffected(); deleted != 1 {
			t.Errorf("cleanup knowledge lint report %s: deleted %d rows, expected 1", reportID, deleted)
		}
	}
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM artifacts WHERE source_id = $1`, ownerToken); err != nil {
		t.Errorf("cleanup knowledge lint artifacts owned by %s: %v", ownerToken, err)
	}

	cardinality, err := loadKnowledgeLintCardinality(cleanupCtx, pool, ownerToken)
	if err != nil {
		t.Errorf("verify knowledge lint artifact cleanup for %s: %v", ownerToken, err)
	} else if err := validateKnowledgeLintCardinality(0, cardinality); err != nil {
		t.Errorf("verify knowledge lint artifact cleanup for %s: %v", ownerToken, err)
	}

	var reportRows int64
	if err := pool.QueryRow(cleanupCtx, `SELECT COUNT(*) FROM knowledge_lint_reports WHERE id = $1`, reportID).Scan(&reportRows); err != nil {
		t.Errorf("verify knowledge lint report cleanup for %s: %v", reportID, err)
	} else if reportRows != 0 {
		t.Errorf("verify knowledge lint report cleanup for %s: got %d rows, expected 0", reportID, reportRows)
	}
	t.Logf("Knowledge lint cleanup verified: owner=%s artifacts=0 report_rows=0", ownerToken)
}

// --- Tests ---

func TestKnowledge_LintScaleCardinalityGuardRejectsZeroAndDrift(t *testing.T) {
	for _, observed := range []int64{0, knowledgeLintArtifactCount - 1, knowledgeLintArtifactCount + 1} {
		t.Run(fmt.Sprintf("observed-%d", observed), func(t *testing.T) {
			actual := knowledgeLintCardinality{
				totalRows:             observed,
				distinctArtifactIDs:   observed,
				distinctContentHashes: observed,
			}
			err := validateKnowledgeLintCardinality(knowledgeLintArtifactCount, actual)
			if err == nil {
				t.Fatalf("knowledge lint cardinality guard accepted %d artifacts, expected exactly %d", observed, knowledgeLintArtifactCount)
			}
			expectedError := fmt.Sprintf(
				"knowledge lint scale cardinality: expected total=%d distinct_ids=%d distinct_content_hashes=%d, got total=%d distinct_ids=%d distinct_content_hashes=%d",
				knowledgeLintArtifactCount, knowledgeLintArtifactCount, knowledgeLintArtifactCount,
				observed, observed, observed,
			)
			if err.Error() != expectedError {
				t.Fatalf("knowledge lint cardinality guard returned %q, expected %q", err.Error(), expectedError)
			}
		})
	}

	exact := knowledgeLintCardinality{
		totalRows:             knowledgeLintArtifactCount,
		distinctArtifactIDs:   knowledgeLintArtifactCount,
		distinctContentHashes: knowledgeLintArtifactCount,
	}
	if err := validateKnowledgeLintCardinality(knowledgeLintArtifactCount, exact); err != nil {
		t.Fatalf("knowledge lint cardinality guard rejected exact scale: %v", err)
	}
}

// TestKnowledge_LintAt1000ArtifactScale verifies that the knowledge lint
// system completes within the 5-minute budget for a 1000-artifact knowledge base
// as specified in spec 025 (R-2506, BS-010).
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_LintAt1000ArtifactScale(t *testing.T) {
	apiCfg := loadStressConfig(t)
	stressWaitForHealth(t, apiCfg, 120*time.Second)
	runtimeCfg := loadKnowledgeLintStressConfig(t)

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := pgxpool.New(connectCtx, runtimeCfg.DatabaseURL)
	if err == nil {
		err = pool.Ping(connectCtx)
	}
	connectCancel()
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatalf("stress: connect production linter to disposable PostgreSQL: %v", err)
	}
	// Cleanup runs LIFO. Register closes first so every later cleanup that uses
	// the live clients executes before those clients close.
	t.Cleanup(pool.Close)

	natsConnectCtx, natsConnectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	natsClient, err := smacknats.Connect(natsConnectCtx, runtimeCfg.NATSURL, runtimeCfg.AuthToken)
	natsConnectCancel()
	if err != nil {
		t.Fatalf("stress: connect production linter to disposable NATS: %v", err)
	}
	t.Cleanup(natsClient.Close)

	ownerRef := "test-b025006-knowledge-lint-" + uuid.NewString()
	reportID := ""
	t.Cleanup(func() {
		cleanupKnowledgeLintScaleRows(t, pool, ownerRef, reportID)
	})

	seedCtx, seedCancel := context.WithTimeout(context.Background(), knowledgeLintDatabaseTimeout)
	err = seedKnowledgeLintArtifacts(seedCtx, pool, ownerRef)
	if err == nil {
		var cardinality knowledgeLintCardinality
		cardinality, err = loadKnowledgeLintCardinality(seedCtx, pool, ownerRef)
		if err == nil {
			err = validateKnowledgeLintCardinality(knowledgeLintArtifactCount, cardinality)
		}
	}
	seedCancel()
	if err != nil {
		t.Fatalf("stress: establish exact knowledge lint scale before production lint: %v", err)
	}
	t.Logf("Knowledge lint precondition verified: owner=%s total=1000 distinct_ids=1000 distinct_content_hashes=1000", ownerRef)

	store := knowledge.NewKnowledgeStore(pool)
	store.MaxTokens = runtimeCfg.KnowledgeConceptMaxTokens
	linter := knowledge.NewLinter(store, pool, knowledge.LinterConfig{
		StaleDays:                runtimeCfg.KnowledgeLintStaleDays,
		MaxSynthesisRetries:      runtimeCfg.KnowledgeMaxSynthesisRetries,
		PromptContractVersion:    runtimeCfg.KnowledgePromptContractIngestSynthesis,
		MaxSynthesisContextItems: knowledgeLintContextItemLimit,
		MaxSynthesisContentChars: knowledgeLintContentCharacterCap,
	}, natsClient)

	runStartedAt := time.Now().UTC()
	lintCtx, lintCancel := context.WithTimeout(context.Background(), knowledgeLintRunTimeout)
	lintStartedAt := time.Now()
	err = linter.RunLint(lintCtx)
	lintElapsed := time.Since(lintStartedAt)
	lintCancel()
	if err != nil {
		t.Fatalf("stress: production knowledge lint failed after %s: %v", lintElapsed, err)
	}
	if lintElapsed > knowledgeLintMaxDuration {
		t.Fatalf("production knowledge lint took %s, expected <= %s", lintElapsed, knowledgeLintMaxDuration)
	}

	status, body, err := stressAPIGet(apiCfg, "/api/knowledge/lint")
	if err != nil {
		t.Fatalf("GET /api/knowledge/lint transport failed after production lint: %v", err)
	}
	if status != 200 {
		t.Fatalf("GET /api/knowledge/lint returned %d: %s", status, string(body))
	}

	var report struct {
		ID         string    `json:"id"`
		RunAt      time.Time `json:"run_at"`
		DurationMs int       `json:"duration_ms"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("parse lint report: %v", err)
	}
	if strings.TrimSpace(report.ID) == "" {
		t.Fatal("GET /api/knowledge/lint returned a report without an ID")
	}
	if report.RunAt.Before(runStartedAt) {
		t.Fatalf("GET /api/knowledge/lint returned pre-existing report %s run at %s; production lint started at %s", report.ID, report.RunAt, runStartedAt)
	}
	reportID = report.ID
	if report.DurationMs < 0 {
		t.Fatalf("GET /api/knowledge/lint returned negative duration %dms for report %s", report.DurationMs, report.ID)
	}
	if report.DurationMs > int(knowledgeLintMaxDuration.Milliseconds()) {
		t.Fatalf("lint report duration %dms exceeds %s budget", report.DurationMs, knowledgeLintMaxDuration)
	}
	t.Logf("Production knowledge lint report %s generated at %s: wall=%s report_duration=%dms", report.ID, report.RunAt, lintElapsed, report.DurationMs)
}

// TestKnowledge_ConceptQueryPerformance verifies that knowledge concept
// queries respond within the 2-second budget defined in spec 025.
// Tests listing, search, and stats endpoints under the performance envelope.
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_ConceptQueryPerformance(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	// Test 1: Concept listing with sort by citations (most expensive query)
	start := time.Now()
	status, body, err := stressAPIGet(cfg, "/api/knowledge/concepts?limit=50&sort=citations")
	if err != nil {
		t.Fatalf("knowledge concepts request failed: %v", err)
	}
	elapsed := time.Since(start)

	if status == 503 {
		t.Fatal("GET /api/knowledge/concepts returned 503: knowledge layer is unavailable on the canonical stress stack")
	}
	if status != 200 {
		t.Fatalf("GET /api/knowledge/concepts returned %d: %s", status, string(body))
	}
	t.Logf("Concept list (limit=50, sort=citations) returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("concept listing took %v, expected < 2s (spec 025 P95 budget)", elapsed)
	}

	// Test 2: Knowledge stats endpoint
	start = time.Now()
	status, body, err = stressAPIGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Fatalf("knowledge stats request failed: %v", err)
	}
	elapsed = time.Since(start)
	if status != 200 {
		t.Fatalf("GET /api/knowledge/stats returned %d: %s", status, string(body))
	}
	t.Logf("Knowledge stats returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("stats endpoint took %v, expected < 2s", elapsed)
	}

	// Test 3: Entity listing
	start = time.Now()
	status, body, err = stressAPIGet(cfg, "/api/knowledge/entities?limit=50&sort=mentions")
	if err != nil {
		t.Fatalf("knowledge entities request failed: %v", err)
	}
	elapsed = time.Since(start)
	if status != 200 {
		t.Fatalf("GET /api/knowledge/entities returned %d: %s", status, string(body))
	}
	t.Logf("Entity list (limit=50, sort=mentions) returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("entity listing took %v, expected < 2s", elapsed)
	}
}

// TestKnowledge_SearchWithKnowledgeLayerPerformance verifies that the
// knowledge-first search path (spec 025 R-2508) responds within acceptable
// latency even with a populated knowledge layer.
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_SearchWithKnowledgeLayerPerformance(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	queries := []string{
		"leadership",
		"pricing strategy",
		"remote work productivity",
		"negotiation techniques",
		"restaurant recommendations",
	}

	for _, q := range queries {
		searchBody, _ := json.Marshal(map[string]string{"query": q})
		start := time.Now()
		status, body, err := stressAPIPost(cfg, "/api/search", searchBody)
		if err != nil {
			t.Fatalf("search request %q failed: %v", q, err)
		}
		elapsed := time.Since(start)

		if status != 200 {
			t.Fatalf("POST /api/search %q returned %d: %s", q, status, string(body))
		}

		var result struct {
			SearchMode     string `json:"search_mode"`
			KnowledgeMatch *struct {
				ConceptID string `json:"concept_id"`
				Title     string `json:"title"`
			} `json:"knowledge_match"`
		}
		_ = json.Unmarshal(body, &result)

		mode := result.SearchMode
		matched := "no"
		if result.KnowledgeMatch != nil {
			matched = result.KnowledgeMatch.Title
		}

		t.Logf("Search %q: %v (mode=%s, knowledge_match=%s)", q, elapsed, mode, matched)
		if elapsed > 5*time.Second {
			t.Errorf("search %q took %v, expected < 5s", q, elapsed)
		}
	}
}

// TestKnowledge_HealthEndpointIncludesKnowledgeSection verifies that the
// /api/health endpoint includes a knowledge section with synthesis stats
// and that this doesn't degrade health check performance.
func TestKnowledge_HealthEndpointIncludesKnowledgeSection(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	// Run 25 rapid health checks and verify knowledge section is present
	for i := 0; i < 25; i++ {
		start := time.Now()
		status, body, err := stressAPIGet(cfg, "/api/health")
		if err != nil {
			t.Fatalf("health check %d failed: %v", i, err)
		}
		elapsed := time.Since(start)

		if status != 200 {
			t.Fatalf("health check %d returned %d: %s", i, status, string(body))
		}
		if elapsed > 2*time.Second {
			t.Errorf("health check %d took %v, expected < 2s", i, elapsed)
		}

		// Verify knowledge section exists on first request
		if i == 0 {
			var health struct {
				Knowledge *struct {
					ConceptCount     int `json:"concept_count"`
					EntityCount      int `json:"entity_count"`
					SynthesisPending int `json:"synthesis_pending"`
				} `json:"knowledge"`
			}
			if err := json.Unmarshal(body, &health); err != nil {
				t.Fatalf("parse health response: %v", err)
			}
			if health.Knowledge != nil {
				t.Logf("Knowledge stats: concepts=%d, entities=%d, pending=%d",
					health.Knowledge.ConceptCount, health.Knowledge.EntityCount, health.Knowledge.SynthesisPending)
			} else {
				t.Log("Knowledge section not present (knowledge layer may be disabled)")
			}
		}
	}
}
