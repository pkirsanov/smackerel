package mealplan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/list"
	"github.com/smackerel/smackerel/internal/recipe"
)

// ShoppingBridge converts meal plan slots into shopping lists using the existing
// RecipeAggregator and Generator from spec 028.
type ShoppingBridge struct {
	Resolver   list.ArtifactResolver
	Aggregator list.Aggregator
	Store      list.ListStore
}

// NewShoppingBridge creates a new shopping bridge.
func NewShoppingBridge(resolver list.ArtifactResolver, aggregator list.Aggregator, store list.ListStore) *ShoppingBridge {
	return &ShoppingBridge{
		Resolver:   resolver,
		Aggregator: aggregator,
		Store:      store,
	}
}

// GenerateFromPlan creates a shopping list from a meal plan's slots.
// It loads domain_data, scales per-slot, then delegates to the existing aggregator.
func (b *ShoppingBridge) GenerateFromPlan(ctx context.Context, plan PlanWithSlots, force bool) (*ShoppingResult, error) {
	if len(plan.Slots) == 0 {
		return nil, &ServiceError{Code: "MEAL_PLAN_EMPTY", Message: "plan has no recipe assignments", Status: 422}
	}

	// Check for existing list
	sourceQuery := fmt.Sprintf("plan:%s", plan.Plan.ID)
	existingList, err := b.findExistingList(ctx, sourceQuery)
	if err != nil {
		slog.Warn("shopping bridge: failed to check existing list", "error", err)
	}
	if existingList != nil && !force {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_LIST_EXISTS",
			Message: "shopping list already exists for this plan",
			Status:  409,
			Details: map[string]any{
				"existing_list_id":         existingList.ID,
				"plan_modified_since_list": plan.Plan.UpdatedAt.After(existingList.CreatedAt),
			},
		}
	}

	// If force and existing list, archive it
	if existingList != nil && force {
		if err := b.Store.ArchiveList(ctx, existingList.ID); err != nil {
			slog.Warn("shopping bridge: failed to archive existing list", "list_id", existingList.ID, "error", err)
		}
	}

	// Collect unique artifact IDs
	artifactIDs := make(map[string]bool)
	for _, slot := range plan.Slots {
		artifactIDs[slot.RecipeArtifactID] = true
	}
	ids := make([]string, 0, len(artifactIDs))
	for id := range artifactIDs {
		ids = append(ids, id)
	}

	// Load domain_data for all recipes
	sources, err := b.Resolver.ResolveByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve artifacts: %w", err)
	}

	// Index domain_data by artifact ID
	domainDataMap := make(map[string]json.RawMessage)
	for _, src := range sources {
		domainDataMap[src.ArtifactID] = src.DomainData
	}

	// Group batch slots and build AggregationSources
	var scaledSources []list.AggregationSource
	var scalingSummary []ScalingSummaryEntry
	var skipped []string

	// Group batch slots by (artifactID, mealType) for consolidation
	type batchKey struct {
		artifactID string
		mealType   string
	}
	batchGroups := make(map[batchKey][]Slot)
	var nonBatchSlots []Slot

	for _, slot := range plan.Slots {
		if slot.BatchFlag {
			key := batchKey{artifactID: slot.RecipeArtifactID, mealType: slot.MealType}
			batchGroups[key] = append(batchGroups[key], slot)
		} else {
			nonBatchSlots = append(nonBatchSlots, slot)
		}
	}

	// Process batch groups (consolidated)
	for key, slots := range batchGroups {
		domainData, ok := domainDataMap[key.artifactID]
		if !ok || len(domainData) == 0 {
			skipped = append(skipped, key.artifactID)
			continue
		}

		totalServings := 0
		for _, sl := range slots {
			totalServings += sl.Servings
		}

		scaledData, err := scaleRecipeDomainData(domainData, totalServings)
		if err != nil {
			slog.Warn("shopping bridge: failed to scale recipe", "artifact_id", key.artifactID, "error", err)
			skipped = append(skipped, key.artifactID)
			continue
		}

		scaledSources = append(scaledSources, list.AggregationSource{
			ArtifactID: key.artifactID,
			DomainData: scaledData,
		})

		scalingSummary = append(scalingSummary, ScalingSummaryEntry{
			RecipeTitle:   resolveTitle(domainData),
			ArtifactID:    key.artifactID,
			Servings:      slots[0].Servings,
			Occurrences:   len(slots),
			TotalServings: totalServings,
		})
	}

	// Process non-batch slots individually
	for _, slot := range nonBatchSlots {
		domainData, ok := domainDataMap[slot.RecipeArtifactID]
		if !ok || len(domainData) == 0 {
			skipped = append(skipped, slot.RecipeArtifactID)
			continue
		}

		scaledData, err := scaleRecipeDomainData(domainData, slot.Servings)
		if err != nil {
			slog.Warn("shopping bridge: failed to scale recipe", "artifact_id", slot.RecipeArtifactID, "error", err)
			skipped = append(skipped, slot.RecipeArtifactID)
			continue
		}

		scaledSources = append(scaledSources, list.AggregationSource{
			ArtifactID: slot.RecipeArtifactID,
			DomainData: scaledData,
		})

		scalingSummary = append(scalingSummary, ScalingSummaryEntry{
			RecipeTitle:   resolveTitle(domainData),
			ArtifactID:    slot.RecipeArtifactID,
			Servings:      slot.Servings,
			Occurrences:   1,
			TotalServings: slot.Servings,
		})
	}

	if len(scaledSources) == 0 {
		return nil, &ServiceError{
			Code:    "MEAL_PLAN_EMPTY",
			Message: "no recipes with ingredient data found in the plan",
			Status:  422,
		}
	}

	// Aggregate using existing RecipeAggregator
	seeds, err := b.Aggregator.Aggregate(scaledSources)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}

	// Build list and items (same pattern as Generator.Generate)
	now := time.Now()
	listID := fmt.Sprintf("lst-%d", now.UnixNano())
	listTitle := fmt.Sprintf("%s Shopping", plan.Plan.Title)

	l := &list.List{
		ID:                listID,
		ListType:          list.TypeShopping,
		Title:             listTitle,
		Status:            list.StatusDraft,
		SourceArtifactIDs: ids,
		SourceQuery:       sourceQuery,
		Domain:            "recipe",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	items := make([]list.ListItem, len(seeds))
	for i, seed := range seeds {
		items[i] = list.ListItem{
			ID:                fmt.Sprintf("itm-%s-%d", listID[:min(8, len(listID))], i),
			ListID:            listID,
			Content:           seed.Content,
			Category:          seed.Category,
			Status:            list.ItemPending,
			SourceArtifactIDs: seed.SourceArtifactIDs,
			Quantity:          seed.Quantity,
			Unit:              seed.Unit,
			NormalizedName:    seed.NormalizedName,
			SortOrder:         seed.SortOrder,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
	}

	if err := b.Store.CreateList(ctx, l, items); err != nil {
		return nil, fmt.Errorf("persist list: %w", err)
	}

	return &ShoppingResult{
		ListID:         listID,
		Title:          listTitle,
		ItemCount:      len(items),
		ScalingSummary: scalingSummary,
		Skipped:        skipped,
	}, nil
}

// scaleRecipeDomainData parses domain_data, scales ingredients to targetServings,
// and returns modified domain_data JSON with scaled quantities.
func scaleRecipeDomainData(domainData json.RawMessage, targetServings int) (json.RawMessage, error) {
	var rd recipe.RecipeData
	if err := json.Unmarshal(domainData, &rd); err != nil {
		return nil, fmt.Errorf("unmarshal recipe data: %w", err)
	}

	originalServings := 1
	if rd.Servings != nil && *rd.Servings > 0 {
		originalServings = *rd.Servings
	}

	if originalServings == targetServings {
		return domainData, nil // No scaling needed
	}

	scaled := recipe.ScaleIngredients(rd.Ingredients, originalServings, targetServings)
	if scaled == nil {
		return domainData, nil // Scaling returned nil (invalid params)
	}

	// Replace ingredients with scaled values
	scaledIngredients := make([]recipe.Ingredient, len(scaled))
	for i, si := range scaled {
		scaledIngredients[i] = recipe.Ingredient{
			Name:        si.Name,
			Quantity:    si.DisplayQuantity,
			Unit:        si.Unit,
			Preparation: si.Preparation,
		}
	}

	rd.Ingredients = scaledIngredients
	rd.Servings = &targetServings

	result, err := json.Marshal(rd)
	if err != nil {
		return nil, fmt.Errorf("marshal scaled data: %w", err)
	}
	return result, nil
}

// resolveTitle extracts the recipe title from domain_data.
func resolveTitle(domainData json.RawMessage) string {
	var rd struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(domainData, &rd); err != nil || rd.Title == "" {
		return "(unknown)"
	}
	return rd.Title
}

// findExistingList checks if a list with the given source query already exists.
func (b *ShoppingBridge) findExistingList(ctx context.Context, sourceQuery string) (*list.List, error) {
	lists, err := b.Store.ListLists(ctx, "", "", 100, 0)
	if err != nil {
		return nil, err
	}
	for _, l := range lists {
		if l.SourceQuery == sourceQuery && l.Status != list.StatusArchived {
			return &l, nil
		}
	}
	return nil, nil
}
