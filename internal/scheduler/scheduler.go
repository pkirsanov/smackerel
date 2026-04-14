package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/topics"
)

// Scheduler manages cron-triggered tasks.
type Scheduler struct {
	cron               *cron.Cron
	digestGen          *digest.Generator
	bot                *telegram.Bot
	engine             *intelligence.Engine
	lifecycle          *topics.Lifecycle
	mu                 sync.Mutex // protects digestPendingRetry and digestPendingDate
	digestPendingRetry bool       // true when last digest was generated but delivery failed
	digestPendingDate  string     // date of the pending digest for retry

	// baseCtx is cancelled by Stop() so in-flight cron callbacks that derive their
	// context from it are interrupted cleanly instead of racing with DB/NATS close.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	// done is closed by Stop() to cancel background goroutines spawned by cron jobs
	// (e.g., digest polling). Prevents goroutine leaks during graceful shutdown.
	done chan struct{}
	wg   sync.WaitGroup // tracks background goroutines for clean shutdown

	stopOnce sync.Once // guards Stop() against double-close panic on done channel

	// Per-group concurrency guards — prevents cron job overlap within each group
	muDigest    sync.Mutex
	muHourly    sync.Mutex
	muDaily     sync.Mutex
	muWeekly    sync.Mutex
	muMonthly   sync.Mutex
	muBriefs    sync.Mutex // pre-meeting briefs (every 5 min)
	muAlerts    sync.Mutex // alert delivery sweep (every 15 min)
	muAlertProd sync.Mutex // daily alert producers (bill, trip, return window)
	muResurface sync.Mutex // resurfacing (daily 8 AM — separate from synthesis/lookups)
	muLookups   sync.Mutex // frequent lookup detection (daily 4 AM)
	muSubs      sync.Mutex // subscription detection (weekly Monday 3 AM)
	muRelCool   sync.Mutex // relationship cooling alert production (weekly Monday 7 AM)
}

// New creates a new scheduler.
func New(digestGen *digest.Generator, bot *telegram.Bot, engine *intelligence.Engine, lifecycle *topics.Lifecycle) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:       cron.New(),
		digestGen:  digestGen,
		bot:        bot,
		engine:     engine,
		lifecycle:  lifecycle,
		baseCtx:    ctx,
		baseCancel: cancel,
		done:       make(chan struct{}),
	}
}

// Start begins running scheduled tasks.
func (s *Scheduler) Start(_ context.Context, cronExpr string) error {
	_, err := s.cron.AddFunc(cronExpr, func() {
		if !s.muDigest.TryLock() {
			slog.Warn("skipping overlapping job", "group", "digest", "job", "digest")
			return
		}
		defer s.muDigest.Unlock()

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
	})
	if err != nil {
		return err
	}

	// Schedule topic momentum updates — hourly
	if s.lifecycle != nil {
		if _, err := s.cron.AddFunc("0 * * * *", func() {
			if !s.muHourly.TryLock() {
				slog.Warn("skipping overlapping job", "group", "hourly", "job", "topic-momentum")
				return
			}
			defer s.muHourly.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
			defer cancel()
			if err := s.lifecycle.UpdateAllMomentum(ctx); err != nil {
				slog.Error("topic momentum update failed", "error", err)
			} else {
				slog.Info("topic momentum updated")
			}
		}); err != nil {
			slog.Warn("failed to schedule topic momentum", "error", err)
		}
	}

	// Schedule intelligence synthesis — daily at 2 AM
	if s.engine != nil {
		if _, err := s.cron.AddFunc("0 2 * * *", func() {
			if !s.muDaily.TryLock() {
				slog.Warn("skipping overlapping job", "group", "daily", "job", "synthesis")
				return
			}
			defer s.muDaily.Unlock()

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
		}); err != nil {
			slog.Warn("failed to schedule synthesis", "error", err)
		}

		// Schedule resurfacing — daily at 8 AM (after digest)
		// Uses dedicated muResurface to avoid contention with synthesis/lookups on muDaily.
		if _, err := s.cron.AddFunc("0 8 * * *", func() {
			if !s.muResurface.TryLock() {
				slog.Warn("skipping overlapping job", "group", "resurface", "job", "resurfacing")
				return
			}
			defer s.muResurface.Unlock()

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
		}); err != nil {
			slog.Warn("failed to schedule resurfacing", "error", err)
		}

		// Schedule pre-meeting briefs — every 5 minutes (R-306)
		if _, err := s.cron.AddFunc("*/5 * * * *", func() {
			if !s.muBriefs.TryLock() {
				slog.Warn("skipping overlapping job", "group", "briefs", "job", "pre-meeting-briefs")
				return
			}
			defer s.muBriefs.Unlock()

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
		}); err != nil {
			slog.Warn("failed to schedule pre-meeting briefs", "error", err)
		}

		// Schedule weekly synthesis — Sunday at 4 PM (R-307)
		if _, err := s.cron.AddFunc("0 16 * * 0", func() {
			if !s.muWeekly.TryLock() {
				slog.Warn("skipping overlapping job", "group", "weekly", "job", "weekly-synthesis")
				return
			}
			defer s.muWeekly.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
			defer cancel()

			ws, err := s.engine.GenerateWeeklySynthesis(ctx)
			if err != nil {
				slog.Error("weekly synthesis failed", "error", err)
			} else if s.bot != nil && ws.SynthesisText != "" {
				s.bot.SendDigest(ws.SynthesisText)
				slog.Info("weekly synthesis delivered", "words", ws.WordCount)
			}
		}); err != nil {
			slog.Warn("failed to schedule weekly synthesis", "error", err)
		}

		// Schedule monthly self-knowledge report — 1st of each month at 3 AM (R-506)
		if _, err := s.cron.AddFunc("0 3 1 * *", func() {
			if !s.muMonthly.TryLock() {
				slog.Warn("skipping overlapping job", "group", "monthly", "job", "monthly-report")
				return
			}
			defer s.muMonthly.Unlock()

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
		}); err != nil {
			slog.Warn("failed to schedule monthly report", "error", err)
		}

		// Schedule subscription detection — weekly on Mondays at 3 AM (R-504)
		// Uses dedicated muSubs to avoid contention with weekly synthesis/relationship
		// cooling on muWeekly. Same pattern as muResurface/muLookups from 013_phase5_stability.
		if _, err := s.cron.AddFunc("0 3 * * 1", func() {
			if !s.muSubs.TryLock() {
				slog.Warn("skipping overlapping job", "group", "subscriptions", "job", "subscription-detection")
				return
			}
			defer s.muSubs.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
			defer cancel()

			subs, err := s.engine.DetectSubscriptions(ctx)
			if err != nil {
				slog.Error("subscription detection failed", "error", err)
			} else {
				slog.Info("subscription detection complete", "detected", len(subs))
			}
		}); err != nil {
			slog.Warn("failed to schedule subscription detection", "error", err)
		}

		// Schedule frequent lookup detection — daily at 4 AM (R-507)
		// Uses dedicated muLookups to avoid contention with synthesis on muDaily.
		if _, err := s.cron.AddFunc("0 4 * * *", func() {
			if !s.muLookups.TryLock() {
				slog.Warn("skipping overlapping job", "group", "lookups", "job", "frequent-lookups")
				return
			}
			defer s.muLookups.Unlock()

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
		}); err != nil {
			slog.Warn("failed to schedule frequent lookup detection", "error", err)
		}

		// Schedule alert delivery sweep — every 15 minutes (R-021-001)
		if _, err := s.cron.AddFunc("*/15 * * * *", func() {
			if !s.muAlerts.TryLock() {
				slog.Warn("skipping overlapping job", "group", "alerts", "job", "alert-delivery")
				return
			}
			defer s.muAlerts.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 1*time.Minute)
			defer cancel()

			s.deliverPendingAlerts(ctx)
		}); err != nil {
			slog.Warn("failed to schedule alert delivery sweep", "error", err)
		}

		// Schedule daily alert production — 6 AM (R-021-002, R-021-003, R-021-004)
		// All three producers run sequentially in one job. Uses dedicated muAlertProd
		// to avoid muDaily contention with synthesis/lookups/resurfacing which could
		// starve alert production if a preceding daily job runs long.
		if _, err := s.cron.AddFunc("0 6 * * *", func() {
			if !s.muAlertProd.TryLock() {
				slog.Warn("skipping overlapping job", "group", "alert-prod", "job", "alert-producers")
				return
			}
			defer s.muAlertProd.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
			defer cancel()

			if err := s.engine.ProduceBillAlerts(ctx); err != nil {
				slog.Error("bill alert production failed", "error", err)
			}
			if err := s.engine.ProduceTripPrepAlerts(ctx); err != nil {
				slog.Error("trip prep alert production failed", "error", err)
			}
			if err := s.engine.ProduceReturnWindowAlerts(ctx); err != nil {
				slog.Error("return window alert production failed", "error", err)
			}
		}); err != nil {
			slog.Warn("failed to schedule daily alert production", "error", err)
		}

		// Schedule relationship cooling alert production — weekly Monday 7 AM (R-021-005)
		// Uses dedicated muRelCool to avoid contention with weekly synthesis on muWeekly,
		// matching the dedicated-mutex pattern used by muAlertProd, muResurface, muLookups, muSubs.
		if _, err := s.cron.AddFunc("0 7 * * 1", func() {
			if !s.muRelCool.TryLock() {
				slog.Warn("skipping overlapping job", "group", "rel-cool", "job", "relationship-cooling-alerts")
				return
			}
			defer s.muRelCool.Unlock()

			ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
			defer cancel()

			if err := s.engine.ProduceRelationshipCoolingAlerts(ctx); err != nil {
				slog.Error("relationship cooling alert production failed", "error", err)
			}
		}); err != nil {
			slog.Warn("failed to schedule relationship cooling alert production", "error", err)
		}
	}

	s.cron.Start()
	slog.Info("scheduler started", "digest_cron", cronExpr)
	return nil
}

// Stop halts all scheduled tasks and waits for background goroutines to finish.
// Safe to call multiple times — second and subsequent calls are no-ops.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		// Cancel the base context so in-flight cron callbacks abort promptly
		// instead of running to their full timeout while DB/NATS are closing.
		s.baseCancel()
		// Signal background goroutines (e.g., digest polling) to exit.
		close(s.done)
		// Stop the cron scheduler and wait for running callbacks to finish.
		cronCtx := s.cron.Stop()
		<-cronCtx.Done()
		// Wait for tracked background goroutines to drain, with a bounded timeout
		// so a stuck job doesn't block shutdown forever.
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			slog.Info("scheduler stopped cleanly")
		case <-time.After(30 * time.Second):
			slog.Warn("scheduler stop timed out waiting for in-flight jobs")
		}
	})
}

// DigestPendingRetry returns the current retry state (thread-safe).
func (s *Scheduler) DigestPendingRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digestPendingRetry
}

// DigestPendingDate returns the current pending date (thread-safe).
func (s *Scheduler) DigestPendingDate() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digestPendingDate
}

// SetDigestPending sets the retry state (thread-safe, used in tests).
func (s *Scheduler) SetDigestPending(retry bool, date string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.digestPendingRetry = retry
	s.digestPendingDate = date
}

// CronEntryCount returns the number of registered cron entries.
func (s *Scheduler) CronEntryCount() int {
	return len(s.cron.Entries())
}

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
			continue
		}

		// Use a detached context for marking delivered so that context cancellation
		// between send and mark doesn't leave sent-but-unmarked alerts (causing
		// duplicate delivery on the next sweep cycle).
		markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.engine.MarkAlertDelivered(markCtx, a.ID); err != nil {
			markCancel()
			slog.Warn("failed to mark alert delivered", "alert_id", a.ID, "error", err)
			continue
		}
		markCancel()

		slog.Info("alert delivered", "alert_id", a.ID, "type", a.AlertType, "title", a.Title)
	}
}
