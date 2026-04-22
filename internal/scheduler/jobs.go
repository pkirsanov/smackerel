package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
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

// runDigestJob handles the digest cron callback.
func (s *Scheduler) runDigestJob() {
	s.runGuarded(&s.muDigest, "digest", "digest", s.doDigestJob)
}

func (s *Scheduler) doDigestJob() {
	slog.Info("digest cron triggered")
	if s.digestGen == nil {
		slog.Warn("digest generator not configured")
		return
	}
	// Derive from baseCtx so shutdown cancellation propagates to in-flight work
	ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
	defer cancel()

	// Retry delivery of a previously generated but undelivered digest
	s.mu.Lock()
	pendingRetry := s.digestPendingRetry
	pendingDate := s.digestPendingDate
	s.mu.Unlock()

	if pendingRetry && s.bot != nil && pendingDate != "" {
		d, err := s.digestGen.GetLatest(ctx, pendingDate)
		if err == nil && d != nil && d.DigestDate.Format("2006-01-02") == pendingDate {
			s.bot.SendDigest(d.DigestText)
			slog.Info("pending digest delivered via retry", "date", pendingDate)
			s.mu.Lock()
			s.digestPendingRetry = false
			s.digestPendingDate = ""
			s.mu.Unlock()
		} else {
			slog.Warn("pending digest retry failed, will try again next cycle", "date", pendingDate)
		}
	}

	digestCtx, err := s.digestGen.Generate(ctx)
	if err != nil {
		slog.Error("digest generation failed", "error", err)
		return
	}

	slog.Info("digest generated",
		"date", digestCtx.DigestDate,
		"action_items", len(digestCtx.ActionItems),
		"artifacts", len(digestCtx.OvernightArtifacts),
		"topics", len(digestCtx.HotTopics),
	)

	// Deliver via Telegram if bot is available
	if s.bot != nil {
		today := digestCtx.DigestDate
		// Fire a background goroutine to poll for the ML-processed digest result
		// so we don't block the cron callback.
		// Tracked via s.wg and cancellable via s.done to prevent goroutine leaks on shutdown.
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			pollCtx, pollCancel := context.WithTimeout(s.baseCtx, 60*time.Second)
			defer pollCancel()

			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-s.done:
					slog.Info("digest delivery cancelled by shutdown", "date", today)
					return
				case <-pollCtx.Done():
					slog.Warn("digest delivery timed out — will retry next cycle", "date", today)
					s.mu.Lock()
					s.digestPendingRetry = true
					s.digestPendingDate = today
					s.mu.Unlock()
					return
				case <-ticker.C:
					d, err := s.digestGen.GetLatest(pollCtx, today)
					if err == nil && d != nil && d.DigestDate.Format("2006-01-02") == today {
						s.bot.SendDigest(d.DigestText)
						slog.Info("digest delivered via Telegram", "date", today)
						return
					}
				}
			}
		}()
	}
}

// runTopicMomentumJob updates topic momentum scores — hourly.
func (s *Scheduler) runTopicMomentumJob() {
	s.runGuarded(&s.muHourly, "hourly", "topic-momentum", s.doTopicMomentumJob)
}

func (s *Scheduler) doTopicMomentumJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
	defer cancel()
	if err := s.lifecycle.UpdateAllMomentum(ctx); err != nil {
		slog.Error("topic momentum update failed", "error", err)
	} else {
		slog.Info("topic momentum updated")
	}
}

// runSynthesisJob runs intelligence synthesis and overdue commitments check — daily at 2 AM.
func (s *Scheduler) runSynthesisJob() {
	s.runGuarded(&s.muDaily, "daily", "synthesis", s.doSynthesisJob)
}

func (s *Scheduler) doSynthesisJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
	defer cancel()

	insights, err := s.engine.RunSynthesis(ctx)
	if err != nil {
		slog.Error("synthesis failed", "error", err)
	} else {
		slog.Info("synthesis complete", "insights", len(insights))
	}

	if err := s.engine.CheckOverdueCommitments(ctx); err != nil {
		slog.Error("overdue commitments check failed", "error", err)
	}
}

// runResurfacingJob resurfaces dormant artifacts — daily at 8 AM.
func (s *Scheduler) runResurfacingJob() {
	s.runGuarded(&s.muResurface, "resurface", "resurfacing", s.doResurfacingJob)
}

func (s *Scheduler) doResurfacingJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
	defer cancel()

	candidates, err := s.engine.Resurface(ctx, 5)
	if err != nil {
		slog.Error("resurfacing failed", "error", err)
	} else if len(candidates) > 0 && s.bot != nil {
		var msg string
		msg = "> Resurfaced for you:\n"
		for _, c := range candidates {
			msg += "- " + c.Title + " (" + c.Reason + ")\n"
		}
		s.bot.SendDigest(msg)

		// Mark delivered artifacts so dormancy scores update and the same
		// items are not resurfaced on subsequent runs.
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ArtifactID
		}
		if err := s.engine.MarkResurfaced(ctx, ids); err != nil {
			slog.Warn("failed to mark resurfaced artifacts", "error", err)
		}

		slog.Info("resurfaced artifacts delivered", "count", len(candidates))
	}
}

// runPreMeetingBriefsJob generates pre-meeting briefs — every 5 minutes (R-306).
func (s *Scheduler) runPreMeetingBriefsJob() {
	s.runGuarded(&s.muBriefs, "briefs", "pre-meeting-briefs", s.doPreMeetingBriefsJob)
}

func (s *Scheduler) doPreMeetingBriefsJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 1*time.Minute)
	defer cancel()

	briefs, err := s.engine.GeneratePreMeetingBriefs(ctx)
	if err != nil {
		slog.Error("pre-meeting brief generation failed", "error", err)
	} else if len(briefs) > 0 {
		for _, b := range briefs {
			if s.bot != nil {
				s.bot.SendDigest(b.BriefText)
			}
		}
		slog.Info("pre-meeting briefs delivered", "count", len(briefs))
	}
}

// runWeeklySynthesisJob generates weekly synthesis — Sunday at 4 PM (R-307).
func (s *Scheduler) runWeeklySynthesisJob() {
	s.runGuarded(&s.muWeekly, "weekly", "weekly-synthesis", s.doWeeklySynthesisJob)
}

func (s *Scheduler) doWeeklySynthesisJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
	defer cancel()

	ws, err := s.engine.GenerateWeeklySynthesis(ctx)
	if err != nil {
		slog.Error("weekly synthesis failed", "error", err)
	} else if s.bot != nil && ws.SynthesisText != "" {
		s.bot.SendDigest(ws.SynthesisText)
		slog.Info("weekly synthesis delivered", "words", ws.WordCount)
	}
}

// runMonthlyReportJob generates monthly self-knowledge report — 1st of each month at 3 AM (R-506).
func (s *Scheduler) runMonthlyReportJob() {
	s.runGuarded(&s.muMonthly, "monthly", "monthly-report", s.doMonthlyReportJob)
}

func (s *Scheduler) doMonthlyReportJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
	defer cancel()

	report, err := s.engine.GenerateMonthlyReport(ctx)
	if err != nil {
		slog.Error("monthly report generation failed", "error", err)
		return
	}
	slog.Info("monthly report generated", "month", report.Month, "words", report.WordCount)

	if s.bot != nil && report.ReportText != "" {
		s.bot.SendDigest(report.ReportText)
		slog.Info("monthly report delivered via Telegram", "month", report.Month)
	}
}

// runSubscriptionDetectionJob detects subscriptions — weekly on Mondays at 3 AM (R-504).
func (s *Scheduler) runSubscriptionDetectionJob() {
	s.runGuarded(&s.muSubs, "subscriptions", "subscription-detection", s.doSubscriptionDetectionJob)
}

func (s *Scheduler) doSubscriptionDetectionJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
	defer cancel()

	subs, err := s.engine.DetectSubscriptions(ctx)
	if err != nil {
		slog.Error("subscription detection failed", "error", err)
	} else {
		slog.Info("subscription detection complete", "detected", len(subs))
	}
}

// runFrequentLookupsJob detects frequent lookups and purges old search logs — daily at 4 AM (R-507).
func (s *Scheduler) runFrequentLookupsJob() {
	s.runGuarded(&s.muLookups, "lookups", "frequent-lookups", s.doFrequentLookupsJob)
}

func (s *Scheduler) doFrequentLookupsJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
	defer cancel()

	lookups, err := s.engine.DetectFrequentLookups(ctx)
	if err != nil {
		slog.Error("frequent lookup detection failed", "error", err)
	} else {
		slog.Info("frequent lookup detection complete", "detected", len(lookups))
		// Auto-create quick references for frequent lookups that don't have one yet (R-507)
		// Cap at 5 per run to avoid flooding the user with Telegram messages.
		const maxQuickRefsPerRun = 5
		created := 0
		for _, fl := range lookups {
			if fl.HasReference {
				continue
			}
			if created >= maxQuickRefsPerRun {
				slog.Info("quick reference creation cap reached, remaining deferred to next run", "cap", maxQuickRefsPerRun)
				break
			}
			content := fmt.Sprintf("Quick reference for: %s (looked up %d times in 30 days)", fl.SampleQuery, fl.LookupCount)
			qr, err := s.engine.CreateQuickReference(ctx, fl.SampleQuery, content, nil)
			if err != nil {
				slog.Warn("quick reference creation failed", "query", fl.SampleQuery, "error", err)
				continue
			}
			created++
			slog.Info("quick reference auto-created", "concept", qr.Concept, "lookups", fl.LookupCount)
			if s.bot != nil {
				msg := fmt.Sprintf("📌 You've looked up \"%s\" %d times. Created a pinned quick reference.", fl.SampleQuery, fl.LookupCount)
				s.bot.SendDigest(msg)
			}
		}
	}

	// Purge search_log entries older than 60 days (2× the 30-day detection window).
	// Runs after lookup detection so the purge does not affect the current run.
	purged, err := s.engine.PurgeOldSearchLogs(ctx, 60)
	if err != nil {
		slog.Warn("search log purge failed", "error", err)
	} else if purged > 0 {
		slog.Info("search log purged", "rows_deleted", purged)
	}
}

// runAlertDeliveryJob sweeps and delivers pending alerts — every 15 minutes (R-021-001).
func (s *Scheduler) runAlertDeliveryJob() {
	s.runGuarded(&s.muAlerts, "alerts", "alert-delivery", func() {
		ctx, cancel := context.WithTimeout(s.baseCtx, 1*time.Minute)
		defer cancel()
		s.deliverPendingAlerts(ctx)
	})
}

// runAlertProductionJob runs daily alert producers — 6 AM (R-021-002, R-021-003, R-021-004).
// All three producers run sequentially in one job.
func (s *Scheduler) runAlertProductionJob() {
	s.runGuarded(&s.muAlertProd, "alert-prod", "alert-producers", s.doAlertProductionJob)
}

func (s *Scheduler) doAlertProductionJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
	defer cancel()

	if err := s.engine.ProduceBillAlerts(ctx); err != nil {
		slog.Error("bill alert production failed", "error", err)
	}
	if ctx.Err() != nil {
		slog.Warn("daily alert production context expired after bill alerts, remaining producers skipped")
		return
	}
	if err := s.engine.ProduceTripPrepAlerts(ctx); err != nil {
		slog.Error("trip prep alert production failed", "error", err)
	}
	if ctx.Err() != nil {
		slog.Warn("daily alert production context expired after trip prep alerts, remaining producers skipped")
		return
	}
	if err := s.engine.ProduceReturnWindowAlerts(ctx); err != nil {
		slog.Error("return window alert production failed", "error", err)
	}
}

// runRelationshipCoolingJob produces relationship cooling alerts — weekly Monday 7 AM (R-021-005).
func (s *Scheduler) runRelationshipCoolingJob() {
	s.runGuarded(&s.muRelCool, "rel-cool", "relationship-cooling-alerts", func() {
		ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
		defer cancel()
		if err := s.engine.ProduceRelationshipCoolingAlerts(ctx); err != nil {
			slog.Error("relationship cooling alert production failed", "error", err)
		}
	})
}

// runKnowledgeLintJob runs the knowledge linter — configurable cron.
func (s *Scheduler) runKnowledgeLintJob() {
	s.runGuarded(&s.muKnowledgeLint, "knowledge-lint", "knowledge-lint", func() {
		ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
		defer cancel()
		if err := s.knowledgeLinter.RunLint(ctx); err != nil {
			slog.Error("knowledge lint failed", "error", err)
		}
	})
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
			metrics.AlertDeliveryFailures.Inc()
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
			metrics.AlertDeliveryFailures.Inc()
			failed++
			continue
		}
		markCancel()

		metrics.AlertsDelivered.WithLabelValues(string(a.AlertType)).Inc()
		delivered++
		slog.Info("alert delivered", "alert_id", a.ID, "type", a.AlertType, "title", a.Title)
	}

	slog.Info("alert delivery sweep complete", "delivered", delivered, "failed", failed, "total", len(alerts))
}

func (s *Scheduler) runMealPlanAutoCompleteJob() {
	s.runGuarded(&s.muMealPlanComplete, "meal-plan-auto-complete", "meal-plan-auto-complete", s.doMealPlanAutoCompleteJob)
}

func (s *Scheduler) doMealPlanAutoCompleteJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 60*time.Second)
	defer cancel()

	n, err := s.mealPlanSvc.AutoCompletePastPlans(ctx)
	if err != nil {
		slog.Error("meal plan auto-complete failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("meal plan auto-complete", "plans_completed", n)
	}
}
