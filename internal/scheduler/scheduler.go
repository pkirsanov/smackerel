package scheduler

import (
	"context"
	"log/slog"
	"sync"

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
	if _, err := s.cron.AddFunc(cronExpr, s.runDigestJob); err != nil {
		return err
	}
	if s.lifecycle != nil {
		if _, err := s.cron.AddFunc("0 * * * *", s.runTopicMomentumJob); err != nil {
			slog.Warn("failed to schedule topic momentum", "error", err)
		}
	}
	if s.engine != nil {
		s.scheduleEngineJobs()
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

// scheduleEngineJobs registers all intelligence-engine-backed cron jobs.
func (s *Scheduler) scheduleEngineJobs() {
	entries := []struct {
		name string
		cron string
		fn   func()
	}{
		{"synthesis", "0 2 * * *", s.runSynthesisJob},
		{"resurfacing", "0 8 * * *", s.runResurfacingJob},
		{"pre-meeting briefs", "*/5 * * * *", s.runPreMeetingBriefsJob},
		{"weekly synthesis", "0 16 * * 0", s.runWeeklySynthesisJob},
		{"monthly report", "0 3 1 * *", s.runMonthlyReportJob},
		{"subscription detection", "0 3 * * 1", s.runSubscriptionDetectionJob},
		{"frequent lookup detection", "0 4 * * *", s.runFrequentLookupsJob},
		{"alert delivery sweep", "*/15 * * * *", s.runAlertDeliveryJob},
		{"daily alert production", "0 6 * * *", s.runAlertProductionJob},
		{"relationship cooling alert production", "0 7 * * 1", s.runRelationshipCoolingJob},
	}
	for _, e := range entries {
		if _, err := s.cron.AddFunc(e.cron, e.fn); err != nil {
			slog.Warn("failed to schedule "+e.name, "error", err)
		}
	}
}
