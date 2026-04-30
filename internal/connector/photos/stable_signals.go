package photos

import (
	"errors"
	"fmt"
	"strings"
)

var ErrLLMDecisionMissingEvidence = errors.New("photos: LLM-owned decision missing confidence or rationale")

type StableSignals struct {
	Filename    string         `json:"filename"`
	MIMEType    string         `json:"mime_type"`
	ContentHash string         `json:"content_hash"`
	PHash       string         `json:"phash"`
	EXIF        map[string]any `json:"exif"`
	Albums      []string       `json:"albums"`
	Tags        []string       `json:"tags"`
}

type StableFact struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

func (signals StableSignals) SeedFacts() []StableFact {
	facts := make([]StableFact, 0, 6)
	if signals.Filename != "" {
		facts = append(facts, StableFact{Name: "filename", Value: signals.Filename})
	}
	if signals.MIMEType != "" {
		facts = append(facts, StableFact{Name: "mime_type", Value: signals.MIMEType})
	}
	if signals.ContentHash != "" {
		facts = append(facts, StableFact{Name: "content_hash", Value: signals.ContentHash})
	}
	if signals.PHash != "" {
		facts = append(facts, StableFact{Name: "phash", Value: signals.PHash})
	}
	if len(signals.EXIF) > 0 {
		facts = append(facts, StableFact{Name: "exif", Value: signals.EXIF})
	}
	if len(signals.Albums) > 0 {
		facts = append(facts, StableFact{Name: "albums", Value: signals.Albums})
	}
	return facts
}

type HeuristicDecision struct {
	Kind   DecisionKind `json:"kind"`
	Reason string       `json:"reason"`
}

func (signals StableSignals) HeuristicClassification() *HeuristicDecision {
	return nil
}

func (signals StableSignals) HeuristicLifecycleDecision() *HeuristicDecision {
	return nil
}

func (signals StableSignals) HeuristicDuplicateBestPick() *HeuristicDecision {
	return nil
}

type DecisionKind string

const (
	DecisionClassification DecisionKind = "classification"
	DecisionLifecycle      DecisionKind = "lifecycle"
	DecisionDedupe         DecisionKind = "dedupe"
	DecisionAesthetic      DecisionKind = "aesthetic"
	DecisionSensitivity    DecisionKind = "sensitivity"
	DecisionRemoval        DecisionKind = "removal"
	DecisionRouting        DecisionKind = "routing"
)

type LLMDecision struct {
	Kind       DecisionKind `json:"kind"`
	Confidence *float64     `json:"confidence"`
	Rationale  string       `json:"rationale"`
}

type DecisionIssue struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Kind    DecisionKind `json:"kind"`
	Visible bool         `json:"visible"`
}

func ValidateLLMDecision(decision LLMDecision) (*DecisionIssue, error) {
	var missing []string
	if decision.Kind == "" {
		missing = append(missing, "kind")
	}
	if decision.Confidence == nil {
		missing = append(missing, "confidence")
	} else if *decision.Confidence < 0 || *decision.Confidence > 1 {
		missing = append(missing, "confidence")
	}
	if strings.TrimSpace(decision.Rationale) == "" {
		missing = append(missing, "rationale")
	}
	if len(missing) == 0 {
		return nil, nil
	}
	issue := &DecisionIssue{
		Code:    "PHOTO_LLM_DECISION_INCOMPLETE",
		Message: fmt.Sprintf("photo %s decision missing required %s", decision.Kind, strings.Join(missing, ", ")),
		Kind:    decision.Kind,
		Visible: true,
	}
	return issue, fmt.Errorf("%w: %s", ErrLLMDecisionMissingEvidence, issue.Message)
}
