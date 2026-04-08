package keep

import (
	"strings"
	"time"
)

// Qualifier evaluates Keep note properties and assigns processing tiers.
type Qualifier struct {
	recentThresholdDays int
}

// NewQualifier creates a new source qualifier engine.
func NewQualifier() *Qualifier {
	return &Qualifier{
		recentThresholdDays: 30,
	}
}

// QualifierResult holds the tier assignment and the rule that triggered it.
type QualifierResult struct {
	Tier   Tier
	Reason string
}

// Evaluate assigns a processing tier to a note based on its properties.
// Evaluation order: trashed→skip, pinned→full, labeled→full, images→full,
// recent(<30d)→standard, archived→light, old(>30d)→light, default→standard.
func (q *Qualifier) Evaluate(note *TakeoutNote) QualifierResult {
	if note.IsTrashed {
		return QualifierResult{Tier: TierSkip, Reason: "trashed"}
	}
	if note.IsPinned {
		return QualifierResult{Tier: TierFull, Reason: "pinned"}
	}
	if len(note.Labels) > 0 {
		return QualifierResult{Tier: TierFull, Reason: "labeled"}
	}

	for _, a := range note.Attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			return QualifierResult{Tier: TierFull, Reason: "has_images"}
		}
	}

	parser := NewTakeoutParser()
	modifiedAt := parser.ModifiedAt(note)
	daysSinceModified := time.Since(modifiedAt).Hours() / 24

	if daysSinceModified <= float64(q.recentThresholdDays) {
		return QualifierResult{Tier: TierStandard, Reason: "recent"}
	}

	if note.IsArchived {
		return QualifierResult{Tier: TierLight, Reason: "archived"}
	}

	if daysSinceModified > float64(q.recentThresholdDays) {
		return QualifierResult{Tier: TierLight, Reason: "old"}
	}

	return QualifierResult{Tier: TierStandard, Reason: "default"}
}

// EvaluateBatch evaluates a batch of notes and returns tier count statistics.
func (q *Qualifier) EvaluateBatch(notes []TakeoutNote) map[Tier]int {
	counts := map[Tier]int{
		TierFull:     0,
		TierStandard: 0,
		TierLight:    0,
		TierSkip:     0,
	}

	for i := range notes {
		result := q.Evaluate(&notes[i])
		counts[result.Tier]++
	}

	return counts
}
