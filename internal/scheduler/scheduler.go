package scheduler

import (
	"context"
	"log/slog"
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
	digestPendingRetry bool   // true when last digest was generated but delivery failed
	digestPendingDate  string // date of the pending digest for retry
}

// New creates a new scheduler.
func New(digestGen *digest.Generator, bot *telegram.Bot, engine *intelligence.Engine, lifecycle *topics.Lifecycle) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		digestGen: digestGen,
		bot:       bot,
		engine:    engine,
		lifecycle: lifecycle,
	}
}

// Start begins running scheduled tasks.
func (s *Scheduler) Start(_ context.Context, cronExpr string) error {
	_, err := s.cron.AddFunc(cronExpr, func() {
		slog.Info("digest cron triggered")
		if s.digestGen == nil {
			slog.Warn("digest generator not configured")
			return
		}
		// Create a fresh context per cron invocation with a timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Retry delivery of a previously generated but undelivered digest
		if s.digestPendingRetry && s.bot != nil && s.digestPendingDate != "" {
			d, err := s.digestGen.GetLatest(ctx, s.digestPendingDate)
			if err == nil && d != nil && d.DigestDate.Format("2006-01-02") == s.digestPendingDate {
				s.bot.SendDigest(d.DigestText)
				slog.Info("pending digest delivered via retry", "date", s.digestPendingDate)
				s.digestPendingRetry = false
				s.digestPendingDate = ""
			} else {
				slog.Warn("pending digest retry failed, will try again next cycle", "date", s.digestPendingDate)
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
			// Poll for the digest result (ML sidecar processes asynchronously)
			today := digestCtx.DigestDate
			var delivered bool
			for attempt := 0; attempt < 10; attempt++ {
				d, err := s.digestGen.GetLatest(ctx, today)
				if err == nil && d != nil && d.DigestDate.Format("2006-01-02") == today {
					s.bot.SendDigest(d.DigestText)
					slog.Info("digest delivered via Telegram", "attempt", attempt+1)
					delivered = true
					break
				}
				time.Sleep(3 * time.Second)
			}
			if !delivered {
				slog.Warn("digest delivery failed: result not available after polling — will retry next cycle")
				s.digestPendingRetry = true
				s.digestPendingDate = today
			}
		}
	})
	if err != nil {
		return err
	}

	// Schedule topic momentum updates — hourly
	if s.lifecycle != nil {
		if _, err := s.cron.AddFunc("0 * * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
		if _, err := s.cron.AddFunc("0 8 * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
				slog.Info("resurfaced artifacts delivered", "count", len(candidates))
			}
		}); err != nil {
			slog.Warn("failed to schedule resurfacing", "error", err)
		}
	}

	s.cron.Start()
	slog.Info("scheduler started", "digest_cron", cronExpr)
	return nil
}

// Stop halts all scheduled tasks.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
