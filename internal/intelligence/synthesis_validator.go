package intelligence

import (
	"fmt"
	"strings"
)

// BUG-004-004 SCOPE-02 validation.
//
// SCN-004-004-04 requires that an invalid or uncited candidate is rejected
// BEFORE persistence is entered, so the rejection cannot leave a partial write
// behind. That ordering is the whole point: validating inside the transaction
// would still be correct, but it would make "no output stored" a consequence of
// rollback rather than a consequence of never having started.
//
// Failure codes are a CLOSED set and carry no synthesis text, source titles,
// artifact content or fingerprints. A code names the class of problem; the
// operator finds the instance in the run, not in the error string.

// SynthesisFailureCode is the closed set of safe terminal failure codes.
type SynthesisFailureCode string

const (
	FailureMissingCitation     SynthesisFailureCode = "missing_citation"
	FailureUnauthorizedSource  SynthesisFailureCode = "unauthorized_source"
	FailureInvalidPayload      SynthesisFailureCode = "invalid_payload"
	FailureRequiredSourceOmit  SynthesisFailureCode = "required_source_omitted"
	FailureConfidenceOutOfBand SynthesisFailureCode = "confidence_out_of_band"
)

// SourceClassPolicy declares which source classes a cadence requires and which
// it may proceed without. A class in neither list is unknown and rejected --
// silently tolerating an unlisted class is how an unauthorized source would
// enter the corpus.
type SourceClassPolicy struct {
	Required []string
	Optional []string
}

// SynthesisCandidate is a producer's output before it is allowed near the
// database.
type SynthesisCandidate struct {
	Key                    SynthesisRunKey
	Kind                   SynthesisOutputKind
	Insights               []SynthesisInsight
	EvaluatedArtifactCount int
	IncludedClasses        []string
	OmittedClasses         []string
}

// SynthesisValidationError carries a safe code and a content-free message.
type SynthesisValidationError struct {
	Code   SynthesisFailureCode
	Detail string
}

func (e *SynthesisValidationError) Error() string {
	return fmt.Sprintf("synthesis candidate rejected [%s]: %s", e.Code, e.Detail)
}

// ValidateCandidate rejects a candidate that must not reach persistence.
//
// authorizedSources is the set of artifact ids the run was permitted to read.
// Citations are checked against it rather than against "any id that looks
// plausible", because an insight citing an artifact outside the authorized set
// is the corpus-leak case, not a formatting problem.
func ValidateCandidate(c SynthesisCandidate, policy SourceClassPolicy, authorizedSources []string) error {
	authorized := make(map[string]struct{}, len(authorizedSources))
	for _, id := range authorizedSources {
		authorized[id] = struct{}{}
	}

	known := make(map[string]bool, len(policy.Required)+len(policy.Optional))
	for _, cl := range policy.Required {
		known[cl] = true
	}
	for _, cl := range policy.Optional {
		known[cl] = false
	}

	included := make(map[string]struct{}, len(c.IncludedClasses))
	for _, cl := range c.IncludedClasses {
		if _, ok := known[cl]; !ok {
			return &SynthesisValidationError{
				Code:   FailureUnauthorizedSource,
				Detail: fmt.Sprintf("source class %q is in neither the required nor the optional policy list", cl),
			}
		}
		included[cl] = struct{}{}
	}
	for _, cl := range c.OmittedClasses {
		required, ok := known[cl]
		if !ok {
			return &SynthesisValidationError{
				Code:   FailureUnauthorizedSource,
				Detail: fmt.Sprintf("omitted source class %q is not in the policy", cl),
			}
		}
		// A REQUIRED class may never be omitted, whatever the output kind.
		// Permitting it would let a partial output silently mean "we skipped
		// something load-bearing".
		if required {
			return &SynthesisValidationError{
				Code:   FailureRequiredSourceOmit,
				Detail: fmt.Sprintf("required source class %q was omitted", cl),
			}
		}
	}
	for _, cl := range policy.Required {
		if _, ok := included[cl]; !ok {
			return &SynthesisValidationError{
				Code:   FailureRequiredSourceOmit,
				Detail: fmt.Sprintf("required source class %q is absent from the included set", cl),
			}
		}
	}

	switch c.Kind {
	case OutputKindQuiet:
		if len(c.Insights) != 0 {
			return &SynthesisValidationError{
				Code:   FailureInvalidPayload,
				Detail: "a quiet output asserts the window produced nothing, so it must carry no insights",
			}
		}
		if len(c.OmittedClasses) != 0 {
			return &SynthesisValidationError{
				Code:   FailureInvalidPayload,
				Detail: "a quiet output must not also claim omissions; that is a partial output",
			}
		}
	case OutputKindPartial:
		if len(c.OmittedClasses) == 0 {
			return &SynthesisValidationError{
				Code:   FailureInvalidPayload,
				Detail: "a partial output must name at least one omitted class",
			}
		}
	case OutputKindFull:
		if len(c.OmittedClasses) != 0 {
			return &SynthesisValidationError{
				Code:   FailureInvalidPayload,
				Detail: "a complete output must omit nothing; name the omissions and use partial",
			}
		}
	default:
		return &SynthesisValidationError{
			Code:   FailureInvalidPayload,
			Detail: fmt.Sprintf("unknown output kind %q", c.Kind),
		}
	}

	if c.EvaluatedArtifactCount < 0 {
		return &SynthesisValidationError{
			Code:   FailureInvalidPayload,
			Detail: "evaluated artifact count cannot be negative",
		}
	}

	for i, in := range c.Insights {
		if strings.TrimSpace(in.ThroughLine) == "" {
			return &SynthesisValidationError{
				Code:   FailureInvalidPayload,
				Detail: fmt.Sprintf("insight %d has an empty through line", i),
			}
		}
		if in.Confidence < 0 || in.Confidence > 1 {
			return &SynthesisValidationError{
				Code:   FailureConfidenceOutOfBand,
				Detail: fmt.Sprintf("insight %d confidence is outside [0,1]", i),
			}
		}
		// Every NON-QUIET insight must be cited. An uncited insight is an
		// assertion the system cannot substantiate, which is the same class of
		// untruth as the health query this packet repairs.
		if len(in.SourceArtifactIDs) == 0 {
			return &SynthesisValidationError{
				Code:   FailureMissingCitation,
				Detail: fmt.Sprintf("insight %d carries no source citations", i),
			}
		}
		for _, src := range in.SourceArtifactIDs {
			if _, ok := authorized[src]; !ok {
				return &SynthesisValidationError{
					Code:   FailureUnauthorizedSource,
					Detail: fmt.Sprintf("insight %d cites an artifact outside the authorized source set", i),
				}
			}
		}
	}
	return nil
}
