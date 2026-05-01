package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
)

// wireRecommendationWatchPoller wires the spec 039 Scope 4 watch evaluator
// into the scheduler and (when configured) the Telegram bot for delivery.
func wireRecommendationWatchPoller(
	sched *scheduler.Scheduler,
	agentBridge *agent.Bridge,
	svc *coreServices,
	cfg *config.Config,
	tgBot *telegram.Bot,
	watchHandlers *api.RecommendationWatchHandlers,
) {
	if !cfg.Recommendations.Enabled {
		return
	}
	if svc.recommendationStore == nil {
		slog.Warn("recommendation watch poller skipped: store not initialised")
		return
	}
	pollCron := cfg.Recommendations.Watches.PollCron
	if pollCron == "" {
		slog.Warn("recommendation watch poller skipped: poll_cron is empty")
		return
	}
	evaluator := watch.NewEvaluator(watch.Options{
		Store:    svc.recommendationStore,
		Registry: svc.recommendationRegistry,
		Clock:    func() time.Time { return time.Now().UTC() },
	})
	source := &recommendationWatchSource{
		store:     svc.recommendationStore,
		evaluator: evaluator,
		bot:       tgBot,
	}
	if tgBot != nil {
		tgBot.SetWatchService(svc.recommendationStore)
	}
	if watchHandlers != nil {
		watchHandlers.SetTriggerEvaluator(&recommendationWatchTriggerAdapter{
			evaluator: evaluator,
			source:    source,
		})
	}
	sched.SetRecommendationWatchPoller(source, agentBridge, pollCron)
	slog.Info("recommendation watch poller wired", "cron", pollCron)
}

// recommendationWatchTriggerAdapter exposes the synchronous trigger surface
// needed by the API endpoint. It runs the evaluator and, when the run results
// in a delivered alert, immediately delivers via the bot like the scheduler
// poller would.
type recommendationWatchTriggerAdapter struct {
	evaluator *watch.Evaluator
	source    *recommendationWatchSource
}

func (a *recommendationWatchTriggerAdapter) EvaluateWatchSync(ctx context.Context, watchID, triggerKind string, triggerContext map[string]any) (api.RecommendationWatchTriggerResult, error) {
	outcome, err := a.evaluator.EvaluateWatch(ctx, watchID, watch.TriggerContext{Kind: triggerKind, Context: triggerContext})
	if err != nil {
		return api.RecommendationWatchTriggerResult{}, err
	}
	if a.source != nil && a.source.bot != nil && outcome.DeliveryDecision == "sent" {
		for _, alert := range outcome.NotifyEnvelopes {
			if sendErr := a.source.bot.SendWatchAlert(ctx, telegram.WatchAlert{
				WatchID:     alert.WatchID,
				WatchName:   alert.WatchName,
				ActorUserID: alert.ActorUserID,
				Title:       alert.Title,
				Subtitle:    alert.Subtitle,
				Provider:    alert.Provider,
				Why:         alert.Why,
				Labels:      alert.Labels,
			}); sendErr != nil {
				slog.Warn("trigger watch alert send failed", "watch_id", alert.WatchID, "error", sendErr)
			}
		}
	}
	return api.RecommendationWatchTriggerResult{
		WatchRunID:        outcome.WatchRunID,
		Status:            outcome.Status,
		DeliveryDecision:  outcome.DeliveryDecision,
		Delivered:         outcome.Delivered,
		Withheld:          outcome.Withheld,
		RawCandidates:     outcome.RawCandidates,
		WithheldReasons:   outcome.WithheldReasons,
		RecommendationIDs: outcome.RecommendationIDs,
	}, nil
}

// recommendationWatchSource adapts *store.Store + watch.Evaluator to the
// scheduler.RecommendationWatchSource contract. It loads only due watches
// (SCN-039-030/031) and produces the structured envelope the scheduler hands
// to FireScenario.
type recommendationWatchSource struct {
	store     *store.Store
	evaluator *watch.Evaluator
	bot       *telegram.Bot
}

func (s *recommendationWatchSource) DueWatchEnvelopes(ctx context.Context, asOf time.Time) ([]scheduler.RecommendationWatchEnvelope, error) {
	watches, err := s.store.DueWatches(ctx, asOf)
	if err != nil {
		return nil, err
	}
	envelopes := make([]scheduler.RecommendationWatchEnvelope, 0, len(watches))
	for _, watchRecord := range watches {
		envelope := scheduler.RecommendationWatchEnvelope{
			WatchID:     watchRecord.ID,
			ActorUserID: watchRecord.ActorUserID,
			Kind:        watchRecord.Kind,
			TriggerKind: triggerForKind(watchRecord.Kind),
			Trigger: map[string]any{
				"watch_name":         watchRecord.Name,
				"location_precision": watchRecord.LocationPrecision,
				"queue_policy":       watchRecord.QueuePolicy,
			},
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}

// EvaluateAndDeliver runs the watch.Evaluator for one envelope and delivers
// any resulting alerts via the configured bot.
func (s *recommendationWatchSource) EvaluateAndDeliver(ctx context.Context, envelope scheduler.RecommendationWatchEnvelope) {
	outcome, err := s.evaluator.EvaluateWatch(ctx, envelope.WatchID, watch.TriggerContext{
		Kind:    envelope.TriggerKind,
		Context: envelope.Trigger,
	})
	if err != nil {
		slog.Warn("recommendation watch evaluation failed", "watch_id", envelope.WatchID, "error", err)
		return
	}
	if s.bot == nil || outcome.DeliveryDecision != "sent" {
		return
	}
	for _, alert := range outcome.NotifyEnvelopes {
		if err := s.bot.SendWatchAlert(ctx, telegram.WatchAlert{
			WatchID:     alert.WatchID,
			WatchName:   alert.WatchName,
			ActorUserID: alert.ActorUserID,
			Title:       alert.Title,
			Subtitle:    alert.Subtitle,
			Provider:    alert.Provider,
			Why:         alert.Why,
			Labels:      alert.Labels,
		}); err != nil {
			slog.Warn("failed to send watch alert", "watch_id", alert.WatchID, "error", err)
		}
	}
}

func triggerForKind(kind string) string {
	switch kind {
	case "location_radius":
		return "dwell"
	case "topic_keyword":
		return "schedule"
	case "trip_context":
		return "trip_window"
	case "price_drop":
		return "price_check"
	}
	return "schedule"
}
