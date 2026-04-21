package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// KnowledgeStore provides CRUD operations for the knowledge layer tables.
type KnowledgeStore struct {
	pool      *pgxpool.Pool
	MaxTokens int // Concept page token cap (design: 4000). 0 means no limit.
}

// NewKnowledgeStore creates a new KnowledgeStore backed by the given connection pool.
func NewKnowledgeStore(pool *pgxpool.Pool) *KnowledgeStore {
	return &KnowledgeStore{pool: pool}
}

// --- Concept CRUD ---

// InsertConcept creates a new concept page.
func (ks *KnowledgeStore) InsertConcept(ctx context.Context, concept *ConceptPage) error {
	if concept.ID == "" {
		concept.ID = ulid.Make().String()
	}
	concept.TitleNormalized = normalizeName(concept.Title)
	now := time.Now().UTC()
	concept.CreatedAt = now
	concept.UpdatedAt = now

	claims := concept.Claims
	if claims == nil {
		claims = json.RawMessage("[]")
	}

	_, err := ks.pool.Exec(ctx, `
		INSERT INTO knowledge_concepts
			(id, title, title_normalized, summary, claims, related_concept_ids,
			 source_artifact_ids, source_type_diversity, token_count,
			 prompt_contract_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		concept.ID, concept.Title, concept.TitleNormalized, concept.Summary,
		claims, concept.RelatedConceptIDs,
		concept.SourceArtifactIDs, concept.SourceTypeDiversity,
		concept.TokenCount, concept.PromptContractVersion,
		concept.CreatedAt, concept.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert concept: %w", err)
	}
	return nil
}

// GetConceptByID retrieves a concept page by its ID.
func (ks *KnowledgeStore) GetConceptByID(ctx context.Context, id string) (*ConceptPage, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, title, title_normalized, summary, claims, related_concept_ids,
		       source_artifact_ids, source_type_diversity, token_count,
		       prompt_contract_version, created_at, updated_at
		FROM knowledge_concepts WHERE id = $1`, id)
	return scanConcept(row)
}

// GetConceptByNormalizedTitle retrieves a concept page by normalized title (case-insensitive).
func (ks *KnowledgeStore) GetConceptByNormalizedTitle(ctx context.Context, title string) (*ConceptPage, error) {
	normalized := normalizeName(title)
	row := ks.pool.QueryRow(ctx, `
		SELECT id, title, title_normalized, summary, claims, related_concept_ids,
		       source_artifact_ids, source_type_diversity, token_count,
		       prompt_contract_version, created_at, updated_at
		FROM knowledge_concepts WHERE title_normalized = $1`, normalized)
	return scanConcept(row)
}

// UpdateConcept updates a concept page's mutable fields.
func (ks *KnowledgeStore) UpdateConcept(ctx context.Context, concept *ConceptPage) error {
	concept.TitleNormalized = normalizeName(concept.Title)
	concept.UpdatedAt = time.Now().UTC()

	claims := concept.Claims
	if claims == nil {
		claims = json.RawMessage("[]")
	}

	tag, err := ks.pool.Exec(ctx, `
		UPDATE knowledge_concepts SET
			title = $2, title_normalized = $3, summary = $4, claims = $5,
			related_concept_ids = $6, source_artifact_ids = $7,
			source_type_diversity = $8, token_count = $9,
			prompt_contract_version = $10, updated_at = $11
		WHERE id = $1`,
		concept.ID, concept.Title, concept.TitleNormalized, concept.Summary,
		claims, concept.RelatedConceptIDs,
		concept.SourceArtifactIDs, concept.SourceTypeDiversity,
		concept.TokenCount, concept.PromptContractVersion,
		concept.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update concept: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("concept not found: %s", concept.ID)
	}
	return nil
}

// SearchConcepts searches for concept pages using trigram similarity on title and summary.
// Returns the best match above the given threshold, or nil if no match.
func (ks *KnowledgeStore) SearchConcepts(ctx context.Context, query string, threshold float64) (*ConceptMatch, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, title, summary, array_length(source_artifact_ids, 1) AS citation_count,
		       source_type_diversity, updated_at,
		       GREATEST(similarity(title, $1), similarity(summary, $1)) AS match_score
		FROM knowledge_concepts
		WHERE similarity(title, $1) > $2 OR similarity(summary, $1) > $2
		ORDER BY match_score DESC LIMIT 1`, query, threshold)

	m := &ConceptMatch{}
	var citationCount *int
	err := row.Scan(&m.ConceptID, &m.Title, &m.Summary, &citationCount,
		&m.SourceTypes, &m.UpdatedAt, &m.MatchScore)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("search concepts: %w", err)
	}
	if citationCount != nil {
		m.CitationCount = *citationCount
	}
	return m, nil
}

// ListConceptsFiltered returns concept pages with optional query filter and sort.
// Supported sort values: "updated" (default), "citations", "title".
func (ks *KnowledgeStore) ListConceptsFiltered(ctx context.Context, q, sort string, limit, offset int) ([]*ConceptPage, int, error) {
	countQuery := "SELECT COUNT(*) FROM knowledge_concepts"
	dataQuery := `SELECT id, title, title_normalized, summary, claims, related_concept_ids,
		       source_artifact_ids, source_type_diversity, token_count,
		       prompt_contract_version, created_at, updated_at
		FROM knowledge_concepts`

	var args []interface{}
	argN := 1

	if q != "" {
		filter := fmt.Sprintf(" WHERE title ILIKE $%d OR summary ILIKE $%d", argN, argN)
		pattern := "%" + q + "%"
		countQuery += filter
		dataQuery += filter
		args = append(args, pattern)
		argN++
	}

	switch sort {
	case "citations":
		dataQuery += " ORDER BY array_length(source_artifact_ids, 1) DESC NULLS LAST"
	case "title":
		dataQuery += " ORDER BY title_normalized ASC"
	default:
		dataQuery += " ORDER BY updated_at DESC"
	}

	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)

	var total int
	if len(args) > 0 {
		err := ks.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count concepts: %w", err)
		}
	} else {
		err := ks.pool.QueryRow(ctx, countQuery).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count concepts: %w", err)
		}
	}

	dataArgs := append(args, limit, offset)
	rows, err := ks.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list concepts: %w", err)
	}
	defer rows.Close()

	var concepts []*ConceptPage
	for rows.Next() {
		c, err := scanConcept(rows)
		if err != nil {
			return nil, 0, err
		}
		concepts = append(concepts, c)
	}
	return concepts, total, rows.Err()
}

// ListConcepts returns concept pages ordered by updated_at descending.
func (ks *KnowledgeStore) ListConcepts(ctx context.Context, limit, offset int) ([]*ConceptPage, int, error) {
	return ks.ListConceptsFiltered(ctx, "", "updated", limit, offset)
}

// --- Entity CRUD ---

// InsertEntity creates a new entity profile.
func (ks *KnowledgeStore) InsertEntity(ctx context.Context, entity *EntityProfile) error {
	if entity.ID == "" {
		entity.ID = ulid.Make().String()
	}
	entity.NameNormalized = normalizeName(entity.Name)
	now := time.Now().UTC()
	entity.CreatedAt = now
	entity.UpdatedAt = now

	mentions := entity.Mentions
	if mentions == nil {
		mentions = json.RawMessage("[]")
	}

	_, err := ks.pool.Exec(ctx, `
		INSERT INTO knowledge_entities
			(id, name, name_normalized, entity_type, summary, mentions,
			 source_types, related_concept_ids, interaction_count,
			 people_id, prompt_contract_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entity.ID, entity.Name, entity.NameNormalized, entity.EntityType,
		entity.Summary, mentions,
		entity.SourceTypes, entity.RelatedConceptIDs,
		entity.InteractionCount, entity.PeopleID,
		entity.PromptContractVersion, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert entity: %w", err)
	}
	return nil
}

// GetEntityByID retrieves an entity profile by its ID.
func (ks *KnowledgeStore) GetEntityByID(ctx context.Context, id string) (*EntityProfile, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, name, name_normalized, entity_type, summary, mentions,
		       source_types, related_concept_ids, interaction_count,
		       people_id, prompt_contract_version, created_at, updated_at
		FROM knowledge_entities WHERE id = $1`, id)
	return scanEntity(row)
}

// GetEntityByNormalizedName retrieves an entity profile by normalized name and type (case-insensitive).
func (ks *KnowledgeStore) GetEntityByNormalizedName(ctx context.Context, name, entityType string) (*EntityProfile, error) {
	normalized := normalizeName(name)
	row := ks.pool.QueryRow(ctx, `
		SELECT id, name, name_normalized, entity_type, summary, mentions,
		       source_types, related_concept_ids, interaction_count,
		       people_id, prompt_contract_version, created_at, updated_at
		FROM knowledge_entities WHERE name_normalized = $1 AND entity_type = $2`, normalized, entityType)
	return scanEntity(row)
}

// UpdateEntity updates an entity profile's mutable fields.
func (ks *KnowledgeStore) UpdateEntity(ctx context.Context, entity *EntityProfile) error {
	entity.NameNormalized = normalizeName(entity.Name)
	entity.UpdatedAt = time.Now().UTC()

	mentions := entity.Mentions
	if mentions == nil {
		mentions = json.RawMessage("[]")
	}

	tag, err := ks.pool.Exec(ctx, `
		UPDATE knowledge_entities SET
			name = $2, name_normalized = $3, entity_type = $4, summary = $5,
			mentions = $6, source_types = $7, related_concept_ids = $8,
			interaction_count = $9, people_id = $10,
			prompt_contract_version = $11, updated_at = $12
		WHERE id = $1`,
		entity.ID, entity.Name, entity.NameNormalized, entity.EntityType,
		entity.Summary, mentions,
		entity.SourceTypes, entity.RelatedConceptIDs,
		entity.InteractionCount, entity.PeopleID,
		entity.PromptContractVersion, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update entity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entity not found: %s", entity.ID)
	}
	return nil
}

// ListEntitiesFiltered returns entity profiles with optional query filter and sort.
// Supported sort values: "updated" (default), "interactions", "name".
func (ks *KnowledgeStore) ListEntitiesFiltered(ctx context.Context, q, sort string, limit, offset int) ([]*EntityProfile, int, error) {
	countQuery := "SELECT COUNT(*) FROM knowledge_entities"
	dataQuery := `SELECT id, name, name_normalized, entity_type, summary, mentions,
		       source_types, related_concept_ids, interaction_count,
		       people_id, prompt_contract_version, created_at, updated_at
		FROM knowledge_entities`

	var args []interface{}
	argN := 1

	if q != "" {
		filter := fmt.Sprintf(" WHERE name ILIKE $%d OR summary ILIKE $%d", argN, argN)
		pattern := "%" + q + "%"
		countQuery += filter
		dataQuery += filter
		args = append(args, pattern)
		argN++
	}

	switch sort {
	case "interactions":
		dataQuery += " ORDER BY interaction_count DESC"
	case "name":
		dataQuery += " ORDER BY name_normalized ASC"
	default:
		dataQuery += " ORDER BY updated_at DESC"
	}

	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)

	var total int
	if len(args) > 0 {
		err := ks.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count entities: %w", err)
		}
	} else {
		err := ks.pool.QueryRow(ctx, countQuery).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count entities: %w", err)
		}
	}

	dataArgs := append(args, limit, offset)
	rows, err := ks.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	var entities []*EntityProfile
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, 0, err
		}
		entities = append(entities, e)
	}
	return entities, total, rows.Err()
}

// ListEntities returns entity profiles ordered by updated_at descending.
func (ks *KnowledgeStore) ListEntities(ctx context.Context, limit, offset int) ([]*EntityProfile, int, error) {
	return ks.ListEntitiesFiltered(ctx, "", "updated", limit, offset)
}

// --- Lint Report CRUD ---

// InsertLintReport stores a new lint report.
func (ks *KnowledgeStore) InsertLintReport(ctx context.Context, report *LintReport) error {
	if report.ID == "" {
		report.ID = ulid.Make().String()
	}
	now := time.Now().UTC()
	report.CreatedAt = now
	if report.RunAt.IsZero() {
		report.RunAt = now
	}

	findings := report.Findings
	if findings == nil {
		findings = json.RawMessage("[]")
	}
	summary := report.Summary
	if summary == nil {
		summary = json.RawMessage("{}")
	}

	_, err := ks.pool.Exec(ctx, `
		INSERT INTO knowledge_lint_reports (id, run_at, duration_ms, findings, summary, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		report.ID, report.RunAt, report.DurationMs, findings, summary, report.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert lint report: %w", err)
	}
	return nil
}

// GetLatestLintReport returns the most recent lint report.
func (ks *KnowledgeStore) GetLatestLintReport(ctx context.Context) (*LintReport, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, run_at, duration_ms, findings, summary, created_at
		FROM knowledge_lint_reports
		ORDER BY run_at DESC
		LIMIT 1`)

	r := &LintReport{}
	err := row.Scan(&r.ID, &r.RunAt, &r.DurationMs, &r.Findings, &r.Summary, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get latest lint report: %w", err)
	}
	return r, nil
}

// GetCrossSourceArtifacts returns artifacts from different source types that contribute
// to a given concept. Returns up to `limit` artifacts, one per source type, prioritizing
// the most recently created artifact from each type.
func (ks *KnowledgeStore) GetCrossSourceArtifacts(ctx context.Context, conceptID string, limit int) ([]CrossSourceArtifactData, error) {
	concept, err := ks.GetConceptByID(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("load concept for cross-source: %w", err)
	}
	if concept == nil || len(concept.SourceArtifactIDs) == 0 {
		return nil, nil
	}

	rows, err := ks.pool.Query(ctx, `
		SELECT DISTINCT ON (COALESCE(artifact_type, ''))
			id, COALESCE(title, ''), COALESCE(artifact_type, ''), COALESCE(summary, '')
		FROM artifacts
		WHERE id = ANY($1)
		ORDER BY COALESCE(artifact_type, ''), created_at DESC
		LIMIT $2`, concept.SourceArtifactIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("query cross-source artifacts: %w", err)
	}
	defer rows.Close()

	var result []CrossSourceArtifactData
	for rows.Next() {
		var a CrossSourceArtifactData
		if err := rows.Scan(&a.ID, &a.Title, &a.SourceType, &a.Summary); err != nil {
			return nil, fmt.Errorf("scan cross-source artifact: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// KnowledgeStats holds summary statistics for the knowledge layer.
type KnowledgeStats struct {
	ConceptCount          int        `json:"concept_count"`
	EntityCount           int        `json:"entity_count"`
	EdgeCount             int        `json:"edge_count"`
	SynthesisCompleted    int        `json:"synthesis_completed"`
	SynthesisPending      int        `json:"synthesis_pending"`
	SynthesisFailed       int        `json:"synthesis_failed"`
	LastSynthesisAt       *time.Time `json:"last_synthesis_at"`
	LintFindingsTotal     int        `json:"lint_findings_total"`
	LintFindingsHigh      int        `json:"lint_findings_high"`
	PromptContractVersion string     `json:"prompt_contract_version"`
}

// GetStats returns summary statistics for the knowledge layer.
func (ks *KnowledgeStore) GetStats(ctx context.Context) (*KnowledgeStats, error) {
	stats := &KnowledgeStats{}

	err := ks.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM knowledge_concepts),
			(SELECT COUNT(*) FROM knowledge_entities),
			(SELECT COUNT(*) FROM edges),
			(SELECT COUNT(*) FROM artifacts WHERE synthesis_status = 'completed'),
			(SELECT COUNT(*) FROM artifacts WHERE synthesis_status = 'pending'),
			(SELECT COUNT(*) FROM artifacts WHERE synthesis_status = 'failed'),
			(SELECT MAX(synthesis_at) FROM artifacts WHERE synthesis_status = 'completed'),
			(SELECT COALESCE(prompt_contract_version, '') FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1)`).Scan(
		&stats.ConceptCount, &stats.EntityCount, &stats.EdgeCount,
		&stats.SynthesisCompleted, &stats.SynthesisPending, &stats.SynthesisFailed,
		&stats.LastSynthesisAt, &stats.PromptContractVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("get knowledge stats: %w", err)
	}

	// Latest lint report summary
	report, err := ks.GetLatestLintReport(ctx)
	if err == nil && report != nil {
		var summary LintSummary
		if json.Unmarshal(report.Summary, &summary) == nil {
			stats.LintFindingsTotal = summary.Total
			stats.LintFindingsHigh = summary.High
		}
	}

	return stats, nil
}

// GetKnowledgeHealthStats returns the subset of knowledge stats needed by the health endpoint.
func (ks *KnowledgeStore) GetKnowledgeHealthStats(ctx context.Context) (*KnowledgeHealthStats, error) {
	stats := &KnowledgeHealthStats{}

	err := ks.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM knowledge_concepts),
			(SELECT COUNT(*) FROM knowledge_entities),
			(SELECT COUNT(*) FROM artifacts WHERE synthesis_status = 'pending'),
			(SELECT MAX(synthesis_at) FROM artifacts WHERE synthesis_status = 'completed')`).Scan(
		&stats.ConceptCount, &stats.EntityCount, &stats.SynthesisPending, &stats.LastSynthesisAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get knowledge health stats: %w", err)
	}

	return stats, nil
}

// --- Synthesis Status Queries ---

// ArtifactSynthesisStatusRow represents an artifact with its synthesis status for lint/retry queries.
type ArtifactSynthesisStatusRow struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	SynthesisStatus string `json:"synthesis_status"`
	SynthesisError  string `json:"synthesis_error"`
	RetryCount      int    `json:"retry_count"`
}

// GetArtifactsBySynthesisStatus returns artifacts matching any of the given synthesis_status values.
func (ks *KnowledgeStore) GetArtifactsBySynthesisStatus(ctx context.Context, statuses []string, limit int) ([]ArtifactSynthesisStatusRow, error) {
	rows, err := ks.pool.Query(ctx, `
		SELECT id, COALESCE(title, ''), synthesis_status, COALESCE(synthesis_error, ''), COALESCE(synthesis_retry_count, 0)
		FROM artifacts
		WHERE synthesis_status = ANY($1)
		ORDER BY created_at ASC
		LIMIT $2`, statuses, limit)
	if err != nil {
		return nil, fmt.Errorf("get artifacts by synthesis status: %w", err)
	}
	defer rows.Close()

	var result []ArtifactSynthesisStatusRow
	for rows.Next() {
		var a ArtifactSynthesisStatusRow
		if err := rows.Scan(&a.ID, &a.Title, &a.SynthesisStatus, &a.SynthesisError, &a.RetryCount); err != nil {
			return nil, fmt.Errorf("scan artifact synthesis status: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// StoreLintReport creates a new lint report from findings and duration.
func (ks *KnowledgeStore) StoreLintReport(ctx context.Context, findings []LintFinding, duration time.Duration) error {
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return fmt.Errorf("marshal lint findings: %w", err)
	}

	// Build summary counts
	summary := LintSummary{Total: len(findings)}
	for _, f := range findings {
		switch f.Severity {
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal lint summary: %w", err)
	}

	report := &LintReport{
		DurationMs: int(duration.Milliseconds()),
		Findings:   findingsJSON,
		Summary:    summaryJSON,
	}
	return ks.InsertLintReport(ctx, report)
}

// --- Helpers ---

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func scanConcept(row pgx.Row) (*ConceptPage, error) {
	c := &ConceptPage{}
	err := row.Scan(
		&c.ID, &c.Title, &c.TitleNormalized, &c.Summary, &c.Claims,
		&c.RelatedConceptIDs, &c.SourceArtifactIDs, &c.SourceTypeDiversity,
		&c.TokenCount, &c.PromptContractVersion, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan concept: %w", err)
	}
	return c, nil
}

func scanEntity(row pgx.Row) (*EntityProfile, error) {
	e := &EntityProfile{}
	err := row.Scan(
		&e.ID, &e.Name, &e.NameNormalized, &e.EntityType, &e.Summary,
		&e.Mentions, &e.SourceTypes, &e.RelatedConceptIDs,
		&e.InteractionCount, &e.PeopleID, &e.PromptContractVersion,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan entity: %w", err)
	}
	return e, nil
}
