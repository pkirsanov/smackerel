package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/db"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
	"github.com/smackerel/smackerel/internal/scheduler"
	"github.com/smackerel/smackerel/internal/telegram"
)

// shutdownAll performs explicit sequential shutdown in reverse-dependency order.
// Sequence: scheduler → HTTP → Telegram → result subscribers → connectors → NATS → DB.
// Each step gets a timeout budget; if a step hangs, a warning is logged and shutdown proceeds.
// An overall deadline prevents individual step budgets from summing beyond the total timeout,
// which would otherwise risk Docker SIGKILL (IMP-022-001).
func shutdownAll(
	timeoutS int,
	sched *scheduler.Scheduler,
	srv *http.Server,
	tgBot *telegram.Bot,
	resultSub *pipeline.ResultSubscriber,
	synthesisSub *pipeline.SynthesisResultSubscriber,
	domainSub *pipeline.DomainResultSubscriber,
	supervisor *connector.Supervisor,
	nc *smacknats.Client,
	pg *db.Postgres,
) {
	shutdownStart := time.Now()
	totalTimeout := time.Duration(timeoutS) * time.Second
	slog.Info("starting graceful shutdown", "timeout_s", timeoutS)

	// Overall deadline prevents individual step budgets from summing beyond totalTimeout.
	// Once this context expires, all remaining steps are skipped immediately.
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), totalTimeout)
	defer deadlineCancel()
	deadline := deadlineCtx.Done()

	defer func() {
		slog.Info("graceful shutdown complete", "elapsed_ms", time.Since(shutdownStart).Milliseconds(), "budget_s", timeoutS)
	}()

	// Step 1: Stop scheduler (no new cron jobs fire) — 2s budget
	runWithTimeout("scheduler", 2*time.Second, deadline, func() {
		if sched != nil {
			sched.Stop()
		}
	})

	// Step 2: Drain HTTP server — allocate most of the budget here
	httpTimeout := totalTimeout - 10*time.Second
	if httpTimeout < 5*time.Second {
		httpTimeout = 5 * time.Second
	}
	runWithTimeout("HTTP server", httpTimeout, deadline, func() {
		if srv != nil {
			httpCtx, httpCancel := context.WithTimeout(context.Background(), httpTimeout)
			defer httpCancel()
			if err := srv.Shutdown(httpCtx); err != nil {
				slog.Warn("shutdown: HTTP server drain error", "error", err)
			}
		}
	})

	// Step 3: Stop Telegram bot (cancel long-poll) — 2s budget
	runWithTimeout("Telegram bot", 2*time.Second, deadline, func() {
		if tgBot != nil {
			tgBot.Stop()
		}
	})

	// Step 4: Stop result subscribers (NATS consumer drain) — 6s budget
	// Budget covers NATS Fetch() MaxWait (5s) + processing margin (1s).
	// If goroutines are still blocked in Fetch after 6s, step 6 (NATS close) will
	// interrupt the call; the done channel ensures no new messages are processed.
	runWithTimeout("result subscribers", 6*time.Second, deadline, func() {
		if resultSub != nil {
			resultSub.Stop()
		}
		if synthesisSub != nil {
			synthesisSub.Stop()
		}
		if domainSub != nil {
			domainSub.Stop()
		}
	})

	// Step 5: Stop connector supervisor (all connectors) — 2s budget
	runWithTimeout("connectors", 2*time.Second, deadline, func() {
		if supervisor != nil {
			supervisor.StopAll()
		}
	})

	// Step 6: Drain NATS connection (after all NATS consumers are stopped) — 2s budget
	runWithTimeout("NATS", 2*time.Second, deadline, func() {
		if nc != nil {
			nc.Close()
		}
	})

	// Step 7: Close DB pool (last — all DB consumers are already stopped) — 1s budget
	runWithTimeout("database pool", 1*time.Second, deadline, func() {
		if pg != nil {
			pg.Close()
		}
	})
}

// runWithTimeout runs fn with a timeout. If fn doesn't complete within budget
// or the overall shutdown deadline fires first, a warning is logged and control
// returns immediately so shutdown can proceed.
func runWithTimeout(step string, budget time.Duration, deadline <-chan struct{}, fn func()) {
	// If overall deadline already expired, skip this step immediately.
	select {
	case <-deadline:
		slog.Warn("shutdown: overall deadline exceeded, skipping step", "step", step)
		return
	default:
	}

	slog.Info("shutdown: stopping "+step, "budget", budget)
	stepStart := time.Now()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("shutdown: "+step+" stopped", "elapsed_ms", time.Since(stepStart).Milliseconds())
	case <-time.After(budget):
		slog.Warn("shutdown: step exceeded timeout, proceeding", "step", step, "budget", budget, "elapsed_ms", time.Since(stepStart).Milliseconds())
	case <-deadline:
		slog.Warn("shutdown: overall deadline exceeded during step, proceeding", "step", step, "elapsed_ms", time.Since(stepStart).Milliseconds())
	}
}
