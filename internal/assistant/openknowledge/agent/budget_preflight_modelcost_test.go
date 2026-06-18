// Spec 096 SCOPE-05 (SCN-096-G03) — load-bearing budget pre-flight unit
// coverage (ADVERSARIAL).
//
// Proves the model-aware budget pre-flight refuses a paid model whose
// month-to-date ledger spend + this turn's worst-case cost would breach the
// per-user OR global USD ceiling BEFORE any provider dispatch. The fake LLM
// carries an empty response queue, so ANY dispatch t.Fatalf's — the test
// fails if the provider call is reached before the ceiling check. A
// CONTROL case (within budget) proves the gate passes and dispatch IS
// reached, so the refusal is conditional, not unconditional (non-tautological).
// No DB, no network: a fake ledger supplies the month-to-date spend.
package agent

import (
	"context"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
)

// fakeSpendLedger is an in-memory SpendLedger: MonthToDateSpend returns canned
// per-user/global spend; AppendUsage records what would be written.
type fakeSpendLedger struct {
	perUser, global float64
	mtdCalls        int
	appended        []UsageRecord
}

func (f *fakeSpendLedger) MonthToDateSpend(context.Context) (float64, float64, error) {
	f.mtdCalls++
	return f.perUser, f.global, nil
}

func (f *fakeSpendLedger) AppendUsage(_ context.Context, u UsageRecord) error {
	f.appended = append(f.appended, u)
	return nil
}

func TestBudgetPreflight_PaidOverBudget_RefusesBeforeDispatch_Spec096(t *testing.T) {
	// Synthetic rate: output 15/1k → PerQueryTokenBudget=100 gives a
	// worst-case turn estimate of 100/1000 * 15 = $1.50.
	const paidModel = "anthropic/claude-3-5-sonnet"
	rates := map[string]ModelRate{
		paidModel: {InputUSDPer1k: 3.0, OutputUSDPer1k: 15.0},
	}

	overCases := []struct {
		name           string
		globalCeiling  float64
		perUserCeiling float64
		ledgerGlobal   float64
		ledgerPerUser  float64
		wantRefusalSub string
	}{
		{
			name:           "global month-to-date spend exhausted",
			globalCeiling:  10.0,
			perUserCeiling: 1000.0,
			ledgerGlobal:   9.5, // 9.5 + 1.5 = 11.0 > 10.0
			ledgerPerUser:  0,
			wantRefusalSub: ok.ErrCapUSDMonthly.Error(),
		},
		{
			name:           "per-user month-to-date spend exhausted",
			globalCeiling:  1000.0,
			perUserCeiling: 10.0,
			ledgerGlobal:   0,
			ledgerPerUser:  9.5, // 9.5 + 1.5 = 11.0 > 10.0
			wantRefusalSub: ok.ErrCapUSDPerUserMonth.Error(),
		},
	}
	for _, tc := range overCases {
		t.Run("refuses_before_dispatch/"+tc.name, func(t *testing.T) {
			ledger := &fakeSpendLedger{perUser: tc.ledgerPerUser, global: tc.ledgerGlobal}
			cfg := baseCfg(3, 100, 1000.0, tc.globalCeiling, tc.perUserCeiling, 0.85, nil)
			cfg.Model = paidModel
			cfg.SynthesisModel = paidModel
			cfg.CostFn = NewModelAwareCostFn(rates)
			cfg.SpendLedger = ledger

			// ADVERSARIAL: empty queue → the first Chat call t.Fatalf's, so
			// reaching dispatch fails the test.
			fl := &fakeLLM{t: t}
			r := newRegistry(t)
			a, err := New(fl, r, citeback.Verify, cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			got, err := a.Run(context.Background(), "anything")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fl.calls != 0 {
				t.Fatalf("LLM dispatched %d times; want 0 (the refusal MUST precede any billable provider call)", fl.calls)
			}
			if got.Status != StatusRefused {
				t.Fatalf("Status=%q want refused", got.Status)
			}
			if got.TerminationReason != TerminationCapUSD {
				t.Fatalf("TerminationReason=%q want cap_usd", got.TerminationReason)
			}
			if !strings.Contains(got.RefusalReason, tc.wantRefusalSub) {
				t.Fatalf("RefusalReason=%q want substring %q", got.RefusalReason, tc.wantRefusalSub)
			}
			if ledger.mtdCalls == 0 {
				t.Fatal("pre-flight never read the spend ledger; the budget cannot be load-bearing")
			}
			if len(ledger.appended) != 0 {
				t.Fatalf("ledger appended %d rows; want 0 (no spend recorded when refused before dispatch)", len(ledger.appended))
			}
		})
	}

	// CONTROL (non-tautological): within budget → the gate PASSES and dispatch
	// IS reached. Proves the gate refuses CONDITIONALLY, not always.
	t.Run("under_budget_reaches_dispatch", func(t *testing.T) {
		ledger := &fakeSpendLedger{perUser: 0, global: 0}
		cfg := baseCfg(1, 100, 1000.0, 1000.0, 1000.0, 0.85, nil)
		cfg.Model = paidModel
		cfg.SynthesisModel = paidModel
		cfg.CostFn = NewModelAwareCostFn(rates)
		cfg.SpendLedger = ledger

		// One programmed response so dispatch is observable; the turn's
		// terminal verdict is irrelevant — only that the LLM WAS reached.
		fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("answer without citations", 50)}}
		r := newRegistry(t)
		a, err := New(fl, r, citeback.Verify, cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, err := a.Run(context.Background(), "anything"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if fl.calls == 0 {
			t.Fatal("within-budget paid turn never dispatched; the budget gate must PASS when spend is within ceilings")
		}
	})
}
