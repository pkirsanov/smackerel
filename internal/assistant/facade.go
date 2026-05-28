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
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
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
	cfg          FacadeConfig
	router       agent.Router
	executor     ScenarioExecutor
	registry     ScenarioRegistry
	manifest     *SkillsManifest
	contextStore assistantctx.Store
	audit        AuditWriter
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
		cfg:          cfg,
		router:       router,
		executor:     executor,
		registry:     registry,
		manifest:     manifest,
		contextStore: contextStore,
		audit:        audit,
	}, nil
}

// Handle implements contracts.Assistant.
//
// The flow is intentionally linear (no transport-keyed branching).
// Every short-circuit path still ends with a persist + audit so the
// conversation row stays coherent.
func (f *Facade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	if msg.UserID == "" || msg.Transport == "" {
		return contracts.AssistantResponse{}, errors.New("assistant: AssistantMessage requires UserID and Transport")
	}

	conv, _, err := f.contextStore.Load(ctx, msg.UserID, msg.Transport)
	if err != nil {
		return contracts.AssistantResponse{}, fmt.Errorf("assistant: load context: %w", err)
	}

	emittedAt := f.cfg.Now()

	// --- Step 1: /reset short-circuit ---
	if msg.Kind == contracts.KindReset {
		if err := f.contextStore.DeleteByKey(ctx, msg.UserID, msg.Transport); err != nil {
			return contracts.AssistantResponse{}, fmt.Errorf("assistant: reset: %w", err)
		}
		resp := contracts.AssistantResponse{
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
				if err := f.contextStore.DeleteByKey(ctx, msg.UserID, msg.Transport); err != nil {
					return contracts.AssistantResponse{}, fmt.Errorf("assistant: reset via shortcut: %w", err)
				}
				resp := contracts.AssistantResponse{
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
			resp := contracts.AssistantResponse{
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

	// --- Step 6: band-driven dispatch ---
	var resp contracts.AssistantResponse
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
			break
		}
		// Manifest enable-gate
		if !f.manifest.Enabled(scenarioID) {
			resp = contracts.AssistantResponse{
				Routing:    &decision,
				Status:     contracts.StatusUnavailable,
				ErrorCause: contracts.ErrMissingScope,
				Body:       "that capability is not enabled.",
				EmittedAt:  emittedAt,
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

		body := translateFinalToBody(result)
		resp = contracts.AssistantResponse{
			Invocation: result,
			Routing:    &decision,
			Status:     translateOutcomeToStatus(result.Outcome, scenarioID),
			Body:       truncateBody(body, f.cfg.BodyMaxChars),
			EmittedAt:  emittedAt,
		}
		// Provenance gate (requires_provenance scenarios only).
		resp = provenance.Enforce(f.manifest.RequiresProvenance(scenarioID), scenarioID, resp)

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
