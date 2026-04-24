package scheduler

import (
	"log/slog"
	"sync"
	"time"
)

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

// runGuarded runs fn under a TryLock guard. If the mutex is already held
// (another invocation of the same job is in progress), the call is skipped
// with a warning log. This centralises the overlap-prevention pattern used
// by all 14 cron jobs (SCN-022-09 through SCN-022-11).
func (s *Scheduler) runGuarded(mu *sync.Mutex, group, job string, fn func()) {
	if !mu.TryLock() {
		slog.Warn("skipping overlapping job", "group", group, "job", job)
		return
	}
	defer mu.Unlock()
	fn()
}
