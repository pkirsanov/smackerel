//go:build integration

// Spec 038 Scope 7 — SCN-038-022 integration anchor.
//
// TestTelegramRetrievalFindsDriveBoardingPassAndDisambiguates proves the
// Retrieval Service end-to-end against the live test database:
//
//  1. Two boarding-pass fixtures are seeded into Drive via the standard
//     Scope 2 fixture flow (scan + extract).
//  2. The Telegram bridge (built on retrieve.Service + the Postgres
//     searcher + the provider-bytes fetcher) is invoked with the user
//     query "boarding pass".
//  3. The reply must be a disambiguation list that includes BOTH
//     fixtures with their title, folder, provider, and sensitivity
//     labels — exactly the four labels Screen 5 + Telegram quote in
//     the spec for the multi-result case.
//  4. The user picks the first candidate by SelectedArtifactID; the
//     bridge re-routes through policy and the second call delivers
//     bytes (the fixture is non-sensitive and within the 5 MB cap).
//
// Adversarial guards:
//   - The fixture set includes a non-matching "Random meeting notes"
//     artifact; the test asserts it does NOT appear in the candidate
//     list (no "always returns everything" tautology).
//   - The bytes path fetches via the same Provider.GetFile call the
//     production wiring uses (function injection in cmd/core/wiring.go
//     mirrored here), so a regression in the provider layer would
//     fail the test.
//   - The disambiguation reply text is asserted by both substring
//     (each candidate present) and structure (both labels carry the
//     provider id "google" rather than a hard-coded constant).
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

func TestTelegramRetrievalFindsDriveBoardingPassAndDisambiguates(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope7-boarding-lisbon",
			Name:       "Lisbon boarding pass.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  18_432,
			FolderPath: []string{"Travel", "Portugal"},
			RevisionID: "scope7-lisbon-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-lisbon",
			Content:    []byte("Boarding pass: passenger LIS to OPO, gate 23, seat 14A, departure 09:55."),
			Shared:     false,
		},
		{
			ID:         "scope7-boarding-porto",
			Name:       "Porto boarding pass.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  17_910,
			FolderPath: []string{"Travel", "Portugal"},
			RevisionID: "scope7-porto-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-porto",
			Content:    []byte("Boarding pass: passenger OPO to LIS, gate 14, seat 7C, departure 18:20."),
			Shared:     false,
		},
		{
			ID:         "scope7-meeting-notes",
			Name:       "Random meeting notes.txt",
			MimeType:   "text/plain",
			SizeBytes:  240,
			FolderPath: []string{"Meetings"},
			RevisionID: "scope7-noise-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope7-noise",
			Content:    []byte("Standup notes about Q3 marketing plan rollout."),
			Shared:     false,
		},
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).
		ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	// Force the boarding-pass artifacts to non-sensitive so the bytes
	// path is reachable; a sensitive fixture would fall under the
	// dedicated SCN-038-020 unit test (sensitive_delivery_test.go).
	if _, err := pool.Exec(ctx, `
		UPDATE drive_files SET sensitivity='none'
		 WHERE provider_file_id IN ('scope7-boarding-lisbon','scope7-boarding-porto')`); err != nil {
		t.Fatalf("normalize sensitivity: %v", err)
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

	// Step 1 — disambiguation list with both boarding passes.
	delivery, err := bridge.RetrieveDriveFile(ctx, "user-1", "boarding pass", "")
	if err != nil {
		t.Fatalf("RetrieveDriveFile (disambig): %v", err)
	}
	if delivery.Mode != retrieve.ModeDisambiguate {
		t.Fatalf("mode = %q, want %q (two matches must disambiguate); delivery=%+v", delivery.Mode, retrieve.ModeDisambiguate, delivery)
	}
	if len(delivery.Candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2; got %+v", len(delivery.Candidates), delivery.Candidates)
	}
	seen := map[string]retrieve.RetrieveCandidate{}
	for _, c := range delivery.Candidates {
		seen[c.Title] = c
		if c.Provider != "google" {
			t.Fatalf("candidate %q provider = %q, want google", c.Title, c.Provider)
		}
	}
	if _, ok := seen["Lisbon boarding pass.pdf"]; !ok {
		t.Fatalf("Lisbon candidate missing: %+v", delivery.Candidates)
	}
	if _, ok := seen["Porto boarding pass.pdf"]; !ok {
		t.Fatalf("Porto candidate missing: %+v", delivery.Candidates)
	}
	for _, c := range delivery.Candidates {
		if c.Folder == "" || c.Provider == "" || c.Sensitivity == "" {
			t.Fatalf("candidate %+v missing required label", c)
		}
	}
	// Adversarial: meeting notes must NOT appear.
	for _, c := range delivery.Candidates {
		if strings.Contains(c.Title, "meeting") || strings.Contains(c.Title, "Random") {
			t.Fatalf("disambiguation leaked unrelated artifact: %+v", c)
		}
	}

	reply := telegram.FormatRetrieveReply(delivery)
	for _, want := range []string{"Lisbon boarding pass.pdf", "Porto boarding pass.pdf", "Travel/Portugal", "google", "none"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q; reply=%q", want, reply)
		}
	}

	// Step 2 — user picks Lisbon; bytes path delivers content under cap.
	lisbon := seen["Lisbon boarding pass.pdf"]
	delivery2, err := bridge.RetrieveDriveFile(ctx, "user-1", "boarding pass", lisbon.ArtifactID)
	if err != nil {
		t.Fatalf("RetrieveDriveFile (selected): %v", err)
	}
	if delivery2.Mode != retrieve.ModeBytes {
		t.Fatalf("mode = %q, want %q (non-sensitive within cap); delivery=%+v", delivery2.Mode, retrieve.ModeBytes, delivery2)
	}
	if len(delivery2.Bytes) == 0 {
		t.Fatalf("delivery2.Bytes is empty; expected fixture content")
	}
	if delivery2.MimeType != "application/pdf" {
		t.Fatalf("mime = %q, want application/pdf", delivery2.MimeType)
	}
	if !strings.Contains(string(delivery2.Bytes), "passenger LIS to OPO") {
		t.Fatalf("bytes do not match Lisbon fixture content; got %q", string(delivery2.Bytes))
	}
}
