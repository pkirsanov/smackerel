package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/telegram"
)

// Scheduler manages cron-triggered tasks.
type Scheduler struct {
	cron      *cron.Cron
	digestGen *digest.Generator
	bot       *telegram.Bot
}

// New creates a new scheduler.
func New(digestGen *digest.Generator, bot *telegram.Bot) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		digestGen: digestGen,
		bot:       bot,
	}
}

// Start begins running scheduled tasks.
func (s *Scheduler) Start(_ context.Context, cronExpr string) error {
	_, err := s.cron.AddFunc(cronExpr, func() {
		slog.Info("digest cron triggered")
		// Create a fresh context per cron invocation with a timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
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
			// Get the stored digest text
			d, err := s.digestGen.GetLatest(ctx, digestCtx.DigestDate)
			if err == nil && d != nil {
				s.bot.SendDigest(d.DigestText)
				slog.Info("digest delivered via Telegram")
			}
		}
	})
	if err != nil {
		return err
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
