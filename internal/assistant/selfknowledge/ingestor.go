package selfknowledge

// ingestor.go — spec 104 SCOPE-03.
//
// The Ingestor projects the derived self-knowledge corpus into the shared
// artifacts store under a dedicated source_id namespace ("smackerel_self")
// via the EXISTING ingestion pipeline (connector.ArtifactPublisher /
// RawArtifactPublisher.PublishRawArtifact), so each entry receives a real
// embedding (vector(384) via the ML sidecar) exactly like a connector
// artifact. It is idempotent (content_hash dedup) and sweeps stale rows
// (removed or changed entries) so the namespace always mirrors the live
// SSTs. Wired to run once at boot (after migrations, before serving).

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/extract"
)

// SelfKnowledgeNamespace is the artifacts.source_id value that isolates
// the product self-knowledge corpus from every user's personal captures.
const SelfKnowledgeNamespace = "smackerel_self"

// selfKnowledgeContentType is the artifact_type stamped on self-knowledge
// rows (a plain-text capability description).
const selfKnowledgeContentType = "capability"

// IngestResult reports what one Ingest run did (for boot logging + tests).
type IngestResult struct {
	Entries   int   // derived corpus size
	Published int   // rows newly inserted (deduped rows are not counted)
	Swept     int64 // stale rows deleted from the namespace
}

// DocSource optionally contributes curated product-doc capability entries
// (SCOPE-05). nil in SCOPE-03. When set, its entries are appended to the
// derived corpus before ingestion.
type DocSource interface {
	Entries() ([]CapabilityEntry, error)
}

// Ingestor ingests the self-knowledge corpus into the smackerel_self
// namespace and sweeps stale rows.
type Ingestor struct {
	manifest  *assistant.SkillsManifest
	publisher connector.ArtifactPublisher
	pool      *pgxpool.Pool
	docs      DocSource // optional (SCOPE-05); nil-safe
}

// NewIngestor constructs an Ingestor. manifest, publisher, and pool are
// required (panic on nil — G028 no silent no-op). docs may be nil.
func NewIngestor(manifest *assistant.SkillsManifest, publisher connector.ArtifactPublisher, pool *pgxpool.Pool) *Ingestor {
	if manifest == nil {
		panic("selfknowledge: Ingestor requires a non-nil SkillsManifest")
	}
	if publisher == nil {
		panic("selfknowledge: Ingestor requires a non-nil ArtifactPublisher")
	}
	if pool == nil {
		panic("selfknowledge: Ingestor requires a non-nil pgx pool")
	}
	return &Ingestor{manifest: manifest, publisher: publisher, pool: pool}
}

// WithDocSource attaches a curated product-doc source (SCOPE-05).
func (ing *Ingestor) WithDocSource(d DocSource) *Ingestor {
	ing.docs = d
	return ing
}

// Corpus returns the full self-knowledge corpus (auto-derived scenarios +
// commands, plus any curated doc entries). Shared by Ingest and the /help
// human twin (SCOPE-06) so the two never drift.
func (ing *Ingestor) Corpus() ([]CapabilityEntry, error) {
	entries := Derive(ing.manifest)
	if ing.docs != nil {
		docEntries, err := ing.docs.Entries()
		if err != nil {
			return nil, fmt.Errorf("selfknowledge: doc source: %w", err)
		}
		entries = append(entries, docEntries...)
	}
	return entries, nil
}

// Ingest (re)projects the corpus into the smackerel_self namespace. Safe to
// re-run: unchanged entries dedup by content_hash; changed/removed entries'
// old rows are swept. Returns a summary.
func (ing *Ingestor) Ingest(ctx context.Context) (IngestResult, error) {
	entries, err := ing.Corpus()
	if err != nil {
		return IngestResult{}, err
	}

	keepHashes := make([]string, 0, len(entries))
	published := 0
	for _, e := range entries {
		id, err := ing.publisher.PublishRawArtifact(ctx, connector.RawArtifact{
			SourceID:    SelfKnowledgeNamespace,
			SourceRef:   e.ID,
			URL:         e.ID, // stored in artifacts.source_url — stable per-entry key
			ContentType: selfKnowledgeContentType,
			Title:       e.Title,
			RawContent:  e.Body,
			Metadata:    map[string]any{"kind": e.Kind, "source_ref": e.SourceRef},
			CapturedAt:  time.Now().UTC(),
		})
		if err != nil {
			return IngestResult{}, fmt.Errorf("selfknowledge: publish %s: %w", e.ID, err)
		}
		if id != "" {
			published++ // "" = deduped by content_hash (unchanged entry)
		}
		keepHashes = append(keepHashes, extract.HashContent(e.Body))
	}

	swept, err := ing.sweepStale(ctx, keepHashes)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Entries: len(entries), Published: published, Swept: swept}, nil
}

// sweepStale deletes smackerel_self artifacts whose content_hash is not in
// the current corpus — i.e. entries that were removed, or the old-body
// version of an entry whose SST text changed. keepHashes must be the exact
// set of current-corpus content hashes.
func (ing *Ingestor) sweepStale(ctx context.Context, keepHashes []string) (int64, error) {
	ct, err := ing.pool.Exec(ctx, `
		DELETE FROM artifacts
		WHERE source_id = $1 AND content_hash <> ALL($2::text[])
	`, SelfKnowledgeNamespace, keepHashes)
	if err != nil {
		return 0, fmt.Errorf("selfknowledge: stale sweep: %w", err)
	}
	return ct.RowsAffected(), nil
}
