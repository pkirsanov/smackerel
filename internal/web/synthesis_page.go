package web

import (
	"context"
	"net/http"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-05 — the Today/Status synthesis read seam.
//
// Today and Status must answer from the SAME committed state. Giving both pages
// one reader is what makes that structural: there is no second query for them
// to disagree about, and no page-local shortcut that could report a different
// story than the API does.

// SynthesisOutcomeReader is the observation seam for durable synthesis state.
// Production injects *intelligence.SynthesisReadModel directly. One scoped
// snapshot prevents a page from pairing an attempt and output from different
// actors or cadences.
type SynthesisOutcomeReader interface {
	ReadSnapshot(ctx context.Context, query intelligence.SynthesisReadQuery) (intelligence.SynthesisReadSnapshot, error)
}

// SynthesisAggregateReader supplies the persisted insight text for a state that
// is allowed to show content. It is separate from SynthesisOutcomeReader so a
// page that only needs the state never acquires the ability to read prose.
type SynthesisAggregateReader interface {
	ReadAggregate(ctx context.Context, outputID string) (*intelligence.SynthesisAggregate, error)
}

// synthesisModel resolves the durable state into the one projection Today and
// Status render.
//
// The two early returns are the load-bearing part. An unauthorized reader is
// answered before any read happens, so no stored field is ever fetched on their
// behalf. A missing reader answers unavailable rather than never-run, because
// reporting "nothing has ever run" when the truth is "I could not look" is
// exactly the false emptiness this bug exists to remove.
func (h *Handler) synthesisModel(ctx context.Context, authorized bool) SynthesisPageModel {
	if !authorized {
		return SynthesisPageModel{State: SynthesisViewAuthRequired}
	}
	if h.SynthesisReader == nil {
		return SynthesisPageModel{State: SynthesisViewUnavailable}
	}

	now := h.now()
	freshnessBudget, err := h.SynthesisFreshnessPolicy.BudgetFor(h.SynthesisCadence)
	if err != nil {
		return SynthesisPageModel{State: SynthesisViewUnavailable}
	}
	snapshot, err := h.SynthesisReader.ReadSnapshot(ctx, intelligence.SynthesisReadQuery{
		Principal:       h.SynthesisPrincipal,
		Cadence:         h.SynthesisCadence,
		FreshnessBudget: freshnessBudget,
		ObservedAt:      now,
	})
	if err != nil {
		// A failed read is never presented as an absence of work.
		return SynthesisPageModel{State: SynthesisViewUnavailable}
	}
	var latest intelligence.SynthesisLatest
	found := snapshot.CurrentOutput != nil
	if found {
		latest = snapshot.CurrentOutput.Latest
	}

	model := ClassifySynthesisView(authorized, snapshot.Outcome, latest, found, now)

	// Text is loaded ONLY for a state that is allowed to show it. Asking
	// HasContent rather than listing states means a state added later without a
	// content decision shows nothing instead of leaking prose.
	if model.HasContent() && model.OutputID != "" && h.SynthesisAggregates != nil {
		agg, aerr := h.SynthesisAggregates.ReadAggregate(ctx, model.OutputID)
		if aerr != nil || agg == nil {
			// The state was verified but its text could not be read. Say so
			// rather than rendering a headline with nothing under it.
			return SynthesisPageModel{State: SynthesisViewUnavailable}
		}
		for _, in := range agg.Insights {
			model.Insights = append(model.Insights, SynthesisInsightView{
				InsightType:     string(in.InsightType),
				ThroughLine:     in.ThroughLine,
				KeyTension:      in.KeyTension,
				SuggestedAction: in.SuggestedAction,
				CitationCount:   len(in.SourceArtifactIDs),
			})
		}
	}

	return model
}

// requestAuthorized reports whether this request carries a real session.
//
// Deriving it rather than assuming it is the point. The web routes sit behind
// auth middleware today, so an expired session is normally turned away before a
// handler runs. Asking the context anyway means that if a synthesis section is
// ever mounted on a route without that middleware, the page renders
// auth_required instead of quietly serving one reader's synthesis to another.
func requestAuthorized(r *http.Request) bool {
	_, ok := auth.SessionFromContext(r.Context())
	return ok
}
