package connector

import (
	"testing"
	"time"
)

func TestBackoff_Exponential(t *testing.T) {
	b := DefaultBackoff()

	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}

	for i, exp := range expected {
		delay, ok := b.Next()
		if !ok {
			t.Fatalf("attempt %d: expected more retries", i)
		}

		// Allow ±50% jitter
		minDelay := exp / 2
		maxDelay := exp * 2
		if delay < minDelay || delay > maxDelay {
			t.Errorf("attempt %d: delay %v outside expected range [%v, %v]", i, delay, minDelay, maxDelay)
		}
	}
}

func TestBackoff_MaxRetries(t *testing.T) {
	b := DefaultBackoff()

	for i := 0; i < 5; i++ {
		_, ok := b.Next()
		if !ok {
			t.Fatalf("attempt %d should have more retries", i)
		}
	}

	_, ok := b.Next()
	if ok {
		t.Error("should be exhausted after max retries")
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := DefaultBackoff()

	for i := 0; i < 3; i++ {
		b.Next()
	}

	b.Reset()
	if b.Attempt() != 0 {
		t.Errorf("expected attempt 0 after reset, got %d", b.Attempt())
	}

	_, ok := b.Next()
	if !ok {
		t.Error("should have retries after reset")
	}
}

func TestBackoff_AttemptCounter(t *testing.T) {
	b := DefaultBackoff()

	for i := 0; i < 5; i++ {
		if b.Attempt() != i {
			t.Errorf("before attempt %d: expected Attempt()=%d, got %d", i, i, b.Attempt())
		}
		b.Next()
	}
	if b.Attempt() != 5 {
		t.Errorf("after all attempts: expected Attempt()=5, got %d", b.Attempt())
	}
}

func TestBackoff_ZeroMaxRetries(t *testing.T) {
	b := &Backoff{
		BaseDelay:  1 * time.Second,
		MaxDelay:   16 * time.Second,
		MaxRetries: 0,
	}

	_, ok := b.Next()
	if ok {
		t.Error("backoff with MaxRetries=0 should be immediately exhausted")
	}
}

func TestBackoff_MaxDelayCap(t *testing.T) {
	b := &Backoff{
		BaseDelay:  1 * time.Second,
		MaxDelay:   4 * time.Second,
		MaxRetries: 10,
	}

	// After attempt 2, base delay would be 4s; attempt 3 would be 8s uncapped
	// With MaxDelay=4s, delays should never exceed 4s + jitter
	for i := 0; i < 5; i++ {
		delay, ok := b.Next()
		if !ok {
			t.Fatalf("attempt %d: should still have retries", i)
		}
		// With ±25% jitter on a 4s max, the absolute max is 5s
		if delay > 5*time.Second {
			t.Errorf("attempt %d: delay %v exceeds MaxDelay cap (4s + jitter)", i, delay)
		}
	}
}

func TestBackoff_CustomParameters(t *testing.T) {
	b := &Backoff{
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   2 * time.Second,
		MaxRetries: 3,
	}

	count := 0
	for {
		_, ok := b.Next()
		if !ok {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected exactly 3 retries with MaxRetries=3, got %d", count)
	}
}

func TestBackoff_ResetAfterExhaustion(t *testing.T) {
	b := DefaultBackoff()

	// Exhaust all retries
	for i := 0; i < 5; i++ {
		b.Next()
	}
	_, ok := b.Next()
	if ok {
		t.Fatal("should be exhausted")
	}

	// Reset and verify full retry budget is restored
	b.Reset()
	if b.Attempt() != 0 {
		t.Errorf("expected attempt 0 after reset, got %d", b.Attempt())
	}
	for i := 0; i < 5; i++ {
		_, ok := b.Next()
		if !ok {
			t.Fatalf("attempt %d should work after reset", i)
		}
	}
}
