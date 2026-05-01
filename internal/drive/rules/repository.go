package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRuleNotFound is returned when a rule lookup misses.
var ErrRuleNotFound = errors.New("rules: rule not found")

// AuditOutcome enumerates the possible drive_rule_audit.outcome values that
// the engine + save service may write. The DB CHECK constraint mirrors this
// list so adding a new outcome requires both a code and a schema change.
type AuditOutcome string

const (
	OutcomeMatched              AuditOutcome = "matched"
	OutcomeSkipped              AuditOutcome = "skipped"
	OutcomeConflict             AuditOutcome = "conflict"
	OutcomeFailed               AuditOutcome = "failed"
	OutcomeAwaitingConfirmation AuditOutcome = "awaiting_confirmation"
)

// AuditRow is one row read from drive_rule_audit.
type AuditRow struct {
	ID               int64
	RuleID           string
	SourceArtifactID string
	Outcome          AuditOutcome
	Reason           string
	CreatedAt        time.Time
}

// Repository provides CRUD + audit + listing operations on drive_rules and
// drive_rule_audit.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository backed by a Postgres connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new rule. The supplied rule.ID is ignored; the repository
// generates a UUID. The returned rule has CreatedAt and UpdatedAt populated
// from the DB row.
func (r *Repository) Create(ctx context.Context, rule Rule) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, errors.New("rules: repository pool not wired")
	}
	if err := rule.Validate(); err != nil {
		return Rule{}, err
	}
	rule.ID = uuid.NewString()
	guardrailJSON, err := json.Marshal(rule.Guardrails)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: marshal guardrails: %w", err)
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	_, err = r.pool.Exec(ctx,
		`INSERT INTO drive_rules (id, name, enabled, source_kinds, classification, sensitivity_in,
		                          confidence_min, provider_id, target_folder_template,
		                          on_missing_folder, on_existing_file, guardrails,
		                          created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		rule.ID, rule.Name, rule.Enabled, rule.SourceKinds, nullableText(rule.Classification), rule.SensitivityIn,
		rule.ConfidenceMin, rule.ProviderID, rule.TargetFolderTemplate,
		string(rule.OnMissingFolder), string(rule.OnExistingFile), guardrailJSON,
		rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: insert: %w", err)
	}
	return rule, nil
}

// Get returns one rule by ID.
func (r *Repository) Get(ctx context.Context, id string) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, errors.New("rules: repository pool not wired")
	}
	row := r.pool.QueryRow(ctx,
		`SELECT id, name, enabled, source_kinds, COALESCE(classification, ''), sensitivity_in,
		        confidence_min, provider_id, target_folder_template,
		        on_missing_folder, on_existing_file, guardrails, created_at, updated_at
		   FROM drive_rules WHERE id=$1`, id,
	)
	rule, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrRuleNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// List returns every rule, ordered by created_at ASC then id ASC for stable
// conflict resolution.
func (r *Repository) List(ctx context.Context) ([]Rule, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("rules: repository pool not wired")
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, enabled, source_kinds, COALESCE(classification, ''), sensitivity_in,
		        confidence_min, provider_id, target_folder_template,
		        on_missing_folder, on_existing_file, guardrails, created_at, updated_at
		   FROM drive_rules
		  ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("rules: list: %w", err)
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// Update replaces an existing rule.
func (r *Repository) Update(ctx context.Context, rule Rule) (Rule, error) {
	if r == nil || r.pool == nil {
		return Rule{}, errors.New("rules: repository pool not wired")
	}
	if rule.ID == "" {
		return Rule{}, errors.New("rules: update requires id")
	}
	if err := rule.Validate(); err != nil {
		return Rule{}, err
	}
	guardrailJSON, err := json.Marshal(rule.Guardrails)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: marshal guardrails: %w", err)
	}
	rule.UpdatedAt = time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE drive_rules
		    SET name=$2, enabled=$3, source_kinds=$4, classification=$5, sensitivity_in=$6,
		        confidence_min=$7, provider_id=$8, target_folder_template=$9,
		        on_missing_folder=$10, on_existing_file=$11, guardrails=$12, updated_at=$13
		  WHERE id=$1`,
		rule.ID, rule.Name, rule.Enabled, rule.SourceKinds, nullableText(rule.Classification), rule.SensitivityIn,
		rule.ConfidenceMin, rule.ProviderID, rule.TargetFolderTemplate,
		string(rule.OnMissingFolder), string(rule.OnExistingFile), guardrailJSON, rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Rule{}, ErrRuleNotFound
	}
	return r.Get(ctx, rule.ID)
}

// Delete removes a rule by ID.
func (r *Repository) Delete(ctx context.Context, id string) error {
	if r == nil || r.pool == nil {
		return errors.New("rules: repository pool not wired")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM drive_rules WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("rules: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// AppendAudit writes an audit row.
func (r *Repository) AppendAudit(ctx context.Context, ruleID, sourceArtifactID string, outcome AuditOutcome, reason string) error {
	if r == nil || r.pool == nil {
		return errors.New("rules: repository pool not wired")
	}
	if outcome == "" {
		return errors.New("rules: audit outcome required")
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO drive_rule_audit (rule_id, source_artifact_id, outcome, reason)
		 VALUES ($1, $2, $3, $4)`,
		nullableUUID(ruleID), nullableText(sourceArtifactID), string(outcome), reason,
	)
	if err != nil {
		return fmt.Errorf("rules: append audit: %w", err)
	}
	return nil
}

// ListAudit returns the most recent audit rows for a rule, newest first.
// limit MUST be positive; values <=0 are clamped to 50.
func (r *Repository) ListAudit(ctx context.Context, ruleID string, limit int) ([]AuditRow, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("rules: repository pool not wired")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(rule_id::text, ''), COALESCE(source_artifact_id, ''), outcome, reason, created_at
		   FROM drive_rule_audit
		  WHERE ($1 = '' OR rule_id::text = $1)
		  ORDER BY created_at DESC
		  LIMIT $2`,
		ruleID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("rules: list audit: %w", err)
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var row AuditRow
		var outcome string
		if err := rows.Scan(&row.ID, &row.RuleID, &row.SourceArtifactID, &outcome, &row.Reason, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.Outcome = AuditOutcome(outcome)
		out = append(out, row)
	}
	return out, rows.Err()
}

// rowScanner is the subset of pgx.Rows / pgx.Row needed by scanRule.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRule(row rowScanner) (Rule, error) {
	var (
		rule           Rule
		classification string
		guardrailJSON  []byte
		onMissing      string
		onExisting     string
	)
	if err := row.Scan(&rule.ID, &rule.Name, &rule.Enabled, &rule.SourceKinds, &classification, &rule.SensitivityIn,
		&rule.ConfidenceMin, &rule.ProviderID, &rule.TargetFolderTemplate,
		&onMissing, &onExisting, &guardrailJSON, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return Rule{}, err
	}
	rule.Classification = classification
	rule.OnMissingFolder = OnMissingFolder(onMissing)
	rule.OnExistingFile = OnExistingFile(onExisting)
	if len(guardrailJSON) > 0 {
		if err := json.Unmarshal(guardrailJSON, &rule.Guardrails); err != nil {
			return Rule{}, fmt.Errorf("rules: unmarshal guardrails: %w", err)
		}
	}
	return rule, nil
}

func nullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullableUUID(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
