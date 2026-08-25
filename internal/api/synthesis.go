package api

import (
	"errors"
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
	ReadModel   *intelligence.SynthesisReadModel
	Persistence *intelligence.SynthesisPersistence
}

func NewSynthesisHandlers(model *intelligence.SynthesisReadModel, persistence *intelligence.SynthesisPersistence) *SynthesisHandlers {
	return &SynthesisHandlers{ReadModel: model, Persistence: persistence}
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
	State  string                   `json:"state"`
	Output *synthesisOutputResponse `json:"output,omitempty"`
}

// GetLatest reports the newest verified output, or an explicit never-run state.
func (h *SynthesisHandlers) GetLatest(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.ReadModel == nil {
		writeError(w, http.StatusServiceUnavailable, "synthesis_unavailable", "Synthesis read model is not configured")
		return
	}

	latest, found, err := h.ReadModel.Latest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "synthesis_read_failed", "Could not read synthesis state")
		return
	}
	if !found {
		// 200 with an explicit never-run state, not 404. A 404 would say "this
		// endpoint has nothing to say", when the honest answer is that synthesis
		// has a state and that state is never-run.
		writeJSON(w, http.StatusOK, synthesisLatestResponse{State: "never-run"})
		return
	}

	writeJSON(w, http.StatusOK, synthesisLatestResponse{
		State:  string(latest.Kind),
		Output: newSynthesisOutputResponse(latest),
	})
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
