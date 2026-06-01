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

// ToolTrace is the persisted record of one tool invocation inside an
// open-knowledge agent turn. Operator-only visibility per design.
type ToolTrace struct {
	ID            string
	AgentAnswerID string
	Sequence      int
	ToolName      string
	Params        json.RawMessage
	ResultSummary json.RawMessage
	Error         string
	ExecutedAt    time.Time
}

// InsertToolTraces persists a batch of tool traces inside the
// caller-supplied transaction (typically the same tx that wrote the
// parent AgentAnswer). For standalone inserts use the *Pool variant
// via InsertAgentAnswer with the trace bundle.
func (ks *KnowledgeStore) InsertToolTraces(ctx context.Context, traces []*ToolTrace) error {
	if len(traces) == 0 {
		return nil
	}
	tx, err := ks.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("insert tool traces: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	for _, tr := range traces {
		if tr == nil {
			continue
		}
		if tr.ID == "" {
			tr.ID = ulid.Make().String()
		}
		if tr.ExecutedAt.IsZero() {
			tr.ExecutedAt = time.Now().UTC()
		}
		if err := insertToolTraceTx(ctx, tx, tr); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("insert tool traces: commit: %w", err)
	}
	committed = true
	return nil
}

func insertToolTraceTx(ctx context.Context, tx pgx.Tx, tr *ToolTrace) error {
	if tr.AgentAnswerID == "" {
		return errors.New("insert tool trace: empty agent_answer_id")
	}
	if tr.ToolName == "" {
		return errors.New("insert tool trace: empty tool_name")
	}
	params := tr.Params
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	resultSummary := tr.ResultSummary
	if len(resultSummary) == 0 {
		resultSummary = json.RawMessage("{}")
	}
	errText := nullableText(tr.Error)
	_, err := tx.Exec(ctx, `
		INSERT INTO tool_traces
			(id, agent_answer_id, sequence, tool_name, params, result_summary, error, executed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		tr.ID, tr.AgentAnswerID, tr.Sequence, tr.ToolName,
		params, resultSummary, errText, tr.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tool_traces row: %w", err)
	}
	return nil
}

// GetToolTraceByID returns the trace with the given id, or (nil, nil)
// if no row exists.
func (ks *KnowledgeStore) GetToolTraceByID(ctx context.Context, id string) (*ToolTrace, error) {
	row := ks.pool.QueryRow(ctx, `
		SELECT id, agent_answer_id, sequence, tool_name, params, result_summary,
		       COALESCE(error, ''), executed_at
		FROM tool_traces WHERE id = $1`, id)
	return scanToolTrace(row)
}

// ListToolTracesByAnswer returns the ordered list of tool traces
// recorded for the given AgentAnswer id.
func (ks *KnowledgeStore) ListToolTracesByAnswer(ctx context.Context, answerID string) ([]*ToolTrace, error) {
	rows, err := ks.pool.Query(ctx, `
		SELECT id, agent_answer_id, sequence, tool_name, params, result_summary,
		       COALESCE(error, ''), executed_at
		FROM tool_traces
		WHERE agent_answer_id = $1
		ORDER BY sequence ASC`, answerID)
	if err != nil {
		return nil, fmt.Errorf("list tool traces: %w", err)
	}
	defer rows.Close()
	var out []*ToolTrace
	for rows.Next() {
		tr, err := scanToolTrace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func scanToolTrace(row rowScanner) (*ToolTrace, error) {
	tr := &ToolTrace{}
	err := row.Scan(
		&tr.ID, &tr.AgentAnswerID, &tr.Sequence, &tr.ToolName,
		&tr.Params, &tr.ResultSummary, &tr.Error, &tr.ExecutedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan tool trace: %w", err)
	}
	return tr, nil
}
