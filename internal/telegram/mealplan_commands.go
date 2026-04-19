package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/mealplan"
)

// MealPlanCommandHandler handles Telegram commands for meal planning.
type MealPlanCommandHandler struct {
	Service  *mealplan.Service
	Shopping *mealplan.ShoppingBridge

	// CookDelegate is called to enter cook mode from plan resolution.
	// Set by the wiring layer to bridge into Bot.handleCookEntry.
	CookDelegate func(chatID int64, recipeName string, servings int)

	// RecipeResolver searches for a recipe artifact by name and returns
	// (artifactID, recipeTitle, error). Set by the wiring layer to bridge
	// into Bot.resolveRecipeByName. When nil, recipeName is used directly
	// (unit test fallback only — live wiring must set this).
	RecipeResolver func(ctx context.Context, name string) (artifactID string, title string, err error)

	// Draft plan context per chat ID (in-process memory, not DB)
	mu     sync.RWMutex
	drafts map[int64]*draftContext
	done   chan struct{}
}

type draftContext struct {
	PlanID    string
	ExpiresAt time.Time
}

const draftTTL = 24 * time.Hour

// NewMealPlanCommandHandler creates a new handler.
func NewMealPlanCommandHandler(svc *mealplan.Service, shopping *mealplan.ShoppingBridge) *MealPlanCommandHandler {
	return &MealPlanCommandHandler{
		Service:  svc,
		Shopping: shopping,
		drafts:   make(map[int64]*draftContext),
		done:     make(chan struct{}),
	}
}

// StartCleanup begins a background goroutine that sweeps expired draft entries.
func (h *MealPlanCommandHandler) StartCleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-h.done:
				return
			case <-ticker.C:
				h.sweepDrafts()
			}
		}
	}()
}

// Stop signals the cleanup goroutine to exit.
func (h *MealPlanCommandHandler) Stop() {
	select {
	case <-h.done:
	default:
		close(h.done)
	}
}

// sweepDrafts removes expired draft entries from the map.
func (h *MealPlanCommandHandler) sweepDrafts() {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for chatID, d := range h.drafts {
		if now.After(d.ExpiresAt) {
			delete(h.drafts, chatID)
		}
	}
}

// Regex patterns for meal plan command matching.
var (
	mealPlanThisWeekRe = regexp.MustCompile(`(?i)^meal\s+plan\s+this\s+week$`)
	mealPlanNextWeekRe = regexp.MustCompile(`(?i)^meal\s+plan\s+next\s+week$`)
	activatePlanRe     = regexp.MustCompile(`(?i)^activate\s+plan$`)
	showPlanRe         = regexp.MustCompile(`(?i)^(meal\s+plan|show\s+plan|plan\s+this\s+week)$`)

	// "what's for dinner tomorrow?"
	whatsForMealRe = regexp.MustCompile(`(?i)^what(?:'?s| is) for (\w+?)(?:\s+(\w+))?\??$`)
	// "{day} meals" / "today's plan"
	dayMealsRe = regexp.MustCompile(`(?i)^(\w+?)(?:'?s)?\s+(?:meals|plan)$`)
	// "dinners this week" / "lunches this week"
	weeklyMealRe = regexp.MustCompile(`(?i)^(\w+)\s+this\s+week\??$`)

	// Slot assignment: "Monday dinner: Pasta Carbonara for 4"
	slotAssignRe = regexp.MustCompile(`(?i)^(\w+)\s+(breakfast|lunch|dinner|snack):?\s+(.+?)(?:\s+for\s+(\d+)(?:\s+servings?)?)?$`)
	// Batch: "Mon-Thu breakfast: Overnight Oats for 2"
	batchSlotRe = regexp.MustCompile(`(?i)^(\w+)-(\w+)\s+(breakfast|lunch|dinner|snack):?\s+(.+?)(?:\s+for\s+(\d+)(?:\s+servings?)?)?$`)

	// "shopping list for plan"
	shoppingForPlanRe = regexp.MustCompile(`(?i)^shopping\s+list\s+for\s+(?:this\s+week(?:'s)?\s+)?plan$`)

	// "cook tonight's dinner"
	cookTonightRe = regexp.MustCompile(`(?i)^cook\s+tonight(?:'?s)?\s+(breakfast|lunch|dinner|snack)$`)
	// "cook {day}'s {meal}"
	cookDayMealRe = regexp.MustCompile(`(?i)^cook\s+(\w+?)(?:'?s)?\s+(breakfast|lunch|dinner|snack)$`)

	// "repeat last week's plan" / "repeat last week"
	repeatLastWeekRe = regexp.MustCompile(`(?i)^repeat\s+last\s+week(?:'?s)?(?:\s+plan)?$`)

	// "archive plan" / "delete plan"
	archivePlanRe = regexp.MustCompile(`(?i)^archive\s+plan$`)
	deletePlanRe  = regexp.MustCompile(`(?i)^delete\s+plan$`)
)

// TryHandle attempts to match and handle a meal plan command.
// Returns true if the message was handled, false if it didn't match.
func (h *MealPlanCommandHandler) TryHandle(ctx context.Context, chatID int64, text string, replyFunc func(int64, string)) bool {
	text = strings.TrimSpace(text)

	// Plan creation
	if mealPlanThisWeekRe.MatchString(text) {
		start, end := thisWeekRange()
		h.handlePlanCreate(ctx, chatID, start, end, replyFunc)
		return true
	}
	if mealPlanNextWeekRe.MatchString(text) {
		start, end := nextWeekRange()
		h.handlePlanCreate(ctx, chatID, start, end, replyFunc)
		return true
	}

	// Plan activation
	if activatePlanRe.MatchString(text) {
		h.handlePlanActivate(ctx, chatID, replyFunc)
		return true
	}

	// Weekly overview
	if showPlanRe.MatchString(text) {
		h.handlePlanView(ctx, chatID, replyFunc)
		return true
	}

	// "what's for dinner tomorrow?"
	if m := whatsForMealRe.FindStringSubmatch(text); len(m) >= 2 {
		meal := strings.ToLower(m[1])
		dayStr := ""
		if len(m) >= 3 && m[2] != "" {
			dayStr = m[2]
		}
		h.handleDailyQuery(ctx, chatID, meal, dayStr, replyFunc)
		return true
	}

	// "{day} meals"
	if m := dayMealsRe.FindStringSubmatch(text); len(m) >= 2 {
		dayStr := m[1]
		h.handleDailyQuery(ctx, chatID, "", dayStr, replyFunc)
		return true
	}

	// "dinners this week" / "lunches this week"
	if m := weeklyMealRe.FindStringSubmatch(text); len(m) >= 2 {
		meal := strings.ToLower(m[1])
		if trimmed := strings.TrimSuffix(meal, "es"); trimmed != meal {
			meal = trimmed
		} else {
			meal = strings.TrimSuffix(meal, "s")
		}
		h.handleWeeklyMealQuery(ctx, chatID, meal, replyFunc)
		return true
	}

	// "shopping list for plan"
	if shoppingForPlanRe.MatchString(text) {
		h.handlePlanShoppingList(ctx, chatID, replyFunc)
		return true
	}

	// "cook tonight's dinner" / "cook {day}'s {meal}"
	if m := cookTonightRe.FindStringSubmatch(text); len(m) >= 2 {
		meal := strings.ToLower(m[1])
		h.handleCookFromPlan(ctx, chatID, "today", meal, replyFunc)
		return true
	}
	if m := cookDayMealRe.FindStringSubmatch(text); len(m) >= 3 {
		dayStr := m[1]
		meal := strings.ToLower(m[2])
		h.handleCookFromPlan(ctx, chatID, dayStr, meal, replyFunc)
		return true
	}

	// "repeat last week's plan"
	if repeatLastWeekRe.MatchString(text) {
		h.handlePlanRepeat(ctx, chatID, replyFunc)
		return true
	}

	// Batch slot assignment
	if m := batchSlotRe.FindStringSubmatch(text); len(m) >= 5 {
		startDay := m[1]
		endDay := m[2]
		meal := strings.ToLower(m[3])
		recipeName := strings.TrimSpace(m[4])
		servings := 0
		if len(m) >= 6 && m[5] != "" {
			servings, _ = strconv.Atoi(m[5])
		}
		h.handleBatchSlotAssign(ctx, chatID, startDay, endDay, meal, recipeName, servings, replyFunc)
		return true
	}

	// Single slot assignment
	if m := slotAssignRe.FindStringSubmatch(text); len(m) >= 4 {
		dayStr := m[1]
		meal := strings.ToLower(m[2])
		recipeName := strings.TrimSpace(m[3])
		servings := 0
		if len(m) >= 5 && m[4] != "" {
			servings, _ = strconv.Atoi(m[4])
		}
		h.handleSlotAssign(ctx, chatID, dayStr, meal, recipeName, servings, replyFunc)
		return true
	}

	// Lifecycle commands
	if archivePlanRe.MatchString(text) {
		h.handlePlanLifecycle(ctx, chatID, mealplan.StatusArchived, replyFunc)
		return true
	}
	if deletePlanRe.MatchString(text) {
		h.handleDeletePlan(ctx, chatID, replyFunc)
		return true
	}

	return false
}

func (h *MealPlanCommandHandler) handlePlanCreate(ctx context.Context, chatID int64, start, end time.Time, replyFunc func(int64, string)) {
	title := fmt.Sprintf("Week of %s", start.Format("Jan 2"))
	result, err := h.Service.CreatePlan(ctx, title, start, end)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? Failed to create plan: %s", err))
		return
	}

	h.setDraft(chatID, result.Plan.ID)
	replyFunc(chatID, fmt.Sprintf(". Created plan: %s (%s - %s)\n  Status: draft\n\n  Add meals with: \"Monday dinner Pasta Carbonara for 4\"\n  Activate when ready: \"activate plan\"",
		result.Plan.Title,
		result.Plan.StartDate.Format("Jan 2"),
		result.Plan.EndDate.Format("Jan 2"),
	))
}

func (h *MealPlanCommandHandler) handleSlotAssign(ctx context.Context, chatID int64, dayStr, meal, recipeName string, servings int, replyFunc func(int64, string)) {
	draftID := h.getDraft(chatID)
	if draftID == "" {
		replyFunc(chatID, "? No draft plan. Create one first: \"meal plan this week\"")
		return
	}

	date := resolveDayName(dayStr)
	if date.IsZero() {
		replyFunc(chatID, fmt.Sprintf("? Could not parse day: %q", dayStr))
		return
	}

	// Resolve recipe name to artifact ID via search.
	artifactID, resolvedTitle, err := h.resolveRecipe(ctx, recipeName)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? No recipe found for %q. Try a different name or /find to search.", recipeName))
		return
	}

	slot, err := h.Service.AddSlot(ctx, draftID, date, meal, artifactID, servings, false, "")
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}

	displayName := resolvedTitle
	if displayName == "" {
		displayName = recipeName
	}

	plan, _ := h.Service.GetPlan(ctx, draftID)
	slotCount := 0
	if plan != nil {
		slotCount = len(plan.Slots)
	}

	replyFunc(chatID, fmt.Sprintf(". %s %s: %s (%d servings)\n  %d slots filled.",
		date.Format("Monday"), meal, displayName, slot.Servings, slotCount))
}

func (h *MealPlanCommandHandler) handleBatchSlotAssign(ctx context.Context, chatID int64, startDay, endDay, meal, recipeName string, servings int, replyFunc func(int64, string)) {
	draftID := h.getDraft(chatID)
	if draftID == "" {
		replyFunc(chatID, "? No draft plan. Create one first: \"meal plan this week\"")
		return
	}

	startDate := resolveDayName(startDay)
	endDate := resolveDayName(endDay)
	if startDate.IsZero() || endDate.IsZero() {
		replyFunc(chatID, fmt.Sprintf("? Could not parse day range: %s-%s", startDay, endDay))
		return
	}

	if endDate.Before(startDate) {
		endDate = endDate.AddDate(0, 0, 7) // Handle wrap-around
	}

	// Resolve recipe name to artifact ID via search.
	artifactID, resolvedTitle, err := h.resolveRecipe(ctx, recipeName)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? No recipe found for %q. Try a different name or /find to search.", recipeName))
		return
	}

	slots, err := h.Service.AddBatchSlots(ctx, draftID, startDate, endDate, meal, artifactID, servings)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}

	displayName := resolvedTitle
	if displayName == "" {
		displayName = recipeName
	}

	replyFunc(chatID, fmt.Sprintf(". %s-%s %s: %s (%d servings each)\n  %d slots added.",
		startDay, endDay, meal, displayName, slots[0].Servings, len(slots)))
}

func (h *MealPlanCommandHandler) handlePlanActivate(ctx context.Context, chatID int64, replyFunc func(int64, string)) {
	draftID := h.getDraft(chatID)
	if draftID == "" {
		replyFunc(chatID, "? No draft plan to activate.")
		return
	}

	overlap, err := h.Service.ActivatePlan(ctx, draftID, false)
	if err != nil {
		if svcErr, ok := err.(*mealplan.ServiceError); ok && svcErr.Code == "MEAL_PLAN_OVERLAP" && overlap != nil {
			replyFunc(chatID, fmt.Sprintf("? %d days overlap with active plan %q.\n  - merge: combine both plans' meals\n  - replace: deactivate the old plan\n  - keep both: run plans in parallel\n\n  Reply: merge · replace · keep both",
				overlap.OverlappingDays, overlap.ConflictingPlanTitle))
			return
		}
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}

	plan, _ := h.Service.GetPlan(ctx, draftID)
	title := "plan"
	if plan != nil {
		title = plan.Plan.Title
	}
	h.clearDraft(chatID)
	replyFunc(chatID, fmt.Sprintf(". Plan %q is now active.", title))
}

func (h *MealPlanCommandHandler) handlePlanView(ctx context.Context, chatID int64, replyFunc func(int64, string)) {
	// Try draft first, then active
	draftID := h.getDraft(chatID)
	if draftID != "" {
		plan, err := h.Service.GetPlan(ctx, draftID)
		if err == nil && plan != nil {
			replyFunc(chatID, formatPlanView(plan))
			return
		}
	}

	// Find active plan for current week
	now := time.Now()
	slots, plan, err := h.Service.QueryByDate(ctx, now, "")
	if err != nil || plan == nil {
		replyFunc(chatID, ". No active meal plan. Create one with \"meal plan this week\".")
		return
	}

	// Get full plan
	fullPlan, err := h.Service.GetPlan(ctx, plan.ID)
	if err != nil || fullPlan == nil {
		replyFunc(chatID, fmt.Sprintf(". Active plan found but failed to load: %v", err))
		return
	}
	_ = slots // We use the full plan view
	replyFunc(chatID, formatPlanView(fullPlan))
}

func (h *MealPlanCommandHandler) handleDailyQuery(ctx context.Context, chatID int64, meal, dayStr string, replyFunc func(int64, string)) {
	date := resolveDayName(dayStr)
	if date.IsZero() {
		date = time.Now().Truncate(24 * time.Hour)
	}

	if meal == "" {
		meal = "" // Query all meals for the day
	}

	slots, plan, err := h.Service.QueryByDate(ctx, date, meal)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? Query failed: %s", err))
		return
	}
	if plan == nil {
		replyFunc(chatID, ". No active meal plan. Create one with \"meal plan this week\".")
		return
	}

	if len(slots) == 0 {
		dayName := date.Format("Monday")
		if meal != "" {
			replyFunc(chatID, fmt.Sprintf(". No %s planned for %s.", meal, dayName))
		} else {
			replyFunc(chatID, fmt.Sprintf(". No meals planned for %s.", dayName))
		}
		return
	}

	if len(slots) == 1 {
		sl := slots[0]
		replyFunc(chatID, fmt.Sprintf("%s %s: %s (%d servings)",
			date.Format("Monday"), sl.MealType, sl.RecipeTitle, sl.Servings))
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s:", date.Format("Monday")))
	for _, sl := range slots {
		lines = append(lines, fmt.Sprintf("  %-9s %s (%d)", sl.MealType, sl.RecipeTitle, sl.Servings))
	}
	replyFunc(chatID, strings.Join(lines, "\n"))
}

func (h *MealPlanCommandHandler) handleWeeklyMealQuery(ctx context.Context, chatID int64, meal string, replyFunc func(int64, string)) {
	start, end := thisWeekRange()
	var lines []string
	lines = append(lines, fmt.Sprintf("%ss this week:", capitalize(meal)))

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		slots, _, err := h.Service.QueryByDate(ctx, d, meal)
		if err != nil {
			continue
		}
		dayName := d.Format("Mon")
		if len(slots) == 0 {
			lines = append(lines, fmt.Sprintf("  %s  —", dayName))
		} else {
			sl := slots[0]
			lines = append(lines, fmt.Sprintf("  %s  %s (%d)", dayName, sl.RecipeTitle, sl.Servings))
		}
	}
	replyFunc(chatID, strings.Join(lines, "\n"))
}

func (h *MealPlanCommandHandler) handlePlanShoppingList(ctx context.Context, chatID int64, replyFunc func(int64, string)) {
	if h.Shopping == nil {
		replyFunc(chatID, "? Shopping list generation not configured.")
		return
	}

	// Find active plan
	now := time.Now()
	_, plan, err := h.Service.QueryByDate(ctx, now, "")
	if err != nil || plan == nil {
		replyFunc(chatID, ". No active meal plan to generate a shopping list from.")
		return
	}

	fullPlan, err := h.Service.GetPlan(ctx, plan.ID)
	if err != nil || fullPlan == nil {
		replyFunc(chatID, "? Failed to load plan.")
		return
	}

	if len(fullPlan.Slots) == 0 {
		replyFunc(chatID, ". Plan is empty. Assign some recipes first: \"Monday dinner Pasta Carbonara for 4\"")
		return
	}

	result, err := h.Shopping.GenerateFromPlan(ctx, *fullPlan, false)
	if err != nil {
		if svcErr, ok := err.(*mealplan.ServiceError); ok && svcErr.Code == "MEAL_PLAN_LIST_EXISTS" {
			replyFunc(chatID, "? A shopping list already exists for this plan.\n  Reply: regenerate · keep")
			return
		}
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf(". Shopping list generated from %q (%d meals).", fullPlan.Plan.Title, len(fullPlan.Slots)))
	lines = append(lines, "")
	lines = append(lines, "  Scaled across recipes:")
	for _, s := range result.ScalingSummary {
		if s.Occurrences > 1 {
			lines = append(lines, fmt.Sprintf("  - %s: %d servings x %d days = %d servings",
				s.RecipeTitle, s.Servings, s.Occurrences, s.TotalServings))
		} else {
			lines = append(lines, fmt.Sprintf("  - %s: %d servings", s.RecipeTitle, s.TotalServings))
		}
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  List: %q", result.Title))
	lines = append(lines, fmt.Sprintf("  Items: %d", result.ItemCount))

	replyFunc(chatID, strings.Join(lines, "\n"))
}

func (h *MealPlanCommandHandler) handleCookFromPlan(ctx context.Context, chatID int64, dayStr, meal string, replyFunc func(int64, string)) {
	date := resolveDayName(dayStr)
	if date.IsZero() {
		date = time.Now().Truncate(24 * time.Hour)
	}

	slots, plan, err := h.Service.QueryByDate(ctx, date, meal)
	if err != nil || plan == nil {
		replyFunc(chatID, ". No active meal plan.")
		return
	}

	if len(slots) == 0 {
		replyFunc(chatID, fmt.Sprintf(". No %s planned for %s.", meal, date.Format("Monday")))
		return
	}

	sl := slots[0]
	// Resolve recipe from plan and delegate to cook mode (BS-010).
	replyFunc(chatID, fmt.Sprintf(". %s's %s: %s (%d servings)\n  Starting cook mode...",
		date.Format("Monday"), meal, sl.RecipeTitle, sl.Servings))

	if h.CookDelegate != nil {
		h.CookDelegate(chatID, sl.RecipeTitle, sl.Servings)
	}
}

func (h *MealPlanCommandHandler) handlePlanRepeat(ctx context.Context, chatID int64, replyFunc func(int64, string)) {
	// Find the most recently completed or archived plan
	plans, err := h.Service.ListPlans(ctx, "completed", nil, nil)
	if err != nil || len(plans) == 0 {
		plans, err = h.Service.ListPlans(ctx, "archived", nil, nil)
		if err != nil || len(plans) == 0 {
			replyFunc(chatID, "? No completed plans to repeat.")
			return
		}
	}

	sourcePlan := plans[0]
	newStart, _ := thisWeekRange()
	newTitle := fmt.Sprintf("Week of %s", newStart.Format("Jan 2"))

	result, err := h.Service.CopyPlan(ctx, sourcePlan.ID, newStart, newTitle, nil)
	if err != nil {
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}

	msg := fmt.Sprintf(". Created plan %q from %q.\n  %d meals copied.", newTitle, sourcePlan.Title, result.SlotsCopied)
	if len(result.SlotsSkipped) > 0 {
		msg += fmt.Sprintf("\n  %d slots skipped (recipes unavailable).", len(result.SlotsSkipped))
	}
	msg += "\n  Status: draft. Activate with: \"activate plan\""

	h.setDraft(chatID, result.Plan.Plan.ID)
	replyFunc(chatID, msg)
}

func (h *MealPlanCommandHandler) handlePlanLifecycle(ctx context.Context, chatID int64, status mealplan.PlanStatus, replyFunc func(int64, string)) {
	// Find active plan
	now := time.Now()
	_, plan, err := h.Service.QueryByDate(ctx, now, "")
	if err != nil || plan == nil {
		replyFunc(chatID, ". No active plan to modify.")
		return
	}

	if err := h.Service.TransitionPlan(ctx, plan.ID, status); err != nil {
		replyFunc(chatID, fmt.Sprintf("? %s", err))
		return
	}
	replyFunc(chatID, fmt.Sprintf(". Plan %q is now %s.", plan.Title, status))
}

func (h *MealPlanCommandHandler) handleDeletePlan(ctx context.Context, chatID int64, replyFunc func(int64, string)) {
	draftID := h.getDraft(chatID)
	if draftID != "" {
		if err := h.Service.DeletePlan(ctx, draftID); err == nil {
			h.clearDraft(chatID)
			replyFunc(chatID, ". Draft plan deleted.")
			return
		}
	}
	replyFunc(chatID, "? No draft plan to delete.")
}

// --- Draft context helpers ---

func (h *MealPlanCommandHandler) setDraft(chatID int64, planID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drafts[chatID] = &draftContext{PlanID: planID, ExpiresAt: time.Now().Add(draftTTL)}
}

func (h *MealPlanCommandHandler) getDraft(chatID int64) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d, ok := h.drafts[chatID]
	if !ok || time.Now().After(d.ExpiresAt) {
		return ""
	}
	return d.PlanID
}

func (h *MealPlanCommandHandler) clearDraft(chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.drafts, chatID)
}

// --- Date helpers ---

func thisWeekRange() (time.Time, time.Time) {
	now := time.Now().Truncate(24 * time.Hour)
	weekday := now.Weekday()
	daysToMonday := int(weekday) - int(time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	monday := now.AddDate(0, 0, -daysToMonday)
	sunday := monday.AddDate(0, 0, 6)
	return monday, sunday
}

func nextWeekRange() (time.Time, time.Time) {
	monday, _ := thisWeekRange()
	nextMonday := monday.AddDate(0, 0, 7)
	nextSunday := nextMonday.AddDate(0, 0, 6)
	return nextMonday, nextSunday
}

var dayNames = map[string]time.Weekday{
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
	"sunday": time.Sunday, "sun": time.Sunday,
}

// resolveRecipe resolves a user-typed recipe name to an artifact ID.
// Uses the RecipeResolver callback when set (live wiring), falls back to
// treating the name as a literal artifact ID (unit test compatibility).
func (h *MealPlanCommandHandler) resolveRecipe(ctx context.Context, name string) (artifactID string, title string, err error) {
	if h.RecipeResolver != nil {
		return h.RecipeResolver(ctx, name)
	}
	// Fallback: treat name as artifact ID directly (unit tests only).
	return name, name, nil
}

func resolveDayName(s string) time.Time {
	s = strings.ToLower(strings.TrimSpace(s))

	if s == "today" || s == "tonight" {
		return time.Now().Truncate(24 * time.Hour)
	}
	if s == "tomorrow" {
		return time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	}

	targetDay, ok := dayNames[s]
	if !ok {
		return time.Time{}
	}

	now := time.Now().Truncate(24 * time.Hour)
	currentDay := now.Weekday()
	daysToMonday := int(currentDay) - int(time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	monday := now.AddDate(0, 0, -daysToMonday)
	offset := int(targetDay) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return monday.AddDate(0, 0, offset)
}

// formatPlanView formats a full plan for Telegram display.
func formatPlanView(plan *mealplan.PlanWithSlots) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("# %s", plan.Plan.Title))
	lines = append(lines, fmt.Sprintf("> %s - %s · %s",
		plan.Plan.StartDate.Format("Jan 2"),
		plan.Plan.EndDate.Format("Jan 2"),
		plan.Plan.Status))
	lines = append(lines, "")

	// Group slots by day
	mealAbbr := map[string]string{
		"breakfast": "bfast",
		"lunch":     "lunch",
		"dinner":    "dinner",
		"snack":     "snack",
	}

	slotsByDay := make(map[string][]mealplan.Slot)
	for _, sl := range plan.Slots {
		key := sl.SlotDate.Format("2006-01-02")
		slotsByDay[key] = append(slotsByDay[key], sl)
	}

	start := plan.Plan.StartDate.Truncate(24 * time.Hour)
	end := plan.Plan.EndDate.Truncate(24 * time.Hour)
	totalMeals := 0

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		daySlots, ok := slotsByDay[key]
		if !ok || len(daySlots) == 0 {
			continue
		}
		for _, sl := range daySlots {
			abbr := mealAbbr[sl.MealType]
			if abbr == "" {
				abbr = sl.MealType
			}
			lines = append(lines, fmt.Sprintf("%-4s %-7s %s (%d)",
				d.Format("Mon"), abbr, sl.RecipeTitle, sl.Servings))
			totalMeals++
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("%d meals planned.", totalMeals))

	return strings.Join(lines, "\n")
}

// capitalize returns the string with the first letter uppercased.
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
