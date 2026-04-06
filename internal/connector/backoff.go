package connector

import (
	"math"
	"math/rand"
	"time"
)

// Backoff implements exponential backoff with jitter for rate-limit handling.
type Backoff struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
	attempt    int
}

// DefaultBackoff returns a backoff policy suitable for API rate limits.
func DefaultBackoff() *Backoff {
	return &Backoff{
		BaseDelay:  1 * time.Second,
		MaxDelay:   16 * time.Second,
		MaxRetries: 5,
	}
}

// Next returns the next delay duration and whether retries remain.
func (b *Backoff) Next() (time.Duration, bool) {
	if b.attempt >= b.MaxRetries {
		return 0, false
	}

	// Exponential: 1s, 2s, 4s, 8s, 16s
	delay := time.Duration(float64(b.BaseDelay) * math.Pow(2, float64(b.attempt)))
	if delay > b.MaxDelay {
		delay = b.MaxDelay
	}

	// Add jitter: ±25%
	jitter := time.Duration(rand.Int63n(int64(delay) / 2)) - time.Duration(int64(delay)/4)
	delay += jitter

	b.attempt++
	return delay, true
}

// Reset resets the backoff counter.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number.
func (b *Backoff) Attempt() int {
	return b.attempt
}
