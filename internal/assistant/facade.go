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
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
	"github.com/smackerel/smackerel/internal/assistant/provenance"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"go.opentelemetry.io/otel/attribute"
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
	intentCompiler   intent.Compiler
	manifest         *SkillsManifest
	contextStore     assistantctx.Store
	audit            AuditWriter
	sourceAssemblers map[string]contracts.SourceAssembler

	// tracer is the spec 061 SCOPE-09b OTel seam. NewFacade installs
	// a no-op tracer by default so emission sites stay unconditional
	// in tests; production wiring (cmd/core/wiring_assistant_facade.go)
	// calls WithTracer to swap in the real one threaded from
	// coreServices.assistantTracer.
	tracer *tracing.Tracer
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
	// Spec 061 SCOPE-09b — install a no-op tracer by default. Real
	// production wiring calls WithTracer to swap in the SDK-backed
	// tracer threaded from coreServices.assistantTracer.
	noopTracer, _, err := tracing.NewTracer(context.Background(), tracing.Config{
		Enabled:     false,
		ServiceName: "smackerel-core",
	})
	if err != nil {
		return nil, fmt.Errorf("assistant: build noop tracer fallback: %w", err)
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
		tracer:           noopTracer,
	}, nil
}

// WithIntentCompiler attaches the spec 068 structured-intent compiler
// to the facade. nil-safe: a nil compiler leaves the facade in its
// pre-spec-068 routing mode (raw text drives the router directly).
// When set, every text turn that is NOT a slash shortcut and NOT an
// operational-command bypass is compiled before Router.Route is
// invoked; the compiled intent is marshalled into
// IntentEnvelope.StructuredContext.compiled_intent so downstream
// scenarios consume structured context rather than raw text alone
// (spec 068 SCN-068-A01/A02).
func (f *Facade) WithIntentCompiler(c intent.Compiler) *Facade {
	if c != nil {
		f.intentCompiler = c
	}
	return f
}

// WithTracer attaches the spec 061 SCOPE-09b OTel tracer to the
// facade. Safe to call exactly once after NewFacade and BEFORE Handle
// is invoked concurrently. nil-safe: a nil tracer leaves the no-op
// default in place so emission sites never panic.
func (f *Facade) WithTracer(tr *tracing.Tracer) *Facade {
	if tr != nil {
		f.tracer = tr
	}
	return f
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

	// Spec 061 SCOPE-09b — `assistant.facade.handle` span (design
	// §8.3.1.A item 2). Child of whatever span the caller already
	// has in ctx (typically `assistant.adapter.translate` started by
	// the transport adapter); becomes a root span when Handle is
	// called directly in tests. The ctx is rebound so every
	// downstream emission (context.load, router.classify, etc.)
	// attaches as a child automatically per design §8.3.1.C.
	hashedUserID := tracing.HashUserID(msg.UserID)
	ctx, facadeSpan := f.tracer.StartSpan(ctx, "assistant.facade.handle",
		transportLabel, hashedUserID, "", "", correlationID)
	defer func() {
		// scenario_id is empty at span start (pre-routing) and gets
		// re-stamped here if routing selected one. design §8.3.1.B
		// allows late attribute stamping for scenario_id.
		if turnScenarioID != "" {
			facadeSpan.SetAttributes(attribute.String("scenario_id", turnScenarioID))
		}
		if turnAssistantTurnID != "" {
			facadeSpan.SetAttributes(attribute.String("assistant_turn_id", turnAssistantTurnID))
		}
		spanStatus := "ok"
		spanErrCause := ""
		switch {
		case err != nil:
			spanStatus = "error"
			spanErrCause = "handle_failed"
		case resp.ErrorCause != "":
			spanStatus = "error"
			spanErrCause = string(resp.ErrorCause)
		case resp.CaptureRoute:
			spanStatus = "noop"
			spanErrCause = "capture_route"
		}
		tracing.EndSpan(facadeSpan, spanStatus, spanErrCause)
	}()

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

	conv, _, loadErr := f.loadContextWithSpan(ctx, msg.UserID, msg.Transport, transportLabel, hashedUserID, correlationID)
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

	// --- Step 1.5: pending disambiguation resolver ---
	//
	// Spec 061 SCOPE-09 — when the prior turn left a PendingDisambig
	// on the conversation, the next inbound turn is interpreted in
	// that context. Three terminal outcomes are emitted on
	// DisambiguationOutcomesTotal:
	//
	//   resolved_timeout_capture    — emittedAt > PendingDisambig.ExpiresAt
	//   resolved_user               — typed disambig reply (Kind==
	//                                 KindDisambiguation) OR text reply
	//                                 whose trimmed body parses to a
	//                                 valid 1-indexed choice number
	//   resolved_non_matching_reply — PendingDisambig present but the
	//                                 reply did not resolve to a choice
	//
	// In all three cases PendingDisambig is cleared. Capture-fallback
	// counters get a paired increment for the two capture paths
	// (borderline_timeout for TTL expiry, low_confidence for the
	// non-matching reply) so the existing dashboards continue to
	// reflect "capture happened" counts. The user_resolved path
	// returns a short confirmation and asks the user to re-send the
	// original request (the original RawInput is not stored).
	if resp, handled := f.resolvePendingDisambig(ctx, msg, conv, transportLabel, emittedAt); handled {
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

	// --- Step 3.5: spec 068 SCOPE-2 — structured intent compilation ---
	//
	// When an intent compiler is wired, every free-text turn that is
	// NOT a slash shortcut and NOT an operational-command bypass
	// (carve-out per spec 068 SCN-068-A07) is compiled BEFORE the
	// router runs. The compiled intent travels into
	// IntentEnvelope.StructuredContext.compiled_intent so the router
	// and downstream scenarios consume structured context rather than
	// raw text alone (SCN-068-A01, SCN-068-A02). When the compiler
	// returns a strong scenario hint we set IntentEnvelope.ScenarioID
	// so the router takes the explicit-id fast path. On compiler
	// failure we emit the canonical refusal-with-capture WITHOUT
	// invoking the router (Hard Constraint 1: raw text alone never
	// drives behavior).
	var compiledIntentRaw []byte
	var compiledScenarioHint string
	var compiled intent.CompiledIntent
	var compiledOK bool
	if f.intentCompiler != nil && msg.Kind == contracts.KindText && shortcutScenarioID == "" {
		if _, isOp := intent.IsOperationalCommand(msg.Text); !isOp {
			rawTurn := intent.RawTurn{
				UserID:             msg.UserID,
				Transport:          msg.Transport,
				TransportMessageID: msg.TransportMessageID,
				Text:               msg.Text,
				ReceivedAt:         emittedAt,
			}
			ci, _, cerr := f.intentCompiler.Compile(ctx, rawTurn)
			if cerr != nil {
				resp = contracts.AssistantResponse{
					Status:       contracts.StatusUnavailable,
					ErrorCause:   contracts.ErrInternalError,
					CaptureRoute: true,
					Body:         "could not interpret your request; saved as a note for review.",
					EmittedAt:    emittedAt,
				}
				conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
				f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
				return resp, nil
			}
			compiled = ci
			compiledOK = true
			if b, mErr := json.Marshal(compiled); mErr == nil {
				compiledIntentRaw = b
			}
			if compiled.ScenarioHint != nil && *compiled.ScenarioHint != "" {
				switch compiled.ActionClass {
				case intent.ActionAnswer, intent.ActionRetrieve,
					intent.ActionExternalLookup, intent.ActionInternalAction,
					intent.ActionStateMutation:
					compiledScenarioHint = *compiled.ScenarioHint
				}
			}
		}
	}

	// --- Step 3.55: spec 068 SCOPE-4 — clarification gate
	// (SCN-068-A05).
	//
	// When the compiler classifies a turn as clarify (or returns
	// missing_slots), the facade emits a clarification response and
	// MUST NOT call the router. Hard Constraint 1: raw text alone
	// (with ambiguous interpretation) never drives a scenario like
	// weather_lookup to pick one city out of several Springfields.
	// The clarification body comes from compiled.ClarificationPrompt
	// when non-nil/non-empty; otherwise a deterministic fallback that
	// names the missing slots.
	if compiledOK && conv.PendingConfirm == nil && requiresClarification(compiled) {
		body := buildClarificationBody(compiled)
		resp = contracts.AssistantResponse{
			Status:     contracts.StatusUnavailable,
			ErrorCause: contracts.ErrSlotMissing,
			Body:       body,
			EmittedAt:  emittedAt,
		}
		conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return resp, nil
	}

	// --- Step 3.6: spec 068 SCOPE-3 — side-effect confirmation gate
	// (SCN-068-A03, SCN-068-A04, SCN-068-A09).
	//
	// When the compiler classifies a turn as write or external_write,
	// the executor MUST NOT run until the user has confirmed. We
	// short-circuit BEFORE the router so no scenario can mutate
	// persistent or external state on the first turn. Existing
	// confirm flows (conv.PendingConfirm != nil) are not re-gated:
	// the spec 061 SCOPE-08 machine already owns the second-turn
	// confirm-reply path.
	if compiledOK && conv.PendingConfirm == nil && intent.RequiresConfirmation(compiled) {
		intent.SideEffectBlockedTotal.WithLabelValues(string(compiled.SideEffectClass), "missing_confirmation").Inc()
		resp = contracts.AssistantResponse{
			Status:       contracts.StatusUnavailable,
			ErrorCause:   contracts.ErrMissingScope,
			CaptureRoute: true,
			Body:         "this would write data; please confirm before I proceed.",
			EmittedAt:    emittedAt,
		}
		conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return resp, nil
	}

	// --- Step 4: build envelope + route ---
	scenarioOverride := shortcutScenarioID
	if scenarioOverride == "" {
		scenarioOverride = compiledScenarioHint
	}
	env := agent.IntentEnvelope{
		Source:     msg.Transport,
		RawInput:   msg.Text,
		ScenarioID: scenarioOverride,
	}
	if compiledIntentRaw != nil {
		body := StripShortcutPrefix(msg.Text)
		payload := map[string]any{
			"query":           body,
			"raw_query":       body,
			"user_id":         msg.UserID,
			"compiled_intent": json.RawMessage(compiledIntentRaw),
		}
		if b, err := json.Marshal(payload); err == nil {
			env.StructuredContext = b
		}
	}
	chosen, decision, ok := f.routeWithSpan(ctx, env, transportLabel, hashedUserID, correlationID)

	// --- Step 5: borderline post-processor ---
	band := f.borderlineWithSpan(ctx, decision, ok, transportLabel, hashedUserID, correlationID)
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
		// Spec 061 SCOPE-09 — persist the disambig so the next user
		// turn can be resolved against it. The Step-0.5 resolver at
		// the top of Handle reads conv.PendingDisambig on the
		// following turn and emits the matching outcome to
		// DisambiguationOutcomesTotal. ExpiresAt is computed from
		// the design-fixed timeout (assistant.disambiguate_timeout).
		choiceIDs := make([]assistantctx.DisambigChoiceID, 0, len(prompt.Choices))
		for _, c := range prompt.Choices {
			choiceIDs = append(choiceIDs, assistantctx.DisambigChoiceID{
				Number: c.Number,
				ID:     c.ID,
			})
		}
		conv.PendingDisambig = &assistantctx.PendingDisambig{
			DisambiguationRef: prompt.DisambiguationRef,
			Choices:           choiceIDs,
			ExpiresAt:         emittedAt.Add(f.cfg.DisambigTimeout),
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

		// Spec 061 Round-55 Defect-3 fix: every executor scenario's
		// input_schema declares type=object with one or more required
		// fields; internal/agent/executor.go Step (1) validates nil
		// StructuredContext as "got null, want object" and fires
		// OutcomeInputSchemaViolation BEFORE any LLM call, tool
		// invocation, or assembler/gate logic runs. The capability-
		// layer dispatch MUST populate a structured_context whose
		// fields satisfy the union of all v1 scenarios' required-field
		// sets. All three v1 schemas (retrieval-qa-v1, weather-query-v1,
		// notification-schedule-v1) omit additionalProperties:false, so
		// a single {query, raw_query, user_id} payload is generically
		// compatible without per-scenario branching. The body is the
		// post-shortcut natural-language tail so the LLM receives clean
		// text. Only populated when nil so explicit StructuredContext
		// callers (e.g. structured forms, tests, future programmatic
		// adapters) are not overridden.
		if env.StructuredContext == nil {
			body := StripShortcutPrefix(msg.Text)
			payload := map[string]string{
				"query":     body,
				"raw_query": body,
				"user_id":   msg.UserID,
			}
			if b, err := json.Marshal(payload); err == nil {
				env.StructuredContext = b
			}
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
		var assemblerOverride *contracts.ResponseOverride
		if assembler, registered := f.sourceAssemblers[scenarioID]; registered && assembler != nil && result != nil {
			assembly := assembler(ctx, result)
			if assembly.Override != nil {
				// BUG-061-003 D5 — deterministic-state override
				// (e.g. recipe_search empty-graph → StatusUnavailable
				// with actionable body, CaptureRoute=false). Skip the
				// provenance gate entirely; the assembler asserts this
				// is a known non-error state and therefore not a gate
				// failure.
				assemblerOverride = assembly.Override
				resp.Status = assembly.Override.Status
				resp.ErrorCause = assembly.Override.ErrorCause
				resp.CaptureRoute = assembly.Override.CaptureRoute
				resp.Body = truncateBody(assembly.Override.Body, f.cfg.BodyMaxChars)
				resp.Sources = nil
				resp.SourcesOverflowCount = 0
			} else {
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
		}

		// Provenance gate (requires_provenance scenarios only).
		// BUG-061-003 — skip the gate when the assembler emitted a
		// deterministic Override (the override path is for known
		// non-error states; there is nothing to refuse).
		if assemblerOverride == nil {
			resp = f.enforceProvenanceWithSpan(ctx,
				f.manifest.RequiresProvenance(scenarioID),
				scenarioID, provenanceCause, resp,
				transportLabel, hashedUserID, correlationID)
		}

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
	// Spec 061 SCOPE-09b — `assistant.context.persist` span (design
	// §8.3.1.A item 8). transport/hashed-user/correlation come from
	// the msg+resp envelope; scenario_id is stamped from the
	// response when routing selected one.
	scenarioForSpan := ""
	if resp.Routing != nil {
		scenarioForSpan = resp.Routing.Chosen
	}
	ctxPersist, persistSpan := f.tracer.StartSpan(ctx, "assistant.context.persist",
		normalizeTransportLabel(msg.Transport),
		tracing.HashUserID(msg.UserID),
		facadeTurnIDFromTime(emittedAt),
		scenarioForSpan,
		facadeCorrelationFromMsg(msg, emittedAt))
	persistErr := f.contextStore.Persist(ctxPersist, conv)
	persistStatus := "ok"
	persistCause := ""
	if persistErr != nil {
		persistStatus = "error"
		persistCause = "persist_failed"
	}
	tracing.EndSpan(persistSpan, persistStatus, persistCause)
	_ = persistErr // existing contract: persist errors are swallowed
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
	// Spec 061 SCOPE-09b — `assistant.audit.write` span (design
	// §8.3.1.A item 9). Errors from the writer are intentionally
	// swallowed (existing contract); the span captures the failure
	// for observability so dashboards can alert on a flatlined
	// audit-write rate.
	scenarioForSpan := ""
	if decision != nil {
		scenarioForSpan = decision.Chosen
	}
	ctxAudit, auditSpan := f.tracer.StartSpan(ctx, "assistant.audit.write",
		normalizeTransportLabel(msg.Transport),
		tracing.HashUserID(msg.UserID),
		facadeTurnIDFromTime(resp.EmittedAt),
		scenarioForSpan,
		facadeCorrelationFromMsg(msg, resp.EmittedAt))
	writeErr := f.audit.Write(ctxAudit, turn)
	writeStatus := "ok"
	writeCause := ""
	if writeErr != nil {
		writeStatus = "error"
		writeCause = "audit_write_failed"
	}
	tracing.EndSpan(auditSpan, writeStatus, writeCause)
	_ = writeErr // existing contract: audit errors are swallowed
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

// facadeCorrelationFromMsg derives the correlation_id stamped on the
// late persist/audit spans from the transport metadata. Falls back to
// the deterministic assistant turn id when the transport supplied no
// correlation token (matches the existing slog fallback in Handle's
// deferred log closure so all three signals — span, log, audit row —
// share the same identifier).
func facadeCorrelationFromMsg(msg contracts.AssistantMessage, emittedAt time.Time) string {
	if msg.TransportMetadata != nil {
		if v, ok := msg.TransportMetadata["telegram_update_id"]; ok && v != "" {
			return v
		}
	}
	return facadeTurnIDFromTime(emittedAt)
}

// loadContextWithSpan wraps contextStore.Load in the
// `assistant.context.load` span (design §8.3.1.A item 3). status/
// error_cause are derived from the loader's error return.
func (f *Facade) loadContextWithSpan(
	ctx context.Context,
	userID, transport, transportLabel, hashedUserID, correlationID string,
) (assistantctx.Conversation, bool, error) {
	ctxSpan, span := f.tracer.StartSpan(ctx, "assistant.context.load",
		transportLabel, hashedUserID, "", "", correlationID)
	conv, existed, err := f.contextStore.Load(ctxSpan, userID, transport)
	if err != nil {
		tracing.EndSpan(span, "error", "load_failed")
		return conv, existed, err
	}
	tracing.EndSpan(span, "ok", "")
	return conv, existed, nil
}

// routeWithSpan wraps router.Route in the `assistant.router.classify`
// span (design §8.3.1.A item 4). When routing selects a scenario the
// chosen id is re-stamped on the span so dashboards can group by it.
func (f *Facade) routeWithSpan(
	ctx context.Context,
	env agent.IntentEnvelope,
	transportLabel, hashedUserID, correlationID string,
) (*agent.Scenario, agent.RoutingDecision, bool) {
	ctxSpan, span := f.tracer.StartSpan(ctx, "assistant.router.classify",
		transportLabel, hashedUserID, "", "", correlationID)
	chosen, decision, ok := f.router.Route(ctxSpan, env)
	if decision.Chosen != "" {
		span.SetAttributes(attribute.String("scenario_id", decision.Chosen))
	}
	if ok {
		tracing.EndSpan(span, "ok", "")
	} else {
		// Router could not classify confidently — that is a noop
		// from the routing perspective (no error), and the band
		// post-processor downstream decides borderline vs low.
		tracing.EndSpan(span, "noop", "no_high_confidence_match")
	}
	return chosen, decision, ok
}

// borderlineWithSpan wraps the pure Borderline() post-processor in
// the `assistant.router.band` span (design §8.3.1.A item 5). The span
// is intentionally thin — Borderline does no I/O — but its presence
// keeps the canonical tree shape verifiable per §8.3.1.
func (f *Facade) borderlineWithSpan(
	ctx context.Context,
	decision agent.RoutingDecision, ok bool,
	transportLabel, hashedUserID, correlationID string,
) Band {
	_, span := f.tracer.StartSpan(ctx, "assistant.router.band",
		transportLabel, hashedUserID, decision.Chosen, "", correlationID)
	band := Borderline(decision, ok, f.cfg.BorderlineFloor, f.cfg.AgentConfidenceFloor)
	// Stamp the resolved band as a closed-vocab attribute so
	// dashboards can filter by it.
	span.SetAttributes(attribute.String("band", string(band)))
	tracing.EndSpan(span, "ok", "")
	return band
}

// enforceProvenanceWithSpan wraps provenance.Enforce in the
// `assistant.provenance.check` span (design §8.3.1.A item 6). When
// the gate refuses, the refusal cause is captured as error_cause so
// dashboards can attribute provenance refusals correctly.
func (f *Facade) enforceProvenanceWithSpan(
	ctx context.Context,
	requiresProvenance bool, scenarioID string,
	cause contracts.ProvenanceCause, resp contracts.AssistantResponse,
	transportLabel, hashedUserID, correlationID string,
) contracts.AssistantResponse {
	_, span := f.tracer.StartSpan(ctx, "assistant.provenance.check",
		transportLabel, hashedUserID, scenarioID, "", correlationID)
	out := provenance.Enforce(requiresProvenance, scenarioID, cause, resp)
	spanStatus := "ok"
	spanCause := ""
	if out.ErrorCause != resp.ErrorCause && out.ErrorCause != "" {
		// Gate flipped the response into a refusal.
		spanStatus = "error"
		spanCause = string(out.ErrorCause)
	}
	tracing.EndSpan(span, spanStatus, spanCause)
	return out
}

// resolvePendingDisambig implements the Step-1.5 disambiguation
// resolver. Returns (resp, true) when a pending disambig was present
// and this turn closed it (capture, user-selection, or non-matching).
// Returns (_, false) when no disambig is pending — Handle continues
// with the normal Step-2 shortcut/Step-3 reference/Step-4 route flow.
//
// On every handled path PendingDisambig is cleared on the
// conversation row before persist. The DisambiguationOutcomesTotal
// counter is incremented exactly once per terminal outcome;
// CaptureFallbackTotal gets a paired increment on the two capture
// paths.
func (f *Facade) resolvePendingDisambig(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	transportLabel string,
	emittedAt time.Time,
) (contracts.AssistantResponse, bool) {
	if conv.PendingDisambig == nil {
		return contracts.AssistantResponse{}, false
	}
	pd := conv.PendingDisambig

	// (a) TTL expired — capture and emit resolved_timeout_capture.
	if emittedAt.After(pd.ExpiresAt) {
		assistantmetrics.DisambiguationOutcomesTotal.WithLabelValues(
			assistantmetrics.DisambigOutcomeResolvedTimeoutCapture,
			transportLabel,
		).Inc()
		assistantmetrics.CaptureFallbackTotal.WithLabelValues(
			assistantmetrics.CauseBorderlineTimeout,
			transportLabel,
		).Inc()
		conv.PendingDisambig = nil
		resp := contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         "saved as an idea — earlier choice expired.",
			EmittedAt:    emittedAt,
		}
		conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return resp, true
	}

	// (b) Attempt to resolve a choice. Two acceptable inbound shapes:
	//   1. Kind == KindDisambiguation with matching DisambiguationRef
	//      and a 1-indexed DisambiguationChoice (typed disambig reply).
	//   2. Kind == KindText whose trimmed body parses to a valid
	//      1-indexed choice number (fallback for adapters that don't
	//      track per-prompt state).
	chosenID := ""
	matched := false

	if msg.Kind == contracts.KindDisambiguation && msg.DisambiguationRef == pd.DisambiguationRef {
		for _, c := range pd.Choices {
			if c.Number == msg.DisambiguationChoice {
				chosenID = c.ID
				matched = true
				break
			}
		}
	} else if msg.Kind == contracts.KindText {
		if n, err := strconv.Atoi(strings.TrimSpace(msg.Text)); err == nil {
			for _, c := range pd.Choices {
				if c.Number == n {
					chosenID = c.ID
					matched = true
					break
				}
			}
		}
	}

	if matched {
		assistantmetrics.DisambiguationOutcomesTotal.WithLabelValues(
			assistantmetrics.DisambigOutcomeResolvedUser,
			transportLabel,
		).Inc()
		conv.PendingDisambig = nil
		var resp contracts.AssistantResponse
		if chosenID == contracts.SaveAsNoteChoiceID {
			// save_as_note is an explicit user choice to capture —
			// emit the user-resolved outcome but skip
			// CaptureFallbackTotal (capture WAS the user's request,
			// not a fallback).
			resp = contracts.AssistantResponse{
				Status:       contracts.StatusSavedAsIdea,
				CaptureRoute: true,
				Body:         "saved as a note.",
				EmittedAt:    emittedAt,
			}
		} else {
			label, _ := f.manifest.UserFacingLabel(chosenID)
			if label == "" {
				label = chosenID
			}
			resp = contracts.AssistantResponse{
				Status:    contracts.StatusSavedAsIdea,
				Body:      fmt.Sprintf("ok, treating as %s — please re-send your request.", label),
				EmittedAt: emittedAt,
			}
		}
		conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandHigh, nil, nil, resp)
		return resp, true
	}

	// (c) Disambig was pending but reply did not resolve — emit
	// resolved_non_matching_reply outcome, paired CaptureFallback,
	// and capture.
	assistantmetrics.DisambiguationOutcomesTotal.WithLabelValues(
		assistantmetrics.DisambigOutcomeResolvedNonMatchingReply,
		transportLabel,
	).Inc()
	assistantmetrics.CaptureFallbackTotal.WithLabelValues(
		assistantmetrics.CauseLowConfidence,
		transportLabel,
	).Inc()
	conv.PendingDisambig = nil
	resp := contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
		EmittedAt:    emittedAt,
	}
	conv = f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
	f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
	return resp, true
}
