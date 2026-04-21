package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// LinterConfig holds configuration values for the knowledge linter.
// All values originate from config/smackerel.yaml via config generate.
type LinterConfig struct {
	StaleDays           int
	MaxSynthesisRetries int
}

// Linter performs periodic quality audits on the knowledge layer.
type Linter struct {
	store *KnowledgeStore
	pool  *pgxpool.Pool
	cfg   LinterConfig
	nats  *smacknats.Client
}

// NewLinter creates a new knowledge Linter.
func NewLinter(store *KnowledgeStore, pool *pgxpool.Pool, cfg LinterConfig, natsClient *smacknats.Client) *Linter {
	return &Linter{
		store: store,
		pool:  pool,
		cfg:   cfg,
		nats:  natsClient,
	}
}

// RunLint orchestrates all 6 lint checks, retries failed synthesis, and stores the report.
func (l *Linter) RunLint(ctx context.Context) error {
	if l.pool == nil {
		return fmt.Errorf("lint: database pool is nil")
	}

	start := time.Now()
	var findings []LintFinding

	orphans, err := l.checkOrphanConcepts(ctx)
	if err != nil {
		slog.Warn("lint: orphan concepts check failed", "error", err)
	} else {
		findings = append(findings, orphans...)
	}

	contradictions, err := l.checkContradictions(ctx)
	if err != nil {
		slog.Warn("lint: contradictions check failed", "error", err)
	} else {
		findings = append(findings, contradictions...)
	}

	stale, err := l.checkStaleKnowledge(ctx)
	if err != nil {
		slog.Warn("lint: stale knowledge check failed", "error", err)
	} else {
		findings = append(findings, stale...)
	}

	backlog, err := l.checkSynthesisBacklog(ctx)
	if err != nil {
		slog.Warn("lint: synthesis backlog check failed", "error", err)
	} else {
		findings = append(findings, backlog...)
	}

	weak, err := l.checkWeakEntities(ctx)
	if err != nil {
		slog.Warn("lint: weak entities check failed", "error", err)
	} else {
		findings = append(findings, weak...)
	}

	unreferenced, err := l.checkUnreferencedClaims(ctx)
	if err != nil {
		slog.Warn("lint: unreferenced claims check failed", "error", err)
	} else {
		findings = append(findings, unreferenced...)
	}

	// Retry failed synthesis after collecting findings
	l.retrySynthesisBacklog(ctx)

	duration := time.Since(start)

	if err := l.store.StoreLintReport(ctx, findings, duration); err != nil {
		return fmt.Errorf("store lint report: %w", err)
	}

	slog.Info("knowledge lint complete",
		"findings", len(findings),
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

// checkOrphanConcepts finds concept pages with zero incoming edges.
func (l *Linter) checkOrphanConcepts(ctx context.Context) ([]LintFinding, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT kc.id, kc.title
		FROM knowledge_concepts kc
		LEFT JOIN edges e ON e.dst_id = kc.id AND e.dst_type = 'concept'
		GROUP BY kc.id, kc.title
		HAVING COUNT(e.id) = 0`)
	if err != nil {
		return nil, fmt.Errorf("query orphan concepts: %w", err)
	}
	defer rows.Close()

	var findings []LintFinding
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("scan orphan concept: %w", err)
		}
		findings = append(findings, LintFinding{
			Type:            "orphan_concept",
			Severity:        "low",
			TargetID:        id,
			TargetType:      "concept",
			TargetTitle:     title,
			Description:     fmt.Sprintf("Concept %q has no incoming edges from other concepts or entities", title),
			SuggestedAction: "Review whether this concept should be linked to related concepts or if it can be merged",
		})
	}
	return findings, rows.Err()
}

// checkContradictions finds CONTRADICTS edges that exist in the knowledge graph.
func (l *Linter) checkContradictions(ctx context.Context) ([]LintFinding, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT e.src_id, e.dst_id, e.metadata
		FROM edges e
		WHERE e.edge_type = 'CONTRADICTS'`)
	if err != nil {
		return nil, fmt.Errorf("query contradictions: %w", err)
	}
	defer rows.Close()

	var findings []LintFinding
	for rows.Next() {
		var srcID, dstID string
		var metadataJSON json.RawMessage
		if err := rows.Scan(&srcID, &dstID, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan contradiction: %w", err)
		}

		var metadata map[string]interface{}
		_ = json.Unmarshal(metadataJSON, &metadata)

		claimA, _ := metadata["claim_a"].(string)
		claimB, _ := metadata["claim_b"].(string)
		conceptID, _ := metadata["concept_id"].(string)

		description := fmt.Sprintf("Contradicting claims between artifacts %s and %s", srcID, dstID)
		if claimA != "" && claimB != "" {
			description = fmt.Sprintf("Conflicting claims: %q vs %q", claimA, claimB)
		}

		targetTitle := conceptID
		if targetTitle == "" {
			targetTitle = srcID + " ↔ " + dstID
		}

		findings = append(findings, LintFinding{
			Type:            "contradiction",
			Severity:        "high",
			TargetID:        srcID,
			TargetType:      "artifact",
			TargetTitle:     targetTitle,
			Description:     description,
			SuggestedAction: "Review both sources and assess which applies to your context",
		})
	}
	return findings, rows.Err()
}

// checkStaleKnowledge finds concepts not updated in staleDays with newer artifacts on the topic.
func (l *Linter) checkStaleKnowledge(ctx context.Context) ([]LintFinding, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT kc.id, kc.title, kc.updated_at
		FROM knowledge_concepts kc
		WHERE kc.updated_at < NOW() - ($1 || ' days')::interval
		  AND EXISTS (
			SELECT 1 FROM artifacts a
			WHERE a.id = ANY(kc.source_artifact_ids)
			  AND a.created_at > kc.updated_at
		  )`, l.cfg.StaleDays)
	if err != nil {
		return nil, fmt.Errorf("query stale knowledge: %w", err)
	}
	defer rows.Close()

	var findings []LintFinding
	for rows.Next() {
		var id, title string
		var updatedAt time.Time
		if err := rows.Scan(&id, &title, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan stale concept: %w", err)
		}
		findings = append(findings, LintFinding{
			Type:            "stale_knowledge",
			Severity:        "medium",
			TargetID:        id,
			TargetType:      "concept",
			TargetTitle:     title,
			Description:     fmt.Sprintf("Concept %q last updated %s, but has newer source artifacts", title, updatedAt.Format("2006-01-02")),
			SuggestedAction: "Re-run synthesis on this concept's artifacts to update the knowledge page",
		})
	}
	return findings, rows.Err()
}

// checkSynthesisBacklog finds artifacts with pending or failed synthesis status.
func (l *Linter) checkSynthesisBacklog(ctx context.Context) ([]LintFinding, error) {
	artifacts, err := l.store.GetArtifactsBySynthesisStatus(ctx, []string{"pending", "failed"}, 100)
	if err != nil {
		return nil, fmt.Errorf("query synthesis backlog: %w", err)
	}

	var findings []LintFinding
	for _, a := range artifacts {
		findings = append(findings, LintFinding{
			Type:            "synthesis_backlog",
			Severity:        "high",
			TargetID:        a.ID,
			TargetType:      "artifact",
			TargetTitle:     a.Title,
			Description:     fmt.Sprintf("Artifact %q has synthesis_status=%q (retry_count=%d)", a.Title, a.SynthesisStatus, a.RetryCount),
			SuggestedAction: "Will be retried automatically if under max retries",
		})
	}
	return findings, nil
}

// checkWeakEntities finds entities with interaction_count = 1 (single mention).
func (l *Linter) checkWeakEntities(ctx context.Context) ([]LintFinding, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT id, name, entity_type
		FROM knowledge_entities
		WHERE interaction_count = 1`)
	if err != nil {
		return nil, fmt.Errorf("query weak entities: %w", err)
	}
	defer rows.Close()

	var findings []LintFinding
	for rows.Next() {
		var id, name, entityType string
		if err := rows.Scan(&id, &name, &entityType); err != nil {
			return nil, fmt.Errorf("scan weak entity: %w", err)
		}
		findings = append(findings, LintFinding{
			Type:            "weak_entity",
			Severity:        "low",
			TargetID:        id,
			TargetType:      "entity",
			TargetTitle:     name,
			Description:     fmt.Sprintf("Entity %q (%s) has only 1 interaction — may not be significant", name, entityType),
			SuggestedAction: "Monitor for additional mentions; may resolve naturally as more content is ingested",
		})
	}
	return findings, rows.Err()
}

// checkUnreferencedClaims finds claims that cite artifacts which no longer exist in the DB.
func (l *Linter) checkUnreferencedClaims(ctx context.Context) ([]LintFinding, error) {
	// Get all concepts and check their claims against existing artifacts
	rows, err := l.pool.Query(ctx, `
		SELECT kc.id, kc.title, kc.claims
		FROM knowledge_concepts kc
		WHERE jsonb_array_length(kc.claims) > 0`)
	if err != nil {
		return nil, fmt.Errorf("query concepts for claim check: %w", err)
	}
	defer rows.Close()

	type conceptClaims struct {
		id     string
		title  string
		claims json.RawMessage
	}
	var toCheck []conceptClaims
	for rows.Next() {
		var cc conceptClaims
		if err := rows.Scan(&cc.id, &cc.title, &cc.claims); err != nil {
			return nil, fmt.Errorf("scan concept claims: %w", err)
		}
		toCheck = append(toCheck, cc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Collect ALL unique artifact IDs across ALL concepts in one pass
	type conceptArtifactRef struct {
		conceptID    string
		conceptTitle string
		artifactID   string
	}
	var refs []conceptArtifactRef
	allArtifactIDs := make(map[string]bool)

	for _, cc := range toCheck {
		var claims []Claim
		if err := json.Unmarshal(cc.claims, &claims); err != nil {
			continue
		}
		for _, c := range claims {
			if c.ArtifactID != "" && !allArtifactIDs[c.ArtifactID] {
				allArtifactIDs[c.ArtifactID] = true
				refs = append(refs, conceptArtifactRef{
					conceptID:    cc.id,
					conceptTitle: cc.title,
					artifactID:   c.ArtifactID,
				})
			}
		}
	}

	if len(allArtifactIDs) == 0 {
		return nil, nil
	}

	// Single batch query: check which artifact IDs actually exist
	allIDs := make([]string, 0, len(allArtifactIDs))
	for id := range allArtifactIDs {
		allIDs = append(allIDs, id)
	}
	existRows, err := l.pool.Query(ctx, `SELECT id FROM artifacts WHERE id = ANY($1)`, allIDs)
	if err != nil {
		return nil, fmt.Errorf("check artifact existence: %w", err)
	}
	defer existRows.Close()

	existingIDs := make(map[string]bool)
	for existRows.Next() {
		var id string
		if err := existRows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan existing artifact: %w", err)
		}
		existingIDs[id] = true
	}
	if err := existRows.Err(); err != nil {
		return nil, err
	}

	// Build findings for missing artifact references
	var findings []LintFinding
	for _, ref := range refs {
		if !existingIDs[ref.artifactID] {
			findings = append(findings, LintFinding{
				Type:            "unreferenced_claim",
				Severity:        "medium",
				TargetID:        ref.conceptID,
				TargetType:      "concept",
				TargetTitle:     ref.conceptTitle,
				Description:     fmt.Sprintf("Concept %q cites artifact %s which no longer exists", ref.conceptTitle, ref.artifactID),
				SuggestedAction: "Remove or update claims referencing the deleted artifact",
			})
		}
	}
	return findings, nil
}

// retrySynthesisBacklog re-publishes failed artifacts and marks those at max retries as abandoned.
func (l *Linter) retrySynthesisBacklog(ctx context.Context) {
	artifacts, err := l.store.GetArtifactsBySynthesisStatus(ctx, []string{"pending", "failed"}, 50)
	if err != nil {
		slog.Warn("lint: failed to get synthesis backlog for retry", "error", err)
		return
	}

	for _, a := range artifacts {
		if a.RetryCount >= l.cfg.MaxSynthesisRetries {
			if err := l.store.UpdateArtifactSynthesisStatus(ctx, a.ID, "abandoned", "max retries exceeded"); err != nil {
				slog.Warn("lint: failed to mark artifact abandoned", "artifact_id", a.ID, "error", err)
			} else {
				slog.Info("lint: artifact abandoned after max retries", "artifact_id", a.ID, "retry_count", a.RetryCount)
			}
			continue
		}

		// Re-publish to synthesis.extract for retry
		req := map[string]interface{}{
			"artifact_id":  a.ID,
			"retry_count":  a.RetryCount + 1,
			"triggered_by": "lint_retry",
		}
		data, err := json.Marshal(req)
		if err != nil {
			slog.Warn("lint: failed to marshal retry request", "artifact_id", a.ID, "error", err)
			continue
		}
		if err := l.nats.Publish(ctx, smacknats.SubjectSynthesisExtract, data); err != nil {
			slog.Warn("lint: failed to re-publish artifact for synthesis", "artifact_id", a.ID, "error", err)
			continue
		}

		slog.Info("lint: re-published artifact for synthesis retry", "artifact_id", a.ID, "retry_count", a.RetryCount+1)
	}
}
