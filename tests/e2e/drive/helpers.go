//go:build e2e

package drive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/google"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

// e2eConfig holds live-stack connection details resolved from environment.
type e2eConfig struct {
	CoreURL   string
	AuthToken string
}

// loadE2EConfig reads live-stack connection details from environment.
// Mirrors the helpers in tests/e2e/browser_history_e2e_test.go so the
// drive e2e suite stays self-contained.
//
// Spec 044 Scope 02 wraps the /v1/connectors/drive/* routes in
// bearerAuthMiddleware. The dev/test stack runs Branch 3 (shared-token
// compare) which REQUIRES an Authorization: Bearer ${SMACKEREL_AUTH_TOKEN}
// header on every authed request. Missing SMACKEREL_AUTH_TOKEN is a
// fail-loud condition — silently skipping would let regressions in the
// auth wiring slip past CI unnoticed. CORE_EXTERNAL_URL keeps its
// existing skip semantics because the absence of a live stack is a
// legitimate "not running e2e here" condition; a missing auth token,
// in contrast, only happens when the stack is up but misconfigured.
func loadE2EConfig(t *testing.T) e2eConfig {
	t.Helper()
	coreURL := os.Getenv("CORE_EXTERNAL_URL")
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; run via ./smackerel.sh test e2e")
	}
	return e2eConfig{CoreURL: coreURL, AuthToken: authToken}
}

func waitForHealth(t *testing.T, cfg e2eConfig, maxWait time.Duration) {
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
	t.Fatalf("e2e: services not healthy after %s at %s", maxWait, cfg.CoreURL)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func driveE2EPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func newE2EGoogleProvider(fixtureServer *fixtures.Server, pool *pgxpool.Pool) *google.Provider {
	return google.New(google.DefaultCapabilities()).ConfigureRuntime(
		pool,
		http.DefaultClient,
		config.DriveGoogleProviderConfig{
			OAuthClientID:     "fixture-client",
			OAuthClientSecret: "fixture-secret",
			OAuthRedirectURL:  "http://127.0.0.1:0/v1/connectors/drive/oauth/callback",
			OAuthBaseURL:      fixtureServer.URL,
			APIBaseURL:        fixtureServer.URL,
			ScopeDefaults:     []string{"https://www.googleapis.com/auth/drive.file"},
		},
	)
}

func createE2EConnection(t *testing.T, pool *pgxpool.Pool, fixtureServer *fixtures.Server, provider *google.Provider, folderIDs []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner := uuid.NewString()
	authURL, state, err := provider.BeginConnect(smdrive.WithOwnerUserID(ctx, owner), smdrive.AccessReadSave, smdrive.Scope{FolderIDs: folderIDs})
	if err != nil {
		t.Fatalf("BeginConnect: %v", err)
	}
	if authURL == "" || state == "" {
		t.Fatalf("BeginConnect returned authURL=%q state=%q", authURL, state)
	}
	connectionID, err := provider.FinalizeConnect(ctx, state, fixtureServer.IssueAuthCode(state))
	if err != nil {
		t.Fatalf("FinalizeConnect: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, connectionID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id LIKE $1`, "drive:google:"+connectionID+":%")
	})
	return connectionID
}

func generateE2EBulkDriveFiles(totalFiles int, folderCount int) []fixtures.File {
	files := make([]fixtures.File, 0, totalFiles)
	for fileIndex := 0; fileIndex < totalFiles; fileIndex = fileIndex + 1 {
		folderIndex := fileIndex % folderCount
		files = append(files, fixtures.File{
			ID:         fmt.Sprintf("e2e-file-%03d", fileIndex),
			Name:       fmt.Sprintf("E2E file %03d.txt", fileIndex),
			MimeType:   "text/plain",
			SizeBytes:  int64(100 + fileIndex),
			FolderPath: []string{fmt.Sprintf("E2E-Folder-%02d", folderIndex)},
			RevisionID: fmt.Sprintf("e2e-rev-%03d", fileIndex),
			Owner:      "fixture-owner@example.com",
			URL:        fmt.Sprintf("https://drive.example/e2e-file-%03d", fileIndex),
			Content:    []byte(fmt.Sprintf("e2e fixture bytes %03d", fileIndex)),
		})
	}
	return files
}

func getText(t *testing.T, cfg e2eConfig, url string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	// Spec 044 Scope 02 wrapped /v1/connectors/drive/* in
	// bearerAuthMiddleware. PWA paths under /pwa/* are still
	// unauthenticated, but attaching the header to those requests
	// is harmless because the middleware never runs for them.
	// loadE2EConfig fails loud if SMACKEREL_AUTH_TOKEN is unset, so
	// cfg.AuthToken is always populated when getText is called.
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, err := readBody(response)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", url, response.StatusCode, string(body))
	}
	return string(body)
}
