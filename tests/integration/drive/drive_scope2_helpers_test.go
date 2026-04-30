//go:build integration

package drive

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/google"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func newScope2GoogleProvider(fixtureServer *fixtures.Server, pool *pgxpool.Pool) *google.Provider {
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

func createScope2Connection(t *testing.T, pool *pgxpool.Pool, fixtureServer *fixtures.Server, provider *google.Provider, scope smdrive.Scope) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	owner := uuid.NewString()
	authURL, state, err := provider.BeginConnect(smdrive.WithOwnerUserID(ctx, owner), smdrive.AccessReadSave, scope)
	if err != nil {
		t.Fatalf("BeginConnect: %v", err)
	}
	if authURL == "" || state == "" {
		t.Fatalf("BeginConnect returned authURL=%q state=%q", authURL, state)
	}
	code := fixtureServer.IssueAuthCode(state)
	connectionID, err := provider.FinalizeConnect(ctx, state, code)
	if err != nil {
		t.Fatalf("FinalizeConnect: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, connectionID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id LIKE $1`, "drive:google:"+connectionID+":%")
	})
	return connectionID
}

func generateBulkDriveFiles(totalFiles int, folderCount int) []fixtures.File {
	files := make([]fixtures.File, 0, totalFiles)
	for fileIndex := 0; fileIndex < totalFiles; fileIndex = fileIndex + 1 {
		folderIndex := fileIndex % folderCount
		files = append(files, fixtures.File{
			ID:         fmt.Sprintf("bulk-file-%04d", fileIndex),
			Name:       fmt.Sprintf("Bulk file %04d.pdf", fileIndex),
			MimeType:   "application/pdf",
			SizeBytes:  int64(4096 + fileIndex),
			FolderPath: []string{fmt.Sprintf("Folder-%02d", folderIndex), "Archive"},
			RevisionID: fmt.Sprintf("rev-%04d", fileIndex),
			Owner:      "fixture-owner@example.com",
			URL:        fmt.Sprintf("https://drive.example/bulk-file-%04d", fileIndex),
			Content:    []byte(fmt.Sprintf("fixture bytes for bulk file %04d", fileIndex)),
			Shared:     folderIndex%2 == 0,
		})
	}
	return files
}
