//go:build e2e

// Spec 038 Scope 7 — End-to-end Telegram retrieval test.
//
// TestTelegramRetrievalReturnsFileProviderLinkOrDisambiguationWithDriveLabels
// runs against the live test stack and proves the retrieval bridge:
//
//  1. Returns a disambiguation list when the user query matches more
//     than one drive file, with full title / folder / provider /
//     sensitivity labels for each candidate.
//  2. Returns a provider_link delivery (no inline bytes) when the
//     selected fixture exceeds the configured Telegram inline cap.
//  3. Returns a bytes delivery for a separate non-sensitive fixture
//     under the cap, so the suite proves both branches and rules out
//     a "always downgrade" tautology.
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

func TestTelegramRetrievalReturnsFileProviderLinkOrDisambiguationWithDriveLabels(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope7-e2e-receipt-small",
			Name:       "E2E Receipt small.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  4_096,
			FolderPath: []string{"Receipts", "2026"},
			RevisionID: "scope7-e2e-receipt-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-e2e-receipt-small",
			Content:    []byte("Receipt small content used for bytes path."),
			Shared:     false,
		},
		{
			ID:         "scope7-e2e-receipt-large",
			Name:       "E2E Receipt large.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  10 * 1024 * 1024, // 10 MB > 5 MB cap → provider_link
			FolderPath: []string{"Receipts", "2026"},
			RevisionID: "scope7-e2e-receipt-rev-2",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-e2e-receipt-large",
			Content:    []byte("Receipt large content; never inlined through Telegram."),
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
	// Pin sensitivity to none and size_bytes to fixture truth so the
	// retrieval branches are deterministic regardless of extract
	// classification.
	if _, err := pool.Exec(ctx, `
		UPDATE drive_files
		   SET sensitivity='none',
		       size_bytes=CASE provider_file_id
		         WHEN 'scope7-e2e-receipt-small' THEN 4096
		         WHEN 'scope7-e2e-receipt-large' THEN 10485760
		         ELSE size_bytes END
		 WHERE provider_file_id IN ('scope7-e2e-receipt-small','scope7-e2e-receipt-large')`); err != nil {
		t.Fatalf("normalize sensitivity/size: %v", err)
	}

	registry := smdrive.NewRegistry()
	registry.Register(provider)
	searcher := retrieve.NewPostgresSearcher(pool)
	fetcher := retrieve.NewProviderBytesFetcher(pool, func(ctx context.Context, providerID, conn, fileID string) (io.ReadCloser, string, error) {
		p, ok := registry.Get(providerID)
		if !ok {
			t.Fatalf("provider %q not registered", providerID)
		}
		body, err := p.GetFile(ctx, conn, fileID)
		if err != nil {
			return nil, "", err
		}
		return body.Reader, body.MimeType, nil
	})
	svc := retrieve.NewService(searcher, fetcher, drivepolicy.NewEngine(), 5*1024*1024, retrieve.DefaultReasonTable())
	bridge := telegram.NewDriveRetrieveBridge(svc)

	// 1) Disambiguation list — both receipts match.
	disambig, err := bridge.RetrieveDriveFile(ctx, "user-e2e", "E2E Receipt", "")
	if err != nil {
		t.Fatalf("RetrieveDriveFile (disambig): %v", err)
	}
	if disambig.Mode != retrieve.ModeDisambiguate {
		t.Fatalf("mode = %q, want disambiguate; delivery=%+v", disambig.Mode, disambig)
	}
	if len(disambig.Candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2", len(disambig.Candidates))
	}
	for _, c := range disambig.Candidates {
		if c.Folder != "Receipts/2026" {
			t.Fatalf("candidate %q folder = %q, want Receipts/2026", c.Title, c.Folder)
		}
		if c.Provider != "google" {
			t.Fatalf("candidate %q provider = %q, want google", c.Title, c.Provider)
		}
		if c.Sensitivity != "none" {
			t.Fatalf("candidate %q sensitivity = %q, want none", c.Title, c.Sensitivity)
		}
	}
	reply := telegram.FormatRetrieveReply(disambig)
	for _, want := range []string{"E2E Receipt small.pdf", "E2E Receipt large.pdf", "Receipts/2026", "google"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("disambig reply missing %q: %q", want, reply)
		}
	}

	// 2) Large fixture → provider_link, no bytes.
	var large, small retrieve.RetrieveCandidate
	for _, c := range disambig.Candidates {
		if strings.Contains(c.Title, "large") {
			large = c
		}
		if strings.Contains(c.Title, "small") {
			small = c
		}
	}
	delLarge, err := bridge.RetrieveDriveFile(ctx, "user-e2e", "E2E Receipt", large.ArtifactID)
	if err != nil {
		t.Fatalf("RetrieveDriveFile (large): %v", err)
	}
	if delLarge.Mode != retrieve.ModeProviderLink {
		t.Fatalf("large mode = %q, want provider_link; delivery=%+v", delLarge.Mode, delLarge)
	}
	if len(delLarge.Bytes) != 0 {
		t.Fatalf("large bytes len = %d, want 0 (provider_link path must not fetch bytes)", len(delLarge.Bytes))
	}
	if delLarge.URL == "" {
		t.Fatalf("large URL is empty; provider_link must include URL")
	}
	largeReply := telegram.FormatRetrieveReply(delLarge)
	if !strings.Contains(largeReply, delLarge.URL) {
		t.Fatalf("large reply does not embed URL %q: %q", delLarge.URL, largeReply)
	}

	// 3) Small fixture → bytes, MIME preserved.
	delSmall, err := bridge.RetrieveDriveFile(ctx, "user-e2e", "E2E Receipt", small.ArtifactID)
	if err != nil {
		t.Fatalf("RetrieveDriveFile (small): %v", err)
	}
	if delSmall.Mode != retrieve.ModeBytes {
		t.Fatalf("small mode = %q, want bytes; delivery=%+v", delSmall.Mode, delSmall)
	}
	if len(delSmall.Bytes) == 0 {
		t.Fatalf("small bytes empty")
	}
	if delSmall.MimeType != "application/pdf" {
		t.Fatalf("small mime = %q, want application/pdf", delSmall.MimeType)
	}
	if !strings.Contains(string(delSmall.Bytes), "Receipt small content") {
		t.Fatalf("small bytes do not match fixture content: %q", string(delSmall.Bytes))
	}
}
