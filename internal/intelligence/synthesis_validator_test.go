package intelligence

import (
	"errors"
	"testing"
	"time"
)

// BUG-004-004 SCOPE-02 — T004-04-VALIDATOR.
//
// Every case here asserts the FAILURE CODE, not merely that an error occurred.
// A validator that rejected everything with one generic error would pass a
// weaker test and tell an operator nothing about which class of problem fired.

func validatorPolicy() SourceClassPolicy {
	return SourceClassPolicy{Required: []string{"article"}, Optional: []string{"video"}}
}

func validatorAuthorized() []string { return []string{"art-a", "art-b"} }

func validatorKey() SynthesisRunKey {
	return SynthesisRunKey{
		Cadence:       CadenceDaily,
		Principal:     "operator",
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "v1",
		SourceIDs:     []string{"art-a", "art-b"},
	}
}

func validCandidate() SynthesisCandidate {
	return SynthesisCandidate{
		Key:  validatorKey(),
		Kind: OutputKindFull,
		Insights: []SynthesisInsight{{
			ID:                "i1",
			InsightType:       InsightThroughLine,
			ThroughLine:       "a real through line",
			SourceArtifactIDs: []string{"art-a"},
			Confidence:        0.6,
		}},
		EvaluatedArtifactCount: 2,
		IncludedClasses:        []string{"article", "video"},
	}
}

func codeOf(t *testing.T, err error) SynthesisFailureCode {
	t.Helper()
	var ve *SynthesisValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a SynthesisValidationError, got %T: %v", err, err)
	}
	return ve.Code
}

// POSITIVE CONTROL. Without it, a validator that rejected every input would
// satisfy all the negative cases below.
func TestValidator_AcceptsAValidCandidate(t *testing.T) {
	if err := ValidateCandidate(validCandidate(), validatorPolicy(), validatorAuthorized()); err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
}

func TestValidator_RejectsUncitedInsight(t *testing.T) {
	c := validCandidate()
	c.Insights[0].SourceArtifactIDs = nil
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureMissingCitation {
		t.Fatalf("got code %q, want %q", got, FailureMissingCitation)
	}
}

// The corpus-leak case: a citation outside the authorized set is not a
// formatting problem, it is an insight built from something the run was never
// permitted to read.
func TestValidator_RejectsUnauthorizedCitation(t *testing.T) {
	c := validCandidate()
	c.Insights[0].SourceArtifactIDs = []string{"art-not-authorized"}
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureUnauthorizedSource {
		t.Fatalf("got code %q, want %q", got, FailureUnauthorizedSource)
	}
}

func TestValidator_RejectsEmptyThroughLine(t *testing.T) {
	c := validCandidate()
	c.Insights[0].ThroughLine = "   "
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureInvalidPayload {
		t.Fatalf("got code %q, want %q", got, FailureInvalidPayload)
	}
}

func TestValidator_RejectsConfidenceOutOfBand(t *testing.T) {
	for _, conf := range []float64{-0.1, 1.1} {
		c := validCandidate()
		c.Insights[0].Confidence = conf
		if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureConfidenceOutOfBand {
			t.Fatalf("confidence %v: got code %q, want %q", conf, got, FailureConfidenceOutOfBand)
		}
	}
}

func TestValidator_RejectsMissingRequiredSourceClass(t *testing.T) {
	c := validCandidate()
	c.IncludedClasses = []string{"video"} // 'article' is required
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureRequiredSourceOmit {
		t.Fatalf("got code %q, want %q", got, FailureRequiredSourceOmit)
	}
}

// A required class may never be omitted, whatever the kind. Permitting it would
// let a partial output quietly mean "we skipped something load-bearing".
func TestValidator_RejectsOmittedRequiredClassEvenWhenPartial(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindPartial
	c.IncludedClasses = []string{"video"}
	c.OmittedClasses = []string{"article"}
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureRequiredSourceOmit {
		t.Fatalf("got code %q, want %q", got, FailureRequiredSourceOmit)
	}
}

func TestValidator_RejectsUnknownSourceClass(t *testing.T) {
	c := validCandidate()
	c.IncludedClasses = []string{"article", "video", "smuggled"}
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureUnauthorizedSource {
		t.Fatalf("got code %q, want %q", got, FailureUnauthorizedSource)
	}
}

// SCN-004-004-07. A quiet output ASSERTS the window produced nothing, so
// carrying insights would make it a lie about its own meaning.
func TestValidator_QuietOutputMustCarryNoInsights(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindQuiet
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureInvalidPayload {
		t.Fatalf("got code %q, want %q", got, FailureInvalidPayload)
	}
}

func TestValidator_AcceptsWellFormedQuietOutput(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindQuiet
	c.Insights = nil
	c.EvaluatedArtifactCount = 412 // evaluated a lot, produced nothing
	if err := ValidateCandidate(c, validatorPolicy(), validatorAuthorized()); err != nil {
		t.Fatalf("well-formed quiet output rejected: %v", err)
	}
}

// SCN-004-004-08. "Partial" with nothing named is a completeness claim wearing a
// hedge; the omissions must be enumerable.
func TestValidator_PartialOutputMustNameOmissions(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindPartial
	c.OmittedClasses = nil
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureInvalidPayload {
		t.Fatalf("got code %q, want %q", got, FailureInvalidPayload)
	}
}

func TestValidator_AcceptsWellFormedPartialOutput(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindPartial
	c.IncludedClasses = []string{"article"}
	c.OmittedClasses = []string{"video"} // optional class, permitted omission
	if err := ValidateCandidate(c, validatorPolicy(), validatorAuthorized()); err != nil {
		t.Fatalf("well-formed partial output rejected: %v", err)
	}
}

func TestValidator_CompleteOutputMustOmitNothing(t *testing.T) {
	c := validCandidate()
	c.Kind = OutputKindFull
	c.OmittedClasses = []string{"video"}
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureInvalidPayload {
		t.Fatalf("got code %q, want %q", got, FailureInvalidPayload)
	}
}

func TestValidator_RejectsUnknownOutputKind(t *testing.T) {
	c := validCandidate()
	c.Kind = SynthesisOutputKind("invented")
	if got := codeOf(t, ValidateCandidate(c, validatorPolicy(), validatorAuthorized())); got != FailureInvalidPayload {
		t.Fatalf("got code %q, want %q", got, FailureInvalidPayload)
	}
}

// Failure detail must stay content-free: a code names the class, the run holds
// the instance. Leaking the through line here would put synthesis text into
// logs and metrics labels.
func TestValidator_FailureDetailCarriesNoSynthesisText(t *testing.T) {
	c := validCandidate()
	c.Insights[0].ThroughLine = "a highly distinctive secret through line"
	c.Insights[0].SourceArtifactIDs = nil
	err := ValidateCandidate(c, validatorPolicy(), validatorAuthorized())
	if err == nil {
		t.Fatal("expected rejection")
	}
	var ve *SynthesisValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("unexpected error type %T", err)
	}
	for _, forbidden := range []string{"secret through line", "highly distinctive"} {
		if containsSubstring(ve.Detail, forbidden) || containsSubstring(ve.Error(), forbidden) {
			t.Fatalf("failure detail leaked synthesis text %q: %s", forbidden, ve.Error())
		}
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
