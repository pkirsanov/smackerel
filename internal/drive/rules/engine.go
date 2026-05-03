// Package rules implements the Spec 038 Scope 5 Save Rules engine.
//
// A Save Rule describes how an inbound artifact (Telegram capture, mobile
// capture, generated meal-plan, etc.) is matched to a destination folder
// in a configured cloud-drive provider. The engine is provider-neutral —
// it consults the artifact's classification, sensitivity, source kind, and
// confidence, then renders a target folder template such as
// "Receipts/{year}" into a concrete provider path. The Save Service is
// responsible for the actual write; this package is responsible only for
// match + render + audit decisions.
package rules

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SourceKind enumerates the inbound source channels a Save Rule can match
// against. Adding a new source MUST extend this list and the validation in
// Rule.Validate so that misconfigured rules fail loudly at write time.
type SourceKind string

const (
	SourceTelegram SourceKind = "telegram"
	SourceMobile   SourceKind = "mobile"
	SourceMealPlan SourceKind = "meal_plan"
	SourceRecipe   SourceKind = "recipe"
	SourceExpense  SourceKind = "expense"
	SourceList     SourceKind = "list"
)

// IsKnownSource reports whether kind is one of the recognized source enums.
// Unknown sources MUST be treated as a config error, not silently allowed.
func IsKnownSource(kind string) bool {
	switch SourceKind(kind) {
	case SourceTelegram, SourceMobile, SourceMealPlan, SourceRecipe, SourceExpense, SourceList:
		return true
	}
	return false
}

// Sensitivity enumerates the sensitivity tiers tracked on drive_files. The
// order public→financial→medical→identity is fixed and informs guardrail
// checks in the Save Service.
type Sensitivity string

const (
	SensitivityNone      Sensitivity = "none"
	SensitivityFinancial Sensitivity = "financial"
	SensitivityMedical   Sensitivity = "medical"
	SensitivityIdentity  Sensitivity = "identity"
)

// IsKnownSensitivity reports whether s is a recognized sensitivity tier.
func IsKnownSensitivity(s string) bool {
	switch Sensitivity(s) {
	case SensitivityNone, SensitivityFinancial, SensitivityMedical, SensitivityIdentity:
		return true
	}
	return false
}

// OnMissingFolder enumerates the policies for the resolved target folder
// when it does not yet exist on the provider.
type OnMissingFolder string

const (
	OnMissingCreate OnMissingFolder = "create"
	OnMissingFail   OnMissingFolder = "fail"
)

// OnExistingFile enumerates the policies for an existing provider file that
// would be overwritten by the save.
type OnExistingFile string

const (
	OnExistingReplace OnExistingFile = "replace"
	OnExistingVersion OnExistingFile = "version"
	OnExistingSkip    OnExistingFile = "skip"
)

// Guardrails are policy fences applied before the Save Service issues a
// provider call. The fields mirror design.md §5.1.
type Guardrails struct {
	NeverLinkShare      bool    `json:"never_link_share"`
	RequireConfirmBelow float64 `json:"require_confirm_below"`
}

// Rule is one Save Rule row.
type Rule struct {
	ID                   string
	Name                 string
	Enabled              bool
	SourceKinds          []string
	Classification       string
	SensitivityIn        []string
	ConfidenceMin        float64
	ProviderID           string
	TargetFolderTemplate string
	OnMissingFolder      OnMissingFolder
	OnExistingFile       OnExistingFile
	Guardrails           Guardrails
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Validate returns an error when the rule fields are not internally
// consistent. The Save Rules HTTP handler MUST run Validate before INSERT
// so a misconfigured rule never reaches the engine.
func (r Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("rules: rule name is required")
	}
	if strings.TrimSpace(r.ProviderID) == "" {
		return errors.New("rules: provider_id is required")
	}
	if strings.TrimSpace(r.TargetFolderTemplate) == "" {
		return errors.New("rules: target_folder_template is required")
	}
	if r.ConfidenceMin < 0 || r.ConfidenceMin > 1 {
		return fmt.Errorf("rules: confidence_min %.3f out of [0,1]", r.ConfidenceMin)
	}
	for _, kind := range r.SourceKinds {
		if !IsKnownSource(kind) {
			return fmt.Errorf("rules: unknown source_kind %q", kind)
		}
	}
	for _, sensitivity := range r.SensitivityIn {
		if !IsKnownSensitivity(sensitivity) {
			return fmt.Errorf("rules: unknown sensitivity %q", sensitivity)
		}
	}
	switch r.OnMissingFolder {
	case OnMissingCreate, OnMissingFail:
	default:
		return fmt.Errorf("rules: invalid on_missing_folder %q", r.OnMissingFolder)
	}
	switch r.OnExistingFile {
	case OnExistingReplace, OnExistingVersion, OnExistingSkip:
	default:
		return fmt.Errorf("rules: invalid on_existing_file %q", r.OnExistingFile)
	}
	if r.Guardrails.RequireConfirmBelow < 0 || r.Guardrails.RequireConfirmBelow > 1 {
		return fmt.Errorf("rules: guardrails.require_confirm_below %.3f out of [0,1]", r.Guardrails.RequireConfirmBelow)
	}
	return nil
}

// Artifact is the inbound payload the engine evaluates against the rule
// set. Callers (pipeline, telegram bot, meal-plan service) populate this
// struct from their own metadata; the engine never reads provider state
// directly.
type Artifact struct {
	ID             string
	SourceKind     string
	Classification string
	Sensitivity    string
	Confidence     float64
	Title          string
	// Tokens supplies values for {token} placeholders in the
	// TargetFolderTemplate. Reserved keys: year, month, isoweek, topic,
	// audience, classification.
	Tokens map[string]string
	// CapturedAt is used to derive {year}/{month}/{isoweek} when the
	// caller does not pre-populate Tokens.
	CapturedAt time.Time
}

// MatchOutcome describes one evaluated rule outcome.
type MatchOutcome struct {
	RuleID          string
	Matched         bool
	Reason          string
	RenderedPath    string
	RenderError     error
	ConflictGroupID string // populated when more than one rule matched
	ConfirmRequired bool   // true when artifact.Confidence < guardrails.require_confirm_below
}

// Decision is the engine's verdict for one Artifact across the rule set.
// Outcomes contains every evaluated rule (matched or skipped); Selected
// is the first stable match the Save Service should execute.
type Decision struct {
	Outcomes        []MatchOutcome
	Selected        *MatchOutcome
	Conflicts       []MatchOutcome
	ConfirmRequired bool
}

// Engine evaluates an Artifact against a list of Save Rules.
type Engine struct {
	now func() time.Time
}

// NewEngine constructs an Engine. timeFn allows tests to fix the wall
// clock used for token rendering.
func NewEngine(timeFn func() time.Time) *Engine {
	if timeFn == nil {
		timeFn = time.Now
	}
	return &Engine{now: timeFn}
}

// Evaluate runs the engine against the supplied rule set in stable order
// (sorted by created_at then ID). Outcomes are returned for every rule;
// Selected is the first matched rule and Conflicts is populated with
// every additional matching rule for the audit log.
func (e *Engine) Evaluate(_ context.Context, artifact Artifact, rules []Rule) Decision {
	sorted := make([]Rule, 0, len(rules))
	sorted = append(sorted, rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	decision := Decision{}
	matched := []MatchOutcome{}
	for _, rule := range sorted {
		outcome := MatchOutcome{RuleID: rule.ID}
		if !rule.Enabled {
			outcome.Reason = "rule_disabled"
			decision.Outcomes = append(decision.Outcomes, outcome)
			continue
		}
		if reason, ok := matchSourceKinds(rule, artifact); !ok {
			outcome.Reason = reason
			decision.Outcomes = append(decision.Outcomes, outcome)
			continue
		}
		if rule.Classification != "" && rule.Classification != artifact.Classification {
			outcome.Reason = "classification_mismatch"
			decision.Outcomes = append(decision.Outcomes, outcome)
			continue
		}
		if reason, ok := matchSensitivity(rule, artifact); !ok {
			outcome.Reason = reason
			decision.Outcomes = append(decision.Outcomes, outcome)
			continue
		}
		if artifact.Confidence < rule.ConfidenceMin {
			outcome.Reason = "below_confidence_min"
			decision.Outcomes = append(decision.Outcomes, outcome)
			continue
		}
		rendered, renderErr := RenderTargetPath(rule.TargetFolderTemplate, e.tokenSet(artifact))
		outcome.Matched = true
		outcome.Reason = "matched"
		outcome.RenderedPath = rendered
		outcome.RenderError = renderErr
		if rule.Guardrails.RequireConfirmBelow > 0 && artifact.Confidence < rule.Guardrails.RequireConfirmBelow {
			outcome.ConfirmRequired = true
		}
		decision.Outcomes = append(decision.Outcomes, outcome)
		matched = append(matched, outcome)
	}
	if len(matched) == 0 {
		return decision
	}
	if len(matched) == 1 {
		first := matched[0]
		decision.Selected = &first
		decision.ConfirmRequired = first.ConfirmRequired
		return decision
	}
	first := matched[0]
	decision.Selected = &first
	decision.ConfirmRequired = first.ConfirmRequired
	for index := 0; index < len(matched); index = index + 1 {
		entry := matched[index]
		entry.ConflictGroupID = first.RuleID
		decision.Conflicts = append(decision.Conflicts, entry)
	}
	return decision
}

func matchSourceKinds(rule Rule, artifact Artifact) (string, bool) {
	if len(rule.SourceKinds) == 0 {
		return "", true
	}
	for _, kind := range rule.SourceKinds {
		if kind == artifact.SourceKind {
			return "", true
		}
	}
	return "source_kind_mismatch", false
}

func matchSensitivity(rule Rule, artifact Artifact) (string, bool) {
	if len(rule.SensitivityIn) == 0 {
		return "", true
	}
	for _, sensitivity := range rule.SensitivityIn {
		if sensitivity == artifact.Sensitivity {
			return "", true
		}
	}
	return "sensitivity_mismatch", false
}

func (e *Engine) tokenSet(artifact Artifact) map[string]string {
	tokens := make(map[string]string, len(artifact.Tokens)+8)
	for k, v := range artifact.Tokens {
		tokens[k] = v
	}
	captured := artifact.CapturedAt
	if captured.IsZero() {
		captured = e.now()
	}
	year, week := captured.ISOWeek()
	if _, ok := tokens["year"]; !ok {
		tokens["year"] = fmt.Sprintf("%04d", captured.Year())
	}
	if _, ok := tokens["month"]; !ok {
		tokens["month"] = fmt.Sprintf("%02d", int(captured.Month()))
	}
	if _, ok := tokens["isoweek"]; !ok {
		tokens["isoweek"] = fmt.Sprintf("Week-%02d", week)
	}
	if _, ok := tokens["isoyear"]; !ok {
		tokens["isoyear"] = fmt.Sprintf("%04d", year)
	}
	if _, ok := tokens["classification"]; !ok && artifact.Classification != "" {
		tokens["classification"] = artifact.Classification
	}
	if _, ok := tokens["topic"]; !ok && artifact.Tokens["topic"] == "" {
		tokens["topic"] = ""
	}
	return tokens
}
