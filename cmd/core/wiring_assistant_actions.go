package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	listdomain "github.com/smackerel/smackerel/internal/list"
)

type assistantCompiledActionExecutor struct {
	lists       listdomain.ListStore
	annotations annotation.AnnotationQuerier
}

func newAssistantCompiledActionExecutor(
	lists listdomain.ListStore,
	annotations annotation.AnnotationQuerier,
) (*assistantCompiledActionExecutor, error) {
	if lists == nil {
		return nil, errors.New("assistant compiled actions require list store")
	}
	if annotations == nil {
		return nil, errors.New("assistant compiled actions require annotation store")
	}
	return &assistantCompiledActionExecutor{lists: lists, annotations: annotations}, nil
}

func (e *assistantCompiledActionExecutor) Prepare(
	_ context.Context,
	proposal assistant.CompiledActionProposal,
) (assistant.PreparedCompiledAction, error) {
	scenario := compiledActionScenario(proposal)
	proposal.ConfirmRef = "confirm-" + uuid.NewString()
	var label string
	switch scenario {
	case "shopping_list_assemble":
		item, err := requiredStringSlot(proposal, "item")
		if err != nil {
			return assistant.PreparedCompiledAction{}, err
		}
		label = "Add " + item + " to a shopping list"
	case "annotation_classify":
		artifactID, err := requiredStringSlot(proposal, "artifact_id")
		if err != nil {
			return assistant.PreparedCompiledAction{}, err
		}
		label = "Apply compiled annotation to " + artifactID
	default:
		return assistant.PreparedCompiledAction{}, fmt.Errorf("unsupported compiled write scenario %q", scenario)
	}
	return assistant.PreparedCompiledAction{Proposal: proposal, ProposedAction: label}, nil
}

func (e *assistantCompiledActionExecutor) Execute(
	ctx context.Context,
	proposal assistant.CompiledActionProposal,
) (assistant.CompiledActionResult, error) {
	switch compiledActionScenario(proposal) {
	case "shopping_list_assemble":
		return e.executeShoppingList(ctx, proposal)
	case "annotation_classify":
		return e.executeAnnotation(ctx, proposal)
	default:
		return assistant.CompiledActionResult{}, fmt.Errorf("unsupported confirmed write scenario %q", compiledActionScenario(proposal))
	}
}

func compiledActionScenario(proposal assistant.CompiledActionProposal) string {
	if proposal.Intent.ScenarioHint == nil {
		return ""
	}
	return strings.TrimSpace(*proposal.Intent.ScenarioHint)
}

func requiredStringSlot(proposal assistant.CompiledActionProposal, key string) (string, error) {
	value, ok := proposal.Intent.Slots[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("compiled action slot %q is required", key)
	}
	return value, nil
}

func (e *assistantCompiledActionExecutor) executeShoppingList(
	ctx context.Context,
	proposal assistant.CompiledActionProposal,
) (assistant.CompiledActionResult, error) {
	item, err := requiredStringSlot(proposal, "item")
	if err != nil {
		return assistant.CompiledActionResult{}, err
	}
	now := time.Now().UTC()
	listID := "lst-assistant-" + uuid.NewString()
	itemID := "itm-assistant-" + uuid.NewString()
	list := &listdomain.List{
		ID: listID, ListType: listdomain.TypeShopping,
		Title: "Assistant shopping list", Status: listdomain.StatusActive,
		SourceArtifactIDs: []string{}, SourceQuery: proposal.TransportMessageID,
		Domain: "assistant", CreatedAt: now, UpdatedAt: now,
	}
	items := []listdomain.ListItem{{
		ID: itemID, ListID: listID, Content: item, Status: listdomain.ItemPending,
		SourceArtifactIDs: []string{}, IsManual: true, SortOrder: 1,
		CreatedAt: now, UpdatedAt: now,
	}}
	if err := e.lists.CreateList(ctx, list, items); err != nil {
		return assistant.CompiledActionResult{}, err
	}
	return assistant.CompiledActionResult{
		Status: contracts.StatusThinking, Body: "Shopping list updated.",
	}, nil
}

func (e *assistantCompiledActionExecutor) executeAnnotation(
	ctx context.Context,
	proposal assistant.CompiledActionProposal,
) (assistant.CompiledActionResult, error) {
	artifactID, err := requiredStringSlot(proposal, "artifact_id")
	if err != nil {
		return assistant.CompiledActionResult{}, err
	}
	interactionRaw, err := requiredStringSlot(proposal, "interaction_type")
	if err != nil {
		return assistant.CompiledActionResult{}, err
	}
	interaction := annotation.InteractionType(interactionRaw)
	switch interaction {
	case annotation.InteractionMadeIt, annotation.InteractionBoughtIt,
		annotation.InteractionReadIt, annotation.InteractionVisited,
		annotation.InteractionTriedIt, annotation.InteractionUsedIt:
	default:
		return assistant.CompiledActionResult{}, fmt.Errorf("compiled interaction_type %q is invalid", interactionRaw)
	}
	parsed := annotation.ParsedAnnotation{InteractionType: interaction}
	if note, ok := proposal.Intent.Slots["note"].(string); ok {
		parsed.Note = strings.TrimSpace(note)
	}
	if ratingRaw, ok := proposal.Intent.Slots["rating"].(float64); ok {
		rating := int(ratingRaw)
		if rating < 1 || rating > 5 || float64(rating) != ratingRaw {
			return assistant.CompiledActionResult{}, fmt.Errorf("compiled rating %v is invalid", ratingRaw)
		}
		parsed.Rating = &rating
	}
	created, err := e.annotations.CreateFromParsedAs(ctx, artifactID, parsed, annotation.ChannelWeb, proposal.UserID)
	if err != nil {
		return assistant.CompiledActionResult{}, err
	}
	if len(created) == 0 {
		return assistant.CompiledActionResult{}, errors.New("compiled annotation produced no events")
	}
	return assistant.CompiledActionResult{
		Status: contracts.StatusThinking, Body: "Annotation applied.",
	}, nil
}
