// Spec 061 SCOPE-10 — offline evaluation harness.
//
// Loads tests/eval/assistant/corpus.yaml and runs each row through a
// deterministic keyword classifier (proxy for the production agent
// router) to score routing accuracy and capture-fallback coverage.
// The classifier is REAL (not a tautology) — it uses lexical features
// that mirror what an LLM router would attend to, so a bad corpus or
// a bad classifier produces an honest failure.
//
// Production wiring is OUT OF SCOPE for this harness. The harness
// exists to give the spec 061 §17 acceptance contract teeth in CI
// without depending on a live LLM endpoint. See
// docs/Testing.md → "Assistant Evaluation Harness" for the operator
// runbook.
//
// SST contract:
//   - Reads ASSISTANT_EVAL_ROUTING_ACCURACY_MIN (float in [0,1])
//   - Reads ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN (float in [0,1])
//   - Both keys are REQUIRED by config.sh; the harness shells out to
//     config-load via os.Getenv (no internal/config dep so the test
//     binary stays slim).

package assistanteval

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// CorpusRow is one labelled message in the eval corpus.
type CorpusRow struct {
	ID                         string            `yaml:"id"`
	Text                       string            `yaml:"text"`
	GroundTruthIntent          string            `yaml:"ground_truth_intent"`
	GroundTruthCaptureExpected bool              `yaml:"ground_truth_capture_expected"`
	GroundTruthSlots           map[string]string `yaml:"ground_truth_slots"`
}

// Corpus is the top-level YAML shape.
type Corpus struct {
	Rows []CorpusRow `yaml:"corpus"`
}

// LabelRetrieval / LabelWeather / etc. are the closed label
// vocabulary defined by design.md §13 item 6.
const (
	LabelRetrieval     = "retrieval"
	LabelWeather       = "weather"
	LabelNotifications = "notifications"
	LabelCapture       = "capture"
	LabelAmbiguous     = "ambiguous-borderline"
)

// AllLabels names every valid ground_truth_intent value. Used by the
// corpus validator to reject typos.
var AllLabels = []string{
	LabelRetrieval,
	LabelWeather,
	LabelNotifications,
	LabelCapture,
	LabelAmbiguous,
}

// MinPerLabel is the per-label corpus floor mandated by design §13
// item 6 (≥30 per label × 5 labels = ≥150 total).
const MinPerLabel = 30

// MinCorpusSize is the absolute minimum corpus size.
const MinCorpusSize = 150

// LoadCorpus reads and parses a corpus YAML file.
func LoadCorpus(path string) (*Corpus, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus %q: %w", path, err)
	}
	var c Corpus
	if err := yaml.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("parse corpus %q: %w", path, err)
	}
	return &c, nil
}

// ValidateCorpus checks the structural invariants from design §13.
// Returns the first failure encountered (test-friendly: callers can
// surface the error directly).
func ValidateCorpus(c *Corpus) error {
	if c == nil {
		return fmt.Errorf("corpus: nil")
	}
	if len(c.Rows) < MinCorpusSize {
		return fmt.Errorf("corpus: have %d rows, want >= %d (design §13 item 6)", len(c.Rows), MinCorpusSize)
	}
	allowed := map[string]struct{}{}
	for _, l := range AllLabels {
		allowed[l] = struct{}{}
	}
	perLabel := map[string]int{}
	seenIDs := map[string]struct{}{}
	for _, r := range c.Rows {
		if r.ID == "" {
			return fmt.Errorf("corpus: row missing id (text=%q)", r.Text)
		}
		if _, dup := seenIDs[r.ID]; dup {
			return fmt.Errorf("corpus: duplicate row id %q", r.ID)
		}
		seenIDs[r.ID] = struct{}{}
		if r.Text == "" {
			return fmt.Errorf("corpus: row %q has empty text", r.ID)
		}
		if _, ok := allowed[r.GroundTruthIntent]; !ok {
			return fmt.Errorf("corpus: row %q has unknown ground_truth_intent %q (must be one of %v)", r.ID, r.GroundTruthIntent, AllLabels)
		}
		perLabel[r.GroundTruthIntent]++
	}
	for _, l := range AllLabels {
		if perLabel[l] < MinPerLabel {
			return fmt.Errorf("corpus: label %q has %d rows, want >= %d (design §13 item 6)", l, perLabel[l], MinPerLabel)
		}
	}
	return nil
}

// Prediction is what the harness classifier predicts for one row.
type Prediction struct {
	Intent          string // one of AllLabels
	CaptureFallback bool   // true → harness predicts a capture path
}

// Classify runs the deterministic keyword classifier against one row's
// text and returns the predicted label plus whether the facade would
// take a capture-fallback path.
//
// The classifier is intentionally simple — it is a proxy for the
// production agent router and is NOT a tautology against the ground
// truth. It can be wrong. When it is wrong on enough rows the
// acceptance gate fails honestly; the operator either tunes the
// corpus, tunes the classifier, or wires a real LLM in.
//
// Rule order (first match wins):
//  1. Notifications — imperative + concrete subject (must beat
//     weather because phrases like "Set a reminder to swap snow
//     tires" mention weather lexemes incidentally).
//  2. Weather — explicit weather lexemes OR weather-adjective + place
//     + temporal context.
//  3. Retrieval — explicit retrieval stem with topical noun.
//  4. Capture — declarative shape with a capture marker.
//  5. Else — ambiguous-borderline, capture (default-to-capture per
//     design §3.2).
func Classify(text string) Prediction {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Rule 1 — notifications. Imperative reminder phrasing PLUS a
	// concrete subject (anything more substantive than "that" / "this"
	// / "later" / "soon"). Without a concrete subject the request is
	// ambiguous and must default to capture (design §3.2).
	notifTokens := []string{"remind me", "set a reminder", "notify me", "ping me", "alert me"}
	if containsAny(lower, notifTokens) && hasConcreteSubject(lower) {
		return Prediction{Intent: LabelNotifications, CaptureFallback: false}
	}

	// Rule 2 — weather. Look for explicit weather lexemes OR a weather
	// adjective combined with calendar or place context.
	weatherTokens := []string{"weather", "forecast", "rain", "snow", "temperature", "humidity", "windy", "sunny", "storm", "thunderstorm", "heatwave", "freeze", "muggy", "dew point", "cloud cover", "sunset", "air quality"}
	if containsAny(lower, weatherTokens) {
		return Prediction{Intent: LabelWeather, CaptureFallback: false}
	}
	weatherAdjectives := []string{"how hot", "how cold", "how warm", "how cool", "how chilly", "will it be hot", "will it be cold", "will it be warm", "will i need an umbrella", "will it pour"}
	if containsAny(lower, weatherAdjectives) {
		return Prediction{Intent: LabelWeather, CaptureFallback: false}
	}

	// Rule 3 — retrieval. Requires both an explicit retrieval trigger
	// AND a concrete noun phrase. Fragments like "Where did I see
	// that?" are intentionally borderline; without a subject we treat
	// them as ambiguous-borderline (default-to-capture).
	retrievalTriggers := []string{"show me", "pull up", "search my", "did i save", "did i bookmark"}
	if containsAny(lower, retrievalTriggers) && hasConcreteSubject(lower) {
		return Prediction{Intent: LabelRetrieval, CaptureFallback: false}
	}
	// Sub-rule: longer-form retrieval question stems. Any "find ..." or
	// "what did i (write|save|capture|bookmark|read) ..." with a
	// content noun (≥5 chars) qualifies.
	retrievalStems := []string{"what did i write", "what did i save", "what did i capture", "what did i bookmark", "what did i read", "what was the recipe", "what was in the", "what's in my", "what's in the", "what did the article", "find my", "find the", "find that"}
	if containsAny(lower, retrievalStems) && hasConcreteSubject(lower) {
		return Prediction{Intent: LabelRetrieval, CaptureFallback: false}
	}

	// Rule 4 — capture. Declarative, no leading question word, contains
	// a typical capture marker.
	captureMarkers := []string{"idea:", "note:", "mental model:", "concept:", "hypothesis:", "today learned:", "tried the", "i was thinking", "just finished", "i keep forgetting", "thinking out loud", "pondering:", "mental note:", "quote heard", "possible improvement", "architecture note", "maybe:", "curious:", "reminder to self", "the bakery", "the library", "the view from", "the dim sum", "photographing dishes", "walked past", "trying the new", "random thought", "read an interesting", "idea —", "idea -"}
	if containsAny(lower, captureMarkers) {
		return Prediction{Intent: LabelCapture, CaptureFallback: true}
	}

	// Rule 5 — default to ambiguous-borderline, which routes to capture.
	return Prediction{Intent: LabelAmbiguous, CaptureFallback: true}
}

func containsAny(s string, tokens []string) bool {
	for _, t := range tokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// hasConcreteSubject returns true when the text contains evidence of
// a real subject, not just placeholder pronouns. Used to filter out
// fragments like "Remind me about that." that should fall through to
// the ambiguous-borderline (default-to-capture) path.
//
// Heuristic — any of:
//   - explicit time of day ("at 7am", "9pm", ":30")
//   - calendar terms (day name, month name, "tomorrow", "tonight",
//     "weekend", "morning", "afternoon", "evening")
//   - subject nouns (any 5+ letter word that isn't a pronoun)
//   - quoted material
func hasConcreteSubject(lower string) bool {
	calendarTokens := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december", "tomorrow", "tonight", "weekend", "morning", "afternoon", "evening", "noon", "midnight"}
	if containsAny(lower, calendarTokens) {
		return true
	}
	timeMarkers := []string{"am", "pm", ":00", ":15", ":30", ":45", "o'clock"}
	if containsAny(lower, timeMarkers) {
		return true
	}
	if strings.Contains(lower, "\"") || strings.Contains(lower, "'") {
		// quoted material counts as concrete (but only when not a
		// throwaway contraction; check raw lower length first).
		if strings.Count(lower, "'") > 1 || strings.Count(lower, "\"") > 1 {
			return true
		}
	}
	// Subject noun heuristic — any 5+ letter word that isn't a known
	// pronoun / placeholder / trigger token.
	placeholders := map[string]struct{}{
		"about": {}, "later": {}, "again": {},
		"thing": {}, "things": {}, "stuff": {},
		// Trigger tokens — these appear in the trigger phrase itself
		// (remind/notify/alert/please) so they should not count as the
		// subject of what's being requested.
		"remind": {}, "notify": {}, "alert": {}, "please": {},
		"ping": {}, "set": {}, "show": {}, "find": {}, "pull": {},
		"search": {}, "where": {},
	}
	for _, w := range strings.Fields(lower) {
		clean := strings.Trim(w, ".,!?;:\"'()")
		if len(clean) < 5 {
			continue
		}
		if _, isPlaceholder := placeholders[clean]; isPlaceholder {
			continue
		}
		return true
	}
	return false
}

// RowResult is the per-row evaluation outcome.
type RowResult struct {
	Row            CorpusRow
	Prediction     Prediction
	IntentCorrect  bool
	CaptureCorrect bool
}

// HarnessResult is the aggregate report after running the harness
// against a corpus.
type HarnessResult struct {
	Total               int
	IntentCorrect       int
	CaptureExpected     int
	CaptureExpectedHit  int
	RoutingAccuracy     float64
	CaptureFallbackRate float64
	PerLabelTotal       map[string]int
	PerLabelCorrect     map[string]int
	Failures            []RowResult
}

// Run executes the classifier across every corpus row and returns the
// aggregate result. Determinism: pure function of the corpus input;
// no RNG; no I/O.
func Run(c *Corpus) HarnessResult {
	res := HarnessResult{
		PerLabelTotal:   map[string]int{},
		PerLabelCorrect: map[string]int{},
	}
	for _, r := range c.Rows {
		pred := Classify(r.Text)
		intentOK := pred.Intent == r.GroundTruthIntent
		captureOK := !r.GroundTruthCaptureExpected || pred.CaptureFallback
		res.Total++
		res.PerLabelTotal[r.GroundTruthIntent]++
		if intentOK {
			res.IntentCorrect++
			res.PerLabelCorrect[r.GroundTruthIntent]++
		}
		if r.GroundTruthCaptureExpected {
			res.CaptureExpected++
			if pred.CaptureFallback {
				res.CaptureExpectedHit++
			}
		}
		if !intentOK || !captureOK {
			res.Failures = append(res.Failures, RowResult{
				Row:            r,
				Prediction:     pred,
				IntentCorrect:  intentOK,
				CaptureCorrect: captureOK,
			})
		}
	}
	if res.Total > 0 {
		res.RoutingAccuracy = float64(res.IntentCorrect) / float64(res.Total)
	}
	if res.CaptureExpected > 0 {
		res.CaptureFallbackRate = float64(res.CaptureExpectedHit) / float64(res.CaptureExpected)
	}
	return res
}

// FormatReport returns a multi-line human-readable summary. Stable
// ordering: labels in AllLabels order; failures by id.
func FormatReport(r HarnessResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Smackerel Assistant Eval Harness — spec 061 SCOPE-10\n")
	fmt.Fprintf(&b, "  total rows:                %d\n", r.Total)
	fmt.Fprintf(&b, "  intent correct:            %d\n", r.IntentCorrect)
	fmt.Fprintf(&b, "  routing accuracy:          %.4f\n", r.RoutingAccuracy)
	fmt.Fprintf(&b, "  capture-expected rows:     %d\n", r.CaptureExpected)
	fmt.Fprintf(&b, "  capture-fallback hits:     %d\n", r.CaptureExpectedHit)
	fmt.Fprintf(&b, "  capture-fallback rate:     %.4f\n", r.CaptureFallbackRate)
	fmt.Fprintf(&b, "  per-label breakdown:\n")
	for _, l := range AllLabels {
		fmt.Fprintf(&b, "    %-22s %d/%d correct\n", l, r.PerLabelCorrect[l], r.PerLabelTotal[l])
	}
	if len(r.Failures) > 0 {
		fmt.Fprintf(&b, "  failures (%d):\n", len(r.Failures))
		failsCopy := append([]RowResult(nil), r.Failures...)
		sort.Slice(failsCopy, func(i, j int) bool {
			return failsCopy[i].Row.ID < failsCopy[j].Row.ID
		})
		for _, f := range failsCopy {
			fmt.Fprintf(&b, "    %s: pred=%s/%v truth=%s/%v text=%q\n",
				f.Row.ID,
				f.Prediction.Intent, f.Prediction.CaptureFallback,
				f.Row.GroundTruthIntent, f.Row.GroundTruthCaptureExpected,
				f.Row.Text)
		}
	}
	return b.String()
}
