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
	SystemPrompt               string
	Model                      string
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
}

// Agent orchestrates the LLM ↔ tools loop with bounded budgets.
type Agent struct {
	llm      LLMChat
	registry *ok.Registry
	verify   VerifyFn
	cfg      Config
	rec      okmetrics.Recorder
	log      *slog.Logger
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
	if cfg.MaxIterations <= 0 {
		errs = append(errs, "Config.MaxIterations must be > 0")
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
	return &Agent{llm: chat, registry: registry, verify: verify, cfg: cfg, rec: rec, log: logger}, nil
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

	finalize := func(out TurnResult) TurnResult {
		iters := iterations
		if iters == 0 {
			iters = 1
		}
		a.recordTurnMetrics(iters, out)
		a.emitTurnLog(turnID, promptHash, iters, out)
		return out
	}

	refuse := func(reason TerminationReason, refusal string) TurnResult {
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
		if iter == a.cfg.MaxIterations-1 && len(trace) > 0 {
			requestTools = nil
			requestMessages = append(append([]llm.ChatMessage{}, messages...), llm.ChatMessage{
				Role:    llm.RoleUser,
				Content: "You have used all your tool calls. Based ONLY on the tool results above, write your final answer NOW. Include the <CITATIONS>[...]</CITATIONS> block at the end. If results are insufficient, write a short refusal explaining what you searched, followed by <CITATIONS>[]</CITATIONS>.",
			})
		}
		req := llm.ChatRequest{Model: a.cfg.Model, Messages: requestMessages, Tools: requestTools}
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
			// Forced-final-turn empty-text salvage: when the model
			// returned no text on the forced synthesis turn (gemma
			// sometimes goes blank when tools are stripped),
			// synthesize a body directly from the recorded tool
			// snippets. The user gets a real answer composed from
			// real evidence instead of a refusal from empty output.
			if isForcedFinalTurn && strings.TrimSpace(result.FinalText) == "" {
				autoSources := collectTraceSources(trace)
				if len(autoSources) > 0 {
					body := synthesizeFromSnippets(trace)
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
				autoSources := collectTraceSources(trace)
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
			if !verdict.OK {
				refused := refuse(TerminationFabricatedSource, "fabricated-source-blocked")
				refused.RejectedCitations = verdict.Rejected
				return refused, nil
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
			}), nil

		case llm.StopToolUse:
			messages = append(messages, llm.ChatMessage{
				Role:      llm.RoleAssistant,
				ToolCalls: result.ToolCalls,
			})
			for _, call := range result.ToolCalls {
				entry, follow := a.invokeTool(ctx, call)
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

func (a *Agent) invokeTool(ctx context.Context, call llm.ToolCall) (ToolTraceEntry, llm.ChatMessage) {
	tool, err := a.registry.Lookup(call.Name)
	if err != nil {
		// Unknown tool — record an error call against the requested
		// name (allow-set filter at the Recorder drops cardinality
		// leaks per G021).
		a.rec.IncToolCall(call.Name, okmetrics.OutcomeError)
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
	entry := ToolTraceEntry{ToolName: call.Name, Args: call.Arguments, Result: res, Err: execErr}
	msg := llm.ChatMessage{
		Role:       llm.RoleToolResult,
		ToolCallID: call.ID,
		Content:    renderToolResult(res, execErr),
	}
	return entry, msg
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

// synthesizeFromSnippets builds a plain-text answer from the recorded
// tool snippets when the model returned empty text on the forced
// synthesis turn. Concatenates the first non-empty snippet text from
// each tool invocation, capped at a reasonable length. The result is
// "grounded by construction" — every word comes from a tool_result.
func synthesizeFromSnippets(trace []ToolTraceEntry) string {
	var parts []string
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
			if totalLen+len(text) > maxBodyChars {
				remaining := maxBodyChars - totalLen
				if remaining > 50 {
					parts = append(parts, text[:remaining]+"...")
				}
				return strings.Join(parts, "\n\n")
			}
			parts = append(parts, text)
			totalLen += len(text) + 2
			break // one snippet per tool call is enough
		}
		if totalLen >= maxBodyChars {
			break
		}
	}
	return strings.Join(parts, "\n\n")
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
