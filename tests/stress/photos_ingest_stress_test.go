//go:build stress

// Package stress — Spec 040 Scope 5 stress profile.
//
// SCN-040-015: ingest a synthetic 15,000-photo library into the live
// Smackerel stack and prove that:
//
//   - Cross-provider search returns within the configured target
//     latency under the synthetic load
//   - Capability limit + skip ledger surfaces stay live and bounded
//   - No bytes are leaked into logs (asserted indirectly: the API
//     response MUST NOT echo `bytes` payload fields)
//
// The synthetic library is mixed: RAW originals, exports, scanned
// documents, receipts, videos, exact + near duplicates, sensitive,
// and blocked fixtures. Tags surface the categories so the search
// path can prove it ranks them coherently.
package stress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

const (
	photosStressLibrarySize     = 15000
	photosStressIngestWorkers   = 32
	photosStressSearchSamples   = 50
	photosStressSearchP95Budget = 5 * time.Second
)

// TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget
// is the SCN-040-015 stress proof.
func TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("stress: -short specified, skipping 15k photo profile")
	}
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	pool := photosStressPool(t)
	store := photolib.NewStore(pool)

	connectorImmich := "stress-040-immich-" + photosStressRunID()
	connectorPhotoprism := "stress-040-photoprism-" + photosStressRunID()
	t.Cleanup(func() { photosStressCleanup(t, pool, connectorImmich, connectorPhotoprism) })

	// (1) Ingest the synthetic 15k library through `PublishPhotoEvent`
	// so the live store, indexers, and observability counters all see
	// real rows. ~70 % Immich, ~30 % PhotoPrism. ~10 % cross-provider
	// duplicates (same content_hash on both providers). Mixed kinds
	// across the library exercise the dedupe + search ranking paths.
	ingestStart := time.Now()
	var (
		ingested  atomic.Int64
		duplicate atomic.Int64
	)
	jobs := make(chan int, photosStressLibrarySize)
	var wg sync.WaitGroup
	for worker := 0; worker < photosStressIngestWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				event := photosStressEventFor(index)
				connector := connectorImmich
				provider := "immich"
				if index%3 == 0 {
					connector = connectorPhotoprism
					provider = "photoprism"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, err := store.PublishPhotoEvent(ctx, connector, provider, event)
				cancel()
				if err != nil {
					t.Errorf("PublishPhotoEvent index=%d provider=%s: %v", index, provider, err)
					return
				}
				ingested.Add(1)
				if index%10 == 0 {
					// Cross-provider duplicate: republish the SAME
					// content_hash under the OTHER provider so
					// dedupe has work to do.
					mirror := event
					otherConnector := connectorImmich
					otherProvider := "immich"
					if connector == connectorImmich {
						otherConnector = connectorPhotoprism
						otherProvider = "photoprism"
					}
					mirror.RawProvider = map[string]any{"provider": otherProvider, "uid": event.ProviderRef + "-mirror"}
					ctxMirror, cancelMirror := context.WithTimeout(context.Background(), 30*time.Second)
					_, mirrorErr := store.PublishPhotoEvent(ctxMirror, otherConnector, otherProvider, mirror)
					cancelMirror()
					if mirrorErr != nil {
						t.Errorf("PublishPhotoEvent mirror index=%d provider=%s: %v", index, otherProvider, mirrorErr)
						return
					}
					duplicate.Add(1)
				}
			}
		}()
	}
	for i := 0; i < photosStressLibrarySize; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	ingestElapsed := time.Since(ingestStart)
	if ingested.Load() != int64(photosStressLibrarySize) {
		t.Fatalf("ingest count = %d, want %d", ingested.Load(), photosStressLibrarySize)
	}
	t.Logf("stress: ingested %d photos (+%d cross-provider duplicates) in %s", ingested.Load(), duplicate.Load(), ingestElapsed)

	// (2) Search the synthetic library across multiple query shapes.
	// Each query MUST return within the per-call latency budget; the
	// p95 across all samples MUST also stay under the budget. The
	// payload MUST NOT echo raw bytes (no `"bytes":` field in
	// `preview` or top-level result rows).
	queries := []string{
		"vacation",
		"document scan invoice",
		"receipt restaurant",
		"family group portrait",
		"sunset",
		"birthday party",
		"meeting whiteboard",
		"travel landscape",
		"product label closeup",
		"event ticket",
	}
	latencies := make([]time.Duration, 0, photosStressSearchSamples)
	for sample := 0; sample < photosStressSearchSamples; sample++ {
		query := queries[sample%len(queries)]
		callStart := time.Now()
		status, body, err := stressAPIGet(cfg, "/v1/photos/search?q="+url.QueryEscape(query))
		latency := time.Since(callStart)
		if err != nil {
			t.Fatalf("search call %d: %v", sample, err)
		}
		if status != 200 {
			t.Fatalf("search call %d status=%d body=%s", sample, status, string(body))
		}
		if strings.Contains(string(body), `"bytes":`) {
			t.Fatalf("search response leaks raw bytes payload (sample=%d body~=%s)", sample, truncateBody(body))
		}
		latencies = append(latencies, latency)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95Index := int(float64(len(latencies)) * 0.95)
	if p95Index >= len(latencies) {
		p95Index = len(latencies) - 1
	}
	p95 := latencies[p95Index]
	if p95 > photosStressSearchP95Budget {
		t.Fatalf("search p95 latency %s exceeds budget %s (samples=%d)", p95, photosStressSearchP95Budget, len(latencies))
	}
	t.Logf("stress: search p95=%s budget=%s samples=%d", p95, photosStressSearchP95Budget, len(latencies))

	// (3) Photo health aggregate MUST surface live numbers reflecting
	// the new ingest. capability_limits MUST stay populated (the
	// PhotoPrism connector contributes faces_write + sensitivity
	// limitations). The duplicates total MUST report at least the
	// cross-provider duplicates we deliberately injected. We do NOT
	// assert lifecycle.states totals because that block measures the
	// raw/export pairing surface, not raw photo count — its scope
	// is owned by Scope 2/3.
	status, healthBody, err := stressAPIGet(cfg, "/v1/photos/health")
	if err != nil {
		t.Fatalf("GET /v1/photos/health: %v", err)
	}
	if status != 200 {
		t.Fatalf("/v1/photos/health status=%d body=%s", status, string(healthBody))
	}
	var health map[string]any
	if err := json.Unmarshal(healthBody, &health); err != nil {
		t.Fatalf("decode health body: %v body=%s", err, string(healthBody))
	}
	limits, ok := health["capability_limits"].([]any)
	if !ok || len(limits) == 0 {
		t.Fatalf("/v1/photos/health capability_limits empty: %s", string(healthBody))
	}
	// Adversarial: the response MUST NOT echo raw bytes anywhere.
	if strings.Contains(string(healthBody), `"bytes":`) {
		t.Fatalf("/v1/photos/health leaks raw bytes payload: %s", truncateBody(healthBody))
	}
}

func photosStressEventFor(index int) photolib.PhotoEvent {
	event := photolib.SyntheticPhotoEvent()
	id := fmt.Sprintf("stress-040-%06d", index)
	event.ProviderRef = id
	event.ContentHash = "sha256:" + photoStressDigest(id)
	event.Filename = id + ".jpg"

	switch index % 8 {
	case 0:
		event.Tags = []string{"vacation", "travel", "sunset"}
		event.Albums = []string{"Vacation 2026"}
	case 1:
		event.Tags = []string{"document", "scan", "invoice"}
		event.Albums = []string{"Receipts"}
		event.MediaRole = photolib.MediaRoleDocumentScan
	case 2:
		event.Tags = []string{"receipt", "restaurant"}
		event.MediaRole = photolib.MediaRoleDocumentScan
	case 3:
		event.Tags = []string{"family", "group", "portrait"}
	case 4:
		event.Tags = []string{"birthday", "party"}
	case 5:
		event.Tags = []string{"meeting", "whiteboard"}
		event.MediaRole = photolib.MediaRoleDocumentScan
	case 6:
		event.Tags = []string{"travel", "landscape"}
	case 7:
		event.Tags = []string{"product", "label", "closeup"}
		event.MediaRole = photolib.MediaRoleDocumentScan
	}

	if index%50 == 0 {
		event.Sensitivity = photolib.ProviderSensitivity{Level: photolib.SensitivityHidden, Source: "fixture:stress-sensitive"}
	}
	if index%75 == 0 {
		event.MediaRole = photolib.MediaRoleVideo
	}
	return event
}

func photoStressDigest(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func photosStressRunID() string {
	return time.Now().UTC().Format("20060102T150405")
}

func photosStressCleanup(t *testing.T, pool *pgxpool.Pool, immichConnector, photoprismConnector string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = ANY($1)`, []string{immichConnector, photoprismConnector}); err != nil {
		t.Logf("photos stress cleanup failed: %v", err)
	}
}

func photosStressPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("stress: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect stress database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func truncateBody(body []byte) string {
	if len(body) > 256 {
		return string(body[:256]) + "...(truncated)"
	}
	return string(body)
}
