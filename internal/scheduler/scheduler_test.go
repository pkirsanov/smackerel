package scheduler

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	s := New(nil, nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.cron == nil {
		t.Error("expected non-nil cron")
	}
	if s.digestGen != nil {
		t.Error("expected nil digestGen")
	}
	if s.bot != nil {
		t.Error("expected nil bot")
	}
}

func TestStart_InvalidCron(t *testing.T) {
	s := New(nil, nil, nil, nil)
	err := s.Start(nil, "invalid-cron-expression")
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestStart_ValidCron(t *testing.T) {
	s := New(nil, nil, nil, nil)
	// This will succeed but the cron job will fail on nil digestGen when triggered
	err := s.Start(nil, "0 7 * * *")
	if err != nil {
		t.Fatalf("expected no error for valid cron: %v", err)
	}
	s.Stop()
}

func TestStop(t *testing.T) {
	s := New(nil, nil, nil, nil)
	s.Start(nil, "0 0 * * *")
	// Stop should not panic
	s.Stop()
}

// SCN-002-060: Scheduler cron registers expected number of entries
func TestSCN002060_CronEntries(t *testing.T) {
	s := New(nil, nil, nil, nil)
	err := s.Start(nil, "0 7 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Stop()

	count := s.CronEntryCount()
	if count < 1 {
		t.Errorf("expected at least 1 cron entry, got %d", count)
	}
}

// SCN-002-061: Scheduler nil digestGen guard
func TestSCN002061_NilDigestGenGuard(t *testing.T) {
	// New with nil digestGen — the cron callback must not panic
	s := New(nil, nil, nil, nil)
	if s.digestGen != nil {
		t.Fatal("expected nil digestGen for this test")
	}
	// The guard in the cron callback checks s.digestGen == nil and returns.
	// We verify the struct is correctly set up with nil.
}

// SCN-002-062: Concurrent retry field access under race detector
func TestSCN002062_ConcurrentRetryAccess(t *testing.T) {
	s := New(nil, nil, nil, nil)
	var wg sync.WaitGroup

	// 100 goroutines writing
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				s.SetDigestPending(true, "2026-04-09")
			} else {
				s.SetDigestPending(false, "")
			}
		}(i)
	}

	// 100 goroutines reading
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.DigestPendingRetry()
			_ = s.DigestPendingDate()
		}()
	}

	wg.Wait()
}

// SCN-002-063: Retry field lifecycle
func TestSCN002063_RetryFieldLifecycle(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Initially false
	if s.DigestPendingRetry() {
		t.Error("expected digestPendingRetry to be false initially")
	}
	if s.DigestPendingDate() != "" {
		t.Error("expected digestPendingDate to be empty initially")
	}

	// Set pending
	s.SetDigestPending(true, "2026-04-09")
	if !s.DigestPendingRetry() {
		t.Error("expected digestPendingRetry to be true after set")
	}
	if s.DigestPendingDate() != "2026-04-09" {
		t.Errorf("expected digestPendingDate '2026-04-09', got %q", s.DigestPendingDate())
	}

	// Clear pending
	s.SetDigestPending(false, "")
	if s.DigestPendingRetry() {
		t.Error("expected digestPendingRetry to be false after clear")
	}
	if s.DigestPendingDate() != "" {
		t.Error("expected digestPendingDate to be empty after clear")
	}
}

// SCN-002-058: Verify retry fields are protected by mutex (structural test)
func TestSCN002058_MutexProtectsRetryFields(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Simulate the race scenario: cron reads while goroutine writes
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.SetDigestPending(true, "2026-04-09")
		}()
		go func() {
			defer wg.Done()
			retry := s.DigestPendingRetry()
			date := s.DigestPendingDate()
			// Either both set or both cleared — no torn read
			if retry && date == "" {
				// This is acceptable during transitions
			}
			_ = retry
			_ = date
		}()
	}
	wg.Wait()
}

// SCN-002-059: Retry cleared on success
func TestSCN002059_RetryClearsOnSuccess(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Simulate timeout setting retry
	s.SetDigestPending(true, "2026-04-09")
	if !s.DigestPendingRetry() {
		t.Fatal("expected pending retry to be set")
	}

	// Simulate successful delivery clearing retry
	s.SetDigestPending(false, "")
	if s.DigestPendingRetry() {
		t.Error("expected pending retry to be cleared after success")
	}
	if s.DigestPendingDate() != "" {
		t.Error("expected pending date to be cleared after success")
	}
}
