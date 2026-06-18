//go:build stress

// Spec 076 SCOPE-2d — Open-Knowledge agent-loop p95 stress (TP-076-02-09).
//
// Drives the openknowledge.Agent loop under representative tool load
// (one calculator tool call followed by an end_turn with a verifiable
// tool_computation citation) and asserts the hot-path p95 SLA carried
// from the existing assistant Facade budget (5 ms per agent.Run).
//
// The LLM transport and tool execution are stubbed to sub-microsecond
// returns so the measurement reflects ONLY the agent-loop overhead:
// tool-registry lookup + dispatch, trace persistence (Nop writer),
// budget accounting, citation parsing, and the citeback verifier in
// enforce mode.

package stress

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

const (
	openKnowledgeStressTurnCount   = 500
	openKnowledgeStressWorkerCount = 16
	openKnowledgeStressP95BudgetMs = 5
)

// queuedLLM returns a programmed two-step sequence per Run:
// (1) tool_use → calculator(2+2), (2) end_turn with the verifiable
// tool_computation citation. A single instance is consumed by one
// agent.Run, so each turn allocates a fresh queuedLLM.
type queuedLLM struct {
	responses []llm.Result
	calls     int
}

func (q *queuedLLM) Chat(_ context.Context, _ llm.ChatRequest) (llm.Result, error) {
	if q.calls >= len(q.responses) {
		return llm.Result{}, fmt.Errorf("queuedLLM: queue exhausted at call %d", q.calls+1)
	}
	r := q.responses[q.calls]
	q.calls++
	return r, nil
}

func newQueuedLLM() *queuedLLM {
	final := "The answer is 4.\n<CITATIONS>[{\"kind\":\"tool_computation\",\"tool\":\"calculator\",\"input\":{\"expression\":\"2+2\"},\"output\":{\"result\":4}}]</CITATIONS>"
	return &queuedLLM{responses: []llm.Result{
		{
			StopReason: llm.StopToolUse,
			ToolCalls: []llm.ToolCall{{
				ID:        "c1",
				Name:      "calculator",
				Arguments: json.RawMessage(`{"expression":"2+2"}`),
			}},
			TokensUsed: 10,
		},
		{StopReason: llm.StopEndTurn, FinalText: final, TokensUsed: 20},
	}}
}

func newOpenKnowledgeStressRegistry() (*ok.Registry, error) {
	r := ok.NewRegistry([]string{"calculator"})
	if err := r.Register(tools.NewCalculator()); err != nil {
		return nil, fmt.Errorf("register calculator: %w", err)
	}
	return r, nil
}

func TestOpenKnowledge_P95SLAUnderToolLoad(t *testing.T) {
	if v := os.Getenv("STRESS_OPENKNOWLEDGE_TURNS"); v == "0" {
		t.Skip("stress: STRESS_OPENKNOWLEDGE_TURNS=0 — open-knowledge stress disabled")
	}
	turns := openKnowledgeStressTurnCount
	if v := os.Getenv("STRESS_OPENKNOWLEDGE_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			turns = n
		}
	}

	registry, err := newOpenKnowledgeStressRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	cfg := okagent.Config{
		SystemPrompt:               "stress-system-prompt",
		Model:                      "stress-model",
		SynthesisModel:             "stress-model",
		MaxIterations:              4,
		PerQueryTokenBudget:        1000,
		PerQueryUSDBudget:          1.0,
		MonthlyBudgetUSDRemaining:  1000.0,
		PerUserMonthlyUSDRemaining: 1000.0,
		CompactionThresholdRatio:   0.8,
		// Spec 096 SCOPE-05 — CostFn is now the model-aware seam.
		CostFn:          func(string, int) (float64, error) { return 0, nil },
		EnforcementMode: string(citeback.EnforcementEnforce),
	}

	latencies := make([]time.Duration, turns)
	work := make(chan int, turns)
	for i := 0; i < turns; i++ {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	for w := 0; w < openKnowledgeStressWorkerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := range work {
				ll := newQueuedLLM()
				a, err := okagent.New(ll, registry, citeback.Verify, cfg)
				if err != nil {
					t.Errorf("turn %d: New: %v", i, err)
					return
				}
				start := time.Now()
				res, err := a.Run(ctx, "what is 2+2")
				latencies[i] = time.Since(start)
				if err != nil {
					t.Errorf("turn %d: Run: %v", i, err)
					return
				}
				if res.Status != okagent.StatusSuccess {
					t.Errorf("turn %d: Status=%q termination=%q refusal=%q",
						i, res.Status, res.TerminationReason, res.RefusalReason)
					return
				}
			}
		}()
	}
	wg.Wait()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]

	t.Logf("Open-Knowledge agent loop — turns=%d workers=%d p50=%v p95=%v p99=%v max=%v",
		turns, openKnowledgeStressWorkerCount, p50, p95, p99, maxL)

	budget := time.Duration(openKnowledgeStressP95BudgetMs) * time.Millisecond
	if p95 > budget {
		t.Errorf("TP-076-02-09 budget breach: p95=%v exceeds budget=%v", p95, budget)
	}
}
