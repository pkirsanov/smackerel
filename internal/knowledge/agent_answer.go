package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
)

// AgentAnswerLifecycle enumerates the lifecycle states a persisted
// AgentAnswer moves through. Default is `derived` (LowPriority graph
// weight per design). `promoted` is set when an operator surfaces the
// answer into the regular knowledge graph; `superseded` is set when a
// later answer for the same prompt replaces this one.
type AgentAnswerLifecycle string

const (
	AgentAnswerDerived    AgentAnswerLifecycle = "derived"
	AgentAnswerPromoted   AgentAnswerLifecycle = "promoted"
	AgentAnswerSuperseded AgentAnswerLifecycle = "superseded"
)

// AgentAnswerSourceKind is the cite-back source type recorded for
// each citation an AgentAnswer references.
type AgentAnswerSourceKind string

const (
	AgentAnswerSourceWeb       AgentAnswerSourceKind = "web"
	AgentAnswerSourceArtifact  AgentAnswerSourceKind = "artifact"
	AgentAnswerSourceToolTrace AgentAnswerSourceKind = "tool_computation"
)

// AgentAnswer is the persisted form of one open-knowledge agent turn.
type AgentAnswer struct {
	ID                string
	PromptArtifactID  string
	FinalText         string
	TerminationReason string
	TokensUsed        int
	USDSpent          float64
	CreatedAt         time.Time
	LifecycleState    AgentAnswerLifecycle
	PriorityWeight    float64
}

// AgentAnswerSource is one cite-back row linking an AgentAnswer to a
// single source (exactly one of WebSnippetID / ArtifactID /
// ToolTraceID is non-empty per design).
type AgentAnswerSource struct {
	ID            string
	AgentAnswerID string
	Kind          AgentAnswerSourceKind
	WebSnippetID  string
	ArtifactID    string
	ToolTraceID   string
	CreatedAt     time.Time
}

// AgentAnswerWrite bundles an AgentAnswer with its tool traces and
// cite-back sources. InsertAgentAnswer persists the bundle in a
// single transaction so partial-write scenarios cannot leave the
// graph inconsistent.
type AgentAnswerWrite struct {
	Answer  *AgentAnswer
	Traces  []*ToolTrace
	Sources []*AgentAnswerSource
}

// InsertAgentAnswer persists the AgentAnswer + ToolTraces + cite-back
// sources atomically. Each input slice element may have an empty ID;
// IDs are minted (ulid) and back-populated on success.
//
// G021: empty Sources is ALLOWED at the DB layer. The agent verifier
// is responsible for refusing answers without grounded sources;
// duplicating that gate in storage would force test fixtures to lie.
func (ks *KnowledgeStore) InsertAgentAnswer(ctx context.Context, write *AgentAnswerWrite) error {
	if write == nil || write.Answer == nil {
		return errors.New("insert agent answer: nil bundle")
	}
	a := write.Answer
	if a.PromptArtifactID == "" {
		return errors.New("insert agent answer: empty prompt_artifact_id")
	}
	if a.TerminationReason == "" {
		return errors.New("insert agent answer: empty termination_reason")
	}
	if a.ID == "" {
		a.ID = ulid.Make().String()
	}
	if a.LifecycleState == "" {
		a.LifecycleState = AgentAnswerDerived
	}
	if a.PriorityWeight == 0 {
		a.PriorityWeight = 0.3
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}

	tx, err := ks.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("insert agent answer: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_answers
			(id, prompt_artifact_id, final_text, termination_reason,
			 tokens_used, usd_spent, created_at, lifecycle_state, priority_weight)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.PromptArtifactID, a.FinalText, a.TerminationReason,
		a.TokensUsed, a.USDSpent, a.CreatedAt,
		string(a.LifecycleState), a.PriorityWeight,
	)
	if err != nil {
		return fmt.Errorf("insert agent answer row: %w", err)
	}

	for _, tr := range write.Traces {
		if tr == nil {
			continue
		}
		if tr.ID == "" {
			tr.ID = ulid.Make().String()
		}
		tr.AgentAnswerID = a.ID
		if tr.ExecutedAt.IsZero() {
			tr.ExecutedAt = time.Now().UTC()
		}
		if err := insertToolTraceTx(ctx, tx, tr); err != nil {
			return err
		}
	}

	for _, s := range write.Sources {
		if s == nil {
			continue
		}
		if s.ID == "" {
			s.ID = ulid.Make().String()
		}
		s.AgentAnswerID = a.ID
		if s.CreatedAt.IsZero() {
			s.CreatedAt = time.Now().UTC()
		}
		if err := insertAgentAnswerSourceTx(ctx, tx, s); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("insert agent answer: commit: %w", err)
	}
	committed = true
	return nil
}

func insertAgentAnswerSourceTx(ctx context.Context, tx pgx.Tx, s *AgentAnswerSource) error {
	if s.Kind == "" {
		return errors.New("insert agent answer source: empty kind")
	}
	// Translate empty strings to NULL so the partial FKs and the
	// "exactly one ref" CHECK constraint behave correctly.
	web := nullableText(s.WebSnippetID)
	art := nullableText(s.ArtifactID)
	trc := nullableText(s.ToolTraceID)
	_, err := tx.Exec(ctx, `
		INSERT INTO agent_answer_sources
			(id, agent_answer_id, source_kind, web_snippet_id, artifact_id, tool_trace_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		s.ID, s.AgentAnswerID, string(s.Kind), web, art, trc, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent_answer_sources row: %w", err)
	}
	return nil
}

// nullableText returns nil when s is empty so the DB stores NULL
// rather than empty-string for nullable FK columns.
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GetAgentAnswerByID returns the AgentAnswer (without traces/sources)
// or (nil, nil) if no row exists. Use GetAgentAnswerFull to fetch the
// bundle.
func (ks *KnowledgeStore) GetAgentAnswerByID(ctx context.Context, id string) (*AgentAnswer, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, prompt_artifact_id, final_text, termination_reason,
		       tokens_used, usd_spent, created_at, lifecycle_state, priority_weight
		FROM agent_answers WHERE id = $1`, id)
	return scanAgentAnswer(row)
}

// GetAgentAnswerFull returns the AgentAnswer together with its
// ToolTraces (ordered by sequence) and cite-back sources.
func (ks *KnowledgeStore) GetAgentAnswerFull(ctx context.Context, id string) (*AgentAnswerWrite, error) {
	a, err := ks.GetAgentAnswerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	traces, err := ks.ListToolTracesByAnswer(ctx, id)
	if err != nil {
		return nil, err
	}
	sources, err := ks.listAgentAnswerSources(ctx, id)
	if err != nil {
		return nil, err
	}
	return &AgentAnswerWrite{Answer: a, Traces: traces, Sources: sources}, nil
}

// ListAgentAnswersByPromptArtifact returns all answers derived for
// the given prompt artifact id, most recent first.
func (ks *KnowledgeStore) ListAgentAnswersByPromptArtifact(ctx context.Context, promptID string) ([]*AgentAnswer, error) {
	rows, err := ks.pool.Query(ctx, `
		SELECT id, prompt_artifact_id, final_text, termination_reason,
		       tokens_used, usd_spent, created_at, lifecycle_state, priority_weight
		FROM agent_answers
		WHERE prompt_artifact_id = $1
		ORDER BY created_at DESC`, promptID)
	if err != nil {
		return nil, fmt.Errorf("list agent answers: %w", err)
	}
	defer rows.Close()
	var out []*AgentAnswer
	for rows.Next() {
		a, err := scanAgentAnswer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (ks *KnowledgeStore) listAgentAnswerSources(ctx context.Context, answerID string) ([]*AgentAnswerSource, error) {
	rows, err := ks.pool.Query(ctx, `
		SELECT id, agent_answer_id, source_kind,
		       COALESCE(web_snippet_id, ''),
		       COALESCE(artifact_id, ''),
		       COALESCE(tool_trace_id, ''),
		       created_at
		FROM agent_answer_sources
		WHERE agent_answer_id = $1
		ORDER BY created_at ASC, id ASC`, answerID)
	if err != nil {
		return nil, fmt.Errorf("list agent answer sources: %w", err)
	}
	defer rows.Close()
	var out []*AgentAnswerSource
	for rows.Next() {
		s := &AgentAnswerSource{}
		var kind string
		if err := rows.Scan(&s.ID, &s.AgentAnswerID, &kind,
			&s.WebSnippetID, &s.ArtifactID, &s.ToolTraceID,
			&s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent answer source: %w", err)
		}
		s.Kind = AgentAnswerSourceKind(kind)
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanAgentAnswer(row rowScanner) (*AgentAnswer, error) {
	a := &AgentAnswer{}
	var state string
	err := row.Scan(
		&a.ID, &a.PromptArtifactID, &a.FinalText, &a.TerminationReason,
		&a.TokensUsed, &a.USDSpent, &a.CreatedAt,
		&state, &a.PriorityWeight,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan agent answer: %w", err)
	}
	a.LifecycleState = AgentAnswerLifecycle(state)
	return a, nil
}

// Ensure json import is retained for future use by serialisers
// (cite-back exporters consume json.RawMessage params/result_summary).
var _ = json.RawMessage(nil)
