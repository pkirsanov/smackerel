// Spec 071 SCOPE-02 — Facade <-> IntentTrace wire-in helpers.
//
// This file owns the small adapter surface between assistant.Facade
// and internal/assistant/intenttrace. Keeping the helpers here keeps
// facade.go focused on routing/dispatch while the trace plumbing
// (sampling decision → redaction → recorder Record) lives next to
// the trace types it produces.

package assistant

import (
	"context"
	"log/slog"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

// IntentTraceWiring carries the spec 071 SCOPE-2 dependencies the
// facade needs to emit one trace per compiled turn. All four fields
// are REQUIRED; nil values disable trace emission outright (the
// facade falls back to its pre-spec-071 behaviour).
type IntentTraceWiring struct {
	Recorder intenttrace.IntentTraceRecorder
	Sampler  intenttrace.Sampler
	Redactor intenttrace.Redactor
	Policy   intenttrace.SourcePolicy
}

// WithIntentTrace attaches the spec 071 SCOPE-2 trace wiring. Safe to
// call exactly once after NewFacade and BEFORE Handle is invoked
// concurrently. A nil wiring (any required field unset) leaves trace
// emission disabled.
func (f *Facade) WithIntentTrace(w IntentTraceWiring) *Facade {
	if w.Recorder == nil || w.Sampler == nil || w.Redactor == nil {
		return f
	}
	f.intentTrace = w
	return f
}

// mapStatusToFinal projects the assistant contracts response status
// onto the closed intenttrace.FinalResponseStatus vocabulary. The
// mapping is intentionally explicit so a new contract status added
// upstream surfaces as a compile-time gap to extend here rather than
// silently degrading to "unavailable".
func mapStatusToFinal(resp contracts.AssistantResponse) intenttrace.FinalResponseStatus {
	if resp.CaptureRoute {
		return intenttrace.StatusCaptureFallback
	}
	switch resp.Status {
	case contracts.StatusCheckingWeather:
		return intenttrace.StatusCheckingWeather
	case contracts.StatusUnavailable:
		switch resp.ErrorCause {
		case contracts.ErrSlotMissing:
			return intenttrace.StatusClarify
		case contracts.ErrMissingScope:
			return intenttrace.StatusRefused
		}
		return intenttrace.StatusUnavailable
	case contracts.StatusThinking:
		// Thinking is the disambig pending state; treat as clarify.
		return intenttrace.StatusClarify
	case contracts.StatusSavedAsIdea:
		// /reset and confirmation acknowledgements; treat as ok.
		return intenttrace.StatusOK
	}
	return intenttrace.StatusOK
}

// emitIntentTrace builds a TurnTraceInput from the per-turn facade
// state and forwards it to the configured recorder. Record errors are
// logged at WARN and swallowed — the user response must never be
// delayed by trace emission per spec 071 Hard Constraint "redaction
// before export" + the existing facade non-blocking persistence
// pattern.
func (f *Facade) emitIntentTrace(
	ctx context.Context,
	traceID string,
	msg contracts.AssistantMessage,
	resp contracts.AssistantResponse,
	compiled intent.CompiledIntent,
	compiledOK bool,
	scenarioID string,
	transportLabel string,
	userIDHash string,
) {
	if f.intentTrace.Recorder == nil || !compiledOK || traceID == "" {
		return
	}
	sampled := f.intentTrace.Sampler.ShouldSample(traceID)
	in := intenttrace.TurnTraceInput{
		TraceID:            traceID,
		TurnID:             traceID,
		UserIDHash:         userIDHash,
		Transport:          intenttrace.Transport(transportLabel),
		TransportMessageID: msg.TransportMessageID,
		CompilerInvoked:    true,
		Sampled:            sampled,
		EmittedAt:          resp.EmittedAt,
	}
	if !sampled {
		in.SampledOutReason = string(intenttrace.SampledOutDeterministic)
		if _, err := f.intentTrace.Recorder.Record(ctx, in); err != nil {
			slog.Warn("intent_trace_record_failed", "trace_id", traceID, "error", err)
		}
		return
	}
	red := f.intentTrace.Redactor.Redact(f.intentTrace.Policy, msg.Text, compiled.Slots)
	confidence := compiled.Confidence
	in.ActionClass = string(compiled.ActionClass)
	in.SideEffectClass = string(compiled.SideEffectClass)
	in.Confidence = &confidence
	in.RouteDecision = scenarioID
	in.FinalResponseStatus = mapStatusToFinal(resp)
	in.RefusalCause = string(resp.ErrorCause)
	if resp.CaptureRoute {
		in.CaptureCause = string(resp.ErrorCause)
	}
	in.SlotsRedactionSummary = red.Summary
	if _, err := f.intentTrace.Recorder.Record(ctx, in); err != nil {
		slog.Warn("intent_trace_record_failed", "trace_id", traceID, "error", err)
	}
}

// isKnownTransportLabel reports whether the transport label belongs to
// the intenttrace v1 closed vocabulary. The facade uses this to skip
// trace emission on unknown transports rather than failing the turn.
func isKnownTransportLabel(label string) bool {
	for _, t := range intenttrace.AllTransports {
		if string(t) == label {
			return true
		}
	}
	return false
}
