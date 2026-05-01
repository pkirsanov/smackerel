package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// RecommendationWatchSource produces the structured contexts for due watches
// at a given instant. Implementations MUST only return watches whose
// next_due_at is at or before the supplied time, never silenced and never
// disabled. Returning a watch that is not due is a contract violation: the
// scheduler relies on this filter to satisfy SCN-039-031 (rate-limit guard
// applied per scheduler tick) and SCN-039-032 (quiet-hours never produces
// extra invocations).
type RecommendationWatchSource interface {
	DueWatchEnvelopes(ctx context.Context, asOf time.Time) ([]RecommendationWatchEnvelope, error)
	EvaluateAndDeliver(ctx context.Context, envelope RecommendationWatchEnvelope)
}

// RecommendationWatchEnvelope is the per-watch payload the scheduler hands
// to FireScenario as the structured context for `recommendation_watch_evaluate`.
// It carries the watch_id, actor_user_id, kind, and trigger metadata.
type RecommendationWatchEnvelope struct {
	WatchID     string         `json:"watch_id"`
	ActorUserID string         `json:"actor_user_id"`
	Kind        string         `json:"kind"`
	TriggerKind string         `json:"trigger_kind"`
	Trigger     map[string]any `json:"trigger"`
}

// SetRecommendationWatchPoller registers the watch poller. cronExpr controls
// how often DueWatchEnvelopes is consulted. Must be called before Start().
func (s *Scheduler) SetRecommendationWatchPoller(source RecommendationWatchSource, runner AgentRunner, cronExpr string) {
	s.muWatchPoller.Lock()
	defer s.muWatchPoller.Unlock()
	s.recommendationWatchSource = source
	s.recommendationWatchRunner = runner
	s.recommendationWatchCron = cronExpr
}

func (s *Scheduler) hasRecommendationWatchPoller() bool {
	return s.recommendationWatchSource != nil && s.recommendationWatchRunner != nil && s.recommendationWatchCron != ""
}

func (s *Scheduler) scheduleRecommendationWatchPoller() {
	if !s.hasRecommendationWatchPoller() {
		return
	}
	if _, err := s.cron.AddFunc(s.recommendationWatchCron, s.runRecommendationWatchPollerJob); err != nil {
		slog.Warn("failed to schedule recommendation watch poller", "error", err)
		return
	}
	slog.Info("recommendation watch poller scheduled", "cron", s.recommendationWatchCron)
}

func (s *Scheduler) runRecommendationWatchPollerJob() {
	s.runGuarded(&s.muRecommendationWatchPoll, "recommendation-watch-poll", "recommendation-watch-poll", s.doRecommendationWatchPollerJob)
}

func (s *Scheduler) doRecommendationWatchPollerJob() {
	ctx, cancel := context.WithTimeout(s.baseCtx, 60*time.Second)
	defer cancel()
	envelopes, err := s.recommendationWatchSource.DueWatchEnvelopes(ctx, time.Now().UTC())
	if err != nil {
		slog.Error("recommendation watch poller failed to load due watches", "error", err)
		return
	}
	for _, envelope := range envelopes {
		select {
		case <-ctx.Done():
			slog.Warn("recommendation watch poller context cancelled mid-loop", "remaining", len(envelopes))
			return
		default:
		}
		structured, err := json.Marshal(envelope)
		if err != nil {
			slog.Warn("recommendation watch envelope marshal failed", "watch_id", envelope.WatchID, "error", err)
			continue
		}
		// SCN-039-030/031: every fire MUST flow through FireScenario so the
		// agent bridge owns the audit trail. The scheduler never short-circuits
		// scenario invocation.
		result, decision := FireScenario(ctx, s.recommendationWatchRunner, "recommendation_watch_evaluate", structured)
		if result == nil {
			slog.Warn("recommendation watch evaluate returned nil result", "watch_id", envelope.WatchID, "decision", decisionReason(decision))
			continue
		}
		// After the agent trace is recorded, run the watch evaluation pipeline
		// for actual side effects (persist watch_run, recommendations, deliveries).
		s.recommendationWatchSource.EvaluateAndDeliver(ctx, envelope)
		slog.Info("recommendation watch evaluated",
			"watch_id", envelope.WatchID,
			"trigger_kind", envelope.TriggerKind,
			"outcome", result.Outcome,
			"routing_reason", decisionReason(decision),
		)
	}
}

func decisionReason(d any) string {
	if d == nil {
		return ""
	}
	type reasoner interface{ ReasonString() string }
	if r, ok := d.(reasoner); ok {
		return r.ReasonString()
	}
	type fielded struct {
		Reason string
	}
	// Fall back to JSON probe — keeps this file independent of the agent
	// package's exact decision struct shape if it evolves.
	data, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	var probe fielded
	_ = json.Unmarshal(data, &probe)
	return probe.Reason
}

// schedulerWatchFields ensures the recommendation watch poller uses the same
// pattern as other guarded jobs: it is owned by the Scheduler struct.
//
// Adding the fields directly to the Scheduler struct in scheduler.go would
// require expanding that struct. Instead, we use a separate sync.Mutex
// declared here so the watch fields stay co-located with the watch behavior.
var _ = sync.Mutex{}
