package scheduler

// DigestPendingRetry returns the current retry state (thread-safe).
func (s *Scheduler) DigestPendingRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digestPendingRetry
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
