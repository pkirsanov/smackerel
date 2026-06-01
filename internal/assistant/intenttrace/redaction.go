// Spec 071 SCOPE-02 — Source-policy-driven redaction (SCN-071-A03).
//
// Redactor converts a turn's raw text + slot map into a
// SlotsRedactionSummary that the recorder persists. It NEVER returns
// raw slot values. When the per-source policy sets PersistRawText=false
// (the default for spec 071 — Principle 8: trust through
// transparency), raw_text is marked "absent" and no caller may
// re-introduce it downstream.

package intenttrace

import "sort"

// SourcePolicy is the per-source privacy contract applied before
// persistence. Spec 071 §"Hard Constraint 2" — redaction is centralised
// and applied before logging/metrics/export. Every field is REQUIRED
// at construction time (no defaults).
type SourcePolicy struct {
	// PersistRawText controls whether the raw user-visible text is
	// persisted on the trace payload. False = "absent".
	PersistRawText bool
	// SensitiveSlotClasses is the set of slot keys whose values
	// must be redacted (replaced by their class label only).
	SensitiveSlotClasses map[string]struct{}
}

// NewSourcePolicy validates the policy. Nil SensitiveSlotClasses is
// allowed (treated as empty set); PersistRawText must be set explicitly
// by the caller because spec 071 forbids hidden defaults.
func NewSourcePolicy(persistRawText bool, sensitive []string) SourcePolicy {
	set := make(map[string]struct{}, len(sensitive))
	for _, k := range sensitive {
		set[k] = struct{}{}
	}
	return SourcePolicy{PersistRawText: persistRawText, SensitiveSlotClasses: set}
}

// Redactor is the central redaction interface.
type Redactor interface {
	Redact(policy SourcePolicy, rawText string, slots map[string]any) RedactionResult
}

// RedactionResult is the structured outcome of a Redact() call.
type RedactionResult struct {
	RawText string // "absent" | "present"
	Summary SlotsRedactionSummary
}

// DefaultRedactor is the production redactor.
type DefaultRedactor struct{}

// NewDefaultRedactor returns the production redactor.
func NewDefaultRedactor() DefaultRedactor { return DefaultRedactor{} }

// Redact implements Redactor.
func (DefaultRedactor) Redact(policy SourcePolicy, rawText string, slots map[string]any) RedactionResult {
	rawDisposition := "absent"
	if policy.PersistRawText && rawText != "" {
		rawDisposition = "present"
	}
	classes := make(map[string]string, len(slots))
	redacted := 0
	// Stable iteration so the persisted JSON is deterministic for
	// payload-hash and golden-fixture tests.
	keys := make([]string, 0, len(slots))
	for k := range slots {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, sensitive := policy.SensitiveSlotClasses[k]; sensitive {
			classes[k] = "redacted"
			redacted++
			continue
		}
		classes[k] = "safe"
	}
	return RedactionResult{
		RawText: rawDisposition,
		Summary: SlotsRedactionSummary{
			RawText:       rawDisposition,
			SlotClasses:   classes,
			RedactedCount: redacted,
		},
	}
}
