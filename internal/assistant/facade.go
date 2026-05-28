// Spec 061 SCOPE-04 — capability-layer facade.
//
// Facade implements contracts.Assistant.Handle. One Handle call drives
// exactly one inbound turn through the spec 061 capability machine:
//
//   1. Load conversation state for (UserID, Transport).
//   2. Resolve reference expressions ("that one", "open 2") against
//      the most recent ContextTurn — short-circuit with ErrSlotMissing
//      when the reference cannot be resolved.
//   3. Detect a slash-shortcut prefix:
//        - /reset            → DeleteByKey + acknowledgement; STOP.
//        - /ask|/weather|... → set IntentEnvelope.ScenarioID so the
//                              router takes the explicit-id fast path.
//   4. Call agent.Router.Route — receives RoutingDecision + ok.
//   5. Run the borderline post-processor → Band (high|borderline|low).
//   6. Dispatch on band:
//        - high       → manifest enable-gate → executor.Run →
//                       Invocation→Response translation →
//                       provenance.Enforce.
//        - borderline → emit DisambiguationPrompt (≤3 enabled choices
//                       + always-last save_as_note sentinel).
//        - low        → emit Status=StatusSavedAsIdea + CaptureRoute=true.
//   7. Append a ContextTurn (bounded by cfg.WindowTurns) and persist
//      the conversation row.
//   8. Audit the turn (fire-and-forget at the contract boundary).
//
// Spec 061 BS-005 invariant: NO transport-specific branching anywhere
// in this file. The fakeTransportAdapter in facade_test.go panics on
// every adapter method except Identity() and proves the facade never
// reaches into the adapter.

package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
	"github.com/smackerel/smackerel/internal/assistant/provenance"
)

// ScenarioExecutor is the small interface the facade depends on so
// tests can substitute a stub for *agent.Executor. The single method
// mirrors agent.Executor.Run exactly.
type ScenarioExecutor interface {
	Run(ctx context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult
}

// ScenarioRegistry resolves a scenario id to its parsed *agent.Scenario.
// The router holds these internally but does not expose a public
// lookup; the facade needs one to call executor.Run on the chosen id.
// In production this is satisfied by a thin wrapper over the loader's
// scenario slice; in tests it is a map literal.
type ScenarioRegistry interface {
	Scenario(scenarioID string) (*agent.Scenario, bool)
}

// FacadeConfig carries the SST-resolved values the facade consults on
// every turn. ALL fields are required; the constructor returns an
// error when any required field is zero.
type FacadeConfig struct {
	// BorderlineFloor is the score below which a routing decision
	// is classified borderline. From assistant.borderline_floor (SCOPE-01).
	BorderlineFloor float64

	// AgentConfidenceFloor is the score below which a routing
	// decision is classified low. From spec 037 RoutingConfig.
	AgentConfidenceFloor float64

	// SourcesMax bounds AssistantResponse.Sources.
	// From assistant.sources_max (SCOPE-01).
	SourcesMax int

	// BodyMaxChars bounds AssistantResponse.Body. From
	// assistant.body_max_chars (SCOPE-01).
	BodyMaxChars int

	// WindowTurns bounds the WorkingContext history kept per
	// conversation. From assistant.context.window_turns (SCOPE-01).
	WindowTurns int

	// DisambigMaxChoices bounds the non-save_as_note Choices in a
	// DisambiguationPrompt. From assistant.disambig_max_choices
	// (SCOPE-01). The save_as_note sentinel is appended on TOP of
	// this cap.
	DisambigMaxChoices int

	// DisambigTimeout is the per-prompt TTL. From
	// assistant.disambig_timeout (SCOPE-01).
	DisambigTimeout time.Duration

	// Now overrides the clock. Tests inject this for deterministic
	// EmittedAt; production passes time.Now.
	Now func() time.Time

	// SourceAssemblers is the per-scenario source-assembly registry
	// the facade consults in BandHigh dispatch between executor.Run
	// and provenance.Enforce. The map is keyed by scenario id
	// (matching the manifest metadata key). nil/empty map is the
	// supported "no assemblers wired" state — the facade then
	// produces resp.Sources=nil for every scenario, and the
	// provenance gate refuses every requires_provenance scenario
	// (correct behavior when no real assembler has been wired).
	//
	// Spec 061 SCOPE-04 (facade-source-assembly-hook). For
	// retrieval_qa wiring see cmd/core/wiring_assistant_facade.go.
	// For per-scenario assembler implementations see
	// internal/agent/tools/<skill>/.
	SourceAssemblers map[string]contracts.SourceAssembler
}

// Validate enforces the required-field contract.
func (c FacadeConfig) Validate() error {
	if c.BorderlineFloor <= 0 {
		return errors.New("assistant: FacadeConfig.BorderlineFloor must be > 0")
	}
	if c.AgentConfidenceFloor < 0 {
		return errors.New("assistant: FacadeConfig.AgentConfidenceFloor must be >= 0")
	}
	if c.AgentConfidenceFloor > c.BorderlineFloor {
		return errors.New("assistant: FacadeConfig requires AgentConfidenceFloor <= BorderlineFloor")
	}
	if c.SourcesMax <= 0 {
		return errors.New("assistant: FacadeConfig.SourcesMax must be > 0")
	}
	if c.BodyMaxChars <= 0 {
		return errors.New("assistant: FacadeConfig.BodyMaxChars must be > 0")
	}
	if c.WindowTurns <= 0 {
		return errors.New("assistant: FacadeConfig.WindowTurns must be > 0")
	}
	if c.DisambigMaxChoices <= 0 {
		return errors.New("assistant: FacadeConfig.DisambigMaxChoices must be > 0")
	}
	if c.DisambigTimeout <= 0 {
		return errors.New("assistant: FacadeConfig.DisambigTimeout must be > 0")
	}
	if c.Now == nil {
		return errors.New("assistant: FacadeConfig.Now must be set")
	}
	return nil
}

// Facade implements contracts.Assistant.
type Facade struct {
	cfg              FacadeConfig
	router           agent.Router
	executor         ScenarioExecutor
	registry         ScenarioRegistry
	manifest         *SkillsManifest
	contextStore     assistantctx.Store
	audit            AuditWriter
	sourceAssemblers map[string]contracts.SourceAssembler
}

// NewFacade constructs a Facade. All non-Now config fields and every
// pointer dependency are required.
func NewFacade(
	cfg FacadeConfig,
	router agent.Router,
	executor ScenarioExecutor,
	registry ScenarioRegistry,
	manifest *SkillsManifest,
	contextStore assistantctx.Store,
	audit AuditWriter,
) (*Facade, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if router == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil router")
	}
	if executor == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil executor")
	}
	if registry == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil scenario registry")
	}
	if manifest == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil manifest")
	}
	if contextStore == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil context store")
	}
	if audit == nil {
		return nil, errors.New("assistant: NewFacade requires a non-nil audit writer")
	}
	return &Facade{
		cfg:              cfg,
		router:           router,
		executor:         executor,
		registry:         registry,
		manifest:         manifest,
		contextStore:     contextStore,
		audit:            audit,
		sourceAssemblers: cfg.SourceAssemblers,
	}, nil
}

// Handle implements contracts.Assistant.
//
// The flow is intentionally linear (no transport-keyed branching).
// Every short-circuit path still ends with a persist + audit so the
// conversation row stays coherent.
func (f *Facade) Handle(ctx context.Context, msg contracts.AssistantMessage) (resp contracts.AssistantResponse, err error) {
	if msg.UserID == "" || msg.Transport == "" {
		return contracts.AssistantResponse{}, errors.New("assistant: AssistantMessage requires UserID and Transport")
	}

	// Spec 061 SCOPE-09 — facade-level metrics + structured log.
	// Capture turn start; the deferred closure reads the named
	// return values (resp, err) and derives the outcome label,
	// then emits FacadeTurnsTotal + FacadeLatencySeconds + one
	// slog.Info per turn. Any panic still records OutcomeError
	// because the closure runs on unwind regardless.
	turnStart := f.cfg.Now()
	transportLabel := normalizeTransportLabel(msg.Transport)
	// Spec 061 design §18.6 — correlation_id propagation. The Telegram
	// adapter stamps TransportMetadata["telegram_update_id"]; other
	// adapters (web/mobile) stamp their own transport-native id. When
	// no metadata is present we fall back to assistant_turn_id so the
	// slog line always carries SOMETHING that uniquely identifies this
	// turn (shell e2e fixtures inject a unique nonce in update_id).
	correlationID := ""
	if msg.TransportMetadata != nil {
		correlationID = msg.TransportMetadata["telegram_update_id"]
	}
	var (
		turnBand            Band
		turnScenarioID      string
		turnTopScore        float64
		turnAssistantTurnID string
	)
	defer func() {
		latency := f.cfg.Now().Sub(turnStart).Seconds()
		var outcome string
		if err != nil {
			outcome = assistantmetrics.OutcomeError
		} else {
			outcome = deriveFacadeOutcome(resp)
		}
		assistantmetrics.FacadeTurnsTotal.WithLabelValues(transportLabel, outcome).Inc()
		assistantmetrics.FacadeLatencySeconds.WithLabelValues(transportLabel, outcome).Observe(latency)
		// Fallback correlation_id when transport supplied none.
		effectiveCorrelationID := correlationID
		if effectiveCorrelationID == "" {
			effectiveCorrelationID = turnAssistantTurnID
		}
		// Spec 061 design §8.2 + §18.5 — one structured log line per turn
		// with the canonical shell-e2e assertion field set: correlation_id,
		// scenario_id, status, error_cause, user_id, transport,
		// body_redacted (Principle 8 affirmation; bodies never logged).
		slog.Info("assistant_turn",
			"user_id", msg.UserID,
			"transport", transportLabel,
			"correlation_id", effectiveCorrelationID,
			"assistant_turn_id", turnAssistantTurnID,
			"scenario_id", turnScenarioID,
			"top_score", turnTopScore,
			"band", string(turnBand),
			"status", string(resp.Status),
			"error_cause", string(resp.ErrorCause),
			"latency_ms", latency*1000,
			"agent_trace_id", agentTraceID(turnAssistantTurnID),
			"body_redacted", true,
		)
	}()

	conv, _, loadErr := f.contextStore.Load(ctx, msg.UserID, msg.Transport)
	if loadErr != nil {
		return contracts.AssistantResponse{}, fmt.Errorf("assistant: load context: %w", loadErr)
	}

	emittedAt := f.cfg.Now()

	// --- Step 1: /reset short-circuit ---
	if msg.Kind == contracts.KindReset {
		if derr := f.contextStore.DeleteByKey(ctx, msg.UserID, msg.Transport); derr != nil {
			return contracts.AssistantResponse{}, fmt.Errorf("assistant: reset: %w", derr)
		}
		resp = contracts.AssistantResponse{
			Status:    contracts.StatusSavedAsIdea,
			Body:      "context reset.",
			EmittedAt: emittedAt,
		}
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return resp, nil
	}

	// --- Step 2: shortcut detection (text turns only) ---
	var shortcutScenarioID string
	if msg.Kind == contracts.KindText {
		scenarioID, isReset, ok := LookupShortcut(msg.Text)
		if ok {
			if isReset {
				if derr := f.contextStore.DeleteByKey(ctx, msg.UserID, msg.Transport); derr != nil {
					return contracts.AssistantResponse{}, fmt.Errorf("assistant: reset via shortcut: %w", derr)
				}
				resp = contracts.AssistantResponse{
					Status:    contracts.StatusSavedAsIdea,
					Body:      "context reset.",
					EmittedAt: emittedAt,
				}
				f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
				return resp, nil
			}
			shortcutScenarioID = scenarioID
		}
	}

	// --- Step 3: reference resolution (text turns only) ---
	if msg.Kind == contracts.KindText && shortcutScenarioID == "" {
		if ref := assistantctx.ResolveReference(msg.Text, conv.WorkingContext); ref.Outcome == assistantctx.ResolveOutcomeMissing {
			body := "cannot resolve reference."
			if ref.AvailableSources > 0 {
				body = fmt.Sprintf("cannot resolve reference. last result has %d sources.", ref.AvailableSources)
			} else if len(conv.WorkingContext.Turns) == 0 {
				body = "cannot resolve reference. no prior result in this conversation."
			}
			resp = contracts.AssistantResponse{
				Status:     contracts.StatusUnavailable,
				ErrorCause: contracts.ErrSlotMissing,
				Body:       body,
				EmittedAt:  emittedAt,
			}
			conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
			f.writeAudit(ctx, msg, "", nil, nil, resp)
			return resp, nil
		}
	}

	// --- Step 4: build envelope + route ---
	env := agent.IntentEnvelope{
		Source:     msg.Transport,
		RawInput:   msg.Text,
		ScenarioID: shortcutScenarioID,
	}
	chosen, decision, ok := f.router.Route(ctx, env)

	// --- Step 5: borderline post-processor ---
	band := Borderline(decision, ok, f.cfg.BorderlineFloor, f.cfg.AgentConfidenceFloor)
	turnBand = band
	turnTopScore = decision.TopScore
	assistantmetrics.RouterBandTotal.WithLabelValues(string(band), transportLabel).Inc()

	// --- Step 6: band-driven dispatch ---
	var invocation *agent.InvocationResult

	switch band {
	case BandLow:
		resp = contracts.AssistantResponse{
			Routing:      &decision,
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         "saved as an idea — i'll surface it later.",
			EmittedAt:    emittedAt,
		}
		assistantmetrics.CaptureFallbackTotal.WithLabelValues(assistantmetrics.CauseLowConfidence, transportLabel).Inc()

	case BandBorderline:
		prompt := f.buildDisambiguationPrompt(decision, emittedAt)
		resp = contracts.AssistantResponse{
			Routing:              &decision,
			Status:               contracts.StatusThinking,
			DisambiguationPrompt: prompt,
			Body:                 "did you mean one of these?",
			EmittedAt:            emittedAt,
		}

	case BandHigh:
		scenarioID := decision.Chosen
		if scenarioID == "" {
			// Defensive: router classed it as a match but did not set
			// Chosen. Treat as capture rather than executing nothing.
			resp = contracts.AssistantResponse{
				Routing:      &decision,
				Status:       contracts.StatusSavedAsIdea,
				CaptureRoute: true,
				Body:         "saved as an idea — i'll surface it later.",
				EmittedAt:    emittedAt,
			}
			assistantmetrics.CaptureFallbackTotal.WithLabelValues(assistantmetrics.CauseLowConfidence, transportLabel).Inc()
			break
		}
		turnScenarioID = scenarioID
		// Manifest enable-gate
		if !f.manifest.Enabled(scenarioID) {
			// Spec 061 spec.md row "Skill disabled" (line 670) — the
			// contract is `errorCause=missing_scope` PLUS
			// `captureRoute=true` follow-up. Without CaptureRoute=true
			// the user's input is silently dropped (the adapter sees
			// CaptureRoute=false, skips the bot-side capture hook,
			// then attempts to render a "capability not enabled"
			// reply). BS-001's webhook regression hits this exact
			// branch when the test stack has no enabled scenarios
			// (e2e_ollama_smoke is enable-gated off in test env).
			resp = contracts.AssistantResponse{
				Routing:      &decision,
				Status:       contracts.StatusUnavailable,
				ErrorCause:   contracts.ErrMissingScope,
				CaptureRoute: true,
				Body:         "that capability is not enabled.",
				EmittedAt:    emittedAt,
			}
			break
		}
		// Resolve scenario from registry. Router may not always
		// hand back the *Scenario pointer (e.g. nil chosen on
		// explicit-id paths with stub routers), so resolve from
		// the registry as the authoritative source.
		sc := chosen
		if sc == nil {
			lookup, found := f.registry.Scenario(scenarioID)
			if !found {
				resp = contracts.AssistantResponse{
					Routing:    &decision,
					Status:     contracts.StatusUnavailable,
					ErrorCause: contracts.ErrInternalError,
					Body:       "internal error: scenario not found.",
					EmittedAt:  emittedAt,
				}
				break
			}
			sc = lookup
		}

		env.Routing = decision
		result := f.executor.Run(ctx, sc, env)
		invocation = result
		assistantmetrics.SkillInvocationsTotal.WithLabelValues(
			scenarioID,
			normalizeSkillOutcome(result),
			transportLabel,
		).Inc()

		body := translateFinalToBody(result)
		resp = contracts.AssistantResponse{
			Invocation: result,
			Routing:    &decision,
			Status:     translateOutcomeToStatus(result.Outcome, scenarioID),
			ErrorCause: translateOutcomeToErrorCause(result.Outcome),
			Body:       truncateBody(body, f.cfg.BodyMaxChars),
			EmittedAt:  emittedAt,
		}

		// Spec 061 SCOPE-04 facade-source-assembly-hook. When a
		// per-scenario SourceAssembler is registered, invoke it
		// BEFORE the provenance gate so resp.Sources is populated
		// from the executor's Final (cited_artifact_ids → []Source
		// via the skill-owned source-assembly invariant). A nil/
		// missing assembler leaves resp.Sources empty — the
		// provenance gate will then correctly refuse the response
		// for requires_provenance scenarios (the BS-007 graph-drift
		// refusal path is the same code path: assembler runs,
		// returns nil Sources because all citations were dropped,
		// gate refuses).
		var provenanceCause contracts.ProvenanceCause
		if assembler, registered := f.sourceAssemblers[scenarioID]; registered && assembler != nil && result != nil {
			assembly := assembler(ctx, result)
			if assembly.Body != "" {
				resp.Body = truncateBody(assembly.Body, f.cfg.BodyMaxChars)
			}
			resp.Sources = assembly.Sources
			resp.SourcesOverflowCount = assembly.OverflowCount
			// Forward the assembler's attribution hint to the
			// provenance gate. Empty Cause means the assembler did
			// not classify (or there were no citations to drop),
			// and the gate falls back to fabricated_source.
			provenanceCause = assembly.Cause
		}

		// Provenance gate (requires_provenance scenarios only).
		resp = provenance.Enforce(f.manifest.RequiresProvenance(scenarioID), scenarioID, provenanceCause, resp)

	default:
		// Unreachable — Band vocabulary is closed.
		resp = contracts.AssistantResponse{
			Routing:    &decision,
			Status:     contracts.StatusUnavailable,
			ErrorCause: contracts.ErrInternalError,
			Body:       fmt.Sprintf("internal error: unknown band %q.", band),
			EmittedAt:  emittedAt,
		}
	}

	// --- Step 7: persist conversation ---
	conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)

	// --- Step 8: audit ---
	f.writeAudit(ctx, msg, band, &decision, invocation, resp)

	// Spec 061 SCOPE-09 — surface the assistant turn id for the
	// deferred structured-log closure; outcome derivation is
	// performed inside the closure from the named (resp, err)
	// return values so early-return paths are covered too.
	turnAssistantTurnID = facadeTurnIDFromTime(emittedAt)

	return resp, nil
}

// buildDisambiguationPrompt selects up to cfg.DisambigMaxChoices
// candidates from decision.Considered (filtered to manifest-enabled
// scenarios), preserves order, and appends the save_as_note sentinel
// LAST per design §3.2. Labels come from the manifest.
func (f *Facade) buildDisambiguationPrompt(decision agent.RoutingDecision, emittedAt time.Time) *contracts.DisambiguationPrompt {
	choices := make([]contracts.DisambiguationChoice, 0, f.cfg.DisambigMaxChoices+1)
	number := 1
	for _, c := range decision.Considered {
		if len(choices) >= f.cfg.DisambigMaxChoices {
			break
		}
		if !f.manifest.Enabled(c.ScenarioID) {
			continue
		}
		label, _ := f.manifest.UserFacingLabel(c.ScenarioID)
		if label == "" {
			label = c.ScenarioID
		}
		choices = append(choices, contracts.DisambiguationChoice{
			Number: number,
			ID:     c.ScenarioID,
			Label:  label,
		})
		number++
	}
	// save_as_note sentinel is always last.
	choices = append(choices, contracts.DisambiguationChoice{
		Number: number,
		ID:     contracts.SaveAsNoteChoiceID,
		Label:  "save as a note",
	})
	ref := fmt.Sprintf("disambig-%d", emittedAt.UnixNano())
	return &contracts.DisambiguationPrompt{
		Choices:           choices,
		Timeout:           f.cfg.DisambigTimeout,
		DisambiguationRef: ref,
	}
}

// appendTurnAndPersist appends a new ContextTurn (bounded by
// cfg.WindowTurns) and writes the conversation back to the store.
// Persist errors are swallowed (logged via audit indirectly) so the
// user response is never delayed by storage hiccups — the next turn
// will simply not see this turn in its WorkingContext.
func (f *Facade) appendTurnAndPersist(
	ctx context.Context,
	conv assistantctx.Conversation,
	msg contracts.AssistantMessage,
	resp contracts.AssistantResponse,
	emittedAt time.Time,
) assistantctx.Conversation {
	sourceIDs := make([]string, 0, len(resp.Sources))
	for _, s := range resp.Sources {
		sourceIDs = append(sourceIDs, s.ID)
	}
	turn := assistantctx.ContextTurn{
		UserText:  msg.Text,
		Body:      resp.Body,
		SourceIDs: sourceIDs,
		EmittedAt: emittedAt,
	}
	conv.WorkingContext.Turns = append(conv.WorkingContext.Turns, turn)
	if len(conv.WorkingContext.Turns) > f.cfg.WindowTurns {
		drop := len(conv.WorkingContext.Turns) - f.cfg.WindowTurns
		conv.WorkingContext.Turns = conv.WorkingContext.Turns[drop:]
	}
	conv.UserID = msg.UserID
	conv.Transport = msg.Transport
	conv.LastActivityAt = emittedAt
	if conv.SchemaVersion == 0 {
		conv.SchemaVersion = 1
	}
	_ = f.contextStore.Persist(ctx, conv)
	return conv
}

// writeAudit emits one AuditTurn for the turn just handled. Errors
// from the writer are intentionally swallowed — the audit boundary is
// fire-and-forget at the facade level.
func (f *Facade) writeAudit(
	ctx context.Context,
	msg contracts.AssistantMessage,
	band Band,
	decision *agent.RoutingDecision,
	invocation *agent.InvocationResult,
	resp contracts.AssistantResponse,
) {
	turn := AuditTurn{
		UserID:             msg.UserID,
		Transport:          msg.Transport,
		TransportMessageID: msg.TransportMessageID,
		InboundKind:        msg.Kind,
		InboundText:        msg.Text,
		Band:               band,
		RoutingDecision:    decision,
		InvocationResult:   invocation,
		Response:           resp,
		EmittedAt:          resp.EmittedAt,
	}
	_ = f.audit.Write(ctx, turn)
}

// translateFinalToBody renders the InvocationResult.Final JSON to a
// human-readable body string. The capability layer does NOT inspect
// the scenario's output_schema here — that lives in the per-skill
// adapter (out of scope for SCOPE-04). For SCOPE-04 the body is the
// raw JSON text when Final has content, or a default acknowledgement
// when Outcome=OK with empty Final.
func translateFinalToBody(result *agent.InvocationResult) string {
	if result == nil {
		return ""
	}
	switch result.Outcome {
	case agent.OutcomeOK:
		if len(result.Final) == 0 {
			return "done."
		}
		// If Final is a JSON string literal, unquote it; otherwise
		// pass the raw JSON through.
		var s string
		if err := json.Unmarshal(result.Final, &s); err == nil {
			return s
		}
		return string(result.Final)
	case agent.OutcomeTimeout:
		return "request timed out."
	case agent.OutcomeProviderError:
		return "provider unavailable."
	case agent.OutcomeSchemaFailure, agent.OutcomeToolReturnInvalid, agent.OutcomeInputSchemaViolation:
		return "internal validation failure."
	case agent.OutcomeLoopLimit:
		return "request exceeded internal limits."
	case agent.OutcomeUnknownIntent:
		return ""
	default:
		// Non-terminal outcomes (per-tool error / hallucinated)
		// SHOULD not surface as terminal Outcome; if they do, treat
		// as internal error so the user sees something coherent.
		return "internal error."
	}
}

// translateOutcomeToStatus maps an agent.Outcome to the closed-vocab
// StatusToken the user sees.
func translateOutcomeToStatus(outcome agent.Outcome, scenarioID string) contracts.StatusToken {
	switch outcome {
	case agent.OutcomeOK:
		// Per-scenario nuance is handled by skill adapters; default
		// to Thinking-class success token. Specific tokens
		// (StatusCheckingWeather, StatusReminderConfirmed, ...)
		// are set by the skill adapters in SCOPE-06/07. SCOPE-04
		// owns the facade default ONLY.
		_ = scenarioID
		return contracts.StatusThinking
	case agent.OutcomeUnknownIntent:
		return contracts.StatusUnavailable
	case agent.OutcomeTimeout, agent.OutcomeProviderError:
		return contracts.StatusUnavailable
	default:
		return contracts.StatusUnavailable
	}
}

// translateOutcomeToErrorCause maps an agent.Outcome to the closed-vocab
// ErrorCause the user sees alongside the StatusToken. Spec 061 BS-006
// requires `errorCause=provider_unavailable` when an external provider
// fails (5xx / timeout / DNS); without explicit propagation here the
// downstream provenance gate would still rewrite Status+Body but the
// transport adapter would lose the cause needed to render the
// `weather: unavailable`-style error line.
//
// OutcomeOK and all other outcomes leave ErrorCause unset (ErrNone);
// the BandHigh dispatch path uses ErrInternalError + ErrMissingScope +
// ErrSlotMissing explicitly for its own short-circuits and does not
// depend on this helper.
func translateOutcomeToErrorCause(outcome agent.Outcome) contracts.ErrorCause {
	switch outcome {
	case agent.OutcomeProviderError, agent.OutcomeTimeout:
		return contracts.ErrProviderUnavailable
	default:
		return contracts.ErrNone
	}
}

// truncateBody bounds the body to maxChars (rune-aware). Returns the
// original string when shorter than the cap.
func truncateBody(body string, maxChars int) string {
	if maxChars <= 0 {
		return body
	}
	runes := []rune(body)
	if len(runes) <= maxChars {
		return body
	}
	return string(runes[:maxChars])
}

// --- Spec 061 SCOPE-09 telemetry helpers ---

// normalizeTransportLabel maps msg.Transport to one of the closed
// vocabulary values declared in internal/assistant/metrics. Unknown
// transports collapse to TransportFake so cardinality stays bounded
// and a new transport being wired is loud but not crashing.
func normalizeTransportLabel(t string) string {
	switch t {
	case assistantmetrics.TransportTelegram:
		return assistantmetrics.TransportTelegram
	case assistantmetrics.TransportFake:
		return assistantmetrics.TransportFake
	default:
		// Defensive: collapse unknown transport to the fake bucket.
		// The labels_test.go vocabulary check refuses any new value
		// added to AllTransports without a corresponding constant, so
		// this branch fires only for genuinely-unrouted callers.
		return assistantmetrics.TransportFake
	}
}

// deriveFacadeOutcome translates the response shape returned by
// Handle into the closed Outcome* vocabulary used by FacadeTurnsTotal
// + FacadeLatencySeconds. Pure mapping; no side effects.
//
// Ordering matters: DisambiguationPrompt first (proposed dominates
// status), then capture-route (captured dominates the BandLow
// StatusSavedAsIdea), then non-capture StatusSavedAsIdea (the
// /reset short-circuit), then explicit error status, then answered.
func deriveFacadeOutcome(resp contracts.AssistantResponse) string {
	if resp.DisambiguationPrompt != nil {
		return assistantmetrics.OutcomeProposed
	}
	if resp.CaptureRoute {
		return assistantmetrics.OutcomeCaptured
	}
	if resp.ErrorCause != "" || resp.Status == contracts.StatusUnavailable {
		return assistantmetrics.OutcomeError
	}
	if resp.Status == contracts.StatusSavedAsIdea {
		// reached only by the /reset short-circuit (CaptureRoute=false)
		return assistantmetrics.OutcomeDiscarded
	}
	return assistantmetrics.OutcomeAnswered
}

// normalizeSkillOutcome maps an *agent.InvocationResult.Outcome into
// the closed SkillOutcome* vocabulary on
// SkillInvocationsTotal{outcome=...}. A nil result is treated as
// SkillOutcomeUnknownIntent (defensive — should not happen for
// BandHigh dispatch).
func normalizeSkillOutcome(r *agent.InvocationResult) string {
	if r == nil {
		return assistantmetrics.SkillOutcomeUnknownIntent
	}
	switch r.Outcome {
	case agent.OutcomeOK:
		return assistantmetrics.SkillOutcomeOK
	case agent.OutcomeTimeout:
		return assistantmetrics.SkillOutcomeTimeout
	case agent.OutcomeProviderError:
		return assistantmetrics.SkillOutcomeProviderError
	case agent.OutcomeSchemaFailure:
		return assistantmetrics.SkillOutcomeSchemaFailure
	case agent.OutcomeToolReturnInvalid:
		return assistantmetrics.SkillOutcomeToolReturnInvalid
	case agent.OutcomeInputSchemaViolation:
		return assistantmetrics.SkillOutcomeInputSchemaViolation
	case agent.OutcomeLoopLimit:
		return assistantmetrics.SkillOutcomeLoopLimit
	case agent.OutcomeUnknownIntent:
		return assistantmetrics.SkillOutcomeUnknownIntent
	default:
		return assistantmetrics.SkillOutcomeUnknownIntent
	}
}

// facadeTurnIDFromTime builds a deterministic assistant turn id from
// the turn's emittedAt timestamp. Format: "asst-<unix-nano>". The
// adapter audit row uses the same value so a single turn can be
// traced from /metrics → log line → conversation row.
func facadeTurnIDFromTime(t time.Time) string {
	return fmt.Sprintf("asst-%d", t.UnixNano())
}

// agentTraceID returns the spec 037 agent trace id associated with
// an assistant turn. v1 derives it from the assistant turn id so
// dashboards can join the two. Once spec 037 publishes a real
// trace-id propagator the substrate executor will replace this.
func agentTraceID(assistantTurnID string) string {
	if assistantTurnID == "" {
		return ""
	}
	return "trace-" + assistantTurnID
}
