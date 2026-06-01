package citeback

import "errors"

// Verdict is the DoD-shaped wrapper around VerifyResult requested by
// spec 064 SCOPE-08. Callers that only need the boolean outcome plus
// the two coarse failure buckets can use Verdict instead of walking
// the typed Rejected slice on VerifyResult.
//
// Bucket assignment is derived from the typed rejection sentinels:
//
//   - FabricatedCites: citations the agent invented — the locator
//     does not appear in the recorded tool trace at all
//     (ReasonNotInTrace).
//   - MissingCites: citations that point at a real recorded source
//     but fail to faithfully represent it — wrong Kind for the same
//     locator (ReasonKindMismatch), wrong ContentHash for a recorded
//     web source (ReasonHashMismatch), or a citation missing a
//     required field for its Kind (ReasonMalformedCitation).
//
// OK is true iff both slices are empty.
type Verdict struct {
	OK              bool
	MissingCites    []Citation
	FabricatedCites []Citation
}

// VerifyVerdict returns the DoD-shaped Verdict for the given citations
// and tool trace. It is a pure adapter over Verify; both share the
// same underlying verification logic.
func VerifyVerdict(answerCitations []Citation, trace ToolTrace) Verdict {
	res := Verify(answerCitations, trace)
	v := Verdict{OK: res.OK}
	for _, r := range res.Rejected {
		switch {
		case errors.Is(r.Reason, ReasonNotInTrace):
			v.FabricatedCites = append(v.FabricatedCites, r.Citation)
		default:
			v.MissingCites = append(v.MissingCites, r.Citation)
		}
	}
	return v
}
