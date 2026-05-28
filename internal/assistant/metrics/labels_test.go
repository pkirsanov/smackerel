// Spec 061 SCOPE-09 — closed-vocabulary label assertions.
//
// Each spec 061 §8.1 metric label MUST resolve to a finite,
// agent-known set; this file is the gate that keeps the
// vocabularies bounded so the Prometheus storage cost stays linear
// in scenario count and transport count (NOT linear in user count
// or message count).
//
// If a future change introduces a label value outside one of these
// sets, the corresponding TestVocabularyClosed_* test fails and the
// PR is blocked.
package assistantmetrics

import (
	"testing"
)

// TestVocabularyClosed_Transport pins the transport label closed
// vocabulary against the design (telegram in v1; fake reserved for
// tests/eval). New transports MUST be added here AND in
// metrics.go::AllTransports in the same PR so cardinality stays
// auditable.
func TestVocabularyClosed_Transport(t *testing.T) {
	expected := map[string]struct{}{
		TransportTelegram: {},
		TransportFake:     {},
	}
	assertVocabClosed(t, "transport", expected, AllTransports)
}

func TestVocabularyClosed_FacadeOutcome(t *testing.T) {
	expected := map[string]struct{}{
		OutcomeAnswered:  {},
		OutcomeCaptured:  {},
		OutcomeProposed:  {},
		OutcomeConfirmed: {},
		OutcomeDiscarded: {},
		OutcomeError:     {},
	}
	assertVocabClosed(t, "facade outcome", expected, AllFacadeOutcomes)
}

func TestVocabularyClosed_Band(t *testing.T) {
	expected := map[string]struct{}{
		BandHigh:       {},
		BandBorderline: {},
		BandLow:        {},
	}
	assertVocabClosed(t, "router band", expected, AllBands)
}

func TestVocabularyClosed_SkillOutcome(t *testing.T) {
	expected := map[string]struct{}{
		SkillOutcomeOK:                   {},
		SkillOutcomeTimeout:              {},
		SkillOutcomeProviderError:        {},
		SkillOutcomeSchemaFailure:        {},
		SkillOutcomeToolReturnInvalid:    {},
		SkillOutcomeInputSchemaViolation: {},
		SkillOutcomeLoopLimit:            {},
		SkillOutcomeUnknownIntent:        {},
	}
	assertVocabClosed(t, "skill outcome", expected, AllSkillOutcomes)
}

func TestVocabularyClosed_CaptureFallbackCause(t *testing.T) {
	expected := map[string]struct{}{
		CauseLowConfidence:         {},
		CauseBorderlineTimeout:     {},
		CauseConfirmDiscarded:      {},
		CauseConfirmTimeout:        {},
		CauseErrorOfferedCapture:   {},
		CauseUnresolvableReference: {},
	}
	assertVocabClosed(t, "capture-fallback cause", expected, AllCaptureFallbackCauses)
}

func TestVocabularyClosed_ConfirmCardOutcome(t *testing.T) {
	expected := map[string]struct{}{
		ConfirmOutcomeConfirmed:        {},
		ConfirmOutcomeDiscardedUser:    {},
		ConfirmOutcomeDiscardedTimeout: {},
	}
	assertVocabClosed(t, "confirm-card outcome", expected, AllConfirmCardOutcomes)
}

func TestVocabularyClosed_DisambigOutcome(t *testing.T) {
	expected := map[string]struct{}{
		DisambigOutcomeResolvedUser:             {},
		DisambigOutcomeResolvedTimeoutCapture:   {},
		DisambigOutcomeResolvedNonMatchingReply: {},
	}
	assertVocabClosed(t, "disambiguation outcome", expected, AllDisambigOutcomes)
}

// assertVocabClosed proves the AllXxx slice is exactly the expected
// set (symmetric: no missing items, no extra items). Adversarial: a
// future drift in either direction fails.
func assertVocabClosed(t *testing.T, name string, expected map[string]struct{}, all []string) {
	t.Helper()
	seen := make(map[string]struct{}, len(all))
	for _, v := range all {
		if _, ok := expected[v]; !ok {
			t.Errorf("vocab[%s]: unexpected value %q in AllXxx slice (cardinality leak)", name, v)
		}
		seen[v] = struct{}{}
	}
	for v := range expected {
		if _, ok := seen[v]; !ok {
			t.Errorf("vocab[%s]: missing expected value %q from AllXxx slice", name, v)
		}
	}
}
