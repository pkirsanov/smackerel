package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/smackerel/smackerel/internal/digest"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/knowledge"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/topics"
)

// SynthesisRunner is the scheduler's narrow durable-run boundary. Both daily
// and weekly jobs must use this path so scheduling cannot bypass persistence.
type SynthesisRunner interface {
	RunAndPersist(context.Context, intelligence.SynthesisCadence, intelligence.SynthesisTriggerKind, time.Time) (*intelligence.SynthesisAggregate, error)
}

// SynthesisSchedule is the required scheduler cadence contract. Both values
// originate in the synthesis SST and are validated before any cron entry is
// registered. Standard cron expressions and robfig descriptors such as
// @every are accepted through the same parser used by cron.AddFunc.
type SynthesisSchedule struct {
	DailyCron  string
	WeeklyCron string
}

func (c SynthesisSchedule) validated() (SynthesisSchedule, error) {
	c.DailyCron = strings.TrimSpace(c.DailyCron)
	c.WeeklyCron = strings.TrimSpace(c.WeeklyCron)
	for _, cadence := range []struct {
		name       string
		expression string
	}{
		{name: "daily", expression: c.DailyCron},
		{name: "weekly", expression: c.WeeklyCron},
	} {
		if cadence.expression == "" {
			return SynthesisSchedule{}, fmt.Errorf("required %s synthesis cron is empty", cadence.name)
		}
		if _, err := cron.ParseStandard(cadence.expression); err != nil {
			return SynthesisSchedule{}, fmt.Errorf("required %s synthesis cron %q is invalid: %w", cadence.name, cadence.expression, err)
		}
	}
	return c, nil
}

// Scheduler manages cron-triggered tasks.
type Scheduler struct {
	cron      *cron.Cron
	digestGen *digest.Generator
	bot       *telegram.Bot
	engine    *intelligence.Engine
	// synthesisProducer persists synthesis output. Nil until wired by cmd/core;
	// when nil the job reports UNAVAILABLE rather than logging a count, because
	// "ran and stored nothing" was the defect BUG-004-004 repairs.
	synthesisProducer         SynthesisRunner
	synthesisSchedule         SynthesisSchedule
	lifecycle                 *topics.Lifecycle
	mu                        sync.Mutex // protects digestPendingRetry and digestPendingDate
	digestPendingRetry        bool
	digestPendingDate         string
	baseCtx                   context.Context
	baseCancel                context.CancelFunc
	done                      chan struct{}
	wg                        sync.WaitGroup
	stopOnce                  sync.Once
	muDigest                  sync.Mutex
	muHourly                  sync.Mutex
	muDaily                   sync.Mutex
	muWeekly                  sync.Mutex
	muMonthly                 sync.Mutex
	muBriefs                  sync.Mutex
	muAlerts                  sync.Mutex
	muAlertProd               sync.Mutex
	muResurface               sync.Mutex
	muLookups                 sync.Mutex
	muSubs                    sync.Mutex
	muRelCool                 sync.Mutex
	muKnowledgeLint           sync.Mutex
	knowledgeLinter           *knowledge.Linter
	knowledgeLintCron         string
	muMealPlanComplete        sync.Mutex
	mealPlanSvc               MealPlanAutoCompleter
	mealPlanCron              string
	muRecommendationWatchPoll sync.Mutex
	muWatchPoller             sync.Mutex
	recommendationWatchSource RecommendationWatchSource
	recommendationWatchRunner AgentRunner
	recommendationWatchCron   string
	// Spec 076 SCOPE-6a — legacy-retirement runtime wiring fields.
	muLegacyRetirement                sync.Mutex
	muLegacyRetirementThreshold       sync.Mutex
	muLegacyRetirementObservation     sync.Mutex
	legacyRetirementThresholdInterval time.Duration
	legacyRetirementThresholdFn       LegacyRetirementJobFunc
	legacyRetirementObservationCron   string
	legacyRetirementObservationFn     LegacyRetirementJobFunc

	// Spec 083 Scope 09 — card-rewards daily-refresh + monthly-recommend
	// job wiring. The pipeline is the shared code path for both the cron
	// jobs and the admin manual triggers (NFR-CR-005); the two crons
	// originate from the fail-loud SST loader (Scope 01). cardRewardsJobs
	// records each registered (name, cron) so the wiring is assertable.
	muCardRewardsRefresh     sync.Mutex
	muCardRewardsRecommend   sync.Mutex
	cardRewardsPipeline      CardRewardsRefresher
	cardRewardsScrapeCron    string
	cardRewardsRecommendCron string
	cardRewardsJobs          []cardRewardsJobReg

	// Spec 021 Scope 4 — unified surfacing controller. When non-nil,
	// every producer routes its candidate through Controller.Propose
	// before dispatching to Telegram / web push / ntfy / email-out.
	// When nil (e.g., legacy tests without SST), producers fall back
	// to the prior direct-dispatch behavior so existing test fixtures
	// keep working.
	surfacingController *surfacing.Controller
}

// SetSurfacingController wires the unified surfacing controller (spec
// 021 Scope 4) so every producer routes through Propose before
// dispatching. Call exactly once during startup.
func (s *Scheduler) SetSurfacingController(c *surfacing.Controller) {
	s.surfacingController = c
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
	if s.engine != nil {
		if _, err := s.synthesisSchedule.validated(); err != nil {
			return fmt.Errorf("validate required synthesis schedule before scheduler start: %w", err)
		}
	}
	if _, err := s.cron.AddFunc(cronExpr, s.runDigestJob); err != nil {
		return err
	}
	if s.lifecycle != nil {
		if _, err := s.cron.AddFunc("0 * * * *", s.runTopicMomentumJob); err != nil {
			slog.Warn("failed to schedule topic momentum", "error", err)
		}
	}
	if s.engine != nil {
		if err := s.scheduleEngineJobs(); err != nil {
			return err
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
	s.scheduleRecommendationWatchPoller()
	s.scheduleLegacyRetirementJobs()
	s.scheduleCardRewardsJobs()
	s.cron.Start()
	slog.Info("scheduler started", "digest_cron", cronExpr)
	return nil
}

type engineJobEntry struct {
	name     string
	cron     string
	fn       func()
	required bool
}

func (s *Scheduler) engineJobEntries() []engineJobEntry {
	return []engineJobEntry{
		{name: "synthesis", cron: s.synthesisSchedule.DailyCron, fn: s.runSynthesisJob, required: true},
		{name: "resurfacing", cron: "0 8 * * *", fn: s.runResurfacingJob},
		{name: "pre-meeting briefs", cron: "*/5 * * * *", fn: s.runPreMeetingBriefsJob},
		{name: "weekly synthesis", cron: s.synthesisSchedule.WeeklyCron, fn: s.runWeeklySynthesisJob, required: true},
		{name: "monthly report", cron: "0 3 1 * *", fn: s.runMonthlyReportJob},
		{name: "subscription detection", cron: "0 3 * * 1", fn: s.runSubscriptionDetectionJob},
		{name: "frequent lookup detection", cron: "0 4 * * *", fn: s.runFrequentLookupsJob},
		{name: "alert delivery sweep", cron: "*/15 * * * *", fn: s.runAlertDeliveryJob},
		{name: "daily alert production", cron: "0 6 * * *", fn: s.runAlertProductionJob},
		{name: "relationship cooling alert production", cron: "0 7 * * 1", fn: s.runRelationshipCoolingJob},
	}
}

// scheduleEngineJobs registers all intelligence-engine-backed cron jobs.
// Synthesis cadences are required and fail scheduler startup if registration
// fails. Existing optional jobs preserve their warn-and-continue behavior.
func (s *Scheduler) scheduleEngineJobs() error {
	entries := s.engineJobEntries()
	for _, e := range entries {
		if _, err := s.cron.AddFunc(e.cron, e.fn); err != nil {
			if e.required {
				return fmt.Errorf("schedule required %s job: %w", e.name, err)
			}
			slog.Warn("failed to schedule "+e.name, "error", err)
		}
	}
	return nil
}

// SetSynthesisProducer wires durable synthesis persistence. Until it is called
// the synthesis job has nowhere to store output and says so explicitly.
func (s *Scheduler) SetSynthesisProducer(p SynthesisRunner) {
	s.synthesisProducer = p
}

// SetSynthesisSchedule validates and stores both required synthesis cadences.
// It must be called before Start; Start validates again so an omitted setter
// cannot silently fall back to an in-source schedule.
func (s *Scheduler) SetSynthesisSchedule(schedule SynthesisSchedule) error {
	validated, err := schedule.validated()
	if err != nil {
		return err
	}
	s.synthesisSchedule = validated
	return nil
}
