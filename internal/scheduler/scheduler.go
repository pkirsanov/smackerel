package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/knowledge"
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
	digestPendingRetry bool
	digestPendingDate  string
	baseCtx            context.Context
	baseCancel         context.CancelFunc
	done               chan struct{}
	wg                 sync.WaitGroup
	stopOnce           sync.Once
	muDigest           sync.Mutex
	muHourly           sync.Mutex
	muDaily            sync.Mutex
	muWeekly           sync.Mutex
	muMonthly          sync.Mutex
	muBriefs           sync.Mutex
	muAlerts           sync.Mutex
	muAlertProd        sync.Mutex
	muResurface        sync.Mutex
	muLookups          sync.Mutex
	muSubs             sync.Mutex
	muRelCool          sync.Mutex
	muKnowledgeLint    sync.Mutex
	knowledgeLinter    *knowledge.Linter
	knowledgeLintCron  string
	muMealPlanComplete sync.Mutex
	mealPlanSvc        MealPlanAutoCompleter
	mealPlanCron       string
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
	_, err := s.cron.AddFunc(cronExpr, s.runDigestJob)
	if err != nil {
		return err
	}
	if s.lifecycle != nil {
		if _, err := s.cron.AddFunc("0 * * * *", s.runTopicMomentumJob); err != nil {
			slog.Warn("failed to schedule topic momentum", "error", err)
		}
	}
	if s.engine != nil {
		if _, err := s.cron.AddFunc("0 2 * * *", s.runSynthesisJob); err != nil {
			slog.Warn("failed to schedule synthesis", "error", err)
		}
		if _, err := s.cron.AddFunc("0 8 * * *", s.runResurfacingJob); err != nil {
			slog.Warn("failed to schedule resurfacing", "error", err)
		}
		if _, err := s.cron.AddFunc("*/5 * * * *", s.runPreMeetingBriefsJob); err != nil {
			slog.Warn("failed to schedule pre-meeting briefs", "error", err)
		}
		if _, err := s.cron.AddFunc("0 16 * * 0", s.runWeeklySynthesisJob); err != nil {
			slog.Warn("failed to schedule weekly synthesis", "error", err)
		}
		if _, err := s.cron.AddFunc("0 3 1 * *", s.runMonthlyReportJob); err != nil {
			slog.Warn("failed to schedule monthly report", "error", err)
		}
		if _, err := s.cron.AddFunc("0 3 * * 1", s.runSubscriptionDetectionJob); err != nil {
			slog.Warn("failed to schedule subscription detection", "error", err)
		}
		if _, err := s.cron.AddFunc("0 4 * * *", s.runFrequentLookupsJob); err != nil {
			slog.Warn("failed to schedule frequent lookup detection", "error", err)
		}
		if _, err := s.cron.AddFunc("*/15 * * * *", s.runAlertDeliveryJob); err != nil {
			slog.Warn("failed to schedule alert delivery sweep", "error", err)
		}
		if _, err := s.cron.AddFunc("0 6 * * *", s.runAlertProductionJob); err != nil {
			slog.Warn("failed to schedule daily alert production", "error", err)
		}
		if _, err := s.cron.AddFunc("0 7 * * 1", s.runRelationshipCoolingJob); err != nil {
			slog.Warn("failed to schedule relationship cooling alert production", "error", err)
		}
	}
	if s.knowledgeLinter != nil && s.knowledgeLintCron != "" {
		if _, err := s.cron.AddFunc(s.knowledgeLintCron, s.runKnowledgeLintJob); err != nil {
			slog.Warn("failed to schedule knowledge lint", "error", err)
		} else {
			slog.Info("knowledge lint scheduled", "cron", s.knowledgeLintCron)
		}
	}
	if s.mealPlanSvc != nil && s.mealPlanCron != "" {
		if _, err := s.cron.AddFunc(s.mealPlanCron, s.runMealPlanAutoCompleteJob); err != nil {
			slog.Warn("failed to schedule meal plan auto-complete", "error", err)
		} else {
			slog.Info("meal plan auto-complete scheduled", "cron", s.mealPlanCron)
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
		s.baseCancel()
		close(s.done)
		cronCtx := s.cron.Stop()
		select {
		case <-cronCtx.Done():
		case <-time.After(5 * time.Second):
			slog.Warn("scheduler: cron.Stop() timed out waiting for running callbacks")
		}
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			slog.Info("scheduler stopped cleanly")
		case <-time.After(5 * time.Second):
			slog.Warn("scheduler stop timed out waiting for background goroutines")
		}
	})
}

// DigestPendingRetry returns the current retry state (thread-safe).
func (s *Scheduler) DigestPendingRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digestPendingRetry
}

// SetKnowledgeLinter configures the knowledge linter and its cron expression.
// Must be called before Start().
func (s *Scheduler) SetKnowledgeLinter(linter *knowledge.Linter, cronExpr string) {
	s.knowledgeLinter = linter
	s.knowledgeLintCron = cronExpr
}

// MealPlanAutoCompleter is the interface for auto-completing meal plans.
type MealPlanAutoCompleter interface {
	AutoCompletePastPlans(ctx context.Context) (int, error)
}

// SetMealPlanAutoComplete configures the meal plan auto-complete job.
// Must be called before Start().
func (s *Scheduler) SetMealPlanAutoComplete(svc MealPlanAutoCompleter, cronExpr string) {
	s.mealPlanSvc = svc
	s.mealPlanCron = cronExpr
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
