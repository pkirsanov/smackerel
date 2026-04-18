package mealplan

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CalDAVClient is the interface for creating/deleting calendar events.
// This abstracts the actual CalDAV connector for testability.
type CalDAVClient interface {
	PutEvent(ctx context.Context, uid, summary, description string, start, end time.Time, categories []string, extraProps map[string]string) error
	DeleteEvent(ctx context.Context, uid string) error
}

// CalendarBridge manages CalDAV event lifecycle for meal plan slots.
type CalendarBridge struct {
	Client    CalDAVClient
	MealTimes map[string]string // meal_type → "HH:MM"
}

// NewCalendarBridge creates a new calendar bridge.
func NewCalendarBridge(client CalDAVClient, mealTimes map[string]string) *CalendarBridge {
	return &CalendarBridge{
		Client:    client,
		MealTimes: mealTimes,
	}
}

// SyncPlan creates or updates CalDAV events for all slots in a plan.
func (b *CalendarBridge) SyncPlan(ctx context.Context, plan PlanWithSlots) (*CalendarSyncResult, error) {
	result := &CalendarSyncResult{}

	for _, slot := range plan.Slots {
		uid := fmt.Sprintf("smackerel-meal-%s", slot.ID)
		summary := slot.RecipeTitle
		if summary == "" {
			summary = fmt.Sprintf("Meal: %s", slot.MealType)
		}

		start := b.slotStartTime(slot)
		end := start.Add(1 * time.Hour)

		description := fmt.Sprintf("Recipe: %s\nServings: %d\nMeal: %s",
			slot.RecipeTitle, slot.Servings, slot.MealType)
		if slot.Notes != "" {
			description += fmt.Sprintf("\nNotes: %s", slot.Notes)
		}

		extraProps := map[string]string{
			"X-SMACKEREL-PLAN-ID": plan.Plan.ID,
			"X-SMACKEREL-SLOT-ID": slot.ID,
		}

		if err := b.Client.PutEvent(ctx, uid, summary, description, start, end, []string{"smackerel-meal"}, extraProps); err != nil {
			slog.Warn("calendar sync: failed to create event",
				"slot_id", slot.ID, "uid", uid, "error", err)
			result.EventsFailed++
			continue
		}
		result.EventsCreated++
	}

	return result, nil
}

// DeletePlanEvents removes CalDAV events for all slots in a plan.
func (b *CalendarBridge) DeletePlanEvents(ctx context.Context, plan PlanWithSlots) {
	for _, slot := range plan.Slots {
		uid := fmt.Sprintf("smackerel-meal-%s", slot.ID)
		if err := b.Client.DeleteEvent(ctx, uid); err != nil {
			slog.Warn("calendar cleanup: failed to delete event",
				"slot_id", slot.ID, "uid", uid, "error", err)
		}
	}
}

// slotStartTime combines slot date with the configured meal time.
func (b *CalendarBridge) slotStartTime(slot Slot) time.Time {
	date := slot.SlotDate.Truncate(24 * time.Hour)

	timeStr, ok := b.MealTimes[strings.ToLower(slot.MealType)]
	if !ok {
		// Fallback: no configured time, use noon
		return date.Add(12 * time.Hour)
	}

	parts := strings.SplitN(timeStr, ":", 2)
	if len(parts) != 2 {
		return date.Add(12 * time.Hour)
	}

	var hour, min int
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return date.Add(12 * time.Hour)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &min); err != nil {
		return date.Add(12 * time.Hour)
	}

	return date.Add(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute)
}
