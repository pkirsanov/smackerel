package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/agent/tools/microtools"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/assistant/confirm"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
)

const compiledActionProposalSchemaV1 = "v1"

// CompiledActionProposal is the server-owned payload persisted behind a
// ConfirmRef. Callback requests carry only the ref and choice.
type CompiledActionProposal struct {
	SchemaVersion      string                `json:"schema_version"`
	ConfirmRef         string                `json:"confirm_ref"`
	UserID             string                `json:"user_id"`
	Transport          string                `json:"transport"`
	TransportMessageID string                `json:"transport_message_id"`
	Intent             intent.CompiledIntent `json:"intent"`
}

// PreparedCompiledAction contains a validated proposal and user-facing label.
type PreparedCompiledAction struct {
	Proposal       CompiledActionProposal
	ProposedAction string
}

// CompiledActionResult is the bounded response produced by a confirmed action.
type CompiledActionResult struct {
	Status contracts.StatusToken
	Body   string
}

// CompiledActionExecutor bridges confirmed compiler output to existing domain
// stores/tools. Prepare must not perform the gated write.
type CompiledActionExecutor interface {
	Prepare(ctx context.Context, proposal CompiledActionProposal) (PreparedCompiledAction, error)
	Execute(ctx context.Context, proposal CompiledActionProposal) (CompiledActionResult, error)
}

// IntentLocationResolver is the established read-only location capability.
type IntentLocationResolver func(context.Context, string) (microtools.Envelope, error)

// IntentWeatherResolver executes the existing read-only weather capability.
type IntentWeatherResolver func(context.Context, string, weather.ForecastWindow) (weather.Forecast, error)

type compiledInteractions struct {
	confirmMachine   *confirm.Machine
	actionExecutor   CompiledActionExecutor
	locationResolver IntentLocationResolver
	weatherResolver  IntentWeatherResolver
	confirmTimeout   time.Duration
}

// ConfigureCompiledInteractions attaches the durable interaction machines.
func (f *Facade) ConfigureCompiledInteractions(
	machine *confirm.Machine,
	executor CompiledActionExecutor,
	resolver IntentLocationResolver,
	weatherResolver IntentWeatherResolver,
	confirmTimeout time.Duration,
) error {
	if machine == nil {
		return errors.New("assistant: compiled interactions require confirm machine")
	}
	if executor == nil {
		return errors.New("assistant: compiled interactions require action executor")
	}
	if resolver == nil {
		return errors.New("assistant: compiled interactions require location resolver")
	}
	if weatherResolver == nil {
		return errors.New("assistant: compiled interactions require weather resolver")
	}
	if confirmTimeout <= 0 {
		return errors.New("assistant: compiled interactions require confirm timeout > 0")
	}
	f.compiledInteractions = &compiledInteractions{
		confirmMachine: machine, actionExecutor: executor,
		locationResolver: resolver, weatherResolver: weatherResolver,
		confirmTimeout: confirmTimeout,
	}
	return nil
}

func (f *Facade) proposeCompiledAction(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	compiled intent.CompiledIntent,
	emittedAt time.Time,
) (contracts.AssistantResponse, error) {
	proposal := CompiledActionProposal{
		SchemaVersion: compiledActionProposalSchemaV1,
		UserID:        msg.UserID, Transport: msg.Transport,
		TransportMessageID: msg.TransportMessageID, Intent: compiled,
	}
	prepared, err := f.compiledInteractions.actionExecutor.Prepare(ctx, proposal)
	if err != nil {
		return contracts.AssistantResponse{}, fmt.Errorf("prepare compiled action: %w", err)
	}
	payload, err := json.Marshal(prepared.Proposal)
	if err != nil {
		return contracts.AssistantResponse{}, fmt.Errorf("encode compiled action proposal: %w", err)
	}
	scenarioID := compiledScenarioID(compiled)
	if err := f.compiledInteractions.confirmMachine.Propose(ctx, confirm.ProposalInput{
		UserID: msg.UserID, Transport: msg.Transport, ScenarioID: scenarioID,
		ConfirmRef:     prepared.Proposal.ConfirmRef,
		ProposedAction: prepared.ProposedAction,
		Payload:        payload,
		ExpiresAt:      emittedAt.Add(f.compiledInteractions.confirmTimeout),
	}, emittedAt); err != nil {
		return contracts.AssistantResponse{}, err
	}
	updated, _, err := f.contextStore.Load(ctx, msg.UserID, msg.Transport)
	if err != nil {
		return contracts.AssistantResponse{}, fmt.Errorf("reload compiled confirm proposal: %w", err)
	}
	resp := contracts.AssistantResponse{
		Status: contracts.StatusThinking,
		Body:   "Please confirm this change.",
		ConfirmCard: &contracts.ConfirmCard{
			ProposedAction: prepared.ProposedAction,
			Timeout:        f.compiledInteractions.confirmTimeout,
			ConfirmRef:     prepared.Proposal.ConfirmRef,
			PositiveLabel:  "confirm",
			NegativeLabel:  "cancel",
		},
		EmittedAt: emittedAt,
	}
	f.appendTurnAndPersist(ctx, updated, msg, resp, emittedAt)
	f.writeAudit(ctx, msg, BandHigh, nil, nil, resp)
	return resp, nil
}

func compiledScenarioID(compiled intent.CompiledIntent) string {
	if compiled.ScenarioHint != nil && strings.TrimSpace(*compiled.ScenarioHint) != "" {
		return strings.TrimSpace(*compiled.ScenarioHint)
	}
	return string(compiled.ActionClass)
}

func (f *Facade) handlePendingConfirm(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	emittedAt time.Time,
) (contracts.AssistantResponse, bool, error) {
	if msg.Kind != contracts.KindConfirm || f.compiledInteractions == nil {
		return contracts.AssistantResponse{}, false, nil
	}
	if msg.ConfirmChoice == contracts.ConfirmNegative {
		err := f.compiledInteractions.confirmMachine.Discard(ctx, confirm.DiscardInput{
			UserID: msg.UserID, Transport: msg.Transport, ConfirmRef: msg.ConfirmRef,
		}, emittedAt)
		return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, CompiledActionResult{
			Status: contracts.StatusSavedAsIdea, Body: "Change cancelled.",
		})
	}
	if msg.ConfirmChoice != contracts.ConfirmPositive {
		return contracts.AssistantResponse{}, true, errors.New("assistant: confirm choice must be positive or negative")
	}
	confirmed, err := f.compiledInteractions.confirmMachine.Confirm(ctx, confirm.ConfirmInput{
		UserID: msg.UserID, Transport: msg.Transport, ConfirmRef: msg.ConfirmRef,
	}, emittedAt)
	if err != nil {
		return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, CompiledActionResult{})
	}
	var proposal CompiledActionProposal
	if err := json.Unmarshal(confirmed.Payload, &proposal); err != nil {
		return contracts.AssistantResponse{}, true, fmt.Errorf("decode confirmed compiled action: %w", err)
	}
	if proposal.SchemaVersion != compiledActionProposalSchemaV1 ||
		proposal.UserID != msg.UserID || proposal.Transport != msg.Transport ||
		proposal.ConfirmRef != msg.ConfirmRef {
		return contracts.AssistantResponse{}, true, errors.New("assistant: confirmed compiled action ownership mismatch")
	}
	result, err := f.compiledInteractions.actionExecutor.Execute(ctx, proposal)
	return f.finishConfirmResponse(ctx, msg, conv, emittedAt, err, result)
}

func (f *Facade) finishConfirmResponse(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	emittedAt time.Time,
	actionErr error,
	result CompiledActionResult,
) (contracts.AssistantResponse, bool, error) {
	if errors.Is(actionErr, confirm.ErrPendingNotFound) {
		resp := contracts.AssistantResponse{
			Status: contracts.StatusUnavailable, ErrorCause: contracts.ErrNoMatch,
			Body: "That confirmation is stale or already resolved.", EmittedAt: emittedAt,
		}
		f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return resp, true, nil
	}
	if actionErr != nil {
		return contracts.AssistantResponse{}, true, actionErr
	}
	updated, _, err := f.contextStore.Load(ctx, msg.UserID, msg.Transport)
	if err != nil {
		return contracts.AssistantResponse{}, true, err
	}
	resp := contracts.AssistantResponse{Status: result.Status, Body: result.Body, EmittedAt: emittedAt}
	f.appendTurnAndPersist(ctx, updated, msg, resp, emittedAt)
	f.writeAudit(ctx, msg, BandHigh, nil, nil, resp)
	return resp, true, nil
}

func (f *Facade) proposeCompilerDisambiguation(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	compiled intent.CompiledIntent,
	emittedAt time.Time,
) (contracts.AssistantResponse, bool, error) {
	location, ok := compiledLocationRaw(compiled)
	if !ok {
		return contracts.AssistantResponse{}, false, nil
	}
	envelope, err := f.compiledInteractions.locationResolver(ctx, location)
	if err != nil {
		return contracts.AssistantResponse{}, false, err
	}
	if envelope.Status != microtools.StatusAmbiguous {
		return contracts.AssistantResponse{}, false, nil
	}
	if len(envelope.Candidates) < 2 {
		return contracts.AssistantResponse{}, false, errors.New("assistant: ambiguous location resolver returned fewer than two choices")
	}
	limit := len(envelope.Candidates)
	if limit > f.cfg.DisambigMaxChoices {
		limit = f.cfg.DisambigMaxChoices
	}
	ref := fmt.Sprintf("disambig-location-%d", emittedAt.UnixNano())
	choices := make([]contracts.DisambiguationChoice, 0, limit)
	persisted := make([]assistantctx.DisambigChoiceID, 0, limit)
	for index, candidate := range envelope.Candidates[:limit] {
		value, err := json.Marshal(candidate.Value)
		if err != nil {
			return contracts.AssistantResponse{}, false, err
		}
		id := fmt.Sprintf("location:%d", candidate.Rank)
		choices = append(choices, contracts.DisambiguationChoice{
			Number: index + 1, ID: id, Label: candidate.Label,
		})
		persisted = append(persisted, assistantctx.DisambigChoiceID{
			Number: index + 1, ID: id, Label: candidate.Label, Value: value,
		})
	}
	prompt := &contracts.DisambiguationPrompt{
		Choices: choices, Timeout: f.cfg.DisambigTimeout, DisambiguationRef: ref,
	}
	compiledRaw, err := json.Marshal(compiled)
	if err != nil {
		return contracts.AssistantResponse{}, false, err
	}
	conv.PendingDisambig = &assistantctx.PendingDisambig{
		DisambiguationRef: ref, Choices: persisted,
		ExpiresAt: emittedAt.Add(f.cfg.DisambigTimeout),
		Resume:    &assistantctx.DisambigResume{CompiledIntent: compiledRaw},
	}
	resp := contracts.AssistantResponse{
		Status: contracts.StatusThinking, Body: buildClarificationBody(compiled),
		DisambiguationPrompt: prompt, EmittedAt: emittedAt,
	}
	f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
	f.writeAudit(ctx, msg, BandBorderline, nil, nil, resp)
	return resp, true, nil
}

func compiledLocationRaw(compiled intent.CompiledIntent) (string, bool) {
	value, ok := compiled.Slots["location"]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	case map[string]any:
		raw, _ := typed["raw"].(string)
		trimmed := strings.TrimSpace(raw)
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}

func (f *Facade) resolveCompilerDisambig(
	ctx context.Context,
	msg contracts.AssistantMessage,
	conv assistantctx.Conversation,
	transportLabel string,
	emittedAt time.Time,
) (contracts.AssistantMessage, assistantctx.Conversation, contracts.AssistantResponse, bool, error) {
	pending := conv.PendingDisambig
	if pending == nil || pending.Resume == nil || msg.Kind != contracts.KindDisambiguation {
		return msg, conv, contracts.AssistantResponse{}, false, nil
	}
	if msg.DisambiguationRef != pending.DisambiguationRef {
		resp := contracts.AssistantResponse{
			Status: contracts.StatusUnavailable, ErrorCause: contracts.ErrNoMatch,
			Body: "That choice reference is stale.", EmittedAt: emittedAt,
		}
		f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return msg, conv, resp, true, nil
	}
	var selected *assistantctx.DisambigChoiceID
	for index := range pending.Choices {
		if pending.Choices[index].Number == msg.DisambiguationChoice {
			selected = &pending.Choices[index]
			break
		}
	}
	if selected == nil || len(selected.Value) == 0 {
		resp := contracts.AssistantResponse{
			Status: contracts.StatusUnavailable, ErrorCause: contracts.ErrNoMatch,
			Body: "That choice is not available.", EmittedAt: emittedAt,
		}
		f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
		f.writeAudit(ctx, msg, BandLow, nil, nil, resp)
		return msg, conv, resp, true, nil
	}
	assistantmetrics.DisambiguationOutcomesTotal.WithLabelValues(
		assistantmetrics.DisambigOutcomeResolvedUser, transportLabel,
	).Inc()
	conv.PendingDisambig = nil
	conv.LastActivityAt = emittedAt
	if conv.SchemaVersion == 0 {
		conv.SchemaVersion = 1
	}
	if err := f.contextStore.Persist(ctx, conv); err != nil {
		return msg, conv, contracts.AssistantResponse{}, true, err
	}
	var compiled intent.CompiledIntent
	if err := json.Unmarshal(pending.Resume.CompiledIntent, &compiled); err != nil {
		return msg, conv, contracts.AssistantResponse{}, true, fmt.Errorf("decode pending compiled intent: %w", err)
	}
	window := compiledWeatherWindow(compiled)
	forecast, err := f.compiledInteractions.weatherResolver(ctx, selected.Label, window)
	if err != nil {
		return msg, conv, contracts.AssistantResponse{}, true, err
	}
	resp := contracts.AssistantResponse{
		Status: contracts.StatusCheckingWeather,
		Body:   forecast.ForecastLine,
		Sources: []contracts.Source{{
			ID:    fmt.Sprintf("%s:%d", forecast.ProviderName, forecast.RetrievedAt.UnixNano()),
			Title: forecast.ProviderName,
			Kind:  contracts.SourceExternalProvider,
			Ref: contracts.ExternalProviderRef{
				ProviderName: forecast.ProviderName,
				RetrievedAt:  forecast.RetrievedAt,
			},
		}},
		EmittedAt: emittedAt,
	}
	f.appendTurnAndPersist(ctx, conv, msg, resp, emittedAt)
	f.writeAudit(ctx, msg, BandHigh, nil, nil, resp)
	return msg, conv, resp, true, nil
}

func compiledWeatherWindow(compiled intent.CompiledIntent) weather.ForecastWindow {
	window, _ := compiled.Slots["window"].(string)
	switch weather.ForecastWindow(strings.TrimSpace(window)) {
	case weather.WindowToday:
		return weather.WindowToday
	case weather.WindowTomorrow:
		return weather.WindowTomorrow
	case weather.WindowWeekend:
		return weather.WindowWeekend
	default:
		return weather.WindowNow
	}
}
