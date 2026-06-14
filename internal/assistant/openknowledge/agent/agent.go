// Package agent — Spec 064 SCOPE-09 open-knowledge agent loop.
//
// Located in a sub-package to break the unavoidable import cycle: the
// citeback package already depends on openknowledge for SourceKind /
// Source / Computation types, so the agent (which depends on citeback
// to verify citations) cannot live in the openknowledge root.
//
// NO-DEFAULTS (G028): every cap, threshold, and model identifier comes
// from Config. Convergence cap (G082) and compaction trigger (G083)
// are enforced mechanically inside Run.
//
// Capture-as-fallback is NOT performed here. The Facade (SCOPE-13
// Telegram surface) is responsible for ALWAYS calling
// capture-as-fallback on the user prompt regardless of TurnResult.
package agent

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	okmetrics "github.com/smackerel/smackerel/internal/assistant/openknowledge/metrics"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tracewriter"
)

// SCOPE-12 (spec 064): the hard-coded SCOPE-09 placeholder prompt has
// been removed. The authoritative prompt now comes from
// config/prompt_contracts/open_knowledge.yaml and is loaded by cmd/core
// wiring into Config.SystemPrompt. The agent has no silent fallback
// (G028 / smackerel-no-defaults): an empty SystemPrompt fails New().

// Status is the terminal verdict for a single agent turn.
type Status string

const (
	StatusSuccess Status = "success"
	StatusRefused Status = "refused"
)

// TerminationReason is the typed cause attached to every TurnResult.
type TerminationReason string

const (
	TerminationFinal            TerminationReason = "final"
	TerminationCapIterations    TerminationReason = "cap_iterations"
	TerminationCapTokens        TerminationReason = "cap_tokens"
	TerminationCapUSD           TerminationReason = "cap_usd"
	TerminationToolError        TerminationReason = "tool_error"
	TerminationToolUnavailable  TerminationReason = "tool_unavailable"
	TerminationFabricatedSource TerminationReason = "fabricated_source"
	TerminationRefused          TerminationReason = "refused"
)

// ToolTraceEntry is one tool invocation recorded by the loop.
type ToolTraceEntry struct {
	ToolName string
	Args     json.RawMessage
	Result   *ok.ToolResult
	Err      error
}

// TurnResult is the agent loop's typed output.
type TurnResult struct {
	Status             Status
	FinalText          string
	Sources            []ok.Source
	ToolTrace          []ToolTraceEntry
	TerminationReason  TerminationReason
	TokensUsed         int
	USDSpent           float64
	CompactionSignaled bool
	RejectedCitations  []citeback.RejectedCitation
	RefusalReason      string
	// Model is the spec 088 attribution: the model of the turn that
	// actually produced the final text, stamped exactly once in the
	// finalize chokepoint. Honest across every terminal path (success,
	// honest-salvage, refuse, early-StopEndTurn). Empty on the rare
	// pre-loop refusal (no LLM round ran). Carried to the HTTP envelope
	// (always) and the Telegram footer (only-on-override) downstream.
	Model string
}

// LLMChat is the narrow contract the agent loop needs from the LLM
// bridge. *llm.Client satisfies it; tests inject a fake.
type LLMChat interface {
	Chat(ctx context.Context, req llm.ChatRequest) (llm.Result, error)
}

// CostFn maps a single LLM round-trip's TokensUsed to its USD cost.
// SCOPE-13 wires a function backed by SST pricing; the agent never
// hardcodes a $/token rate (G028).
type CostFn func(tokensUsed int) float64

// VerifyFn is the cite-back verifier contract. Pass citeback.Verify
// in production wiring; tests inject a fake.
type VerifyFn func(citations []citeback.Citation, trace citeback.ToolTrace) citeback.VerifyResult

// Config bundles the per-turn parameters the agent loop needs.
type Config struct {
	// SystemPrompt is the authoritative system prompt loaded from
	// config/prompt_contracts/open_knowledge.yaml. REQUIRED — empty is
	// rejected by New() per G028 (no silent default).
	SystemPrompt string
	Model        string
	// SynthesisModel / SynthesisRetryBudget — spec 087. The tools-
	// stripped forced-final synthesis turn (and its retries) uses
	// SynthesisModel (a reasoning model) instead of Model; an empty or
	// ungrounded forced-final is retried up to SynthesisRetryBudget
	// times with an escalated prompt before the honest snippet salvage.
	// SynthesisModel REQUIRED non-empty; SynthesisRetryBudget REQUIRED
	// >= 0 (0 = the exact spec-084 salvage timing).
	SynthesisModel             string
	SynthesisRetryBudget       int
	MaxIterations              int
	PerQueryTokenBudget        int
	PerQueryUSDBudget          float64
	MonthlyBudgetUSDRemaining  float64
	PerUserMonthlyUSDRemaining float64
	// CompactionThresholdRatio in (0,1]. The agent records
	// CompactionSignaled=true on TurnResult once TokensUsed >= ratio *
	// PerQueryTokenBudget. The actual context compaction is operator
	// tooling (separate scope); SCOPE-09 just signals.
	CompactionThresholdRatio float64
	CostFn                   CostFn
	// Recorder receives per-turn and per-tool-call metric events.
	// Optional — nil is replaced by metrics.Nop{} in New(). Tests
	// that don't care about metrics may leave this unset.
	Recorder okmetrics.Recorder
	// Logger receives one redacted INFO line per Run() turn with
	// turn_id, prompt SHA-256, tool-name+outcome trace, termination,
	// tokens, usd, iterations, num_sources. Optional — nil falls
	// back to slog.Default(). The raw prompt, raw LLM responses,
	// tool args, web snippets, and API keys are NEVER logged.
	Logger *slog.Logger
	// TraceWriter persists one row per tool invocation into
	// `assistant_tool_traces` (spec 076 SCOPE-2a). Optional — nil
	// falls back to tracewriter.Nop{} so tests and harnesses that
	// do not exercise persistence can leave it unset.
	TraceWriter tracewriter.Writer
	// EnforcementMode wires citeback verdicts to either log-only
	// (shadow) or refuse-with-capture (enforce). Spec 076 SCOPE-2c
	// (SCN-064-A06) — REQUIRED, no silent default (G028).
	EnforcementMode string
	// SourcesMax caps the number of sources the agent attaches to a
	// salvaged answer (BUG-064-002 DEFECT 3b). Sourced from the SST
	// key assistant.sources_max. REQUIRED — New() rejects a
	// non-positive value (G028 / smackerel-no-defaults; no silent
	// default).
	SourcesMax int
}

// Agent orchestrates the LLM ↔ tools loop with bounded budgets.
type Agent struct {
	llm         LLMChat
	registry    *ok.Registry
	verify      VerifyFn
	cfg         Config
	rec         okmetrics.Recorder
	log         *slog.Logger
	traces      tracewriter.Writer
	enforcement citeback.EnforcementMode
}

// ErrAgentInvalid is the construction error sentinel.
var ErrAgentInvalid = errors.New("openknowledge/agent: invalid config")

// ToolErrorCodeCircuitOpen is the well-known ToolResult.Error.Code
// the agent loop interprets as "provider circuit breaker is open;
// terminate the turn with TerminationToolUnavailable". The producer
// of this code is internal/assistant/openknowledge/tools/web_search.go
// via classifyProviderError on web.ErrCircuitOpen (SCOPE-16). The
// constant is duplicated here to keep the agent package free of an
// import on the tools package (which would form a cycle).
const ToolErrorCodeCircuitOpen = "provider_circuit_open"

// New validates inputs and returns a ready Agent.
func New(chat LLMChat, registry *ok.Registry, verify VerifyFn, cfg Config) (*Agent, error) {
	var errs []string
	if chat == nil {
		errs = append(errs, "llm chat is required")
	}
	if registry == nil {
		errs = append(errs, "registry is required")
	}
	if verify == nil {
		errs = append(errs, "verify is required")
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		errs = append(errs, "Config.SystemPrompt is required (G028 — no silent default; load from open_knowledge.yaml)")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		errs = append(errs, "Config.Model is required")
	}
	if strings.TrimSpace(cfg.SynthesisModel) == "" {
		errs = append(errs, "Config.SynthesisModel is required (G028 — no silent default; load from assistant.open_knowledge.synthesis_model_id)")
	}
	if cfg.MaxIterations <= 0 {
		errs = append(errs, "Config.MaxIterations must be > 0")
	}
	if cfg.SynthesisRetryBudget < 0 {
		errs = append(errs, "Config.SynthesisRetryBudget must be >= 0")
	}
	if cfg.PerQueryTokenBudget <= 0 {
		errs = append(errs, "Config.PerQueryTokenBudget must be > 0")
	}
	if cfg.PerQueryUSDBudget < 0 {
		errs = append(errs, "Config.PerQueryUSDBudget must be >= 0")
	}
	if cfg.MonthlyBudgetUSDRemaining < 0 {
		errs = append(errs, "Config.MonthlyBudgetUSDRemaining must be >= 0")
	}
	if cfg.PerUserMonthlyUSDRemaining < 0 {
		errs = append(errs, "Config.PerUserMonthlyUSDRemaining must be >= 0")
	}
	if cfg.CompactionThresholdRatio <= 0 || cfg.CompactionThresholdRatio > 1 {
		errs = append(errs, "Config.CompactionThresholdRatio must be in (0,1]")
	}
	if cfg.CostFn == nil {
		errs = append(errs, "Config.CostFn is required")
	}
	enforcementMode, modeErr := citeback.ParseEnforcementMode(cfg.EnforcementMode)
	if modeErr != nil {
		errs = append(errs, "Config.EnforcementMode: "+modeErr.Error())
	}
	if cfg.SourcesMax <= 0 {
		errs = append(errs, "Config.SourcesMax must be > 0 (G028 — no silent default; load from assistant.sources_max)")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrAgentInvalid, strings.Join(errs, "; "))
	}
	rec := cfg.Recorder
	if rec == nil {
		rec = okmetrics.Nop{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	traces := cfg.TraceWriter
	if traces == nil {
		traces = tracewriter.Nop{}
	}
	return &Agent{llm: chat, registry: registry, verify: verify, cfg: cfg, rec: rec, log: logger, traces: traces, enforcement: enforcementMode}, nil
}

// WithModelOverride returns a shallow, per-invocation copy of the Agent
// whose SynthesisModel is replaced by the (already-validated) override.
// Spec 088 — Fork B: the override re-points the spec-087 forced-final
// SYNTHESIS turn (and its retries) ONLY; the gather/tool turns keep the
// baseline Model. The receiver (the SST singleton installed at wiring)
// is NEVER mutated (C6 / build-once); a zero override returns the
// receiver unchanged so the no-override path is byte-for-byte identical
// to spec 087 (NFR-4). All deps (llm, registry, verify, rec, log,
// traces, enforcement) are concurrency-safe and shared by the clone.
func (a *Agent) WithModelOverride(o modelswitch.Override) *Agent {
	if o.IsZero() {
		return a
	}
	clone := *a
	clone.cfg.SynthesisModel = o.SynthesisModel
	return &clone
}

// Run executes the agent loop for a single user prompt. Budget caps,
// fabricated citations, and iteration caps are surfaced as
// Status+TerminationReason, not errors. A non-nil error is returned
// only for infrastructure-level failures (LLM transport, malformed
// sidecar response).
func (a *Agent) Run(ctx context.Context, userPrompt string) (TurnResult, error) {
	budget, err := ok.NewBudgetTracker(
		a.cfg.PerQueryTokenBudget,
		a.cfg.PerQueryUSDBudget,
		a.cfg.MonthlyBudgetUSDRemaining,
		a.cfg.PerUserMonthlyUSDRemaining,
	)
	if err != nil {
		return TurnResult{}, err
	}

	tools := a.buildLLMTools()
	messages := []llm.ChatMessage{
		{Role: llm.RoleSystem, Content: a.cfg.SystemPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	var trace []ToolTraceEntry
	iterations := 0
	turnID := newTurnID()
	promptHash := sha256Hex(userPrompt)

	// Spec 088 — answeringModel tracks the model of the turn currently
	// in flight so finalize can stamp TurnResult.Model with the model
	// that actually produced the final text (CT-3/CT-4). Init to the
	// gather/tool model; re-pointed to reqModel each iteration after the
	// per-turn model switch, and to SynthesisModel inside the synthesis-
	// retry loop. Captured by reference by the finalize closure below.
	answeringModel := a.cfg.Model

	finalize := func(out TurnResult) TurnResult {
		// Spec 088 — stamp the answering model exactly once. Every
		// terminal TurnResult routes through finalize (success, salvage,
		// refuse, early-StopEndTurn), so this single stamp is honest
		// across all paths. Respect an already-set Model (defensive).
		if out.Model == "" {
			out.Model = answeringModel
		}
		iters := iterations
		if iters == 0 {
			iters = 1
		}
		a.recordTurnMetrics(iters, out)
		a.emitTurnLog(turnID, promptHash, iters, out)
		return out
	}

	refuse := func(reason TerminationReason, refusal string) TurnResult {
		// Spec 076 SCOPE-2b DoD: budget-exhaustion / tool-failure /
		// tool-unavailable refusals must emit a `call_outcome=refused`
		// trace row through the SCOPE-2a writer. Pick the most recent
		// tool name from the trace for attribution; fall back to the
		// stable "agent" label for pre-flight refusals before any tool
		// dispatched. Lookup/exec failures already wrote a 'failed'
		// row from invokeTool; this row records the terminal refusal.
		switch reason {
		case TerminationCapTokens, TerminationCapUSD, TerminationToolError, TerminationToolUnavailable:
			toolName := "agent"
			if n := len(trace); n > 0 && trace[n-1].ToolName != "" {
				toolName = trace[n-1].ToolName
			}
			a.persistTrace(ctx, turnID, llm.ToolCall{Name: toolName}, tracewriter.OutcomeRefused, string(reason))
		}
		return finalize(TurnResult{
			Status:             StatusRefused,
			ToolTrace:          trace,
			TerminationReason:  reason,
			TokensUsed:         budget.TokensUsed(),
			USDSpent:           budget.USDSpent(),
			CompactionSignaled: a.compactionSignaled(budget),
			RefusalReason:      refusal,
		})
	}

	// Spec 076 SCOPE-2b — SCN-064-A08 per-user monthly budget
	// pre-flight. When the user has zero monthly USD remaining, refuse
	// BEFORE the first LLM round and BEFORE any tool dispatches.
	if a.cfg.PerUserMonthlyUSDRemaining <= 0 {
		return refuse(TerminationCapUSD, ok.ErrCapUSDPerUserMonth.Error()), nil
	}

	for iter := 0; iter < a.cfg.MaxIterations; iter++ {
		iterations = iter + 1
		// Final-turn forcing: on the LAST iteration, strip tools from
		// the request and inject a synthesizer reminder so the model
		// is forced to produce a text answer instead of another
		// tool_call (which would hit cap_iterations). Without this,
		// gemma-class models tend to keep searching indefinitely
		// rather than synthesizing from prior results.
		requestTools := tools
		requestMessages := messages
		reqModel := a.cfg.Model
		switch {
		case iter == a.cfg.MaxIterations-1 && len(trace) > 0:
			requestTools = nil
			// Spec 087 — the forced-final SYNTHESIS turn uses the reasoning
			// synthesis model (tools are already stripped here, so the
			// synthesis model's weaker tool-calling is irrelevant) and a
			// structured "reason then write the verdict now" prompt. The
			// prompt preserves the spec-084 "write your final answer NOW"
			// trigger phrase.
			reqModel = a.cfg.SynthesisModel
			requestMessages = append(append([]llm.ChatMessage{}, messages...), llm.ChatMessage{
				Role:    llm.RoleUser,
				Content: synthesisFinalPrompt,
			})
		case iter == a.cfg.MaxIterations-2 && len(trace) > 0:
			// Spec 084 (CHANGE 2) — reflect-before-final nudge. On the
			// second-to-last iteration, prompt the model to check that its
			// evidence actually answers the question and covers every part /
			// side, and to spend its last tool-calling turn filling a gap if
			// one remains. Question-agnostic (examples, not a closed list).
			// Ephemeral (mirrors the forced-final pattern): appended to a copy
			// of messages so it never pollutes the running history, within the
			// existing iteration budget, no new model/dependency.
			requestMessages = append(append([]llm.ChatMessage{}, messages...), llm.ChatMessage{
				Role:    llm.RoleUser,
				Content: "Before you give your final answer: re-read the question and check whether the evidence you have actually answers what was asked, and whether you have covered every part of it — for a comparison, every option; for a why/how question, the mechanism; for a recommendation, the deciding criteria. If a needed piece is still missing, issue ONE more targeted tool call now to fill that specific gap. If your evidence already answers the question, proceed to your final answer.",
			})
		}
		// Spec 088 — record the model this turn runs on so finalize can
		// attribute the answer to it: the forced-final SYNTHESIS turn (and
		// its retries) reports SynthesisModel (the overridden model under a
		// per-request override, Fork B); every gather/tool turn reports the
		// baseline Model.
		answeringModel = reqModel
		req := llm.ChatRequest{Model: reqModel, Messages: requestMessages, Tools: requestTools}
		result, err := a.llm.Chat(ctx, req)
		if err != nil {
			return TurnResult{}, fmt.Errorf("openknowledge/agent: llm chat: %w", err)
		}

		costUSD := a.cfg.CostFn(result.TokensUsed)
		// Sidecar reports combined TokensUsed; charge as completion.
		if capErr := budget.RecordLLMCall(0, result.TokensUsed, costUSD); capErr != nil {
			return refuse(mapCapErr(capErr), capErr.Error()), nil
		}

		switch result.StopReason {
		case llm.StopEndTurn:
			isForcedFinalTurn := iter == a.cfg.MaxIterations-1
			// Spec 087 — strip the synthesis model's <think> chain-of-
			// thought BEFORE any parsing / salvage / cite-back so it can
			// never reach the user body or become a citation. No-op for
			// non-reasoning models (no <think> present).
			result.FinalText = stripThinkBlocks(result.FinalText)
			// Spec 087 — retry-before-salvage. On the forced-final
			// synthesis turn, an empty or ungrounded-excuse result is
			// re-synthesized with an escalated prompt (same synthesis
			// model, tools stripped) up to SynthesisRetryBudget times
			// before the honest snippet salvage fires. Each retry is a
			// budgeted LLM call counted as an iteration.
			if isForcedFinalTurn {
				for retry := 0; retry < a.cfg.SynthesisRetryBudget && synthesisNeedsRetry(result.FinalText); retry++ {
					iterations++
					// Spec 088 — the retry runs on SynthesisModel (the
					// overridden model under an override); attribute the
					// answer to it even when the forced-final turn itself
					// did not re-point reqModel (empty-trace edge case).
					answeringModel = a.cfg.SynthesisModel
					retryMessages := append(append([]llm.ChatMessage{}, messages...), llm.ChatMessage{
						Role:    llm.RoleUser,
						Content: synthesisRetryPrompt,
					})
					retryResult, retryErr := a.llm.Chat(ctx, llm.ChatRequest{Model: a.cfg.SynthesisModel, Messages: retryMessages, Tools: nil})
					if retryErr != nil {
						return TurnResult{}, fmt.Errorf("openknowledge/agent: llm chat (synthesis retry %d): %w", retry+1, retryErr)
					}
					if capErr := budget.RecordLLMCall(0, retryResult.TokensUsed, a.cfg.CostFn(retryResult.TokensUsed)); capErr != nil {
						return refuse(mapCapErr(capErr), capErr.Error()), nil
					}
					retryResult.FinalText = stripThinkBlocks(retryResult.FinalText)
					result = retryResult
					if !synthesisNeedsRetry(result.FinalText) {
						break
					}
				}
			}
			// Forced-final-turn empty-text salvage: when the model
			// returned no text on the forced synthesis turn (gemma
			// sometimes goes blank when tools are stripped),
			// synthesize a body directly from the recorded tool
			// snippets. The user gets a real answer composed from
			// real evidence instead of a refusal from empty output.
			if isForcedFinalTurn && strings.TrimSpace(result.FinalText) == "" {
				autoSources := a.cappedTraceSources(trace)
				if len(autoSources) > 0 {
					body := honestSalvageBody(trace)
					if body != "" {
						return finalize(TurnResult{
							Status:             StatusSuccess,
							FinalText:          body,
							Sources:            autoSources,
							ToolTrace:          trace,
							TerminationReason:  TerminationFinal,
							TokensUsed:         budget.TokensUsed(),
							USDSpent:           budget.USDSpent(),
							CompactionSignaled: a.compactionSignaled(budget),
						}), nil
					}
				}
			}
			finalText, citations, parseErr := parseCitations(result.FinalText)
			if parseErr != nil {
				// Forced-final-turn missing-CITATIONS salvage: when
				// the model wrote a text answer on the last iteration
				// but forgot the <CITATIONS> block, treat the
				// recorded tool-trace sources as the citation set.
				// The text is grounded (the forced-turn prompt is
				// "answer based ONLY on tool results above"), so
				// refusing as fabricated_source would punish a
				// well-formed answer for a formatting omission.
				autoSources := a.cappedTraceSources(trace)
				trimmedText := strings.TrimSpace(result.FinalText)
				if isForcedFinalTurn && len(trace) > 0 && len(autoSources) > 0 && trimmedText != "" {
					return finalize(TurnResult{
						Status:             StatusSuccess,
						FinalText:          trimmedText,
						Sources:            autoSources,
						ToolTrace:          trace,
						TerminationReason:  TerminationFinal,
						TokensUsed:         budget.TokensUsed(),
						USDSpent:           budget.USDSpent(),
						CompactionSignaled: a.compactionSignaled(budget),
					}), nil
				}
				return refuse(TerminationFabricatedSource, parseErr.Error()), nil
			}
			verdict := a.verify(citations, toolTraceForVerifier(trace))
			decision := citeback.Decide(verdict, a.enforcement)
			if decision.Mismatch {
				a.log.Info("openknowledge_citeback_mismatch",
					"turn_id", turnID,
					"prompt_sha", promptHash,
					"mode", string(a.enforcement),
					"rejected_count", len(verdict.Rejected),
					"refused", decision.Refuse,
				)
			}
			if decision.Refuse {
				refused := refuse(TerminationFabricatedSource, "fabricated-source-blocked")
				refused.RejectedCitations = verdict.Rejected
				return refused, nil
			}
			// Empty-citations salvage: when the model wrote a real
			// text answer but emitted <CITATIONS>[]</CITATIONS> (or
			// otherwise produced zero verified citations) AND we
			// have tool-trace sources from successful web_search /
			// internal_retrieval calls, attach those as the source
			// set. The text is grounded by construction (the system
			// prompt requires answers based on tool results); the
			// downstream provenance gate refuses any zero-source
			// response, so without this the user gets "I don't
			// have a sourced answer" even after 3 successful tool
			// calls. Fires on any iteration, not just the forced
			// final one, because nothing about the empty-citations
			// failure mode is forced-turn-specific.
			if len(verdict.Verified) == 0 && strings.TrimSpace(finalText) != "" {
				autoSources := a.cappedTraceSources(trace)
				if len(autoSources) > 0 {
					// Body-quality salvage: if the model wrote a
					// "no tool results were provided" / "I am unable
					// to" body but tools DID run and DID return
					// content, the body is a lie. Replace it with
					// real snippet text so the user sees actual
					// evidence. The trace+sources prove it.
					salvageBody := finalText
					if isUngroundedExcuse(finalText) {
						if syn := honestSalvageBody(trace); syn != "" {
							salvageBody = syn
						}
					}
					return finalize(TurnResult{
						Status:             StatusSuccess,
						FinalText:          salvageBody,
						Sources:            autoSources,
						ToolTrace:          trace,
						TerminationReason:  TerminationFinal,
						TokensUsed:         budget.TokensUsed(),
						USDSpent:           budget.USDSpent(),
						CompactionSignaled: a.compactionSignaled(budget),
						RejectedCitations:  verdict.Rejected,
					}), nil
				}
			}
			return finalize(TurnResult{
				Status:             StatusSuccess,
				FinalText:          finalText,
				Sources:            verdict.Verified,
				ToolTrace:          trace,
				TerminationReason:  TerminationFinal,
				TokensUsed:         budget.TokensUsed(),
				USDSpent:           budget.USDSpent(),
				CompactionSignaled: a.compactionSignaled(budget),
				RejectedCitations:  verdict.Rejected,
			}), nil

		case llm.StopToolUse:
			messages = append(messages, llm.ChatMessage{
				Role:      llm.RoleAssistant,
				ToolCalls: result.ToolCalls,
			})
			for _, call := range result.ToolCalls {
				entry, follow := a.invokeTool(ctx, turnID, call)
				trace = append(trace, entry)
				messages = append(messages, follow)
				// SCOPE-16 — circuit-open from a tool means the
				// underlying provider is unhealthy and the breaker
				// is short-circuiting. Retrying inside the same turn
				// cannot help, so terminate with a typed reason that
				// maps to RefusalToolUnavailable downstream. The
				// budget event still fires below for accounting
				// parity with normal tool calls.
				if entry.Result != nil && entry.Result.Error != nil && entry.Result.Error.Code == ToolErrorCodeCircuitOpen {
					return refuse(TerminationToolUnavailable, entry.Result.Error.Message), nil
				}
				if capErr := budget.RecordToolCall(call.Name, 0); capErr != nil {
					return refuse(mapCapErr(capErr), capErr.Error()), nil
				}
			}

		default:
			return refuse(TerminationToolError, fmt.Sprintf("unknown stop_reason %q", result.StopReason)), nil
		}
	}

	return refuse(TerminationCapIterations, "max iterations exceeded"), nil
}

func (a *Agent) invokeTool(ctx context.Context, turnID string, call llm.ToolCall) (ToolTraceEntry, llm.ChatMessage) {
	tool, err := a.registry.Lookup(call.Name)
	if err != nil {
		// Unknown tool — record an error call against the requested
		// name (allow-set filter at the Recorder drops cardinality
		// leaks per G021).
		a.rec.IncToolCall(call.Name, okmetrics.OutcomeError)
		a.persistTrace(ctx, turnID, call, tracewriter.OutcomeFailed, "tool_lookup")
		entry := ToolTraceEntry{ToolName: call.Name, Args: call.Arguments, Err: err}
		msg := llm.ChatMessage{
			Role:       llm.RoleToolResult,
			ToolCallID: call.ID,
			Content:    fmt.Sprintf(`{"error":{"code":"tool_lookup","message":%q}}`, err.Error()),
		}
		return entry, msg
	}
	start := time.Now()
	res, execErr := tool.Execute(ctx, call.Arguments)
	a.rec.ObserveToolLatency(call.Name, time.Since(start).Seconds())
	outcome := okmetrics.OutcomeSuccess
	if execErr != nil || (res != nil && res.Error != nil) {
		outcome = okmetrics.OutcomeError
	}
	a.rec.IncToolCall(call.Name, outcome)
	callOutcome := tracewriter.OutcomeSucceeded
	errCode := ""
	if execErr != nil {
		callOutcome = tracewriter.OutcomeFailed
		errCode = "exec_error"
	} else if res != nil && res.Error != nil {
		callOutcome = tracewriter.OutcomeFailed
		errCode = res.Error.Code
	}
	a.persistTrace(ctx, turnID, call, callOutcome, errCode)
	entry := ToolTraceEntry{ToolName: call.Name, Args: call.Arguments, Result: res, Err: execErr}
	msg := llm.ChatMessage{
		Role:       llm.RoleToolResult,
		ToolCallID: call.ID,
		Content:    renderToolResult(res, execErr),
	}
	return entry, msg
}

// persistTrace writes one redacted `assistant_tool_traces` row for
// the given call. Errors are logged at WARN; persistence failure
// must not break the turn (the in-memory ToolTrace is still the
// authoritative trace for the citeback verifier).
func (a *Agent) persistTrace(ctx context.Context, turnID string, call llm.ToolCall, outcome tracewriter.CallOutcome, errCode string) {
	keys := argKeysFromJSON(call.Arguments)
	err := a.traces.Write(ctx, tracewriter.Entry{
		TurnID:      turnID,
		ToolName:    call.Name,
		ArgKeys:     keys,
		CallOutcome: outcome,
		ErrorCode:   errCode,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		a.log.Warn("openknowledge.tool_trace_persist_failed",
			slog.String("turn_id", turnID),
			slog.String("tool_name", call.Name),
			slog.String("outcome", string(outcome)),
			slog.String("err", err.Error()),
		)
	}
}

// argKeysFromJSON returns the top-level keys of a JSON object as a
// sorted slice. Values are dropped — the redacted payload records
// shape only. Non-object inputs return an empty slice.
func argKeysFromJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func renderToolResult(res *ok.ToolResult, execErr error) string {
	if execErr != nil {
		return fmt.Sprintf(`{"error":{"code":"execute","message":%q}}`, execErr.Error())
	}
	if res == nil {
		return `{"error":{"code":"nil_result","message":"tool returned nil result"}}`
	}
	if res.Error != nil {
		return fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, res.Error.Code, res.Error.Message)
	}
	out := map[string]any{"snippets": res.Snippets}
	if res.Computation != nil {
		out["computation"] = map[string]any{
			"tool":   res.Computation.Tool,
			"input":  json.RawMessage(res.Computation.Input),
			"output": json.RawMessage(res.Computation.Output),
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf(`{"error":{"code":"marshal","message":%q}}`, err.Error())
	}
	return string(b)
}

func (a *Agent) buildLLMTools() []llm.Tool {
	enabled := a.registry.Enabled()
	out := make([]llm.Tool, 0, len(enabled))
	for _, t := range enabled {
		out = append(out, llm.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.ParamsSchema(),
		})
	}
	return out
}

func (a *Agent) compactionSignaled(b *ok.BudgetTracker) bool {
	threshold := int(float64(b.PerQueryTokenBudget()) * a.cfg.CompactionThresholdRatio)
	return b.TokensUsed() >= threshold
}

func mapCapErr(err error) TerminationReason {
	switch {
	case errors.Is(err, ok.ErrCapTokens):
		return TerminationCapTokens
	case errors.Is(err, ok.ErrCapUSDPerQuery),
		errors.Is(err, ok.ErrCapUSDMonthly),
		errors.Is(err, ok.ErrCapUSDPerUserMonth):
		return TerminationCapUSD
	default:
		return TerminationRefused
	}
}

func toolTraceForVerifier(trace []ToolTraceEntry) citeback.ToolTrace {
	out := make(citeback.ToolTrace, 0, len(trace))
	for _, e := range trace {
		if e.Result == nil {
			continue
		}
		out = append(out, citeback.ToolInvocation{
			ToolName:        e.ToolName,
			RecordedSources: e.Result.Sources,
		})
	}
	return out
}

// collectTraceSources flattens all ok.Source entries recorded by tool
// invocations into a deduplicated slice (by Kind + locator). Used by
// the forced-final-turn salvage path when the model produced a text
// answer without an explicit <CITATIONS> block.
// truncatePreview returns the first n characters of s with an ellipsis
// suffix when truncated. Used for log-only previews.
func truncatePreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// synthesisFinalPrompt is the spec 087 structured forced-final synthesis
// instruction. It preserves the spec-084 "write your final answer NOW"
// trigger phrase (asserted by the reasoning-loop tests) while adding the
// reason-then-verdict scaffold (Axis C): the model must REASON over the
// gathered evidence and write the ACTUAL answer (a comparison verdict, a
// causal explanation, or a recommendation), reconciling contradictions —
// NOT a per-source recap. Used on the tools-stripped forced-final turn,
// which spec 087 routes to the reasoning synthesis model.
const synthesisFinalPrompt = "You have used all your tool calls; do NOT search again. Based ONLY on the tool results above, REASON over the evidence and write your final answer NOW as a direct verdict — the actual answer to the question. For a comparison, say which option is better and WHY, drawing the specific evidence for each option and reconciling any contradiction (prefer the more specific or authoritative source) instead of restating both sides. For a why/how question, give the mechanism. For a recommendation, give the choice and the deciding factors. Write it in your own words, NOT a per-source recap. Include the <CITATIONS>[...]</CITATIONS> block at the end (one entry per cited source, copied verbatim from a tool_result). If the evidence is genuinely insufficient, write a short honest sentence explaining what you searched, followed by <CITATIONS>[]</CITATIONS>."

// synthesisRetryPrompt is the spec 087 escalated retry instruction issued
// when the first forced-final synthesis came back empty or as an ungrounded
// excuse. It targets the reasoning model's "emitted a <think> block but
// never concluded" failure mode: output ONLY the verdict, no <think>, no
// preamble, no apology.
const synthesisRetryPrompt = "Your previous attempt did not produce a usable answer, but you already have all the evidence you need in the tool results above. Output ONLY your final answer now — a direct verdict, in your own words, that resolves the question — with NO <think> block, NO preamble, and NO apology. End with the <CITATIONS>[...]</CITATIONS> block, one entry per cited source copied verbatim from a tool_result. Only if the evidence genuinely cannot answer the question, write one short honest sentence and then <CITATIONS>[]</CITATIONS>."

// thinkBlockRE matches a closed <think>...</think> reasoning block (dotall,
// non-greedy). strayOpenThinkRE matches a trailing UNCLOSED <think> (a
// reasoning model that "thought" but never concluded — everything from the
// stray <think> to end is dropped, leaving empty text that the forced-final
// retry / salvage path then handles).
var thinkBlockRE = regexp.MustCompile(`(?s)<think>.*?</think>`)
var strayOpenThinkRE = regexp.MustCompile(`(?s)<think>.*$`)

// stripThinkBlocks removes every <think>...</think> block (and a trailing
// unclosed <think>) from s and trims surrounding whitespace. Spec 087 —
// reasoning models (deepseek-r1 and similar) emit a <think> chain-of-thought
// BEFORE the answer; it is stripped before parseCitations and cite-back so
// it can never reach the user body or become a citation. A no-op for
// non-reasoning models (no <think> present). Mirrors the
// ml/app/processor.py reasoning-preamble strip.
func stripThinkBlocks(s string) string {
	if !strings.Contains(s, "<think>") {
		return s
	}
	s = thinkBlockRE.ReplaceAllString(s, "")
	s = strayOpenThinkRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// synthesisNeedsRetry reports whether a forced-final synthesis result
// (post-<think>-strip) is empty or an ungrounded excuse and should be
// retried with an escalated prompt before the honest snippet salvage fires
// (spec 087). A genuine answer — even one missing its CITATIONS block — is
// NOT retried; the existing salvage paths attach trace sources to it.
func synthesisNeedsRetry(finalText string) bool {
	if strings.TrimSpace(finalText) == "" {
		return true
	}
	return isUngroundedExcuse(finalText)
}

// honestSalvagePrefix frames a snippet-salvaged body as raw findings so the
// user is never shown a stitched snippet wall dressed up as a reasoned answer
// (spec 084 CHANGE 4 / Principle 8 — Trust Through Transparency). It is applied
// ONLY when genuine synthesis did not happen and the platform falls back to
// synthesizeFromSnippets — never on the genuine cited-synthesis happy path.
const honestSalvagePrefix = "I searched but couldn't directly answer your question. Here is the most relevant information I found:"

// honestSalvageBody wraps synthesizeFromSnippets(trace) with the honest frame.
// Returns "" when there are no snippets to salvage, so callers fall through to
// their normal empty-body handling (e.g. a canonical refusal).
func honestSalvageBody(trace []ToolTraceEntry) string {
	syn := synthesizeFromSnippets(trace)
	if syn == "" {
		return ""
	}
	return honestSalvagePrefix + "\n\n" + syn
}

// synthesizeFromSnippets builds a plain-text answer from the recorded
// tool snippets when the model returned empty text on the forced
// synthesis turn. Concatenates the first non-empty, NON-DUPLICATE
// snippet text from each tool invocation, capped at a reasonable
// length. The result is "grounded by construction" — every word comes
// from a tool_result.
//
// BUG-064-002 DEFECT 2: multiple web_search calls for the same question
// routinely return the same top snippet; without de-duplication the
// identical block was emitted once per tool call (the live triplicate
// dump). The dedup key normalises whitespace + case so spacing/case
// variants of the same snippet collapse to one block.
func synthesizeFromSnippets(trace []ToolTraceEntry) string {
	var parts []string
	seen := make(map[string]struct{})
	totalLen := 0
	const maxBodyChars = 1500
	for _, e := range trace {
		if e.Result == nil {
			continue
		}
		for _, snip := range e.Result.Snippets {
			text := strings.TrimSpace(snip.Text)
			if text == "" {
				continue
			}
			key := snippetDedupKey(text)
			if _, dup := seen[key]; dup {
				continue // already included this snippet; try the next one
			}
			seen[key] = struct{}{}
			if totalLen+len(text) > maxBodyChars {
				remaining := maxBodyChars - totalLen
				if remaining > 50 {
					parts = append(parts, text[:remaining]+"...")
				}
				return strings.Join(parts, "\n\n")
			}
			parts = append(parts, text)
			totalLen += len(text) + 2
			break // one UNIQUE snippet per tool call is enough
		}
		if totalLen >= maxBodyChars {
			break
		}
	}
	return strings.Join(parts, "\n\n")
}

// snippetDedupKey normalises snippet text (lowercase + collapsed
// whitespace) so snippets that differ only in spacing or case are
// treated as duplicates by synthesizeFromSnippets (BUG-064-002
// DEFECT 2).
func snippetDedupKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// collectTraceSources flattens all ok.Source entries recorded by tool
// invocations into a deduplicated slice (by Kind + locator). Used by
// the forced-final-turn salvage path when the model produced a text
// answer without an explicit <CITATIONS> block.
func collectTraceSources(trace []ToolTraceEntry) []ok.Source {
	seen := make(map[string]struct{})
	out := make([]ok.Source, 0)
	for _, e := range trace {
		if e.Result == nil {
			continue
		}
		for _, s := range e.Result.Sources {
			key := ""
			switch s.Kind {
			case ok.SourceWeb:
				if s.Web == nil {
					continue
				}
				key = "web|" + s.Web.URL + "|" + s.Web.ContentHash
			case ok.SourceArtifact:
				if s.Artifact == nil {
					continue
				}
				key = "artifact|" + s.Artifact.ID
			default:
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// cappedTraceSources returns collectTraceSources(trace) bounded to
// cfg.SourcesMax (BUG-064-002 DEFECT 3b). collectTraceSources already
// de-duplicates by (kind, locator, content_hash); this caps the
// salvaged set to the SST assistant.sources_max so a salvaged answer
// never carries an absurd number of arbitrary trace sources (the live
// num_sources=32). The non-salvage path attaches the verified cited
// set (verdict.Verified) instead and is unaffected.
func (a *Agent) cappedTraceSources(trace []ToolTraceEntry) []ok.Source {
	srcs := collectTraceSources(trace)
	if a.cfg.SourcesMax > 0 && len(srcs) > a.cfg.SourcesMax {
		srcs = srcs[:a.cfg.SourcesMax]
	}
	return srcs
}

// citationsBlockRE matches the trailing <CITATIONS>...</CITATIONS>
// block emitted by the agent system prompt.
//
// Citation-extraction strategy rationale: the LLM bridge contract
// (ChatResponse.text) is a single string, so the LLM cannot return a
// structured citations[] alongside text without changing the schema
// (a SCOPE-12 task). The simplest provable approach for SCOPE-09 is a
// fenced trailing block the prompt mandates and a regex+JSON parser
// validates. Answers without the block are treated as fabricated
// (TerminationFabricatedSource); the parser strips the block from the
// returned FinalText so callers see the answer only.
var citationsBlockRE = regexp.MustCompile(`(?s)<CITATIONS>\s*(\[.*?\])\s*</CITATIONS>\s*$`)

type rawCitation struct {
	Kind        string          `json:"kind"`
	ArtifactID  string          `json:"artifact_id,omitempty"`
	URL         string          `json:"url,omitempty"`
	ContentHash string          `json:"content_hash,omitempty"`
	Tool        string          `json:"tool,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
}

func parseCitations(finalText string) (string, []citeback.Citation, error) {
	m := citationsBlockRE.FindStringSubmatchIndex(finalText)
	if m == nil {
		return "", nil, fmt.Errorf("final answer missing trailing <CITATIONS>...</CITATIONS> block")
	}
	jsonStart, jsonEnd := m[2], m[3]
	blockStart := m[0]
	raw := finalText[jsonStart:jsonEnd]
	stripped := strings.TrimRight(finalText[:blockStart], " \t\r\n")

	var rawList []rawCitation
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rawList); err != nil {
		return "", nil, fmt.Errorf("CITATIONS block not valid JSON: %w", err)
	}
	out := make([]citeback.Citation, 0, len(rawList))
	for i, rc := range rawList {
		c, err := convertCitation(rc)
		if err != nil {
			return "", nil, fmt.Errorf("citation[%d]: %w", i, err)
		}
		out = append(out, c)
	}
	return stripped, out, nil
}

func convertCitation(rc rawCitation) (citeback.Citation, error) {
	switch rc.Kind {
	case "artifact":
		return citeback.Citation{Kind: ok.SourceArtifact, ArtifactID: rc.ArtifactID}, nil
	case "web":
		return citeback.Citation{Kind: ok.SourceWeb, URL: rc.URL, ContentHash: rc.ContentHash}, nil
	case "tool_computation":
		return citeback.Citation{Kind: ok.SourceToolComputation, Tool: rc.Tool, Input: rc.Input, Output: rc.Output}, nil
	default:
		return citeback.Citation{}, fmt.Errorf("unknown citation kind %q", rc.Kind)
	}
}

// ── SCOPE-14 observability helpers ──────────────────────────────────

// terminationToRefusalCause is a local mirror of the agenttool
// mapping (the two packages cannot share without an import cycle —
// agenttool already imports this package). The contract is asserted
// in agenttool/substrate_tool_test.go; this helper feeds the
// openknowledge_refusal_total counter only.
func terminationToRefusalCause(r TerminationReason) string {
	switch r {
	case TerminationCapIterations, TerminationCapTokens, TerminationCapUSD:
		return "budget_exhausted"
	case TerminationToolError, TerminationToolUnavailable:
		return "tool_unavailable"
	case TerminationFabricatedSource:
		return "fabricated_source_blocked"
	case TerminationRefused, TerminationFinal:
		return "default"
	default:
		return "default"
	}
}

// terminationToBudgetScope reports the budget scope for cap-hit
// terminations, "" otherwise.
func terminationToBudgetScope(r TerminationReason) string {
	switch r {
	case TerminationCapIterations:
		return "iterations"
	case TerminationCapTokens:
		return "tokens"
	case TerminationCapUSD:
		return "usd"
	default:
		return ""
	}
}

// recordTurnMetrics emits the per-turn histograms and any
// termination-specific counters. USD is converted to cents to match
// the openknowledge_usd_cents_per_query histogram bucket layout.
func (a *Agent) recordTurnMetrics(iterations int, out TurnResult) {
	a.rec.RecordTurn(iterations, out.TokensUsed, out.USDSpent*100.0)
	if out.CompactionSignaled {
		a.rec.IncCompactionSignaled()
	}
	if scope := terminationToBudgetScope(out.TerminationReason); scope != "" {
		a.rec.IncBudgetExhausted(scope)
	}
	if out.TerminationReason == TerminationFabricatedSource {
		a.rec.IncFabricatedSource()
	}
	if out.Status == StatusRefused {
		a.rec.IncRefusal(terminationToRefusalCause(out.TerminationReason))
	}
}

// emitTurnLog writes a single structured INFO line per turn. The
// payload is engineered for redaction: the raw prompt is replaced by
// its SHA-256 hex, tool args are dropped, tool results reduced to
// {name, outcome}. No field carries an API key, full URL, or web
// snippet body.
func (a *Agent) emitTurnLog(turnID, promptHash string, iterations int, out TurnResult) {
	calls := make([]map[string]string, 0, len(out.ToolTrace))
	for _, e := range out.ToolTrace {
		outcome := "success"
		if e.Err != nil || (e.Result != nil && e.Result.Error != nil) {
			outcome = "error"
		}
		calls = append(calls, map[string]string{
			"name":    e.ToolName,
			"outcome": outcome,
		})
	}
	a.log.Info("openknowledge.turn",
		slog.String("turn_id", turnID),
		slog.String("prompt_sha256", promptHash),
		slog.Int("iterations", iterations),
		slog.Int("tokens_used", out.TokensUsed),
		slog.Float64("usd_spent", out.USDSpent),
		slog.String("status", string(out.Status)),
		slog.String("termination_reason", string(out.TerminationReason)),
		slog.Int("num_sources", len(out.Sources)),
		slog.Bool("compaction_signaled", out.CompactionSignaled),
		slog.Any("tool_calls", calls),
		slog.String("refusal_reason", out.RefusalReason),
	)
}

// newTurnID returns a 16-hex turn identifier (8 random bytes). The
// value carries no user input and is safe to log.
func newTurnID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return "no-rng"
	}
	return hex.EncodeToString(b[:])
}

// sha256Hex returns lowercase hex of SHA-256(s). Used to derive a
// stable non-reversible identifier for the user prompt.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// hostOnly returns just the host of a URL, used by future log sites
// that need to reference a web URL without leaking path/query
// (which can carry tokens or PII). The current Run loop logs no URL
// fields; this helper keeps the redaction primitive available.
//
//nolint:unused // intentional API surface for future log sites
func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "invalid_url"
	}
	return u.Host
}

// isUngroundedExcuse returns true when the model's final text reads as
// an "I cannot answer because tools failed" excuse, even though the
// tool trace shows that tools DID return real content. llama3.1:8b
// (and similar mid-size models) often write apologetic refusals after
// the forced-final turn even when they have evidence in the message
// history. When this fires AND the trace has sources, the empty-
// citations salvage replaces the body with a snippet synthesis so the
// user sees the real evidence instead of the model's lie.
//
// Returns false for genuine answers (so we never replace a good body).
// The match set is intentionally conservative: it targets phrases the
// model produces ONLY when it claims no evidence, not phrases that
// might appear in a real grounded answer.
func isUngroundedExcuse(text string) bool {
	lower := strings.ToLower(text)
	excusePhrases := []string{
		"no tool results were provided",
		"no tool results provided",
		"no search tools were executed",
		"no search tools executed",
		"i was unable to find",
		"i am unable to find",
		"i'm unable to find",
		"i cannot provide",
		"i can't provide",
		"i do not have access to",
		"i don't have access to",
		"i was unable to retrieve",
		"i am unable to retrieve",
		"i was unable to gather",
		"unable to ground",
		"to ground the answer",
		"to ground this answer",
		"sorry, i could not find",
		"sorry, i couldn't find",
		"no results were returned",
		"no relevant results",
		"the search returned no",
		"no information was found",
		"i was not able to find",
	}
	for _, p := range excusePhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
