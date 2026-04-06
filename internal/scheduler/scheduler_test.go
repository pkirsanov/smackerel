package scheduler

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New(nil, nil)
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
	s := New(nil, nil)
	err := s.Start(nil, "invalid-cron-expression")
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestStart_ValidCron(t *testing.T) {
	s := New(nil, nil)
	// This will succeed but the cron job will fail on nil digestGen when triggered
	err := s.Start(nil, "0 7 * * *")
	if err != nil {
		t.Fatalf("expected no error for valid cron: %v", err)
	}
	s.Stop()
}

func TestStop(t *testing.T) {
	s := New(nil, nil)
	s.Start(nil, "0 0 * * *")
	// Stop should not panic
	s.Stop()
}
