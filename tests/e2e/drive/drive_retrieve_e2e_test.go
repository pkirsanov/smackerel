//go:build e2e

// Spec 038 Scope 7 — End-to-end sensitive retrieval test.
//
// TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly proves
// that a sensitive drive_file (medical) NEVER returns inline bytes
// through the Telegram channel even when the file size is well below
// the inline cap. This is the live-stack BS-025 anchor for spec 038
// Scope 7 — paired with the unit-level sensitive_delivery_test.go in
// internal/drive/retrieve so a regression at any layer fails before
// the broader gauntlet completes.
package drive

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	drivepolicy "github.com/smackerel/smackerel/internal/drive/policy"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveRetrieveE2E_SensitiveTelegramRequestUsesSafeModeOnly(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope7-e2e-medical",
			Name:       "Lab results 2026.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  6_144,
			FolderPath: []string{"Health", "Labs"},
			RevisionID: "scope7-medical-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-e2e-medical",
			Content:    []byte("Sensitive medical fixture content that must NOT leak through Telegram bytes."),
			Shared:     false,
		},
		// Adversarial control: a non-sensitive fixture that DOES return bytes
		// proves the test is not asserting "everything is refused".
		{
			ID:         "scope7-e2e-control",
			Name:       "Lab schedule readme.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  2_048,
			FolderPath: []string{"Health", "Labs"},
			RevisionID: "scope7-control-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-e2e-control",
			Content:    []byte("Lab schedule README. Non-sensitive fixture content."),
			Shared:     false,
		},
	})

	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).
		ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE drive_files SET sensitivity=CASE provider_file_id
		    WHEN 'scope7-e2e-medical' THEN 'medical'
		    ELSE 'none' END,
		   size_bytes=CASE provider_file_id
		    WHEN 'scope7-e2e-medical' THEN 6144
		    WHEN 'scope7-e2e-control' THEN 2048
		    ELSE size_bytes END
		 WHERE provider_file_id IN ('scope7-e2e-medical','scope7-e2e-control')`); err != nil {
		t.Fatalf("normalize sensitivity/size: %v", err)
	}

	registry := smdrive.NewRegistry()
	registry.Register(provider)
	searcher := retrieve.NewPostgresSearcher(pool)
	fetchCount := 0
	const maxInlineBytes int64 = 5 * 1024 * 1024
	fetcher := retrieve.NewProviderBytesFetcher(pool, func(ctx context.Context, providerID, conn, fileID string) (io.ReadCloser, string, error) {
		fetchCount++
		p, ok := registry.Get(providerID)
		if !ok {
			t.Fatalf("provider %q not registered", providerID)
		}
		body, err := p.GetFile(ctx, conn, fileID)
		if err != nil {
			return nil, "", err
		}
		return body.Reader, body.MimeType, nil
	}, maxInlineBytes)
	svc := retrieve.NewService(searcher, fetcher, drivepolicy.NewEngine(), maxInlineBytes, retrieve.DefaultReasonTable())
	bridge := telegram.NewDriveRetrieveBridge(svc)

	// Direct retrieve for the medical artifact: the lone match returns
	// secure_link, no bytes, no provider fetch.
	medical, err := bridge.RetrieveDriveFile(ctx, "user-e2e", "Lab results", "")
	if err != nil {
		t.Fatalf("RetrieveDriveFile (medical): %v", err)
	}
	if medical.Mode != retrieve.ModeSecureLink {
		t.Fatalf("BS-025 VIOLATION: medical mode = %q, want secure_link; delivery=%+v", medical.Mode, medical)
	}
	if len(medical.Bytes) != 0 {
		t.Fatalf("BS-025 VIOLATION: medical bytes len = %d, want 0", len(medical.Bytes))
	}
	if fetchCount != 0 {
		t.Fatalf("BS-025 VIOLATION: fetcher.calls = %d for medical retrieval; must be 0", fetchCount)
	}
	if medical.PolicyReason == "" {
		t.Fatalf("policy_reason empty; agent trace must explain the downgrade")
	}
	reply := telegram.FormatRetrieveReply(medical)
	if !strings.Contains(reply, "sensitive") {
		t.Fatalf("reply does not flag sensitivity: %q", reply)
	}

	// Adversarial control: the non-sensitive control artifact MUST
	// return bytes. If both branches refused, the test could pass on
	// a "always refuse" regression — this proves the bytes path
	// is reachable in the same harness.
	control, err := bridge.RetrieveDriveFile(ctx, "user-e2e", "Lab schedule readme", "")
	if err != nil {
		t.Fatalf("RetrieveDriveFile (control): %v", err)
	}
	if control.Mode != retrieve.ModeBytes {
		t.Fatalf("control mode = %q, want bytes (proves bytes path reachable); delivery=%+v", control.Mode, control)
	}
	if len(control.Bytes) == 0 {
		t.Fatalf("control bytes empty; bytes path is broken")
	}
	if fetchCount != 1 {
		t.Fatalf("fetcher.calls after control retrieval = %d, want exactly 1 (medical=0 + control=1)", fetchCount)
	}
}
