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
