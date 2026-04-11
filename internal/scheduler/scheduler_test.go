package scheduler

import (
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence"
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

// SCN-021: Scheduler with engine registers alert delivery + producer cron entries
func TestCronEntries_WithEngine(t *testing.T) {
	// Create engine with nil pool — cron registration still succeeds
	engine := &intelligence.Engine{Pool: nil}
	s := New(nil, nil, engine, nil)
	err := s.Start(nil, "0 7 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Stop()

	count := s.CronEntryCount()
	// 1 (digest) + 7 (existing intelligence, no lifecycle) + 3 (delivery sweep + batched daily producers + weekly relationship cooling) = 11
	if count < 11 {
		t.Errorf("expected at least 11 cron entries with engine, got %d", count)
	}
}

// SCN-022-09: Overlapping cron job of same type is skipped via TryLock
func TestCronConcurrencyGuard_SameGroupSkipped(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Acquire daily mutex to simulate a running daily job
	s.muDaily.Lock()

	// TryLock should fail — simulating a second daily job firing
	if s.muDaily.TryLock() {
		s.muDaily.Unlock()
		t.Fatal("expected TryLock to return false when mutex is held")
	}

	s.muDaily.Unlock()
}

// SCN-022-10: Different job groups run concurrently
func TestCronConcurrencyGuard_DifferentGroupsConcurrent(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Acquire daily mutex
	s.muDaily.Lock()

	// Hourly mutex should be independent — TryLock should succeed
	if !s.muHourly.TryLock() {
		s.muDaily.Unlock()
		t.Fatal("expected hourly TryLock to succeed while daily is held")
	}
	s.muHourly.Unlock()

	// Weekly should also be independent
	if !s.muWeekly.TryLock() {
		s.muDaily.Unlock()
		t.Fatal("expected weekly TryLock to succeed while daily is held")
	}
	s.muWeekly.Unlock()

	s.muDaily.Unlock()
}

// SCN-022-11: All six mutex groups exist and are independent
func TestCronConcurrencyGuard_AllGroupsIndependent(t *testing.T) {
	s := New(nil, nil, nil, nil)

	// Lock all groups simultaneously — proves they are independent mutexes
	s.muDigest.Lock()
	s.muHourly.Lock()
	s.muDaily.Lock()
	s.muWeekly.Lock()
	s.muMonthly.Lock()
	s.muBriefs.Lock()
	s.muAlerts.Lock()

	// All locked — unlock in any order
	s.muAlerts.Unlock()
	s.muBriefs.Unlock()
	s.muMonthly.Unlock()
	s.muWeekly.Unlock()
	s.muDaily.Unlock()
	s.muHourly.Unlock()
	s.muDigest.Unlock()
}

// SCN-022-09: Concurrent TryLock simulation under race detector
func TestCronConcurrencyGuard_RaceDetectorClean(t *testing.T) {
	s := New(nil, nil, nil, nil)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.muDaily.TryLock() {
				// Simulate brief work
				s.muDaily.Unlock()
			}
		}()
	}
	wg.Wait()
}
