package photos

import (
	"strings"
)

type ClassificationDecision struct {
	Caption         string   `json:"caption"`
	PrimaryCategory string   `json:"primary_category"`
	DocumentType    string   `json:"document_type,omitempty"`
	OCRText         string   `json:"ocr_text,omitempty"`
	OCRSnippet      string   `json:"ocr_snippet,omitempty"`
	Objects         []string `json:"objects,omitempty"`
	Confidence      float64  `json:"confidence"`
	Rationale       string   `json:"rationale"`
	Embedded        bool     `json:"embedded"`
}

func (decision ClassificationDecision) Validate() (*DecisionIssue, error) {
	issue, err := ValidateLLMDecision(LLMDecision{
		Kind:       DecisionClassification,
		Confidence: &decision.Confidence,
		Rationale:  decision.Rationale,
	})
	if err != nil {
		return issue, err
	}
	var missing []string
	if strings.TrimSpace(decision.Caption) == "" {
		missing = append(missing, "caption")
	}
	if strings.TrimSpace(decision.PrimaryCategory) == "" {
		missing = append(missing, "primary_category")
	}
	if len(missing) == 0 {
		return nil, nil
	}
	decisionIssue := &DecisionIssue{
		Code:    "PHOTO_CLASSIFICATION_INCOMPLETE",
		Message: "photo classification missing required " + strings.Join(missing, ", "),
		Kind:    DecisionClassification,
		Visible: true,
	}
	return decisionIssue, ErrLLMDecisionMissingEvidence
}

func (decision ClassificationDecision) SearchText() string {
	parts := []string{
		decision.Caption,
		decision.PrimaryCategory,
		decision.DocumentType,
		decision.OCRText,
		decision.OCRSnippet,
		strings.Join(decision.Objects, " "),
		decision.Rationale,
	}
	return strings.Join(parts, " ")
}

func (decision ClassificationDecision) Snippet() string {
	if strings.TrimSpace(decision.OCRSnippet) != "" {
		return decision.OCRSnippet
	}
	if strings.TrimSpace(decision.OCRText) != "" {
		return decision.OCRText
	}
	return decision.Caption
}
