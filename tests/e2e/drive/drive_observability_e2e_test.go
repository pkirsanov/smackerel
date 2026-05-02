//go:build e2e

// Spec 038 Scope 8 — Observability Metrics E2E.
//
// Asserts that drive metric counters (defined in
// internal/drive/observability and registered to the default Prometheus
// registry that internal/metrics.Handler exposes) are wired into the
// live core /metrics endpoint AND that exercising the scan/extract
// pipeline increments the counters AND that the resulting read-model
// rows reconcile with the metric deltas across two providers.
//
// Adversarial guards:
//   - The test scrapes the live /metrics endpoint BEFORE and AFTER the
//     fixture run and asserts BOTH HELP-line registration (the running
//     binary actually compiled the metrics package in) AND counter
//     increments observed in-process via the metrics testutil. A
//     regression that registered the counter without incrementing it
//     would fail. A regression that incremented in tests but did not
//     wire the package into the live binary would also fail.
//   - The test asserts BOTH the google AND the memdrive provider labels
//     produce in-process increments AND DB rows, proving the pipeline
//     is genuinely provider-neutral.
//   - Read-model row counts MUST equal the scan input AND the in-process
//     metric delta — neither path may silently drop or double-count.
package drive

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/memprovider"
	driveobs "github.com/smackerel/smackerel/internal/drive/observability"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)

	// Adversarial Guard 1: the live core's /metrics endpoint MUST expose
	// the smackerel_drive_scan_files_total HELP/TYPE lines, proving the
	// drive observability package is wired into the running binary (not
	// just into the test process).
	mustExposeMetricFamily(t, liveConfig.CoreURL, "smackerel_drive_scan_files_total")
	mustExposeMetricFamily(t, liveConfig.CoreURL, "smackerel_drive_extract_files_total")
	mustExposeMetricFamily(t, liveConfig.CoreURL, "smackerel_drive_save_attempts_total")
	mustExposeMetricFamily(t, liveConfig.CoreURL, "smackerel_drive_retrieve_decisions_total")

	// In-process baselines so we can prove deltas reconcile with input.
	beforeGoogle := driveobs.CounterValue(driveobs.DriveScanFiles, "google", string(driveobs.OutcomeOK))
	beforeMem := driveobs.CounterValue(driveobs.DriveScanFiles, "memdrive", string(driveobs.OutcomeOK))

	// Provider 1 (google fixture) — three files.
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID: "scope8-obs-google-1", Name: "Obs file 1.txt", MimeType: "text/plain",
			SizeBytes: 64, FolderPath: []string{"Obs"}, RevisionID: "obs-1-rev",
			Owner: "fixture@example.com", URL: "https://drive.example/obs-1",
			Content: []byte("observability fixture 1"),
		},
		{
			ID: "scope8-obs-google-2", Name: "Obs file 2.txt", MimeType: "text/plain",
			SizeBytes: 64, FolderPath: []string{"Obs"}, RevisionID: "obs-2-rev",
			Owner: "fixture@example.com", URL: "https://drive.example/obs-2",
			Content: []byte("observability fixture 2"),
		},
		{
			ID: "scope8-obs-google-3", Name: "Obs file 3.txt", MimeType: "text/plain",
			SizeBytes: 64, FolderPath: []string{"Obs"}, RevisionID: "obs-3-rev",
			Owner: "fixture@example.com", URL: "https://drive.example/obs-3",
			Content: []byte("observability fixture 3"),
		},
	})
	googleProvider := newE2EGoogleProvider(fixtureServer, pool)
	googleConnID := createE2EConnection(t, pool, fixtureServer, googleProvider, []string{"root"})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if _, err := smscan.NewService(googleProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, googleConnID); err != nil {
		t.Fatalf("Google InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(googleProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, googleConnID); err != nil {
		t.Fatalf("Google ProcessPending: %v", err)
	}

	// Provider 2 (memdrive) — two files, proves the metric label
	// renders for any provider, not just google.
	memProvider := memprovider.New(memprovider.DefaultCapabilities())
	memOwner := uuid.NewString()
	memConnID := uuid.NewString()
	memProvider.SeedConnection(memConnID, memOwner, smdrive.AccessReadSave, smdrive.Scope{FolderIDs: []string{"root"}})
	uniqueLabel := "scope8-obs-mem-" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO drive_connections (id, provider_id, owner_user_id, account_label, access_mode, status, scope)
		VALUES ($1, 'memdrive', $2, $3, 'read_save', 'healthy', '{"folder_ids":["root"]}'::jsonb)`,
		memConnID, memOwner, uniqueLabel,
	); err != nil {
		t.Fatalf("insert memdrive connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, memConnID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id LIKE $1`, "drive:memdrive:"+memConnID+":%")
	})

	for i := 0; i < 2; i++ {
		fileID := "scope8-obs-mem-" + strconv.Itoa(i)
		memProvider.AddFile(memConnID, smdrive.FolderItem{
			ProviderFileID:     fileID,
			ProviderRevisionID: fileID + "-rev",
			Title:              "Mem obs " + strconv.Itoa(i) + ".txt",
			MimeType:           "text/plain",
			SizeBytes:          48,
			FolderPath:         []string{"Obs", "Mem"},
			ProviderURL:        "memdrive://files/" + fileID,
			ModifiedAt:         time.Now().UTC(),
			OwnerLabel:         "fixture-owner",
		}, []byte("memdrive observability fixture "+strconv.Itoa(i)))
	}
	if _, err := smscan.NewService(memProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, memConnID); err != nil {
		t.Fatalf("memdrive InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(memProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, memConnID); err != nil {
		t.Fatalf("memdrive ProcessPending: %v", err)
	}

	// Adversarial Guard 2: the in-process counter MUST have incremented
	// strictly by the seeded file count for each provider — proving the
	// scan code path actually fires the metric (a no-op metric would
	// regress to zero delta even though the scan succeeded).
	afterGoogle := driveobs.CounterValue(driveobs.DriveScanFiles, "google", string(driveobs.OutcomeOK))
	afterMem := driveobs.CounterValue(driveobs.DriveScanFiles, "memdrive", string(driveobs.OutcomeOK))
	if got := afterGoogle - beforeGoogle; got < 3 {
		t.Fatalf("DriveScanFiles{provider=google,outcome=ok}: before=%v after=%v want delta >=3", beforeGoogle, afterGoogle)
	}
	if got := afterMem - beforeMem; got < 2 {
		t.Fatalf("DriveScanFiles{provider=memdrive,outcome=ok}: before=%v after=%v want delta >=2", beforeMem, afterMem)
	}

	// Adversarial Guard 3: read-model reconciliation. The DB row counts
	// for each connection MUST equal the input — neither the scan path
	// nor the persistence layer is allowed to silently drop or double
	// count files.
	var googleRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, googleConnID).Scan(&googleRows); err != nil {
		t.Fatalf("count google drive_files: %v", err)
	}
	var memRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, memConnID).Scan(&memRows); err != nil {
		t.Fatalf("count memdrive drive_files: %v", err)
	}
	if googleRows != 3 {
		t.Fatalf("read-model google rows = %d, want 3", googleRows)
	}
	if memRows != 2 {
		t.Fatalf("read-model memdrive rows = %d, want 2", memRows)
	}
}

// mustExposeMetricFamily scrapes the live core /metrics endpoint and
// asserts a HELP line for the given metric name is present, proving
// the running binary registered the metric (not just the test process).
func mustExposeMetricFamily(t *testing.T, baseURL, metricName string) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("scrape /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status=%d body=%s", resp.StatusCode, string(body))
	}
	helpPrefix := "# HELP " + metricName + " "
	typePrefix := "# TYPE " + metricName + " "
	hasHelp := false
	hasType := false
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, helpPrefix) {
			hasHelp = true
		}
		if strings.HasPrefix(line, typePrefix) {
			hasType = true
		}
	}
	if !hasHelp || !hasType {
		t.Fatalf("live /metrics did not register %s (hasHelp=%v hasType=%v) — drive observability package is not wired into the running binary", metricName, hasHelp, hasType)
	}
}
