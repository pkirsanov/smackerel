// Spec 076 SCOPE-4b — InlineClassifier wraps the legacy inline
// `interactionMap` / `sortedInteractionPhrasesList` / Parse()
// phrase-matching loop in parser.go. This adapter exists so the
// dual-write shadow comparator can speak the Classifier interface
// for BOTH the primary (inline literal) and shadow
// (`annotation.classify.v1`) read paths without modifying parser.go.
//
// The inline literal in parser.go is intentionally UNCHANGED in
// SCOPE-4b; deletion is owned by SCOPE-4c.
package annotation

import "context"

// InlineClassifier is the Classifier façade over Parse(). It re-invokes
// the existing deterministic regex + phrase-match path and returns the
// resulting InteractionType. Confidence is reported as 1.0 when a
// phrase matched (the inline literal is deterministic), 0.0 when the
// text did not contain any known interaction phrase.
//
// Errors: InlineClassifier never returns an error. A
// non-matching input is reported as ("", 0.0, nil) so the shadow
// comparator can compare it against the LLM classifier's view.
type InlineClassifier struct{}

// Classify delegates to Parse() and returns ParsedAnnotation.InteractionType.
func (InlineClassifier) Classify(_ context.Context, text string, _ SourceChannel) (InteractionType, float64, error) {
	parsed := Parse(text)
	if parsed.InteractionType == "" {
		return "", 0.0, nil
	}
	return parsed.InteractionType, 1.0, nil
}

// Compile-time assertion: InlineClassifier implements Classifier.
var _ Classifier = InlineClassifier{}
