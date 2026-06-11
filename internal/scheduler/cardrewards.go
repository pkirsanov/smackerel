package scheduler

// Card-rewards scheduler jobs + manual triggers (spec 083 Scope 09, design §8 /
// FR-CR-018, FR-CR-019, NFR-CR-005).
//
// Registers two cron jobs via the same AddFunc + runGuarded pattern every other
// scheduler job uses:
//
//   - card_rewards_refresh   on card_rewards.scrape_cron            (SCN-083-I01)
//   - card_rewards_recommend on card_rewards.monthly_recommend_cron (SCN-083-I02)
//
// Both crons originate from the fail-loud SST loader (config.CardRewardsConfig,
// Scope 01) — the scheduler introduces NO default and registers a job only when
// its cron is non-empty and a pipeline is wired.
//
// The admin "scrape now" / "sync calendar now" manual triggers
// (TriggerCardRewardsRefreshNow / TriggerCardRewardsRecommendNow) call the SAME
// pipeline methods as the cron callbacks, differing only in the trigger label
// ("manual" vs "scheduled", NFR-CR-005). They run under the same overlap guard
// as the scheduled jobs, so a manual trigger fired while the cron job is in
// flight is skipped rather than run concurrently.

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Trigger labels passed to the pipeline. They mirror cardrewards.RunTrigger*
// and the card_runs.trigger CHECK constraint; the pipeline validates them, so a
// drift here fails loud rather than silently mis-auditing.
const (
	cardRewardsTriggerScheduled = "scheduled"
	cardRewardsTriggerManual    = "manual"
)

// cardRewardsJobTimeout bounds one full pipeline run (the daily refresh fans
// out connector sync → extract → reconcile → lifecycle; the monthly recommend
// runs optimize → recommend → calendar sync). Generous because extraction may
// call the ML sidecar per rotating card.
const cardRewardsJobTimeout = 10 * time.Minute

// CardRewardsRefresher is the pipeline seam the scheduler drives for the
// card-rewards daily-refresh and monthly-recommend jobs (NFR-CR-005 shared code
// path). *cardrewards.Pipeline satisfies it. Declaring the interface here keeps
// the scheduler decoupled from internal/cardrewards (no import) and lets the
// wiring unit test drive a fake that records the trigger each path used. Both
// methods return only an error; the observable outcome is the persisted
// card_runs / rotating_categories / card_recommendations state.
type CardRewardsRefresher interface {
	RunDailyRefresh(ctx context.Context, trigger string) error
	RunMonthlyRecommend(ctx context.Context, trigger string) error
}

// cardRewardsJobReg records one registered card-rewards cron job so the wiring
// is assertable (SCN-083-I01/I02): the unit test reads s.cardRewardsJobs and
// confirms each job name is registered on EXACTLY its configured cron.
type cardRewardsJobReg struct {
	name string
	cron string
}

// SetCardRewardsJobs wires the card-rewards refresh/recommend pipeline and its
// two cron expressions (spec 083 Scope 09). Both crons come from the fail-loud
// SST loader (Scope 01); the scheduler adds NO default. Must be called before
// Start(). Passing a nil pipeline (feature disabled) registers nothing.
func (s *Scheduler) SetCardRewardsJobs(pipeline CardRewardsRefresher, scrapeCron, recommendCron string) {
	s.cardRewardsPipeline = pipeline
	s.cardRewardsScrapeCron = scrapeCron
	s.cardRewardsRecommendCron = recommendCron
}

// scheduleCardRewardsJobs registers the daily-refresh and monthly-recommend
// jobs when a pipeline is wired and the corresponding cron is non-empty. It
// records each successful registration in s.cardRewardsJobs (name + cron) so
// the wiring is assertable. Called from Start().
func (s *Scheduler) scheduleCardRewardsJobs() {
	if s.cardRewardsPipeline == nil {
		return
	}
	if s.cardRewardsScrapeCron != "" {
		if _, err := s.cron.AddFunc(s.cardRewardsScrapeCron, s.runCardRewardsRefreshJob); err != nil {
			slog.Warn("failed to schedule card_rewards_refresh", "cron", s.cardRewardsScrapeCron, "error", err)
		} else {
			s.cardRewardsJobs = append(s.cardRewardsJobs, cardRewardsJobReg{name: "card_rewards_refresh", cron: s.cardRewardsScrapeCron})
			slog.Info("card_rewards_refresh scheduled", "cron", s.cardRewardsScrapeCron)
		}
	}
	if s.cardRewardsRecommendCron != "" {
		if _, err := s.cron.AddFunc(s.cardRewardsRecommendCron, s.runCardRewardsRecommendJob); err != nil {
			slog.Warn("failed to schedule card_rewards_recommend", "cron", s.cardRewardsRecommendCron, "error", err)
		} else {
			s.cardRewardsJobs = append(s.cardRewardsJobs, cardRewardsJobReg{name: "card_rewards_recommend", cron: s.cardRewardsRecommendCron})
			slog.Info("card_rewards_recommend scheduled", "cron", s.cardRewardsRecommendCron)
		}
	}
}

// runCardRewardsRefreshJob is the scheduled `card_rewards_refresh` cron
// callback. It runs the daily refresh pipeline with trigger="scheduled" under
// the refresh overlap guard.
func (s *Scheduler) runCardRewardsRefreshJob() {
	s.runGuarded(&s.muCardRewardsRefresh, "card-rewards", "card_rewards_refresh", func() {
		s.execCardRewardsRefresh(cardRewardsTriggerScheduled)
	})
}

// runCardRewardsRecommendJob is the scheduled `card_rewards_recommend` cron
// callback. It runs the monthly recommend pipeline with trigger="scheduled"
// under the recommend overlap guard.
func (s *Scheduler) runCardRewardsRecommendJob() {
	s.runGuarded(&s.muCardRewardsRecommend, "card-rewards", "card_rewards_recommend", func() {
		s.execCardRewardsRecommend(cardRewardsTriggerScheduled)
	})
}

// execCardRewardsRefresh runs the daily-refresh pipeline with the given trigger
// and logs (does not propagate) a pipeline error — a scheduled run that fails
// is recorded in its card_runs audit rows and surfaced via logs; it never
// crashes the scheduler.
func (s *Scheduler) execCardRewardsRefresh(trigger string) {
	ctx, cancel := context.WithTimeout(s.baseCtx, cardRewardsJobTimeout)
	defer cancel()
	if err := s.cardRewardsPipeline.RunDailyRefresh(ctx, trigger); err != nil {
		slog.Error("card_rewards_refresh failed", "trigger", trigger, "error", err)
	}
}

// execCardRewardsRecommend runs the monthly-recommend pipeline with the given
// trigger and logs (does not propagate) a pipeline error.
func (s *Scheduler) execCardRewardsRecommend(trigger string) {
	ctx, cancel := context.WithTimeout(s.baseCtx, cardRewardsJobTimeout)
	defer cancel()
	if err := s.cardRewardsPipeline.RunMonthlyRecommend(ctx, trigger); err != nil {
		slog.Error("card_rewards_recommend failed", "trigger", trigger, "error", err)
	}
}

// TriggerCardRewardsRefreshNow runs the daily-refresh pipeline immediately with
// trigger="manual" — the admin "scrape now" action. It reuses the SAME pipeline
// method as the scheduled job (NFR-CR-005); the only difference is the trigger
// label. It runs under the refresh overlap guard, so it is skipped (returns nil)
// when a scheduled refresh is already in flight. Returns an error only when the
// pipeline is not configured or the run itself errors.
func (s *Scheduler) TriggerCardRewardsRefreshNow(ctx context.Context) error {
	if s.cardRewardsPipeline == nil {
		return fmt.Errorf("scheduler: card-rewards pipeline not configured")
	}
	var runErr error
	s.runGuarded(&s.muCardRewardsRefresh, "card-rewards", "card_rewards_refresh_manual", func() {
		runErr = s.cardRewardsPipeline.RunDailyRefresh(ctx, cardRewardsTriggerManual)
	})
	return runErr
}

// TriggerCardRewardsRecommendNow runs the monthly-recommend pipeline immediately
// with trigger="manual" — the admin "sync calendar now" / "recommend now"
// action. It reuses the SAME pipeline method as the scheduled job (NFR-CR-005),
// under the recommend overlap guard. Returns an error only when the pipeline is
// not configured or the run itself errors.
func (s *Scheduler) TriggerCardRewardsRecommendNow(ctx context.Context) error {
	if s.cardRewardsPipeline == nil {
		return fmt.Errorf("scheduler: card-rewards pipeline not configured")
	}
	var runErr error
	s.runGuarded(&s.muCardRewardsRecommend, "card-rewards", "card_rewards_recommend_manual", func() {
		runErr = s.cardRewardsPipeline.RunMonthlyRecommend(ctx, cardRewardsTriggerManual)
	})
	return runErr
}
