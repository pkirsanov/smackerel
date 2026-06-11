package scheduler

// Spec 083 Card Rewards Companion (Scope 09) — T-09-01 (unit).
//
// These tests prove the SCHEDULER WIRING for the card-rewards jobs:
//   - SCN-083-I01/I02: card_rewards_refresh and card_rewards_recommend are
//     registered on EXACTLY their configured crons (and not swapped), only when
//     a pipeline + non-empty cron are present (fail-loud SST — no default).
//   - The scheduled cron callbacks invoke the pipeline with trigger="scheduled".
//   - SCN-083-I05 (wiring half): the admin manual triggers reuse the SAME
//     pipeline methods with trigger="manual" (NFR-CR-005 shared code path).
//
// The pipeline is faked so these stay pure unit tests with no DB. The live-PG
// behavior of the pipeline itself (I03/I04 full pipelines audited, I05 manual
// reuse, I06 idempotency) is proven in
// internal/cardrewards/pipeline_integration_test.go.

import (
	"context"
	"errors"
	"testing"
)

// fakeCardRewardsPipeline records the trigger label each path used so the
// wiring tests can assert scheduled→"scheduled" and manual→"manual" (and that
// both reuse the SAME methods).
type fakeCardRewardsPipeline struct {
	refreshTriggers   []string
	recommendTriggers []string
	refreshErr        error
	recommendErr      error
}

func (f *fakeCardRewardsPipeline) RunDailyRefresh(_ context.Context, trigger string) error {
	f.refreshTriggers = append(f.refreshTriggers, trigger)
	return f.refreshErr
}

func (f *fakeCardRewardsPipeline) RunMonthlyRecommend(_ context.Context, trigger string) error {
	f.recommendTriggers = append(f.recommendTriggers, trigger)
	return f.recommendErr
}

// cronFor returns the registered cron for a card-rewards job name, or "" when
// the job was not registered.
func cronFor(s *Scheduler, name string) string {
	for _, j := range s.cardRewardsJobs {
		if j.name == name {
			return j.cron
		}
	}
	return ""
}

// SCN-083-I01, I02 — both jobs register on EXACTLY their configured crons.
func TestCardRewardsJobsRegisteredOnConfiguredCrons_I01_I02(t *testing.T) {
	const scrapeCron = "0 6 * * *"    // daily 06:00
	const recommendCron = "0 7 1 * *" // 1st of month 07:00
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	fake := &fakeCardRewardsPipeline{}
	s.SetCardRewardsJobs(fake, scrapeCron, recommendCron)

	before := s.CronEntryCount()
	s.scheduleCardRewardsJobs()
	after := s.CronEntryCount()

	if got := after - before; got != 2 {
		t.Fatalf("expected 2 new cron entries, got %d (before=%d after=%d)", got, before, after)
	}

	// I01: refresh job registered on the scrape cron.
	if got := cronFor(s, "card_rewards_refresh"); got != scrapeCron {
		t.Fatalf("card_rewards_refresh registered on %q, want %q", got, scrapeCron)
	}
	// I02: recommend job registered on the monthly-recommend cron.
	if got := cronFor(s, "card_rewards_recommend"); got != recommendCron {
		t.Fatalf("card_rewards_recommend registered on %q, want %q", got, recommendCron)
	}

	// ADVERSARIAL: the crons must not be swapped. A wiring regression that put
	// refresh on the monthly cron (or recommend on the daily cron) would fire
	// the daily pipeline once a month — this catches it.
	if cronFor(s, "card_rewards_refresh") == recommendCron {
		t.Fatal("card_rewards_refresh is on the monthly-recommend cron — refresh/recommend crons are swapped")
	}
	if cronFor(s, "card_rewards_recommend") == scrapeCron {
		t.Fatal("card_rewards_recommend is on the daily scrape cron — refresh/recommend crons are swapped")
	}
}

// The scheduled cron callbacks invoke the pipeline with trigger="scheduled".
func TestCardRewardsScheduledJobsUseScheduledTrigger(t *testing.T) {
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	fake := &fakeCardRewardsPipeline{}
	s.SetCardRewardsJobs(fake, "0 6 * * *", "0 7 1 * *")

	s.runCardRewardsRefreshJob()
	s.runCardRewardsRecommendJob()

	if len(fake.refreshTriggers) != 1 || fake.refreshTriggers[0] != cardRewardsTriggerScheduled {
		t.Fatalf("refresh triggers = %v, want one %q", fake.refreshTriggers, cardRewardsTriggerScheduled)
	}
	if len(fake.recommendTriggers) != 1 || fake.recommendTriggers[0] != cardRewardsTriggerScheduled {
		t.Fatalf("recommend triggers = %v, want one %q", fake.recommendTriggers, cardRewardsTriggerScheduled)
	}
}

// SCN-083-I05 (wiring half) — the manual triggers reuse the SAME pipeline
// methods with trigger="manual". This is the shared-code-path proof at the
// scheduler boundary (NFR-CR-005); the live-PG reuse is proven in the pipeline
// integration test.
func TestCardRewardsManualTriggersReuseSameMethodsWithManualTrigger_I05(t *testing.T) {
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	fake := &fakeCardRewardsPipeline{}
	s.SetCardRewardsJobs(fake, "0 6 * * *", "0 7 1 * *")
	ctx := context.Background()

	if err := s.TriggerCardRewardsRefreshNow(ctx); err != nil {
		t.Fatalf("TriggerCardRewardsRefreshNow: %v", err)
	}
	if err := s.TriggerCardRewardsRecommendNow(ctx); err != nil {
		t.Fatalf("TriggerCardRewardsRecommendNow: %v", err)
	}

	if len(fake.refreshTriggers) != 1 || fake.refreshTriggers[0] != cardRewardsTriggerManual {
		t.Fatalf("manual refresh triggers = %v, want one %q", fake.refreshTriggers, cardRewardsTriggerManual)
	}
	if len(fake.recommendTriggers) != 1 || fake.recommendTriggers[0] != cardRewardsTriggerManual {
		t.Fatalf("manual recommend triggers = %v, want one %q", fake.recommendTriggers, cardRewardsTriggerManual)
	}
}

// The manual trigger propagates a pipeline error to the caller (admin UI shows
// the failure instead of a silent success).
func TestCardRewardsManualTriggerPropagatesPipelineError(t *testing.T) {
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	wantErr := errors.New("boom")
	fake := &fakeCardRewardsPipeline{refreshErr: wantErr}
	s.SetCardRewardsJobs(fake, "0 6 * * *", "0 7 1 * *")

	if err := s.TriggerCardRewardsRefreshNow(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("TriggerCardRewardsRefreshNow err = %v, want %v", err, wantErr)
	}
}

// Fail-loud SST: with no pipeline wired, no jobs register and the manual
// trigger errors (it never silently no-ops).
func TestCardRewardsJobsNotRegisteredWhenPipelineNil(t *testing.T) {
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	s.SetCardRewardsJobs(nil, "0 6 * * *", "0 7 1 * *")

	before := s.CronEntryCount()
	s.scheduleCardRewardsJobs()
	if after := s.CronEntryCount(); after != before {
		t.Fatalf("expected no cron entries with a nil pipeline, before=%d after=%d", before, after)
	}
	if len(s.cardRewardsJobs) != 0 {
		t.Fatalf("expected no registered jobs with a nil pipeline, got %v", s.cardRewardsJobs)
	}
	if err := s.TriggerCardRewardsRefreshNow(context.Background()); err == nil {
		t.Fatal("expected an error from TriggerCardRewardsRefreshNow with no pipeline configured")
	}
}

// An empty cron registers nothing for that job (no default cron is invented),
// while a present cron still registers its job.
func TestCardRewardsEmptyCronSkipsThatJob(t *testing.T) {
	s := New(nil, nil, nil, nil)
	defer s.Stop()
	fake := &fakeCardRewardsPipeline{}
	s.SetCardRewardsJobs(fake, "0 6 * * *", "") // recommend cron empty

	s.scheduleCardRewardsJobs()

	if got := cronFor(s, "card_rewards_refresh"); got != "0 6 * * *" {
		t.Fatalf("card_rewards_refresh cron = %q, want %q", got, "0 6 * * *")
	}
	if got := cronFor(s, "card_rewards_recommend"); got != "" {
		t.Fatalf("card_rewards_recommend should not be registered with an empty cron, got %q", got)
	}
}
