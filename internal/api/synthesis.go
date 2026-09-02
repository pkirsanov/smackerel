package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-04 — the synthesis read API.
//
// Three routes over the ONE durable read model, so the API, the health probe
// and the alert evaluator cannot disagree about the state of synthesis. Before
// this the only synthesis surface was an aggregate health field, and there was
// no way for an operator to ask what the latest output actually was.

// SynthesisHandlers serves the read surface. A nil ReadModel is treated as
// unavailable rather than empty, because an unconfigured API answering "no
// synthesis" would be indistinguishable from a system that has never run.
type SynthesisHandlers struct {
	ReadModel       *intelligence.SynthesisReadModel
	Persistence     *intelligence.SynthesisPersistence
	Principal       string
	FreshnessBudget time.Duration
	// Producer backs the operator retry route. Nil leaves retry unavailable
	// rather than accepting a request it cannot act on -- a 202 for work that
	// will never happen is the failure mode this packet exists to remove.
	Producer *intelligence.SynthesisProducer
}

func NewSynthesisHandlers(
	model *intelligence.SynthesisReadModel,
	persistence *intelligence.SynthesisPersistence,
	principal string,
	freshnessBudget time.Duration,
) (*SynthesisHandlers, error) {
	if principal == "" {
		return nil, errors.New("synthesis handlers require a principal")
	}
	if freshnessBudget <= 0 {
		return nil, errors.New("synthesis handlers require a positive freshness budget")
	}
	return &SynthesisHandlers{
		ReadModel: model, Persistence: persistence,
		Principal: principal, FreshnessBudget: freshnessBudget,
	}, nil
}

// WithProducer enables the operator retry route.
func (h *SynthesisHandlers) WithProducer(p *intelligence.SynthesisProducer) *SynthesisHandlers {
	h.Producer = p
	return h
}

// synthesisOutputResponse is the content-free summary. It carries counts,
// identity and window but never a through-line, source title or artifact text,
// so it is safe to return, log, or attach to a span.
type synthesisOutputResponse struct {
	OutputID               string `json:"outputId"`
	Cadence                string `json:"cadence"`
	Kind                   string `json:"kind"`
	InsightCount           int    `json:"insightCount"`
	CitationCount          int    `json:"citationCount"`
	EvaluatedArtifactCount int    `json:"evaluatedArtifactCount,omitempty"`
	WindowStart            string `json:"windowStart"`
	WindowEnd              string `json:"windowEnd"`
	LifecycleState         string `json:"lifecycleState"`
	CreatedAt              string `json:"createdAt"`
}

// synthesisLatestResponse always carries an explicit state. A caller must never
// have to infer "never ran" from an absent field, because an absent field also
// describes a serialisation bug.
type synthesisLatestResponse struct {
	State         string                    `json:"state"`
	LatestAttempt *synthesisAttemptResponse `json:"latestAttempt,omitempty"`
	Output        *synthesisOutputResponse  `json:"output,omitempty"`
}

type synthesisAttemptResponse struct {
	RunID       string `json:"runId"`
	AttemptNo   int    `json:"attemptNo"`
	State       string `json:"state"`
	AttemptedAt string `json:"attemptedAt"`
}

// GetLatest reports the newest verified output, or an explicit never-run state.
func (h *SynthesisHandlers) GetLatest(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.ReadModel == nil || h.Principal == "" || h.FreshnessBudget <= 0 {
		writeError(w, http.StatusServiceUnavailable, "synthesis_unavailable", "Synthesis read model is not configured")
		return
	}

	cadence, err := requiredSynthesisCadence(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cadence", err.Error())
		return
	}
	snapshot, err := h.ReadModel.ReadSnapshot(r.Context(), intelligence.SynthesisReadQuery{
		Principal:       h.Principal,
		Cadence:         cadence,
		FreshnessBudget: h.FreshnessBudget,
		ObservedAt:      time.Now().UTC(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "synthesis_read_failed", "Could not read synthesis state")
		return
	}
	if snapshot.LatestAttempt == nil && snapshot.CurrentOutput == nil {
		// 200 with an explicit never-run state, not 404. A 404 would say "this
		// endpoint has nothing to say", when the honest answer is that synthesis
		// has a state and that state is never-run.
		writeJSON(w, http.StatusOK, synthesisLatestResponse{State: "never-run"})
		return
	}

	response := synthesisLatestResponse{State: synthesisLatestState(snapshot)}
	if snapshot.LatestAttempt != nil {
		response.LatestAttempt = &synthesisAttemptResponse{
			RunID: snapshot.LatestAttempt.RunID, AttemptNo: snapshot.LatestAttempt.AttemptNo,
			State:       string(snapshot.LatestAttempt.EventType),
			AttemptedAt: snapshot.LatestAttempt.StartedAt.UTC().Format(time.RFC3339),
		}
	}
	if snapshot.CurrentOutput != nil {
		response.Output = newSynthesisOutputResponse(snapshot.CurrentOutput.Latest)
	}
	writeJSON(w, http.StatusOK, response)
}

func requiredSynthesisCadence(r *http.Request) (intelligence.SynthesisCadence, error) {
	switch r.URL.Query().Get("cadence") {
	case string(intelligence.CadenceDaily):
		return intelligence.CadenceDaily, nil
	case string(intelligence.CadenceWeekly):
		return intelligence.CadenceWeekly, nil
	default:
		return "", errors.New("cadence must be daily or weekly")
	}
}

func synthesisLatestState(snapshot intelligence.SynthesisReadSnapshot) string {
	switch snapshot.Outcome.Phase {
	case intelligence.PhaseRunning:
		return "running"
	case intelligence.PhaseWriteFailed:
		if snapshot.Outcome.HasPriorVerifiedOutput {
			return "failed-with-prior-output"
		}
		return "failed-without-output"
	case intelligence.PhaseCommitted:
		if snapshot.Outcome.ReadBack != intelligence.ReadBackOK || snapshot.CurrentOutput == nil {
			return "read-degraded"
		}
		return string(snapshot.CurrentOutput.Latest.Kind)
	default:
		return "read-degraded"
	}
}

func newSynthesisOutputResponse(l intelligence.SynthesisLatest) *synthesisOutputResponse {
	return &synthesisOutputResponse{
		OutputID:               l.OutputID,
		Cadence:                l.Cadence,
		Kind:                   string(l.Kind),
		InsightCount:           l.InsightCount,
		CitationCount:          l.CitationCount,
		EvaluatedArtifactCount: l.EvaluatedArtifactCount,
		WindowStart:            l.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:              l.WindowEnd.UTC().Format(time.RFC3339),
		LifecycleState:         l.LifecycleState,
		CreatedAt:              l.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type synthesisHistoryResponse struct {
	Runs []synthesisOutputResponse `json:"runs"`
}

// ListRuns returns verified outputs newest-first.
func (h *SynthesisHandlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.ReadModel == nil {
		writeError(w, http.StatusServiceUnavailable, "synthesis_unavailable", "Synthesis read model is not configured")
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be an integer")
			return
		}
		limit = parsed
	}

	entries, err := h.ReadModel.History(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "synthesis_read_failed", "Could not read synthesis history")
		return
	}

	// A non-nil empty slice, so the field serialises as [] rather than null. A
	// null would make "no runs" look like a missing field to a client.
	runs := make([]synthesisOutputResponse, 0, len(entries))
	for _, e := range entries {
		runs = append(runs, synthesisOutputResponse{
			OutputID:       e.OutputID,
			Cadence:        e.Cadence,
			Kind:           string(e.Kind),
			InsightCount:   e.InsightCount,
			CitationCount:  e.CitationCount,
			WindowStart:    e.WindowStart.UTC().Format(time.RFC3339),
			WindowEnd:      e.WindowEnd.UTC().Format(time.RFC3339),
			LifecycleState: e.LifecycleState,
			CreatedAt:      e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, synthesisHistoryResponse{Runs: runs})
}

type synthesisInsightResponse struct {
	InsightType     string   `json:"insightType"`
	ThroughLine     string   `json:"throughLine"`
	KeyTension      string   `json:"keyTension,omitempty"`
	SuggestedAction string   `json:"suggestedAction,omitempty"`
	Confidence      float64  `json:"confidence"`
	Citations       []string `json:"citations"`
}

type synthesisDetailResponse struct {
	Output   synthesisOutputResponse    `json:"output"`
	Included []string                   `json:"includedSourceClasses"`
	Omitted  []string                   `json:"omittedSourceClasses"`
	Insights []synthesisInsightResponse `json:"insights"`
}

// GetRun returns one output with its insights and citations.
//
// This is the only route that carries synthesis TEXT, which is why it is a
// separate route rather than a flag on the listing: a caller that only needs
// counts never has to receive content it would then have to be trusted with.
func (h *SynthesisHandlers) GetRun(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Persistence == nil {
		writeError(w, http.StatusServiceUnavailable, "synthesis_unavailable", "Synthesis persistence is not configured")
		return
	}

	outputID := chi.URLParam(r, "outputID")
	if outputID == "" {
		writeError(w, http.StatusBadRequest, "invalid_output_id", "outputId is required")
		return
	}

	agg, err := h.Persistence.ReadAggregate(r.Context(), outputID)
	if err != nil {
		// Not-found and read-failure are deliberately the same shape here. An
		// output id is not guessable, and a distinct 404 would confirm to an
		// unauthorised prober which ids exist.
		if errors.Is(err, intelligence.ErrSynthesisOutputNotFound) {
			writeError(w, http.StatusNotFound, "synthesis_output_not_found", "No such synthesis output")
			return
		}
		writeError(w, http.StatusInternalServerError, "synthesis_read_failed", "Could not read synthesis output")
		return
	}

	insights := make([]synthesisInsightResponse, 0, len(agg.Insights))
	for _, in := range agg.Insights {
		citations := in.SourceArtifactIDs
		if citations == nil {
			citations = []string{}
		}
		insights = append(insights, synthesisInsightResponse{
			InsightType:     string(in.InsightType),
			ThroughLine:     in.ThroughLine,
			KeyTension:      in.KeyTension,
			SuggestedAction: in.SuggestedAction,
			Confidence:      in.Confidence,
			Citations:       citations,
		})
	}

	included := agg.IncludedClasses
	if included == nil {
		included = []string{}
	}
	omitted := agg.OmittedClasses
	if omitted == nil {
		omitted = []string{}
	}

	writeJSON(w, http.StatusOK, synthesisDetailResponse{
		Output: synthesisOutputResponse{
			OutputID:               agg.OutputID,
			Kind:                   string(agg.Kind),
			InsightCount:           agg.InsightCount,
			CitationCount:          agg.CitationCount,
			EvaluatedArtifactCount: agg.EvaluatedArtifactCount,
			CreatedAt:              agg.CreatedAt.UTC().Format(time.RFC3339),
		},
		Included: included,
		Omitted:  omitted,
		Insights: insights,
	})
}

type synthesisRetryRequest struct {
	Cadence string `json:"cadence"`
}

type synthesisRetryResponse struct {
	Outcome string                   `json:"outcome"`
	Output  *synthesisOutputResponse `json:"output,omitempty"`
}

// Retry runs a cadence window on demand.
//
// It reports what actually HAPPENED rather than that the request was accepted.
// A 202 "accepted" would be the same shape of claim the original defect made:
// true about the request, silent about whether anything was stored.
func (h *SynthesisHandlers) Retry(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Producer == nil {
		writeError(w, http.StatusServiceUnavailable, "synthesis_retry_unavailable", "Synthesis retry is not configured")
		return
	}

	var req synthesisRetryRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be JSON with a cadence field")
		return
	}

	var cadence intelligence.SynthesisCadence
	switch req.Cadence {
	case string(intelligence.CadenceDaily):
		cadence = intelligence.CadenceDaily
	case string(intelligence.CadenceWeekly):
		cadence = intelligence.CadenceWeekly
	default:
		// A closed vocabulary, refused rather than defaulted. Silently running
		// daily for an unrecognised cadence would produce a real output that
		// answers a question nobody asked.
		writeError(w, http.StatusBadRequest, "invalid_cadence", "cadence must be daily or weekly")
		return
	}

	agg, err := h.Producer.RunAndPersist(r.Context(), cadence, intelligence.TriggerOperatorRetry, time.Now().UTC())
	switch {
	case errors.Is(err, intelligence.ErrRunClaimedElsewhere):
		// Not an error condition. Another process holds the window and is doing
		// the work, which is the coordination succeeding.
		writeJSON(w, http.StatusConflict, synthesisRetryResponse{Outcome: "claimed-elsewhere"})
		return
	case err != nil:
		var ve *intelligence.SynthesisValidationError
		if errors.As(err, &ve) {
			// The failure CLASS is safe to return; the candidate content that
			// caused it is not, and the validator already keeps it out.
			writeError(w, http.StatusUnprocessableEntity, string(ve.Code), "Synthesis candidate was rejected")
			return
		}
		var auditErr *intelligence.SynthesisAuditPersistenceError
		if errors.As(err, &auditErr) {
			// The typed class is safe at the API boundary; its operation and
			// wrapped database cause are intentionally not serialized.
			writeError(w, http.StatusInternalServerError, string(intelligence.FailureAudit),
				"Required synthesis audit record could not be stored")
			return
		}
		writeError(w, http.StatusInternalServerError, "synthesis_retry_failed", "Synthesis run failed")
		return
	}

	writeJSON(w, http.StatusOK, synthesisRetryResponse{
		Outcome: "persisted",
		Output: &synthesisOutputResponse{
			OutputID:               agg.OutputID,
			Kind:                   string(agg.Kind),
			InsightCount:           agg.InsightCount,
			CitationCount:          agg.CitationCount,
			EvaluatedArtifactCount: agg.EvaluatedArtifactCount,
			CreatedAt:              agg.CreatedAt.UTC().Format(time.RFC3339),
		},
	})
}
