package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// AlertTypeIcons maps alert types to emoji icons for Telegram formatting.
var AlertTypeIcons = map[string]string{
	"bill":                 "💰",
	"return_window":        "📦",
	"trip_prep":            "✈️",
	"relationship_cooling": "👋",
	"commitment_overdue":   "⏰",
	"meeting_brief":        "📋",
}

// FormatAlertMessage formats an alert for Telegram delivery with type icon.
func FormatAlertMessage(alertType string, title string, body string) string {
	icon := AlertTypeIcons[alertType]
	if icon == "" {
		icon = "🔔"
	}
	return fmt.Sprintf("%s %s\n%s", icon, title, body)
}

// deliverPendingAlerts fetches pending alerts and delivers them via Telegram.
// Extracted from the cron callback for testability.
func (s *Scheduler) deliverPendingAlerts(ctx context.Context) {
	if s.engine == nil {
		return
	}
	if s.bot == nil {
		slog.Debug("alert delivery skipped, no Telegram bot configured")
		return
	}

	alerts, err := s.engine.GetPendingAlerts(ctx)
	if err != nil {
		slog.Error("alert delivery sweep failed", "error", err)
		return
	}

	if len(alerts) == 0 {
		return
	}

	var delivered, failed int
	for i, a := range alerts {
		if ctx.Err() != nil {
			slog.Warn("alert delivery sweep context expired, remaining alerts deferred",
				"remaining", len(alerts)-i)
			break
		}

		msg := FormatAlertMessage(string(a.AlertType), a.Title, a.Body)

		if err := s.bot.SendAlertMessage(msg); err != nil {
			slog.Warn("alert delivery failed, will retry next sweep",
				"alert_id", a.ID, "error", err)
			failed++
			continue
		}

		// Use a detached context for marking delivered so that context cancellation
		// between send and mark doesn't leave sent-but-unmarked alerts (causing
		// duplicate delivery on the next sweep cycle).
		markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.engine.MarkAlertDelivered(markCtx, a.ID); err != nil {
			markCancel()
			slog.Warn("failed to mark alert delivered", "alert_id", a.ID, "error", err)
			failed++
			continue
		}
		markCancel()

		delivered++
		slog.Info("alert delivered", "alert_id", a.ID, "type", a.AlertType, "title", a.Title)
	}

	slog.Info("alert delivery sweep complete", "delivered", delivered, "failed", failed, "total", len(alerts))
}
